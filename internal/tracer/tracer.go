package tracer

import (
	"context"

	"github.com/qiangli/aperio/internal/trace"
)

// Config holds configuration for a tracer.
type Config struct {
	// IncludePatterns are glob patterns for modules/packages to trace.
	// If empty, traces everything in the working directory.
	IncludePatterns []string

	// ExcludePatterns are glob patterns for modules/packages to exclude.
	ExcludePatterns []string

	// TraceOutputPath is where the tracer writes its span data.
	TraceOutputPath string

	// WorkingDir is the agent's working directory.
	WorkingDir string
}

// Tracer instruments and launches an agent process, capturing function-level execution traces.
type Tracer interface {
	// Setup prepares the environment for tracing (e.g., writes injector scripts, sets env vars).
	// Returns environment variables to set and modified command args.
	Setup(ctx context.Context, cfg Config, args []string) (env []string, newArgs []string, err error)

	// Collect reads the trace output after the agent process exits and returns function spans.
	Collect(cfg Config) ([]*trace.Span, error)

	// Cleanup removes temporary files created during Setup.
	Cleanup() error
}
