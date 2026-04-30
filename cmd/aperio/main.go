package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qiangli/aperio/internal/benchmark"
	_ "github.com/qiangli/aperio/internal/benchmark/adapters" // register benchmark adapters
	"github.com/qiangli/aperio/internal/eval"
	"github.com/qiangli/aperio/internal/export"
	"github.com/qiangli/aperio/internal/proxy"
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
	cmd.AddCommand(exportCmd())
	cmd.AddCommand(mergeTracesCmd())
	cmd.AddCommand(benchmarkCmd())
	cmd.AddCommand(compareCmd())
	cmd.AddCommand(leaderboardCmd())
	cmd.AddCommand(metricsCmd())

	return cmd
}

func recordCmd() *cobra.Command {
	var output string
	var targets []string
	var emitGraph bool
	var goTracer string
	var correlationID string
	var agentRole string
	var redactConfig string
	var redactBuiltin bool

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

			// Build redactor if configured
			var redactor *proxy.Redactor
			if redactConfig != "" || redactBuiltin {
				var cfg proxy.RedactionConfig
				if redactConfig != "" {
					loaded, err := proxy.LoadRedactionConfig(redactConfig)
					if err != nil {
						return fmt.Errorf("load redaction config: %w", err)
					}
					cfg = *loaded
				}
				if redactBuiltin {
					cfg.BuiltinPII = true
				}
				var err error
				redactor, err = proxy.NewRedactor(cfg)
				if err != nil {
					return fmt.Errorf("create redactor: %w", err)
				}
			}

			err := runner.Record(ctx, runner.RecordOptions{
				Command:         args,
				OutputPath:      output,
				Targets:         targets,
				GoTracerBackend: goTracer,
				CorrelationID:   correlationID,
				AgentRole:       agentRole,
				Redactor:        redactor,
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
	cmd.Flags().StringVar(&goTracer, "go-tracer", "dlv", "Go tracer backend: dlv or runtime")
	cmd.Flags().StringVar(&correlationID, "correlation-id", "", "correlation ID for multi-agent tracing")
	cmd.Flags().StringVar(&agentRole, "agent-role", "", "role label for this agent (e.g., orchestrator, coder)")
	cmd.Flags().StringVar(&redactConfig, "redact", "", "path to YAML redaction config file")
	cmd.Flags().BoolVar(&redactBuiltin, "redact-builtin", false, "enable built-in PII redaction patterns (email, phone, IP, etc.)")

	return cmd
}

func replayCmd() *cobra.Command {
	var input string
	var output string
	var strict bool
	var matchStrategy string
	var cassettePath string

	cmd := &cobra.Command{
		Use:   "replay [flags] -- <command> [args...]",
		Short: "Replay an agent execution with mocked LLM responses",
		Long:  `Re-run an agent with recorded LLM responses for deterministic testing.`,
		Example: `  aperio replay --input trace.json -- python agent.py "What is the weather?"
  aperio replay --input trace.json --strict -- node agent.js "Summarize this"
  aperio replay --cassette recording.yaml --match-strategy body -- python agent.py`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if input == "" && cassettePath == "" {
				return fmt.Errorf("either --input or --cassette is required")
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			return runner.Replay(ctx, runner.ReplayOptions{
				Command:       args,
				InputPath:     input,
				OutputPath:    output,
				Strict:        strict,
				MatchStrategy: matchStrategy,
				CassettePath:  cassettePath,
			})
		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", "input trace file path")
	cmd.Flags().StringVarP(&output, "output", "o", "", "output replay trace file path")
	cmd.Flags().BoolVar(&strict, "strict", false, "fail on unmatched requests")
	cmd.Flags().StringVar(&matchStrategy, "match-strategy", "sequential", "request matching strategy: sequential, fingerprint, or body")
	cmd.Flags().StringVar(&cassettePath, "cassette", "", "load replay data from YAML cassette file")

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
	var viewFlag bool

	cmd := &cobra.Command{
		Use:     "diff <trace1> <trace2>",
		Short:   "Compare two execution traces and show differences",
		Example: `  aperio diff recorded.json replayed.json
  aperio diff --view recorded.json replayed.json`,
		Args: cobra.ExactArgs(2),
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

			if viewFlag {
				url, err := viewer.ServeDiff(viewer.DiffOptions{
					Port:       0,
					LeftFile:   args[0],
					RightFile:  args[1],
					DiffResult: diffs,
				})
				if err != nil {
					return fmt.Errorf("start diff viewer: %w", err)
				}
				fmt.Printf("Diff viewer: %s\n", url)
				openBrowser(url)

				// Block until interrupted
				ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
				defer cancel()
				<-ctx.Done()
				return nil
			}

			fmt.Println(trace.FormatDiff(diffs))
			return nil
		},
	}

	cmd.Flags().BoolVar(&viewFlag, "view", false, "open side-by-side diff viewer in browser")

	return cmd
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
	var semantic bool
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
				BehavioralOnly:         behavioralOnly,
				IncludeSemanticMetrics: semantic,
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
	cmd.Flags().BoolVar(&semantic, "semantic", false, "include semantic text metrics (BLEU, ROUGE, etc.)")
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

func exportCmd() *cobra.Command {
	var endpoint string
	var dryRun bool
	var serviceName string
	var authToken string
	var apiKey string
	var compress bool

	cmd := &cobra.Command{
		Use:   "export <trace-file>",
		Short: "Export a trace to an OTLP-compatible endpoint",
		Long: `Convert an Aperio trace to OpenTelemetry Protocol (OTLP) JSON format
and send it to an observability backend like Arize Phoenix, Jaeger, or Grafana Tempo.`,
		Example: `  aperio export trace.json --endpoint http://localhost:6006/v1/traces
  aperio export trace.json --dry-run
  aperio export trace.json --service-name my-agent`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := trace.ReadTrace(args[0])
			if err != nil {
				return fmt.Errorf("read trace: %w", err)
			}

			otlpReq := export.ConvertTrace(t, serviceName)

			if dryRun {
				out, err := export.FormatJSON(otlpReq)
				if err != nil {
					return err
				}
				fmt.Println(out)
				return nil
			}

			if err := export.Send(otlpReq, export.SendOptions{
				Endpoint:  endpoint,
				AuthToken: authToken,
				APIKey:    apiKey,
				Compress:  compress,
			}); err != nil {
				return fmt.Errorf("export: %w", err)
			}

			fmt.Printf("Exported %d spans to %s\n", len(t.Spans), endpoint)
			return nil
		},
	}

	cmd.Flags().StringVar(&endpoint, "endpoint", "http://localhost:6006/v1/traces", "OTLP endpoint URL")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print OTLP JSON to stdout without sending")
	cmd.Flags().StringVar(&serviceName, "service-name", "aperio", "resource service name")
	cmd.Flags().StringVar(&authToken, "auth-token", "", "Bearer token for OTLP endpoint authentication")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key for OTLP endpoint authentication")
	cmd.Flags().BoolVar(&compress, "compress", false, "enable gzip compression of OTLP payload")

	return cmd
}

func mergeTracesCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "merge-traces <trace1.json> <trace2.json> [trace3.json...]",
		Short: "Merge correlated multi-agent traces into a single DAG",
		Long: `Combine multiple trace files from correlated agent processes into a unified
trace that shows the complete multi-agent execution as a single graph.`,
		Example: `  aperio merge-traces orchestrator.json coder.json reviewer.json -o combined.json
  aperio merge-traces traces/*.json -o merged.json`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var traces []*trace.Trace
			for _, path := range args {
				t, err := trace.ReadTrace(path)
				if err != nil {
					return fmt.Errorf("read %s: %w", path, err)
				}
				traces = append(traces, t)
			}

			merged, err := trace.MergeTraces(traces)
			if err != nil {
				return fmt.Errorf("merge: %w", err)
			}

			if output == "" {
				output = "merged-trace.json"
			}

			if err := trace.WriteTrace(output, merged); err != nil {
				return fmt.Errorf("write merged trace: %w", err)
			}

			fmt.Printf("Merged %d traces (%d total spans) → %s\n",
				len(traces), len(merged.Spans), output)
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output file (default: merged-trace.json)")

	return cmd
}

func benchmarkCmd() *cobra.Command {
	var outputDir string
	var runs int
	var format string
	var pricingFile string
	var passK string
	var regression bool
	var saveBaseline string

	cmd := &cobra.Command{
		Use:   "benchmark <spec.yaml>",
		Short: "Run a benchmark comparing multiple AI coding tools",
		Long: `Execute a benchmark specification that runs the same task against multiple
tools and produces a comparison report with rankings.`,
		Example: `  aperio benchmark spec.yaml
  aperio benchmark spec.yaml --runs 5 --format html
  aperio benchmark spec.yaml --pricing custom-pricing.yaml
  aperio benchmark spec.yaml --pass-k 1,3,5 --runs 10
  aperio benchmark spec.yaml --regression --save-baseline baselines/current.json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, err := benchmark.ParseSpec(args[0])
			if err != nil {
				return fmt.Errorf("parse spec: %w", err)
			}

			// Apply flag overrides
			if outputDir != "" {
				spec.OutputDir = outputDir
			}
			if runs > 0 {
				spec.Runs = runs
			}

			// Parse pass@k values from flag
			var passKValues []int
			if passK != "" {
				for _, s := range strings.Split(passK, ",") {
					var k int
					if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &k); err == nil && k > 0 {
						passKValues = append(passKValues, k)
					}
				}
			}
			// Merge with spec-level pass_k
			if len(spec.PassK) > 0 && len(passKValues) == 0 {
				passKValues = spec.PassK
			}

			// Load pricing
			pricing := benchmark.DefaultPricing()
			if pricingFile != "" {
				custom, err := benchmark.LoadPricing(pricingFile)
				if err != nil {
					return fmt.Errorf("load pricing: %w", err)
				}
				pricing = benchmark.MergePricing(pricing, custom)
			}
			if len(spec.Pricing) > 0 {
				overlay := benchmark.PricingTable{Models: spec.Pricing}
				pricing = benchmark.MergePricing(pricing, overlay)
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			// Run benchmark
			results, err := benchmark.Run(ctx, spec)
			if err != nil {
				return fmt.Errorf("benchmark: %w", err)
			}

			// Generate comparison (with pass@k if specified)
			var comparison *benchmark.ComparisonResult
			if len(passKValues) > 0 {
				comparison = benchmark.CompareWithPassK(results, pricing, passKValues)
			} else {
				comparison = benchmark.Compare(results, pricing)
			}

			// Save baseline if requested
			if saveBaseline != "" {
				if err := benchmark.SaveBaseline(saveBaseline, comparison); err != nil {
					return fmt.Errorf("save baseline: %w", err)
				}
				fmt.Printf("Baseline saved to: %s\n", saveBaseline)
			}

			// Check regression if enabled
			if regression || spec.Regression != nil {
				regConfig := spec.Regression
				if regConfig == nil {
					regConfig = &benchmark.RegressionConfig{}
				}
				baselinePath := regConfig.BaselinePath
				if baselinePath == "" {
					baselinePath = filepath.Join(spec.OutputDir, "baseline.json")
				}
				regResult, err := benchmark.CheckRegression(comparison, baselinePath, regConfig)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: regression check failed: %v\n", err)
				} else if regResult.Regressed {
					fmt.Fprintf(os.Stderr, "\n⚠ REGRESSION DETECTED:\n")
					for _, d := range regResult.Details {
						if d.Regressed {
							fmt.Fprintf(os.Stderr, "  %s/%s: %.2f → %.2f (%.1f%% change)\n",
								d.Tool, d.Metric, d.Baseline, d.Current, d.DeltaPct*100)
						}
					}
					if regConfig.FailOnRegression {
						defer func() { os.Exit(1) }()
					}
				}
			}

			// Update leaderboard
			lbPath := filepath.Join(spec.OutputDir, "leaderboard.json")
			lb, _ := benchmark.LoadLeaderboard(lbPath)
			entry := benchmark.ComparisonToEntry(spec.Name, comparison)
			lb.AddEntry(entry)
			benchmark.SaveLeaderboard(lbPath, lb)

			// Output report
			reportDir := spec.OutputDir
			switch format {
			case "json":
				outPath := filepath.Join(reportDir, "report.json")
				if err := benchmark.GenerateJSON(comparison, outPath); err != nil {
					return err
				}
				fmt.Printf("Report saved to: %s\n", outPath)
			case "csv":
				outPath := filepath.Join(reportDir, "report.csv")
				if err := benchmark.GenerateCSV(comparison, outPath); err != nil {
					return err
				}
				fmt.Printf("Report saved to: %s\n", outPath)
			case "all":
				for _, ext := range []string{"html", "csv", "json"} {
					outPath := filepath.Join(reportDir, "report."+ext)
					switch ext {
					case "html":
						benchmark.GenerateHTML(comparison, outPath)
					case "csv":
						benchmark.GenerateCSV(comparison, outPath)
					case "json":
						benchmark.GenerateJSON(comparison, outPath)
					}
				}
				fmt.Printf("Reports saved to: %s/report.{html,csv,json}\n", reportDir)
			default: // html
				outPath := filepath.Join(reportDir, "report.html")
				if err := benchmark.GenerateHTML(comparison, outPath); err != nil {
					return err
				}
				fmt.Printf("Report saved to: %s\n", outPath)
			}

			// Also print text summary
			fmt.Print(benchmark.FormatText(comparison))
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputDir, "output-dir", "o", "", "override output directory")
	cmd.Flags().IntVar(&runs, "runs", 0, "override number of runs per tool")
	cmd.Flags().StringVar(&format, "format", "html", "report format: html, csv, json, all")
	cmd.Flags().StringVar(&pricingFile, "pricing", "", "path to custom pricing YAML")
	cmd.Flags().StringVar(&passK, "pass-k", "", "compute pass@k metrics (comma-separated, e.g., 1,3,5)")
	cmd.Flags().BoolVar(&regression, "regression", false, "check for performance regressions against baseline")
	cmd.Flags().StringVar(&saveBaseline, "save-baseline", "", "save current results as baseline to this path")

	cmd.AddCommand(listTasksCmd())

	return cmd
}

func listTasksCmd() *cobra.Command {
	var adapterName string
	var limit int
	var difficulty string
	var languages string

	cmd := &cobra.Command{
		Use:   "list-tasks",
		Short: "List available tasks from a benchmark adapter",
		Example: `  aperio benchmark list-tasks --adapter swe-bench
  aperio benchmark list-tasks --adapter exercism --languages python,go --limit 20`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if adapterName == "" {
				return fmt.Errorf("--adapter is required")
			}

			spec := &benchmark.AdapterSpec{Name: adapterName}
			adapter, err := benchmark.GetAdapter(spec)
			if err != nil {
				return err
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			if err := adapter.Setup(ctx); err != nil {
				return fmt.Errorf("adapter setup: %w", err)
			}

			filter := benchmark.AdapterFilter{
				Limit:      limit,
				Difficulty: difficulty,
			}
			if languages != "" {
				filter.Languages = strings.Split(languages, ",")
			}

			tasks, err := adapter.ListTasks(filter)
			if err != nil {
				return err
			}

			fmt.Printf("Found %d tasks from adapter %q:\n", len(tasks), adapterName)
			for _, id := range tasks {
				fmt.Printf("  %s\n", id)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&adapterName, "adapter", "", "adapter name (swe-bench, exercism, terminal-bench)")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of tasks to list")
	cmd.Flags().StringVar(&difficulty, "difficulty", "", "filter by difficulty (easy, medium, hard)")
	cmd.Flags().StringVar(&languages, "languages", "", "filter by languages (comma-separated)")

	return cmd
}

func compareCmd() *cobra.Command {
	var names string
	var format string
	var output string
	var pricingFile string

	cmd := &cobra.Command{
		Use:   "compare <trace1.json> <trace2.json> [trace3.json...]",
		Short: "Compare multiple traces and produce rankings",
		Long: `Perform an N-way comparison of execution traces from different tools,
computing metrics, rankings, and pairwise similarity.`,
		Example: `  aperio compare tool-a.json tool-b.json tool-c.json
  aperio compare *.json --names "ycode,aider,cline" --format html -o report.html`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load traces
			var traces []*trace.Trace
			for _, path := range args {
				t, err := trace.ReadTrace(path)
				if err != nil {
					return fmt.Errorf("read %s: %w", path, err)
				}
				traces = append(traces, t)
			}

			// Determine tool names
			var toolNames []string
			if names != "" {
				toolNames = strings.Split(names, ",")
				if len(toolNames) != len(traces) {
					return fmt.Errorf("--names count (%d) must match trace count (%d)", len(toolNames), len(traces))
				}
			} else {
				for _, path := range args {
					toolNames = append(toolNames, strings.TrimSuffix(filepath.Base(path), ".json"))
				}
			}

			// Load pricing
			pricing := benchmark.DefaultPricing()
			if pricingFile != "" {
				custom, err := benchmark.LoadPricing(pricingFile)
				if err != nil {
					return fmt.Errorf("load pricing: %w", err)
				}
				pricing = benchmark.MergePricing(pricing, custom)
			}

			// Compare
			comparison := benchmark.CompareTraces(toolNames, traces, pricing)

			// Output
			switch format {
			case "json":
				if output == "" {
					data, _ := json.MarshalIndent(comparison, "", "  ")
					fmt.Println(string(data))
				} else {
					benchmark.GenerateJSON(comparison, output)
					fmt.Printf("Report saved to: %s\n", output)
				}
			case "csv":
				if output == "" {
					output = "comparison.csv"
				}
				benchmark.GenerateCSV(comparison, output)
				fmt.Printf("Report saved to: %s\n", output)
			case "html":
				if output == "" {
					output = "comparison.html"
				}
				benchmark.GenerateHTML(comparison, output)
				fmt.Printf("Report saved to: %s\n", output)
			default:
				fmt.Print(benchmark.FormatText(comparison))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&names, "names", "", "comma-separated tool names (default: filenames)")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text, html, csv, json")
	cmd.Flags().StringVarP(&output, "output", "o", "", "output file path")
	cmd.Flags().StringVar(&pricingFile, "pricing", "", "path to custom pricing YAML")

	return cmd
}

func leaderboardCmd() *cobra.Command {
	var clear bool
	var format string
	var lbPath string

	cmd := &cobra.Command{
		Use:   "leaderboard",
		Short: "View accumulated benchmark rankings over time",
		Long: `Display the leaderboard of accumulated benchmark results showing
all-time best scores, recent entries, and trends per tool.`,
		Example: `  aperio leaderboard
  aperio leaderboard --path ./results/leaderboard.json
  aperio leaderboard --format json
  aperio leaderboard --clear`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if lbPath == "" {
				lbPath = filepath.Join("benchmark-results", "leaderboard.json")
			}

			lb, err := benchmark.LoadLeaderboard(lbPath)
			if err != nil {
				return fmt.Errorf("load leaderboard: %w", err)
			}

			if clear {
				lb.Clear()
				if err := benchmark.SaveLeaderboard(lbPath, lb); err != nil {
					return fmt.Errorf("save leaderboard: %w", err)
				}
				fmt.Println("Leaderboard cleared.")
				return nil
			}

			switch format {
			case "json":
				data, _ := json.MarshalIndent(lb, "", "  ")
				fmt.Println(string(data))
			default:
				fmt.Print(benchmark.FormatLeaderboard(lb))
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&clear, "clear", false, "reset the leaderboard")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")
	cmd.Flags().StringVar(&lbPath, "path", "", "leaderboard file path")

	return cmd
}

func metricsCmd() *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "metrics <trace-file> [trace-file...]",
		Short: "Start a Prometheus metrics endpoint for trace data",
		Long: `Load one or more trace files and expose agent performance metrics
via a Prometheus-compatible /metrics HTTP endpoint.`,
		Example: `  aperio metrics trace.json --port 9090
  aperio metrics traces/*.json`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			collector := export.NewMetricsCollector()

			for _, path := range args {
				t, err := trace.ReadTrace(path)
				if err != nil {
					return fmt.Errorf("read trace %s: %w", path, err)
				}
				collector.RecordTrace(t)
				fmt.Printf("Loaded trace: %s (%d spans)\n", path, len(t.Spans))
			}

			addr := fmt.Sprintf(":%d", port)
			http.Handle("/metrics", collector.Handler())
			fmt.Printf("Metrics endpoint: http://localhost:%d/metrics\n", port)
			return http.ListenAndServe(addr, nil)
		},
	}

	cmd.Flags().IntVar(&port, "port", 9090, "port for metrics HTTP server")

	return cmd
}
