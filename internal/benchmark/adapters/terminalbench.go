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
	benchmark.RegisterAdapter("terminal-bench", newTerminalBenchAdapter)
}

// TerminalBenchTask represents a single Terminal-Bench evaluation task.
type TerminalBenchTask struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Difficulty  string   `json:"difficulty"`
	Docker      string   `json:"docker_image,omitempty"`
	SetupCmds   []string `json:"setup_commands,omitempty"`
	Validation  []TBValidation `json:"validation,omitempty"`
	Timeout     string   `json:"timeout,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// TBValidation represents a Terminal-Bench validation check.
type TBValidation struct {
	Type   string `json:"type"`
	Value  string `json:"value"`
	Expect string `json:"expect,omitempty"`
}

// TerminalBenchAdapter loads tasks from the Terminal-Bench dataset.
type TerminalBenchAdapter struct {
	spec  *benchmark.AdapterSpec
	tasks []TerminalBenchTask
}

func newTerminalBenchAdapter(spec *benchmark.AdapterSpec) (benchmark.Adapter, error) {
	return &TerminalBenchAdapter{spec: spec}, nil
}

func (a *TerminalBenchAdapter) Name() string { return "terminal-bench" }

func (a *TerminalBenchAdapter) Setup(ctx context.Context) error {
	dataDir := a.spec.DataDir
	if dataDir == "" {
		dataDir = "terminal-bench-data"
	}

	dataFile := filepath.Join(dataDir, "tasks.json")
	data, err := os.ReadFile(dataFile)
	if err != nil {
		return fmt.Errorf("read Terminal-Bench tasks %s: %w", dataFile, err)
	}

	if err := json.Unmarshal(data, &a.tasks); err != nil {
		return fmt.Errorf("parse Terminal-Bench tasks: %w", err)
	}

	return nil
}

func (a *TerminalBenchAdapter) ListTasks(filter benchmark.AdapterFilter) ([]string, error) {
	if len(a.tasks) == 0 {
		return nil, fmt.Errorf("adapter not set up; call Setup() first")
	}

	var ids []string
	for _, task := range a.tasks {
		if !matchTBFilter(task, filter) {
			continue
		}
		ids = append(ids, task.ID)
		if filter.Limit > 0 && len(ids) >= filter.Limit {
			break
		}
	}
	return ids, nil
}

func (a *TerminalBenchAdapter) GenerateSpec(taskID string, opts benchmark.AdapterOptions) (*benchmark.BenchmarkSpec, error) {
	task := a.findTask(taskID)
	if task == nil {
		return nil, fmt.Errorf("task %q not found", taskID)
	}

	workDir := filepath.Join(opts.OutputDir, "workspaces", taskID)

	// Build setup command from task setup commands
	setupCmd := fmt.Sprintf("mkdir -p %s", workDir)
	for _, cmd := range task.SetupCmds {
		setupCmd += " && " + cmd
	}

	// Convert TB validations to Aperio format
	var checks []benchmark.ValidationCheck
	for _, v := range task.Validation {
		checks = append(checks, benchmark.ValidationCheck{
			Type:   v.Type,
			Value:  v.Value,
			Expect: v.Expect,
		})
	}
	if len(checks) == 0 {
		checks = []benchmark.ValidationCheck{{Type: "exit_code", Expect: "0"}}
	}

	// If the task specifies a Docker image, use it
	docker := opts.Docker
	if task.Docker != "" && docker == nil {
		docker = &benchmark.DockerConfig{
			Enabled: true,
			Image:   task.Docker,
		}
	}

	timeout := opts.Timeout
	if timeout == "" && task.Timeout != "" {
		timeout = task.Timeout
	}

	spec := &benchmark.BenchmarkSpec{
		Name:      fmt.Sprintf("terminal-bench/%s", taskID),
		OutputDir: filepath.Join(opts.OutputDir, taskID),
		Tools:     opts.Tools,
		Runs:      opts.Runs,
		Docker:    docker,
		Task: benchmark.TaskSpec{
			Query:      task.Description,
			WorkingDir: workDir,
			SetupCmd:   setupCmd,
			CleanupCmd: fmt.Sprintf("rm -rf %s", workDir),
			Timeout:    timeout,
			Validation: checks,
		},
	}

	if spec.Runs <= 0 {
		spec.Runs = 1
	}
	if spec.Task.Timeout == "" {
		spec.Task.Timeout = "5m"
	}

	return spec, nil
}

func (a *TerminalBenchAdapter) findTask(id string) *TerminalBenchTask {
	for i := range a.tasks {
		if a.tasks[i].ID == id {
			return &a.tasks[i]
		}
	}
	return nil
}

func matchTBFilter(task TerminalBenchTask, filter benchmark.AdapterFilter) bool {
	if filter.Difficulty != "" && task.Difficulty != filter.Difficulty {
		return false
	}
	if len(filter.Tags) > 0 {
		matched := false
		for _, tag := range filter.Tags {
			for _, taskTag := range task.Tags {
				if tag == taskTag || tag == task.Category {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}
