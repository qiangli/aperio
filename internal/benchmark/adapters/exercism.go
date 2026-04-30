package adapters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qiangli/aperio/internal/benchmark"
)

func init() {
	benchmark.RegisterAdapter("exercism", newExercismAdapter)
}

// ExercismExercise represents a single exercise from an Exercism track.
type ExercismExercise struct {
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	Difficulty string `json:"difficulty"`
	Language   string `json:"language"`
	Track      string `json:"track"`
}

// ExercismAdapter loads exercises from Exercism track directories.
type ExercismAdapter struct {
	spec      *benchmark.AdapterSpec
	exercises []ExercismExercise
}

func newExercismAdapter(spec *benchmark.AdapterSpec) (benchmark.Adapter, error) {
	return &ExercismAdapter{spec: spec}, nil
}

func (a *ExercismAdapter) Name() string { return "exercism" }

func (a *ExercismAdapter) Setup(ctx context.Context) error {
	dataDir := a.spec.DataDir
	if dataDir == "" {
		dataDir = "exercism-data"
	}

	// Scan for track directories (e.g., exercism-data/python/exercises/practice/)
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return fmt.Errorf("read exercism data dir %s: %w", dataDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		track := entry.Name()
		practiceDir := filepath.Join(dataDir, track, "exercises", "practice")
		exercises, err := os.ReadDir(practiceDir)
		if err != nil {
			continue // track may not have practice dir
		}

		for _, ex := range exercises {
			if !ex.IsDir() {
				continue
			}
			a.exercises = append(a.exercises, ExercismExercise{
				Slug:     ex.Name(),
				Name:     ex.Name(),
				Language: track,
				Track:    track,
			})
		}
	}

	if len(a.exercises) == 0 {
		return fmt.Errorf("no exercises found in %s", dataDir)
	}

	return nil
}

func (a *ExercismAdapter) ListTasks(filter benchmark.AdapterFilter) ([]string, error) {
	if len(a.exercises) == 0 {
		return nil, fmt.Errorf("adapter not set up; call Setup() first")
	}

	var ids []string
	for _, ex := range a.exercises {
		if !matchExercismFilter(ex, filter) {
			continue
		}
		ids = append(ids, fmt.Sprintf("%s/%s", ex.Track, ex.Slug))
		if filter.Limit > 0 && len(ids) >= filter.Limit {
			break
		}
	}
	return ids, nil
}

func (a *ExercismAdapter) GenerateSpec(taskID string, opts benchmark.AdapterOptions) (*benchmark.BenchmarkSpec, error) {
	ex := a.findExercise(taskID)
	if ex == nil {
		return nil, fmt.Errorf("exercise %q not found", taskID)
	}

	dataDir := a.spec.DataDir
	if dataDir == "" {
		dataDir = "exercism-data"
	}

	exerciseDir := filepath.Join(dataDir, ex.Track, "exercises", "practice", ex.Slug)
	workDir := filepath.Join(opts.OutputDir, "workspaces", taskID)

	// Build the query from exercise instructions
	query := fmt.Sprintf("Implement the %s exercise in %s. Follow the instructions in the exercise directory.", ex.Name, ex.Language)
	instructionsPath := filepath.Join(exerciseDir, ".docs", "instructions.md")
	if data, err := os.ReadFile(instructionsPath); err == nil {
		query = string(data)
	}

	// Language-specific test command
	testCmd := exercismTestCommand(ex.Language, workDir)

	spec := &benchmark.BenchmarkSpec{
		Name:      fmt.Sprintf("exercism/%s", taskID),
		OutputDir: filepath.Join(opts.OutputDir, strings.ReplaceAll(taskID, "/", "-")),
		Tools:     opts.Tools,
		Runs:      opts.Runs,
		Docker:    opts.Docker,
		Task: benchmark.TaskSpec{
			Query:      query,
			WorkingDir: workDir,
			SetupCmd:   fmt.Sprintf("cp -r %s %s", exerciseDir, workDir),
			CleanupCmd: fmt.Sprintf("rm -rf %s", workDir),
			Timeout:    opts.Timeout,
			Validation: []benchmark.ValidationCheck{
				{Type: "exit_code", Expect: "0"},
				{Type: "command", Value: testCmd},
			},
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

func (a *ExercismAdapter) findExercise(taskID string) *ExercismExercise {
	for i := range a.exercises {
		id := fmt.Sprintf("%s/%s", a.exercises[i].Track, a.exercises[i].Slug)
		if id == taskID {
			return &a.exercises[i]
		}
	}
	return nil
}

func matchExercismFilter(ex ExercismExercise, filter benchmark.AdapterFilter) bool {
	if filter.Difficulty != "" && ex.Difficulty != filter.Difficulty {
		return false
	}
	if len(filter.Languages) > 0 {
		matched := false
		for _, lang := range filter.Languages {
			if strings.EqualFold(ex.Language, lang) {
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

func exercismTestCommand(language, workDir string) string {
	switch language {
	case "python":
		return fmt.Sprintf("cd %s && python -m pytest", workDir)
	case "go":
		return fmt.Sprintf("cd %s && go test ./...", workDir)
	case "javascript", "typescript":
		return fmt.Sprintf("cd %s && npm test", workDir)
	case "rust":
		return fmt.Sprintf("cd %s && cargo test", workDir)
	case "ruby":
		return fmt.Sprintf("cd %s && ruby *_test.rb", workDir)
	case "java":
		return fmt.Sprintf("cd %s && gradle test", workDir)
	case "cpp", "c":
		return fmt.Sprintf("cd %s && make test", workDir)
	default:
		return fmt.Sprintf("cd %s && echo 'no test runner for %s'", workDir, language)
	}
}
