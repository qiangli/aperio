package benchmark

import (
	"fmt"
	"sort"

	"github.com/qiangli/aperio/internal/eval"
	"github.com/qiangli/aperio/internal/trace"
)

// ComparisonResult holds the N-way comparison output.
type ComparisonResult struct {
	Tools              []string             `json:"tools"`
	MetricMatrix       map[string][]float64 `json:"metric_matrix"`
	Rankings           map[string][]int     `json:"rankings"`
	PairwiseSimilarity [][]float64          `json:"pairwise_similarity"`
	AggregateScores    []AggregateScore     `json:"aggregate_scores"`
	PerTool            []ToolSummary        `json:"per_tool"`
}

// AggregateScore is a tool's overall ranking.
type AggregateScore struct {
	Tool     string  `json:"tool"`
	AvgRank  float64 `json:"avg_rank"`
	WinCount int     `json:"win_count"`
}

// ToolSummary holds per-tool details.
type ToolSummary struct {
	Tool       string        `json:"tool"`
	AvgMetrics *TraceMetrics `json:"avg_metrics"`
}

// Metrics where lower is better.
var lowerIsBetter = map[string]bool{
	"total_duration_ms":  true,
	"total_cost":         true,
	"total_input_tokens": true,
	"p50_latency_ms":     true,
	"p95_latency_ms":     true,
	"error_count":        true,
}

// metricNames returns the standard list of metrics for comparison.
func metricNames() []string {
	return []string{
		"total_duration_ms", "total_cost", "total_input_tokens", "total_output_tokens",
		"token_efficiency", "tool_call_count", "llm_call_count",
		"p50_latency_ms", "p95_latency_ms",
		"files_written", "pass_rate", "error_count",
		"pass_at_1",
	}
}

// CompareWithPassK performs comparison and computes pass@k if passKValues are specified.
func CompareWithPassK(results *BenchmarkResults, pricing PricingTable, passKValues []int) *ComparisonResult {
	cr := Compare(results, pricing)

	if len(passKValues) > 0 {
		for i, toolRun := range results.ToolRuns {
			passK := ComputeAllPassK(toolRun.Runs, passKValues)
			if cr.PerTool[i].AvgMetrics != nil {
				cr.PerTool[i].AvgMetrics.PassAtK = passK
			}
		}
		// Update metric matrix with pass@k values
		for _, k := range passKValues {
			key := fmt.Sprintf("pass_at_%d", k)
			var values []float64
			for _, ts := range cr.PerTool {
				if ts.AvgMetrics != nil && ts.AvgMetrics.PassAtK != nil {
					values = append(values, ts.AvgMetrics.PassAtK[k])
				} else {
					values = append(values, 0)
				}
			}
			cr.MetricMatrix[key] = values
			cr.Rankings[key] = rankValues(values, false) // higher is better
		}
		// Recompute aggregates with new metrics
		cr.AggregateScores = computeAggregates(cr.Tools, cr.Rankings)
	}

	return cr
}

// Compare performs an N-way comparison across benchmark results.
func Compare(results *BenchmarkResults, pricing PricingTable) *ComparisonResult {
	cr := &ComparisonResult{
		MetricMatrix: make(map[string][]float64),
		Rankings:     make(map[string][]int),
	}

	// Extract per-tool averaged metrics
	for _, toolRun := range results.ToolRuns {
		cr.Tools = append(cr.Tools, toolRun.Tool.Name)

		var runMetrics []*TraceMetrics
		for _, run := range toolRun.Runs {
			tr, err := trace.ReadTrace(run.TracePath)
			if err != nil {
				continue
			}
			m := Extract(tr, pricing)
			m.PassRate = ComputePassRate(run.Validation)
			runMetrics = append(runMetrics, m)
		}

		avg := AverageMetrics(runMetrics)
		cr.PerTool = append(cr.PerTool, ToolSummary{
			Tool:       toolRun.Tool.Name,
			AvgMetrics: avg,
		})
	}

	// Build metric matrix
	metrics := metricNames()

	for _, metric := range metrics {
		var values []float64
		for _, ts := range cr.PerTool {
			values = append(values, getMetricValue(ts.AvgMetrics, metric))
		}
		cr.MetricMatrix[metric] = values
	}

	// Compute rankings
	for metric, values := range cr.MetricMatrix {
		cr.Rankings[metric] = rankValues(values, lowerIsBetter[metric])
	}

	// Compute aggregate scores
	cr.AggregateScores = computeAggregates(cr.Tools, cr.Rankings)

	// Compute pairwise similarity
	cr.PairwiseSimilarity = computePairwiseSimilarity(results, pricing)

	return cr
}

// CompareTraces performs an N-way comparison on pre-existing traces.
func CompareTraces(tools []string, traces []*trace.Trace, pricing PricingTable) *ComparisonResult {
	cr := &ComparisonResult{
		Tools:        tools,
		MetricMatrix: make(map[string][]float64),
		Rankings:     make(map[string][]int),
	}

	for i, tr := range traces {
		m := Extract(tr, pricing)
		cr.PerTool = append(cr.PerTool, ToolSummary{
			Tool:       tools[i],
			AvgMetrics: m,
		})
	}

	for _, metric := range metricNames() {
		var values []float64
		for _, ts := range cr.PerTool {
			values = append(values, getMetricValue(ts.AvgMetrics, metric))
		}
		cr.MetricMatrix[metric] = values
	}

	for metric, values := range cr.MetricMatrix {
		cr.Rankings[metric] = rankValues(values, lowerIsBetter[metric])
	}

	cr.AggregateScores = computeAggregates(cr.Tools, cr.Rankings)

	// Pairwise similarity
	n := len(traces)
	cr.PairwiseSimilarity = make([][]float64, n)
	for i := range cr.PairwiseSimilarity {
		cr.PairwiseSimilarity[i] = make([]float64, n)
		cr.PairwiseSimilarity[i][i] = 1.0
	}

	for i := 0; i < n; i++ {
		gi := trace.BuildGraph(traces[i])
		for j := i + 1; j < n; j++ {
			gj := trace.BuildGraph(traces[j])
			result := eval.Evaluate(gi, gj, nil)
			cr.PairwiseSimilarity[i][j] = result.OverallSimilarity
			cr.PairwiseSimilarity[j][i] = result.OverallSimilarity
		}
	}

	return cr
}

func getMetricValue(m *TraceMetrics, metric string) float64 {
	if m == nil {
		return 0
	}
	switch metric {
	case "total_duration_ms":
		return float64(m.TotalDurationMs)
	case "total_cost":
		return m.TotalCost
	case "total_input_tokens":
		return float64(m.TotalInputTokens)
	case "total_output_tokens":
		return float64(m.TotalOutputTokens)
	case "token_efficiency":
		return m.TokenEfficiency
	case "tool_call_count":
		return float64(m.ToolCallCount)
	case "llm_call_count":
		return float64(m.LLMCallCount)
	case "p50_latency_ms":
		return float64(m.P50LatencyMs)
	case "p95_latency_ms":
		return float64(m.P95LatencyMs)
	case "files_written":
		return float64(m.FilesWritten)
	case "pass_rate":
		return m.PassRate
	case "error_count":
		return float64(m.ErrorCount)
	case "pass_at_1":
		if m.PassAtK != nil {
			return m.PassAtK[1]
		}
		return 0
	default:
		return 0
	}
}

// rankValues assigns dense ranks (1 = best). Ties share the same rank.
func rankValues(values []float64, lowerBetter bool) []int {
	type indexed struct {
		idx int
		val float64
	}

	items := make([]indexed, len(values))
	for i, v := range values {
		items[i] = indexed{i, v}
	}

	sort.Slice(items, func(a, b int) bool {
		if lowerBetter {
			return items[a].val < items[b].val
		}
		return items[a].val > items[b].val
	})

	ranks := make([]int, len(values))
	rank := 1
	for i, item := range items {
		if i > 0 && items[i].val != items[i-1].val {
			rank = i + 1
		}
		ranks[item.idx] = rank
	}
	return ranks
}

func computeAggregates(tools []string, rankings map[string][]int) []AggregateScore {
	n := len(tools)
	scores := make([]AggregateScore, n)
	for i, tool := range tools {
		scores[i].Tool = tool
	}

	metricCount := 0
	for _, ranks := range rankings {
		metricCount++
		for i, rank := range ranks {
			scores[i].AvgRank += float64(rank)
			if rank == 1 {
				scores[i].WinCount++
			}
		}
	}

	if metricCount > 0 {
		for i := range scores {
			scores[i].AvgRank /= float64(metricCount)
		}
	}

	// Sort by average rank
	sort.Slice(scores, func(a, b int) bool {
		return scores[a].AvgRank < scores[b].AvgRank
	})

	return scores
}

func computePairwiseSimilarity(results *BenchmarkResults, pricing PricingTable) [][]float64 {
	n := len(results.ToolRuns)
	matrix := make([][]float64, n)
	for i := range matrix {
		matrix[i] = make([]float64, n)
		matrix[i][i] = 1.0
	}

	// Load first trace from each tool for comparison
	traces := make([]*trace.Trace, n)
	for i, toolRun := range results.ToolRuns {
		if len(toolRun.Runs) > 0 {
			tr, err := trace.ReadTrace(toolRun.Runs[0].TracePath)
			if err == nil {
				traces[i] = tr
			}
		}
	}

	for i := 0; i < n; i++ {
		if traces[i] == nil {
			continue
		}
		gi := trace.BuildGraph(traces[i])
		for j := i + 1; j < n; j++ {
			if traces[j] == nil {
				continue
			}
			gj := trace.BuildGraph(traces[j])
			result := eval.Evaluate(gi, gj, nil)
			matrix[i][j] = result.OverallSimilarity
			matrix[j][i] = result.OverallSimilarity
		}
	}

	return matrix
}
