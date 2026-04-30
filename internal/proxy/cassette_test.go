package proxy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qiangli/aperio/internal/trace"
)

func TestWriteReadCassetteRoundTrip(t *testing.T) {
	spans := []*trace.Span{
		{
			ID:   "s1",
			Type: trace.SpanLLMRequest,
			Name: "POST /v1/chat/completions",
			Attributes: map[string]any{
				"http.method":        "POST",
				"http.url":           "https://api.openai.com/v1/chat/completions",
				"http.status_code":   float64(200),
				"http.request_body":  `{"model":"gpt-4","messages":[]}`,
				"http.response_body": `{"id":"chatcmpl-abc","choices":[]}`,
			},
		},
		{
			ID:   "s2",
			Type: trace.SpanFunction,
			Name: "main",
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")

	if err := WriteCassette(path, spans); err != nil {
		t.Fatalf("WriteCassette: %v", err)
	}

	// Verify file exists
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("cassette file is empty")
	}

	// Read back
	entries, err := ReadCassette(path)
	if err != nil {
		t.Fatalf("ReadCassette: %v", err)
	}

	// Only LLM_REQUEST spans should be in cassette
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.method != "POST" {
		t.Errorf("method: expected POST, got %s", e.method)
	}
	if e.response.statusCode != 200 {
		t.Errorf("status: expected 200, got %d", e.response.statusCode)
	}
}

func TestSpansToCassette(t *testing.T) {
	spans := []*trace.Span{
		{
			ID:   "llm1",
			Type: trace.SpanLLMRequest,
			Attributes: map[string]any{
				"http.method":      "POST",
				"http.url":         "https://api.openai.com/v1/chat/completions",
				"http.status_code": float64(200),
			},
		},
		{
			ID:   "func1",
			Type: trace.SpanFunction,
		},
		{
			ID:   "llm2",
			Type: trace.SpanLLMRequest,
			Attributes: map[string]any{
				"http.method":      "POST",
				"http.url":         "https://api.openai.com/v1/chat/completions",
				"http.status_code": float64(200),
			},
		},
	}

	cassette := SpansToCassette(spans)
	if cassette.Version != 1 {
		t.Errorf("expected version 1, got %d", cassette.Version)
	}
	if len(cassette.Interactions) != 2 {
		t.Errorf("expected 2 interactions, got %d", len(cassette.Interactions))
	}
}

func TestReadCassetteMissing(t *testing.T) {
	_, err := ReadCassette("/nonexistent/file.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
