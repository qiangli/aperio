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

func TestNodeTracerSetup(t *testing.T) {
	nt := NewNodeTracer()
	defer nt.Cleanup()

	cfg := Config{
		TraceOutputPath: "/tmp/test-trace.json",
		WorkingDir:      "/tmp/agent",
	}

	env, args, err := nt.Setup(context.Background(), cfg, []string{"node", "agent.js"})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// Verify --require was injected
	if len(args) < 3 || args[1] != "--require" {
		t.Errorf("expected --require injection, got args: %v", args)
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
}

func TestNodeTracerInjectRequire(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantFlag string
	}{
		{"node", []string{"node", "app.js"}, "--require"},
		{"tsx", []string{"tsx", "app.ts"}, "--require"},
		{"bun", []string{"bun", "run", "app.ts"}, "--preload"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := injectRequire(tt.args, "/tmp/tracer.js")
			found := false
			for _, arg := range result {
				if arg == tt.wantFlag {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected %s in args, got: %v", tt.wantFlag, result)
			}
		})
	}
}

func TestNodeTracerIntegration(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not found, skipping integration test")
	}

	agentDir := t.TempDir()

	// Create a simple Node.js agent with CommonJS exports
	err := os.WriteFile(filepath.Join(agentDir, "helper.js"), []byte(`
function greet(name) {
  return "Hello, " + name + "!";
}

function process(query) {
  return greet("World") + " Query: " + query;
}

module.exports = { greet, process };
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(agentDir, "agent.js"), []byte(`
const { process } = require('./helper');
const result = process("test query");
console.log(result);
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	traceOutput := filepath.Join(t.TempDir(), "trace.json")

	nt := NewNodeTracer()
	defer nt.Cleanup()

	cfg := Config{
		TraceOutputPath: traceOutput,
		WorkingDir:      agentDir,
	}

	env, args, err := nt.Setup(context.Background(), cfg, []string{"node", filepath.Join(agentDir, "agent.js")})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir = agentDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("agent run failed: %v\nOutput: %s", err, out)
	}

	if !strings.Contains(string(out), "Hello, World!") {
		t.Errorf("unexpected output: %s", out)
	}

	spans, err := nt.Collect(cfg)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if len(spans) == 0 {
		t.Fatal("no spans collected")
	}

	funcNames := make(map[string]bool)
	for _, s := range spans {
		if s.Type == trace.SpanFunction {
			funcNames[s.Name] = true
		}
	}

	t.Logf("Captured functions: %v", funcNames)

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

func TestNodeTracerExecCapture(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not found")
	}

	agentDir := t.TempDir()
	os.WriteFile(filepath.Join(agentDir, "exec_agent.js"), []byte(`
const { execSync } = require('child_process');
const result = execSync('echo hello-aperio').toString().trim();
console.log(result);
`), 0644)

	traceOutput := filepath.Join(t.TempDir(), "trace.json")
	nt := NewNodeTracer()
	defer nt.Cleanup()

	cfg := Config{TraceOutputPath: traceOutput, WorkingDir: agentDir}
	env, args, err := nt.Setup(context.Background(), cfg, []string{"node", filepath.Join(agentDir, "exec_agent.js")})
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

	spans, err := nt.Collect(cfg)
	if err != nil {
		t.Fatal(err)
	}

	hasExec := false
	for _, s := range spans {
		if s.Type == trace.SpanType("EXEC") {
			hasExec = true
		}
	}
	if !hasExec {
		types := map[string]int{}
		for _, s := range spans {
			types[string(s.Type)]++
		}
		t.Errorf("expected EXEC span, types: %v", types)
	}
}

func TestNodeTracerFSCapture(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not found")
	}

	agentDir := t.TempDir()
	inputFile := filepath.Join(agentDir, "input.txt")
	os.WriteFile(inputFile, []byte("test-data"), 0644)

	os.WriteFile(filepath.Join(agentDir, "fs_agent.js"), []byte(`
const fs = require('fs');
const data = fs.readFileSync('`+inputFile+`', 'utf8');
fs.writeFileSync('`+filepath.Join(agentDir, "output.txt")+`', 'processed: ' + data);
console.log('done');
`), 0644)

	traceOutput := filepath.Join(t.TempDir(), "trace.json")
	nt := NewNodeTracer()
	defer nt.Cleanup()

	cfg := Config{TraceOutputPath: traceOutput, WorkingDir: agentDir}
	env, args, err := nt.Setup(context.Background(), cfg, []string{"node", filepath.Join(agentDir, "fs_agent.js")})
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

	spans, err := nt.Collect(cfg)
	if err != nil {
		t.Fatal(err)
	}

	types := map[string]int{}
	for _, s := range spans {
		types[string(s.Type)]++
	}

	if types["FS_READ"] == 0 {
		t.Errorf("expected FS_READ span, types: %v", types)
	}
	if types["FS_WRITE"] == 0 {
		t.Errorf("expected FS_WRITE span, types: %v", types)
	}
}

func TestNodeTracerAllIOCombined(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not found")
	}

	agentDir := t.TempDir()
	inputFile := filepath.Join(agentDir, "config.txt")
	os.WriteFile(inputFile, []byte("value=42"), 0644)

	os.WriteFile(filepath.Join(agentDir, "full_agent.js"), []byte(`
const fs = require('fs');
const { execSync } = require('child_process');
const config = fs.readFileSync('`+inputFile+`', 'utf8');
const result = execSync('echo ' + config.trim()).toString();
fs.writeFileSync('`+filepath.Join(agentDir, "result.txt")+`', result);
console.log('done');
`), 0644)

	traceOutput := filepath.Join(t.TempDir(), "trace.json")
	nt := NewNodeTracer()
	defer nt.Cleanup()

	cfg := Config{TraceOutputPath: traceOutput, WorkingDir: agentDir}
	env, args, err := nt.Setup(context.Background(), cfg, []string{"node", filepath.Join(agentDir, "full_agent.js")})
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

	spans, err := nt.Collect(cfg)
	if err != nil {
		t.Fatal(err)
	}

	types := map[string]int{}
	for _, s := range spans {
		types[string(s.Type)]++
	}

	for _, expected := range []string{"FS_READ", "FS_WRITE", "EXEC"} {
		if types[expected] == 0 {
			t.Errorf("expected %s span, types: %v", expected, types)
		}
	}
}

func TestNodeCollectMissingFile(t *testing.T) {
	nt := NewNodeTracer()
	_, err := nt.Collect(Config{TraceOutputPath: "/nonexistent/trace.json"})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestNodeCollectInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	os.WriteFile(path, []byte("not json"), 0644)

	nt := NewNodeTracer()
	_, err := nt.Collect(Config{TraceOutputPath: path})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestNodeCleanup(t *testing.T) {
	nt := NewNodeTracer()
	cfg := Config{TraceOutputPath: "/tmp/test.json", WorkingDir: "/tmp"}
	nt.Setup(context.Background(), cfg, []string{"node", "test.js"})

	tmpDir := nt.tmpDir
	if tmpDir == "" {
		t.Fatal("tmpDir should be set after Setup")
	}
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		t.Fatal("temp dir should exist before cleanup")
	}

	nt.Cleanup()

	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Error("temp dir should be removed after cleanup")
	}
}
