package trace

import (
	"testing"
	"time"
)

func TestEnrichOpenAI(t *testing.T) {
	now := time.Now()

	tr := &Trace{
		ID: "test",
		Spans: []*Span{
			{
				ID:        "req-1",
				Type:      SpanLLMRequest,
				Name:      "POST https://api.openai.com/v1/chat/completions",
				StartTime: now,
				EndTime:   now.Add(time.Second),
				Attributes: map[string]any{
					"http.url": "https://api.openai.com/v1/chat/completions",
					"http.request_body": `{
						"model": "gpt-4",
						"messages": [
							{"role": "user", "content": "What is the weather in NYC?"}
						]
					}`,
					"http.response_body": `{
						"model": "gpt-4",
						"choices": [{
							"message": {
								"role": "assistant",
								"content": null,
								"tool_calls": [{
									"id": "call_123",
									"type": "function",
									"function": {
										"name": "get_weather",
										"arguments": "{\"location\": \"NYC\"}"
									}
								}]
							}
						}],
						"usage": {
							"prompt_tokens": 20,
							"completion_tokens": 15,
							"total_tokens": 35
						}
					}`,
				},
			},
		},
	}

	Enrich(tr)

	// Should have original span + USER_INPUT + TOOL_CALL
	if len(tr.Spans) < 3 {
		t.Fatalf("expected at least 3 spans, got %d", len(tr.Spans))
	}

	userInputs := tr.SpansByType(SpanUserInput)
	if len(userInputs) != 1 {
		t.Fatalf("expected 1 USER_INPUT span, got %d", len(userInputs))
	}
	if userInputs[0].ParentID != "req-1" {
		t.Errorf("USER_INPUT parent should be req-1, got %s", userInputs[0].ParentID)
	}

	toolCalls := tr.SpansByType(SpanToolCall)
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 TOOL_CALL span, got %d", len(toolCalls))
	}
	if toolCalls[0].Attributes["tool_name"] != "get_weather" {
		t.Errorf("expected tool_name get_weather, got %v", toolCalls[0].Attributes["tool_name"])
	}

	// Check token counts were added to parent
	if tr.Spans[0].Attributes["llm.token_count.total"] != 35 {
		t.Errorf("expected total tokens 35, got %v (type %T)", tr.Spans[0].Attributes["llm.token_count.total"], tr.Spans[0].Attributes["llm.token_count.total"])
	}
}

func TestEnrichOpenAIFinalAnswer(t *testing.T) {
	now := time.Now()

	tr := &Trace{
		ID: "test",
		Spans: []*Span{
			{
				ID:        "req-1",
				Type:      SpanLLMRequest,
				Name:      "POST https://api.openai.com/v1/chat/completions",
				StartTime: now,
				EndTime:   now.Add(time.Second),
				Attributes: map[string]any{
					"http.url": "https://api.openai.com/v1/chat/completions",
					"http.request_body": `{
						"model": "gpt-4",
						"messages": [
							{"role": "user", "content": "Hello"}
						]
					}`,
					"http.response_body": `{
						"model": "gpt-4",
						"choices": [{
							"message": {
								"role": "assistant",
								"content": "Hi there! How can I help?"
							}
						}]
					}`,
				},
			},
		},
	}

	Enrich(tr)

	outputs := tr.SpansByType(SpanAgentOutput)
	if len(outputs) != 1 {
		t.Fatalf("expected 1 AGENT_OUTPUT span, got %d", len(outputs))
	}
	content, _ := outputs[0].Attributes["content"].(string)
	if content != "Hi there! How can I help?" {
		t.Errorf("unexpected content: %s", content)
	}
}

func TestEnrichAnthropic(t *testing.T) {
	now := time.Now()

	tr := &Trace{
		ID: "test",
		Spans: []*Span{
			{
				ID:        "req-1",
				Type:      SpanLLMRequest,
				Name:      "POST https://api.anthropic.com/v1/messages",
				StartTime: now,
				EndTime:   now.Add(time.Second),
				Attributes: map[string]any{
					"http.url": "https://api.anthropic.com/v1/messages",
					"http.request_body": `{
						"model": "claude-3-opus-20240229",
						"messages": [
							{"role": "user", "content": "What time is it?"}
						]
					}`,
					"http.response_body": `{
						"model": "claude-3-opus-20240229",
						"content": [
							{"type": "tool_use", "id": "tu_123", "name": "get_time", "input": {"timezone": "UTC"}},
							{"type": "text", "text": "Let me check the time for you."}
						],
						"usage": {
							"input_tokens": 15,
							"output_tokens": 25
						}
					}`,
				},
			},
		},
	}

	Enrich(tr)

	toolCalls := tr.SpansByType(SpanToolCall)
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 TOOL_CALL span, got %d", len(toolCalls))
	}
	if toolCalls[0].Attributes["tool_name"] != "get_time" {
		t.Errorf("expected tool_name get_time, got %v", toolCalls[0].Attributes["tool_name"])
	}

	outputs := tr.SpansByType(SpanAgentOutput)
	if len(outputs) != 1 {
		t.Fatalf("expected 1 AGENT_OUTPUT span, got %d", len(outputs))
	}
}

func TestEnrichOpenAIMultipleToolCalls(t *testing.T) {
	now := time.Now()
	tr := &Trace{
		ID: "test",
		Spans: []*Span{{
			ID: "req-1", Type: SpanLLMRequest, StartTime: now, EndTime: now.Add(time.Second),
			Attributes: map[string]any{
				"http.url":          "https://api.openai.com/v1/chat/completions",
				"http.request_body": `{"model":"gpt-4","messages":[{"role":"user","content":"Weather and time?"}]}`,
				"http.response_body": `{"model":"gpt-4","choices":[{"message":{"role":"assistant","content":null,"tool_calls":[
					{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{}"}},
					{"id":"call_2","type":"function","function":{"name":"get_time","arguments":"{}"}}
				]}}]}`,
			},
		}},
	}
	Enrich(tr)
	toolCalls := tr.SpansByType(SpanToolCall)
	if len(toolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(toolCalls))
	}
	names := map[string]bool{}
	for _, tc := range toolCalls {
		names[tc.Attributes["tool_name"].(string)] = true
	}
	if !names["get_weather"] || !names["get_time"] {
		t.Errorf("expected get_weather and get_time, got %v", names)
	}
}

func TestEnrichOpenAIToolResult(t *testing.T) {
	now := time.Now()
	tr := &Trace{
		ID: "test",
		Spans: []*Span{{
			ID: "req-1", Type: SpanLLMRequest, StartTime: now, EndTime: now.Add(time.Second),
			Attributes: map[string]any{
				"http.url": "https://api.openai.com/v1/chat/completions",
				"http.request_body": `{"model":"gpt-4","messages":[
					{"role":"user","content":"Weather?"},
					{"role":"tool","name":"get_weather","tool_call_id":"call_1","content":"72F sunny"}
				]}`,
				"http.response_body": `{"model":"gpt-4","choices":[{"message":{"role":"assistant","content":"It's 72F and sunny."}}]}`,
			},
		}},
	}
	Enrich(tr)
	toolResults := tr.SpansByType(SpanToolResult)
	if len(toolResults) != 1 {
		t.Fatalf("expected 1 tool result, got %d", len(toolResults))
	}
	if toolResults[0].Attributes["tool_name"] != "get_weather" {
		t.Errorf("unexpected tool_name: %v", toolResults[0].Attributes["tool_name"])
	}
}

func TestEnrichOpenAIMissingUsage(t *testing.T) {
	now := time.Now()
	tr := &Trace{
		ID: "test",
		Spans: []*Span{{
			ID: "req-1", Type: SpanLLMRequest, StartTime: now, EndTime: now.Add(time.Second),
			Attributes: map[string]any{
				"http.url":           "https://api.openai.com/v1/chat/completions",
				"http.request_body":  `{"model":"gpt-4","messages":[{"role":"user","content":"Hi"}]}`,
				"http.response_body": `{"model":"gpt-4","choices":[{"message":{"role":"assistant","content":"Hello"}}]}`,
			},
		}},
	}
	Enrich(tr)
	// Should not crash, and should not have token count attributes
	if _, ok := tr.Spans[0].Attributes["llm.token_count.total"]; ok {
		t.Error("should not have token count when usage is missing")
	}
}

func TestEnrichUnknownProvider(t *testing.T) {
	now := time.Now()
	tr := &Trace{
		ID: "test",
		Spans: []*Span{{
			ID: "req-1", Type: SpanLLMRequest, StartTime: now, EndTime: now.Add(time.Second),
			Attributes: map[string]any{
				"http.url":           "https://unknown-llm.example.com/api/generate",
				"http.request_body":  `{"prompt":"hello"}`,
				"http.response_body": `{"text":"world"}`,
			},
		}},
	}
	initialLen := len(tr.Spans)
	Enrich(tr)
	// Should not crash, no new spans added for unknown provider
	if len(tr.Spans) != initialLen {
		t.Errorf("expected no new spans for unknown provider, got %d", len(tr.Spans)-initialLen)
	}
}

func TestEnrichMissingAttributes(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name  string
		attrs map[string]any
	}{
		{"nil attributes", nil},
		{"missing url", map[string]any{"http.request_body": "{}"}},
		{"missing request body", map[string]any{"http.url": "https://api.openai.com/v1/chat/completions"}},
		{"missing response body", map[string]any{
			"http.url":          "https://api.openai.com/v1/chat/completions",
			"http.request_body": `{"messages":[]}`,
		}},
		{"empty bodies", map[string]any{
			"http.url":           "https://api.openai.com/v1/chat/completions",
			"http.request_body":  "",
			"http.response_body": "",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &Trace{
				ID:    "test",
				Spans: []*Span{{ID: "req-1", Type: SpanLLMRequest, StartTime: now, EndTime: now.Add(time.Second), Attributes: tt.attrs}},
			}
			// Should not panic
			Enrich(tr)
		})
	}
}

func TestEnrichInvalidJSON(t *testing.T) {
	now := time.Now()
	tr := &Trace{
		ID: "test",
		Spans: []*Span{{
			ID: "req-1", Type: SpanLLMRequest, StartTime: now, EndTime: now.Add(time.Second),
			Attributes: map[string]any{
				"http.url":           "https://api.openai.com/v1/chat/completions",
				"http.request_body":  "not valid json {{{",
				"http.response_body": "also not json",
			},
		}},
	}
	initialLen := len(tr.Spans)
	Enrich(tr) // Should not panic
	if len(tr.Spans) != initialLen {
		t.Error("should not add spans from invalid JSON")
	}
}

func TestDetectProvider(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://api.openai.com/v1/chat/completions", "openai"},
		{"http://localhost:8080/v1/chat/completions", "openai"},
		{"https://api.anthropic.com/v1/messages", "anthropic"},
		{"http://localhost:8080/v1/messages", "anthropic"},
		{"https://generativelanguage.googleapis.com/v1/models/gemini:generate", "google"},
		{"https://random-api.example.com/generate", "unknown"},
		{"", "unknown"},
	}
	for _, tt := range tests {
		got := detectProvider(tt.url)
		if got != tt.want {
			t.Errorf("detectProvider(%q) = %s, want %s", tt.url, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if truncate("short", 10) != "short" {
		t.Error("should not truncate short string")
	}
	result := truncate("this is a long string", 10)
	if len(result) > 14 { // 10 + "..."
		t.Errorf("should truncate: %s", result)
	}
	if truncate("", 10) != "" {
		t.Error("empty string should stay empty")
	}
}

func TestGetStringAttr(t *testing.T) {
	span := &Span{Attributes: map[string]any{
		"key1": "value1",
		"key2": 42,
	}}
	if getStringAttr(span, "key1") != "value1" {
		t.Error("should return string value")
	}
	if getStringAttr(span, "key2") != "" {
		t.Error("should return empty for non-string")
	}
	if getStringAttr(span, "missing") != "" {
		t.Error("should return empty for missing key")
	}
	nilSpan := &Span{Attributes: nil}
	if getStringAttr(nilSpan, "key") != "" {
		t.Error("should return empty for nil attributes")
	}
}
