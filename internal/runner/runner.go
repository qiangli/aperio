package runner

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
	"github.com/qiangli/aperio/internal/tracer"
)

// RecordOptions configures a recording session.
type RecordOptions struct {
	// Command is the agent command and args (e.g., ["python", "agent.py", "hello"]).
	Command []string

	// OutputPath is where to write the trace file.
	OutputPath string

	// Targets are LLM API hosts to intercept (empty = all).
	Targets []string

	// WorkingDir is the agent's working directory (default: current dir).
	WorkingDir string

	// ExcludePatterns for function tracing.
	ExcludePatterns []string
}

// ReplayOptions configures a replay session.
type ReplayOptions struct {
	// Command is the agent command and args.
	Command []string

	// InputPath is the recorded trace file to replay from.
	InputPath string

	// OutputPath is where to write the replay trace (for comparison).
	OutputPath string

	// Strict fails on unmatched requests.
	Strict bool

	// MatchStrategy is "sequential" or "fingerprint".
	MatchStrategy string

	// WorkingDir is the agent's working directory.
	WorkingDir string
}

// Record runs an agent with full tracing and saves the execution trace.
func Record(ctx context.Context, opts RecordOptions) error {
	if len(opts.Command) == 0 {
		return fmt.Errorf("no command specified")
	}

	if opts.WorkingDir == "" {
		opts.WorkingDir, _ = os.Getwd()
	}

	if opts.OutputPath == "" {
		opts.OutputPath = fmt.Sprintf("trace-%s.json", time.Now().Format("20060102-150405"))
	}

	lang := DetectLanguage(opts.Command)

	// If language not detected from command, inspect the binary
	var runtimeInfo *RuntimeInfo
	if lang == LangUnknown && len(opts.Command) > 0 {
		ri, err := DetectRuntime(opts.Command[0])
		if err == nil && ri.Runtime != LangUnknown {
			lang = ri.Runtime
			runtimeInfo = ri
			log.Info().
				Str("binary", ri.BinaryPath).
				Str("runtime", string(ri.Runtime)).
				Str("confidence", ri.Confidence).
				Msg("detected runtime from binary inspection")
		}
	}

	// Detect the agent's source directory for function tracing
	agentDir := detectAgentDir(opts.Command, opts.WorkingDir)
	log.Info().
		Str("language", string(lang)).
		Strs("command", opts.Command).
		Str("output", opts.OutputPath).
		Msg("starting recording")

	// Start the proxy
	p, err := proxy.New(proxy.Options{
		Mode:    proxy.ModeRecord,
		Port:    0,
		Targets: opts.Targets,
	})
	if err != nil {
		return fmt.Errorf("start proxy: %w", err)
	}
	defer p.Close()

	go p.Serve()

	// Set up language-specific tracer
	traceOutputPath := filepath.Join(os.TempDir(), fmt.Sprintf("aperio-func-trace-%s.json", uuid.New().String()))
	var t tracer.Tracer
	isBinaryAgent := runtimeInfo != nil

	switch lang {
	case LangPython:
		t = tracer.NewPythonTracer()
	case LangNode:
		nt := tracer.NewNodeTracer()
		if isBinaryAgent {
			nt.SetBinaryMode(true)
		}
		t = nt
	case LangGo:
		t = tracer.NewGoTracer()
	default:
		log.Warn().Str("language", string(lang)).Msg("no tracer available for language, recording API calls only")
	}

	tracerCfg := tracer.Config{
		TraceOutputPath: traceOutputPath,
		WorkingDir:      agentDir,
		ExcludePatterns: opts.ExcludePatterns,
	}

	args := opts.Command
	var tracerEnv []string

	if t != nil {
		var err error
		tracerEnv, args, err = t.Setup(ctx, tracerCfg, opts.Command)
		if err != nil {
			return fmt.Errorf("setup tracer: %w", err)
		}
		defer t.Cleanup()
	}

	// Build environment with proxy settings
	env := os.Environ()
	proxyURL := fmt.Sprintf("http://127.0.0.1:%d", p.Port())
	env = append(env,
		fmt.Sprintf("HTTP_PROXY=%s", proxyURL),
		fmt.Sprintf("HTTPS_PROXY=%s", proxyURL),
		fmt.Sprintf("http_proxy=%s", proxyURL),
		fmt.Sprintf("https_proxy=%s", proxyURL),
		// Also set provider-specific base URLs as fallback
		fmt.Sprintf("OPENAI_BASE_URL=http://127.0.0.1:%d/v1", p.Port()),
	)
	env = append(env, tracerEnv...)

	// For HTTPS MITM, disable TLS verification in common SDKs
	env = append(env,
		"NODE_TLS_REJECT_UNAUTHORIZED=0",
		"PYTHONHTTPSVERIFY=0",
	)

	// Run the agent
	log.Info().Strs("args", args).Msg("launching agent")
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = env
	cmd.Dir = opts.WorkingDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	agentErr := cmd.Run()

	// Collect traces
	apiSpans := p.Spans()
	log.Info().Int("api_spans", len(apiSpans)).Msg("collected API spans")

	var funcSpans []*trace.Span
	if t != nil {
		funcSpans, err = t.Collect(tracerCfg)
		if err != nil {
			log.Warn().Err(err).Msg("failed to collect function traces")
		} else {
			log.Info().Int("func_spans", len(funcSpans)).Msg("collected function spans")
		}
	}

	// Merge and enrich
	allSpans := trace.Merge(apiSpans, funcSpans)

	result := &trace.Trace{
		ID:        uuid.New().String(),
		CreatedAt: time.Now(),
		Metadata: trace.Metadata{
			Command:    strings.Join(opts.Command, " "),
			Language:   string(lang),
			WorkingDir: opts.WorkingDir,
		},
		Spans: allSpans,
	}

	trace.Enrich(result)
	trace.ClassifyAndAnnotate(result)

	// Write trace
	if err := trace.WriteTrace(opts.OutputPath, result); err != nil {
		return fmt.Errorf("write trace: %w", err)
	}

	log.Info().
		Str("output", opts.OutputPath).
		Int("total_spans", len(result.Spans)).
		Msg("trace saved")

	if agentErr != nil {
		return fmt.Errorf("agent exited with error: %w", agentErr)
	}

	return nil
}

// Replay runs an agent with mocked LLM responses from a recorded trace.
func Replay(ctx context.Context, opts ReplayOptions) error {
	if len(opts.Command) == 0 {
		return fmt.Errorf("no command specified")
	}

	if opts.WorkingDir == "" {
		opts.WorkingDir, _ = os.Getwd()
	}

	if opts.OutputPath == "" {
		opts.OutputPath = fmt.Sprintf("replay-%s.json", time.Now().Format("20060102-150405"))
	}

	// Load recorded trace
	recorded, err := trace.ReadTrace(opts.InputPath)
	if err != nil {
		return fmt.Errorf("read recorded trace: %w", err)
	}

	log.Info().
		Str("input", opts.InputPath).
		Int("recorded_spans", len(recorded.Spans)).
		Msg("loaded recorded trace")

	lang := DetectLanguage(opts.Command)

	// If language not detected from command, inspect the binary
	var replayRuntimeInfo *RuntimeInfo
	if lang == LangUnknown && len(opts.Command) > 0 {
		ri, err := DetectRuntime(opts.Command[0])
		if err == nil && ri.Runtime != LangUnknown {
			lang = ri.Runtime
			replayRuntimeInfo = ri
		}
	}

	// Start replay proxy
	p, err := proxy.New(proxy.Options{
		Mode:          proxy.ModeReplay,
		Port:          0,
		RecordedTrace: recorded,
		Strict:        opts.Strict,
		MatchStrategy: opts.MatchStrategy,
	})
	if err != nil {
		return fmt.Errorf("start replay proxy: %w", err)
	}
	defer p.Close()

	go p.Serve()

	// Set up tracer for the replay run too
	traceOutputPath := filepath.Join(os.TempDir(), fmt.Sprintf("aperio-replay-trace-%s.json", uuid.New().String()))
	var t tracer.Tracer

	isBinaryReplay := replayRuntimeInfo != nil

	switch lang {
	case LangPython:
		t = tracer.NewPythonTracer()
	case LangNode:
		nt := tracer.NewNodeTracer()
		if isBinaryReplay {
			nt.SetBinaryMode(true)
		}
		t = nt
	case LangGo:
		t = tracer.NewGoTracer()
	default:
		log.Warn().Msg("no tracer available for language")
	}

	tracerCfg := tracer.Config{
		TraceOutputPath: traceOutputPath,
		WorkingDir:      opts.WorkingDir,
	}

	args := opts.Command
	var tracerEnv []string

	if t != nil {
		tracerEnv, args, err = t.Setup(ctx, tracerCfg, opts.Command)
		if err != nil {
			return fmt.Errorf("setup tracer: %w", err)
		}
		defer t.Cleanup()
	}

	// Build environment
	env := os.Environ()
	proxyURL := fmt.Sprintf("http://127.0.0.1:%d", p.Port())
	env = append(env,
		fmt.Sprintf("HTTP_PROXY=%s", proxyURL),
		fmt.Sprintf("HTTPS_PROXY=%s", proxyURL),
		fmt.Sprintf("http_proxy=%s", proxyURL),
		fmt.Sprintf("https_proxy=%s", proxyURL),
		"NODE_TLS_REJECT_UNAUTHORIZED=0",
		"PYTHONHTTPSVERIFY=0",
	)
	env = append(env, tracerEnv...)

	// Run the agent
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = env
	cmd.Dir = opts.WorkingDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	agentErr := cmd.Run()

	// Collect replay traces
	var funcSpans []*trace.Span
	if t != nil {
		funcSpans, _ = t.Collect(tracerCfg)
	}

	allSpans := trace.Merge(p.Spans(), funcSpans)

	result := &trace.Trace{
		ID:        uuid.New().String(),
		CreatedAt: time.Now(),
		Metadata: trace.Metadata{
			Command:    strings.Join(opts.Command, " "),
			Language:   string(lang),
			WorkingDir: opts.WorkingDir,
		},
		Spans: allSpans,
	}

	trace.Enrich(result)
	trace.ClassifyAndAnnotate(result)

	if err := trace.WriteTrace(opts.OutputPath, result); err != nil {
		return fmt.Errorf("write replay trace: %w", err)
	}

	log.Info().
		Str("output", opts.OutputPath).
		Int("total_spans", len(result.Spans)).
		Msg("replay trace saved")

	if agentErr != nil {
		return fmt.Errorf("agent exited with error: %w", agentErr)
	}

	return nil
}
