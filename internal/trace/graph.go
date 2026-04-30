package trace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// TraceGraph is the explicit graph representation of an execution trace.
type TraceGraph struct {
	TraceID   string      `json:"trace_id"`
	CreatedAt time.Time   `json:"created_at"`
	Metadata  Metadata    `json:"metadata"`
	Nodes     []GraphNode `json:"nodes"`
	Edges     []GraphEdge `json:"edges"`
	Stats     GraphStats  `json:"stats"`
}

// GraphNode represents a span projected into graph form.
type GraphNode struct {
	ID         string         `json:"id"`
	Type       SpanType       `json:"type"`
	Name       string         `json:"name"`
	StartTime  time.Time      `json:"start_time"`
	EndTime    time.Time      `json:"end_time"`
	DurationMs int64          `json:"duration_ms"`
	Depth      int            `json:"depth"`
	ChildIndex int            `json:"child_index"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// GraphEdge represents a parent-child relationship in the trace graph.
type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// GraphStats holds summary statistics about the graph.
type GraphStats struct {
	NodeCount      int            `json:"node_count"`
	EdgeCount      int            `json:"edge_count"`
	MaxDepth       int            `json:"max_depth"`
	SpanTypeCounts map[string]int `json:"span_type_counts"`
}

// BuildGraph converts a flat Trace into an explicit TraceGraph.
func BuildGraph(t *Trace) *TraceGraph {
	g := &TraceGraph{
		TraceID:   t.ID,
		CreatedAt: t.CreatedAt,
		Metadata:  t.Metadata,
		Stats: GraphStats{
			SpanTypeCounts: make(map[string]int),
		},
	}

	if len(t.Spans) == 0 {
		g.Nodes = []GraphNode{}
		g.Edges = []GraphEdge{}
		return g
	}

	// Index spans by ID
	spanByID := make(map[string]*Span, len(t.Spans))
	for _, s := range t.Spans {
		spanByID[s.ID] = s
	}

	// Build children map
	children := make(map[string][]string)
	var roots []string
	for _, s := range t.Spans {
		if s.ParentID == "" {
			roots = append(roots, s.ID)
		} else {
			children[s.ParentID] = append(children[s.ParentID], s.ID)
		}
	}

	// Sort children by StartTime
	for parentID, childIDs := range children {
		sort.Slice(childIDs, func(i, j int) bool {
			si := spanByID[childIDs[i]]
			sj := spanByID[childIDs[j]]
			return si.StartTime.Before(sj.StartTime)
		})
		children[parentID] = childIDs
	}

	// Sort roots by StartTime
	sort.Slice(roots, func(i, j int) bool {
		return spanByID[roots[i]].StartTime.Before(spanByID[roots[j]].StartTime)
	})

	// DFS to compute Depth, ChildIndex and build nodes/edges
	maxDepth := 0
	var dfs func(id string, depth, childIdx int)
	dfs = func(id string, depth, childIdx int) {
		s := spanByID[id]
		if s == nil {
			return
		}

		node := GraphNode{
			ID:         s.ID,
			Type:       s.Type,
			Name:       s.Name,
			StartTime:  s.StartTime,
			EndTime:    s.EndTime,
			DurationMs: s.EndTime.Sub(s.StartTime).Milliseconds(),
			Depth:      depth,
			ChildIndex: childIdx,
			Attributes: s.Attributes,
		}
		g.Nodes = append(g.Nodes, node)
		g.Stats.SpanTypeCounts[string(s.Type)]++

		if depth > maxDepth {
			maxDepth = depth
		}

		for i, childID := range children[id] {
			g.Edges = append(g.Edges, GraphEdge{
				Source: id,
				Target: childID,
			})
			dfs(childID, depth+1, i)
		}
	}

	for i, rootID := range roots {
		dfs(rootID, 0, i)
	}

	g.Stats.NodeCount = len(g.Nodes)
	g.Stats.EdgeCount = len(g.Edges)
	g.Stats.MaxDepth = maxDepth

	return g
}

// WriteGraph writes a TraceGraph as JSON to the given path atomically.
func WriteGraph(path string, g *TraceGraph) error {
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal graph: %w", err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "aperio-graph-*.json")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

// ReadGraph reads a TraceGraph from a JSON file.
func ReadGraph(path string) (*TraceGraph, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read graph file: %w", err)
	}

	var g TraceGraph
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("unmarshal graph: %w", err)
	}

	return &g, nil
}
