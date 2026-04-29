"""
Aperio Python Tracer — injected via sitecustomize.py

Captures:
  1. Function call/return events via sys.settrace()
  2. Subprocess execution (subprocess.run, subprocess.Popen, os.system, os.popen)
  3. Filesystem I/O (open, os.read/write, os.listdir, os.stat, etc.)
  4. Network I/O (socket.connect, urllib, requests, httpx)
  5. Database queries (sqlite3, psycopg2, pymysql, redis, pymongo)

Writes span data to a JSON file for the Go parent process to consume.

Environment variables:
  APERIO_TRACE_OUTPUT  - path to write trace JSON
  APERIO_TRACE_DIR     - project directory to trace (only functions in this tree are recorded)
  APERIO_EXCLUDE       - comma-separated module prefixes to exclude
  APERIO_MODE          - "record" (default) or "replay"
  APERIO_REPLAY_DATA   - path to replay data file (for replay mode)
"""

import atexit
import json
import os
import sys
import time
import threading

_trace_output = os.environ.get("APERIO_TRACE_OUTPUT", "")
_trace_dir = os.environ.get("APERIO_TRACE_DIR", "")
_mode = os.environ.get("APERIO_MODE", "record")
_replay_data_path = os.environ.get("APERIO_REPLAY_DATA", "")
_exclude_prefixes = [
    p.strip()
    for p in os.environ.get("APERIO_EXCLUDE", "").split(",")
    if p.strip()
]

# Default exclusions: standard library, common frameworks, aperio itself
_default_excludes = [
    "<frozen",
    "importlib",
    "sitecustomize",
    "_bootstrap",
    "aperio_tracer",
    "encodings",
    "codecs",
    "abc",
    "posixpath",
    "genericpath",
    "stat",
    "os.path",
    "_collections_abc",
    "typing",
    "enum",
    "re",
    "sre_",
    "functools",
    "copyreg",
    "warnings",
]

_spans = []
_call_stack = {}  # thread_id -> list of (span_id, span_index)
_lock = threading.Lock()
_counter = 0

# Replay data: loaded from file in replay mode
_replay_data = {}  # keyed by span type + sequence index
_replay_counters = {}  # type -> next index


# =============================================================================
# Helpers
# =============================================================================

def _next_id():
    global _counter
    _counter += 1
    return f"py-{_counter}"


def _current_parent_id():
    thread_id = threading.get_ident()
    if thread_id in _call_stack and _call_stack[thread_id]:
        return _call_stack[thread_id][-1][0]
    return ""


def _add_io_span(span_type, name, attributes, start_time=None, end_time=None):
    """Add a non-function I/O span."""
    with _lock:
        span = {
            "id": _next_id(),
            "parent_id": _current_parent_id(),
            "type": span_type,
            "name": name,
            "start_time": start_time or time.time(),
            "end_time": end_time or time.time(),
            "attributes": attributes,
        }
        _spans.append(span)
        return span


def _truncate(s, max_len=2000):
    if s is None:
        return None
    s = str(s)
    if len(s) > max_len:
        return s[:max_len] + f"... ({len(s)} bytes total)"
    return s


def _get_replay_response(span_type):
    """Get the next replay response for a given span type."""
    if not _replay_data:
        return None
    key = span_type
    idx = _replay_counters.get(key, 0)
    _replay_counters[key] = idx + 1
    entries = _replay_data.get(key, [])
    if idx < len(entries):
        return entries[idx]
    return None


# =============================================================================
# 1. Function tracing (sys.settrace)
# =============================================================================

def _should_trace(filename):
    """Determine if a file should be traced based on configuration."""
    if not filename:
        return False
    if filename.startswith("<"):
        return False
    for prefix in _default_excludes:
        if prefix in filename:
            return False
    for prefix in _exclude_prefixes:
        if prefix in filename:
            return False
    if _trace_dir:
        try:
            real_file = os.path.realpath(filename)
            real_dir = os.path.realpath(_trace_dir)
            return real_file.startswith(real_dir)
        except (OSError, ValueError):
            return False
    return True


def _trace_func(frame, event, arg):
    """sys.settrace callback for capturing function calls and returns."""
    filename = frame.f_code.co_filename
    if not _should_trace(filename):
        return None

    thread_id = threading.get_ident()

    if event == "call":
        func_name = frame.f_code.co_name
        module = frame.f_globals.get("__name__", "")
        lineno = frame.f_lineno

        args_info = {}
        try:
            varnames = frame.f_code.co_varnames[: frame.f_code.co_argcount]
            for name in varnames:
                if name in frame.f_locals:
                    val = frame.f_locals[name]
                    val_str = repr(val)
                    if len(val_str) > 200:
                        val_str = val_str[:200] + "..."
                    args_info[name] = val_str
        except Exception:
            pass

        with _lock:
            span_id = _next_id()
            parent_id = ""
            if thread_id in _call_stack and _call_stack[thread_id]:
                parent_id = _call_stack[thread_id][-1][0]

            span = {
                "id": span_id,
                "parent_id": parent_id,
                "type": "FUNCTION",
                "name": f"{module}.{func_name}",
                "start_time": time.time(),
                "end_time": 0,
                "attributes": {
                    "module": module,
                    "function": func_name,
                    "filename": filename,
                    "lineno": lineno,
                    "args": args_info,
                },
            }
            idx = len(_spans)
            _spans.append(span)

            if thread_id not in _call_stack:
                _call_stack[thread_id] = []
            _call_stack[thread_id].append((span_id, idx))

        return _trace_func

    elif event == "return":
        with _lock:
            if thread_id in _call_stack and _call_stack[thread_id]:
                span_id, idx = _call_stack[thread_id].pop()
                if idx < len(_spans):
                    _spans[idx]["end_time"] = time.time()
                    if arg is not None:
                        ret_str = repr(arg)
                        if len(ret_str) > 200:
                            ret_str = ret_str[:200] + "..."
                        _spans[idx]["attributes"]["return_value"] = ret_str

    return _trace_func


# =============================================================================
# 1b. Command classification
# =============================================================================

_EXEC_CATEGORIES = {
    "git": ["git"],
    "filesystem": [
        "cd", "mkdir", "rmdir", "rm", "cp", "mv", "ls", "find",
        "chmod", "chown", "ln", "touch", "stat", "realpath", "readlink",
        "basename", "dirname", "mktemp", "install", "tree", "du", "df",
    ],
    "editor": [
        "sed", "awk", "cat", "head", "tail", "tee", "grep", "rg",
        "sort", "uniq", "wc", "cut", "tr", "xargs", "diff", "patch",
        "jq", "yq", "less", "more", "vi", "vim", "nano", "ed",
    ],
    "network": [
        "curl", "wget", "ssh", "scp", "sftp", "nc", "ncat",
        "dig", "nslookup", "host", "ping", "traceroute", "openssl", "http",
    ],
    "package_manager": [
        "pip", "pip3", "npm", "npx", "yarn", "pnpm", "bun",
        "cargo", "apt", "apt-get", "brew", "conda", "mamba",
        "gem", "composer", "poetry", "pdm", "uv", "pipx",
    ],
    "build": [
        "make", "cmake", "ninja", "gcc", "g++", "cc", "clang",
        "tsc", "esbuild", "vite", "webpack", "rollup",
        "javac", "rustc", "bazel", "gradle", "mvn",
    ],
    "runtime": [
        "python", "python3", "node", "java", "ruby",
        "deno", "php", "perl", "bash", "sh", "zsh",
    ],
    "docker": ["docker", "docker-compose", "podman", "kubectl", "helm"],
    "test": ["pytest", "jest", "vitest", "mocha", "phpunit", "rspec", "unittest"],
    "lint": [
        "eslint", "prettier", "black", "ruff", "isort", "mypy",
        "pyright", "flake8", "pylint", "golangci-lint", "clippy",
        "shellcheck", "stylelint", "biome",
    ],
    "env": ["export", "env", "printenv", "source", "which", "where", "echo", "printf", "set", "unset"],
}

# Two-word command rules
_EXEC_TWO_WORD = {
    "go get": "package_manager", "go install": "package_manager", "go mod": "package_manager",
    "go build": "build", "cargo build": "build", "docker build": "build",
    "go run": "runtime", "cargo run": "runtime", "bun run": "runtime",
    "go test": "test", "cargo test": "test", "npm test": "test", "yarn test": "test",
}

# Reverse lookup for single-word
_EXEC_LOOKUP = {}
for _cat, _cmds in _EXEC_CATEGORIES.items():
    for _cmd in _cmds:
        _EXEC_LOOKUP[_cmd] = _cat


def _classify_command(cmd_str):
    """Classify a command string into (category, subcategory)."""
    parts = cmd_str.strip().split()
    if not parts:
        return "other", ""

    exe = os.path.basename(parts[0])

    # Try two-word match first
    if len(parts) >= 2:
        two_word = f"{exe} {parts[1]}"
        if two_word in _EXEC_TWO_WORD:
            subcat = parts[2] if len(parts) > 2 else ""
            return _EXEC_TWO_WORD[two_word], subcat

    # Single-word match
    if exe in _EXEC_LOOKUP:
        subcat = parts[1] if len(parts) > 1 else ""
        return _EXEC_LOOKUP[exe], subcat

    return "other", ""


# =============================================================================
# 2. Subprocess execution monkey-patching
# =============================================================================

def _patch_subprocess():
    """Monkey-patch subprocess module to record/replay exec calls."""
    try:
        import subprocess
    except ImportError:
        return

    _original_run = subprocess.run
    _original_popen_init = subprocess.Popen.__init__
    _original_popen_communicate = subprocess.Popen.communicate

    def _patched_run(*args, **kwargs):
        cmd = args[0] if args else kwargs.get("args", [])
        cmd_str = cmd if isinstance(cmd, str) else " ".join(str(c) for c in cmd)
        input_data = kwargs.get("input")
        category, subcategory = _classify_command(cmd_str)

        if _mode == "replay":
            replay = _get_replay_response("EXEC")
            if replay:
                _add_io_span("EXEC", f"exec: {cmd_str}", {
                    "command": cmd_str,
                    "exec.category": category,
                    "exec.subcategory": subcategory,
                    "exit_code": replay.get("exit_code", 0),
                    "stdout": replay.get("stdout", ""),
                    "stderr": replay.get("stderr", ""),
                    "source": "replay",
                })
                result = subprocess.CompletedProcess(
                    args=cmd,
                    returncode=replay.get("exit_code", 0),
                    stdout=replay.get("stdout", "").encode() if kwargs.get("capture_output") or kwargs.get("stdout") == subprocess.PIPE else None,
                    stderr=replay.get("stderr", "").encode() if kwargs.get("capture_output") or kwargs.get("stderr") == subprocess.PIPE else None,
                )
                return result

        start = time.time()
        result = _original_run(*args, **kwargs)
        end = time.time()

        stdout_str = _truncate(result.stdout.decode("utf-8", errors="replace") if isinstance(result.stdout, bytes) else result.stdout)
        stderr_str = _truncate(result.stderr.decode("utf-8", errors="replace") if isinstance(result.stderr, bytes) else result.stderr)

        _add_io_span("EXEC", f"exec: {cmd_str}", {
            "command": cmd_str,
            "exec.category": category,
            "exec.subcategory": subcategory,
            "input": _truncate(input_data),
            "exit_code": result.returncode,
            "stdout": stdout_str,
            "stderr": stderr_str,
        }, start, end)

        return result

    subprocess.run = _patched_run

    # Also patch os.system
    _original_os_system = os.system

    def _patched_os_system(command):
        category, subcategory = _classify_command(command)

        if _mode == "replay":
            replay = _get_replay_response("EXEC")
            if replay:
                _add_io_span("EXEC", f"os.system: {command}", {
                    "command": command,
                    "exec.category": category,
                    "exec.subcategory": subcategory,
                    "exit_code": replay.get("exit_code", 0),
                    "source": "replay",
                })
                return replay.get("exit_code", 0)

        start = time.time()
        ret = _original_os_system(command)
        end = time.time()

        _add_io_span("EXEC", f"os.system: {command}", {
            "command": command,
            "exec.category": category,
            "exec.subcategory": subcategory,
            "exit_code": ret,
        }, start, end)
        return ret

    os.system = _patched_os_system


# =============================================================================
# 3. Filesystem I/O monkey-patching
# =============================================================================

def _patch_filesystem():
    """Monkey-patch filesystem operations to record/replay."""
    import builtins

    _original_open = builtins.open

    def _patched_open(file, mode="r", *args, **kwargs):
        file_str = str(file)
        is_write = any(c in mode for c in "wxa")
        is_read = "r" in mode or (not is_write and "+" not in mode)

        # Skip tracing our own output file
        if file_str == _trace_output:
            return _original_open(file, mode, *args, **kwargs)

        if _mode == "replay" and is_read:
            replay = _get_replay_response("FS_READ")
            if replay and replay.get("path") == file_str:
                import io
                content = replay.get("content", "")
                _add_io_span("FS_READ", f"read: {file_str}", {
                    "path": file_str,
                    "mode": mode,
                    "size": len(content),
                    "source": "replay",
                })
                if "b" in mode:
                    return io.BytesIO(content.encode("utf-8"))
                return io.StringIO(content)

        start = time.time()
        f = _original_open(file, mode, *args, **kwargs)
        end = time.time()

        span_type = "FS_WRITE" if is_write else "FS_READ"
        _add_io_span(span_type, f"{'write' if is_write else 'read'}: {file_str}", {
            "path": file_str,
            "mode": mode,
        }, start, end)

        # Wrap the file object to capture content on read/write
        if is_write:
            return _WrappedWriteFile(f, file_str)
        elif is_read:
            return _WrappedReadFile(f, file_str)

        return f

    builtins.open = _patched_open


class _WrappedReadFile:
    """Wrapper that records file read content."""
    def __init__(self, f, path):
        self._f = f
        self._path = path
        self._content = []

    def read(self, *args):
        data = self._f.read(*args)
        self._content.append(str(data)[:2000] if data else "")
        return data

    def readline(self, *args):
        data = self._f.readline(*args)
        self._content.append(str(data)[:500] if data else "")
        return data

    def readlines(self, *args):
        data = self._f.readlines(*args)
        self._content.append(str(data)[:2000] if data else "")
        return data

    def close(self):
        content = "".join(self._content)
        if content:
            # Update the last FS_READ span for this path with content
            with _lock:
                for span in reversed(_spans):
                    if span["type"] == "FS_READ" and span["attributes"].get("path") == self._path:
                        span["attributes"]["content"] = _truncate(content)
                        span["attributes"]["size"] = len(content)
                        break
        return self._f.close()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()

    def __iter__(self):
        return iter(self._f)

    def __getattr__(self, name):
        return getattr(self._f, name)


class _WrappedWriteFile:
    """Wrapper that records file write content."""
    def __init__(self, f, path):
        self._f = f
        self._path = path
        self._content = []

    def write(self, data):
        self._content.append(str(data)[:2000] if data else "")
        return self._f.write(data)

    def writelines(self, lines):
        for line in lines:
            self._content.append(str(line)[:500])
        return self._f.writelines(lines)

    def close(self):
        content = "".join(self._content)
        if content:
            with _lock:
                for span in reversed(_spans):
                    if span["type"] == "FS_WRITE" and span["attributes"].get("path") == self._path:
                        span["attributes"]["content"] = _truncate(content)
                        span["attributes"]["size"] = len(content)
                        break
        return self._f.close()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()

    def __getattr__(self, name):
        return getattr(self._f, name)


# =============================================================================
# 4. Network I/O monkey-patching
# =============================================================================

def _patch_network():
    """Monkey-patch socket to record/replay network connections."""
    import socket

    _original_connect = socket.socket.connect
    _original_sendall = socket.socket.sendall
    _original_send = socket.socket.send
    _original_recv = socket.socket.recv

    def _patched_connect(self, address):
        host = str(address[0]) if isinstance(address, tuple) else str(address)
        port = address[1] if isinstance(address, tuple) and len(address) > 1 else 0

        _add_io_span("NET_IO", f"connect: {host}:{port}", {
            "operation": "connect",
            "host": host,
            "port": port,
        })

        return _original_connect(self, address)

    def _patched_sendall(self, data, *args):
        _add_io_span("NET_IO", f"send: {len(data)} bytes", {
            "operation": "send",
            "size": len(data),
            "data_preview": _truncate(data, 500),
        })
        return _original_sendall(self, data, *args)

    def _patched_send(self, data, *args):
        _add_io_span("NET_IO", f"send: {len(data)} bytes", {
            "operation": "send",
            "size": len(data),
            "data_preview": _truncate(data, 500),
        })
        return _original_send(self, data, *args)

    def _patched_recv(self, bufsize, *args):
        if _mode == "replay":
            replay = _get_replay_response("NET_IO")
            if replay and replay.get("operation") == "recv":
                data = replay.get("data", "").encode()
                _add_io_span("NET_IO", f"recv: {len(data)} bytes", {
                    "operation": "recv",
                    "size": len(data),
                    "source": "replay",
                })
                return data

        data = _original_recv(self, bufsize, *args)
        _add_io_span("NET_IO", f"recv: {len(data)} bytes", {
            "operation": "recv",
            "size": len(data),
            "data_preview": _truncate(data, 500),
        })
        return data

    socket.socket.connect = _patched_connect
    socket.socket.sendall = _patched_sendall
    socket.socket.send = _patched_send
    socket.socket.recv = _patched_recv


# =============================================================================
# 5. Database monkey-patching
# =============================================================================

def _patch_databases():
    """Monkey-patch common database libraries to record/replay queries."""

    # sqlite3 — note: sqlite3.Cursor is a C extension type and may be immutable.
    # We patch sqlite3.Connection.execute instead (Python-level wrapper).
    try:
        import sqlite3
        _original_sqlite3_conn_execute = sqlite3.Connection.execute

        def _patched_conn_execute(self, sql, parameters=()):
            if _mode == "replay":
                replay = _get_replay_response("DB_QUERY")
                if replay:
                    _add_io_span("DB_QUERY", f"sqlite3: {sql[:80]}", {
                        "database": "sqlite3",
                        "query": _truncate(sql, 1000),
                        "parameters": str(parameters)[:500],
                        "source": "replay",
                    })
                    return _original_sqlite3_conn_execute(self, sql, parameters)

            start = time.time()
            result = _original_sqlite3_conn_execute(self, sql, parameters)
            end = time.time()

            _add_io_span("DB_QUERY", f"sqlite3: {sql[:80]}", {
                "database": "sqlite3",
                "query": _truncate(sql, 1000),
                "parameters": str(parameters)[:500],
            }, start, end)
            return result

        sqlite3.Connection.execute = _patched_conn_execute
    except (ImportError, AttributeError, TypeError):
        pass

    # Generic DB-API 2.0 patching for psycopg2, pymysql, etc.
    for module_name in ["psycopg2", "pymysql", "mysql.connector"]:
        try:
            mod = __import__(module_name)
            if hasattr(mod, "extensions") and hasattr(mod.extensions, "cursor"):
                cursor_cls = mod.extensions.cursor
            elif hasattr(mod, "cursors") and hasattr(mod.cursors, "Cursor"):
                cursor_cls = mod.cursors.Cursor
            else:
                continue

            _orig_execute = cursor_cls.execute

            def _make_patched_execute(orig, db_name):
                def _patched(self, query, args=None):
                    start = time.time()
                    result = orig(self, query, args)
                    end = time.time()
                    _add_io_span("DB_QUERY", f"{db_name}: {str(query)[:80]}", {
                        "database": db_name,
                        "query": _truncate(str(query), 1000),
                        "parameters": str(args)[:500] if args else None,
                    }, start, end)
                    return result
                return _patched

            cursor_cls.execute = _make_patched_execute(_orig_execute, module_name)
        except (ImportError, AttributeError):
            pass

    # Redis
    try:
        import redis
        _original_redis_execute = redis.client.Redis.execute_command

        def _patched_redis_execute(self, *args, **kwargs):
            cmd = " ".join(str(a) for a in args[:3])

            if _mode == "replay":
                replay = _get_replay_response("DB_QUERY")
                if replay:
                    _add_io_span("DB_QUERY", f"redis: {cmd}", {
                        "database": "redis",
                        "command": cmd,
                        "result": replay.get("result"),
                        "source": "replay",
                    })
                    return replay.get("result")

            start = time.time()
            result = _original_redis_execute(self, *args, **kwargs)
            end = time.time()

            _add_io_span("DB_QUERY", f"redis: {cmd}", {
                "database": "redis",
                "command": cmd,
                "result": _truncate(result, 500),
            }, start, end)
            return result

        redis.client.Redis.execute_command = _patched_redis_execute
    except (ImportError, AttributeError):
        pass


# =============================================================================
# Output and install
# =============================================================================

def _flush_spans():
    """Write collected spans to the output file."""
    if not _trace_output:
        return

    with _lock:
        data = list(_spans)

    for span in data:
        if span["start_time"]:
            span["start_time"] = _epoch_to_iso(span["start_time"])
        if span["end_time"]:
            span["end_time"] = _epoch_to_iso(span["end_time"])

    try:
        # Use os-level write to avoid triggering our own patched open()
        import builtins
        _real_open = builtins.__dict__.get("_aperio_original_open", builtins.open)
        with _real_open(_trace_output, "w") as f:
            json.dump(data, f, indent=2)
    except Exception as e:
        sys.stderr.write(f"[aperio] Error writing trace: {e}\n")


def _epoch_to_iso(ts):
    """Convert epoch timestamp to ISO 8601 format."""
    from datetime import datetime, timezone
    return datetime.fromtimestamp(ts, tz=timezone.utc).isoformat()


def _load_replay_data():
    """Load replay data from file."""
    global _replay_data
    if _replay_data_path and os.path.exists(_replay_data_path):
        try:
            with open(_replay_data_path) as f:
                entries = json.load(f)
            # Group by span type
            for entry in entries:
                t = entry.get("type", "")
                if t not in _replay_data:
                    _replay_data[t] = []
                _replay_data[t].append(entry.get("attributes", {}))
        except Exception as e:
            sys.stderr.write(f"[aperio] Error loading replay data: {e}\n")


def install():
    """Install all trace hooks."""
    if not _trace_output:
        return

    # Save reference to real open before patching
    import builtins
    builtins._aperio_original_open = builtins.open

    # Load replay data if in replay mode
    if _mode == "replay":
        _load_replay_data()

    # Install function tracer
    sys.settrace(_trace_func)
    threading.settrace(_trace_func)

    # Install I/O interceptors — each wrapped in try/except so one failure
    # doesn't prevent the others from installing
    for patcher in [_patch_subprocess, _patch_filesystem, _patch_network, _patch_databases]:
        try:
            patcher()
        except Exception as e:
            sys.stderr.write(f"[aperio] Warning: {patcher.__name__} failed: {e}\n")

    atexit.register(_flush_spans)


# Auto-install when imported
install()
