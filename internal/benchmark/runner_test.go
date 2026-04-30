package benchmark

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunShellCmd(t *testing.T) {
	err := runShellCmd(context.Background(), "echo hello", t.TempDir())
	if err != nil {
		t.Fatalf("runShellCmd: %v", err)
	}
}

func TestRunShellCmdFail(t *testing.T) {
	err := runShellCmd(context.Background(), "false", t.TempDir())
	if err == nil {
		t.Fatal("expected error for failing command")
	}
}

func TestRunBasicBenchmark(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	dir := t.TempDir()
	workDir := filepath.Join(dir, "workspace")
	os.MkdirAll(workDir, 0755)

	spec := &BenchmarkSpec{
		Name: "test-benchmark",
		Task: TaskSpec{
			Query:      "test query",
			WorkingDir: workDir,
			Validation: []ValidationCheck{
				{Type: "exit_code", Expect: "0"},
			},
		},
		Tools: []ToolSpec{
			{
				Name:    "echo-tool",
				Command: []string{"echo", "{{.Query}}"},
				Mode:    "blackbox",
			},
		},
		Runs:      1,
		OutputDir: filepath.Join(dir, "results"),
	}

	results, err := Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(results.ToolRuns) != 1 {
		t.Fatalf("expected 1 tool run, got %d", len(results.ToolRuns))
	}

	toolRun := results.ToolRuns[0]
	if len(toolRun.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(toolRun.Runs))
	}

	run := toolRun.Runs[0]
	if run.ExitCode != 0 {
		t.Errorf("exit code: expected 0, got %d", run.ExitCode)
	}

	// Trace file should exist
	if _, err := os.Stat(run.TracePath); err != nil {
		t.Errorf("trace file missing: %v", err)
	}

	// Validation should pass
	if len(run.Validation) != 1 {
		t.Fatalf("expected 1 validation, got %d", len(run.Validation))
	}
	if !run.Validation[0].Passed {
		t.Errorf("validation failed: %s", run.Validation[0].Detail)
	}
}

func TestRunMultipleRuns(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	dir := t.TempDir()
	spec := &BenchmarkSpec{
		Name: "multi-run",
		Task: TaskSpec{
			Query:      "hello",
			WorkingDir: dir,
		},
		Tools: []ToolSpec{
			{
				Name:    "echo",
				Command: []string{"echo", "{{.Query}}"},
				Mode:    "blackbox",
			},
		},
		Runs:      3,
		OutputDir: filepath.Join(dir, "results"),
	}

	results, err := Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(results.ToolRuns[0].Runs) != 3 {
		t.Errorf("expected 3 runs, got %d", len(results.ToolRuns[0].Runs))
	}

	// Each run should have a unique trace file
	paths := make(map[string]bool)
	for _, run := range results.ToolRuns[0].Runs {
		if paths[run.TracePath] {
			t.Errorf("duplicate trace path: %s", run.TracePath)
		}
		paths[run.TracePath] = true
	}
}

func TestRunWithSetupCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	dir := t.TempDir()
	markerFile := filepath.Join(dir, "setup-marker")

	spec := &BenchmarkSpec{
		Name: "setup-test",
		Task: TaskSpec{
			Query:      "test",
			WorkingDir: dir,
			SetupCmd:   "touch " + markerFile,
			CleanupCmd: "rm -f " + markerFile,
		},
		Tools: []ToolSpec{
			{Name: "echo", Command: []string{"echo"}, Mode: "blackbox"},
		},
		Runs:      1,
		OutputDir: filepath.Join(dir, "results"),
	}

	_, err := Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Cleanup should have removed the marker
	if _, err := os.Stat(markerFile); !os.IsNotExist(err) {
		t.Error("cleanup should have removed marker file")
	}
}

func TestRunSetupFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	dir := t.TempDir()
	spec := &BenchmarkSpec{
		Name: "setup-fail",
		Task: TaskSpec{
			Query:      "test",
			WorkingDir: dir,
			SetupCmd:   "false",
		},
		Tools: []ToolSpec{
			{Name: "echo", Command: []string{"echo"}, Mode: "blackbox"},
		},
		Runs:      1,
		OutputDir: filepath.Join(dir, "results"),
	}

	results, err := Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Run should have an error from setup failure
	if results.ToolRuns[0].Runs[0].Error == "" {
		t.Error("expected error from failed setup")
	}
}
