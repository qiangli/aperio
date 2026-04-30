package trace

import (
	"testing"
	"time"
)

func TestMergeTracesEmpty(t *testing.T) {
	_, err := MergeTraces(nil)
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestMergeTracesSingle(t *testing.T) {
	tr := &Trace{ID: "single", Metadata: Metadata{Language: "python"}}
	result, err := MergeTraces([]*Trace{tr})
	if err != nil {
		t.Fatalf("MergeTraces: %v", err)
	}
	if result.ID != "single" {
		t.Error("single trace should be returned as-is")
	}
}

func TestMergeTracesTwo(t *testing.T) {
	now := time.Now()
	corrID := "corr-123"

	t1 := &Trace{
		ID:        "trace-a",
		CreatedAt: now,
		Metadata: Metadata{
			Command:       "python orchestrator.py",
			Language:      "python",
			CorrelationID: corrID,
			AgentRole:     "orchestrator",
		},
		Spans: []*Span{
			{ID: "proc-a", Type: SpanProcess, Name: "orchestrator", StartTime: now, EndTime: now.Add(3 * time.Second)},
			{ID: "f1", ParentID: "proc-a", Type: SpanFunction, Name: "main", StartTime: now, EndTime: now.Add(3 * time.Second)},
			{ID: "l1", ParentID: "f1", Type: SpanLLMRequest, Name: "api", StartTime: now.Add(time.Second), EndTime: now.Add(2 * time.Second)},
		},
	}

	t2 := &Trace{
		ID:        "trace-b",
		CreatedAt: now.Add(time.Second),
		Metadata: Metadata{
			Command:       "python coder.py",
			Language:      "python",
			CorrelationID: corrID,
			AgentRole:     "coder",
			ParentTraceID: "trace-a",
			ParentSpanID:  "proc-a",
		},
		Spans: []*Span{
			{ID: "proc-b", Type: SpanProcess, Name: "coder", StartTime: now.Add(time.Second), EndTime: now.Add(2 * time.Second)},
			{ID: "f2", ParentID: "proc-b", Type: SpanFunction, Name: "code", StartTime: now.Add(time.Second), EndTime: now.Add(2 * time.Second)},
		},
	}

	merged, err := MergeTraces([]*Trace{t1, t2})
	if err != nil {
		t.Fatalf("MergeTraces: %v", err)
	}

	if merged.Metadata.CorrelationID != corrID {
		t.Errorf("expected correlation ID %s, got %s", corrID, merged.Metadata.CorrelationID)
	}

	// Should have all spans from both traces (3 + 2 = 5)
	if len(merged.Spans) != 5 {
		t.Errorf("expected 5 spans, got %d", len(merged.Spans))
	}

	// trace-b's PROCESS span should be linked to trace-a's process span
	var coderProcess *Span
	for _, s := range merged.Spans {
		if s.Name == "coder" && s.Type == SpanProcess {
			coderProcess = s
			break
		}
	}
	if coderProcess == nil {
		t.Fatal("coder PROCESS span not found")
	}
	expectedParent := namespaceID("trace-a", "proc-a")
	if coderProcess.ParentID != expectedParent {
		t.Errorf("coder process parent: expected %s, got %s", expectedParent, coderProcess.ParentID)
	}
}

func TestMergeTracesMultiRootGetsSymtheticRoot(t *testing.T) {
	now := time.Now()

	t1 := &Trace{
		ID: "t1",
		Spans: []*Span{
			{ID: "p1", Type: SpanProcess, Name: "agent1", StartTime: now, EndTime: now.Add(time.Second)},
		},
	}
	t2 := &Trace{
		ID: "t2",
		Spans: []*Span{
			{ID: "p2", Type: SpanProcess, Name: "agent2", StartTime: now, EndTime: now.Add(time.Second)},
		},
	}

	merged, err := MergeTraces([]*Trace{t1, t2})
	if err != nil {
		t.Fatalf("MergeTraces: %v", err)
	}

	// Should have synthetic root + 2 process spans = 3
	if len(merged.Spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(merged.Spans))
	}

	// First span should be synthetic root
	if merged.Spans[0].Name != "multi-agent execution" {
		t.Errorf("expected synthetic root, got %s", merged.Spans[0].Name)
	}
	if merged.Spans[0].ParentID != "" {
		t.Error("synthetic root should have no parent")
	}

	// Both process spans should be children of synthetic root
	for _, s := range merged.Spans[1:] {
		if s.ParentID != merged.Spans[0].ID {
			t.Errorf("expected parent %s, got %s", merged.Spans[0].ID, s.ParentID)
		}
	}
}

func TestMergeTracesPreservesAttributes(t *testing.T) {
	now := time.Now()
	tr := &Trace{
		ID: "t1",
		Metadata: Metadata{AgentRole: "tester"},
		Spans: []*Span{
			{
				ID:        "s1",
				Type:      SpanFunction,
				Name:      "fn",
				StartTime: now,
				EndTime:   now.Add(time.Second),
				Attributes: map[string]any{
					"custom_key": "custom_value",
				},
			},
		},
	}

	merged, err := MergeTraces([]*Trace{tr})
	if err != nil {
		t.Fatalf("MergeTraces: %v", err)
	}

	// Single trace is returned as-is
	if merged.Spans[0].Attributes["custom_key"] != "custom_value" {
		t.Error("expected custom_key to be preserved")
	}
}

func TestMergeTracesNamespacing(t *testing.T) {
	now := time.Now()
	// Two traces with same span IDs — should not collide after merge
	t1 := &Trace{
		ID: "aaaa-bbbb",
		Spans: []*Span{
			{ID: "span1", Type: SpanFunction, Name: "fn1", StartTime: now, EndTime: now.Add(time.Second)},
		},
	}
	t2 := &Trace{
		ID: "cccc-dddd",
		Spans: []*Span{
			{ID: "span1", Type: SpanFunction, Name: "fn2", StartTime: now, EndTime: now.Add(time.Second)},
		},
	}

	merged, err := MergeTraces([]*Trace{t1, t2})
	if err != nil {
		t.Fatalf("MergeTraces: %v", err)
	}

	// Span IDs should be different after namespacing
	ids := make(map[string]bool)
	for _, s := range merged.Spans {
		if ids[s.ID] {
			t.Errorf("duplicate span ID: %s", s.ID)
		}
		ids[s.ID] = true
	}
}

func TestNamespaceID(t *testing.T) {
	tests := []struct {
		traceID, spanID, want string
	}{
		{"trace-abc", "span-1", "trace-ab:span-1"},
		{"short", "s1", "short:s1"},
		{"trace-id", "", ""},
	}
	for _, tt := range tests {
		got := namespaceID(tt.traceID, tt.spanID)
		if got != tt.want {
			t.Errorf("namespaceID(%q, %q) = %q, want %q", tt.traceID, tt.spanID, got, tt.want)
		}
	}
}

func TestFindCorrelationID(t *testing.T) {
	traces := []*Trace{
		{Metadata: Metadata{CorrelationID: "abc"}},
		{Metadata: Metadata{CorrelationID: "abc"}},
		{Metadata: Metadata{CorrelationID: "xyz"}},
	}
	got := findCorrelationID(traces)
	if got != "abc" {
		t.Errorf("expected abc (most common), got %s", got)
	}
}

func TestFindCorrelationIDEmpty(t *testing.T) {
	traces := []*Trace{
		{Metadata: Metadata{}},
	}
	got := findCorrelationID(traces)
	if got == "" {
		t.Error("expected auto-generated correlation ID")
	}
}
