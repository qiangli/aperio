package trace

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

// ContentRedactor is an interface for redacting string content during enrichment.
// This avoids a direct dependency on the proxy package.
type ContentRedactor interface {
	RedactString(s string) string
}

// EnrichOptions configures enrichment behavior.
type EnrichOptions struct {
	// Redactor applies redaction to extracted content strings.
	Redactor ContentRedactor
}

// Enrich processes raw LLM_REQUEST spans and extracts semantic spans
// (USER_INPUT, TOOL_CALL, TOOL_RESULT, AGENT_OUTPUT) from the API payloads.
func Enrich(t *Trace, opts ...EnrichOptions) {
	var redactor ContentRedactor
	if len(opts) > 0 && opts[0].Redactor != nil {
		redactor = opts[0].Redactor
	}

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
		case "embeddings":
			newSpans = append(newSpans, extractVectorSearch(span, reqBody, respBody)...)
		case "openai":
			newSpans = append(newSpans, extractOpenAI(span, reqBody, respBody, redactor)...)
		case "anthropic":
			newSpans = append(newSpans, extractAnthropic(span, reqBody, respBody, redactor)...)
		}
	}

	t.Spans = append(t.Spans, newSpans...)
}

func detectProvider(url string) string {
	switch {
	case strings.Contains(url, "/v1/embeddings"):
		return "embeddings"
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
func extractOpenAI(parent *Span, reqBody, respBody string, redactor ContentRedactor) []*Span {
	var spans []*Span

	// Parse request to find user messages
	var req openAIRequest
	if err := json.Unmarshal([]byte(reqBody), &req); err == nil {
		// Detect context injection and memory retrieval
		spans = append(spans, extractContextInjection(parent, req.Messages)...)
		spans = append(spans, extractMemoryRetrieval(parent, req.Messages)...)

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
						"content": redactContent(truncate(msg.Content, 500), redactor),
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
						"tool_name":    msg.Name,
						"tool_call_id": msg.ToolCallID,
						"content":      redactContent(truncate(msg.Content, 500), redactor),
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
						"content": redactContent(truncate(msg.Content, 500), redactor),
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
func extractAnthropic(parent *Span, reqBody, respBody string, redactor ContentRedactor) []*Span {
	var spans []*Span

	// Parse request
	var req anthropicRequest
	if err := json.Unmarshal([]byte(reqBody), &req); err == nil {
		// Detect context injection in system prompt
		if req.System != "" && len(req.System) >= 100 && hasRAGMarkers(req.System) {
			spans = append(spans, &Span{
				ID:        uuid.New().String(),
				ParentID:  parent.ID,
				Type:      SpanContextInjection,
				Name:      "context injection",
				StartTime: parent.StartTime,
				EndTime:   parent.StartTime,
				Attributes: map[string]any{
					"context_type":   "system",
					"token_estimate": len(req.System) / 4,
					"content":        truncate(req.System, 500),
				},
			})
		}

		// Detect memory retrieval from tool_result blocks
		for _, msg := range req.Messages {
			if msg.Role != "user" {
				continue
			}
			var blocks []anthropicContentBlock
			if err := json.Unmarshal(msg.Content, &blocks); err == nil {
				for _, b := range blocks {
					if b.Type == "tool_result" && isMemoryTool(b.Name) {
						spans = append(spans, &Span{
							ID:        uuid.New().String(),
							ParentID:  parent.ID,
							Type:      SpanMemoryRetrieval,
							Name:      "memory retrieval: " + b.Name,
							StartTime: parent.StartTime,
							EndTime:   parent.StartTime,
							Attributes: map[string]any{
								"source":    b.Name,
								"tool_name": b.Name,
								"content":   truncate(b.Text, 500),
							},
						})
					}
				}
			}
		}

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
						"content": redactContent(truncate(content, 500), redactor),
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
						"content": redactContent(truncate(block.Text, 500), redactor),
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

// RAG context markers used to detect context injection in system messages.
var ragMarkers = []string{
	"<context>", "<documents>", "<retrieved>", "<search_results>",
	"Source:", "Document:", "Retrieved context:",
	"## Context", "## Retrieved Documents", "## Search Results",
}

// Memory tool name patterns used to detect memory retrieval tool calls.
var memoryToolPatterns = []string{
	"search", "retrieve", "memory", "lookup", "vector_search",
	"rag", "recall", "fetch_context", "query_knowledge",
}

// extractContextInjection detects system messages containing RAG-style context.
func extractContextInjection(parent *Span, messages []openAIMessage) []*Span {
	var spans []*Span
	for _, msg := range messages {
		if msg.Role != "system" || len(msg.Content) < 100 {
			continue
		}
		if !hasRAGMarkers(msg.Content) {
			continue
		}
		spans = append(spans, &Span{
			ID:        uuid.New().String(),
			ParentID:  parent.ID,
			Type:      SpanContextInjection,
			Name:      "context injection",
			StartTime: parent.StartTime,
			EndTime:   parent.StartTime,
			Attributes: map[string]any{
				"context_type":   "system",
				"token_estimate": len(msg.Content) / 4, // rough token estimate
				"content":        truncate(msg.Content, 500),
			},
		})
	}
	return spans
}

func hasRAGMarkers(content string) bool {
	lower := strings.ToLower(content)
	for _, marker := range ragMarkers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

// extractMemoryRetrieval detects tool results from memory/retrieval tools.
func extractMemoryRetrieval(parent *Span, messages []openAIMessage) []*Span {
	var spans []*Span
	for _, msg := range messages {
		if msg.Role != "tool" {
			continue
		}
		if !isMemoryTool(msg.Name) {
			continue
		}
		spans = append(spans, &Span{
			ID:        uuid.New().String(),
			ParentID:  parent.ID,
			Type:      SpanMemoryRetrieval,
			Name:      "memory retrieval: " + msg.Name,
			StartTime: parent.StartTime,
			EndTime:   parent.StartTime,
			Attributes: map[string]any{
				"source":    msg.Name,
				"tool_name": msg.Name,
				"content":   truncate(msg.Content, 500),
			},
		})
	}
	return spans
}

func isMemoryTool(name string) bool {
	lower := strings.ToLower(name)
	for _, pattern := range memoryToolPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// extractVectorSearch extracts vector/embedding search spans from embedding API calls.
func extractVectorSearch(parent *Span, reqBody, respBody string) []*Span {
	var req struct {
		Input any    `json:"input"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal([]byte(reqBody), &req); err != nil {
		return nil
	}

	query := ""
	switch v := req.Input.(type) {
	case string:
		query = v
	case []any:
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				query = s
			}
		}
	}

	var resp struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
		Usage *openAIUsage `json:"usage,omitempty"`
		Model string       `json:"model"`
	}
	var resultCount int
	if err := json.Unmarshal([]byte(respBody), &resp); err == nil {
		resultCount = len(resp.Data)
		if resp.Model != "" {
			parent.Attributes["llm.model"] = resp.Model
		}
		if resp.Usage != nil {
			parent.Attributes["llm.token_count.prompt"] = resp.Usage.PromptTokens
			parent.Attributes["llm.token_count.total"] = resp.Usage.TotalTokens
		}
	}

	return []*Span{{
		ID:        uuid.New().String(),
		ParentID:  parent.ID,
		Type:      SpanVectorSearch,
		Name:      "vector search",
		StartTime: parent.StartTime,
		EndTime:   parent.EndTime,
		Attributes: map[string]any{
			"query":        truncate(query, 500),
			"model":        req.Model,
			"result_count": resultCount,
		},
	}}
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
	System   string             `json:"system,omitempty"`
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

func redactContent(s string, redactor ContentRedactor) string {
	if redactor == nil {
		return s
	}
	return redactor.RedactString(s)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
