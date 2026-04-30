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
	SpanProcess     SpanType = "PROCESS"     // agent process lifetime (multi-agent root)
	SpanGoroutine        SpanType = "GOROUTINE"         // Go goroutine lifetime
	SpanGC               SpanType = "GC"               // Go garbage collection
	SpanMemoryRetrieval  SpanType = "MEMORY_RETRIEVAL"  // episodic/semantic memory retrieval
	SpanContextInjection SpanType = "CONTEXT_INJECTION" // context chunks fed to LLM call
	SpanVectorSearch     SpanType = "VECTOR_SEARCH"     // embedding/vector similarity query
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
	Command       string `json:"command"`
	Language      string `json:"language"`
	WorkingDir    string `json:"working_dir"`
	CorrelationID string `json:"correlation_id,omitempty"`
	ParentTraceID string `json:"parent_trace_id,omitempty"`
	ParentSpanID  string `json:"parent_span_id,omitempty"`
	AgentRole     string `json:"agent_role,omitempty"`
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

// StringAttr returns a string attribute value, or empty string if not found.
func (s *Span) StringAttr(key string) string {
	if s.Attributes == nil {
		return ""
	}
	v, ok := s.Attributes[key]
	if !ok {
		return ""
	}
	str, ok := v.(string)
	if !ok {
		return ""
	}
	return str
}

// IntAttr returns an integer attribute value, or 0 if not found or not numeric.
func (s *Span) IntAttr(key string) int {
	if s.Attributes == nil {
		return 0
	}
	v, ok := s.Attributes[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case int64:
		return int(n)
	default:
		return 0
	}
}

// FloatAttr returns a float64 attribute value, or 0 if not found or not numeric.
func (s *Span) FloatAttr(key string) float64 {
	if s.Attributes == nil {
		return 0
	}
	v, ok := s.Attributes[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

// BoolAttr returns a boolean attribute value, or false if not found.
func (s *Span) BoolAttr(key string) bool {
	if s.Attributes == nil {
		return false
	}
	v, ok := s.Attributes[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	if !ok {
		return false
	}
	return b
}

// SetAttr sets an attribute value on the span, initializing the map if needed.
func (s *Span) SetAttr(key string, value any) {
	if s.Attributes == nil {
		s.Attributes = make(map[string]any)
	}
	s.Attributes[key] = value
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
