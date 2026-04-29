package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"

	"github.com/spf13/cobra"

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

	return cmd
}

func recordCmd() *cobra.Command {
	var output string
	var targets []string

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

			return runner.Record(ctx, runner.RecordOptions{
				Command:    args,
				OutputPath: output,
				Targets:    targets,
			})
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output trace file path (default: trace-<timestamp>.json)")
	cmd.Flags().StringSliceVar(&targets, "target", nil, "LLM API hosts to intercept (default: all)")

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
