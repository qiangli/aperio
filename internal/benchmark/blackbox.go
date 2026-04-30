package benchmark

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/qiangli/aperio/internal/proxy"
	"github.com/qiangli/aperio/internal/trace"
)

// BlackBoxOptions configures a black-box recording session.
type BlackBoxOptions struct {
	Command    []string
	WorkingDir string
	OutputPath string
	Targets    []string
	Env        map[string]string
	Timeout    time.Duration
}

// BlackBoxRecord traces a tool without source access by observing its external behavior:
// file system changes, network traffic (if tool honors HTTPS_PROXY), and wall-clock time.
func BlackBoxRecord(ctx context.Context, opts BlackBoxOptions) (*trace.Trace, int, error) {
	if len(opts.Command) == 0 {
		return nil, -1, fmt.Errorf("no command specified")
	}

	if opts.WorkingDir == "" {
		opts.WorkingDir, _ = os.Getwd()
	}

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	// Snapshot file state before execution
	beforeFiles, _ := snapshotFiles(opts.WorkingDir)

	// Start proxy for LLM capture (if tool honors HTTPS_PROXY)
	p, err := proxy.New(proxy.Options{
		Mode:    proxy.ModeRecord,
		Port:    0,
		Targets: opts.Targets,
	})
	if err != nil {
		return nil, -1, fmt.Errorf("start proxy: %w", err)
	}
	defer p.Close()
	go p.Serve()

	// Build environment
	env := os.Environ()
	proxyURL := fmt.Sprintf("http://127.0.0.1:%d", p.Port())
	env = append(env,
		fmt.Sprintf("HTTPS_PROXY=%s", proxyURL),
		fmt.Sprintf("HTTP_PROXY=%s", proxyURL),
		fmt.Sprintf("https_proxy=%s", proxyURL),
		fmt.Sprintf("http_proxy=%s", proxyURL),
		"NODE_TLS_REJECT_UNAUTHORIZED=0",
		"PYTHONHTTPSVERIFY=0",
	)
	for k, v := range opts.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Execute the tool
	startTime := time.Now()

	cmd := exec.CommandContext(ctx, opts.Command[0], opts.Command[1:]...)
	cmd.Env = env
	cmd.Dir = opts.WorkingDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	exitCode := 0
	cmdErr := cmd.Run()
	if cmdErr != nil {
		if exitErr, ok := cmdErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	endTime := time.Now()

	// Snapshot file state after execution
	afterFiles, _ := snapshotFiles(opts.WorkingDir)

	// Build trace from observations
	processSpanID := uuid.New().String()
	var spans []*trace.Span

	// PROCESS span for overall timing
	processSpan := &trace.Span{
		ID:        processSpanID,
		Type:      trace.SpanProcess,
		Name:      strings.Join(opts.Command, " "),
		StartTime: startTime,
		EndTime:   endTime,
		Attributes: map[string]any{
			"exit_code":       exitCode,
			"process.command": strings.Join(opts.Command, " "),
			"tracing.mode":   "blackbox",
		},
	}
	spans = append(spans, processSpan)

	// EXEC span for the command
	execSpan := &trace.Span{
		ID:        uuid.New().String(),
		ParentID:  processSpanID,
		Type:      trace.SpanExec,
		Name:      opts.Command[0],
		StartTime: startTime,
		EndTime:   endTime,
		Attributes: map[string]any{
			"command":   strings.Join(opts.Command, " "),
			"exit_code": exitCode,
		},
	}
	spans = append(spans, execSpan)

	// FS_WRITE spans for files that changed
	changed := diffFiles(beforeFiles, afterFiles)
	for _, f := range changed {
		spans = append(spans, &trace.Span{
			ID:        uuid.New().String(),
			ParentID:  processSpanID,
			Type:      trace.SpanFSWrite,
			Name:      f.path,
			StartTime: endTime, // approximate
			EndTime:   endTime,
			Attributes: map[string]any{
				"path":   f.path,
				"size":   f.size,
				"change": f.changeType,
			},
		})
	}

	// LLM_REQUEST spans from proxy (if tool honored HTTPS_PROXY)
	apiSpans := p.Spans()
	for _, s := range apiSpans {
		s.ParentID = processSpanID
	}
	spans = append(spans, apiSpans...)

	if len(apiSpans) == 0 {
		log.Warn().Str("tool", opts.Command[0]).Msg("no LLM traffic captured; tool may not honor HTTPS_PROXY")
	}

	result := &trace.Trace{
		ID:        uuid.New().String(),
		CreatedAt: time.Now(),
		Metadata: trace.Metadata{
			Command:    strings.Join(opts.Command, " "),
			Language:   "blackbox",
			WorkingDir: opts.WorkingDir,
		},
		Spans: spans,
	}

	// Enrich any captured LLM traffic
	trace.Enrich(result)

	// Write trace
	if opts.OutputPath != "" {
		if err := trace.WriteTrace(opts.OutputPath, result); err != nil {
			return nil, exitCode, fmt.Errorf("write trace: %w", err)
		}
	}

	return result, exitCode, nil
}

type fileInfo struct {
	path    string
	size    int64
	modTime time.Time
}

type fileChange struct {
	path       string
	size       int64
	changeType string // "created" or "modified"
}

func snapshotFiles(dir string) (map[string]fileInfo, error) {
	files := make(map[string]fileInfo)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		relPath, _ := filepath.Rel(dir, path)
		files[relPath] = fileInfo{
			path:    relPath,
			size:    info.Size(),
			modTime: info.ModTime(),
		}
		return nil
	})
	return files, err
}

func diffFiles(before, after map[string]fileInfo) []fileChange {
	var changes []fileChange

	for path, afterInfo := range after {
		beforeInfo, existed := before[path]
		if !existed {
			changes = append(changes, fileChange{
				path:       path,
				size:       afterInfo.size,
				changeType: "created",
			})
		} else if afterInfo.modTime != beforeInfo.modTime || afterInfo.size != beforeInfo.size {
			changes = append(changes, fileChange{
				path:       path,
				size:       afterInfo.size,
				changeType: "modified",
			})
		}
	}

	return changes
}
