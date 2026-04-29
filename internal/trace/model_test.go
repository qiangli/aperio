package trace

import (
	"testing"
	"time"
)

func TestAddSpan(t *testing.T) {
	tr := &Trace{
		ID:        "test-trace",
		CreatedAt: time.Now(),
	}

	span := &Span{
		ID:        "span-1",
		Type:      SpanFunction,
		Name:      "main.run",
		StartTime: time.Now(),
		EndTime:   time.Now(),
	}

	tr.AddSpan(span)

	if len(tr.Spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(tr.Spans))
	}
	if tr.Spans[0].ID != "span-1" {
		t.Fatalf("expected span ID span-1, got %s", tr.Spans[0].ID)
	}
}

func TestSpansByType(t *testing.T) {
	tr := &Trace{
		ID:        "test-trace",
		CreatedAt: time.Now(),
		Spans: []*Span{
			{ID: "1", Type: SpanFunction, Name: "main.run"},
			{ID: "2", Type: SpanLLMRequest, Name: "POST /v1/chat/completions"},
			{ID: "3", Type: SpanLLMResponse, Name: "200 OK"},
			{ID: "4", Type: SpanFunction, Name: "tools.execute"},
			{ID: "5", Type: SpanToolCall, Name: "get_weather"},
		},
	}

	funcs := tr.SpansByType(SpanFunction)
	if len(funcs) != 2 {
		t.Fatalf("expected 2 function spans, got %d", len(funcs))
	}

	tools := tr.SpansByType(SpanToolCall)
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool call span, got %d", len(tools))
	}

	empty := tr.SpansByType(SpanAgentOutput)
	if len(empty) != 0 {
		t.Fatalf("expected 0 agent output spans, got %d", len(empty))
	}
}
