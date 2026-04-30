package benchmark

import "context"

// Adapter generates BenchmarkSpecs from an external benchmark suite.
// Adapters are responsible for fetching/generating tasks, preparing workspaces,
// and defining validation criteria. They produce specs that flow through the
// standard Run() pipeline.
type Adapter interface {
	// Name returns the adapter identifier (e.g., "swe-bench", "exercism").
	Name() string

	// ListTasks returns available task IDs, optionally filtered.
	ListTasks(filter AdapterFilter) ([]string, error)

	// GenerateSpec produces a BenchmarkSpec for a given task ID and tool set.
	GenerateSpec(taskID string, opts AdapterOptions) (*BenchmarkSpec, error)

	// Setup performs one-time preparation (clone repos, pull Docker images, etc).
	Setup(ctx context.Context) error
}

// AdapterFilter controls which tasks an adapter returns.
type AdapterFilter struct {
	Limit      int      `yaml:"limit,omitempty" json:"limit,omitempty"`
	Difficulty string   `yaml:"difficulty,omitempty" json:"difficulty,omitempty"` // "easy", "medium", "hard"
	Languages  []string `yaml:"languages,omitempty" json:"languages,omitempty"`
	Tags       []string `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// AdapterOptions provides context for spec generation.
type AdapterOptions struct {
	Tools     []ToolSpec    `yaml:"tools" json:"tools"`
	Runs      int           `yaml:"runs,omitempty" json:"runs,omitempty"`
	OutputDir string        `yaml:"output_dir,omitempty" json:"output_dir,omitempty"`
	Docker    *DockerConfig `yaml:"docker,omitempty" json:"docker,omitempty"`
	Timeout   string        `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

// AdapterSpec is the YAML representation of an adapter configuration within a BenchmarkSpec.
type AdapterSpec struct {
	Name    string        `yaml:"name" json:"name"`
	Variant string        `yaml:"variant,omitempty" json:"variant,omitempty"`
	DataDir string        `yaml:"data_dir,omitempty" json:"data_dir,omitempty"`
	Filter  AdapterFilter `yaml:"filter,omitempty" json:"filter,omitempty"`
}
