package benchmark

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// DockerConfig controls container-based execution isolation.
type DockerConfig struct {
	Enabled bool              `yaml:"enabled" json:"enabled"`
	Image   string            `yaml:"image" json:"image"`
	Memory  string            `yaml:"memory,omitempty" json:"memory,omitempty"`
	Timeout string            `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Network string            `yaml:"network,omitempty" json:"network,omitempty"` // "host", "none", or custom
	Volumes []string          `yaml:"volumes,omitempty" json:"volumes,omitempty"`
	Env     map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
}

// DockerRunner wraps tool execution in a Docker container.
type DockerRunner struct {
	Config DockerConfig
}

// DockerRunOptions describes a single containerized execution.
type DockerRunOptions struct {
	Command    []string
	WorkingDir string
	Env        map[string]string
	TracePath  string // host path where trace output should land
	Timeout    time.Duration
}

// Run executes a command inside a Docker container and copies the trace out.
func (d *DockerRunner) Run(ctx context.Context, opts DockerRunOptions) (int, error) {
	args := d.buildArgs(opts)

	log.Debug().
		Str("image", d.Config.Image).
		Strs("command", opts.Command).
		Msg("docker run")

	runCtx := ctx
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return 1, fmt.Errorf("docker run: %w", err)
	}
	return 0, nil
}

func (d *DockerRunner) buildArgs(opts DockerRunOptions) []string {
	args := []string{"run", "--rm"}

	// Resource limits
	if d.Config.Memory != "" {
		args = append(args, "--memory", d.Config.Memory)
	}

	// Network mode
	if d.Config.Network != "" {
		args = append(args, "--network", d.Config.Network)
	}

	// Working directory
	if opts.WorkingDir != "" {
		args = append(args, "-w", "/workspace")
		args = append(args, "-v", opts.WorkingDir+":/workspace")
	}

	// Trace output volume
	if opts.TracePath != "" {
		traceDir := filepath.Dir(opts.TracePath)
		args = append(args, "-v", traceDir+":/traces")
	}

	// Additional volumes from config
	for _, vol := range d.Config.Volumes {
		args = append(args, "-v", vol)
	}

	// Environment variables from config
	for k, v := range d.Config.Env {
		args = append(args, "-e", k+"="+v)
	}

	// Environment variables from run options
	for k, v := range opts.Env {
		args = append(args, "-e", k+"="+v)
	}

	// Trace output path inside container
	if opts.TracePath != "" {
		traceName := filepath.Base(opts.TracePath)
		args = append(args, "-e", "APERIO_TRACE_OUTPUT=/traces/"+traceName)
	}

	args = append(args, d.Config.Image)
	args = append(args, opts.Command...)

	return args
}

// IsDockerAvailable checks if Docker is accessible.
func IsDockerAvailable() bool {
	cmd := exec.Command("docker", "info")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// PullImage ensures the Docker image is available locally.
func PullImage(ctx context.Context, image string) error {
	// Check if already present
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", image)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if cmd.Run() == nil {
		return nil
	}

	log.Info().Str("image", image).Msg("pulling Docker image")
	pull := exec.CommandContext(ctx, "docker", "pull", image)
	pull.Stdout = os.Stdout
	pull.Stderr = os.Stderr
	return pull.Run()
}

// FormatDockerCommand returns the full docker command as a string for debugging.
func FormatDockerCommand(args []string) string {
	return "docker " + strings.Join(args, " ")
}
