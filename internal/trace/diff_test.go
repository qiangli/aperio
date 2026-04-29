package trace

import (
	"strings"
	"testing"
)

func TestDiffIdenticalTraces(t *testing.T) {
	tr := &Trace{
		Metadata: Metadata{Language: "python"},
		Spans: []*Span{
			{ID: "1", Type: SpanLLMRequest, Name: "POST /v1/chat"},
			{ID: "2", Type: SpanToolCall, Name: "tool_call: get_weather", Attributes: map[string]any{"tool_name": "get_weather"}},
		},
	}
	diffs := Diff(tr, tr)
	if len(diffs) != 0 {
		t.Fatalf("expected 0 diffs for identical traces, got %d: %v", len(diffs), diffs)
	}
}

func TestDiffLanguageChanged(t *testing.T) {
	left := &Trace{Metadata: Metadata{Language: "python"}}
	right := &Trace{Metadata: Metadata{Language: "node"}}

	diffs := Diff(left, right)
	found := false
	for _, d := range diffs {
		if d.Field == "metadata.language" && d.Type == "changed" {
			found = true
		}
	}
	if !found {
		t.Error("expected language change diff")
	}
}

func TestDiffSpanCountDifferences(t *testing.T) {
	left := &Trace{Spans: []*Span{
		{ID: "1", Type: SpanLLMRequest, Name: "req1"},
		{ID: "2", Type: SpanLLMRequest, Name: "req2"},
	}}
	right := &Trace{Spans: []*Span{
		{ID: "1", Type: SpanLLMRequest, Name: "req1"},
	}}

	diffs := Diff(left, right)
	found := false
	for _, d := range diffs {
		if strings.Contains(d.Field, "span_count") {
			found = true
		}
	}
	if !found {
		t.Error("expected span count difference")
	}
}

func TestDiffLLMRequestSequence(t *testing.T) {
	left := &Trace{Spans: []*Span{
		{ID: "1", Type: SpanLLMRequest, Name: "POST /v1/chat"},
		{ID: "2", Type: SpanLLMRequest, Name: "POST /v1/chat"},
	}}
	right := &Trace{Spans: []*Span{
		{ID: "1", Type: SpanLLMRequest, Name: "POST /v1/chat"},
		{ID: "2", Type: SpanLLMRequest, Name: "POST /v1/messages"},
	}}

	diffs := Diff(left, right)
	found := false
	for _, d := range diffs {
		if d.Field == "name" && d.Type == "changed" {
			found = true
		}
	}
	if !found {
		t.Error("expected LLM request name change")
	}
}

func TestDiffExtraAndMissingLLM(t *testing.T) {
	left := &Trace{Spans: []*Span{
		{ID: "1", Type: SpanLLMRequest, Name: "req1"},
	}}
	right := &Trace{Spans: []*Span{
		{ID: "1", Type: SpanLLMRequest, Name: "req1"},
		{ID: "2", Type: SpanLLMRequest, Name: "req2"},
	}}

	diffs := Diff(left, right)
	hasExtra := false
	for _, d := range diffs {
		if d.Type == "extra" {
			hasExtra = true
		}
	}
	if !hasExtra {
		t.Error("expected extra LLM request diff")
	}

	// Reverse: left has more
	diffs2 := Diff(right, left)
	hasMissing := false
	for _, d := range diffs2 {
		if d.Type == "missing" {
			hasMissing = true
		}
	}
	if !hasMissing {
		t.Error("expected missing LLM request diff")
	}
}

func TestDiffToolCallSequence(t *testing.T) {
	left := &Trace{Spans: []*Span{
		{ID: "1", Type: SpanToolCall, Name: "tool_call: get_weather", Attributes: map[string]any{"tool_name": "get_weather"}},
	}}
	right := &Trace{Spans: []*Span{
		{ID: "1", Type: SpanToolCall, Name: "tool_call: get_time", Attributes: map[string]any{"tool_name": "get_time"}},
	}}

	diffs := Diff(left, right)
	found := false
	for _, d := range diffs {
		if d.Field == "tool_name" && d.Left == "get_weather" && d.Right == "get_time" {
			found = true
		}
	}
	if !found {
		t.Error("expected tool_name change diff")
	}
}

func TestDiffIOSpanTypes(t *testing.T) {
	tests := []struct {
		name    string
		spanType SpanType
		attrKey string
		leftVal string
		rightVal string
	}{
		{"exec command", SpanExec, "command", "ls -la", "ls -al"},
		{"fs_read path", SpanFSRead, "path", "/tmp/a.txt", "/tmp/b.txt"},
		{"fs_write path", SpanFSWrite, "path", "/out/a.txt", "/out/b.txt"},
		{"net_io url", SpanNetIO, "url", "http://api.com/v1", "http://api.com/v2"},
		{"db_query query", SpanDBQuery, "query", "SELECT * FROM users", "SELECT * FROM orders"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left := &Trace{Spans: []*Span{
				{ID: "1", Type: tt.spanType, Name: "span", Attributes: map[string]any{tt.attrKey: tt.leftVal}},
			}}
			right := &Trace{Spans: []*Span{
				{ID: "1", Type: tt.spanType, Name: "span", Attributes: map[string]any{tt.attrKey: tt.rightVal}},
			}}

			diffs := Diff(left, right)
			found := false
			for _, d := range diffs {
				if d.Field == tt.attrKey && d.Type == "changed" {
					found = true
				}
			}
			if !found {
				t.Errorf("expected %s change diff, got %v", tt.attrKey, diffs)
			}
		})
	}
}

func TestDiffBothEmpty(t *testing.T) {
	left := &Trace{}
	right := &Trace{}
	diffs := Diff(left, right)
	if len(diffs) != 0 {
		t.Fatalf("expected 0 diffs for empty traces, got %d", len(diffs))
	}
}

func TestDiffOneEmpty(t *testing.T) {
	left := &Trace{Spans: []*Span{
		{ID: "1", Type: SpanExec, Name: "exec: ls", Attributes: map[string]any{"command": "ls"}},
	}}
	right := &Trace{}

	diffs := Diff(left, right)
	hasDiff := false
	for _, d := range diffs {
		if d.Type == "changed" || d.Type == "missing" {
			hasDiff = true
		}
	}
	if !hasDiff {
		t.Error("expected diffs when one trace is empty")
	}
}

func TestFormatDiffNoDiffs(t *testing.T) {
	result := FormatDiff(nil)
	if !strings.Contains(result, "identical") {
		t.Errorf("expected 'identical' message, got: %s", result)
	}
}

func TestFormatDiffWithDiffs(t *testing.T) {
	diffs := []Difference{
		{Type: "missing", Message: "missing LLM request"},
		{Type: "extra", Message: "extra tool call"},
		{Type: "changed", Message: "tool name changed"},
	}
	result := FormatDiff(diffs)
	if !strings.Contains(result, "3 difference(s)") {
		t.Errorf("expected count in output, got: %s", result)
	}
	if !strings.Contains(result, "MISSING") {
		t.Error("expected MISSING prefix")
	}
	if !strings.Contains(result, "EXTRA") {
		t.Error("expected EXTRA prefix")
	}
	if !strings.Contains(result, "CHANGED") {
		t.Error("expected CHANGED prefix")
	}
}

func TestGroupByType(t *testing.T) {
	spans := []*Span{
		{ID: "1", Type: SpanFunction},
		{ID: "2", Type: SpanExec},
		{ID: "3", Type: SpanFunction},
		{ID: "4", Type: SpanExec},
		{ID: "5", Type: SpanFSRead},
	}
	groups := groupByType(spans)
	if len(groups[SpanFunction]) != 2 {
		t.Errorf("expected 2 FUNCTION spans, got %d", len(groups[SpanFunction]))
	}
	if len(groups[SpanExec]) != 2 {
		t.Errorf("expected 2 EXEC spans, got %d", len(groups[SpanExec]))
	}
	if len(groups[SpanFSRead]) != 1 {
		t.Errorf("expected 1 FS_READ span, got %d", len(groups[SpanFSRead]))
	}
}

func TestCompareSpanSequenceExtraAndMissing(t *testing.T) {
	left := []*Span{
		{ID: "1", Name: "a", Attributes: map[string]any{"cmd": "ls"}},
	}
	right := []*Span{
		{ID: "1", Name: "a", Attributes: map[string]any{"cmd": "ls"}},
		{ID: "2", Name: "b", Attributes: map[string]any{"cmd": "pwd"}},
	}

	diffs := compareSpanSequence(left, right, "exec", "cmd")
	hasExtra := false
	for _, d := range diffs {
		if d.Type == "extra" {
			hasExtra = true
		}
	}
	if !hasExtra {
		t.Error("expected extra diff")
	}

}
