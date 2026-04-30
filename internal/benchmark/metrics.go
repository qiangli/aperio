package benchmark

import (
	"sort"
	"strings"

	"github.com/qiangli/aperio/internal/trace"
)

// TraceMetrics holds all quantitative metrics extracted from a single trace.
type TraceMetrics struct {
	// Speed
	TotalDurationMs int64 `json:"total_duration_ms"`
	P50LatencyMs    int64 `json:"p50_latency_ms"`
	P95LatencyMs    int64 `json:"p95_latency_ms"`
	P99LatencyMs    int64 `json:"p99_latency_ms"`
	AvgLatencyMs    int64 `json:"avg_latency_ms"`

	// Cost
	TotalCost float64 `json:"total_cost"`

	// Tokens
	TotalInputTokens  int     `json:"total_input_tokens"`
	TotalOutputTokens int     `json:"total_output_tokens"`
	TokenEfficiency   float64 `json:"token_efficiency"`

	// Tool usage
	ToolCallCount int      `json:"tool_call_count"`
	UniqueTools   []string `json:"unique_tools"`
	LLMCallCount  int      `json:"llm_call_count"`

	// File operations
	FilesRead    int   `json:"files_read"`
	FilesWritten int   `json:"files_written"`
	TotalBytesIO int64 `json:"total_bytes_io"`

	// Quality
	PassRate float64 `json:"pass_rate"`

	// Reliability
	ErrorCount int `json:"error_count"`

	// Per-model breakdown
	ModelUsage map[string]ModelMetrics `json:"model_usage"`

	// Output text for semantic comparison
	OutputText string `json:"output_text,omitempty"`
}

// ModelMetrics holds per-model usage stats.
type ModelMetrics struct {
	CallCount    int     `json:"call_count"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalCost    float64 `json:"total_cost"`
}

// Extract computes all metrics from a trace using the given pricing table.
func Extract(t *trace.Trace, pricing PricingTable) *TraceMetrics {
	m := &TraceMetrics{
		ModelUsage: make(map[string]ModelMetrics),
	}

	var llmLatencies []int64
	toolSet := make(map[string]bool)
	var outputParts []string

	for _, s := range t.Spans {
		switch s.Type {
		case trace.SpanProcess:
			dur := s.EndTime.Sub(s.StartTime).Milliseconds()
			if dur > m.TotalDurationMs {
				m.TotalDurationMs = dur
			}

		case trace.SpanLLMRequest:
			m.LLMCallCount++
			latency := s.EndTime.Sub(s.StartTime).Milliseconds()
			llmLatencies = append(llmLatencies, latency)

			model := getStrAttr(s, "llm.model")
			inputTokens := getIntAttr(s, "llm.token_count.prompt")
			outputTokens := getIntAttr(s, "llm.token_count.completion")

			m.TotalInputTokens += inputTokens
			m.TotalOutputTokens += outputTokens

			cost := pricing.ComputeCost(model, inputTokens, outputTokens)
			m.TotalCost += cost

			if model != "" {
				mu := m.ModelUsage[model]
				mu.CallCount++
				mu.InputTokens += inputTokens
				mu.OutputTokens += outputTokens
				mu.TotalCost += cost
				m.ModelUsage[model] = mu
			}

		case trace.SpanToolCall:
			m.ToolCallCount++
			toolName := getStrAttr(s, "tool_name")
			if toolName != "" {
				toolSet[toolName] = true
			}

		case trace.SpanFSRead:
			m.FilesRead++
			m.TotalBytesIO += getInt64Attr(s, "size")

		case trace.SpanFSWrite:
			m.FilesWritten++
			m.TotalBytesIO += getInt64Attr(s, "size")

		case trace.SpanAgentOutput:
			content := getStrAttr(s, "content")
			if content != "" {
				outputParts = append(outputParts, content)
			}

		case trace.SpanExec:
			exitCode := getIntAttr(s, "exit_code")
			if exitCode != 0 {
				m.ErrorCount++
			}
		}
	}

	// Compute latency percentiles
	if len(llmLatencies) > 0 {
		sort.Slice(llmLatencies, func(i, j int) bool { return llmLatencies[i] < llmLatencies[j] })
		m.P50LatencyMs = percentile(llmLatencies, 50)
		m.P95LatencyMs = percentile(llmLatencies, 95)
		m.P99LatencyMs = percentile(llmLatencies, 99)

		var sum int64
		for _, l := range llmLatencies {
			sum += l
		}
		m.AvgLatencyMs = sum / int64(len(llmLatencies))
	}

	// Token efficiency
	if m.TotalInputTokens > 0 {
		m.TokenEfficiency = float64(m.TotalOutputTokens) / float64(m.TotalInputTokens)
	}

	// Unique tools
	for tool := range toolSet {
		m.UniqueTools = append(m.UniqueTools, tool)
	}
	sort.Strings(m.UniqueTools)

	// Output text
	m.OutputText = strings.Join(outputParts, "\n")

	return m
}

// AverageMetrics computes the arithmetic mean of multiple TraceMetrics.
func AverageMetrics(metrics []*TraceMetrics) *TraceMetrics {
	if len(metrics) == 0 {
		return &TraceMetrics{ModelUsage: make(map[string]ModelMetrics)}
	}
	if len(metrics) == 1 {
		return metrics[0]
	}

	n := float64(len(metrics))
	avg := &TraceMetrics{ModelUsage: make(map[string]ModelMetrics)}

	for _, m := range metrics {
		avg.TotalDurationMs += m.TotalDurationMs
		avg.P50LatencyMs += m.P50LatencyMs
		avg.P95LatencyMs += m.P95LatencyMs
		avg.P99LatencyMs += m.P99LatencyMs
		avg.AvgLatencyMs += m.AvgLatencyMs
		avg.TotalCost += m.TotalCost
		avg.TotalInputTokens += m.TotalInputTokens
		avg.TotalOutputTokens += m.TotalOutputTokens
		avg.TokenEfficiency += m.TokenEfficiency
		avg.ToolCallCount += m.ToolCallCount
		avg.LLMCallCount += m.LLMCallCount
		avg.FilesRead += m.FilesRead
		avg.FilesWritten += m.FilesWritten
		avg.TotalBytesIO += m.TotalBytesIO
		avg.PassRate += m.PassRate
		avg.ErrorCount += m.ErrorCount
	}

	avg.TotalDurationMs = int64(float64(avg.TotalDurationMs) / n)
	avg.P50LatencyMs = int64(float64(avg.P50LatencyMs) / n)
	avg.P95LatencyMs = int64(float64(avg.P95LatencyMs) / n)
	avg.P99LatencyMs = int64(float64(avg.P99LatencyMs) / n)
	avg.AvgLatencyMs = int64(float64(avg.AvgLatencyMs) / n)
	avg.TotalCost /= n
	avg.TotalInputTokens = int(float64(avg.TotalInputTokens) / n)
	avg.TotalOutputTokens = int(float64(avg.TotalOutputTokens) / n)
	avg.TokenEfficiency /= n
	avg.ToolCallCount = int(float64(avg.ToolCallCount) / n)
	avg.LLMCallCount = int(float64(avg.LLMCallCount) / n)
	avg.FilesRead = int(float64(avg.FilesRead) / n)
	avg.FilesWritten = int(float64(avg.FilesWritten) / n)
	avg.TotalBytesIO = int64(float64(avg.TotalBytesIO) / n)
	avg.PassRate /= n
	avg.ErrorCount = int(float64(avg.ErrorCount) / n)

	return avg
}

func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (p * len(sorted)) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func getStrAttr(s *trace.Span, key string) string {
	if s.Attributes == nil {
		return ""
	}
	if v, ok := s.Attributes[key]; ok {
		if str, ok := v.(string); ok {
			return str
		}
	}
	return ""
}

func getIntAttr(s *trace.Span, key string) int {
	if s.Attributes == nil {
		return 0
	}
	if v, ok := s.Attributes[key]; ok {
		switch val := v.(type) {
		case float64:
			return int(val)
		case int:
			return val
		}
	}
	return 0
}

func getInt64Attr(s *trace.Span, key string) int64 {
	if s.Attributes == nil {
		return 0
	}
	if v, ok := s.Attributes[key]; ok {
		switch val := v.(type) {
		case float64:
			return int64(val)
		case int64:
			return val
		case int:
			return int64(val)
		}
	}
	return 0
}
