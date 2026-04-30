package eval

import (
	"math"
	"testing"
	"time"

	"github.com/qiangli/aperio/internal/trace"
)

func makeTestGraph(nodes []trace.GraphNode, edges []trace.GraphEdge) *trace.TraceGraph {
	stats := trace.GraphStats{
		NodeCount:      len(nodes),
		EdgeCount:      len(edges),
		SpanTypeCounts: make(map[string]int),
	}
	for _, n := range nodes {
		stats.SpanTypeCounts[string(n.Type)]++
	}
	return &trace.TraceGraph{
		TraceID:   "test",
		CreatedAt: time.Now(),
		Metadata:  trace.Metadata{Language: "python"},
		Nodes:     nodes,
		Edges:     edges,
		Stats:     stats,
	}
}

func TestEvaluateIdenticalGraphs(t *testing.T) {
	now := time.Now()
	g := makeTestGraph(
		[]trace.GraphNode{
			{ID: "f1", Type: trace.SpanFunction, Name: "main", StartTime: now, EndTime: now.Add(time.Second), Depth: 0, ChildIndex: 0},
			{ID: "l1", Type: trace.SpanLLMRequest, Name: "api", StartTime: now, EndTime: now.Add(time.Second), Depth: 1, ChildIndex: 0},
		},
		[]trace.GraphEdge{{Source: "f1", Target: "l1"}},
	)

	result := Evaluate(g, g, nil)

	if math.Abs(result.OverallSimilarity-1.0) > 1e-10 {
		t.Errorf("expected overall similarity 1.0, got %f", result.OverallSimilarity)
	}
	if math.Abs(result.BehavioralSimilarity-1.0) > 1e-10 {
		t.Errorf("expected behavioral similarity 1.0, got %f", result.BehavioralSimilarity)
	}
}

func TestEvaluateEmptyGraphs(t *testing.T) {
	g := makeTestGraph(nil, nil)
	result := Evaluate(g, g, nil)

	if result.OverallSimilarity != 1.0 {
		t.Errorf("expected similarity 1.0 for empty graphs, got %f", result.OverallSimilarity)
	}
}

func TestEvaluateDifferentGraphs(t *testing.T) {
	now := time.Now()
	g1 := makeTestGraph(
		[]trace.GraphNode{
			{ID: "f1", Type: trace.SpanFunction, Name: "main", StartTime: now, EndTime: now.Add(time.Second), Depth: 0, ChildIndex: 0},
			{ID: "l1", Type: trace.SpanLLMRequest, Name: "openai", StartTime: now, EndTime: now.Add(time.Second), Depth: 1, ChildIndex: 0},
			{ID: "t1", Type: trace.SpanToolCall, Name: "get_weather", StartTime: now, EndTime: now.Add(time.Second), Depth: 2, ChildIndex: 0,
				Attributes: map[string]any{"tool_name": "get_weather"}},
		},
		[]trace.GraphEdge{
			{Source: "f1", Target: "l1"},
			{Source: "l1", Target: "t1"},
		},
	)
	g2 := makeTestGraph(
		[]trace.GraphNode{
			{ID: "f1", Type: trace.SpanFunction, Name: "main", StartTime: now, EndTime: now.Add(time.Second), Depth: 0, ChildIndex: 0},
			{ID: "l1", Type: trace.SpanLLMRequest, Name: "anthropic", StartTime: now, EndTime: now.Add(time.Second), Depth: 1, ChildIndex: 0},
			{ID: "t1", Type: trace.SpanToolCall, Name: "search_web", StartTime: now, EndTime: now.Add(time.Second), Depth: 2, ChildIndex: 0,
				Attributes: map[string]any{"tool_name": "search_web"}},
		},
		[]trace.GraphEdge{
			{Source: "f1", Target: "l1"},
			{Source: "l1", Target: "t1"},
		},
	)

	result := Evaluate(g1, g2, nil)

	if result.OverallSimilarity >= 1.0 {
		t.Errorf("expected similarity < 1.0 for different graphs, got %f", result.OverallSimilarity)
	}
	if result.OverallSimilarity <= 0 {
		t.Errorf("expected similarity > 0 for structurally similar graphs, got %f", result.OverallSimilarity)
	}
	if result.EditDistance <= 0 {
		t.Errorf("expected edit distance > 0, got %f", result.EditDistance)
	}
}

func TestEvaluateBehavioralOnly(t *testing.T) {
	now := time.Now()
	g := makeTestGraph(
		[]trace.GraphNode{
			{ID: "f1", Type: trace.SpanFunction, Name: "main", StartTime: now, EndTime: now.Add(time.Second), Depth: 0, ChildIndex: 0},
			{ID: "l1", Type: trace.SpanLLMRequest, Name: "api", StartTime: now, EndTime: now.Add(time.Second), Depth: 1, ChildIndex: 0},
		},
		[]trace.GraphEdge{{Source: "f1", Target: "l1"}},
	)

	result := Evaluate(g, g, &EvalConfig{BehavioralOnly: true})

	// Structural similarity should be 0 (not computed)
	if result.StructuralSimilarity != 0 {
		t.Errorf("expected structural similarity 0 in behavioral-only mode, got %f", result.StructuralSimilarity)
	}
	if math.Abs(result.BehavioralSimilarity-1.0) > 1e-10 {
		t.Errorf("expected behavioral similarity 1.0, got %f", result.BehavioralSimilarity)
	}
}

func TestExtractBehavioralSpine(t *testing.T) {
	now := time.Now()
	g := makeTestGraph(
		[]trace.GraphNode{
			{ID: "f1", Type: trace.SpanFunction, Name: "main", StartTime: now, EndTime: now.Add(time.Second), Depth: 0, ChildIndex: 0},
			{ID: "l1", Type: trace.SpanLLMRequest, Name: "api", StartTime: now, EndTime: now.Add(time.Second), Depth: 1, ChildIndex: 0},
			{ID: "f2", Type: trace.SpanFunction, Name: "helper", StartTime: now, EndTime: now.Add(time.Second), Depth: 2, ChildIndex: 0},
			{ID: "t1", Type: trace.SpanToolCall, Name: "tool", StartTime: now, EndTime: now.Add(time.Second), Depth: 3, ChildIndex: 0},
		},
		[]trace.GraphEdge{
			{Source: "f1", Target: "l1"},
			{Source: "l1", Target: "f2"},
			{Source: "f2", Target: "t1"},
		},
	)

	spine := extractBehavioralSpine(g)

	if spine.Stats.NodeCount != 2 {
		t.Errorf("expected 2 behavioral nodes, got %d", spine.Stats.NodeCount)
	}

	// t1 should be connected to l1 (skipping f2)
	if len(spine.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(spine.Edges))
	}
	if spine.Edges[0].Source != "l1" || spine.Edges[0].Target != "t1" {
		t.Errorf("expected edge l1->t1, got %s->%s", spine.Edges[0].Source, spine.Edges[0].Target)
	}
}

func TestExtractBehavioralSpineNoBehavioral(t *testing.T) {
	now := time.Now()
	g := makeTestGraph(
		[]trace.GraphNode{
			{ID: "f1", Type: trace.SpanFunction, Name: "main", StartTime: now, EndTime: now.Add(time.Second)},
		},
		nil,
	)

	spine := extractBehavioralSpine(g)
	if spine.Stats.NodeCount != 0 {
		t.Errorf("expected 0 nodes, got %d", spine.Stats.NodeCount)
	}
}

func TestExtractBehavioralSpineNil(t *testing.T) {
	spine := extractBehavioralSpine(nil)
	if spine != nil {
		t.Errorf("expected nil for nil input")
	}
}

func TestGraphToTree(t *testing.T) {
	now := time.Now()
	g := makeTestGraph(
		[]trace.GraphNode{
			{ID: "r", Type: trace.SpanFunction, Name: "root", Depth: 0, ChildIndex: 0,
				StartTime: now, EndTime: now.Add(time.Second)},
			{ID: "c1", Type: trace.SpanLLMRequest, Name: "llm", Depth: 1, ChildIndex: 0,
				StartTime: now, EndTime: now.Add(time.Second)},
		},
		[]trace.GraphEdge{{Source: "r", Target: "c1"}},
	)

	tree := graphToTree(g)
	if tree == nil {
		t.Fatal("expected non-nil tree")
	}
	if tree.Label != "FUNCTION:root" {
		t.Errorf("expected root label FUNCTION:root, got %s", tree.Label)
	}
	if len(tree.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(tree.Children))
	}
	if tree.Children[0].Label != "LLM_REQUEST:llm" {
		t.Errorf("expected child label LLM_REQUEST:llm, got %s", tree.Children[0].Label)
	}
}

func TestGraphToTreeMultiRoot(t *testing.T) {
	now := time.Now()
	g := makeTestGraph(
		[]trace.GraphNode{
			{ID: "r1", Type: trace.SpanFunction, Name: "a", Depth: 0, ChildIndex: 0,
				StartTime: now, EndTime: now.Add(time.Second)},
			{ID: "r2", Type: trace.SpanFunction, Name: "b", Depth: 0, ChildIndex: 1,
				StartTime: now, EndTime: now.Add(time.Second)},
		},
		nil,
	)

	tree := graphToTree(g)
	if tree == nil {
		t.Fatal("expected non-nil tree")
	}
	if tree.Label != "__ROOT__:" {
		t.Errorf("expected synthetic root, got %s", tree.Label)
	}
	if len(tree.Children) != 2 {
		t.Errorf("expected 2 children, got %d", len(tree.Children))
	}
}

func TestGraphToTreeEmpty(t *testing.T) {
	g := makeTestGraph(nil, nil)
	tree := graphToTree(g)
	if tree != nil {
		t.Errorf("expected nil tree for empty graph")
	}
}

func TestNormalizeSimilarity(t *testing.T) {
	tests := []struct {
		dist, maxDist, want float64
	}{
		{0, 10, 1.0},
		{10, 10, 0.0},
		{5, 10, 0.5},
		{0, 0, 1.0},
	}
	for _, tt := range tests {
		got := normalizeSimilarity(tt.dist, tt.maxDist)
		if math.Abs(got-tt.want) > 1e-10 {
			t.Errorf("normalizeSimilarity(%f, %f) = %f, want %f",
				tt.dist, tt.maxDist, got, tt.want)
		}
	}
}

func TestFormatText(t *testing.T) {
	r := &EvalResult{
		OverallSimilarity:    0.847,
		StructuralSimilarity: 0.912,
		BehavioralSimilarity: 0.783,
		EditDistance:          3.40,
		MaxDistance:           22.10,
		TypeBreakdown: map[string]TypeEvalResult{
			"FUNCTION": {LeftCount: 10, RightCount: 10, Matched: 8, Deleted: 2, Similarity: 0.8},
		},
	}

	text := FormatText(r, false)
	if text == "" {
		t.Fatal("expected non-empty text")
	}
	if !contains(text, "0.847") {
		t.Error("expected overall similarity in output")
	}
	if !contains(text, "FUNCTION") {
		t.Error("expected type breakdown in output")
	}
}

func TestFormatJSON(t *testing.T) {
	r := &EvalResult{
		OverallSimilarity: 0.5,
		TypeBreakdown:     map[string]TypeEvalResult{},
	}

	jsonStr, err := FormatJSON(r)
	if err != nil {
		t.Fatalf("FormatJSON: %v", err)
	}
	if !contains(jsonStr, "overall_similarity") {
		t.Error("expected overall_similarity in JSON")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
