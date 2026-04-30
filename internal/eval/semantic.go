package eval

import (
	"github.com/qiangli/aperio/internal/trace"
)

// SemanticEvalResult holds the results of semantic text comparison.
type SemanticEvalResult struct {
	PairScores []TextPairScore `json:"pair_scores"`
	Aggregate  TextMetrics     `json:"aggregate"`
}

// TextPairScore holds text similarity scores for a single matched pair.
type TextPairScore struct {
	LeftSpan  string      `json:"left_span"`
	RightSpan string      `json:"right_span"`
	SpanType  string      `json:"span_type"`
	Metrics   TextMetrics `json:"metrics"`
}

// TextMetrics holds individual text similarity metrics.
type TextMetrics struct {
	BLEU             float64 `json:"bleu"`
	ROUGE1           float64 `json:"rouge_1"`
	ROUGE2           float64 `json:"rouge_2"`
	ROUGEL           float64 `json:"rouge_l"`
	LevenshteinSim   float64 `json:"levenshtein"`
	CosineSimilarity float64 `json:"cosine_similarity"`
}

// Span types whose text content is worth comparing semantically.
var semanticSpanTypes = map[trace.SpanType]bool{
	trace.SpanAgentOutput: true,
	trace.SpanToolResult:  true,
	trace.SpanUserInput:   true,
}

// EvaluateSemantic compares text content of matched span pairs between two graphs.
// It uses the edit script from the structural evaluation to identify corresponding spans.
func EvaluateSemantic(left, right *trace.TraceGraph, editScript []EditOperation) *SemanticEvalResult {
	if left == nil || right == nil {
		return &SemanticEvalResult{}
	}

	// Build node maps by label for quick lookup
	leftNodes := buildContentMap(left)
	rightNodes := buildContentMap(right)

	var pairs []TextPairScore

	for _, op := range editScript {
		if op.Op != "match" && op.Op != "rename" {
			continue
		}

		leftLabel := op.NodeA
		rightLabel := op.NodeB
		if rightLabel == "" {
			rightLabel = leftLabel
		}

		// Check if this is a semantic span type
		spanType, _ := splitLabel(leftLabel)
		if !semanticSpanTypes[trace.SpanType(spanType)] {
			continue
		}

		leftContent := leftNodes[leftLabel]
		rightContent := rightNodes[rightLabel]

		if leftContent == "" && rightContent == "" {
			continue
		}

		metrics := computeTextMetrics(leftContent, rightContent)
		pairs = append(pairs, TextPairScore{
			LeftSpan:  leftLabel,
			RightSpan: rightLabel,
			SpanType:  spanType,
			Metrics:   metrics,
		})
	}

	result := &SemanticEvalResult{
		PairScores: pairs,
	}

	if len(pairs) > 0 {
		result.Aggregate = aggregateMetrics(pairs)
	}

	return result
}

// computeTextMetrics computes all text similarity metrics for a pair of texts.
func computeTextMetrics(a, b string) TextMetrics {
	return TextMetrics{
		BLEU:             BLEU(a, b),
		ROUGE1:           ROUGEN(a, b, 1),
		ROUGE2:           ROUGEN(a, b, 2),
		ROUGEL:           ROUGEL(a, b),
		LevenshteinSim:   Levenshtein(a, b),
		CosineSimilarity: CosineSimilarity(a, b),
	}
}

// aggregateMetrics computes average metrics across all pairs.
func aggregateMetrics(pairs []TextPairScore) TextMetrics {
	n := float64(len(pairs))
	var agg TextMetrics

	for _, p := range pairs {
		agg.BLEU += p.Metrics.BLEU
		agg.ROUGE1 += p.Metrics.ROUGE1
		agg.ROUGE2 += p.Metrics.ROUGE2
		agg.ROUGEL += p.Metrics.ROUGEL
		agg.LevenshteinSim += p.Metrics.LevenshteinSim
		agg.CosineSimilarity += p.Metrics.CosineSimilarity
	}

	agg.BLEU /= n
	agg.ROUGE1 /= n
	agg.ROUGE2 /= n
	agg.ROUGEL /= n
	agg.LevenshteinSim /= n
	agg.CosineSimilarity /= n

	return agg
}

// buildContentMap builds a map from span label to content text.
func buildContentMap(g *trace.TraceGraph) map[string]string {
	m := make(map[string]string)
	for _, node := range g.Nodes {
		if !semanticSpanTypes[node.Type] {
			continue
		}
		label := string(node.Type) + ":" + node.Name
		if content, ok := node.Attributes["content"]; ok {
			if s, ok := content.(string); ok {
				m[label] = s
			}
		}
	}
	return m
}
