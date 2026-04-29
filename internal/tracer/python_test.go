package tracer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/aperio/internal/trace"
)

func TestPythonTracerSetup(t *testing.T) {
	pt := NewPythonTracer()
	defer pt.Cleanup()

	cfg := Config{
		TraceOutputPath: "/tmp/test-trace.json",
		WorkingDir:      "/tmp/agent",
		ExcludePatterns: []string{"venv", "site-packages"},
	}

	env, args, err := pt.Setup(context.Background(), cfg, []string{"python", "agent.py"})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// Verify args are unchanged
	if len(args) != 2 || args[0] != "python" || args[1] != "agent.py" {
		t.Errorf("args modified: %v", args)
	}

	// Verify env vars
	envMap := make(map[string]string)
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		envMap[parts[0]] = parts[1]
	}

	if envMap["APERIO_TRACE_OUTPUT"] != "/tmp/test-trace.json" {
		t.Errorf("APERIO_TRACE_OUTPUT: %s", envMap["APERIO_TRACE_OUTPUT"])
	}
	if envMap["APERIO_TRACE_DIR"] != "/tmp/agent" {
		t.Errorf("APERIO_TRACE_DIR: %s", envMap["APERIO_TRACE_DIR"])
	}
	if !strings.Contains(envMap["APERIO_EXCLUDE"], "venv") {
		t.Errorf("APERIO_EXCLUDE missing venv: %s", envMap["APERIO_EXCLUDE"])
	}

	// Verify PYTHONPATH points to temp dir
	pythonPath := envMap["PYTHONPATH"]
	if pythonPath == "" {
		t.Fatal("PYTHONPATH not set")
	}

	// Verify sitecustomize.py exists in the temp dir
	sitePath := filepath.Join(strings.Split(pythonPath, ":")[0], "sitecustomize.py")
	if _, err := os.Stat(sitePath); os.IsNotExist(err) {
		t.Error("sitecustomize.py not created")
	}

	// Verify tracer script exists
	tracerPath := filepath.Join(strings.Split(pythonPath, ":")[0], "aperio_tracer.py")
	if _, err := os.Stat(tracerPath); os.IsNotExist(err) {
		t.Error("aperio_tracer.py not created")
	}
}

func TestPythonTracerIntegration(t *testing.T) {
	// Skip if python3 is not available
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found, skipping integration test")
	}

	// Create a simple test Python script
	agentDir := t.TempDir()
	agentScript := filepath.Join(agentDir, "test_agent.py")
	err := os.WriteFile(agentScript, []byte(`
def greet(name):
    return f"Hello, {name}!"

def process(query):
    result = greet("World")
    return result

if __name__ == "__main__":
    output = process("test query")
    print(output)
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	traceOutput := filepath.Join(t.TempDir(), "trace.json")

	pt := NewPythonTracer()
	defer pt.Cleanup()

	cfg := Config{
		TraceOutputPath: traceOutput,
		WorkingDir:      agentDir,
	}

	env, args, err := pt.Setup(context.Background(), cfg, []string{"python3", agentScript})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// Run the agent with tracing
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir = agentDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("agent run failed: %v\nOutput: %s", err, out)
	}

	if !strings.Contains(string(out), "Hello, World!") {
		t.Errorf("unexpected agent output: %s", out)
	}

	// Collect the trace
	spans, err := pt.Collect(cfg)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if len(spans) == 0 {
		t.Fatal("no spans collected")
	}

	// Verify we captured the expected functions
	funcNames := make(map[string]bool)
	for _, s := range spans {
		if s.Type == trace.SpanFunction {
			funcNames[s.Name] = true
		}
	}

	// Should have captured process and greet functions
	found := false
	for name := range funcNames {
		if strings.Contains(name, "process") || strings.Contains(name, "greet") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected to find process or greet functions, got: %v", funcNames)
	}
}

func TestPythonTracerExecCapture(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found")
	}

	agentDir := t.TempDir()
	agentScript := filepath.Join(agentDir, "exec_agent.py")
	os.WriteFile(agentScript, []byte(`
import subprocess

def run_cmd():
    result = subprocess.run(["echo", "hello aperio"], capture_output=True, text=True)
    print(result.stdout.strip())

if __name__ == "__main__":
    run_cmd()
`), 0644)

	traceOutput := filepath.Join(t.TempDir(), "trace.json")
	pt := NewPythonTracer()
	defer pt.Cleanup()

	cfg := Config{TraceOutputPath: traceOutput, WorkingDir: agentDir}
	env, args, err := pt.Setup(context.Background(), cfg, []string{"python3", agentScript})
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir = agentDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run failed: %v\n%s", err, out)
	}

	spans, err := pt.Collect(cfg)
	if err != nil {
		t.Fatal(err)
	}

	hasExec := false
	for _, s := range spans {
		if s.Type == trace.SpanType("EXEC") {
			hasExec = true
			if cmd, ok := s.Attributes["command"].(string); ok {
				if !strings.Contains(cmd, "echo") {
					t.Errorf("EXEC command should contain 'echo', got: %s", cmd)
				}
			}
		}
	}
	if !hasExec {
		t.Errorf("expected EXEC span, types found: %v", spanTypes(spans))
	}
}

func TestPythonTracerFSCapture(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found")
	}

	agentDir := t.TempDir()
	inputFile := filepath.Join(agentDir, "input.txt")
	os.WriteFile(inputFile, []byte("test data"), 0644)

	agentScript := filepath.Join(agentDir, "fs_agent.py")
	os.WriteFile(agentScript, []byte(`
import os

def read_and_write():
    with open("`+inputFile+`", "r") as f:
        data = f.read()
    out_path = os.path.join("`+agentDir+`", "output.txt")
    with open(out_path, "w") as f:
        f.write("processed: " + data)
    print("done")

if __name__ == "__main__":
    read_and_write()
`), 0644)

	traceOutput := filepath.Join(t.TempDir(), "trace.json")
	pt := NewPythonTracer()
	defer pt.Cleanup()

	cfg := Config{TraceOutputPath: traceOutput, WorkingDir: agentDir}
	env, args, err := pt.Setup(context.Background(), cfg, []string{"python3", agentScript})
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir = agentDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run failed: %v\n%s", err, out)
	}

	spans, err := pt.Collect(cfg)
	if err != nil {
		t.Fatal(err)
	}

	types := spanTypes(spans)
	if _, ok := types["FS_READ"]; !ok {
		t.Errorf("expected FS_READ span, types found: %v", types)
	}
	if _, ok := types["FS_WRITE"]; !ok {
		t.Errorf("expected FS_WRITE span, types found: %v", types)
	}
}

func TestPythonTracerAllIOCombined(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found")
	}

	agentDir := t.TempDir()
	inputFile := filepath.Join(agentDir, "config.txt")
	os.WriteFile(inputFile, []byte("value=42"), 0644)

	agentScript := filepath.Join(agentDir, "full_agent.py")
	os.WriteFile(agentScript, []byte(`
import subprocess

def main():
    with open("`+inputFile+`", "r") as f:
        config = f.read()
    result = subprocess.run(["echo", config.strip()], capture_output=True, text=True)
    with open("`+filepath.Join(agentDir, "result.txt")+`", "w") as f:
        f.write(result.stdout)
    print("done")

if __name__ == "__main__":
    main()
`), 0644)

	traceOutput := filepath.Join(t.TempDir(), "trace.json")
	pt := NewPythonTracer()
	defer pt.Cleanup()

	cfg := Config{TraceOutputPath: traceOutput, WorkingDir: agentDir}
	env, args, err := pt.Setup(context.Background(), cfg, []string{"python3", agentScript})
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir = agentDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run failed: %v\n%s", err, out)
	}

	spans, err := pt.Collect(cfg)
	if err != nil {
		t.Fatal(err)
	}

	types := spanTypes(spans)
	for _, expected := range []string{"FUNCTION", "FS_READ", "FS_WRITE", "EXEC"} {
		if _, ok := types[expected]; !ok {
			t.Errorf("expected %s span, types found: %v", expected, types)
		}
	}
}

func TestPythonCollectMissingFile(t *testing.T) {
	pt := NewPythonTracer()
	_, err := pt.Collect(Config{TraceOutputPath: "/nonexistent/trace.json"})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestPythonCollectInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	os.WriteFile(path, []byte("not json"), 0644)

	pt := NewPythonTracer()
	_, err := pt.Collect(Config{TraceOutputPath: path})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestPythonCleanup(t *testing.T) {
	pt := NewPythonTracer()
	cfg := Config{TraceOutputPath: "/tmp/test.json", WorkingDir: "/tmp"}
	pt.Setup(context.Background(), cfg, []string{"python3", "test.py"})

	tmpDir := pt.tmpDir
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		t.Fatal("temp dir should exist before cleanup")
	}

	pt.Cleanup()

	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Error("temp dir should be removed after cleanup")
	}
}

func spanTypes(spans []*trace.Span) map[string]int {
	types := make(map[string]int)
	for _, s := range spans {
		types[string(s.Type)]++
	}
	return types
}
