package tracer

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/qiangli/aperio/internal/trace"
)

// GoTracer instruments Go agents via dlv trace subprocess.
type GoTracer struct {
	dlvOutput string // path to dlv trace output file
}

func NewGoTracer() *GoTracer {
	return &GoTracer{}
}

func (gt *GoTracer) Setup(ctx context.Context, cfg Config, args []string) ([]string, []string, error) {
	// Check if dlv is available
	if _, err := exec.LookPath("dlv"); err != nil {
		log.Warn().Msg("dlv (Delve) not found — Go function tracing disabled. Install: go install github.com/go-delve/delve/cmd/dlv@latest")
		return nil, args, nil
	}

	gt.dlvOutput = cfg.TraceOutputPath

	// Build dlv trace command
	// dlv trace [package] [flags] -- [pass-to-program]
	// We trace functions matching a pattern in the working directory
	pattern := buildTracePattern(cfg)

	// Construct: dlv trace --output /dev/null <package-or-binary> <pattern> -- <program-args>
	newArgs := []string{"dlv", "trace"}

	if isGoRunCommand(args) {
		// `go run ./cmd/agent arg1 arg2` → `dlv trace ./cmd/agent <pattern> -- arg1 arg2`
		pkg, progArgs := splitGoRunArgs(args)
		newArgs = append(newArgs, pkg, pattern)
		if len(progArgs) > 0 {
			newArgs = append(newArgs, "--")
			newArgs = append(newArgs, progArgs...)
		}
	} else {
		// Direct binary: `./agent arg1` → `dlv trace ./agent <pattern> -- arg1`
		binary := args[0]
		newArgs = append(newArgs, binary, pattern)
		if len(args) > 1 {
			newArgs = append(newArgs, "--")
			newArgs = append(newArgs, args[1:]...)
		}
	}

	return nil, newArgs, nil
}

func (gt *GoTracer) Collect(cfg Config) ([]*trace.Span, error) {
	data, err := os.ReadFile(cfg.TraceOutputPath)
	if err != nil {
		return nil, fmt.Errorf("read go trace output: %w", err)
	}

	return parseDlvOutput(data)
}

func (gt *GoTracer) Cleanup() error {
	return nil
}

// buildTracePattern creates a regex pattern for dlv trace.
func buildTracePattern(cfg Config) string {
	// Default: trace all functions in the main package and any package
	// under the working directory
	if len(cfg.IncludePatterns) > 0 {
		return strings.Join(cfg.IncludePatterns, "|")
	}

	// Trace common patterns for agent code
	return "main\\..+"
}

// isGoRunCommand checks if args represent a `go run` command.
func isGoRunCommand(args []string) bool {
	return len(args) >= 3 && args[0] == "go" && args[1] == "run"
}

// splitGoRunArgs splits `go run ./pkg arg1 arg2` into package and program args.
func splitGoRunArgs(args []string) (string, []string) {
	// args = ["go", "run", "./cmd/agent", "arg1", "arg2"]
	pkg := args[2]
	var progArgs []string
	if len(args) > 3 {
		progArgs = args[3:]
	}
	return pkg, progArgs
}

// dlvTraceEntry represents a parsed line from dlv trace output.
var dlvTraceCallRegex = regexp.MustCompile(`^> goroutine\((\d+)\): (.+)\((.*)?\)`)
var dlvTraceReturnRegex = regexp.MustCompile(`^=> (.*)$`)

// parseDlvOutput parses dlv trace text output into spans.
// dlv trace output format:
//
//	> goroutine(1): main.processQuery(query = "hello")
//	=> "result"
func parseDlvOutput(data []byte) ([]*trace.Span, error) {
	var spans []*trace.Span
	scanner := bufio.NewScanner(strings.NewReader(string(data)))

	var currentSpan *trace.Span
	now := time.Now()
	counter := 0

	for scanner.Scan() {
		line := scanner.Text()

		if matches := dlvTraceCallRegex.FindStringSubmatch(line); matches != nil {
			counter++
			goroutine := matches[1]
			funcName := matches[2]
			argsStr := matches[3]

			currentSpan = &trace.Span{
				ID:        fmt.Sprintf("go-%d", counter),
				Type:      trace.SpanFunction,
				Name:      funcName,
				StartTime: now.Add(time.Duration(counter) * time.Microsecond),
				Attributes: map[string]any{
					"goroutine": goroutine,
					"function":  funcName,
					"args_raw":  argsStr,
				},
			}
			spans = append(spans, currentSpan)

		} else if matches := dlvTraceReturnRegex.FindStringSubmatch(line); matches != nil {
			if currentSpan != nil {
				currentSpan.EndTime = now.Add(time.Duration(counter)*time.Microsecond + time.Microsecond)
				retVal := matches[1]
				if len(retVal) > 200 {
					retVal = retVal[:200] + "..."
				}
				currentSpan.Attributes["return_value"] = retVal
				currentSpan = nil
			}
		}
	}

	return spans, nil
}

