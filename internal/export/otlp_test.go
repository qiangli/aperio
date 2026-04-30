package export

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/qiangli/aperio/internal/trace"
)

func TestUUIDToTraceID(t *testing.T) {
	tests := []struct {
		uuid string
		want string
	}{
		{"550e8400-e29b-41d4-a716-446655440000", "550e8400e29b41d4a716446655440000"},
		{"abc", "abc00000000000000000000000000000"},
		{"12345678901234567890123456789012", "12345678901234567890123456789012"},
	}
	for _, tt := range tests {
		got := uuidToTraceID(tt.uuid)
		if got != tt.want {
			t.Errorf("uuidToTraceID(%q) = %q, want %q", tt.uuid, got, tt.want)
		}
		if !ValidateTraceID(got) {
			t.Errorf("uuidToTraceID(%q) produced invalid trace ID: %q", tt.uuid, got)
		}
	}
}

func TestUUIDToSpanID(t *testing.T) {
	tests := []struct {
		uuid string
		want string
	}{
		{"550e8400-e29b-41d4-a716-446655440000", "a716446655440000"},
		{"abc", "abc0000000000000"},
	}
	for _, tt := range tests {
		got := uuidToSpanID(tt.uuid)
		if got != tt.want {
			t.Errorf("uuidToSpanID(%q) = %q, want %q", tt.uuid, got, tt.want)
		}
		if !ValidateSpanID(got) {
			t.Errorf("uuidToSpanID(%q) produced invalid span ID: %q", tt.uuid, got)
		}
	}
}

func TestSpanTypeToKind(t *testing.T) {
	tests := []struct {
		spanType trace.SpanType
		want     int
	}{
		{trace.SpanLLMRequest, SpanKindClient},
		{trace.SpanLLMResponse, SpanKindClient},
		{trace.SpanNetIO, SpanKindClient},
		{trace.SpanFunction, SpanKindInternal},
		{trace.SpanToolCall, SpanKindInternal},
		{trace.SpanExec, SpanKindInternal},
	}
	for _, tt := range tests {
		got := spanTypeToKind(tt.spanType)
		if got != tt.want {
			t.Errorf("spanTypeToKind(%s) = %d, want %d", tt.spanType, got, tt.want)
		}
	}
}

func TestConvertTrace(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tr := &trace.Trace{
		ID:        "550e8400-e29b-41d4-a716-446655440000",
		CreatedAt: now,
		Metadata: trace.Metadata{
			Command:    "python agent.py",
			Language:   "python",
			WorkingDir: "/tmp",
		},
		Spans: []*trace.Span{
			{
				ID:        "func-1",
				Type:      trace.SpanFunction,
				Name:      "main",
				StartTime: now,
				EndTime:   now.Add(time.Second),
				Attributes: map[string]any{
					"module": "main",
				},
			},
			{
				ID:        "llm-1",
				ParentID:  "func-1",
				Type:      trace.SpanLLMRequest,
				Name:      "POST /v1/chat/completions",
				StartTime: now.Add(100 * time.Millisecond),
				EndTime:   now.Add(500 * time.Millisecond),
				Attributes: map[string]any{
					"http.method":                "POST",
					"http.url":                   "https://api.openai.com/v1/chat/completions",
					"http.status_code":           float64(200),
					"llm.model":                  "gpt-4",
					"llm.token_count.prompt":     float64(45),
					"llm.token_count.completion": float64(30),
				},
			},
		},
	}

	result := ConvertTrace(tr, "test-service")

	if len(result.ResourceSpans) != 1 {
		t.Fatalf("expected 1 ResourceSpans, got %d", len(result.ResourceSpans))
	}

	rs := result.ResourceSpans[0]

	// Check resource attributes
	foundService := false
	for _, attr := range rs.Resource.Attributes {
		if attr.Key == "service.name" && *attr.Value.StringValue == "test-service" {
			foundService = true
		}
	}
	if !foundService {
		t.Error("missing service.name attribute")
	}

	// Check spans
	if len(rs.ScopeSpans) != 1 {
		t.Fatalf("expected 1 ScopeSpans, got %d", len(rs.ScopeSpans))
	}

	spans := rs.ScopeSpans[0].Spans
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	// All spans should have the same trace ID
	traceID := spans[0].TraceID
	for _, s := range spans {
		if s.TraceID != traceID {
			t.Errorf("span %s has different trace ID: %s vs %s", s.Name, s.TraceID, traceID)
		}
	}

	// Check parent-child relationship
	funcSpan := spans[0]
	llmSpan := spans[1]
	if funcSpan.ParentSpanID != "" {
		t.Errorf("root span should have no parent, got %s", funcSpan.ParentSpanID)
	}
	if llmSpan.ParentSpanID == "" {
		t.Error("LLM span should have parent")
	}
	if llmSpan.ParentSpanID != funcSpan.SpanID {
		t.Errorf("LLM parent should be func span: %s vs %s", llmSpan.ParentSpanID, funcSpan.SpanID)
	}

	// Check span kind
	if funcSpan.Kind != SpanKindInternal {
		t.Errorf("func span kind: expected %d, got %d", SpanKindInternal, funcSpan.Kind)
	}
	if llmSpan.Kind != SpanKindClient {
		t.Errorf("LLM span kind: expected %d, got %d", SpanKindClient, llmSpan.Kind)
	}

	// Check aperio.span_type attribute
	foundType := false
	for _, attr := range llmSpan.Attributes {
		if attr.Key == "aperio.span_type" && *attr.Value.StringValue == "LLM_REQUEST" {
			foundType = true
		}
	}
	if !foundType {
		t.Error("missing aperio.span_type attribute on LLM span")
	}
}

func TestConvertAttributeMapping(t *testing.T) {
	now := time.Now()
	tr := &trace.Trace{
		ID: "test-trace",
		Spans: []*trace.Span{
			{
				ID:        "s1",
				Type:      trace.SpanLLMRequest,
				Name:      "test",
				StartTime: now,
				EndTime:   now.Add(time.Second),
				Attributes: map[string]any{
					"llm.model":              "gpt-4",
					"llm.token_count.prompt": float64(100),
					"http.method":            "POST",
				},
			},
		},
	}

	result := ConvertTrace(tr, "svc")
	span := result.ResourceSpans[0].ScopeSpans[0].Spans[0]

	expectedMappings := map[string]bool{
		"gen_ai.request.model":    false,
		"gen_ai.usage.input_tokens": false,
		"http.request.method":     false,
	}

	for _, attr := range span.Attributes {
		if _, ok := expectedMappings[attr.Key]; ok {
			expectedMappings[attr.Key] = true
		}
	}

	for key, found := range expectedMappings {
		if !found {
			t.Errorf("expected mapped attribute %s not found", key)
		}
	}
}

func TestConvertTraceEmpty(t *testing.T) {
	tr := &trace.Trace{
		ID:       "empty",
		Metadata: trace.Metadata{Language: "go"},
	}

	result := ConvertTrace(tr, "svc")
	spans := result.ResourceSpans[0].ScopeSpans[0].Spans
	if len(spans) != 0 {
		t.Errorf("expected 0 spans, got %d", len(spans))
	}
}

func TestOTLPJSONRoundTrip(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tr := &trace.Trace{
		ID: "test",
		Spans: []*trace.Span{
			{ID: "s1", Type: trace.SpanFunction, Name: "main", StartTime: now, EndTime: now.Add(time.Second)},
		},
	}

	result := ConvertTrace(tr, "svc")

	jsonStr, err := FormatJSON(result)
	if err != nil {
		t.Fatalf("FormatJSON: %v", err)
	}

	// Verify it's valid JSON
	var parsed OTLPExportRequest
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}

	if len(parsed.ResourceSpans) != 1 {
		t.Errorf("expected 1 ResourceSpans after round-trip, got %d", len(parsed.ResourceSpans))
	}
}

func TestSendToEndpoint(t *testing.T) {
	var received []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		var err error
		received, err = readBody(r)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tr := &trace.Trace{
		ID: "send-test",
		Spans: []*trace.Span{
			{ID: "s1", Type: trace.SpanFunction, Name: "fn", StartTime: time.Now(), EndTime: time.Now().Add(time.Second)},
		},
	}

	req := ConvertTrace(tr, "test")
	err := Send(req, SendOptions{Endpoint: server.URL})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if len(received) == 0 {
		t.Error("server received empty body")
	}

	// Verify received data is valid OTLP JSON
	var parsed OTLPExportRequest
	if err := json.Unmarshal(received, &parsed); err != nil {
		t.Fatalf("unmarshal received: %v", err)
	}
}

func TestSendErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	req := &OTLPExportRequest{}
	err := Send(req, SendOptions{Endpoint: server.URL})
	if err == nil {
		t.Fatal("expected error for bad response")
	}
}

func TestMapAttributeName(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"llm.model", "gen_ai.request.model"},
		{"http.method", "http.request.method"},
		{"unknown_attr", "unknown_attr"},
	}
	for _, tt := range tests {
		got := mapAttributeName(tt.input)
		if got != tt.want {
			t.Errorf("mapAttributeName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestValidateIDs(t *testing.T) {
	if !ValidateTraceID("550e8400e29b41d4a716446655440000") {
		t.Error("valid trace ID rejected")
	}
	if ValidateTraceID("short") {
		t.Error("short trace ID accepted")
	}
	if ValidateTraceID("zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz") {
		t.Error("non-hex trace ID accepted")
	}
	if !ValidateSpanID("a716446655440000") {
		t.Error("valid span ID rejected")
	}
	if ValidateSpanID("short") {
		t.Error("short span ID accepted")
	}
}

func readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	buf := make([]byte, 0, 4096)
	for {
		tmp := make([]byte, 1024)
		n, err := r.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf, nil
}
