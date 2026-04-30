# Aperio Feature Summary

Aperio is a CLI tool for recording, replaying, benchmarking, and visualizing execution traces of AI agent applications.

## CLI Commands

### `aperio record`
Record an agent execution with full tracing.
- Source-level tracing for Python (`sys.settrace`), Node.js (`--require` injection), Go (`dlv trace` or `runtime/trace`)
- LLM API capture via MITM HTTPS proxy
- `--go-tracer runtime` uses Go's `runtime/trace` via `-overlay` injection for goroutine/GC/syscall visibility
- `--correlation-id` and `--agent-role` for multi-agent trace linking
- `--graph` flag auto-produces graph JSON alongside trace

### `aperio replay`
Replay an agent with mocked LLM responses for deterministic testing.
- Three matching strategies: `sequential`, `fingerprint` (URL hash), `body` (canonicalized JSON body)
- YAML cassette format (`--cassette`) compatible with VCR-style tools
- Response normalization strips volatile fields (timestamps, IDs)

### `aperio view`
Interactive HTML visualization of a trace.
- Cytoscape.js DAG with dagre layout
- Cost summary panel (total tokens, estimated USD, per-model breakdown)
- Per-span cost annotations on LLM nodes
- Gantt-style latency timeline
- Multi-agent color coding: distinct border colors per agent role, agent filter buttons, color legend

### `aperio diff`
Compare two traces showing structural differences (span counts, LLM request sequences, tool call sequences).

### `aperio graph`
Convert a trace to explicit graph JSON (nodes, edges, stats) for programmatic analysis.

### `aperio evals`
Compare two traces using Zhang-Shasha tree edit distance.
- Three similarity scores: overall, structural, behavioral
- Per-span-type breakdown (matched/inserted/deleted/renamed)
- `--semantic` adds text metrics: BLEU, ROUGE-1/2/L, Levenshtein, cosine similarity
- `--threshold` returns exit code 1 if similarity below value (CI-friendly)

### `aperio export`
Export traces to OTLP JSON format for observability backends (Arize Phoenix, Jaeger, Grafana Tempo).
- Lightweight exporter (no OTEL SDK dependency)
- Attribute mapping to OTEL semantic conventions (`gen_ai.*`, `http.*`)
- `--dry-run` prints OTLP JSON to stdout

### `aperio merge-traces`
Combine correlated multi-agent traces into a single unified DAG.
- Resolves cross-process parent-child links via correlation metadata
- Namespaces span IDs to prevent collisions
- Creates synthetic root span for multi-root graphs

### `aperio benchmark`
Run the same task against N tools and produce comparison reports.
- YAML spec defines task, tools, validation checks, pricing
- Source-traced mode (full internal tracing) and black-box mode (file watcher + proxy + timing)
- Configurable number of runs per tool
- Setup/cleanup commands between runs
- Auto-generates HTML report with charts and updates leaderboard

### `aperio compare`
Ad-hoc N-way comparison of pre-existing trace files.
- Computes metrics, rankings, pairwise similarity for any number of traces
- Output formats: text, HTML (with Chart.js charts), CSV, JSON

### `aperio leaderboard`
View accumulated benchmark rankings over time.
- Persists results across benchmark runs (JSON file)
- Shows all-time best scores per tool
- Recent entries with trend indicators
- `--clear` to reset

## Tracing Modes

| Mode | How it works | Use case |
|------|-------------|----------|
| Source (Python) | `sitecustomize.py` via PYTHONPATH hooks `sys.settrace()` | Tracing Python agents with source |
| Source (Node.js) | `--require node_tracer.js` monkey-patches `Module.prototype.require` | Tracing Node/TS agents with source |
| Source (Go/dlv) | Wraps agent with `dlv trace` subprocess | Function-level Go tracing |
| Source (Go/runtime) | Injects `init()` via `go run -overlay`, uses `runtime/trace` | Goroutine/GC/syscall visibility |
| Black-box | File system diff + MITM proxy + wall-clock timing | Tools without source (Cursor, Claude Code) |

## Evaluation Metrics

### Structural (Tree Edit Distance)
- Zhang-Shasha algorithm on ordered labeled trees
- Domain-specific cost weights: LLM/tool spans = 1.0, function spans = 0.3
- Behavioral spine extraction (LLM + tool spans only)

### Semantic (Text Metrics)
- BLEU (n-gram precision with brevity penalty)
- ROUGE-1, ROUGE-2 (recall-oriented n-gram overlap)
- ROUGE-L (longest common subsequence F-measure)
- Levenshtein similarity (character-level edit distance)
- Cosine similarity (TF-IDF vectors)

### Benchmark Metrics
- Speed: total duration, P50/P95/P99 LLM latency
- Cost: per-call and total (configurable model pricing)
- Tokens: input, output, efficiency ratio
- Tool usage: call count, unique tools, success rate
- File operations: read/write counts, bytes
- Quality: task validation pass rate
- Reliability: error count

## Multi-Agent Support

- Correlation via `APERIO_TRACE_ID` + `APERIO_PARENT_SPAN_ID` env vars
- Automatically propagated to subprocesses
- W3C `traceparent` header injection for OTEL interop
- `merge-traces` command combines correlated traces
- Viewer shows per-agent color coding with filter buttons

## Benchmark Spec Format (YAML)

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
    - type: command
      value: "go test ./..."
    - type: file_exists
      value: "feature.go"

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
  claude-sonnet-4:
    input: 0.003
    output: 0.015
```

## Library Usage

Aperio can be embedded in other Go projects. Public packages are thin facades over the internal implementation, using Go type aliases for full type compatibility.

```go
import (
    "github.com/qiangli/aperio/trace"
    "github.com/qiangli/aperio/runner"
    "github.com/qiangli/aperio/eval"
    "github.com/qiangli/aperio/export"
    "github.com/qiangli/aperio/proxy"
    "github.com/qiangli/aperio/viewer"
    "github.com/qiangli/aperio/benchmark"
)
```

### Examples

**Read and analyze a trace:**
```go
t, err := trace.ReadTrace("agent-trace.json")
graph := trace.BuildGraph(t)
llmSpans := t.SpansByType(trace.SpanLLMRequest)
```

**Record an agent execution:**
```go
err := runner.Record(ctx, runner.RecordOptions{
    Command:    []string{"python", "agent.py"},
    OutputPath: "trace.json",
})
```

**Compare two traces:**
```go
left, _ := trace.ReadGraph("a.graph.json")
right, _ := trace.ReadGraph("b.graph.json")
result := eval.Evaluate(left, right, nil)
fmt.Println(eval.FormatText(result, true))
```

**Export to OTLP:**
```go
t, _ := trace.ReadTrace("trace.json")
otlp := export.ConvertTrace(t, "my-service")
err := export.Send(otlp, export.SendOptions{Endpoint: "http://localhost:4318/v1/traces"})
```

| Package | Import Path | Key Types / Functions |
|---------|------------|----------------------|
| `trace` | `github.com/qiangli/aperio/trace` | `Span`, `Trace`, `TraceGraph`, `ReadTrace`, `WriteTrace`, `BuildGraph`, `Merge`, `Enrich`, `Diff` |
| `runner` | `github.com/qiangli/aperio/runner` | `Record`, `Replay`, `RecordOptions`, `ReplayOptions`, `DetectLanguage` |
| `proxy` | `github.com/qiangli/aperio/proxy` | `Proxy`, `Options`, `Redactor`, `RedactionConfig`, `New` |
| `eval` | `github.com/qiangli/aperio/eval` | `Evaluate`, `EvalResult`, `EvalConfig`, `FormatText`, `FormatJSON` |
| `export` | `github.com/qiangli/aperio/export` | `ConvertTrace`, `Send`, `OTLPExportRequest`, `MetricsCollector`, `Batcher` |
| `viewer` | `github.com/qiangli/aperio/viewer` | `Serve`, `ServeDiff`, `Options`, `DiffOptions` |
| `benchmark` | `github.com/qiangli/aperio/benchmark` | `Run`, `Compare`, `ParseSpec`, `BenchmarkSpec`, `ComparisonResult`, `GenerateHTML` |

## Architecture

```
Public API (importable):
  trace/               Core data model, I/O, merge, enrich, filter, diff, graph, stream
  runner/              Record/replay orchestration, language detection
  proxy/               MITM proxy, redaction
  eval/                Tree edit distance, similarity metrics
  export/              OTLP conversion, send, batching, Prometheus metrics
  viewer/              Interactive HTML trace viewer
  benchmark/           Benchmarking framework, comparison, leaderboard, adapters

CLI:
  cmd/aperio/          CLI entry point (11 subcommands)

Internal implementation:
  internal/
    runner/            Record/replay orchestration, language detection
    tracer/            Python, Node.js, Go (dlv + runtime/trace) tracers
    proxy/             MITM proxy, cassette format, body matching, normalization
    trace/             Data model, store, merge, enrich, filter, diff, graph, multi-trace
    eval/              Tree edit distance, cost functions, semantic metrics
    export/            OTLP JSON exporter with attribute mapping
    benchmark/         Spec parsing, runner, black-box tracer, metrics, comparison, reports, leaderboard
    viewer/            HTML viewer with Cytoscape.js, cost panel, timeline, agent colors
```
