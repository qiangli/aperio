# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
make build          # Build binary to bin/aperio (injects version from git tags)
make test           # Run all tests with verbose output
make test-short     # Skip long-running tests
make lint           # Run go vet
make install        # Install to $GOPATH/bin
go test ./internal/trace/... -run TestMerge  # Run a single test
```

## Architecture

Aperio is a CLI tool that records, replays, and visualizes execution traces of AI agent applications. It uses a **two-layer tracing model** that requires no code modifications to the target agent.

### Two-Layer Tracing

**Layer 1 — Internal execution tracing** (language-specific):
- **Python**: Injects `sitecustomize.py` via `PYTHONPATH` to hook `sys.settrace()`
- **Node.js**: Injects `--require node_tracer.js` to monkey-patch `Module.prototype.require`
- **Go**: `dlv trace` (default) or `runtime/trace` via overlay injection (`--go-tracer runtime`)

**Layer 2 — LLM API capture** (language-agnostic):
- Forward HTTPS proxy (`HTTPS_PROXY` env var) using `goproxy` MITM
- Records/replays HTTP request/response pairs to LLM APIs (OpenAI, Anthropic, Gemini)

### Trace Processing Pipeline

After capture, traces flow through: **Merge** (correlate function + API spans by timestamp) → **Enrich** (extract semantic spans like TOOL_CALL, USER_INPUT from LLM payloads) → **Filter** (remove stdlib noise) → output JSON.

### Package Layout

- `cmd/aperio/` — CLI (cobra) with subcommands: `record`, `replay`, `view`, `diff`, `graph`, `evals`, `export`, `merge-traces`, `benchmark`, `compare`, `leaderboard`
- `internal/runner/` — Orchestrates the full record/replay lifecycle, language detection
- `internal/tracer/` — Language-specific tracer implementations + embedded scripts (`scripts/`)
- `internal/proxy/` — MITM proxy with recorder, replayer, cassette format (YAML), body-aware matching, response normalization
- `internal/trace/` — Data model (`Span`, `Trace`, `TraceGraph`), store (atomic JSON I/O), merge, enrich, filter, diff, classify, graph
- `internal/eval/` — Zhang-Shasha tree edit distance, cost functions, semantic text metrics (BLEU, ROUGE, Levenshtein, cosine)
- `internal/export/` — Lightweight OTLP JSON exporter (no OTEL SDK dependency) with attribute mapping to semantic conventions
- `internal/benchmark/` — Multi-tool benchmarking framework: spec parsing, black-box tracing, metrics extraction, N-way comparison, HTML/CSV/JSON report generation
- `internal/viewer/` — HTTP server with Cytoscape.js graph, cost summary panel, per-span cost annotations, Gantt timeline

### Key Design Decisions

- Tracer scripts (`python_tracer.py`, `node_tracer.js`) are embedded via `go:embed` and extracted at runtime
- Trace files use atomic writes (temp file + rename) to prevent corruption
- Replay supports three matching strategies: sequential, fingerprint (URL hash), and body (canonicalized JSON body comparison)
- The viewer is a self-contained HTML page with no build toolchain — Cytoscape.js is loaded from CDN
- `TraceGraph` is an explicit node+edge graph format with precomputed depth/childIndex, used by the `evals` command
- The `evals` command uses Zhang-Shasha tree edit distance with domain-specific cost weights (LLM/tool spans weighted 1.0, function spans 0.3) and computes three similarity scores: overall, structural, and behavioral. `--semantic` adds BLEU/ROUGE/Levenshtein/cosine text metrics
- The `export` command converts traces to OTLP JSON for backends like Arize Phoenix, Jaeger, or Grafana Tempo
- Replay supports YAML cassette files (`--cassette`) as an alternative to trace JSON for recorded interactions
- Multi-agent correlation via `APERIO_TRACE_ID` + `APERIO_PARENT_SPAN_ID` env vars, propagated automatically to subprocesses. `merge-traces` combines correlated traces into a single DAG
- The proxy injects W3C `traceparent` headers for OTEL-compatible distributed tracing
- `benchmark` command runs same task against N tools (YAML spec), supports source-traced and black-box modes, produces ranked HTML reports with charts
- `compare` command does ad-hoc N-way comparison of pre-existing trace files with configurable pricing
