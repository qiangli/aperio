package benchmark

import (
	"testing"
	"time"

	"github.com/qiangli/aperio/internal/trace"
)

func TestRankValuesLowerBetter(t *testing.T) {
	values := []float64{100, 50, 200, 50}
	ranks := rankValues(values, true)
	// 50=rank1, 50=rank1, 100=rank3, 200=rank4
	if ranks[0] != 3 {
		t.Errorf("ranks[0]: expected 3, got %d", ranks[0])
	}
	if ranks[1] != 1 {
		t.Errorf("ranks[1]: expected 1, got %d", ranks[1])
	}
	if ranks[2] != 4 {
		t.Errorf("ranks[2]: expected 4, got %d", ranks[2])
	}
	if ranks[3] != 1 {
		t.Errorf("ranks[3]: expected 1 (tie), got %d", ranks[3])
	}
}

func TestRankValuesHigherBetter(t *testing.T) {
	values := []float64{0.8, 0.95, 0.7}
	ranks := rankValues(values, false)
	// 0.95=rank1, 0.8=rank2, 0.7=rank3
	if ranks[0] != 2 {
		t.Errorf("ranks[0]: expected 2, got %d", ranks[0])
	}
	if ranks[1] != 1 {
		t.Errorf("ranks[1]: expected 1, got %d", ranks[1])
	}
	if ranks[2] != 3 {
		t.Errorf("ranks[2]: expected 3, got %d", ranks[2])
	}
}

func TestComputeAggregates(t *testing.T) {
	tools := []string{"A", "B", "C"}
	rankings := map[string][]int{
		"cost":  {1, 2, 3},
		"speed": {2, 1, 3},
		"quality": {3, 1, 2},
	}

	agg := computeAggregates(tools, rankings)
	if len(agg) != 3 {
		t.Fatalf("expected 3 scores, got %d", len(agg))
	}

	// B should be best (avg rank = (2+1+1)/3 = 1.33)
	if agg[0].Tool != "B" {
		t.Errorf("expected B to be ranked first, got %s", agg[0].Tool)
	}
	if agg[0].WinCount != 2 {
		t.Errorf("B win count: expected 2, got %d", agg[0].WinCount)
	}
}

func TestCompareTraces(t *testing.T) {
	now := time.Now()
	t1 := &trace.Trace{
		ID: "t1",
		Spans: []*trace.Span{
			{ID: "p1", Type: trace.SpanProcess, Name: "tool-a",
				StartTime: now, EndTime: now.Add(2 * time.Second)},
			{ID: "l1", ParentID: "p1", Type: trace.SpanLLMRequest, Name: "api",
				StartTime: now, EndTime: now.Add(time.Second),
				Attributes: map[string]any{"llm.model": "gpt-4", "llm.token_count.prompt": float64(100), "llm.token_count.completion": float64(50)}},
		},
	}
	t2 := &trace.Trace{
		ID: "t2",
		Spans: []*trace.Span{
			{ID: "p2", Type: trace.SpanProcess, Name: "tool-b",
				StartTime: now, EndTime: now.Add(3 * time.Second)},
			{ID: "l2", ParentID: "p2", Type: trace.SpanLLMRequest, Name: "api",
				StartTime: now, EndTime: now.Add(2 * time.Second),
				Attributes: map[string]any{"llm.model": "gpt-4", "llm.token_count.prompt": float64(200), "llm.token_count.completion": float64(100)}},
		},
	}
	t3 := &trace.Trace{
		ID: "t3",
		Spans: []*trace.Span{
			{ID: "p3", Type: trace.SpanProcess, Name: "tool-c",
				StartTime: now, EndTime: now.Add(time.Second)},
			{ID: "l3", ParentID: "p3", Type: trace.SpanLLMRequest, Name: "api",
				StartTime: now, EndTime: now.Add(500 * time.Millisecond),
				Attributes: map[string]any{"llm.model": "gpt-4", "llm.token_count.prompt": float64(50), "llm.token_count.completion": float64(25)}},
		},
	}

	cr := CompareTraces(
		[]string{"tool-a", "tool-b", "tool-c"},
		[]*trace.Trace{t1, t2, t3},
		DefaultPricing(),
	)

	if len(cr.Tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(cr.Tools))
	}

	// Metric matrix should have entries
	if len(cr.MetricMatrix) == 0 {
		t.Error("metric matrix is empty")
	}

	// Rankings should exist for all metrics
	if len(cr.Rankings) == 0 {
		t.Error("rankings are empty")
	}

	// tool-c should be fastest (1s duration)
	durationRanks := cr.Rankings["total_duration_ms"]
	if durationRanks[2] != 1 {
		t.Errorf("tool-c should be rank 1 for duration, got %d", durationRanks[2])
	}

	// Pairwise similarity should be NxN
	if len(cr.PairwiseSimilarity) != 3 {
		t.Fatalf("expected 3x3 similarity matrix, got %d", len(cr.PairwiseSimilarity))
	}
	// Diagonal should be 1.0
	for i := 0; i < 3; i++ {
		if cr.PairwiseSimilarity[i][i] != 1.0 {
			t.Errorf("diagonal[%d][%d] should be 1.0", i, i)
		}
	}
	// Matrix should be symmetric
	for i := 0; i < 3; i++ {
		for j := i + 1; j < 3; j++ {
			if cr.PairwiseSimilarity[i][j] != cr.PairwiseSimilarity[j][i] {
				t.Errorf("matrix not symmetric at [%d][%d]", i, j)
			}
		}
	}

	// Aggregate scores should be sorted by avg rank
	if len(cr.AggregateScores) != 3 {
		t.Fatalf("expected 3 aggregate scores, got %d", len(cr.AggregateScores))
	}
	for i := 1; i < len(cr.AggregateScores); i++ {
		if cr.AggregateScores[i].AvgRank < cr.AggregateScores[i-1].AvgRank {
			t.Error("aggregate scores should be sorted by avg rank")
		}
	}
}

func TestGetMetricValue(t *testing.T) {
	m := &TraceMetrics{
		TotalDurationMs:   5000,
		TotalCost:         0.10,
		TotalInputTokens:  100,
		TotalOutputTokens: 50,
		TokenEfficiency:   0.5,
		ToolCallCount:     3,
		LLMCallCount:      2,
		PassRate:          1.0,
		ModelUsage:        map[string]ModelMetrics{},
	}

	if getMetricValue(m, "total_duration_ms") != 5000 {
		t.Error("total_duration_ms")
	}
	if getMetricValue(m, "total_cost") != 0.10 {
		t.Error("total_cost")
	}
	if getMetricValue(m, "pass_rate") != 1.0 {
		t.Error("pass_rate")
	}
	if getMetricValue(nil, "total_cost") != 0 {
		t.Error("nil metrics should return 0")
	}
	if getMetricValue(m, "unknown") != 0 {
		t.Error("unknown metric should return 0")
	}
}
