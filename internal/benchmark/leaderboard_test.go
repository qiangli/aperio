package benchmark

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadLeaderboardNonexistent(t *testing.T) {
	lb, err := LoadLeaderboard("/nonexistent/leaderboard.json")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if len(lb.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(lb.Entries))
	}
	if lb.BestScores == nil {
		t.Error("BestScores should be initialized")
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lb.json")

	lb := &Leaderboard{
		BestScores:  make(map[string]BestScore),
		LastUpdated: time.Now(),
	}
	lb.AddEntry(LeaderboardEntry{
		Timestamp: time.Now(),
		SpecName:  "test-spec",
		ToolResults: []LeaderboardToolResult{
			{
				Tool:     "tool-a",
				Metrics:  &TraceMetrics{TotalCost: 0.05, TotalDurationMs: 1000, PassRate: 1.0, ModelUsage: map[string]ModelMetrics{}},
				Rank:     1,
				WinCount: 3,
				AvgRank:  1.5,
			},
		},
	})

	if err := SaveLeaderboard(path, lb); err != nil {
		t.Fatalf("SaveLeaderboard: %v", err)
	}

	lb2, err := LoadLeaderboard(path)
	if err != nil {
		t.Fatalf("LoadLeaderboard: %v", err)
	}

	if len(lb2.Entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(lb2.Entries))
	}
	if lb2.Entries[0].SpecName != "test-spec" {
		t.Errorf("spec name: %s", lb2.Entries[0].SpecName)
	}
	if lb2.BestScores["tool-a"].BestCost != 0.05 {
		t.Errorf("best cost: %f", lb2.BestScores["tool-a"].BestCost)
	}
}

func TestAddEntryUpdatesBestScores(t *testing.T) {
	lb := &Leaderboard{BestScores: make(map[string]BestScore)}

	// First entry
	lb.AddEntry(LeaderboardEntry{
		Timestamp: time.Now(),
		SpecName:  "run-1",
		ToolResults: []LeaderboardToolResult{
			{Tool: "tool-a", Metrics: &TraceMetrics{TotalCost: 0.10, TotalDurationMs: 2000, PassRate: 0.8, ModelUsage: map[string]ModelMetrics{}}, AvgRank: 2.0},
		},
	})

	if lb.BestScores["tool-a"].BestCost != 0.10 {
		t.Errorf("initial best cost: %f", lb.BestScores["tool-a"].BestCost)
	}

	// Second entry with better cost but worse pass rate
	lb.AddEntry(LeaderboardEntry{
		Timestamp: time.Now(),
		SpecName:  "run-2",
		ToolResults: []LeaderboardToolResult{
			{Tool: "tool-a", Metrics: &TraceMetrics{TotalCost: 0.05, TotalDurationMs: 3000, PassRate: 0.5, ModelUsage: map[string]ModelMetrics{}}, AvgRank: 1.0},
		},
	})

	best := lb.BestScores["tool-a"]
	if best.BestCost != 0.05 {
		t.Errorf("best cost should improve: %f", best.BestCost)
	}
	if best.BestDuration != 2000 {
		t.Errorf("best duration should stay at 2000: %d", best.BestDuration)
	}
	if best.BestPassRate != 0.8 {
		t.Errorf("best pass rate should stay at 0.8: %f", best.BestPassRate)
	}
	if best.BestAvgRank != 1.0 {
		t.Errorf("best avg rank should improve to 1.0: %f", best.BestAvgRank)
	}
}

func TestClear(t *testing.T) {
	lb := &Leaderboard{BestScores: make(map[string]BestScore)}
	lb.AddEntry(LeaderboardEntry{
		Timestamp:   time.Now(),
		SpecName:    "run",
		ToolResults: []LeaderboardToolResult{{Tool: "a", Metrics: &TraceMetrics{TotalCost: 1, ModelUsage: map[string]ModelMetrics{}}, AvgRank: 1}},
	})

	lb.Clear()

	if len(lb.Entries) != 0 {
		t.Error("entries should be empty after clear")
	}
	if len(lb.BestScores) != 0 {
		t.Error("best scores should be empty after clear")
	}
}

func TestComparisonToEntry(t *testing.T) {
	cr := &ComparisonResult{
		Tools: []string{"tool-a", "tool-b"},
		PerTool: []ToolSummary{
			{Tool: "tool-a", AvgMetrics: &TraceMetrics{TotalCost: 0.05, TotalDurationMs: 1000, ModelUsage: map[string]ModelMetrics{}}},
			{Tool: "tool-b", AvgMetrics: &TraceMetrics{TotalCost: 0.10, TotalDurationMs: 2000, ModelUsage: map[string]ModelMetrics{}}},
		},
		AggregateScores: []AggregateScore{
			{Tool: "tool-a", AvgRank: 1.0, WinCount: 5},
			{Tool: "tool-b", AvgRank: 2.0, WinCount: 1},
		},
	}

	entry := ComparisonToEntry("test-benchmark", cr)

	if entry.SpecName != "test-benchmark" {
		t.Errorf("spec name: %s", entry.SpecName)
	}
	if len(entry.ToolResults) != 2 {
		t.Fatalf("expected 2 tool results, got %d", len(entry.ToolResults))
	}
	if entry.ToolResults[0].Tool != "tool-a" {
		t.Errorf("first tool: %s", entry.ToolResults[0].Tool)
	}
	if entry.ToolResults[0].WinCount != 5 {
		t.Errorf("win count: %d", entry.ToolResults[0].WinCount)
	}
}

func TestFormatLeaderboardEmpty(t *testing.T) {
	lb := &Leaderboard{BestScores: make(map[string]BestScore)}
	text := FormatLeaderboard(lb)
	if text == "" {
		t.Error("expected non-empty output")
	}
	if !containsStr(text, "No benchmark entries") {
		t.Error("expected empty message")
	}
}

func TestFormatLeaderboardWithEntries(t *testing.T) {
	lb := &Leaderboard{BestScores: make(map[string]BestScore)}
	lb.AddEntry(LeaderboardEntry{
		Timestamp: time.Now(),
		SpecName:  "fibonacci",
		ToolResults: []LeaderboardToolResult{
			{Tool: "ycode", Metrics: &TraceMetrics{TotalCost: 0.05, TotalDurationMs: 1000, PassRate: 1.0, ModelUsage: map[string]ModelMetrics{}}, AvgRank: 1.0, WinCount: 3, Rank: 1},
			{Tool: "aider", Metrics: &TraceMetrics{TotalCost: 0.10, TotalDurationMs: 2000, PassRate: 0.8, ModelUsage: map[string]ModelMetrics{}}, AvgRank: 2.0, WinCount: 1, Rank: 2},
		},
	})

	text := FormatLeaderboard(lb)
	if !containsStr(text, "ycode") {
		t.Error("expected ycode in output")
	}
	if !containsStr(text, "aider") {
		t.Error("expected aider in output")
	}
	if !containsStr(text, "All-Time Best") {
		t.Error("expected best scores section")
	}
	if !containsStr(text, "fibonacci") {
		t.Error("expected spec name")
	}
}

func TestSaveLeaderboardCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	path := filepath.Join(dir, "lb.json")

	lb := &Leaderboard{BestScores: make(map[string]BestScore)}
	if err := SaveLeaderboard(path, lb); err != nil {
		t.Fatalf("SaveLeaderboard should create dir: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("file should exist: %v", err)
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
