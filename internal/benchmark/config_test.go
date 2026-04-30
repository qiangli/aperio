package benchmark

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSpecValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.yaml")
	content := `
name: "test-benchmark"
description: "A test"
runs: 3
output_dir: "./results"
task:
  query: "Hello world"
  working_dir: "/tmp"
  timeout: "2m"
tools:
  - name: tool-a
    command: ["echo", "{{.Query}}"]
    mode: source
  - name: tool-b
    command: ["echo", "hello"]
    mode: blackbox
`
	os.WriteFile(path, []byte(content), 0644)

	spec, err := ParseSpec(path)
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}

	if spec.Name != "test-benchmark" {
		t.Errorf("name: %s", spec.Name)
	}
	if spec.Runs != 3 {
		t.Errorf("runs: %d", spec.Runs)
	}
	if len(spec.Tools) != 2 {
		t.Errorf("tools: %d", len(spec.Tools))
	}
	if spec.Tools[0].Mode != "source" {
		t.Errorf("tool-a mode: %s", spec.Tools[0].Mode)
	}
}

func TestParseSpecDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.yaml")
	content := `
name: "test"
task:
  query: "test query"
tools:
  - name: t
    command: ["echo"]
    mode: source
`
	os.WriteFile(path, []byte(content), 0644)

	spec, err := ParseSpec(path)
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}

	if spec.Runs != 1 {
		t.Errorf("default runs should be 1, got %d", spec.Runs)
	}
	if spec.OutputDir != "benchmark-results" {
		t.Errorf("default output_dir: %s", spec.OutputDir)
	}
}

func TestParseSpecMissingName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.yaml")
	content := `
task:
  query: "test"
tools:
  - name: t
    command: ["echo"]
    mode: source
`
	os.WriteFile(path, []byte(content), 0644)

	_, err := ParseSpec(path)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestParseSpecInvalidMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.yaml")
	content := `
name: "test"
task:
  query: "test"
tools:
  - name: t
    command: ["echo"]
    mode: invalid
`
	os.WriteFile(path, []byte(content), 0644)

	_, err := ParseSpec(path)
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestParseSpecMissingFile(t *testing.T) {
	_, err := ParseSpec("/nonexistent/spec.yaml")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseSpecEnvExpansion(t *testing.T) {
	t.Setenv("TEST_QUERY", "expanded-query")

	dir := t.TempDir()
	path := filepath.Join(dir, "spec.yaml")
	content := `
name: "test"
task:
  query: "${TEST_QUERY}"
tools:
  - name: t
    command: ["echo"]
    mode: source
`
	os.WriteFile(path, []byte(content), 0644)

	spec, err := ParseSpec(path)
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	if spec.Task.Query != "expanded-query" {
		t.Errorf("expected expanded query, got %s", spec.Task.Query)
	}
}

func TestExpandCommand(t *testing.T) {
	args := []string{"tool", "--message", "{{.Query}}", "--flag"}
	result, err := ExpandCommand(args, "hello world")
	if err != nil {
		t.Fatalf("ExpandCommand: %v", err)
	}

	if result[0] != "tool" {
		t.Errorf("expected 'tool', got %s", result[0])
	}
	if result[2] != "hello world" {
		t.Errorf("expected 'hello world', got %s", result[2])
	}
	if result[3] != "--flag" {
		t.Errorf("expected '--flag', got %s", result[3])
	}
}

func TestExpandCommandNoTemplate(t *testing.T) {
	args := []string{"echo", "hello"}
	result, err := ExpandCommand(args, "query")
	if err != nil {
		t.Fatalf("ExpandCommand: %v", err)
	}
	if result[1] != "hello" {
		t.Errorf("non-template args should be unchanged")
	}
}

func TestParseTimeout(t *testing.T) {
	d, err := ParseTimeout("5m")
	if err != nil {
		t.Fatalf("ParseTimeout: %v", err)
	}
	if d.Minutes() != 5 {
		t.Errorf("expected 5m, got %v", d)
	}
}

func TestParseTimeoutDefault(t *testing.T) {
	d, err := ParseTimeout("")
	if err != nil {
		t.Fatalf("ParseTimeout: %v", err)
	}
	if d.Minutes() != 5 {
		t.Errorf("default should be 5m, got %v", d)
	}
}
