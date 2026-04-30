package benchmark

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckExitCodePass(t *testing.T) {
	r := runCheck(ValidationCheck{Type: "exit_code", Expect: "0"}, "", 0, "")
	if !r.Passed {
		t.Errorf("expected pass, got: %s", r.Detail)
	}
}

func TestCheckExitCodeFail(t *testing.T) {
	r := runCheck(ValidationCheck{Type: "exit_code", Expect: "0"}, "", 1, "")
	if r.Passed {
		t.Error("expected fail for exit code 1")
	}
}

func TestCheckExitCodeDefaultExpect(t *testing.T) {
	r := runCheck(ValidationCheck{Type: "exit_code"}, "", 0, "")
	if !r.Passed {
		t.Error("default expect should be 0")
	}
}

func TestCheckFileExistsPass(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.go"), []byte("package main"), 0644)

	r := runCheck(ValidationCheck{Type: "file_exists", Value: "test.go"}, dir, 0, "")
	if !r.Passed {
		t.Errorf("expected pass: %s", r.Detail)
	}
}

func TestCheckFileExistsFail(t *testing.T) {
	dir := t.TempDir()
	r := runCheck(ValidationCheck{Type: "file_exists", Value: "missing.go"}, dir, 0, "")
	if r.Passed {
		t.Error("expected fail for missing file")
	}
}

func TestCheckRegexPass(t *testing.T) {
	r := runCheck(ValidationCheck{Type: "regex", Value: `hello.*world`}, "", 0, "hello beautiful world")
	if !r.Passed {
		t.Errorf("expected pass: %s", r.Detail)
	}
}

func TestCheckRegexFail(t *testing.T) {
	r := runCheck(ValidationCheck{Type: "regex", Value: `xyz123`}, "", 0, "no match here")
	if r.Passed {
		t.Error("expected fail")
	}
}

func TestCheckRegexInvalid(t *testing.T) {
	r := runCheck(ValidationCheck{Type: "regex", Value: `[invalid`}, "", 0, "text")
	if r.Passed {
		t.Error("expected fail for invalid regex")
	}
}

func TestCheckCommandPass(t *testing.T) {
	r := runCheck(ValidationCheck{Type: "command", Value: "true"}, t.TempDir(), 0, "")
	if !r.Passed {
		t.Errorf("expected pass: %s", r.Detail)
	}
}

func TestCheckCommandFail(t *testing.T) {
	r := runCheck(ValidationCheck{Type: "command", Value: "false"}, t.TempDir(), 0, "")
	if r.Passed {
		t.Error("expected fail for 'false' command")
	}
}

func TestCheckUnknownType(t *testing.T) {
	r := runCheck(ValidationCheck{Type: "unknown"}, "", 0, "")
	if r.Passed {
		t.Error("expected fail for unknown type")
	}
}

func TestRunChecks(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)

	checks := []ValidationCheck{
		{Type: "exit_code", Expect: "0"},
		{Type: "file_exists", Value: "main.go"},
		{Type: "regex", Value: "hello"},
	}

	results := RunChecks(checks, dir, 0, "hello world")
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.Passed {
			t.Errorf("check %s failed: %s", r.Check.Type, r.Detail)
		}
	}
}

func TestComputePassRate(t *testing.T) {
	results := []ValidationResult{
		{Passed: true},
		{Passed: true},
		{Passed: false},
		{Passed: false},
	}
	rate := ComputePassRate(results)
	if rate != 0.5 {
		t.Errorf("expected 0.5, got %f", rate)
	}
}

func TestComputePassRateEmpty(t *testing.T) {
	rate := ComputePassRate(nil)
	if rate != 1.0 {
		t.Errorf("expected 1.0 for empty, got %f", rate)
	}
}

func TestComputePassRateAllPass(t *testing.T) {
	results := []ValidationResult{{Passed: true}, {Passed: true}}
	rate := ComputePassRate(results)
	if rate != 1.0 {
		t.Errorf("expected 1.0, got %f", rate)
	}
}
