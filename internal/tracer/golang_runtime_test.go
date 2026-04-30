package tracer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoRuntimeTracerSetupGoRun(t *testing.T) {
	rt := NewGoRuntimeTracer()
	defer rt.Cleanup()

	// Create a temp package dir to simulate go run target
	tmpDir := t.TempDir()
	mainFile := filepath.Join(tmpDir, "main.go")
	os.WriteFile(mainFile, []byte("package main\nfunc main() {}\n"), 0644)

	cfg := Config{
		TraceOutputPath: filepath.Join(t.TempDir(), "trace.out"),
		WorkingDir:      tmpDir,
	}

	env, args, err := rt.Setup(context.Background(), cfg, []string{"go", "run", tmpDir})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// Should have env var for trace output
	foundTraceEnv := false
	for _, e := range env {
		if strings.HasPrefix(e, "APERIO_RUNTIME_TRACE_OUTPUT=") {
			foundTraceEnv = true
		}
	}
	if !foundTraceEnv {
		t.Error("expected APERIO_RUNTIME_TRACE_OUTPUT env var")
	}

	// Should have -overlay flag injected
	foundOverlay := false
	for _, a := range args {
		if strings.HasPrefix(a, "-overlay=") {
			foundOverlay = true
			// Verify overlay file exists
			overlayPath := strings.TrimPrefix(a, "-overlay=")
			if _, err := os.Stat(overlayPath); err != nil {
				t.Errorf("overlay file missing: %v", err)
			}
		}
	}
	if !foundOverlay {
		t.Error("expected -overlay flag in args")
	}

	// Verify args structure: go run -overlay=... <pkg> [args...]
	if args[0] != "go" || args[1] != "run" {
		t.Errorf("expected 'go run ...', got %v", args[:2])
	}
}

func TestGoRuntimeTracerSetupNonGoRun(t *testing.T) {
	rt := NewGoRuntimeTracer()
	defer rt.Cleanup()

	cfg := Config{TraceOutputPath: "/tmp/trace.out"}

	env, args, err := rt.Setup(context.Background(), cfg, []string{"./myagent", "arg1"})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// Should return original args unchanged (graceful fallback)
	if len(env) != 0 {
		t.Errorf("expected no env vars for non-go-run, got %v", env)
	}
	if len(args) != 2 || args[0] != "./myagent" {
		t.Errorf("expected original args, got %v", args)
	}
}

func TestGoRuntimeTracerCollectMissingFile(t *testing.T) {
	rt := &GoRuntimeTracer{traceFile: "/nonexistent/trace.out"}

	spans, err := rt.Collect(Config{})
	if err != nil {
		t.Fatalf("Collect should not error for missing file: %v", err)
	}
	if len(spans) != 0 {
		t.Errorf("expected 0 spans for missing file, got %d", len(spans))
	}
}

func TestGoRuntimeTracerCollectEmptyFile(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "trace.out")
	os.WriteFile(tmpFile, []byte{}, 0644)

	rt := &GoRuntimeTracer{traceFile: tmpFile}

	spans, err := rt.Collect(Config{})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(spans) != 0 {
		t.Errorf("expected 0 spans for empty file, got %d", len(spans))
	}
}

func TestGoRuntimeTracerCollectValidData(t *testing.T) {
	// Use real trace fixture
	rt := &GoRuntimeTracer{traceFile: "../../testdata/go_runtime_trace.out"}

	spans, err := rt.Collect(Config{})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(spans) == 0 {
		t.Fatal("expected spans from real trace, got 0")
	}

	// Should have goroutine, region, task, or log spans
	hasGoroutine := false
	hasRegion := false
	for _, s := range spans {
		if s.Type == "GOROUTINE" {
			hasGoroutine = true
		}
		if s.Type == "FUNCTION" && s.Attributes != nil {
			if _, ok := s.Attributes["user.region"]; ok {
				hasRegion = true
			}
		}
	}
	if !hasGoroutine {
		t.Error("expected at least one GOROUTINE span")
	}
	if !hasRegion {
		t.Error("expected at least one user region span (from trace.WithRegion)")
	}
}

func TestParseRuntimeTraceFullFallback(t *testing.T) {
	// Corrupted data should trigger fallback to basic
	tmpFile := filepath.Join(t.TempDir(), "trace.out")
	fakeData := make([]byte, 100)
	copy(fakeData, []byte("not a real trace"))
	os.WriteFile(tmpFile, fakeData, 0644)

	rt := &GoRuntimeTracer{traceFile: tmpFile}

	spans, err := rt.Collect(Config{})
	if err != nil {
		t.Fatalf("Collect should not error (fallback): %v", err)
	}
	// Should get at least the basic summary span
	if len(spans) == 0 {
		t.Error("expected fallback to produce at least 1 span")
	}
}

func TestGoRuntimeTracerCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "aperio-test")
	os.Mkdir(subDir, 0755)

	rt := &GoRuntimeTracer{tmpDir: subDir}
	if err := rt.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	if _, err := os.Stat(subDir); !os.IsNotExist(err) {
		t.Error("expected temp dir to be removed")
	}
}

func TestGoRuntimeTracerCleanupEmpty(t *testing.T) {
	rt := &GoRuntimeTracer{}
	if err := rt.Cleanup(); err != nil {
		t.Fatalf("Cleanup should not error with empty tmpDir: %v", err)
	}
}

func TestGenerateTraceInitCode(t *testing.T) {
	code := generateTraceInitCode()
	if !strings.Contains(code, "package main") {
		t.Error("init code should be in package main")
	}
	if !strings.Contains(code, "runtime/trace") {
		t.Error("init code should import runtime/trace")
	}
	if !strings.Contains(code, "APERIO_RUNTIME_TRACE_OUTPUT") {
		t.Error("init code should read APERIO_RUNTIME_TRACE_OUTPUT")
	}
	if !strings.Contains(code, "trace.Start") {
		t.Error("init code should call trace.Start")
	}
}

func TestResolvePackageDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Test with directory path
	dir, err := resolvePackageDir([]string{"go", "run", tmpDir})
	if err != nil {
		t.Fatalf("resolvePackageDir: %v", err)
	}
	if dir != tmpDir {
		t.Errorf("expected %s, got %s", tmpDir, dir)
	}

	// Test with file path
	mainFile := filepath.Join(tmpDir, "main.go")
	os.WriteFile(mainFile, []byte("package main"), 0644)
	dir, err = resolvePackageDir([]string{"go", "run", mainFile})
	if err != nil {
		t.Fatalf("resolvePackageDir with file: %v", err)
	}
	if dir != tmpDir {
		t.Errorf("expected %s, got %s", tmpDir, dir)
	}
}

func TestParseRuntimeTraceBasicTooShort(t *testing.T) {
	spans := parseRuntimeTraceBasic([]byte("short"))
	if len(spans) != 0 {
		t.Errorf("expected 0 spans for short data, got %d", len(spans))
	}
}
