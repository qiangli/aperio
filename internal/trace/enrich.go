package trace

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

// Enrich processes raw LLM_REQUEST spans and extracts semantic spans
// (USER_INPUT, TOOL_CALL, TOOL_RESULT, AGENT_OUTPUT) from the API payloads.
func Enrich(t *Trace) {
	var newSpans []*Span

	for _, span := range t.Spans {
		if span.Type != SpanLLMRequest {
			continue
		}

		reqBody := getStringAttr(span, "http.request_body")
		respBody := getStringAttr(span, "http.response_body")
		url := getStringAttr(span, "http.url")

		provider := detectProvider(url)

		switch provider {
		case "openai":
			newSpans = append(newSpans, extractOpenAI(span, reqBody, respBody)...)
		case "anthropic":
			newSpans = append(newSpans, extractAnthropic(span, reqBody, respBody)...)
		}
	}

	t.Spans = append(t.Spans, newSpans...)
}

func detectProvider(url string) string {
	switch {
	case strings.Contains(url, "/v1/chat/completions"):
		return "openai"
	case strings.Contains(url, "/v1/messages"):
		return "anthropic"
	case strings.Contains(url, "generativelanguage.googleapis.com"):
		return "google"
	default:
		return "unknown"
	}
}

// extractOpenAI parses OpenAI chat completion request/response bodies.
func extractOpenAI(parent *Span, reqBody, respBody string) []*Span {
	var spans []*Span

	// Parse request to find user messages
	var req openAIRequest
	if err := json.Unmarshal([]byte(reqBody), &req); err == nil {
		for _, msg := range req.Messages {
			if msg.Role == "user" {
				spans = append(spans, &Span{
					ID:        uuid.New().String(),
					ParentID:  parent.ID,
					Type:      SpanUserInput,
					Name:      "user message",
					StartTime: parent.StartTime,
					EndTime:   parent.StartTime,
					Attributes: map[string]any{
						"content": truncate(msg.Content, 500),
					},
				})
			}
			if msg.Role == "tool" {
				spans = append(spans, &Span{
					ID:        uuid.New().String(),
					ParentID:  parent.ID,
					Type:      SpanToolResult,
					Name:      "tool_result: " + msg.Name,
					StartTime: parent.StartTime,
					EndTime:   parent.StartTime,
					Attributes: map[string]any{
						"tool_name": msg.Name,
						"tool_call_id": msg.ToolCallID,
						"content":   truncate(msg.Content, 500),
					},
				})
			}
		}
	}

	// Parse response to find tool calls and assistant output
	var resp openAIResponse
	if err := json.Unmarshal([]byte(respBody), &resp); err == nil {
		for _, choice := range resp.Choices {
			msg := choice.Message
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					spans = append(spans, &Span{
						ID:        uuid.New().String(),
						ParentID:  parent.ID,
						Type:      SpanToolCall,
						Name:      "tool_call: " + tc.Function.Name,
						StartTime: parent.EndTime,
						EndTime:   parent.EndTime,
						Attributes: map[string]any{
							"tool_call_id": tc.ID,
							"tool_name":    tc.Function.Name,
							"arguments":    tc.Function.Arguments,
						},
					})
				}
			}
			if msg.Content != "" && len(msg.ToolCalls) == 0 {
				spans = append(spans, &Span{
					ID:        uuid.New().String(),
					ParentID:  parent.ID,
					Type:      SpanAgentOutput,
					Name:      "assistant response",
					StartTime: parent.EndTime,
					EndTime:   parent.EndTime,
					Attributes: map[string]any{
						"content": truncate(msg.Content, 500),
					},
				})
			}
		}

		if resp.Usage != nil {
			parent.Attributes["llm.model"] = resp.Model
			parent.Attributes["llm.token_count.prompt"] = resp.Usage.PromptTokens
			parent.Attributes["llm.token_count.completion"] = resp.Usage.CompletionTokens
			parent.Attributes["llm.token_count.total"] = resp.Usage.TotalTokens
		}
	}

	return spans
}

// extractAnthropic parses Anthropic messages API request/response bodies.
func extractAnthropic(parent *Span, reqBody, respBody string) []*Span {
	var spans []*Span

	// Parse request
	var req anthropicRequest
	if err := json.Unmarshal([]byte(reqBody), &req); err == nil {
		for _, msg := range req.Messages {
			if msg.Role == "user" {
				content := extractAnthropicContent(msg.Content)
				spans = append(spans, &Span{
					ID:        uuid.New().String(),
					ParentID:  parent.ID,
					Type:      SpanUserInput,
					Name:      "user message",
					StartTime: parent.StartTime,
					EndTime:   parent.StartTime,
					Attributes: map[string]any{
						"content": truncate(content, 500),
					},
				})
			}
		}
	}

	// Parse response
	var resp anthropicResponse
	if err := json.Unmarshal([]byte(respBody), &resp); err == nil {
		for _, block := range resp.Content {
			switch block.Type {
			case "tool_use":
				argsJSON, _ := json.Marshal(block.Input)
				spans = append(spans, &Span{
					ID:        uuid.New().String(),
					ParentID:  parent.ID,
					Type:      SpanToolCall,
					Name:      "tool_call: " + block.Name,
					StartTime: parent.EndTime,
					EndTime:   parent.EndTime,
					Attributes: map[string]any{
						"tool_call_id": block.ID,
						"tool_name":    block.Name,
						"arguments":    string(argsJSON),
					},
				})
			case "text":
				spans = append(spans, &Span{
					ID:        uuid.New().String(),
					ParentID:  parent.ID,
					Type:      SpanAgentOutput,
					Name:      "assistant response",
					StartTime: parent.EndTime,
					EndTime:   parent.EndTime,
					Attributes: map[string]any{
						"content": truncate(block.Text, 500),
					},
				})
			}
		}

		if resp.Usage != nil {
			parent.Attributes["llm.model"] = resp.Model
			parent.Attributes["llm.token_count.input"] = resp.Usage.InputTokens
			parent.Attributes["llm.token_count.output"] = resp.Usage.OutputTokens
		}
	}

	return spans
}

// OpenAI types
type openAIRequest struct {
	Messages []openAIMessage `json:"messages"`
	Model    string          `json:"model"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	Name       string           `json:"name,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
	Usage *openAIUsage `json:"usage,omitempty"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Anthropic types
type anthropicRequest struct {
	Messages []anthropicMessage `json:"messages"`
	Model    string             `json:"model"`
}

type anthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // can be string or array of content blocks
}

type anthropicContentBlock struct {
	Type  string         `json:"type"`
	Text  string         `json:"text,omitempty"`
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`
}

type anthropicResponse struct {
	Model   string                  `json:"model"`
	Content []anthropicContentBlock `json:"content"`
	Usage   *anthropicUsage         `json:"usage,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// extractAnthropicContent handles both string and array content formats.
func extractAnthropicContent(raw json.RawMessage) string {
	// Try string first
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	// Try array of content blocks
	var blocks []anthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}

	return string(raw)
}

func getStringAttr(span *Span, key string) string {
	if span.Attributes == nil {
		return ""
	}
	v, ok := span.Attributes[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
