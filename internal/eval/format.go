package eval

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// FormatText returns a human-readable text representation of the evaluation result.
func FormatText(r *EvalResult, verbose bool) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Trace Similarity Evaluation\n")
	fmt.Fprintf(&b, "===========================\n")
	fmt.Fprintf(&b, "Overall Similarity:    %.3f\n", r.OverallSimilarity)
	fmt.Fprintf(&b, "Structural Similarity: %.3f\n", r.StructuralSimilarity)
	fmt.Fprintf(&b, "Behavioral Similarity: %.3f\n", r.BehavioralSimilarity)
	fmt.Fprintf(&b, "\n")

	if len(r.TypeBreakdown) > 0 {
		fmt.Fprintf(&b, "Type Breakdown:\n")

		// Sort types for stable output
		types := make([]string, 0, len(r.TypeBreakdown))
		for t := range r.TypeBreakdown {
			types = append(types, t)
		}
		sort.Strings(types)

		for _, t := range types {
			tr := r.TypeBreakdown[t]
			fmt.Fprintf(&b, "  %-16s %d/%d matched, %d inserted, %d deleted, %d renamed  (%.3f)\n",
				t+":", tr.Matched, max(tr.LeftCount, tr.RightCount),
				tr.Inserted, tr.Deleted, tr.Renamed, tr.Similarity)
		}
		fmt.Fprintf(&b, "\n")
	}

	fmt.Fprintf(&b, "Edit Distance: %.2f (max possible: %.2f)\n", r.EditDistance, r.MaxDistance)

	if r.SemanticScores != nil && len(r.SemanticScores.PairScores) > 0 {
		fmt.Fprintf(&b, "\nSemantic Similarity:\n")
		agg := r.SemanticScores.Aggregate
		fmt.Fprintf(&b, "  BLEU:       %.3f\n", agg.BLEU)
		fmt.Fprintf(&b, "  ROUGE-1:    %.3f\n", agg.ROUGE1)
		fmt.Fprintf(&b, "  ROUGE-2:    %.3f\n", agg.ROUGE2)
		fmt.Fprintf(&b, "  ROUGE-L:    %.3f\n", agg.ROUGEL)
		fmt.Fprintf(&b, "  Levenshtein: %.3f\n", agg.LevenshteinSim)
		fmt.Fprintf(&b, "  Cosine:     %.3f\n", agg.CosineSimilarity)

		if verbose {
			fmt.Fprintf(&b, "\n  Per-Pair Scores (%d pairs):\n", len(r.SemanticScores.PairScores))
			for _, p := range r.SemanticScores.PairScores {
				fmt.Fprintf(&b, "    %s: BLEU=%.3f ROUGE-L=%.3f Cosine=%.3f\n",
					p.LeftSpan, p.Metrics.BLEU, p.Metrics.ROUGEL, p.Metrics.CosineSimilarity)
			}
		}
	}

	if verbose && len(r.EditScript) > 0 {
		fmt.Fprintf(&b, "\nEdit Script (%d operations):\n", len(r.EditScript))
		for _, op := range r.EditScript {
			symbol := " "
			switch op.Op {
			case "delete":
				symbol = "-"
			case "insert":
				symbol = "+"
			case "rename":
				symbol = "~"
			case "match":
				symbol = "="
			}
			fmt.Fprintf(&b, "  %s %s (cost: %.2f)\n", symbol, op.Detail, op.Cost)
		}
	}

	return b.String()
}

// FormatJSON returns the evaluation result as formatted JSON.
func FormatJSON(r *EvalResult) (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal eval result: %w", err)
	}
	return string(data), nil
}

