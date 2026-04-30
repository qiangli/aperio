package benchmark

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"
)

// BenchmarkSpec defines a complete benchmark run.
type BenchmarkSpec struct {
	Name        string                `yaml:"name" json:"name"`
	Description string                `yaml:"description" json:"description"`
	Task        TaskSpec              `yaml:"task" json:"task"`
	Tools       []ToolSpec            `yaml:"tools" json:"tools"`
	Runs        int                   `yaml:"runs" json:"runs"`
	Parallel    bool                  `yaml:"parallel" json:"parallel"`
	OutputDir   string                `yaml:"output_dir" json:"output_dir"`
	Pricing     map[string]ModelPrice `yaml:"pricing,omitempty" json:"pricing,omitempty"`
}

// TaskSpec describes what the tools should accomplish.
type TaskSpec struct {
	Query      string            `yaml:"query" json:"query"`
	WorkingDir string            `yaml:"working_dir" json:"working_dir"`
	SetupCmd   string            `yaml:"setup_cmd,omitempty" json:"setup_cmd,omitempty"`
	CleanupCmd string            `yaml:"cleanup_cmd,omitempty" json:"cleanup_cmd,omitempty"`
	Validation []ValidationCheck `yaml:"validation,omitempty" json:"validation,omitempty"`
	Timeout    string            `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

// ToolSpec describes a single tool to benchmark.
type ToolSpec struct {
	Name     string            `yaml:"name" json:"name"`
	Command  []string          `yaml:"command" json:"command"`
	Mode     string            `yaml:"mode" json:"mode"` // "source" or "blackbox"
	Env      map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Targets  []string          `yaml:"targets,omitempty" json:"targets,omitempty"`
	GoTracer string            `yaml:"go_tracer,omitempty" json:"go_tracer,omitempty"`
}

// ValidationCheck defines a post-run check.
type ValidationCheck struct {
	Type   string `yaml:"type" json:"type"`     // exit_code, file_exists, regex, command, git_diff
	Value  string `yaml:"value" json:"value"`
	Expect string `yaml:"expect,omitempty" json:"expect,omitempty"`
}

// ParseSpec reads and validates a benchmark specification from a YAML file.
func ParseSpec(path string) (*BenchmarkSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read spec: %w", err)
	}

	// Expand environment variables
	expanded := os.ExpandEnv(string(data))

	var spec BenchmarkSpec
	if err := yaml.Unmarshal([]byte(expanded), &spec); err != nil {
		return nil, fmt.Errorf("parse spec: %w", err)
	}

	if err := validateSpec(&spec); err != nil {
		return nil, fmt.Errorf("invalid spec: %w", err)
	}

	// Apply defaults
	if spec.Runs <= 0 {
		spec.Runs = 1
	}
	if spec.OutputDir == "" {
		spec.OutputDir = "benchmark-results"
	}

	return &spec, nil
}

func validateSpec(spec *BenchmarkSpec) error {
	if spec.Name == "" {
		return fmt.Errorf("name is required")
	}
	if spec.Task.Query == "" {
		return fmt.Errorf("task.query is required")
	}
	if len(spec.Tools) == 0 {
		return fmt.Errorf("at least one tool is required")
	}
	for i, tool := range spec.Tools {
		if tool.Name == "" {
			return fmt.Errorf("tools[%d].name is required", i)
		}
		if len(tool.Command) == 0 {
			return fmt.Errorf("tools[%d].command is required", i)
		}
		if tool.Mode != "source" && tool.Mode != "blackbox" {
			return fmt.Errorf("tools[%d].mode must be 'source' or 'blackbox', got %q", i, tool.Mode)
		}
	}
	return nil
}

// ParseTimeout parses the timeout string into a duration.
func ParseTimeout(s string) (time.Duration, error) {
	if s == "" {
		return 5 * time.Minute, nil // default
	}
	return time.ParseDuration(s)
}

// ExpandCommand applies template expansion to command arguments.
// Supports {{.Query}} for task query substitution.
func ExpandCommand(args []string, query string) ([]string, error) {
	data := struct{ Query string }{Query: query}

	result := make([]string, len(args))
	for i, arg := range args {
		if !strings.Contains(arg, "{{") {
			result[i] = arg
			continue
		}
		tmpl, err := template.New("arg").Parse(arg)
		if err != nil {
			return nil, fmt.Errorf("parse template in arg %d: %w", i, err)
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return nil, fmt.Errorf("expand template in arg %d: %w", i, err)
		}
		result[i] = buf.String()
	}
	return result, nil
}
