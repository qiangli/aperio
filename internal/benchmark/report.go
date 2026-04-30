package benchmark

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// FormatText produces a human-readable text comparison report.
func FormatText(cr *ComparisonResult) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Benchmark Comparison Report\n")
	fmt.Fprintf(&b, "===========================\n\n")

	// Aggregate rankings
	fmt.Fprintf(&b, "Overall Rankings:\n")
	for i, score := range cr.AggregateScores {
		fmt.Fprintf(&b, "  #%d  %-20s  avg_rank=%.2f  wins=%d\n",
			i+1, score.Tool, score.AvgRank, score.WinCount)
	}
	fmt.Fprintf(&b, "\n")

	// Per-tool summary
	fmt.Fprintf(&b, "Per-Tool Metrics:\n")
	fmt.Fprintf(&b, "  %-20s %10s %10s %10s %10s %8s %8s %8s\n",
		"Tool", "Duration", "Cost", "InTokens", "OutTokens", "LLMCalls", "Tools", "Pass%")
	fmt.Fprintf(&b, "  %s\n", strings.Repeat("-", 96))

	for _, ts := range cr.PerTool {
		m := ts.AvgMetrics
		if m == nil {
			continue
		}
		fmt.Fprintf(&b, "  %-20s %8dms $%8.4f %10d %10d %8d %8d %7.0f%%\n",
			ts.Tool, m.TotalDurationMs, m.TotalCost,
			m.TotalInputTokens, m.TotalOutputTokens,
			m.LLMCallCount, m.ToolCallCount, m.PassRate*100)
	}
	fmt.Fprintf(&b, "\n")

	// Metric rankings
	fmt.Fprintf(&b, "Per-Metric Rankings:\n")
	metricNames := make([]string, 0, len(cr.Rankings))
	for k := range cr.Rankings {
		metricNames = append(metricNames, k)
	}
	sort.Strings(metricNames)

	for _, metric := range metricNames {
		ranks := cr.Rankings[metric]
		fmt.Fprintf(&b, "  %-22s", metric)
		for i, rank := range ranks {
			winner := ""
			if rank == 1 {
				winner = "*"
			}
			fmt.Fprintf(&b, " %s:#%d%s", cr.Tools[i], rank, winner)
		}
		fmt.Fprintf(&b, "\n")
	}
	fmt.Fprintf(&b, "\n")

	// Pairwise similarity
	if len(cr.PairwiseSimilarity) > 0 {
		fmt.Fprintf(&b, "Pairwise Similarity:\n")
		fmt.Fprintf(&b, "  %-15s", "")
		for _, tool := range cr.Tools {
			fmt.Fprintf(&b, " %10s", truncate(tool, 10))
		}
		fmt.Fprintf(&b, "\n")
		for i, tool := range cr.Tools {
			fmt.Fprintf(&b, "  %-15s", truncate(tool, 15))
			for j := range cr.Tools {
				fmt.Fprintf(&b, " %10.3f", cr.PairwiseSimilarity[i][j])
			}
			fmt.Fprintf(&b, "\n")
		}
	}

	return b.String()
}

// GenerateJSON writes the comparison result as JSON.
func GenerateJSON(cr *ComparisonResult, outputPath string) error {
	data, err := json.MarshalIndent(cr, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal comparison: %w", err)
	}
	return os.WriteFile(outputPath, data, 0644)
}

// GenerateCSV writes the comparison as a CSV table.
func GenerateCSV(cr *ComparisonResult, outputPath string) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create CSV: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	// Header
	header := []string{"Tool", "Duration(ms)", "Cost($)", "InputTokens", "OutputTokens",
		"TokenEfficiency", "LLMCalls", "ToolCalls", "FilesWritten", "PassRate", "Errors", "AvgRank", "Wins"}
	w.Write(header)

	// Find aggregate scores by tool
	aggMap := make(map[string]AggregateScore)
	for _, agg := range cr.AggregateScores {
		aggMap[agg.Tool] = agg
	}

	for _, ts := range cr.PerTool {
		m := ts.AvgMetrics
		if m == nil {
			continue
		}
		agg := aggMap[ts.Tool]
		row := []string{
			ts.Tool,
			fmt.Sprintf("%d", m.TotalDurationMs),
			fmt.Sprintf("%.6f", m.TotalCost),
			fmt.Sprintf("%d", m.TotalInputTokens),
			fmt.Sprintf("%d", m.TotalOutputTokens),
			fmt.Sprintf("%.3f", m.TokenEfficiency),
			fmt.Sprintf("%d", m.LLMCallCount),
			fmt.Sprintf("%d", m.ToolCallCount),
			fmt.Sprintf("%d", m.FilesWritten),
			fmt.Sprintf("%.2f", m.PassRate),
			fmt.Sprintf("%d", m.ErrorCount),
			fmt.Sprintf("%.2f", agg.AvgRank),
			fmt.Sprintf("%d", agg.WinCount),
		}
		w.Write(row)
	}

	return nil
}

// GenerateHTML writes a self-contained HTML benchmark report.
func GenerateHTML(cr *ComparisonResult, outputPath string) error {
	data, err := json.Marshal(cr)
	if err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}

	html := buildHTMLReport(string(data))
	return os.WriteFile(outputPath, []byte(html), 0644)
}

func buildHTMLReport(jsonData string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>Aperio Benchmark Report</title>
<script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js"></script>
<style>
body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; background: #1a1a2e; color: #e0e0e0; margin: 0; padding: 20px; }
h1, h2 { color: #4fc3f7; }
table { border-collapse: collapse; width: 100%%; margin: 20px 0; }
th, td { border: 1px solid #333; padding: 8px 12px; text-align: right; }
th { background: #16213e; color: #4fc3f7; }
td:first-child, th:first-child { text-align: left; }
tr:nth-child(even) { background: #16213e; }
.rank-1 { color: #4caf50; font-weight: bold; }
.rank-2 { color: #ffc107; }
.rank-3 { color: #ff5722; }
.chart-container { max-width: 800px; margin: 20px auto; background: #16213e; padding: 20px; border-radius: 8px; }
.section { margin: 30px 0; }
.winner { background: #1b5e20; }
</style>
</head>
<body>
<h1>Aperio Benchmark Report</h1>
<div id="content"></div>
<script>
var DATA = %s;

function render() {
  var el = document.getElementById('content');
  el.innerHTML = renderSummary() + renderTable() + renderCharts() + renderSimilarity();
}

function renderSummary() {
  var html = '<div class="section"><h2>Overall Rankings</h2><table><tr><th>Rank</th><th>Tool</th><th>Avg Rank</th><th>Wins</th></tr>';
  DATA.aggregate_scores.forEach(function(s, i) {
    var cls = i === 0 ? 'winner' : '';
    html += '<tr class="' + cls + '"><td>#' + (i+1) + '</td><td>' + s.tool + '</td><td>' + s.avg_rank.toFixed(2) + '</td><td>' + s.win_count + '</td></tr>';
  });
  return html + '</table></div>';
}

function renderTable() {
  var html = '<div class="section"><h2>Per-Tool Metrics</h2><table><tr><th>Tool</th><th>Duration(ms)</th><th>Cost($)</th><th>Input Tokens</th><th>Output Tokens</th><th>LLM Calls</th><th>Tool Calls</th><th>Pass Rate</th></tr>';
  DATA.per_tool.forEach(function(t) {
    var m = t.avg_metrics;
    html += '<tr><td>' + t.tool + '</td><td>' + m.total_duration_ms + '</td><td>' + m.total_cost.toFixed(4) + '</td><td>' + m.total_input_tokens + '</td><td>' + m.total_output_tokens + '</td><td>' + m.llm_call_count + '</td><td>' + m.tool_call_count + '</td><td>' + (m.pass_rate*100).toFixed(0) + '%%</td></tr>';
  });
  return html + '</table></div>';
}

function renderCharts() {
  return '<div class="section"><h2>Cost vs Speed</h2><div class="chart-container"><canvas id="scatter"></canvas></div></div>' +
         '<div class="section"><h2>Token Usage</h2><div class="chart-container"><canvas id="tokens"></canvas></div></div>';
}

function renderSimilarity() {
  if (!DATA.pairwise_similarity || DATA.pairwise_similarity.length === 0) return '';
  var html = '<div class="section"><h2>Pairwise Similarity</h2><table><tr><th></th>';
  DATA.tools.forEach(function(t) { html += '<th>' + t + '</th>'; });
  html += '</tr>';
  DATA.tools.forEach(function(t, i) {
    html += '<tr><td><strong>' + t + '</strong></td>';
    DATA.pairwise_similarity[i].forEach(function(v) {
      html += '<td>' + v.toFixed(3) + '</td>';
    });
    html += '</tr>';
  });
  return html + '</table></div>';
}

render();

// Render charts after DOM
setTimeout(function() {
  // Scatter: cost vs speed
  var scatterData = DATA.per_tool.map(function(t) {
    return {x: t.avg_metrics.total_duration_ms / 1000, y: t.avg_metrics.total_cost, label: t.tool};
  });
  new Chart(document.getElementById('scatter'), {
    type: 'scatter',
    data: { datasets: [{
      data: scatterData,
      backgroundColor: ['#4fc3f7','#ffc107','#4caf50','#ff5722','#9c27b0','#00bcd4'],
      pointRadius: 8
    }]},
    options: {
      scales: { x: {title: {display:true, text:'Duration (s)', color:'#ccc'}, ticks:{color:'#ccc'}}, y: {title: {display:true, text:'Cost ($)', color:'#ccc'}, ticks:{color:'#ccc'}} },
      plugins: { legend: {display: false}, tooltip: { callbacks: { label: function(ctx) { return scatterData[ctx.dataIndex].label + ': ' + ctx.parsed.x.toFixed(1) + 's, $' + ctx.parsed.y.toFixed(4); } } } }
    }
  });

  // Bar: tokens
  new Chart(document.getElementById('tokens'), {
    type: 'bar',
    data: {
      labels: DATA.tools,
      datasets: [
        {label: 'Input Tokens', data: DATA.per_tool.map(function(t){return t.avg_metrics.total_input_tokens;}), backgroundColor: '#4fc3f7'},
        {label: 'Output Tokens', data: DATA.per_tool.map(function(t){return t.avg_metrics.total_output_tokens;}), backgroundColor: '#ffc107'}
      ]
    },
    options: { scales: { y: {ticks:{color:'#ccc'}}, x: {ticks:{color:'#ccc'}} }, plugins: {legend:{labels:{color:'#ccc'}}} }
  });
}, 100);
</script>
</body>
</html>`, jsonData)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
