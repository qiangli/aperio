# Aperio

Aperio records, replays, and visualizes execution traces of AI agent applications. It captures both internal program execution and outbound LLM API calls **without requiring any code changes to the target agent**, then merges them into a single trace you can view, diff, evaluate, and export to standard observability backends.

## Why

Debugging and benchmarking AI agents is hard because the interesting behavior is split across two layers: the agent's own code (function calls, tool invocations, control flow) and its conversations with an LLM provider over HTTPS. Aperio captures both layers simultaneously and correlates them by timestamp, giving you a single causal view of what the agent did and what the model told it to do.

## Install

```bash
git clone https://github.com/qiangli/aperio
cd aperio
make install      # installs to $GOPATH/bin
# or
make build        # produces ./bin/aperio
```

Requires Go 1.25+. For Go agent tracing in `dlv` mode, install Delve: `go install github.com/go-delve/delve/cmd/dlv@latest`.

## Quickstart

```bash
# Record a Python agent (Node.js and Go also supported)
aperio record -o trace.json -- python agent.py

# Open the interactive viewer (DAG, Gantt timeline, cost panel)
aperio view trace.json

# Replay deterministically against the recorded LLM responses
aperio replay -i trace.json -- python agent.py

# Compare two runs (Zhang–Shasha tree edit distance + semantic text metrics)
aperio evals run-a.graph.json run-b.graph.json --semantic

# Export to any OTLP backend (Phoenix, Jaeger, Tempo, ...)
aperio export trace.json --endpoint http://localhost:4318/v1/traces
```

## How it works

Aperio uses a two-layer tracing model:

1. **Internal execution tracing** is language-specific:
   - **Python** — `sitecustomize.py` injected via `PYTHONPATH` hooks `sys.settrace()`
   - **Node.js** — `--require node_tracer.js` monkey-patches `Module.prototype.require`
   - **Go** — `dlv trace` (default) or `runtime/trace` via `-overlay` injection
2. **LLM API capture** is language-agnostic — a forward HTTPS proxy (set as `HTTPS_PROXY`) MITM-decrypts and records request/response pairs to OpenAI, Anthropic, Gemini, etc.

After capture, traces flow through **Merge** (correlate function and API spans by timestamp) → **Enrich** (extract semantic spans like `TOOL_CALL`, `USER_INPUT` from LLM payloads) → **Filter** (drop stdlib noise).

| Mode | How | Use case |
|------|-----|----------|
| Source (Python) | `sitecustomize.py` via `PYTHONPATH` | Python agents with source |
| Source (Node.js) | `--require node_tracer.js` | Node/TS agents with source |
| Source (Go/dlv) | Wrap with `dlv trace` | Function-level Go tracing |
| Source (Go/runtime) | `go run -overlay` + `runtime/trace` | Goroutine/GC/syscall visibility |
| Black-box | FS diff + MITM proxy + wall-clock | Closed tools (Cursor, Claude Code) |

## Multi-agent

For agents that spawn subprocess agents, propagation is automatic:

- `APERIO_TRACE_ID` and `APERIO_PARENT_SPAN_ID` env vars are inherited by children
- W3C `traceparent` headers are injected on outbound HTTPS for OTEL interop
- `aperio merge-traces` combines correlated per-process traces into a single DAG
- The viewer color-codes spans by agent role and offers per-agent filter buttons

## CLI commands

| Command | Purpose |
|---------|---------|
| `record` | Trace an agent execution (source or black-box) |
| `replay` | Re-run an agent against recorded LLM responses |
| `view` | Open an interactive HTML viewer (DAG + Gantt + costs) |
| `diff` | Show structural differences between two traces |
| `graph` | Convert a trace to explicit graph JSON |
| `evals` | Tree-edit-distance + semantic similarity scoring |
| `metrics` | Print speed/cost/token/tool metrics for one or more traces |
| `export` | Convert and send traces in OTLP JSON |
| `merge-traces` | Combine multi-agent traces into one DAG |
| `benchmark` | Run a YAML-defined task across N tools and rank them |
| `compare` | N-way ad-hoc comparison of pre-existing traces |
| `leaderboard` | Persistent benchmark results across runs |
| `list-tasks` | List recently captured tasks |

Run `aperio <cmd> --help` for full flags.

## Library usage

Aperio's public Go packages can be imported directly:

```go
import (
    "github.com/qiangli/aperio/trace"
    "github.com/qiangli/aperio/runner"
    "github.com/qiangli/aperio/proxy"
    "github.com/qiangli/aperio/eval"
    "github.com/qiangli/aperio/export"
    "github.com/qiangli/aperio/viewer"
    "github.com/qiangli/aperio/benchmark"
)
```

Examples:

```go
// Record an agent execution
err := runner.Record(ctx, runner.RecordOptions{
    Command:    []string{"python", "agent.py"},
    OutputPath: "trace.json",
})

// Read, analyze, evaluate
t, _   := trace.ReadTrace("trace.json")
g      := trace.BuildGraph(t)
llms   := t.SpansByType(trace.SpanLLMRequest)
result := eval.Evaluate(left, right, nil)
fmt.Println(eval.FormatText(result, true))

// Export to OTLP
otlp := export.ConvertTrace(t, "my-service")
_ = export.Send(otlp, export.SendOptions{Endpoint: "http://localhost:4318/v1/traces"})
```

| Package | Key types / functions |
|---------|----------------------|
| `trace` | `Span`, `Trace`, `TraceGraph`, `ReadTrace`, `WriteTrace`, `BuildGraph`, `Merge`, `Enrich`, `Diff` |
| `runner` | `Record`, `Replay`, `RecordOptions`, `ReplayOptions`, `DetectLanguage` |
| `proxy` | `Proxy`, `Options`, `Redactor`, `RedactionConfig`, `New` |
| `eval` | `Evaluate`, `EvalResult`, `EvalConfig`, `FormatText`, `FormatJSON` |
| `export` | `ConvertTrace`, `Send`, `OTLPExportRequest`, `MetricsCollector`, `Batcher` |
| `viewer` | `Serve`, `ServeDiff`, `Options`, `DiffOptions` |
| `benchmark` | `Run`, `Compare`, `ParseSpec`, `BenchmarkSpec`, `ComparisonResult`, `GenerateHTML` |

## Benchmark spec

```yaml
name: "benchmark-name"
runs: 3
output_dir: "./results"

task:
  query: "Implement feature X"
  working_dir: "/tmp/workspace"
  setup_cmd: "git checkout main"
  timeout: "5m"
  validation:
    - { type: command,     value: "go test ./..." }
    - { type: file_exists, value: "feature.go" }

tools:
  - name: my-agent
    command: ["go", "run", "./cmd/agent", "{{.Query}}"]
    mode: source
    go_tracer: runtime
  - name: other-tool
    command: ["other-tool", "--message", "{{.Query}}"]
    mode: blackbox
    targets: ["api.openai.com"]

pricing:
  claude-sonnet-4: { input: 0.003, output: 0.015 }
```

## Development

```bash
make build        # build ./bin/aperio
make test         # full test suite (verbose)
make test-short   # skip long-running tests
make lint         # go vet
go test ./internal/trace/... -run TestMerge   # single test
```

See [docs/FEATURES.md](docs/FEATURES.md) for the full feature reference and [CLAUDE.md](CLAUDE.md) for architecture notes.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
