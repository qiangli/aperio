package benchmark

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckRegression(t *testing.T) {
	// Create a baseline
	baseline := &Baseline{
		PerTool: map[string]*TraceMetrics{
			"tool-a": {
				TotalDurationMs: 1000,
				TotalCost:       0.10,
				PassRate:        0.90,
				ErrorCount:      2,
				P95LatencyMs:    500,
			},
		},
	}

	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "baseline.json")
	data, _ := json.MarshalIndent(baseline, "", "  ")
	os.WriteFile(baselinePath, data, 0644)

	tests := []struct {
		name       string
		current    *ComparisonResult
		regressed  bool
	}{
		{
			name: "no regression",
			current: &ComparisonResult{
				PerTool: []ToolSummary{{
					Tool: "tool-a",
					AvgMetrics: &TraceMetrics{
						TotalDurationMs: 1050, // 5% slower (under 20% threshold)
						TotalCost:       0.105,
						PassRate:        0.88, // 2% drop (under 5% threshold)
						ErrorCount:      2,
						P95LatencyMs:    520,
					},
				}},
			},
			regressed: false,
		},
		{
			name: "duration regression",
			current: &ComparisonResult{
				PerTool: []ToolSummary{{
					Tool: "tool-a",
					AvgMetrics: &TraceMetrics{
						TotalDurationMs: 1300, // 30% slower (over 20% threshold)
						TotalCost:       0.10,
						PassRate:        0.90,
						ErrorCount:      2,
						P95LatencyMs:    500,
					},
				}},
			},
			regressed: true,
		},
		{
			name: "pass rate regression",
			current: &ComparisonResult{
				PerTool: []ToolSummary{{
					Tool: "tool-a",
					AvgMetrics: &TraceMetrics{
						TotalDurationMs: 1000,
						TotalCost:       0.10,
						PassRate:        0.80, // 10% absolute drop (over 5% threshold)
						ErrorCount:      2,
						P95LatencyMs:    500,
					},
				}},
			},
			regressed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CheckRegression(tt.current, baselinePath, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Regressed != tt.regressed {
				t.Errorf("Regressed = %v, want %v\nDetails: %+v", result.Regressed, tt.regressed, result.Details)
			}
		})
	}
}

func TestApplyTrustFilter(t *testing.T) {
	rates := []float64{0.3, 0.5, 0.65, 0.75, 0.85, 0.95}
	filter := &TrustFilter{MinPassRate: 0.6, MaxPassRate: 0.8}

	filtered := ApplyTrustFilter(rates, filter)

	if len(filtered) != 2 {
		t.Errorf("expected 2 rates after filtering, got %d: %v", len(filtered), filtered)
	}
	if filtered[0] != 0.65 || filtered[1] != 0.75 {
		t.Errorf("unexpected filtered values: %v", filtered)
	}
}

func TestSaveAndLoadBaseline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "baseline.json")

	result := &ComparisonResult{
		PerTool: []ToolSummary{
			{Tool: "a", AvgMetrics: &TraceMetrics{TotalCost: 1.5, PassRate: 0.9}},
			{Tool: "b", AvgMetrics: &TraceMetrics{TotalCost: 2.0, PassRate: 0.8}},
		},
	}

	if err := SaveBaseline(path, result); err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}

	loaded, err := LoadBaseline(path)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}

	if loaded.PerTool["a"].TotalCost != 1.5 {
		t.Errorf("tool-a cost = %v, want 1.5", loaded.PerTool["a"].TotalCost)
	}
	if loaded.PerTool["b"].PassRate != 0.8 {
		t.Errorf("tool-b pass_rate = %v, want 0.8", loaded.PerTool["b"].PassRate)
	}
}
