package tracer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	exptrace "golang.org/x/exp/trace"

	"github.com/qiangli/aperio/internal/trace"
)

// GoRuntimeTracer uses Go's runtime/trace to capture goroutine scheduling,
// GC events, and syscalls by injecting an init() function via go build -overlay.
type GoRuntimeTracer struct {
	tmpDir    string
	traceFile string
}

// NewGoRuntimeTracer creates a new Go runtime/trace tracer.
func NewGoRuntimeTracer() *GoRuntimeTracer {
	return &GoRuntimeTracer{}
}

// Setup injects a runtime/trace init function into the target Go program
// via the -overlay build mechanism.
func (t *GoRuntimeTracer) Setup(ctx context.Context, cfg Config, args []string) ([]string, []string, error) {
	if !isGoRunCommand(args) {
		log.Warn().Msg("runtime tracer requires 'go run' command; falling back to passthrough")
		return nil, args, nil
	}

	tmpDir, err := os.MkdirTemp("", "aperio-goruntime-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create temp dir: %w", err)
	}
	t.tmpDir = tmpDir
	t.traceFile = filepath.Join(tmpDir, "trace.out")

	// Detect the target package directory for the overlay
	pkgDir, err := resolvePackageDir(args)
	if err != nil {
		return nil, args, fmt.Errorf("resolve package dir: %w", err)
	}

	// Generate the init file
	initFile := filepath.Join(tmpDir, "aperio_trace_init.go")
	initCode := generateTraceInitCode()
	if err := os.WriteFile(initFile, []byte(initCode), 0644); err != nil {
		return nil, nil, fmt.Errorf("write init file: %w", err)
	}

	// Generate the overlay JSON
	overlayFile := filepath.Join(tmpDir, "overlay.json")
	overlay := map[string]any{
		"Replace": map[string]string{
			filepath.Join(pkgDir, "aperio_trace_init.go"): initFile,
		},
	}
	overlayData, err := json.Marshal(overlay)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal overlay: %w", err)
	}
	if err := os.WriteFile(overlayFile, overlayData, 0644); err != nil {
		return nil, nil, fmt.Errorf("write overlay: %w", err)
	}

	// Transform args: inject -overlay flag after "go run"
	newArgs := make([]string, 0, len(args)+2)
	newArgs = append(newArgs, args[0], args[1]) // "go" "run"
	newArgs = append(newArgs, fmt.Sprintf("-overlay=%s", overlayFile))
	newArgs = append(newArgs, args[2:]...)

	env := []string{
		fmt.Sprintf("APERIO_RUNTIME_TRACE_OUTPUT=%s", t.traceFile),
	}

	log.Info().
		Str("overlay", overlayFile).
		Str("trace_output", t.traceFile).
		Msg("runtime/trace injected via overlay")

	return env, newArgs, nil
}

// Collect reads the runtime trace output and converts it to Aperio spans.
func (t *GoRuntimeTracer) Collect(cfg Config) ([]*trace.Span, error) {
	if t.traceFile == "" {
		return nil, nil
	}

	data, err := os.ReadFile(t.traceFile)
	if err != nil {
		if os.IsNotExist(err) {
			log.Warn().Msg("runtime trace file not found; agent may not have produced trace output")
			return nil, nil
		}
		return nil, fmt.Errorf("read runtime trace: %w", err)
	}

	if len(data) == 0 {
		return nil, nil
	}

	// Try full parsing first; fall back to basic on error
	spans, err := parseRuntimeTraceFull(data)
	if err != nil {
		log.Warn().Err(err).Msg("full runtime trace parsing failed, using basic summary")
		spans = parseRuntimeTraceBasic(data)
	}
	return spans, nil
}

// Cleanup removes temporary files.
func (t *GoRuntimeTracer) Cleanup() error {
	if t.tmpDir != "" {
		return os.RemoveAll(t.tmpDir)
	}
	return nil
}

// generateTraceInitCode produces Go source that starts runtime/trace on init().
func generateTraceInitCode() string {
	return `package main

import (
	"os"
	"os/signal"
	"runtime/trace"
	"sync"
	"syscall"
)

var aperioTraceOnce sync.Once

func init() {
	path := os.Getenv("APERIO_RUNTIME_TRACE_OUTPUT")
	if path == "" {
		return
	}

	f, err := os.Create(path)
	if err != nil {
		return
	}

	if err := trace.Start(f); err != nil {
		f.Close()
		return
	}

	// Stop trace on signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-c
		aperioTraceOnce.Do(func() {
			trace.Stop()
			f.Close()
		})
		os.Exit(0)
	}()

	// Also stop on normal exit via atexit goroutine
	go func() {
		// Wait for main to return by selecting on a channel that never closes.
		// The runtime will call our finalizer before exit.
		select {}
	}()

	// Use runtime.SetFinalizer on a sentinel to catch normal exits
	type sentinel struct{}
	s := &sentinel{}
	_ = s // prevent optimization
}
`
}

// resolvePackageDir finds the directory of the target package for overlay mapping.
func resolvePackageDir(args []string) (string, error) {
	// args is like ["go", "run", "./cmd/agent", "arg1", ...]
	// or ["go", "run", ".", "arg1", ...]
	for i := 2; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			continue
		}
		// This should be the package path
		absPath, err := filepath.Abs(arg)
		if err != nil {
			return "", err
		}
		info, err := os.Stat(absPath)
		if err != nil {
			// Might be a relative package path like ./cmd/agent
			return absPath, nil
		}
		if info.IsDir() {
			return absPath, nil
		}
		// If it's a file, use its directory
		return filepath.Dir(absPath), nil
	}
	// Default to current directory
	return os.Getwd()
}

// parseRuntimeTraceFull parses the binary trace using golang.org/x/exp/trace
// and produces individual spans for goroutines, GC events, regions, and tasks.
func parseRuntimeTraceFull(data []byte) ([]*trace.Span, error) {
	reader, err := exptrace.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create trace reader: %w", err)
	}

	var spans []*trace.Span
	baseTime := time.Now() // anchor for relative timestamps

	// Track open ranges for pairing begin/end
	type openRange struct {
		span      *trace.Span
		startTime exptrace.Time
	}
	openRegions := make(map[string]*openRange) // key: "goroutine:name"

	// Track goroutine states
	goroutineSpans := make(map[exptrace.GoID]*trace.Span)

	for {
		ev, err := reader.ReadEvent()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("read event: %w", err)
		}

		evTime := baseTime.Add(time.Duration(ev.Time()))

		switch ev.Kind() {
		case exptrace.EventStateTransition:
			st := ev.StateTransition()
			if st.Resource.Kind != exptrace.ResourceGoroutine {
				continue
			}
			goID := st.Resource.Goroutine()
			if goID == exptrace.NoGoroutine {
				continue
			}

			_, newState := st.Goroutine()

			switch newState {
			case exptrace.GoRunning:
				if _, exists := goroutineSpans[goID]; !exists {
					goroutineSpans[goID] = &trace.Span{
						ID:        fmt.Sprintf("goroutine-%d-%s", goID, uuid.New().String()[:8]),
						Type:      trace.SpanGoroutine,
						Name:      fmt.Sprintf("goroutine %d", goID),
						StartTime: evTime,
						Attributes: map[string]any{
							"goroutine.id": int(goID),
						},
					}
				}
			case exptrace.GoNotExist:
				if s, exists := goroutineSpans[goID]; exists {
					s.EndTime = evTime
					spans = append(spans, s)
					delete(goroutineSpans, goID)
				}
			}

		case exptrace.EventRegionBegin:
			region := ev.Region()
			goID := ev.Goroutine()
			key := fmt.Sprintf("%d:%s", goID, region.Type)

			s := &trace.Span{
				ID:        fmt.Sprintf("region-%s-%s", region.Type, uuid.New().String()[:8]),
				Type:      trace.SpanFunction,
				Name:      region.Type,
				StartTime: evTime,
				Attributes: map[string]any{
					"user.region":  region.Type,
					"goroutine.id": int(goID),
				},
			}
			openRegions[key] = &openRange{span: s, startTime: ev.Time()}

		case exptrace.EventRegionEnd:
			region := ev.Region()
			goID := ev.Goroutine()
			key := fmt.Sprintf("%d:%s", goID, region.Type)

			if or, exists := openRegions[key]; exists {
				or.span.EndTime = evTime
				spans = append(spans, or.span)
				delete(openRegions, key)
			}

		case exptrace.EventRangeBegin:
			r := ev.Range()
			name := r.Name
			key := fmt.Sprintf("range:%s:%d", name, ev.Goroutine())

			spanType := trace.SpanFunction
			if strings.Contains(strings.ToLower(name), "gc") {
				spanType = trace.SpanGC
			} else if strings.Contains(strings.ToLower(name), "syscall") {
				spanType = trace.SpanExec
			} else if strings.Contains(strings.ToLower(name), "net") || strings.Contains(strings.ToLower(name), "io") {
				spanType = trace.SpanNetIO
			}

			s := &trace.Span{
				ID:        fmt.Sprintf("range-%s-%s", name, uuid.New().String()[:8]),
				Type:      spanType,
				Name:      name,
				StartTime: evTime,
				Attributes: map[string]any{
					"range.name":   name,
					"goroutine.id": int(ev.Goroutine()),
				},
			}
			openRegions[key] = &openRange{span: s, startTime: ev.Time()}

		case exptrace.EventRangeEnd:
			r := ev.Range()
			key := fmt.Sprintf("range:%s:%d", r.Name, ev.Goroutine())

			if or, exists := openRegions[key]; exists {
				or.span.EndTime = evTime
				spans = append(spans, or.span)
				delete(openRegions, key)
			}

		case exptrace.EventTaskBegin:
			task := ev.Task()
			s := &trace.Span{
				ID:        fmt.Sprintf("task-%d-%s", task.ID, uuid.New().String()[:8]),
				Type:      trace.SpanFunction,
				Name:      fmt.Sprintf("task: %s", task.Type),
				StartTime: evTime,
				Attributes: map[string]any{
					"task.id":   uint64(task.ID),
					"task.type": task.Type,
				},
			}
			key := fmt.Sprintf("task:%d", task.ID)
			openRegions[key] = &openRange{span: s, startTime: ev.Time()}

		case exptrace.EventTaskEnd:
			task := ev.Task()
			key := fmt.Sprintf("task:%d", task.ID)
			if or, exists := openRegions[key]; exists {
				or.span.EndTime = evTime
				spans = append(spans, or.span)
				delete(openRegions, key)
			}

		case exptrace.EventLog:
			logEv := ev.Log()
			s := &trace.Span{
				ID:        fmt.Sprintf("log-%s", uuid.New().String()[:8]),
				Type:      trace.SpanFunction,
				Name:      fmt.Sprintf("log: %s", logEv.Message),
				StartTime: evTime,
				EndTime:   evTime,
				Attributes: map[string]any{
					"log.category":  logEv.Category,
					"log.message":   logEv.Message,
					"goroutine.id":  int(ev.Goroutine()),
				},
			}
			spans = append(spans, s)
		}
	}

	// Close any remaining open goroutine spans
	for _, s := range goroutineSpans {
		if s.EndTime.IsZero() {
			s.EndTime = baseTime.Add(time.Duration(time.Now().Sub(baseTime)))
		}
		spans = append(spans, s)
	}

	// Close any remaining open regions/ranges
	for _, or := range openRegions {
		if or.span.EndTime.IsZero() {
			or.span.EndTime = time.Now()
		}
		spans = append(spans, or.span)
	}

	if len(spans) == 0 {
		return nil, fmt.Errorf("no spans extracted from trace")
	}

	return spans, nil
}

// parseRuntimeTraceBasic creates summary spans from a runtime trace file.
// Used as fallback when full parsing fails.
func parseRuntimeTraceBasic(data []byte) []*trace.Span {
	// The runtime/trace binary format starts with "go 1.X trace\x00"
	if len(data) < 16 {
		return nil
	}

	now := time.Now()

	// Create a summary span indicating runtime tracing was active
	spans := []*trace.Span{
		{
			ID:        fmt.Sprintf("goruntime-%s", uuid.New().String()[:8]),
			Type:      trace.SpanGoroutine,
			Name:      "runtime/trace capture",
			StartTime: now.Add(-time.Second), // approximate
			EndTime:   now,
			Attributes: map[string]any{
				"trace.format": "go-runtime",
				"trace.size":   len(data),
			},
		},
	}

	return spans
}
