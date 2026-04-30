package viewer

import (
	"embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/qiangli/aperio/internal/trace"
)

//go:embed templates/index.html
var viewerHTML embed.FS

//go:embed templates/diff.html
var diffHTML embed.FS

// Options configures the viewer server.
type Options struct {
	Port      int
	TraceFile string
}

// Serve starts the viewer HTTP server and returns the URL.
func Serve(opts Options) (string, error) {
	// Read the trace
	t, err := trace.ReadTrace(opts.TraceFile)
	if err != nil {
		return "", fmt.Errorf("read trace: %w", err)
	}

	// Serialize trace to JSON
	traceJSON, err := json.Marshal(t)
	if err != nil {
		return "", fmt.Errorf("marshal trace: %w", err)
	}

	// Read HTML template
	htmlBytes, err := viewerHTML.ReadFile("templates/index.html")
	if err != nil {
		return "", fmt.Errorf("read template: %w", err)
	}

	// Inject trace data
	html := strings.Replace(
		string(htmlBytes),
		"// APERIO_TRACE_DATA_PLACEHOLDER\n  var TRACE_DATA = {};",
		fmt.Sprintf("var TRACE_DATA = %s;", string(traceJSON)),
		1,
	)

	// Create HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(html))
	})

	// Listen
	addr := fmt.Sprintf(":%d", opts.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return "", fmt.Errorf("listen: %w", err)
	}

	url := fmt.Sprintf("http://localhost:%d", listener.Addr().(*net.TCPAddr).Port)
	log.Info().Str("url", url).Msg("viewer running")

	// Serve in background
	go http.Serve(listener, mux)

	return url, nil
}

// DiffOptions configures the diff viewer.
type DiffOptions struct {
	Port       int
	LeftFile   string
	RightFile  string
	DiffResult []trace.Difference
}

// ServeDiff starts a side-by-side trace diff viewer.
func ServeDiff(opts DiffOptions) (string, error) {
	left, err := trace.ReadTrace(opts.LeftFile)
	if err != nil {
		return "", fmt.Errorf("read left trace: %w", err)
	}
	right, err := trace.ReadTrace(opts.RightFile)
	if err != nil {
		return "", fmt.Errorf("read right trace: %w", err)
	}

	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	diffJSON, _ := json.Marshal(opts.DiffResult)

	htmlBytes, err := diffHTML.ReadFile("templates/diff.html")
	if err != nil {
		return "", fmt.Errorf("read diff template: %w", err)
	}

	html := string(htmlBytes)
	html = strings.Replace(html, "\"__LEFT_TRACE__\"", string(leftJSON), 1)
	html = strings.Replace(html, "\"__RIGHT_TRACE__\"", string(rightJSON), 1)
	html = strings.Replace(html, "\"__DIFF_DATA__\"", string(diffJSON), 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(html))
	})

	addr := fmt.Sprintf(":%d", opts.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return "", fmt.Errorf("listen: %w", err)
	}

	url := fmt.Sprintf("http://localhost:%d", listener.Addr().(*net.TCPAddr).Port)
	log.Info().Str("url", url).Msg("diff viewer running")

	go http.Serve(listener, mux)

	return url, nil
}
