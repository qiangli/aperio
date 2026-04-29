package trace

import (
	"testing"
	"time"
)

func TestMerge(t *testing.T) {
	now := time.Now()

	funcSpans := []*Span{
		{
			ID:        "func-1",
			Type:      SpanFunction,
			Name:      "main.run",
			StartTime: now,
			EndTime:   now.Add(5 * time.Second),
		},
		{
			ID:        "func-2",
			ParentID:  "func-1",
			Type:      SpanFunction,
			Name:      "agent.callLLM",
			StartTime: now.Add(time.Second),
			EndTime:   now.Add(3 * time.Second),
		},
	}

	apiSpans := []*Span{
		{
			ID:        "api-1",
			Type:      SpanLLMRequest,
			Name:      "POST /v1/chat/completions",
			StartTime: now.Add(time.Second + 100*time.Millisecond),
			EndTime:   now.Add(2 * time.Second),
		},
	}

	merged := Merge(apiSpans, funcSpans)

	if len(merged) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(merged))
	}

	// API span should have been parented to the innermost containing function span
	for _, s := range merged {
		if s.ID == "api-1" {
			if s.ParentID != "func-2" {
				t.Errorf("api span parent: got %s, want func-2", s.ParentID)
			}
		}
	}
}

func TestMergeEmptyFuncSpans(t *testing.T) {
	apiSpans := []*Span{{ID: "a1", Type: SpanLLMRequest}}
	result := Merge(apiSpans, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 span, got %d", len(result))
	}
}

func TestMergeEmptyAPISpans(t *testing.T) {
	funcSpans := []*Span{{ID: "f1", Type: SpanFunction}}
	result := Merge(nil, funcSpans)
	if len(result) != 1 {
		t.Fatalf("expected 1 span, got %d", len(result))
	}
}

func TestMergeBothNil(t *testing.T) {
	result := Merge(nil, nil)
	if len(result) != 0 {
		t.Fatalf("expected 0 spans, got %d", len(result))
	}
}

func TestMergeAPISpanWithExistingParent(t *testing.T) {
	now := time.Now()
	funcSpans := []*Span{
		{ID: "func-1", Type: SpanFunction, StartTime: now, EndTime: now.Add(5 * time.Second)},
	}
	apiSpans := []*Span{
		{ID: "api-1", ParentID: "existing-parent", Type: SpanLLMRequest, StartTime: now.Add(time.Second), EndTime: now.Add(2 * time.Second)},
	}

	merged := Merge(apiSpans, funcSpans)
	for _, s := range merged {
		if s.ID == "api-1" {
			if s.ParentID != "existing-parent" {
				t.Errorf("should not overwrite existing parent, got %s", s.ParentID)
			}
		}
	}
}

func TestMergeDeeplyNested(t *testing.T) {
	now := time.Now()
	funcSpans := []*Span{
		{ID: "f1", Type: SpanFunction, StartTime: now, EndTime: now.Add(10 * time.Second)},
		{ID: "f2", Type: SpanFunction, ParentID: "f1", StartTime: now.Add(time.Second), EndTime: now.Add(9 * time.Second)},
		{ID: "f3", Type: SpanFunction, ParentID: "f2", StartTime: now.Add(2 * time.Second), EndTime: now.Add(8 * time.Second)},
		{ID: "f4", Type: SpanFunction, ParentID: "f3", StartTime: now.Add(3 * time.Second), EndTime: now.Add(7 * time.Second)},
		{ID: "f5", Type: SpanFunction, ParentID: "f4", StartTime: now.Add(4 * time.Second), EndTime: now.Add(6 * time.Second)},
	}
	apiSpans := []*Span{
		{ID: "api-1", Type: SpanLLMRequest, StartTime: now.Add(4500 * time.Millisecond), EndTime: now.Add(5500 * time.Millisecond)},
	}

	merged := Merge(apiSpans, funcSpans)
	for _, s := range merged {
		if s.ID == "api-1" {
			// Should be parented to f5 (innermost containing span)
			if s.ParentID != "f5" {
				t.Errorf("expected parent f5, got %s", s.ParentID)
			}
		}
	}
}

func TestMergeZeroDurationSpans(t *testing.T) {
	now := time.Now()
	funcSpans := []*Span{
		{ID: "f1", Type: SpanFunction, StartTime: now, EndTime: now},
	}
	apiSpans := []*Span{
		{ID: "a1", Type: SpanLLMRequest, StartTime: now, EndTime: now},
	}

	// Should not panic
	merged := Merge(apiSpans, funcSpans)
	if len(merged) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(merged))
	}
}

func TestMergeSortsByStartTime(t *testing.T) {
	now := time.Now()
	funcSpans := []*Span{
		{ID: "f1", Type: SpanFunction, StartTime: now.Add(2 * time.Second), EndTime: now.Add(3 * time.Second)},
	}
	apiSpans := []*Span{
		{ID: "a1", Type: SpanLLMRequest, StartTime: now, EndTime: now.Add(time.Second)},
	}

	merged := Merge(apiSpans, funcSpans)
	if merged[0].ID != "a1" {
		t.Errorf("expected a1 first (earlier start time), got %s", merged[0].ID)
	}
}
