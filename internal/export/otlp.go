package export

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/qiangli/aperio/internal/trace"
)

// OTLP SpanKind constants (matching the OTLP proto enum values).
const (
	SpanKindUnspecified = 0
	SpanKindInternal    = 1
	SpanKindServer      = 2
	SpanKindClient      = 3
	SpanKindProducer    = 4
	SpanKindConsumer    = 5
)

// OTLPExportRequest is the top-level OTLP JSON structure.
type OTLPExportRequest struct {
	ResourceSpans []ResourceSpans `json:"resourceSpans"`
}

// ResourceSpans groups spans by resource.
type ResourceSpans struct {
	Resource   Resource     `json:"resource"`
	ScopeSpans []ScopeSpans `json:"scopeSpans"`
}

// Resource describes the entity producing spans.
type Resource struct {
	Attributes []KeyValue `json:"attributes"`
}

// ScopeSpans groups spans by instrumentation scope.
type ScopeSpans struct {
	Scope InstrumentationScope `json:"scope"`
	Spans []OTLPSpan           `json:"spans"`
}

// InstrumentationScope identifies the instrumentation library.
type InstrumentationScope struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// OTLPSpan is the OTLP JSON representation of a span.
type OTLPSpan struct {
	TraceID           string      `json:"traceId"`
	SpanID            string      `json:"spanId"`
	ParentSpanID      string      `json:"parentSpanId,omitempty"`
	Name              string      `json:"name"`
	Kind              int         `json:"kind"`
	StartTimeUnixNano string      `json:"startTimeUnixNano"`
	EndTimeUnixNano   string      `json:"endTimeUnixNano"`
	Attributes        []KeyValue  `json:"attributes,omitempty"`
	Status            *SpanStatus `json:"status,omitempty"`
	Events            []SpanEvent `json:"events,omitempty"`
	Links             []SpanLink  `json:"links,omitempty"`
}

// SpanStatus represents the status of a span.
type SpanStatus struct {
	Code    int    `json:"code"`              // 0=Unset, 1=OK, 2=Error
	Message string `json:"message,omitempty"`
}

// SpanEvent represents a named event within a span's timeline.
type SpanEvent struct {
	Name              string     `json:"name"`
	TimeUnixNano      string     `json:"timeUnixNano"`
	Attributes        []KeyValue `json:"attributes,omitempty"`
}

// SpanLink connects a span to another span in a different (or same) trace.
type SpanLink struct {
	TraceID    string     `json:"traceId"`
	SpanID     string     `json:"spanId"`
	Attributes []KeyValue `json:"attributes,omitempty"`
}

// OTLP status codes.
const (
	StatusCodeUnset = 0
	StatusCodeOK    = 1
	StatusCodeError = 2
)

// KeyValue is an OTLP attribute key-value pair.
type KeyValue struct {
	Key   string     `json:"key"`
	Value AnyValue   `json:"value"`
}

// AnyValue represents an OTLP attribute value.
type AnyValue struct {
	StringValue *string  `json:"stringValue,omitempty"`
	IntValue    *string  `json:"intValue,omitempty"`
	DoubleValue *float64 `json:"doubleValue,omitempty"`
	BoolValue   *bool    `json:"boolValue,omitempty"`
}

// ConvertTrace converts an Aperio Trace to an OTLP export request.
func ConvertTrace(t *trace.Trace, serviceName string) *OTLPExportRequest {
	traceID := uuidToTraceID(t.ID)

	// Build OTLP spans
	otlpSpans := make([]OTLPSpan, 0, len(t.Spans))
	for _, span := range t.Spans {
		otlpSpans = append(otlpSpans, convertSpan(span, traceID))
	}

	return &OTLPExportRequest{
		ResourceSpans: []ResourceSpans{
			{
				Resource: Resource{
					Attributes: []KeyValue{
						stringKV("service.name", serviceName),
						stringKV("aperio.command", t.Metadata.Command),
						stringKV("aperio.language", t.Metadata.Language),
						stringKV("aperio.working_dir", t.Metadata.WorkingDir),
					},
				},
				ScopeSpans: []ScopeSpans{
					{
						Scope: InstrumentationScope{
							Name:    "aperio",
							Version: "0.1.0",
						},
						Spans: otlpSpans,
					},
				},
			},
		},
	}
}

func convertSpan(s *trace.Span, traceID string) OTLPSpan {
	attrs := convertAttributes(s)

	// Add the original Aperio span type as an attribute
	attrs = append(attrs, stringKV("aperio.span_type", string(s.Type)))

	os := OTLPSpan{
		TraceID:           traceID,
		SpanID:            uuidToSpanID(s.ID),
		Name:              s.Name,
		Kind:              spanTypeToKind(s.Type),
		StartTimeUnixNano: timeToNano(s.StartTime),
		EndTimeUnixNano:   timeToNano(s.EndTime),
		Attributes:        attrs,
	}

	if s.ParentID != "" {
		os.ParentSpanID = uuidToSpanID(s.ParentID)
	}

	// Set span status from attributes
	os.Status = deriveStatus(s)

	// Add token usage events for LLM spans
	if s.Type == trace.SpanLLMRequest {
		if event := tokenUsageEvent(s); event != nil {
			os.Events = append(os.Events, *event)
		}
	}

	// Add span links for multi-agent correlation
	if link := correlationLink(s); link != nil {
		os.Links = append(os.Links, *link)
	}

	return os
}

func deriveStatus(s *trace.Span) *SpanStatus {
	if s.Attributes == nil {
		return &SpanStatus{Code: StatusCodeOK}
	}

	// Check HTTP status code
	if statusCode := s.IntAttr("http.status_code"); statusCode >= 400 {
		return &SpanStatus{
			Code:    StatusCodeError,
			Message: fmt.Sprintf("HTTP %d", statusCode),
		}
	}

	// Check exit code
	if exitCode := s.IntAttr("exit_code"); exitCode != 0 {
		return &SpanStatus{
			Code:    StatusCodeError,
			Message: fmt.Sprintf("exit code %d", exitCode),
		}
	}

	return &SpanStatus{Code: StatusCodeOK}
}

func tokenUsageEvent(s *trace.Span) *SpanEvent {
	prompt := s.IntAttr("llm.token_count.prompt")
	completion := s.IntAttr("llm.token_count.completion")
	input := s.IntAttr("llm.token_count.input")
	output := s.IntAttr("llm.token_count.output")

	// Use whichever naming convention is present
	if prompt == 0 && input > 0 {
		prompt = input
	}
	if completion == 0 && output > 0 {
		completion = output
	}

	if prompt == 0 && completion == 0 {
		return nil
	}

	promptStr := fmt.Sprintf("%d", prompt)
	completionStr := fmt.Sprintf("%d", completion)

	return &SpanEvent{
		Name:         "gen_ai.usage",
		TimeUnixNano: timeToNano(s.EndTime),
		Attributes: []KeyValue{
			{Key: "gen_ai.usage.input_tokens", Value: AnyValue{IntValue: &promptStr}},
			{Key: "gen_ai.usage.output_tokens", Value: AnyValue{IntValue: &completionStr}},
		},
	}
}

func correlationLink(s *trace.Span) *SpanLink {
	if s.Attributes == nil {
		return nil
	}
	corrID := s.StringAttr("correlation.id")
	parentTrace := s.StringAttr("agent.source_trace")
	if corrID == "" && parentTrace == "" {
		return nil
	}

	linkTraceID := corrID
	if parentTrace != "" {
		linkTraceID = parentTrace
	}

	return &SpanLink{
		TraceID: uuidToTraceID(linkTraceID),
		SpanID:  uuidToSpanID(s.ID),
		Attributes: []KeyValue{
			stringKV("aperio.link.type", "multi-agent-correlation"),
		},
	}
}

func convertAttributes(s *trace.Span) []KeyValue {
	if len(s.Attributes) == 0 {
		return nil
	}

	var kvs []KeyValue
	for k, v := range s.Attributes {
		mappedKey := mapAttributeName(k)

		switch val := v.(type) {
		case string:
			kvs = append(kvs, stringKV(mappedKey, val))
		case float64:
			// JSON numbers are float64; check if it's an integer
			if val == float64(int64(val)) {
				intStr := fmt.Sprintf("%d", int64(val))
				kvs = append(kvs, KeyValue{
					Key:   mappedKey,
					Value: AnyValue{IntValue: &intStr},
				})
			} else {
				kvs = append(kvs, KeyValue{
					Key:   mappedKey,
					Value: AnyValue{DoubleValue: &val},
				})
			}
		case bool:
			kvs = append(kvs, KeyValue{
				Key:   mappedKey,
				Value: AnyValue{BoolValue: &val},
			})
		default:
			// Convert other types to string
			str := fmt.Sprintf("%v", v)
			kvs = append(kvs, stringKV(mappedKey, str))
		}
	}

	return kvs
}

func spanTypeToKind(t trace.SpanType) int {
	switch t {
	case trace.SpanLLMRequest:
		return SpanKindClient
	case trace.SpanLLMResponse:
		return SpanKindClient
	case trace.SpanNetIO:
		return SpanKindClient
	case trace.SpanMemoryRetrieval:
		return SpanKindClient
	case trace.SpanVectorSearch:
		return SpanKindClient
	case trace.SpanContextInjection:
		return SpanKindInternal
	default:
		return SpanKindInternal
	}
}

// uuidToTraceID converts a UUID string to a 32-char hex trace ID.
// OTLP trace IDs are 16 bytes (32 hex chars).
func uuidToTraceID(uuid string) string {
	clean := strings.ReplaceAll(uuid, "-", "")
	if len(clean) >= 32 {
		return clean[:32]
	}
	// Pad with zeros if needed
	return clean + strings.Repeat("0", 32-len(clean))
}

// uuidToSpanID converts a UUID string to a 16-char hex span ID.
// OTLP span IDs are 8 bytes (16 hex chars).
func uuidToSpanID(uuid string) string {
	clean := strings.ReplaceAll(uuid, "-", "")
	if len(clean) >= 16 {
		// Use last 16 chars to maximize uniqueness across spans
		return clean[len(clean)-16:]
	}
	return clean + strings.Repeat("0", 16-len(clean))
}

func timeToNano(t time.Time) string {
	return fmt.Sprintf("%d", t.UnixNano())
}

func stringKV(key, value string) KeyValue {
	return KeyValue{
		Key:   key,
		Value: AnyValue{StringValue: &value},
	}
}

// ValidateTraceID checks if a hex string is a valid OTLP trace ID.
func ValidateTraceID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

// ValidateSpanID checks if a hex string is a valid OTLP span ID.
func ValidateSpanID(id string) bool {
	if len(id) != 16 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}
