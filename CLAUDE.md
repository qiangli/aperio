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
- **Go**: Wraps the agent with `dlv trace` subprocess

**Layer 2 — LLM API capture** (language-agnostic):
- Forward HTTPS proxy (`HTTPS_PROXY` env var) using `goproxy` MITM
- Records/replays HTTP request/response pairs to LLM APIs (OpenAI, Anthropic, Gemini)

### Trace Processing Pipeline

After capture, traces flow through: **Merge** (correlate function + API spans by timestamp) → **Enrich** (extract semantic spans like TOOL_CALL, USER_INPUT from LLM payloads) → **Filter** (remove stdlib noise) → output JSON.

### Package Layout

- `cmd/aperio/` — CLI entry point with six subcommands: `record`, `replay`, `view`, `diff`, `graph`, `evals`
- `internal/runner/` — Orchestrates the full record/replay lifecycle, language detection
- `internal/tracer/` — Language-specific tracer implementations + embedded scripts (`scripts/`)
- `internal/proxy/` — MITM proxy with recorder and replayer handlers
- `internal/trace/` — Data model (`Span`, `Trace`, `TraceGraph`), store (atomic JSON I/O), merge, enrich, filter, diff, classify, graph
- `internal/eval/` — Zhang-Shasha tree edit distance algorithm, domain-specific cost functions, evaluation orchestration
- `internal/viewer/` — HTTP server that injects trace JSON into a Cytoscape.js HTML template

### Key Design Decisions

- Tracer scripts (`python_tracer.py`, `node_tracer.js`) are embedded via `go:embed` and extracted at runtime
- Trace files use atomic writes (temp file + rename) to prevent corruption
- Replay supports two matching strategies: sequential (Nth request → Nth response) or fingerprint (request hash)
- The viewer is a self-contained HTML page with no build toolchain — Cytoscape.js is loaded from CDN
- `TraceGraph` is an explicit node+edge graph format with precomputed depth/childIndex, used by the `evals` command
- The `evals` command uses Zhang-Shasha tree edit distance with domain-specific cost weights (LLM/tool spans weighted 1.0, function spans 0.3) and computes three similarity scores: overall, structural, and behavioral
