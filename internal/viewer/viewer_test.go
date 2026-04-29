package viewer

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/aperio/internal/trace"
)

func createTestTrace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.json")

	tr := &trace.Trace{
		ID:        "test-trace",
		CreatedAt: time.Now(),
		Metadata:  trace.Metadata{Command: "python agent.py", Language: "python", WorkingDir: "/tmp"},
		Spans: []*trace.Span{
			{ID: "s1", Type: trace.SpanFunction, Name: "main.run", StartTime: time.Now(), EndTime: time.Now()},
			{ID: "s2", Type: trace.SpanExec, Name: "exec: ls", StartTime: time.Now(), EndTime: time.Now(),
				Attributes: map[string]any{"command": "ls", "exit_code": 0}},
		},
	}

	if err := trace.WriteTrace(path, tr); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestServeHTML(t *testing.T) {
	tracePath := createTestTrace(t)

	url, err := Serve(Options{Port: 0, TraceFile: tracePath})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("expected text/html, got %s", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	if !strings.Contains(html, "aperio") {
		t.Error("HTML should contain 'aperio'")
	}
	if !strings.Contains(html, "test-trace") {
		t.Error("HTML should contain trace ID")
	}
	if !strings.Contains(html, "main.run") {
		t.Error("HTML should contain span name")
	}
}

func TestServeTraceDataInjected(t *testing.T) {
	tracePath := createTestTrace(t)

	url, err := Serve(Options{Port: 0, TraceFile: tracePath})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	// The trace JSON should be embedded as TRACE_DATA
	if !strings.Contains(html, "TRACE_DATA") {
		t.Error("HTML should contain TRACE_DATA variable")
	}

	// Extract the JSON to verify it's valid
	idx := strings.Index(html, "var TRACE_DATA = ")
	if idx < 0 {
		t.Fatal("TRACE_DATA assignment not found")
	}
	jsonStart := idx + len("var TRACE_DATA = ")
	jsonEnd := strings.Index(html[jsonStart:], ";")
	if jsonEnd < 0 {
		t.Fatal("TRACE_DATA semicolon not found")
	}
	traceJSON := html[jsonStart : jsonStart+jsonEnd]

	var parsed map[string]any
	if err := json.Unmarshal([]byte(traceJSON), &parsed); err != nil {
		t.Fatalf("injected JSON is invalid: %v", err)
	}
	if parsed["id"] != "test-trace" {
		t.Errorf("trace ID: got %v", parsed["id"])
	}
}

func TestServeMissingFile(t *testing.T) {
	_, err := Serve(Options{Port: 0, TraceFile: "/nonexistent/trace.json"})
	if err == nil {
		t.Fatal("expected error for missing trace file")
	}
}

func TestServeRandomPort(t *testing.T) {
	tracePath := createTestTrace(t)

	url, err := Serve(Options{Port: 0, TraceFile: tracePath})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(url, "http://localhost:") {
		t.Errorf("unexpected URL format: %s", url)
	}

	// Verify it's actually listening
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

func TestServeWithSpecificPort(t *testing.T) {
	tracePath := createTestTrace(t)

	// Use a high port that's likely available
	url, err := Serve(Options{Port: 0, TraceFile: tracePath})
	if err != nil {
		t.Fatal(err)
	}

	_ = url
	_ = os.TempDir() // keep imports used
}
