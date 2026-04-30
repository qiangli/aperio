package eval

import (
	"math"
	"testing"
	"time"

	"github.com/qiangli/aperio/internal/trace"
)

func TestEvaluateSemanticIdentical(t *testing.T) {
	now := time.Now()
	g := makeTestGraph(
		[]trace.GraphNode{
			{ID: "o1", Type: trace.SpanAgentOutput, Name: "response",
				StartTime: now, EndTime: now.Add(time.Second),
				Attributes: map[string]any{"content": "The weather is 72°F and sunny."}},
		},
		nil,
	)

	editScript := []EditOperation{
		{Op: "match", NodeA: "AGENT_OUTPUT:response", NodeB: "AGENT_OUTPUT:response"},
	}

	result := EvaluateSemantic(g, g, editScript)

	if len(result.PairScores) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(result.PairScores))
	}

	m := result.PairScores[0].Metrics
	if math.Abs(m.BLEU-1.0) > 0.01 {
		t.Errorf("BLEU: expected ~1.0, got %f", m.BLEU)
	}
	if math.Abs(m.ROUGEL-1.0) > 0.01 {
		t.Errorf("ROUGE-L: expected ~1.0, got %f", m.ROUGEL)
	}
	if math.Abs(m.CosineSimilarity-1.0) > 0.01 {
		t.Errorf("Cosine: expected ~1.0, got %f", m.CosineSimilarity)
	}
}

func TestEvaluateSemanticDifferent(t *testing.T) {
	now := time.Now()
	left := makeTestGraph(
		[]trace.GraphNode{
			{ID: "o1", Type: trace.SpanAgentOutput, Name: "response",
				StartTime: now, EndTime: now.Add(time.Second),
				Attributes: map[string]any{"content": "The weather in NYC is 72°F and sunny."}},
		},
		nil,
	)
	right := makeTestGraph(
		[]trace.GraphNode{
			{ID: "o1", Type: trace.SpanAgentOutput, Name: "response",
				StartTime: now, EndTime: now.Add(time.Second),
				Attributes: map[string]any{"content": "It is currently raining in London at 55°F."}},
		},
		nil,
	)

	editScript := []EditOperation{
		{Op: "match", NodeA: "AGENT_OUTPUT:response", NodeB: "AGENT_OUTPUT:response"},
	}

	result := EvaluateSemantic(left, right, editScript)

	if len(result.PairScores) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(result.PairScores))
	}

	m := result.PairScores[0].Metrics
	if m.BLEU >= 1.0 {
		t.Errorf("BLEU should be < 1.0 for different texts, got %f", m.BLEU)
	}
	if m.ROUGEL >= 1.0 {
		t.Errorf("ROUGE-L should be < 1.0, got %f", m.ROUGEL)
	}
}

func TestEvaluateSemanticSkipsNonSemanticTypes(t *testing.T) {
	editScript := []EditOperation{
		{Op: "match", NodeA: "FUNCTION:main", NodeB: "FUNCTION:main"},
		{Op: "match", NodeA: "LLM_REQUEST:api", NodeB: "LLM_REQUEST:api"},
	}

	g := makeTestGraph(nil, nil)
	result := EvaluateSemantic(g, g, editScript)

	if len(result.PairScores) != 0 {
		t.Errorf("expected 0 pairs for non-semantic types, got %d", len(result.PairScores))
	}
}

func TestEvaluateSemanticNilGraphs(t *testing.T) {
	result := EvaluateSemantic(nil, nil, nil)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.PairScores) != 0 {
		t.Errorf("expected 0 pairs, got %d", len(result.PairScores))
	}
}

func TestEvaluateSemanticMultiplePairs(t *testing.T) {
	now := time.Now()
	g := makeTestGraph(
		[]trace.GraphNode{
			{ID: "u1", Type: trace.SpanUserInput, Name: "input",
				StartTime: now, EndTime: now.Add(time.Second),
				Attributes: map[string]any{"content": "What is the weather?"}},
			{ID: "o1", Type: trace.SpanAgentOutput, Name: "output",
				StartTime: now, EndTime: now.Add(time.Second),
				Attributes: map[string]any{"content": "It is sunny."}},
		},
		nil,
	)

	editScript := []EditOperation{
		{Op: "match", NodeA: "USER_INPUT:input", NodeB: "USER_INPUT:input"},
		{Op: "match", NodeA: "AGENT_OUTPUT:output", NodeB: "AGENT_OUTPUT:output"},
	}

	result := EvaluateSemantic(g, g, editScript)

	if len(result.PairScores) != 2 {
		t.Fatalf("expected 2 pairs, got %d", len(result.PairScores))
	}

	// Aggregate should be average of identical pairs (all 1.0)
	if math.Abs(result.Aggregate.BLEU-1.0) > 0.01 {
		t.Errorf("aggregate BLEU: expected ~1.0, got %f", result.Aggregate.BLEU)
	}
}

func TestAggregateMetrics(t *testing.T) {
	pairs := []TextPairScore{
		{Metrics: TextMetrics{BLEU: 0.8, ROUGE1: 0.9, ROUGE2: 0.7, ROUGEL: 0.85, LevenshteinSim: 0.75, CosineSimilarity: 0.9}},
		{Metrics: TextMetrics{BLEU: 0.6, ROUGE1: 0.7, ROUGE2: 0.5, ROUGEL: 0.65, LevenshteinSim: 0.55, CosineSimilarity: 0.7}},
	}

	agg := aggregateMetrics(pairs)
	if math.Abs(agg.BLEU-0.7) > 0.01 {
		t.Errorf("aggregate BLEU: expected 0.7, got %f", agg.BLEU)
	}
	if math.Abs(agg.ROUGE1-0.8) > 0.01 {
		t.Errorf("aggregate ROUGE-1: expected 0.8, got %f", agg.ROUGE1)
	}
}
