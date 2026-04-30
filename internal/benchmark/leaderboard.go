package benchmark

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Leaderboard accumulates benchmark results across multiple runs.
type Leaderboard struct {
	Entries     []LeaderboardEntry   `json:"entries"`
	BestScores  map[string]BestScore `json:"best_scores"`
	LastUpdated time.Time            `json:"last_updated"`
}

// LeaderboardEntry records a single benchmark run's results.
type LeaderboardEntry struct {
	Timestamp   time.Time               `json:"timestamp"`
	SpecName    string                  `json:"spec_name"`
	ToolResults []LeaderboardToolResult `json:"tool_results"`
}

// LeaderboardToolResult holds one tool's results within an entry.
type LeaderboardToolResult struct {
	Tool     string        `json:"tool"`
	Metrics  *TraceMetrics `json:"metrics"`
	Rank     int           `json:"rank"`
	WinCount int           `json:"win_count"`
	AvgRank  float64       `json:"avg_rank"`
}

// BestScore tracks the best-ever results for a tool.
type BestScore struct {
	Tool         string    `json:"tool"`
	BestCost     float64   `json:"best_cost"`
	BestDuration int64     `json:"best_duration_ms"`
	BestPassRate float64   `json:"best_pass_rate"`
	BestAvgRank  float64   `json:"best_avg_rank"`
	RecordedAt   time.Time `json:"recorded_at"`
}

// LoadLeaderboard reads a leaderboard from a JSON file.
// Returns an empty leaderboard if the file doesn't exist.
func LoadLeaderboard(path string) (*Leaderboard, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Leaderboard{BestScores: make(map[string]BestScore)}, nil
		}
		return nil, fmt.Errorf("read leaderboard: %w", err)
	}

	var lb Leaderboard
	if err := json.Unmarshal(data, &lb); err != nil {
		return nil, fmt.Errorf("parse leaderboard: %w", err)
	}
	if lb.BestScores == nil {
		lb.BestScores = make(map[string]BestScore)
	}
	return &lb, nil
}

// SaveLeaderboard writes the leaderboard to a JSON file atomically.
func SaveLeaderboard(path string, lb *Leaderboard) error {
	data, err := json.MarshalIndent(lb, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal leaderboard: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "aperio-lb-*.json")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}

	return nil
}

// AddEntry appends a benchmark entry and updates best scores.
func (lb *Leaderboard) AddEntry(entry LeaderboardEntry) {
	lb.Entries = append(lb.Entries, entry)
	lb.LastUpdated = time.Now()

	for _, tr := range entry.ToolResults {
		best, exists := lb.BestScores[tr.Tool]
		if !exists {
			lb.BestScores[tr.Tool] = BestScore{
				Tool:         tr.Tool,
				BestCost:     tr.Metrics.TotalCost,
				BestDuration: tr.Metrics.TotalDurationMs,
				BestPassRate: tr.Metrics.PassRate,
				BestAvgRank:  tr.AvgRank,
				RecordedAt:   entry.Timestamp,
			}
			continue
		}

		updated := false
		if tr.Metrics.TotalCost < best.BestCost || best.BestCost == 0 {
			best.BestCost = tr.Metrics.TotalCost
			updated = true
		}
		if tr.Metrics.TotalDurationMs < best.BestDuration || best.BestDuration == 0 {
			best.BestDuration = tr.Metrics.TotalDurationMs
			updated = true
		}
		if tr.Metrics.PassRate > best.BestPassRate {
			best.BestPassRate = tr.Metrics.PassRate
			updated = true
		}
		if tr.AvgRank < best.BestAvgRank || best.BestAvgRank == 0 {
			best.BestAvgRank = tr.AvgRank
			updated = true
		}
		if updated {
			best.RecordedAt = entry.Timestamp
			lb.BestScores[tr.Tool] = best
		}
	}
}

// Clear resets the leaderboard.
func (lb *Leaderboard) Clear() {
	lb.Entries = nil
	lb.BestScores = make(map[string]BestScore)
	lb.LastUpdated = time.Now()
}

// ComparisonToEntry converts a ComparisonResult into a LeaderboardEntry.
func ComparisonToEntry(specName string, cr *ComparisonResult) LeaderboardEntry {
	entry := LeaderboardEntry{
		Timestamp: time.Now(),
		SpecName:  specName,
	}

	// Build aggregate score map for lookup
	aggMap := make(map[string]AggregateScore)
	for _, agg := range cr.AggregateScores {
		aggMap[agg.Tool] = agg
	}

	for i, ts := range cr.PerTool {
		agg := aggMap[ts.Tool]
		entry.ToolResults = append(entry.ToolResults, LeaderboardToolResult{
			Tool:     ts.Tool,
			Metrics:  ts.AvgMetrics,
			Rank:     i + 1,
			WinCount: agg.WinCount,
			AvgRank:  agg.AvgRank,
		})
	}

	return entry
}

// FormatLeaderboard produces a human-readable text report.
func FormatLeaderboard(lb *Leaderboard) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Aperio Benchmark Leaderboard\n")
	fmt.Fprintf(&b, "============================\n")

	if len(lb.Entries) == 0 {
		fmt.Fprintf(&b, "\nNo benchmark entries yet. Run 'aperio benchmark' to add results.\n")
		return b.String()
	}

	fmt.Fprintf(&b, "Total runs: %d | Last updated: %s\n\n",
		len(lb.Entries), lb.LastUpdated.Format("2006-01-02 15:04:05"))

	// All-time best scores
	if len(lb.BestScores) > 0 {
		fmt.Fprintf(&b, "All-Time Best Scores:\n")
		fmt.Fprintf(&b, "  %-20s %12s %12s %10s %10s\n",
			"Tool", "Best Cost", "Best Time", "Best Pass", "Best Rank")
		fmt.Fprintf(&b, "  %s\n", strings.Repeat("-", 68))

		for _, best := range lb.BestScores {
			fmt.Fprintf(&b, "  %-20s $%10.4f %10dms %9.0f%% %10.2f\n",
				best.Tool, best.BestCost, best.BestDuration,
				best.BestPassRate*100, best.BestAvgRank)
		}
		fmt.Fprintf(&b, "\n")
	}

	// Recent entries (last 10)
	fmt.Fprintf(&b, "Recent Entries:\n")
	start := 0
	if len(lb.Entries) > 10 {
		start = len(lb.Entries) - 10
	}

	for i := len(lb.Entries) - 1; i >= start; i-- {
		entry := lb.Entries[i]
		fmt.Fprintf(&b, "  [%s] %s\n",
			entry.Timestamp.Format("2006-01-02 15:04"), entry.SpecName)
		for _, tr := range entry.ToolResults {
			indicator := " "
			if tr.Rank == 1 {
				indicator = "*"
			}
			fmt.Fprintf(&b, "    %s %-18s rank=%.1f wins=%d cost=$%.4f time=%dms pass=%.0f%%\n",
				indicator, tr.Tool, tr.AvgRank, tr.WinCount,
				tr.Metrics.TotalCost, tr.Metrics.TotalDurationMs,
				tr.Metrics.PassRate*100)
		}
	}

	return b.String()
}
