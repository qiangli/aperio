package eval

import (
	"fmt"
	"sort"

	"github.com/qiangli/aperio/internal/trace"
)

// EvalResult holds the complete evaluation output.
type EvalResult struct {
	OverallSimilarity    float64                  `json:"overall_similarity"`
	StructuralSimilarity float64                  `json:"structural_similarity"`
	BehavioralSimilarity float64                  `json:"behavioral_similarity"`
	EditDistance          float64                  `json:"edit_distance"`
	MaxDistance           float64                  `json:"max_distance"`
	TypeBreakdown        map[string]TypeEvalResult `json:"type_breakdown"`
	EditScript           []EditOperation           `json:"edit_script"`
}

// TypeEvalResult shows per-type evaluation details.
type TypeEvalResult struct {
	LeftCount  int     `json:"left_count"`
	RightCount int     `json:"right_count"`
	Matched    int     `json:"matched"`
	Inserted   int     `json:"inserted"`
	Deleted    int     `json:"deleted"`
	Renamed    int     `json:"renamed"`
	Similarity float64 `json:"similarity"`
}

// EditOperation is the user-facing representation of an edit operation.
type EditOperation struct {
	Op     string  `json:"op"`
	NodeA  string  `json:"node_a,omitempty"`
	NodeB  string  `json:"node_b,omitempty"`
	Cost   float64 `json:"cost"`
	Detail string  `json:"detail"`
}

// EvalConfig configures the evaluation.
type EvalConfig struct {
	BehavioralOnly bool
}

// Behavioral span types that form the "behavioral spine".
var behavioralTypes = map[trace.SpanType]bool{
	trace.SpanLLMRequest:  true,
	trace.SpanLLMResponse: true,
	trace.SpanToolCall:    true,
	trace.SpanToolResult:  true,
	trace.SpanUserInput:   true,
	trace.SpanAgentOutput: true,
}

// Evaluate compares two trace graphs and returns a detailed evaluation result.
func Evaluate(left, right *trace.TraceGraph, cfg *EvalConfig) *EvalResult {
	if cfg == nil {
		cfg = &EvalConfig{}
	}

	result := &EvalResult{
		TypeBreakdown: make(map[string]TypeEvalResult),
	}

	leftTree := graphToTree(left)
	rightTree := graphToTree(right)
	costFn := DefaultCostFn()

	// Overall similarity
	dist, ops := zhangShasha(leftTree, rightTree, costFn)
	maxDist := maxDistance(leftTree, rightTree, costFn)
	result.EditDistance = dist
	result.MaxDistance = maxDist
	result.OverallSimilarity = normalizeSimilarity(dist, maxDist)

	// Build edit script
	leftIdx := indexedNodes(left)
	rightIdx := indexedNodes(right)
	result.EditScript = translateOps(ops, leftTree, rightTree, leftIdx, rightIdx)

	// Type breakdown from edit script
	result.TypeBreakdown = computeTypeBreakdown(left, right, result.EditScript)

	if !cfg.BehavioralOnly {
		// Structural similarity: type-only labels
		structCost := structuralCostFn()
		structLeft := graphToTreeStructural(left)
		structRight := graphToTreeStructural(right)
		structDist, _ := zhangShasha(structLeft, structRight, structCost)
		structMax := maxDistance(structLeft, structRight, structCost)
		result.StructuralSimilarity = normalizeSimilarity(structDist, structMax)
	}

	// Behavioral similarity: only behavioral spine
	behavLeft := extractBehavioralSpine(left)
	behavRight := extractBehavioralSpine(right)
	behavLeftTree := graphToTree(behavLeft)
	behavRightTree := graphToTree(behavRight)
	behavDist, _ := zhangShasha(behavLeftTree, behavRightTree, costFn)
	behavMax := maxDistance(behavLeftTree, behavRightTree, costFn)
	result.BehavioralSimilarity = normalizeSimilarity(behavDist, behavMax)

	return result
}

// graphToTree converts a TraceGraph into a TreeNode hierarchy for the TED algorithm.
// Multi-root graphs get a synthetic root with zero-cost label.
func graphToTree(g *trace.TraceGraph) *TreeNode {
	if g == nil || len(g.Nodes) == 0 {
		return nil
	}

	// Build node map and children map from edges
	nodeMap := make(map[string]*trace.GraphNode)
	for i := range g.Nodes {
		nodeMap[g.Nodes[i].ID] = &g.Nodes[i]
	}

	childrenOf := make(map[string][]string)
	hasParent := make(map[string]bool)
	for _, e := range g.Edges {
		childrenOf[e.Source] = append(childrenOf[e.Source], e.Target)
		hasParent[e.Target] = true
	}

	// Find roots
	var roots []string
	for _, n := range g.Nodes {
		if !hasParent[n.ID] {
			roots = append(roots, n.ID)
		}
	}

	// Sort children by child index
	for parentID, children := range childrenOf {
		sort.Slice(children, func(i, j int) bool {
			ni := nodeMap[children[i]]
			nj := nodeMap[children[j]]
			if ni == nil || nj == nil {
				return false
			}
			return ni.ChildIndex < nj.ChildIndex
		})
		childrenOf[parentID] = children
	}

	var buildTree func(id string) *TreeNode
	buildTree = func(id string) *TreeNode {
		gn := nodeMap[id]
		if gn == nil {
			return nil
		}
		tn := &TreeNode{
			Label: fmt.Sprintf("%s:%s", gn.Type, gn.Name),
			Attrs: gn.Attributes,
		}
		for _, childID := range childrenOf[id] {
			child := buildTree(childID)
			if child != nil {
				tn.Children = append(tn.Children, child)
			}
		}
		return tn
	}

	if len(roots) == 1 {
		return buildTree(roots[0])
	}

	// Synthetic root for multi-root graphs
	synthetic := &TreeNode{Label: "__ROOT__:"}
	for _, rootID := range roots {
		child := buildTree(rootID)
		if child != nil {
			synthetic.Children = append(synthetic.Children, child)
		}
	}
	return synthetic
}

// graphToTreeStructural creates a tree using only span types as labels (no names).
func graphToTreeStructural(g *trace.TraceGraph) *TreeNode {
	if g == nil || len(g.Nodes) == 0 {
		return nil
	}

	nodeMap := make(map[string]*trace.GraphNode)
	for i := range g.Nodes {
		nodeMap[g.Nodes[i].ID] = &g.Nodes[i]
	}

	childrenOf := make(map[string][]string)
	hasParent := make(map[string]bool)
	for _, e := range g.Edges {
		childrenOf[e.Source] = append(childrenOf[e.Source], e.Target)
		hasParent[e.Target] = true
	}

	var roots []string
	for _, n := range g.Nodes {
		if !hasParent[n.ID] {
			roots = append(roots, n.ID)
		}
	}

	for parentID, children := range childrenOf {
		sort.Slice(children, func(i, j int) bool {
			ni := nodeMap[children[i]]
			nj := nodeMap[children[j]]
			if ni == nil || nj == nil {
				return false
			}
			return ni.ChildIndex < nj.ChildIndex
		})
		childrenOf[parentID] = children
	}

	var buildTree func(id string) *TreeNode
	buildTree = func(id string) *TreeNode {
		gn := nodeMap[id]
		if gn == nil {
			return nil
		}
		tn := &TreeNode{Label: string(gn.Type)}
		for _, childID := range childrenOf[id] {
			child := buildTree(childID)
			if child != nil {
				tn.Children = append(tn.Children, child)
			}
		}
		return tn
	}

	if len(roots) == 1 {
		return buildTree(roots[0])
	}

	synthetic := &TreeNode{Label: "__ROOT__"}
	for _, rootID := range roots {
		child := buildTree(rootID)
		if child != nil {
			synthetic.Children = append(synthetic.Children, child)
		}
	}
	return synthetic
}

// structuralCostFn uses unit costs for structural comparison.
func structuralCostFn() CostFn {
	return CostFn{
		Insert: func(n *TreeNode) float64 { return 1 },
		Delete: func(n *TreeNode) float64 { return 1 },
		Rename: func(a, b *TreeNode) float64 {
			if a.Label == b.Label {
				return 0
			}
			return 1
		},
	}
}

// extractBehavioralSpine extracts a subgraph containing only behavioral span types.
// Non-behavioral nodes are removed and their behavioral children are reconnected
// to the nearest behavioral ancestor.
func extractBehavioralSpine(g *trace.TraceGraph) *trace.TraceGraph {
	if g == nil {
		return nil
	}

	result := &trace.TraceGraph{
		TraceID:   g.TraceID,
		CreatedAt: g.CreatedAt,
		Metadata:  g.Metadata,
		Stats: trace.GraphStats{
			SpanTypeCounts: make(map[string]int),
		},
	}

	// Find behavioral nodes
	behavNodes := make(map[string]bool)
	for _, n := range g.Nodes {
		if behavioralTypes[n.Type] {
			behavNodes[n.ID] = true
		}
	}

	if len(behavNodes) == 0 {
		result.Nodes = []trace.GraphNode{}
		result.Edges = []trace.GraphEdge{}
		return result
	}

	// Build parent map from edges
	parentOf := make(map[string]string)
	childrenOf := make(map[string][]string)
	for _, e := range g.Edges {
		parentOf[e.Target] = e.Source
		childrenOf[e.Source] = append(childrenOf[e.Source], e.Target)
	}

	// For each behavioral node, find its nearest behavioral ancestor
	var findBehavAncestor func(id string) string
	findBehavAncestor = func(id string) string {
		parent := parentOf[id]
		if parent == "" {
			return ""
		}
		if behavNodes[parent] {
			return parent
		}
		return findBehavAncestor(parent)
	}

	// Copy behavioral nodes
	nodeByID := make(map[string]*trace.GraphNode)
	for i, n := range g.Nodes {
		if behavNodes[n.ID] {
			result.Nodes = append(result.Nodes, g.Nodes[i])
			nodeByID[n.ID] = &result.Nodes[len(result.Nodes)-1]
			result.Stats.SpanTypeCounts[string(n.Type)]++
		}
	}

	// Create edges based on nearest behavioral ancestor
	edgeSet := make(map[string]bool)
	for _, n := range result.Nodes {
		ancestor := findBehavAncestor(n.ID)
		if ancestor != "" {
			edgeKey := ancestor + "->" + n.ID
			if !edgeSet[edgeKey] {
				result.Edges = append(result.Edges, trace.GraphEdge{
					Source: ancestor,
					Target: n.ID,
				})
				edgeSet[edgeKey] = true
			}
		}
	}

	result.Stats.NodeCount = len(result.Nodes)
	result.Stats.EdgeCount = len(result.Edges)

	return result
}

// maxDistance computes the maximum possible edit distance between two trees.
func maxDistance(t1, t2 *TreeNode, costFn CostFn) float64 {
	if t1 == nil && t2 == nil {
		return 0
	}

	var cost float64
	var walk func(n *TreeNode, fn func(*TreeNode) float64)
	walk = func(n *TreeNode, fn func(*TreeNode) float64) {
		if n == nil {
			return
		}
		cost += fn(n)
		for _, c := range n.Children {
			walk(c, fn)
		}
	}

	walk(t1, costFn.Delete)
	walk(t2, costFn.Insert)
	return cost
}

// normalizeSimilarity converts edit distance to a [0, 1] similarity score.
func normalizeSimilarity(dist, maxDist float64) float64 {
	if maxDist == 0 {
		return 1.0
	}
	s := 1.0 - (dist / maxDist)
	if s < 0 {
		return 0
	}
	return s
}

// indexedNodes creates a post-order index mapping for the graph nodes.
func indexedNodes(g *trace.TraceGraph) map[int]string {
	if g == nil || len(g.Nodes) == 0 {
		return nil
	}
	// Build the same tree and index it to get post-order mapping
	tree := graphToTree(g)
	it := newIndexedTree(tree)

	// Map post-order index to node label
	result := make(map[int]string)
	for i := 1; i <= it.size; i++ {
		result[i] = it.nodes[i].Label
	}
	return result
}

// translateOps converts internal EditOps to user-facing EditOperations.
func translateOps(ops []EditOp, _, _ *TreeNode, leftIdx, rightIdx map[int]string) []EditOperation {
	var result []EditOperation
	for _, op := range ops {
		eo := EditOperation{
			Op:   op.Op,
			Cost: op.Cost,
		}
		if op.IdxA > 0 {
			eo.NodeA = leftIdx[op.IdxA]
		}
		if op.IdxB > 0 {
			eo.NodeB = rightIdx[op.IdxB]
		}

		switch op.Op {
		case "match":
			eo.Detail = fmt.Sprintf("matched %s", eo.NodeA)
		case "rename":
			eo.Detail = fmt.Sprintf("renamed %s → %s", eo.NodeA, eo.NodeB)
		case "delete":
			eo.Detail = fmt.Sprintf("deleted %s", eo.NodeA)
		case "insert":
			eo.Detail = fmt.Sprintf("inserted %s", eo.NodeB)
		}
		result = append(result, eo)
	}
	return result
}

// computeTypeBreakdown computes per-type evaluation statistics from the edit script.
func computeTypeBreakdown(left, right *trace.TraceGraph, ops []EditOperation) map[string]TypeEvalResult {
	breakdown := make(map[string]TypeEvalResult)

	// Count nodes per type
	if left != nil {
		for _, n := range left.Nodes {
			t := string(n.Type)
			tr := breakdown[t]
			tr.LeftCount++
			breakdown[t] = tr
		}
	}
	if right != nil {
		for _, n := range right.Nodes {
			t := string(n.Type)
			tr := breakdown[t]
			tr.RightCount++
			breakdown[t] = tr
		}
	}

	// Count operations per type
	for _, op := range ops {
		var nodeType string
		switch op.Op {
		case "match", "rename":
			nodeType, _ = splitLabel(op.NodeA)
		case "delete":
			nodeType, _ = splitLabel(op.NodeA)
		case "insert":
			nodeType, _ = splitLabel(op.NodeB)
		}

		if nodeType == "" || nodeType == "__ROOT__" {
			continue
		}

		tr := breakdown[nodeType]
		switch op.Op {
		case "match":
			tr.Matched++
		case "rename":
			tr.Renamed++
		case "delete":
			tr.Deleted++
		case "insert":
			tr.Inserted++
		}
		breakdown[nodeType] = tr
	}

	// Compute per-type similarity
	for t, tr := range breakdown {
		total := tr.LeftCount + tr.RightCount
		if total > 0 {
			tr.Similarity = float64(tr.Matched*2) / float64(total)
		} else {
			tr.Similarity = 1.0
		}
		breakdown[t] = tr
	}

	return breakdown
}
