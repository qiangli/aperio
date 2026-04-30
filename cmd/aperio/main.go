package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qiangli/aperio/internal/eval"
	"github.com/qiangli/aperio/internal/runner"
	"github.com/qiangli/aperio/internal/trace"
	"github.com/qiangli/aperio/internal/viewer"
)

var version = "dev"

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "aperio",
		Short: "Execution tracing and visualization for AI agents",
		Long: `Aperio records, replays, and visualizes execution traces of AI agentic applications.
It captures internal function calls, LLM API interactions, and tool invocations,
then renders the execution graph as an interactive HTML page.`,
		Version: version,
	}

	cmd.AddCommand(recordCmd())
	cmd.AddCommand(replayCmd())
	cmd.AddCommand(viewCmd())
	cmd.AddCommand(diffCmd())
	cmd.AddCommand(graphCmd())
	cmd.AddCommand(evalsCmd())

	return cmd
}

func recordCmd() *cobra.Command {
	var output string
	var targets []string
	var emitGraph bool

	cmd := &cobra.Command{
		Use:   "record [flags] -- <command> [args...]",
		Short: "Record an agent execution trace",
		Long:  `Run an agent with full tracing enabled and save the execution trace.`,
		Example: `  aperio record -- python agent.py "What is the weather?"
  aperio record --output trace.json -- node agent.js "Summarize this"
  aperio record --target api.openai.com -- go run ./cmd/agent "Hello"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			err := runner.Record(ctx, runner.RecordOptions{
				Command:    args,
				OutputPath: output,
				Targets:    targets,
			})
			if err != nil {
				return err
			}

			if emitGraph {
				tracePath := output
				if tracePath == "" {
					// The runner will have generated a default path; we need to find it
					// Use the same default pattern the runner uses
					return fmt.Errorf("--graph requires --output to be specified")
				}
				t, err := trace.ReadTrace(tracePath)
				if err != nil {
					return fmt.Errorf("read trace for graph: %w", err)
				}
				g := trace.BuildGraph(t)
				graphPath := strings.TrimSuffix(tracePath, ".json") + "-graph.json"
				if err := trace.WriteGraph(graphPath, g); err != nil {
					return fmt.Errorf("write graph: %w", err)
				}
				fmt.Printf("Graph saved to: %s\n", graphPath)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output trace file path (default: trace-<timestamp>.json)")
	cmd.Flags().StringSliceVar(&targets, "target", nil, "LLM API hosts to intercept (default: all)")
	cmd.Flags().BoolVar(&emitGraph, "graph", false, "also emit graph JSON alongside the trace")

	return cmd
}

func replayCmd() *cobra.Command {
	var input string
	var output string
	var strict bool
	var matchStrategy string

	cmd := &cobra.Command{
		Use:   "replay [flags] -- <command> [args...]",
		Short: "Replay an agent execution with mocked LLM responses",
		Long:  `Re-run an agent with recorded LLM responses for deterministic testing.`,
		Example: `  aperio replay --input trace.json -- python agent.py "What is the weather?"
  aperio replay --input trace.json --strict -- node agent.js "Summarize this"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			return runner.Replay(ctx, runner.ReplayOptions{
				Command:       args,
				InputPath:     input,
				OutputPath:    output,
				Strict:        strict,
				MatchStrategy: matchStrategy,
			})
		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", "input trace file path (required)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "output replay trace file path")
	cmd.Flags().BoolVar(&strict, "strict", false, "fail on unmatched requests")
	cmd.Flags().StringVar(&matchStrategy, "match-strategy", "sequential", "request matching strategy: sequential or fingerprint")
	cmd.MarkFlagRequired("input")

	return cmd
}

func viewCmd() *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:     "view <trace-file>",
		Short:   "Visualize an execution trace as an interactive HTML graph",
		Example: `  aperio view trace.json`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url, err := viewer.Serve(viewer.Options{
				Port:      port,
				TraceFile: args[0],
			})
			if err != nil {
				return err
			}

			fmt.Printf("Viewer running at: %s\n", url)
			fmt.Println("Press Ctrl+C to stop")

			// Try to open browser
			openBrowser(url)

			// Wait for interrupt
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()
			<-ctx.Done()
			return nil
		},
	}

	cmd.Flags().IntVar(&port, "port", 0, "port for viewer server (default: random)")

	return cmd
}

func diffCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "diff <trace1> <trace2>",
		Short:   "Compare two execution traces and show differences",
		Example: `  aperio diff recorded.json replayed.json`,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			left, err := trace.ReadTrace(args[0])
			if err != nil {
				return fmt.Errorf("read left trace: %w", err)
			}
			right, err := trace.ReadTrace(args[1])
			if err != nil {
				return fmt.Errorf("read right trace: %w", err)
			}

			diffs := trace.Diff(left, right)
			fmt.Println(trace.FormatDiff(diffs))
			return nil
		},
	}
}

func openBrowser(url string) {
	// Best-effort: try common browser openers
	for _, cmd := range [][]string{
		{"open", url},          // macOS
		{"xdg-open", url},     // Linux
		{"cmd", "/c", "start", url}, // Windows
	} {
		if c := exec.Command(cmd[0], cmd[1:]...); c.Start() == nil {
			return
		}
	}
}

func graphCmd() *cobra.Command {
	var output string
	var toStdout bool

	cmd := &cobra.Command{
		Use:   "graph <trace-file>",
		Short: "Convert a trace file to an explicit graph JSON representation",
		Example: `  aperio graph trace.json
  aperio graph trace.json -o graph.json
  aperio graph trace.json --stdout`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := trace.ReadTrace(args[0])
			if err != nil {
				return fmt.Errorf("read trace: %w", err)
			}

			g := trace.BuildGraph(t)

			if toStdout {
				data, err := json.MarshalIndent(g, "", "  ")
				if err != nil {
					return fmt.Errorf("marshal graph: %w", err)
				}
				fmt.Println(string(data))
				return nil
			}

			if output == "" {
				base := strings.TrimSuffix(filepath.Base(args[0]), ".json")
				output = base + "-graph.json"
			}

			if err := trace.WriteGraph(output, g); err != nil {
				return fmt.Errorf("write graph: %w", err)
			}

			fmt.Printf("Graph written to: %s (%d nodes, %d edges)\n",
				output, g.Stats.NodeCount, g.Stats.EdgeCount)
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output graph file path")
	cmd.Flags().BoolVar(&toStdout, "stdout", false, "write graph JSON to stdout")

	return cmd
}

func evalsCmd() *cobra.Command {
	var format string
	var behavioralOnly bool
	var threshold float64
	var verbose bool

	cmd := &cobra.Command{
		Use:   "evals <graph1.json> <graph2.json>",
		Short: "Compare two trace graphs and compute similarity scores",
		Long: `Evaluate how similar two execution traces are using tree edit distance.
Accepts either graph JSON files (from 'aperio graph') or trace JSON files.`,
		Example: `  aperio evals recorded-graph.json replayed-graph.json
  aperio evals trace1.json trace2.json --format json
  aperio evals a.json b.json --threshold 0.8`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			left, err := loadGraph(args[0])
			if err != nil {
				return fmt.Errorf("load left: %w", err)
			}
			right, err := loadGraph(args[1])
			if err != nil {
				return fmt.Errorf("load right: %w", err)
			}

			result := eval.Evaluate(left, right, &eval.EvalConfig{
				BehavioralOnly: behavioralOnly,
			})

			switch format {
			case "json":
				out, err := eval.FormatJSON(result)
				if err != nil {
					return err
				}
				fmt.Println(out)
			default:
				fmt.Print(eval.FormatText(result, verbose))
			}

			if threshold > 0 && result.OverallSimilarity < threshold {
				return fmt.Errorf("similarity %.3f is below threshold %.3f",
					result.OverallSimilarity, threshold)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")
	cmd.Flags().BoolVar(&behavioralOnly, "behavioral-only", false, "only compute behavioral similarity")
	cmd.Flags().Float64Var(&threshold, "threshold", 0, "fail if similarity is below this value")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "include full edit script in output")

	return cmd
}

// loadGraph loads a graph from either a graph JSON file or a trace JSON file.
func loadGraph(path string) (*trace.TraceGraph, error) {
	// Try loading as graph first
	g, err := trace.ReadGraph(path)
	if err == nil && g.TraceID != "" {
		return g, nil
	}

	// Fall back to loading as trace and converting
	t, err := trace.ReadTrace(path)
	if err != nil {
		return nil, fmt.Errorf("read file %s (tried as graph and trace): %w", path, err)
	}

	return trace.BuildGraph(t), nil
}
