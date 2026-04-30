package benchmark

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/qiangli/aperio/internal/runner"
	"github.com/qiangli/aperio/internal/trace"
)

// BenchmarkResults holds the complete output of a benchmark run.
type BenchmarkResults struct {
	Spec      BenchmarkSpec   `json:"spec"`
	StartTime time.Time       `json:"start_time"`
	EndTime   time.Time       `json:"end_time"`
	ToolRuns  []ToolRunResult `json:"tool_runs"`
}

// ToolRunResult holds the output of all runs for a single tool.
type ToolRunResult struct {
	Tool ToolSpec    `json:"tool"`
	Runs []RunResult `json:"runs"`
}

// RunResult holds the output of a single tool run.
type RunResult struct {
	RunIndex   int                `json:"run_index"`
	TracePath  string             `json:"trace_path"`
	ExitCode   int                `json:"exit_code"`
	Validation []ValidationResult `json:"validation"`
	Error      string             `json:"error,omitempty"`
	StartTime  time.Time          `json:"start_time"`
	EndTime    time.Time          `json:"end_time"`
}

// Run executes the full benchmark as defined in the spec.
func Run(ctx context.Context, spec *BenchmarkSpec) (*BenchmarkResults, error) {
	results := &BenchmarkResults{
		Spec:      *spec,
		StartTime: time.Now(),
	}

	// Create output directory
	if err := os.MkdirAll(spec.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	timeout, err := ParseTimeout(spec.Task.Timeout)
	if err != nil {
		return nil, fmt.Errorf("parse timeout: %w", err)
	}

	for _, tool := range spec.Tools {
		log.Info().Str("tool", tool.Name).Int("runs", spec.Runs).Msg("benchmarking tool")

		toolDir := filepath.Join(spec.OutputDir, tool.Name)
		if err := os.MkdirAll(toolDir, 0755); err != nil {
			return nil, fmt.Errorf("create tool dir: %w", err)
		}

		toolResult := ToolRunResult{Tool: tool}

		for i := 0; i < spec.Runs; i++ {
			runResult := runTool(ctx, spec, &tool, i, toolDir, timeout)
			toolResult.Runs = append(toolResult.Runs, runResult)

			log.Info().
				Str("tool", tool.Name).
				Int("run", i).
				Int("exit_code", runResult.ExitCode).
				Str("error", runResult.Error).
				Msg("run complete")
		}

		results.ToolRuns = append(results.ToolRuns, toolResult)
	}

	results.EndTime = time.Now()
	return results, nil
}

func runTool(ctx context.Context, spec *BenchmarkSpec, tool *ToolSpec, runIndex int, toolDir string, timeout time.Duration) RunResult {
	result := RunResult{
		RunIndex:  runIndex,
		StartTime: time.Now(),
	}

	tracePath := filepath.Join(toolDir, fmt.Sprintf("run-%d.json", runIndex))
	result.TracePath = tracePath

	// Run setup command
	if spec.Task.SetupCmd != "" {
		if err := runShellCmd(ctx, spec.Task.SetupCmd, spec.Task.WorkingDir); err != nil {
			result.Error = fmt.Sprintf("setup failed: %s", err)
			result.EndTime = time.Now()
			return result
		}
	}

	// Expand command templates
	expandedCmd, err := ExpandCommand(tool.Command, spec.Task.Query)
	if err != nil {
		result.Error = fmt.Sprintf("expand command: %s", err)
		result.EndTime = time.Now()
		return result
	}

	// Execute the tool
	var tr *trace.Trace
	var exitCode int

	switch tool.Mode {
	case "source":
		exitCode, err = runSourceMode(ctx, expandedCmd, tool, spec, tracePath, timeout)
		if err != nil && result.Error == "" {
			result.Error = err.Error()
		}
		// Load the trace for validation
		tr, _ = trace.ReadTrace(tracePath)

	case "blackbox":
		tr, exitCode, err = BlackBoxRecord(ctx, BlackBoxOptions{
			Command:    expandedCmd,
			WorkingDir: spec.Task.WorkingDir,
			OutputPath: tracePath,
			Targets:    tool.Targets,
			Env:        tool.Env,
			Timeout:    timeout,
		})
		if err != nil && result.Error == "" {
			result.Error = err.Error()
		}
	}

	result.ExitCode = exitCode

	// Run validation checks
	outputText := ""
	if tr != nil {
		for _, s := range tr.Spans {
			if s.Type == trace.SpanAgentOutput {
				if content, ok := s.Attributes["content"].(string); ok {
					outputText += content + "\n"
				}
			}
		}
	}
	result.Validation = RunChecks(spec.Task.Validation, spec.Task.WorkingDir, exitCode, outputText)

	// Run cleanup command
	if spec.Task.CleanupCmd != "" {
		if err := runShellCmd(ctx, spec.Task.CleanupCmd, spec.Task.WorkingDir); err != nil {
			log.Warn().Err(err).Msg("cleanup command failed")
		}
	}

	result.EndTime = time.Now()
	return result
}

func runSourceMode(ctx context.Context, command []string, tool *ToolSpec, spec *BenchmarkSpec, tracePath string, timeout time.Duration) (int, error) {
	runCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	err := runner.Record(runCtx, runner.RecordOptions{
		Command:         command,
		OutputPath:      tracePath,
		Targets:         tool.Targets,
		WorkingDir:      spec.Task.WorkingDir,
		GoTracerBackend: tool.GoTracer,
	})

	if err != nil {
		// Try to extract exit code from error
		return 1, err
	}
	return 0, nil
}

func runShellCmd(ctx context.Context, command, workDir string) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if workDir != "" {
		if _, err := os.Stat(workDir); err == nil {
			cmd.Dir = workDir
		}
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
