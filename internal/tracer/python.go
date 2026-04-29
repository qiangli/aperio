package tracer

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qiangli/aperio/internal/trace"
)

//go:embed scripts/python_tracer.py
var pythonTracerScript embed.FS

// PythonTracer instruments Python agents via sys.settrace injection.
type PythonTracer struct {
	tmpDir string
}

func NewPythonTracer() *PythonTracer {
	return &PythonTracer{}
}

func (pt *PythonTracer) Setup(ctx context.Context, cfg Config, args []string) ([]string, []string, error) {
	tmpDir, err := os.MkdirTemp("", "aperio-python-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create temp dir: %w", err)
	}
	pt.tmpDir = tmpDir

	// Write the tracer script
	tracerContent, err := pythonTracerScript.ReadFile("scripts/python_tracer.py")
	if err != nil {
		return nil, nil, fmt.Errorf("read embedded tracer: %w", err)
	}

	tracerPath := filepath.Join(tmpDir, "aperio_tracer.py")
	if err := os.WriteFile(tracerPath, tracerContent, 0644); err != nil {
		return nil, nil, fmt.Errorf("write tracer script: %w", err)
	}

	// Write sitecustomize.py that imports and installs the tracer
	siteCustomize := `import aperio_tracer`
	sitePath := filepath.Join(tmpDir, "sitecustomize.py")
	if err := os.WriteFile(sitePath, []byte(siteCustomize), 0644); err != nil {
		return nil, nil, fmt.Errorf("write sitecustomize: %w", err)
	}

	// Build environment variables
	env := []string{
		fmt.Sprintf("APERIO_TRACE_OUTPUT=%s", cfg.TraceOutputPath),
		fmt.Sprintf("APERIO_TRACE_DIR=%s", cfg.WorkingDir),
	}

	if len(cfg.ExcludePatterns) > 0 {
		env = append(env, fmt.Sprintf("APERIO_EXCLUDE=%s", strings.Join(cfg.ExcludePatterns, ",")))
	}

	// Prepend our temp dir to PYTHONPATH so sitecustomize.py and aperio_tracer.py are found
	existingPythonPath := os.Getenv("PYTHONPATH")
	if existingPythonPath != "" {
		env = append(env, fmt.Sprintf("PYTHONPATH=%s:%s", tmpDir, existingPythonPath))
	} else {
		env = append(env, fmt.Sprintf("PYTHONPATH=%s", tmpDir))
	}

	return env, args, nil
}

// pySpan is the JSON structure written by python_tracer.py.
type pySpan struct {
	ID         string         `json:"id"`
	ParentID   string         `json:"parent_id"`
	Type       string         `json:"type"`
	Name       string         `json:"name"`
	StartTime  string         `json:"start_time"`
	EndTime    string         `json:"end_time"`
	Attributes map[string]any `json:"attributes"`
}

func (pt *PythonTracer) Collect(cfg Config) ([]*trace.Span, error) {
	data, err := os.ReadFile(cfg.TraceOutputPath)
	if err != nil {
		return nil, fmt.Errorf("read python trace output: %w", err)
	}

	var pySpans []pySpan
	if err := json.Unmarshal(data, &pySpans); err != nil {
		return nil, fmt.Errorf("unmarshal python trace: %w", err)
	}

	spans := make([]*trace.Span, 0, len(pySpans))
	for _, ps := range pySpans {
		startTime, _ := time.Parse(time.RFC3339Nano, ps.StartTime)
		endTime, _ := time.Parse(time.RFC3339Nano, ps.EndTime)

		spans = append(spans, &trace.Span{
			ID:         ps.ID,
			ParentID:   ps.ParentID,
			Type:       trace.SpanType(ps.Type),
			Name:       ps.Name,
			StartTime:  startTime,
			EndTime:    endTime,
			Attributes: ps.Attributes,
		})
	}

	return spans, nil
}

func (pt *PythonTracer) Cleanup() error {
	if pt.tmpDir != "" {
		return os.RemoveAll(pt.tmpDir)
	}
	return nil
}
