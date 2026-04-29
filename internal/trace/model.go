package trace

import (
	"time"
)

// SpanType represents the kind of span in an execution trace.
type SpanType string

const (
	SpanFunction    SpanType = "FUNCTION"
	SpanLLMRequest  SpanType = "LLM_REQUEST"
	SpanLLMResponse SpanType = "LLM_RESPONSE"
	SpanToolCall    SpanType = "TOOL_CALL"
	SpanToolResult  SpanType = "TOOL_RESULT"
	SpanUserInput   SpanType = "USER_INPUT"
	SpanAgentOutput SpanType = "AGENT_OUTPUT"
	SpanExec        SpanType = "EXEC"        // subprocess execution
	SpanFSRead      SpanType = "FS_READ"     // filesystem read
	SpanFSWrite     SpanType = "FS_WRITE"    // filesystem write
	SpanNetIO       SpanType = "NET_IO"      // network I/O (TCP/HTTP/gRPC)
	SpanDBQuery     SpanType = "DB_QUERY"    // database query
)

// Span represents a single unit of work in an execution trace.
type Span struct {
	ID         string         `json:"id"`
	ParentID   string         `json:"parent_id,omitempty"`
	Type       SpanType       `json:"type"`
	Name       string         `json:"name"`
	StartTime  time.Time      `json:"start_time"`
	EndTime    time.Time      `json:"end_time"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// Metadata holds contextual information about the trace.
type Metadata struct {
	Command    string `json:"command"`
	Language   string `json:"language"`
	WorkingDir string `json:"working_dir"`
}

// Trace represents a complete execution trace of an agent session.
type Trace struct {
	ID        string  `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Metadata  Metadata  `json:"metadata"`
	Spans     []*Span   `json:"spans"`
}

// AddSpan appends a span to the trace.
func (t *Trace) AddSpan(s *Span) {
	t.Spans = append(t.Spans, s)
}

// SpansByType returns all spans matching the given type.
func (t *Trace) SpansByType(st SpanType) []*Span {
	var result []*Span
	for _, s := range t.Spans {
		if s.Type == st {
			result = append(result, s)
		}
	}
	return result
}
