package benchmark

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"
)

// RegressionConfig controls regression checking behavior.
type RegressionConfig struct {
	Mode             string                `yaml:"mode" json:"mode"` // "nightly", "ci", "manual"
	Trials           int                   `yaml:"trials,omitempty" json:"trials,omitempty"`
	TrustFilter      *TrustFilter          `yaml:"trust_filter,omitempty" json:"trust_filter,omitempty"`
	BaselinePath     string                `yaml:"baseline_path,omitempty" json:"baseline_path,omitempty"`
	FailOnRegression bool                  `yaml:"fail_on_regression,omitempty" json:"fail_on_regression,omitempty"`
	Thresholds       *RegressionThresholds `yaml:"thresholds,omitempty" json:"thresholds,omitempty"`
}

// RegressionThresholds configures per-metric thresholds for regression detection.
type RegressionThresholds struct {
	TotalDurationMs float64 `yaml:"total_duration_ms" json:"total_duration_ms"`
	TotalCost       float64 `yaml:"total_cost" json:"total_cost"`
	PassRate        float64 `yaml:"pass_rate" json:"pass_rate"`
	ErrorCount      float64 `yaml:"error_count" json:"error_count"`
	P95LatencyMs    float64 `yaml:"p95_latency_ms" json:"p95_latency_ms"`
}

// DefaultThresholds returns the default regression thresholds.
func DefaultThresholds() *RegressionThresholds {
	return &RegressionThresholds{
		TotalDurationMs: 0.20,
		TotalCost:       0.15,
		PassRate:        0.05,
		ErrorCount:      0.50,
		P95LatencyMs:    0.25,
	}
}

func (rt *RegressionThresholds) get(metric string) float64 {
	switch metric {
	case "total_duration_ms":
		return rt.TotalDurationMs
	case "total_cost":
		return rt.TotalCost
	case "pass_rate":
		return rt.PassRate
	case "error_count":
		return rt.ErrorCount
	case "p95_latency_ms":
		return rt.P95LatencyMs
	default:
		return 0.20
	}
}

// TrustFilter implements the 60/80 rule: tasks must pass at least MinPassRate
// to be considered in aggregate metrics. Tasks above MaxPassRate are "always passes"
// and can serve as baseline sanity checks.
type TrustFilter struct {
	MinPassRate float64 `yaml:"min_pass_rate" json:"min_pass_rate"` // e.g., 0.6
	MaxPassRate float64 `yaml:"max_pass_rate" json:"max_pass_rate"` // e.g., 0.8
}

// RegressionResult holds the outcome of a regression check.
type RegressionResult struct {
	Regressed bool               `json:"regressed"`
	Details   []RegressionDetail `json:"details"`
}

// RegressionDetail describes a single metric regression.
type RegressionDetail struct {
	Tool          string  `json:"tool"`
	Metric        string  `json:"metric"`
	Baseline      float64 `json:"baseline"`
	Current       float64 `json:"current"`
	DeltaPct      float64 `json:"delta_pct"` // percentage change (positive = worse)
	Regressed     bool    `json:"regressed"`
	ConfidencePct float64 `json:"confidence_pct,omitempty"` // confidence level of regression detection
}

// Baseline is a snapshot of comparison results used for regression checking.
type Baseline struct {
	PerTool map[string]*TraceMetrics `json:"per_tool"`
}


// CheckRegression compares current results against a saved baseline.
func CheckRegression(current *ComparisonResult, baselinePath string, config *RegressionConfig) (*RegressionResult, error) {
	baseline, err := LoadBaseline(baselinePath)
	if err != nil {
		return nil, fmt.Errorf("load baseline: %w", err)
	}

	thresholds := DefaultThresholds()
	if config != nil && config.Thresholds != nil {
		thresholds = config.Thresholds
	}

	result := &RegressionResult{}

	for _, ts := range current.PerTool {
		baselineMetrics, ok := baseline.PerTool[ts.Tool]
		if !ok {
			continue // New tool, no baseline to compare against
		}

		details := compareMetrics(ts.Tool, ts.AvgMetrics, baselineMetrics, thresholds)
		for _, d := range details {
			if d.Regressed {
				result.Regressed = true
			}
			result.Details = append(result.Details, d)
		}
	}

	return result, nil
}

func compareMetrics(tool string, current, baseline *TraceMetrics, thresholds *RegressionThresholds) []RegressionDetail {
	var details []RegressionDetail

	checks := []struct {
		metric      string
		current     float64
		baseline    float64
		lowerBetter bool
	}{
		{"total_duration_ms", float64(current.TotalDurationMs), float64(baseline.TotalDurationMs), true},
		{"total_cost", current.TotalCost, baseline.TotalCost, true},
		{"pass_rate", current.PassRate, baseline.PassRate, false},
		{"error_count", float64(current.ErrorCount), float64(baseline.ErrorCount), true},
		{"p95_latency_ms", float64(current.P95LatencyMs), float64(baseline.P95LatencyMs), true},
	}

	for _, c := range checks {
		if c.baseline == 0 {
			continue
		}

		var deltaPct float64
		if c.lowerBetter {
			deltaPct = (c.current - c.baseline) / c.baseline
		} else {
			deltaPct = (c.baseline - c.current) / c.baseline
		}

		threshold := thresholds.get(c.metric)
		regressed := deltaPct > threshold

		// Special case: pass_rate uses absolute difference
		if c.metric == "pass_rate" {
			regressed = (c.baseline - c.current) > threshold
			deltaPct = c.baseline - c.current
		}

		details = append(details, RegressionDetail{
			Tool:      tool,
			Metric:    c.metric,
			Baseline:  c.baseline,
			Current:   c.current,
			DeltaPct:  deltaPct,
			Regressed: regressed,
		})
	}

	return details
}

// SaveBaseline persists current results as the new baseline.
func SaveBaseline(path string, result *ComparisonResult) error {
	baseline := &Baseline{
		PerTool: make(map[string]*TraceMetrics),
	}
	for _, ts := range result.PerTool {
		baseline.PerTool[ts.Tool] = ts.AvgMetrics
	}

	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal baseline: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// LoadBaseline reads a baseline from disk.
func LoadBaseline(path string) (*Baseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var baseline Baseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return nil, fmt.Errorf("parse baseline: %w", err)
	}
	return &baseline, nil
}

// ApplyTrustFilter removes tasks whose pass rate falls outside the trust window.
// Tasks below MinPassRate are too flaky to trust.
// Tasks above MaxPassRate are "always passes" (useful for baseline sanity but not
// for measuring improvement).
func ApplyTrustFilter(passRates []float64, filter *TrustFilter) []float64 {
	if filter == nil {
		return passRates
	}

	var filtered []float64
	for _, rate := range passRates {
		if rate >= filter.MinPassRate && rate <= filter.MaxPassRate {
			filtered = append(filtered, rate)
		}
	}
	return filtered
}

// AggregatePassRates computes an average pass rate from filtered rates.
func AggregatePassRates(rates []float64) float64 {
	if len(rates) == 0 {
		return 0
	}
	sum := 0.0
	for _, r := range rates {
		sum += r
	}
	return math.Round(sum/float64(len(rates))*1000) / 1000
}

// BaselineHistory stores multiple baselines over time for trend analysis.
type BaselineHistory struct {
	Entries []HistoricalBaseline `json:"entries"`
}

// HistoricalBaseline is a timestamped baseline snapshot.
type HistoricalBaseline struct {
	Timestamp string                   `json:"timestamp"` // ISO 8601
	PerTool   map[string]*TraceMetrics `json:"per_tool"`
}

// AppendBaseline adds the current result to a baseline history file.
func AppendBaseline(historyPath string, result *ComparisonResult) error {
	var history BaselineHistory

	if data, err := os.ReadFile(historyPath); err == nil {
		json.Unmarshal(data, &history)
	}

	entry := HistoricalBaseline{
		Timestamp: time.Now().Format(time.RFC3339),
		PerTool:   make(map[string]*TraceMetrics),
	}
	for _, ts := range result.PerTool {
		entry.PerTool[ts.Tool] = ts.AvgMetrics
	}
	history.Entries = append(history.Entries, entry)

	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal history: %w", err)
	}

	dir := filepath.Dir(historyPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	return os.WriteFile(historyPath, data, 0644)
}

// CheckRegressionWithHistory compares current results against historical baselines,
// detecting sustained degradation patterns rather than one-off noise.
func CheckRegressionWithHistory(current *ComparisonResult, historyPath string, config *RegressionConfig) (*RegressionResult, error) {
	data, err := os.ReadFile(historyPath)
	if err != nil {
		return nil, fmt.Errorf("read history: %w", err)
	}

	var history BaselineHistory
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, fmt.Errorf("parse history: %w", err)
	}

	if len(history.Entries) == 0 {
		return &RegressionResult{}, nil
	}

	thresholds := DefaultThresholds()
	if config != nil && config.Thresholds != nil {
		thresholds = config.Thresholds
	}

	result := &RegressionResult{}

	for _, ts := range current.PerTool {
		// Collect historical values for each metric
		var durations, costs, passRates, errors, p95s []float64
		for _, entry := range history.Entries {
			if m, ok := entry.PerTool[ts.Tool]; ok {
				durations = append(durations, float64(m.TotalDurationMs))
				costs = append(costs, m.TotalCost)
				passRates = append(passRates, m.PassRate)
				errors = append(errors, float64(m.ErrorCount))
				p95s = append(p95s, float64(m.P95LatencyMs))
			}
		}

		if len(durations) < 2 {
			continue // Not enough history for trend analysis
		}

		// Compare current values against historical mean + stddev
		metricChecks := []struct {
			metric      string
			current     float64
			historical  []float64
			lowerBetter bool
		}{
			{"total_duration_ms", float64(ts.AvgMetrics.TotalDurationMs), durations, true},
			{"total_cost", ts.AvgMetrics.TotalCost, costs, true},
			{"pass_rate", ts.AvgMetrics.PassRate, passRates, false},
			{"error_count", float64(ts.AvgMetrics.ErrorCount), errors, true},
			{"p95_latency_ms", float64(ts.AvgMetrics.P95LatencyMs), p95s, true},
		}

		for _, mc := range metricChecks {
			mean, stddev := meanStddev(mc.historical)
			if mean == 0 {
				continue
			}

			threshold := thresholds.get(mc.metric)
			var deltaPct float64
			if mc.lowerBetter {
				deltaPct = (mc.current - mean) / mean
			} else {
				deltaPct = (mean - mc.current) / mean
			}

			regressed := deltaPct > threshold

			// Confidence: how many stddevs away from mean
			var confidence float64
			if stddev > 0 {
				if mc.lowerBetter {
					confidence = (mc.current - mean) / stddev
				} else {
					confidence = (mean - mc.current) / stddev
				}
				// Convert to rough percentage (2 stddev ~= 95% confidence)
				confidence = math.Min(confidence/2.0*100, 99.0)
			}

			if mc.metric == "pass_rate" {
				regressed = (mean - mc.current) > threshold
				deltaPct = mean - mc.current
			}

			if regressed {
				result.Regressed = true
			}

			result.Details = append(result.Details, RegressionDetail{
				Tool:          ts.Tool,
				Metric:        mc.metric,
				Baseline:      mean,
				Current:       mc.current,
				DeltaPct:      deltaPct,
				Regressed:     regressed,
				ConfidencePct: math.Max(0, confidence),
			})
		}
	}

	return result, nil
}

func meanStddev(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}

	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))

	if len(values) < 2 {
		return mean, 0
	}

	varSum := 0.0
	for _, v := range values {
		diff := v - mean
		varSum += diff * diff
	}
	stddev := math.Sqrt(varSum / float64(len(values)-1))

	return mean, stddev
}

// ComputeConfidenceInterval computes a confidence interval for a set of values.
// Uses t-distribution approximation for small samples.
func ComputeConfidenceInterval(values []float64, confidenceLevel float64) (lower, upper float64) {
	mean, stddev := meanStddev(values)
	n := float64(len(values))
	if n < 2 {
		return mean, mean
	}

	// Approximate z-score for common confidence levels
	var z float64
	switch {
	case confidenceLevel >= 0.99:
		z = 2.576
	case confidenceLevel >= 0.95:
		z = 1.96
	case confidenceLevel >= 0.90:
		z = 1.645
	default:
		z = 1.96
	}

	margin := z * stddev / math.Sqrt(n)
	return mean - margin, mean + margin
}
