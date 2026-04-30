// Package adapters provides benchmark adapter implementations for external
// evaluation suites (SWE-Bench, Exercism, Terminal-Bench).
package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/qiangli/aperio/internal/benchmark"
)

func init() {
	benchmark.RegisterAdapter("swe-bench", newSWEBenchAdapter)
}

// SWEBenchInstance represents a single SWE-Bench task.
type SWEBenchInstance struct {
	InstanceID  string `json:"instance_id"`
	Repo        string `json:"repo"`
	BaseCommit  string `json:"base_commit"`
	Problem     string `json:"problem_statement"`
	Patch       string `json:"patch"`
	TestPatch   string `json:"test_patch,omitempty"`
	Version     string `json:"version,omitempty"`
	Difficulty  string `json:"difficulty,omitempty"`
	Environment string `json:"environment_setup_commit,omitempty"`
}

// SWEBenchAdapter loads and runs SWE-Bench evaluation tasks.
type SWEBenchAdapter struct {
	spec      *benchmark.AdapterSpec
	instances []SWEBenchInstance
}

func newSWEBenchAdapter(spec *benchmark.AdapterSpec) (benchmark.Adapter, error) {
	return &SWEBenchAdapter{spec: spec}, nil
}

func (a *SWEBenchAdapter) Name() string { return "swe-bench" }

func (a *SWEBenchAdapter) Setup(ctx context.Context) error {
	dataDir := a.spec.DataDir
	if dataDir == "" {
		dataDir = "swe-bench-data"
	}

	// Look for dataset file based on variant
	variant := a.spec.Variant
	if variant == "" {
		variant = "verified"
	}

	dataFile := filepath.Join(dataDir, fmt.Sprintf("swe-bench-%s.json", variant))
	data, err := os.ReadFile(dataFile)
	if err != nil {
		return fmt.Errorf("read SWE-Bench dataset %s: %w (ensure data is downloaded to %s)", dataFile, err, dataDir)
	}

	if err := json.Unmarshal(data, &a.instances); err != nil {
		return fmt.Errorf("parse SWE-Bench dataset: %w", err)
	}

	return nil
}

func (a *SWEBenchAdapter) ListTasks(filter benchmark.AdapterFilter) ([]string, error) {
	if len(a.instances) == 0 {
		return nil, fmt.Errorf("adapter not set up; call Setup() first")
	}

	var ids []string
	for _, inst := range a.instances {
		if !matchFilter(inst, filter) {
			continue
		}
		ids = append(ids, inst.InstanceID)
		if filter.Limit > 0 && len(ids) >= filter.Limit {
			break
		}
	}
	return ids, nil
}

func (a *SWEBenchAdapter) GenerateSpec(taskID string, opts benchmark.AdapterOptions) (*benchmark.BenchmarkSpec, error) {
	inst := a.findInstance(taskID)
	if inst == nil {
		return nil, fmt.Errorf("instance %q not found", taskID)
	}

	workDir := filepath.Join(opts.OutputDir, "workspaces", taskID)

	spec := &benchmark.BenchmarkSpec{
		Name:      fmt.Sprintf("swe-bench/%s", taskID),
		OutputDir: filepath.Join(opts.OutputDir, taskID),
		Tools:     opts.Tools,
		Runs:      opts.Runs,
		Docker:    opts.Docker,
		Task: benchmark.TaskSpec{
			Query:      inst.Problem,
			WorkingDir: workDir,
			SetupCmd:   fmt.Sprintf("mkdir -p %s && git clone https://github.com/%s %s && cd %s && git checkout %s", workDir, inst.Repo, workDir, workDir, inst.BaseCommit),
			CleanupCmd: fmt.Sprintf("rm -rf %s", workDir),
			Timeout:    opts.Timeout,
			Validation: []benchmark.ValidationCheck{
				{Type: "exit_code", Expect: "0"},
				{Type: "command", Value: fmt.Sprintf("cd %s && git diff --quiet HEAD || echo 'has changes'", workDir)},
			},
		},
	}

	if spec.Runs <= 0 {
		spec.Runs = 1
	}
	if spec.Task.Timeout == "" {
		spec.Task.Timeout = "10m"
	}

	return spec, nil
}

func (a *SWEBenchAdapter) findInstance(id string) *SWEBenchInstance {
	for i := range a.instances {
		if a.instances[i].InstanceID == id {
			return &a.instances[i]
		}
	}
	return nil
}

func matchFilter(inst SWEBenchInstance, filter benchmark.AdapterFilter) bool {
	if filter.Difficulty != "" && inst.Difficulty != filter.Difficulty {
		return false
	}
	if len(filter.Tags) > 0 {
		matched := false
		for _, tag := range filter.Tags {
			if inst.Repo == tag || inst.InstanceID == tag {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}
