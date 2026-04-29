≠# Aperio Implementation Plan

## Context

Aperio is a CLI tool for recording, replaying, and visualizing execution traces of AI agentic applications. Given a source code repo of an agent (Python, Go, Node.js/TypeScript), aperio:

1. **Records** a live session — runs the agent with dynamic tracing enabled, captures internal function calls, prompt assembly, tool dispatch, AND LLM API request/response details
2. **Replays** the session — re-runs the agent with mocked LLM responses for deterministic, repeatable testing; verifies the execution flow matches
3. **Visualizes** the execution graph as an interactive HTML page with node details (function calls, tool names, arguments, results, LLM payloads)

**Supported agent languages**: Python, Go, Node.js/TypeScript (with awareness of C/C++/Rust native dependencies).

## Architecture

Aperio has **two tracing layers** that produce a unified trace:

### Layer 1: Internal Execution Tracing (language-specific)
Captures function call graphs, prompt assembly, and internal code paths using per-language dynamic tracing:
- **Python**: Inject via `sitecustomize.py` or `PYTHONSTARTUP` → `sys.settrace()` hook captures function call/return events
- **Go**: Use Delve (`dlv trace`) as a subprocess to trace function calls matching configurable patterns; alternatively eBPF uprobes on Linux
- **Node.js/TypeScript**: Inject via `node --require tracer.js` → monkey-patch `Module.prototype.require` to wrap target functions

### Layer 2: LLM API Capture (language-agnostic)
Captures all HTTP traffic to LLM providers using a local MITM proxy:
- Forward proxy via `HTTPS_PROXY` env var (works for all languages)
- Fallback: reverse proxy mode via `OPENAI_BASE_URL` / `ANTHROPIC_BASE_URL` (avoids TLS cert setup)
- Records request/response pairs including streaming SSE responses
- Extracts tool calls, user messages, and assistant responses from API payloads

### Unified Trace
Both layers write spans to a shared trace file. Internal function spans are correlated with LLM API spans via timestamps and call stack context — when a function calls the LLM client library, the internal trace span becomes the parent of the API span.

## Usage

```bash
# Record: run agent with full tracing, save trace
aperio record --output trace.json -- python agent.py "What is the weather?"

# Replay: re-run with mocked LLM responses, verify execution matches
aperio replay --input trace.json -- python agent.py "What is the weather?"

# Visualize: open interactive HTML graph
aperio view trace.json
```

Aperio detects the language from the command and applies the appropriate tracer automatically.

## Trace Format (JSON)

```json
{
  "id": "trace-uuid",
  "created_at": "ISO8601",
  "metadata": {
    "command": "python agent.py 'What is the weather?'",
    "language": "python",
    "working_dir": "/path/to/agent"
  },
  "spans": [{
    "id": "span-uuid",
    "parent_id": "span-uuid or null",
    "type": "FUNCTION|LLM_REQUEST|LLM_RESPONSE|TOOL_CALL|TOOL_RESULT|USER_INPUT|AGENT_OUTPUT|EXEC|FS_READ|FS_WRITE|NET_IO|DB_QUERY",
    "name": "string",
    "start_time": "ISO8601",
    "end_time": "ISO8601",
    "attributes": {}
  }]
}
```

**Span types**:
- `FUNCTION` — internal function call (name, module, file:line, args summary, return value summary)
- `LLM_REQUEST` — outgoing HTTP request to LLM API (url, method, headers, body with messages/tools)
- `LLM_RESPONSE` — LLM API response (status, body with assistant message, token counts)
- `TOOL_CALL` — tool invocation extracted from LLM response (tool name, arguments)
- `TOOL_RESULT` — tool result sent back to LLM (tool name, output)
- `USER_INPUT` — initial user query
- `AGENT_OUTPUT` — final agent response
- `EXEC` — subprocess execution (command, args, stdout/stderr, exit code)
- `FS_READ` — filesystem read (path, content, size)
- `FS_WRITE` — filesystem write (path, content, size)
- `NET_IO` — network I/O: TCP connect, HTTP requests (host, port, url, status)
- `DB_QUERY` — database query (database type, query, parameters, row count)

## Project Structure

```
aperio/
├── cmd/aperio/main.go                # CLI (cobra): record, replay, view
├── internal/
│   ├── runner/
│   │   ├── runner.go                 # Orchestrator: detect language, start tracer + proxy, launch agent
│   │   ├── detect.go                 # Language detection from command
│   │   └── runner_test.go
│   ├── tracer/
│   │   ├── tracer.go                 # Tracer interface
│   │   ├── python.go                 # Python: sys.settrace injection via sitecustomize
│   │   ├── golang.go                 # Go: dlv trace subprocess
│   │   ├── node.go                   # Node.js: --require injection
│   │   └── scripts/                  # Injected tracer scripts (embedded via go:embed)
│   │       ├── python_tracer.py      # Python sys.settrace hook
│   │       └── node_tracer.js        # Node.js require hook
│   ├── proxy/
│   │   ├── proxy.go                  # goproxy MITM setup, TLS cert management
│   │   ├── recorder.go               # Record mode: forward + capture req/resp
│   │   └── replayer.go               # Replay mode: serve recorded responses
│   ├── trace/
│   │   ├── model.go                  # Trace/Span data types
│   │   ├── store.go                  # Read/write trace JSON files
│   │   ├── enrich.go                 # Parse LLM payloads → semantic spans (tool calls, etc.)
│   │   ├── merge.go                  # Merge internal trace + API trace, correlate by timestamp
│   │   ├── matcher.go                # Request matching for replay
│   │   └── filter.go                 # Filter out stdlib/framework noise from function traces
│   └── viewer/
│       ├── viewer.go                 # HTTP server, go:embed, opens browser
│       └── templates/index.html      # Self-contained HTML+Cytoscape.js viewer
├── testdata/                         # Fixture traces
├── go.mod
└── Makefile
```

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/elazarl/goproxy` | HTTPS MITM proxy |
| `github.com/google/uuid` | Span/trace IDs |
| `github.com/pkg/browser` | Open viewer in browser |
| `github.com/rs/zerolog` | Structured logging |

## Implementation Phases

### Phase 1: Foundation & CLI
1. Initialize Go module, directory structure, Cobra CLI skeleton with `record`, `replay`, `view` subcommands
2. Implement `internal/trace/model.go` — Trace/Span types with all span types
3. Implement `internal/trace/store.go` — JSON read/write with atomic writes
4. Implement `internal/runner/detect.go` — detect language from command (python/go/node)
5. Unit tests for model serialization, store round-trips, language detection

### Phase 2: Python Tracer
6. Write `internal/tracer/scripts/python_tracer.py`:
   - Uses `sys.settrace()` to capture function call/return events
   - Filters out stdlib and common framework internals (configurable include/exclude patterns)
   - Writes function spans to a temp JSON file (read by Go parent process)
   - Captures function name, module, file:line, arg summary, return value summary
7. Implement `internal/tracer/python.go`:
   - Creates temp dir with `sitecustomize.py` that imports the tracer
   - Sets `PYTHONPATH` to prepend the temp dir so tracer auto-loads
   - Launches `python agent.py` as subprocess with modified env
   - Reads function trace output after process exits
8. Integration test: trace a simple Python script, verify function spans captured

### Phase 3: LLM API Proxy (Record Mode)
9. Implement `internal/proxy/proxy.go` — goproxy MITM setup, CA cert generation (`~/.aperio/ca-cert.pem`)
10. Implement `internal/proxy/recorder.go`:
    - OnRequest: buffer request body, strip auth headers, create LLM_REQUEST span
    - OnResponse: buffer full response (including SSE streams), create LLM_RESPONSE span
    - SSE handling: read entire stream, re-stream to client
11. Implement `internal/trace/enrich.go` — parse LLM request/response bodies:
    - Provider detection (OpenAI `/v1/chat/completions`, Anthropic `/v1/messages`, Google Gemini)
    - Extract USER_INPUT, TOOL_CALL, TOOL_RESULT, AGENT_OUTPUT spans
12. Integration test: local mock LLM server → proxy → verify API spans

### Phase 4: Runner Orchestrator (Record Mode)
13. Implement `internal/runner/runner.go` for record mode:
    - Start proxy on random port
    - Set env vars: `HTTPS_PROXY`, `HTTP_PROXY`, plus provider base URLs as fallback
    - Start language-specific tracer (wraps the agent subprocess)
    - Wait for agent to exit
    - Collect internal trace (from tracer) + API trace (from proxy)
14. Implement `internal/trace/merge.go`:
    - Merge function spans and API spans into unified trace
    - Correlate by timestamp: when a FUNCTION span's time window contains an LLM_REQUEST span, make the function span the parent
    - Build the full parent-child hierarchy
15. Implement `internal/trace/filter.go`:
    - Configurable include/exclude patterns per language
    - Default: exclude stdlib, common frameworks; include project code
    - Special focus: always include functions that touch LLM client libraries
16. Wire `record` subcommand end-to-end
17. E2E test: record a Python agent that calls OpenAI (mocked), verify unified trace

### Phase 5: Node.js Tracer
18. Write `internal/tracer/scripts/node_tracer.js`:
    - Monkey-patch `Module.prototype.require` to wrap exported functions
    - Track function call/return with timestamps
    - Filter by module path (exclude `node_modules/` except configured packages)
    - Write trace to temp JSON file
19. Implement `internal/tracer/node.go`:
    - Launches `node --require /path/to/node_tracer.js agent.js` with modified env
    - Reads function trace output
20. Integration test with simple Node.js agent

### Phase 6: Go Tracer
21. Implement `internal/tracer/golang.go`:
    - Launch target Go binary under `dlv trace` with function regex patterns
    - Parse dlv trace output into function spans
    - Alternative: if target is a Go source project, build with race detector / coverage instrumentation
22. Integration test with simple Go agent

### Phase 7: Replay Engine
23. Implement `internal/trace/matcher.go`:
    - `SequentialMatcher` — Nth request gets Nth recorded response
    - `FingerprintMatcher` — hash by (method, URL, normalized body)
24. Implement `internal/proxy/replayer.go` — match requests, serve recorded responses
25. Implement replay mode in `internal/runner/runner.go`:
    - Same as record but proxy uses replayer instead of recorder
    - Internal tracer still runs to capture execution path
    - After completion, compare replay trace vs original trace for divergence
26. Wire `replay` subcommand
27. E2E test: record → replay → verify traces match

### Phase 8: HTML Visualization
28. Build `internal/viewer/templates/index.html` — self-contained HTML with Cytoscape.js:
    - Dagre (hierarchical) layout for execution tree
    - Node styling by type:
      - FUNCTION: gray rectangle
      - USER_INPUT: green rounded rectangle
      - LLM_REQUEST/RESPONSE: blue ellipse
      - TOOL_CALL/RESULT: orange hexagon
      - AGENT_OUTPUT: purple diamond
    - Left panel (70%): graph canvas with pan/zoom
    - Right panel (30%): detail panel on node click (syntax-highlighted JSON: full request body, response, function args, return values)
    - Top bar: trace metadata, filter by span type, search by function/tool name
    - Collapse/expand: group related spans (e.g., LLM request+response pair)
29. Implement `internal/viewer/viewer.go` — go:embed HTML, inject trace JSON as `<script>`, serve on random port, open browser
30. Wire `view` subcommand

### Phase 9: Polish
31. `aperio diff trace1.json trace2.json` — compare two traces, highlight execution divergence
32. Export to Mermaid/DOT formats
33. `--target` host filtering for proxy (only intercept specific LLM API hosts)
34. `--filter` patterns for function tracing (include/exclude by module/package)
35. Reverse proxy mode as TLS-free alternative

## Key Technical Decisions

- **Two-layer tracing**: Internal execution (language-specific) + LLM API capture (language-agnostic proxy). Both are needed to show the full picture.
- **Language-specific tracers**: Python `sys.settrace`, Go `dlv trace`, Node.js `--require` hook. Each is the lowest-friction, most capable approach for its language. No code changes to target agent required.
- **Tracer scripts embedded in Go binary**: Python/Node.js tracer scripts are embedded via `go:embed` and written to temp files at runtime. Single binary distribution.
- **Forward proxy (MITM)**: `HTTPS_PROXY` env var works for all languages and LLM providers. Reverse proxy mode available as fallback.
- **Trace merge by timestamp correlation**: Function spans and API spans are produced independently and merged post-hoc. Parent-child relationships established by time containment (function span that encloses an API span becomes its parent).
- **Filtering is critical**: Raw `sys.settrace` produces thousands of spans per second. Must aggressively filter to project code only, with special inclusion rules for LLM client library calls.
- **Single HTML file viewer**: No build toolchain. Cytoscape.js from CDN. Self-contained.

## Known Challenges

1. **Trace volume from sys.settrace**: Python's trace hook fires on every function call/return/line. Must filter aggressively — default to only tracing functions in the agent's project directory, plus LLM client library entry points.
2. **Go tracing without code changes**: `dlv trace` adds overhead and requires the binary to be built with debug info. eBPF uprobes are Linux-only. May need to offer a "build wrapper" mode for Go that injects `runtime/trace` calls.
3. **SSE reassembly**: Different LLM providers use different streaming formats. Enricher needs per-provider parsers.
4. **Correlating trace layers**: Timestamp-based correlation assumes reasonable clock precision. In practice, since both layers run in the same process tree on the same machine, this is reliable.
5. **TLS certificate trust**: User must trust Aperio CA for HTTPS MITM. Mitigated by reverse proxy mode fallback and clear setup instructions.
6. **C/C++/Rust native deps**: Some Python/Node.js packages have native extensions. Function tracing won't descend into native code, but the HTTP proxy layer still captures all LLM API calls regardless.

## Verification

1. **Unit tests**: model serialization, store read/write, language detection, enrich with fixture API responses (OpenAI, Anthropic, Gemini), matcher logic, filter patterns
2. **Integration tests per tracer**: run a simple script in each language through the tracer, verify function spans are captured correctly
3. **Integration tests for proxy**: local mock LLM server, proxy in record/replay modes, verify API spans
4. **E2E test**: full record → replay → view cycle with a sample Python agent that calls a mock LLM, makes tool calls, and produces output. Verify:
   - Record produces a complete trace with both function and API spans
   - Replay produces an identical trace
   - View opens an HTML page showing the correct graph
5. **Viewer**: load sample traces from `testdata/` directly in browser during development
