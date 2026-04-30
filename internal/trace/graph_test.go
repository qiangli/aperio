package trace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildGraphEmpty(t *testing.T) {
	tr := &Trace{
		ID:        "empty",
		CreatedAt: time.Now(),
		Metadata:  Metadata{Language: "python"},
	}

	g := BuildGraph(tr)
	if len(g.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(g.Edges))
	}
	if g.Stats.NodeCount != 0 {
		t.Errorf("expected node count 0, got %d", g.Stats.NodeCount)
	}
}

func TestBuildGraphSingleRoot(t *testing.T) {
	now := time.Now()
	tr := &Trace{
		ID:        "single",
		CreatedAt: now,
		Metadata:  Metadata{Language: "python"},
		Spans: []*Span{
			{ID: "root", Type: SpanFunction, Name: "main", StartTime: now, EndTime: now.Add(3 * time.Second)},
			{ID: "child1", ParentID: "root", Type: SpanLLMRequest, Name: "llm1", StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(1 * time.Second)},
			{ID: "child2", ParentID: "root", Type: SpanToolCall, Name: "tool1", StartTime: now.Add(1500 * time.Millisecond), EndTime: now.Add(2 * time.Second)},
		},
	}

	g := BuildGraph(tr)

	if g.Stats.NodeCount != 3 {
		t.Fatalf("expected 3 nodes, got %d", g.Stats.NodeCount)
	}
	if g.Stats.EdgeCount != 2 {
		t.Fatalf("expected 2 edges, got %d", g.Stats.EdgeCount)
	}
	if g.Stats.MaxDepth != 1 {
		t.Errorf("expected max depth 1, got %d", g.Stats.MaxDepth)
	}

	// Root should be depth 0
	if g.Nodes[0].Depth != 0 {
		t.Errorf("root depth: expected 0, got %d", g.Nodes[0].Depth)
	}

	// Children should be depth 1 and ordered by StartTime
	if g.Nodes[1].ID != "child1" || g.Nodes[1].ChildIndex != 0 {
		t.Errorf("first child: expected child1 at index 0, got %s at %d", g.Nodes[1].ID, g.Nodes[1].ChildIndex)
	}
	if g.Nodes[2].ID != "child2" || g.Nodes[2].ChildIndex != 1 {
		t.Errorf("second child: expected child2 at index 1, got %s at %d", g.Nodes[2].ID, g.Nodes[2].ChildIndex)
	}
}

func TestBuildGraphMultiRoot(t *testing.T) {
	now := time.Now()
	tr := &Trace{
		ID: "multi",
		Spans: []*Span{
			{ID: "r1", Type: SpanFunction, Name: "root1", StartTime: now, EndTime: now.Add(1 * time.Second)},
			{ID: "r2", Type: SpanFunction, Name: "root2", StartTime: now.Add(2 * time.Second), EndTime: now.Add(3 * time.Second)},
		},
	}

	g := BuildGraph(tr)
	if g.Stats.NodeCount != 2 {
		t.Fatalf("expected 2 nodes, got %d", g.Stats.NodeCount)
	}
	if g.Stats.EdgeCount != 0 {
		t.Errorf("expected 0 edges, got %d", g.Stats.EdgeCount)
	}
	// Roots should have depth 0 and be ordered
	if g.Nodes[0].ID != "r1" || g.Nodes[0].ChildIndex != 0 {
		t.Errorf("expected r1 at index 0")
	}
	if g.Nodes[1].ID != "r2" || g.Nodes[1].ChildIndex != 1 {
		t.Errorf("expected r2 at index 1")
	}
}

func TestBuildGraphDeepNesting(t *testing.T) {
	now := time.Now()
	tr := &Trace{
		ID: "deep",
		Spans: []*Span{
			{ID: "d0", Type: SpanFunction, Name: "level0", StartTime: now, EndTime: now.Add(4 * time.Second)},
			{ID: "d1", ParentID: "d0", Type: SpanFunction, Name: "level1", StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(3 * time.Second)},
			{ID: "d2", ParentID: "d1", Type: SpanLLMRequest, Name: "level2", StartTime: now.Add(200 * time.Millisecond), EndTime: now.Add(2 * time.Second)},
			{ID: "d3", ParentID: "d2", Type: SpanToolCall, Name: "level3", StartTime: now.Add(300 * time.Millisecond), EndTime: now.Add(1 * time.Second)},
		},
	}

	g := BuildGraph(tr)
	if g.Stats.MaxDepth != 3 {
		t.Errorf("expected max depth 3, got %d", g.Stats.MaxDepth)
	}
	if g.Stats.EdgeCount != 3 {
		t.Errorf("expected 3 edges, got %d", g.Stats.EdgeCount)
	}
}

func TestBuildGraphDuration(t *testing.T) {
	now := time.Now()
	tr := &Trace{
		ID: "dur",
		Spans: []*Span{
			{ID: "s1", Type: SpanFunction, Name: "fn", StartTime: now, EndTime: now.Add(1500 * time.Millisecond)},
		},
	}

	g := BuildGraph(tr)
	if g.Nodes[0].DurationMs != 1500 {
		t.Errorf("expected duration 1500ms, got %d", g.Nodes[0].DurationMs)
	}
}

func TestBuildGraphSpanTypeCounts(t *testing.T) {
	now := time.Now()
	tr := &Trace{
		ID: "counts",
		Spans: []*Span{
			{ID: "f1", Type: SpanFunction, Name: "fn1", StartTime: now, EndTime: now.Add(time.Second)},
			{ID: "f2", Type: SpanFunction, Name: "fn2", StartTime: now, EndTime: now.Add(time.Second)},
			{ID: "l1", Type: SpanLLMRequest, Name: "llm1", StartTime: now, EndTime: now.Add(time.Second)},
		},
	}

	g := BuildGraph(tr)
	if g.Stats.SpanTypeCounts["FUNCTION"] != 2 {
		t.Errorf("expected 2 FUNCTION, got %d", g.Stats.SpanTypeCounts["FUNCTION"])
	}
	if g.Stats.SpanTypeCounts["LLM_REQUEST"] != 1 {
		t.Errorf("expected 1 LLM_REQUEST, got %d", g.Stats.SpanTypeCounts["LLM_REQUEST"])
	}
}

func TestWriteReadGraphRoundTrip(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tr := &Trace{
		ID:        "roundtrip",
		CreatedAt: now,
		Metadata:  Metadata{Command: "python agent.py", Language: "python", WorkingDir: "/tmp"},
		Spans: []*Span{
			{ID: "r", Type: SpanFunction, Name: "main", StartTime: now, EndTime: now.Add(time.Second)},
			{ID: "c", ParentID: "r", Type: SpanLLMRequest, Name: "llm", StartTime: now.Add(100 * time.Millisecond), EndTime: now.Add(500 * time.Millisecond)},
		},
	}

	g := BuildGraph(tr)

	dir := t.TempDir()
	path := filepath.Join(dir, "graph.json")

	if err := WriteGraph(path, g); err != nil {
		t.Fatalf("WriteGraph: %v", err)
	}

	g2, err := ReadGraph(path)
	if err != nil {
		t.Fatalf("ReadGraph: %v", err)
	}

	if g2.TraceID != g.TraceID {
		t.Errorf("TraceID mismatch: %s vs %s", g2.TraceID, g.TraceID)
	}
	if len(g2.Nodes) != len(g.Nodes) {
		t.Errorf("node count mismatch: %d vs %d", len(g2.Nodes), len(g.Nodes))
	}
	if len(g2.Edges) != len(g.Edges) {
		t.Errorf("edge count mismatch: %d vs %d", len(g2.Edges), len(g.Edges))
	}
	if g2.Stats.MaxDepth != g.Stats.MaxDepth {
		t.Errorf("max depth mismatch: %d vs %d", g2.Stats.MaxDepth, g.Stats.MaxDepth)
	}
}

func TestReadGraphMissingFile(t *testing.T) {
	_, err := ReadGraph("/nonexistent/path.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestWriteGraphInvalidDir(t *testing.T) {
	g := &TraceGraph{Nodes: []GraphNode{}, Edges: []GraphEdge{}}
	err := WriteGraph("/nonexistent/dir/graph.json", g)
	if err == nil {
		t.Fatal("expected error for invalid directory")
	}
}

func TestBuildGraphFromSampleTrace(t *testing.T) {
	tr, err := ReadTrace("../../testdata/sample_trace.json")
	if err != nil {
		t.Fatalf("ReadTrace: %v", err)
	}

	g := BuildGraph(tr)

	if g.Stats.NodeCount != len(tr.Spans) {
		t.Errorf("expected %d nodes, got %d", len(tr.Spans), g.Stats.NodeCount)
	}

	// Verify edges: each span with a parent_id should have an edge
	expectedEdges := 0
	for _, s := range tr.Spans {
		if s.ParentID != "" {
			expectedEdges++
		}
	}
	if g.Stats.EdgeCount != expectedEdges {
		t.Errorf("expected %d edges, got %d", expectedEdges, g.Stats.EdgeCount)
	}

	// Write and verify the file is valid JSON
	dir := t.TempDir()
	path := filepath.Join(dir, "sample_graph.json")
	if err := WriteGraph(path, g); err != nil {
		t.Fatalf("WriteGraph: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("graph file is empty")
	}
}
