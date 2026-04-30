package benchmark

import (
	"math"
	"testing"
	"time"

	"github.com/qiangli/aperio/internal/trace"
)

func TestExtractBasicMetrics(t *testing.T) {
	now := time.Now()
	tr := &trace.Trace{
		ID: "test",
		Spans: []*trace.Span{
			{
				ID: "proc", Type: trace.SpanProcess, Name: "agent",
				StartTime: now, EndTime: now.Add(5 * time.Second),
			},
			{
				ID: "llm1", ParentID: "proc", Type: trace.SpanLLMRequest, Name: "api",
				StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(1100 * time.Millisecond),
				Attributes: map[string]any{
					"llm.model":                  "gpt-4",
					"llm.token_count.prompt":     float64(100),
					"llm.token_count.completion": float64(50),
				},
			},
			{
				ID: "llm2", ParentID: "proc", Type: trace.SpanLLMRequest, Name: "api",
				StartTime: now.Add(2 * time.Second), EndTime: now.Add(3 * time.Second),
				Attributes: map[string]any{
					"llm.model":                  "gpt-4",
					"llm.token_count.prompt":     float64(200),
					"llm.token_count.completion": float64(100),
				},
			},
			{
				ID: "tool1", ParentID: "llm1", Type: trace.SpanToolCall, Name: "tool",
				StartTime: now.Add(500 * time.Millisecond), EndTime: now.Add(600 * time.Millisecond),
				Attributes: map[string]any{"tool_name": "read_file"},
			},
			{
				ID: "tool2", ParentID: "llm2", Type: trace.SpanToolCall, Name: "tool",
				StartTime: now.Add(2500 * time.Millisecond), EndTime: now.Add(2600 * time.Millisecond),
				Attributes: map[string]any{"tool_name": "write_file"},
			},
			{
				ID: "fs1", Type: trace.SpanFSRead, Name: "read",
				StartTime: now, EndTime: now,
				Attributes: map[string]any{"size": float64(1024)},
			},
			{
				ID: "fs2", Type: trace.SpanFSWrite, Name: "write",
				StartTime: now, EndTime: now,
				Attributes: map[string]any{"size": float64(2048)},
			},
			{
				ID: "out", Type: trace.SpanAgentOutput, Name: "output",
				StartTime: now, EndTime: now,
				Attributes: map[string]any{"content": "Hello, world!"},
			},
		},
	}

	pricing := DefaultPricing()
	m := Extract(tr, pricing)

	if m.TotalDurationMs != 5000 {
		t.Errorf("duration: expected 5000ms, got %d", m.TotalDurationMs)
	}
	if m.LLMCallCount != 2 {
		t.Errorf("LLM calls: expected 2, got %d", m.LLMCallCount)
	}
	if m.TotalInputTokens != 300 {
		t.Errorf("input tokens: expected 300, got %d", m.TotalInputTokens)
	}
	if m.TotalOutputTokens != 150 {
		t.Errorf("output tokens: expected 150, got %d", m.TotalOutputTokens)
	}
	if m.TotalCost == 0 {
		t.Error("cost should be > 0")
	}
	if m.ToolCallCount != 2 {
		t.Errorf("tool calls: expected 2, got %d", m.ToolCallCount)
	}
	if len(m.UniqueTools) != 2 {
		t.Errorf("unique tools: expected 2, got %d", len(m.UniqueTools))
	}
	if m.FilesRead != 1 {
		t.Errorf("files read: expected 1, got %d", m.FilesRead)
	}
	if m.FilesWritten != 1 {
		t.Errorf("files written: expected 1, got %d", m.FilesWritten)
	}
	if m.TotalBytesIO != 3072 {
		t.Errorf("bytes IO: expected 3072, got %d", m.TotalBytesIO)
	}
	if m.OutputText != "Hello, world!" {
		t.Errorf("output: %s", m.OutputText)
	}
	if m.TokenEfficiency != 0.5 {
		t.Errorf("efficiency: expected 0.5, got %f", m.TokenEfficiency)
	}
}

func TestExtractEmptyTrace(t *testing.T) {
	tr := &trace.Trace{ID: "empty"}
	m := Extract(tr, DefaultPricing())

	if m.TotalDurationMs != 0 {
		t.Errorf("expected 0 duration, got %d", m.TotalDurationMs)
	}
	if m.LLMCallCount != 0 {
		t.Error("expected 0 LLM calls")
	}
}

func TestExtractLatencyPercentiles(t *testing.T) {
	now := time.Now()
	var spans []*trace.Span
	// Create 100 LLM spans with latencies 1ms, 2ms, ..., 100ms
	for i := 1; i <= 100; i++ {
		spans = append(spans, &trace.Span{
			ID: "llm", Type: trace.SpanLLMRequest,
			StartTime:  now,
			EndTime:    now.Add(time.Duration(i) * time.Millisecond),
			Attributes: map[string]any{"llm.model": "gpt-4"},
		})
	}

	tr := &trace.Trace{ID: "latency", Spans: spans}
	m := Extract(tr, DefaultPricing())

	// With 100 values (1..100), index-based percentile: P50=idx50=51, P95=idx95=96, P99=idx99=100
	if m.P50LatencyMs < 49 || m.P50LatencyMs > 52 {
		t.Errorf("P50: expected ~50, got %d", m.P50LatencyMs)
	}
	if m.P95LatencyMs < 94 || m.P95LatencyMs > 97 {
		t.Errorf("P95: expected ~95, got %d", m.P95LatencyMs)
	}
	if m.P99LatencyMs < 98 || m.P99LatencyMs > 100 {
		t.Errorf("P99: expected ~99, got %d", m.P99LatencyMs)
	}
}

func TestExtractErrorCounting(t *testing.T) {
	now := time.Now()
	tr := &trace.Trace{
		ID: "errors",
		Spans: []*trace.Span{
			{ID: "e1", Type: trace.SpanExec, StartTime: now, EndTime: now,
				Attributes: map[string]any{"exit_code": float64(0)}},
			{ID: "e2", Type: trace.SpanExec, StartTime: now, EndTime: now,
				Attributes: map[string]any{"exit_code": float64(1)}},
			{ID: "e3", Type: trace.SpanExec, StartTime: now, EndTime: now,
				Attributes: map[string]any{"exit_code": float64(2)}},
		},
	}

	m := Extract(tr, DefaultPricing())
	if m.ErrorCount != 2 {
		t.Errorf("errors: expected 2 (exit codes 1 and 2), got %d", m.ErrorCount)
	}
}

func TestExtractModelUsage(t *testing.T) {
	now := time.Now()
	tr := &trace.Trace{
		ID: "models",
		Spans: []*trace.Span{
			{ID: "l1", Type: trace.SpanLLMRequest, StartTime: now, EndTime: now.Add(time.Second),
				Attributes: map[string]any{"llm.model": "gpt-4", "llm.token_count.prompt": float64(100), "llm.token_count.completion": float64(50)}},
			{ID: "l2", Type: trace.SpanLLMRequest, StartTime: now, EndTime: now.Add(time.Second),
				Attributes: map[string]any{"llm.model": "claude-sonnet-4", "llm.token_count.prompt": float64(200), "llm.token_count.completion": float64(100)}},
		},
	}

	m := Extract(tr, DefaultPricing())
	if len(m.ModelUsage) != 2 {
		t.Fatalf("expected 2 models, got %d", len(m.ModelUsage))
	}
	if m.ModelUsage["gpt-4"].CallCount != 1 {
		t.Error("expected 1 gpt-4 call")
	}
	if m.ModelUsage["claude-sonnet-4"].InputTokens != 200 {
		t.Error("expected 200 input tokens for claude")
	}
}

func TestAverageMetrics(t *testing.T) {
	m1 := &TraceMetrics{TotalDurationMs: 1000, TotalCost: 0.10, LLMCallCount: 2, ModelUsage: map[string]ModelMetrics{}}
	m2 := &TraceMetrics{TotalDurationMs: 2000, TotalCost: 0.20, LLMCallCount: 4, ModelUsage: map[string]ModelMetrics{}}

	avg := AverageMetrics([]*TraceMetrics{m1, m2})
	if avg.TotalDurationMs != 1500 {
		t.Errorf("avg duration: expected 1500, got %d", avg.TotalDurationMs)
	}
	if math.Abs(avg.TotalCost-0.15) > 0.001 {
		t.Errorf("avg cost: expected 0.15, got %f", avg.TotalCost)
	}
	if avg.LLMCallCount != 3 {
		t.Errorf("avg LLM calls: expected 3, got %d", avg.LLMCallCount)
	}
}

func TestAverageMetricsEmpty(t *testing.T) {
	avg := AverageMetrics(nil)
	if avg.TotalDurationMs != 0 {
		t.Error("empty average should be zero")
	}
}

func TestAverageMetricsSingle(t *testing.T) {
	m := &TraceMetrics{TotalDurationMs: 1000, ModelUsage: map[string]ModelMetrics{}}
	avg := AverageMetrics([]*TraceMetrics{m})
	if avg.TotalDurationMs != 1000 {
		t.Error("single metric should return as-is")
	}
}
