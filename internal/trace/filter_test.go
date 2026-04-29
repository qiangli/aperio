package trace

import (
	"testing"
)

func TestFilterEmptyConfig(t *testing.T) {
	spans := []*Span{
		{ID: "1", Type: SpanFunction, Name: "main.run"},
		{ID: "2", Type: SpanExec, Name: "exec: ls"},
		{ID: "3", Type: SpanLLMRequest, Name: "POST /v1/chat"},
	}
	result := Filter(spans, FilterConfig{})
	if len(result) != 3 {
		t.Fatalf("expected 3 spans unchanged, got %d", len(result))
	}
}

func TestFilterNonFunctionSpansAlwaysPassThrough(t *testing.T) {
	spans := []*Span{
		{ID: "1", Type: SpanExec, Name: "exec: ls"},
		{ID: "2", Type: SpanFSRead, Name: "read: config.txt"},
		{ID: "3", Type: SpanFSWrite, Name: "write: out.txt"},
		{ID: "4", Type: SpanNetIO, Name: "connect: api.com:443"},
		{ID: "5", Type: SpanDBQuery, Name: "sqlite: SELECT *"},
		{ID: "6", Type: SpanLLMRequest, Name: "POST /v1/chat"},
		{ID: "7", Type: SpanToolCall, Name: "tool_call: get_weather"},
		{ID: "8", Type: SpanUserInput, Name: "user message"},
		{ID: "9", Type: SpanAgentOutput, Name: "assistant response"},
	}

	// Even with restrictive filter, non-FUNCTION spans pass through
	result := Filter(spans, FilterConfig{
		ExcludePatterns: []string{"everything"},
		MaxDepth:        1,
	})

	if len(result) != 9 {
		t.Fatalf("expected all 9 non-FUNCTION spans to pass, got %d", len(result))
	}
}

func TestFilterIncludeDirs(t *testing.T) {
	spans := []*Span{
		{ID: "1", Type: SpanFunction, Name: "agent.run", Attributes: map[string]any{"filename": "/project/agent.py"}},
		{ID: "2", Type: SpanFunction, Name: "lib.helper", Attributes: map[string]any{"filename": "/other/lib.py"}},
		{ID: "3", Type: SpanFunction, Name: "no-filename", Attributes: map[string]any{}},
	}

	result := Filter(spans, FilterConfig{
		IncludeDirs: []string{"/project"},
	})

	// Span 1: filename under /project → kept
	// Span 2: filename under /other → excluded
	// Span 3: empty filename → kept (empty filename skips the dir check)
	funcSpans := 0
	for _, s := range result {
		if s.Type == SpanFunction {
			funcSpans++
		}
	}
	if funcSpans != 2 {
		t.Fatalf("expected 2 function spans (under /project + no filename), got %d", funcSpans)
	}
}

func TestFilterExcludePatterns(t *testing.T) {
	spans := []*Span{
		{ID: "1", Type: SpanFunction, Name: "main.run", Attributes: map[string]any{"filename": "/project/main.py"}},
		{ID: "2", Type: SpanFunction, Name: "venv.lib.something", Attributes: map[string]any{"filename": "/project/venv/lib.py"}},
		{ID: "3", Type: SpanFunction, Name: "agent.process", Attributes: map[string]any{"filename": "/project/agent.py"}},
	}

	result := Filter(spans, FilterConfig{
		ExcludePatterns: []string{"venv"},
	})

	// Span 2 excluded (name and filename both contain "venv"), spans 1 and 3 remain
	funcCount := 0
	for _, s := range result {
		if s.Type == SpanFunction {
			funcCount++
		}
	}
	if funcCount != 2 {
		t.Fatalf("expected 2 function spans (main.run + agent.process), got %d", funcCount)
	}
}

func TestFilterExcludeByName(t *testing.T) {
	spans := []*Span{
		{ID: "1", Type: SpanFunction, Name: "main.run", Attributes: map[string]any{}},
		{ID: "2", Type: SpanFunction, Name: "internal._helper", Attributes: map[string]any{}},
	}

	result := Filter(spans, FilterConfig{
		ExcludePatterns: []string{"internal."},
	})

	if len(result) != 1 || result[0].ID != "1" {
		t.Fatalf("expected only main.run, got %v", result)
	}
}

func TestFilterMaxDepth(t *testing.T) {
	spans := []*Span{
		{ID: "1", Type: SpanFunction, Name: "depth1"},
		{ID: "2", Type: SpanFunction, Name: "depth2", ParentID: "1"},
		{ID: "3", Type: SpanFunction, Name: "depth3", ParentID: "2"},
		{ID: "4", Type: SpanFunction, Name: "depth4", ParentID: "3"},
	}

	tests := []struct {
		maxDepth int
		want     int
	}{
		{0, 4}, // unlimited
		{1, 1},
		{2, 2},
		{3, 3},
		{4, 4},
		{10, 4},
	}

	for _, tt := range tests {
		result := Filter(spans, FilterConfig{MaxDepth: tt.maxDepth})
		funcCount := 0
		for _, s := range result {
			if s.Type == SpanFunction {
				funcCount++
			}
		}
		if funcCount != tt.want {
			t.Errorf("MaxDepth=%d: expected %d function spans, got %d", tt.maxDepth, tt.want, funcCount)
		}
	}
}

func TestFilterCombined(t *testing.T) {
	spans := []*Span{
		{ID: "1", Type: SpanFunction, Name: "main.run", Attributes: map[string]any{"filename": "/project/main.py"}},
		{ID: "2", Type: SpanFunction, Name: "main.deep", ParentID: "1", Attributes: map[string]any{"filename": "/project/main.py"}},
		{ID: "3", Type: SpanFunction, Name: "main.deeper", ParentID: "2", Attributes: map[string]any{"filename": "/project/main.py"}},
		{ID: "4", Type: SpanFunction, Name: "venv.lib", Attributes: map[string]any{"filename": "/project/venv/lib.py"}},
		{ID: "5", Type: SpanExec, Name: "exec: ls"},
	}

	result := Filter(spans, FilterConfig{
		IncludeDirs:     []string{"/project"},
		ExcludePatterns: []string{"venv"},
		MaxDepth:        2,
	})

	// Expect: span 1 (depth 1), span 2 (depth 2), span 5 (non-function)
	// Span 3 exceeds depth 2, span 4 excluded by pattern
	funcCount := 0
	for _, s := range result {
		if s.Type == SpanFunction {
			funcCount++
		}
	}
	if funcCount != 2 {
		t.Errorf("expected 2 function spans, got %d", funcCount)
	}
	// Non-function span always passes
	hasExec := false
	for _, s := range result {
		if s.Type == SpanExec {
			hasExec = true
		}
	}
	if !hasExec {
		t.Error("EXEC span should pass through filter")
	}
}

func TestFilterNilAttributes(t *testing.T) {
	spans := []*Span{
		{ID: "1", Type: SpanFunction, Name: "main.run", Attributes: nil},
	}
	// Should not panic
	result := Filter(spans, FilterConfig{
		IncludeDirs: []string{"/project"},
	})
	// nil attributes means empty filename, which skips the matchesAnyDir check → passes through
	funcCount := 0
	for _, s := range result {
		if s.Type == SpanFunction {
			funcCount++
		}
	}
	if funcCount != 1 {
		t.Errorf("expected 1 function span (nil attrs passes dir check), got %d", funcCount)
	}
}

func TestFilterMissingParentForDepth(t *testing.T) {
	// Parent not in depths map — should default to depth 1
	spans := []*Span{
		{ID: "2", Type: SpanFunction, Name: "orphan", ParentID: "nonexistent"},
	}
	result := Filter(spans, FilterConfig{MaxDepth: 1})
	// orphan has parent not in map, defaults to depth 1+0=1, which <= maxDepth 1
	funcCount := 0
	for _, s := range result {
		if s.Type == SpanFunction {
			funcCount++
		}
	}
	if funcCount != 1 {
		t.Errorf("expected 1 (orphan at depth 1), got %d", funcCount)
	}
}

func TestMatchesAnyPattern(t *testing.T) {
	if matchesAnyPattern("hello world", []string{"xyz"}) {
		t.Error("should not match")
	}
	if !matchesAnyPattern("hello world", []string{"world"}) {
		t.Error("should match")
	}
	if matchesAnyPattern("hello", nil) {
		t.Error("nil patterns should not match")
	}
	if matchesAnyPattern("hello", []string{}) {
		t.Error("empty patterns should not match")
	}
}
