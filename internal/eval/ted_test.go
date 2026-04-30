package eval

import (
	"math"
	"testing"
)

// unitCost uses cost 1 for all operations (standard textbook definition).
var unitCost = CostFn{
	Insert: func(n *TreeNode) float64 { return 1 },
	Delete: func(n *TreeNode) float64 { return 1 },
	Rename: func(a, b *TreeNode) float64 {
		if a.Label == b.Label {
			return 0
		}
		return 1
	},
}

func TestTEDIdenticalTrees(t *testing.T) {
	// Tree: A(B, C)
	t1 := &TreeNode{Label: "A", Children: []*TreeNode{
		{Label: "B"},
		{Label: "C"},
	}}
	t2 := &TreeNode{Label: "A", Children: []*TreeNode{
		{Label: "B"},
		{Label: "C"},
	}}

	dist, ops := zhangShasha(t1, t2, unitCost)
	if dist != 0 {
		t.Errorf("expected distance 0 for identical trees, got %f", dist)
	}
	for _, op := range ops {
		if op.Op != "match" {
			t.Errorf("expected only match ops, got %s", op.Op)
		}
	}
}

func TestTEDSingleNodeRename(t *testing.T) {
	t1 := &TreeNode{Label: "A"}
	t2 := &TreeNode{Label: "B"}

	dist, _ := zhangShasha(t1, t2, unitCost)
	if dist != 1 {
		t.Errorf("expected distance 1, got %f", dist)
	}
}

func TestTEDInsertLeaf(t *testing.T) {
	// T1: A(B)
	// T2: A(B, C)
	// Distance: 1 (insert C)
	t1 := &TreeNode{Label: "A", Children: []*TreeNode{
		{Label: "B"},
	}}
	t2 := &TreeNode{Label: "A", Children: []*TreeNode{
		{Label: "B"},
		{Label: "C"},
	}}

	dist, _ := zhangShasha(t1, t2, unitCost)
	if dist != 1 {
		t.Errorf("expected distance 1, got %f", dist)
	}
}

func TestTEDDeleteLeaf(t *testing.T) {
	// T1: A(B, C)
	// T2: A(B)
	// Distance: 1 (delete C)
	t1 := &TreeNode{Label: "A", Children: []*TreeNode{
		{Label: "B"},
		{Label: "C"},
	}}
	t2 := &TreeNode{Label: "A", Children: []*TreeNode{
		{Label: "B"},
	}}

	dist, _ := zhangShasha(t1, t2, unitCost)
	if dist != 1 {
		t.Errorf("expected distance 1, got %f", dist)
	}
}

func TestTEDEmptyToTree(t *testing.T) {
	// Empty tree to A(B, C) = 3 inserts
	t2 := &TreeNode{Label: "A", Children: []*TreeNode{
		{Label: "B"},
		{Label: "C"},
	}}

	dist, _ := zhangShasha(nil, t2, unitCost)
	if dist != 3 {
		t.Errorf("expected distance 3, got %f", dist)
	}
}

func TestTEDTreeToEmpty(t *testing.T) {
	t1 := &TreeNode{Label: "A", Children: []*TreeNode{
		{Label: "B"},
		{Label: "C"},
	}}

	dist, _ := zhangShasha(t1, nil, unitCost)
	if dist != 3 {
		t.Errorf("expected distance 3, got %f", dist)
	}
}

func TestTEDBothNil(t *testing.T) {
	dist, ops := zhangShasha(nil, nil, unitCost)
	if dist != 0 {
		t.Errorf("expected distance 0, got %f", dist)
	}
	if len(ops) != 0 {
		t.Errorf("expected no ops, got %d", len(ops))
	}
}

func TestTEDTextbookExample(t *testing.T) {
	// Classic example:
	// T1: A(B(D, E), C(F))
	// T2: A(B(D), C(E, F))
	// Distance: 2 (delete E from under B, insert E under C)
	t1 := &TreeNode{Label: "A", Children: []*TreeNode{
		{Label: "B", Children: []*TreeNode{
			{Label: "D"},
			{Label: "E"},
		}},
		{Label: "C", Children: []*TreeNode{
			{Label: "F"},
		}},
	}}
	t2 := &TreeNode{Label: "A", Children: []*TreeNode{
		{Label: "B", Children: []*TreeNode{
			{Label: "D"},
		}},
		{Label: "C", Children: []*TreeNode{
			{Label: "E"},
			{Label: "F"},
		}},
	}}

	dist, _ := zhangShasha(t1, t2, unitCost)
	if dist != 2 {
		t.Errorf("expected distance 2, got %f", dist)
	}
}

func TestTEDCompletelyDifferent(t *testing.T) {
	// T1: A(B, C)
	// T2: X(Y, Z)
	// Distance: 3 (rename all)
	t1 := &TreeNode{Label: "A", Children: []*TreeNode{
		{Label: "B"},
		{Label: "C"},
	}}
	t2 := &TreeNode{Label: "X", Children: []*TreeNode{
		{Label: "Y"},
		{Label: "Z"},
	}}

	dist, _ := zhangShasha(t1, t2, unitCost)
	if dist != 3 {
		t.Errorf("expected distance 3, got %f", dist)
	}
}

func TestTEDDeepChain(t *testing.T) {
	// T1: A -> B -> C -> D
	// T2: A -> B -> C
	// Distance: 1 (delete D)
	t1 := &TreeNode{Label: "A", Children: []*TreeNode{
		{Label: "B", Children: []*TreeNode{
			{Label: "C", Children: []*TreeNode{
				{Label: "D"},
			}},
		}},
	}}
	t2 := &TreeNode{Label: "A", Children: []*TreeNode{
		{Label: "B", Children: []*TreeNode{
			{Label: "C"},
		}},
	}}

	dist, _ := zhangShasha(t1, t2, unitCost)
	if dist != 1 {
		t.Errorf("expected distance 1, got %f", dist)
	}
}

func TestTEDEditScript(t *testing.T) {
	// T1: A(B)
	// T2: A(C)
	// Should have: match A, rename B->C
	t1 := &TreeNode{Label: "A", Children: []*TreeNode{
		{Label: "B"},
	}}
	t2 := &TreeNode{Label: "A", Children: []*TreeNode{
		{Label: "C"},
	}}

	dist, ops := zhangShasha(t1, t2, unitCost)
	if dist != 1 {
		t.Errorf("expected distance 1, got %f", dist)
	}

	matchCount := 0
	renameCount := 0
	for _, op := range ops {
		switch op.Op {
		case "match":
			matchCount++
		case "rename":
			renameCount++
		}
	}
	if matchCount != 1 {
		t.Errorf("expected 1 match, got %d", matchCount)
	}
	if renameCount != 1 {
		t.Errorf("expected 1 rename, got %d", renameCount)
	}
}

func TestTEDWeightedCosts(t *testing.T) {
	weighted := CostFn{
		Insert: func(n *TreeNode) float64 {
			if n.Label == "important" {
				return 2.0
			}
			return 0.5
		},
		Delete: func(n *TreeNode) float64 {
			if n.Label == "important" {
				return 2.0
			}
			return 0.5
		},
		Rename: func(a, b *TreeNode) float64 {
			if a.Label == b.Label {
				return 0
			}
			return 1.0
		},
	}

	// T1: A(important), T2: A
	// Optimal: rename "important"->A (cost 1.0) + delete A (cost 0.5) = 1.5
	// (cheaper than match A + delete "important" which is 0 + 2.0 = 2.0)
	t1 := &TreeNode{Label: "A", Children: []*TreeNode{
		{Label: "important"},
	}}
	t2 := &TreeNode{Label: "A"}

	dist, _ := zhangShasha(t1, t2, weighted)
	if math.Abs(dist-1.5) > 1e-10 {
		t.Errorf("expected distance 1.5, got %f", dist)
	}
}

func TestNewIndexedTree(t *testing.T) {
	// A(B(D, E), C(F))
	root := &TreeNode{Label: "A", Children: []*TreeNode{
		{Label: "B", Children: []*TreeNode{
			{Label: "D"},
			{Label: "E"},
		}},
		{Label: "C", Children: []*TreeNode{
			{Label: "F"},
		}},
	}}

	it := newIndexedTree(root)
	if it.size != 6 {
		t.Errorf("expected 6 nodes, got %d", it.size)
	}

	// Post-order should be: D, E, B, F, C, A
	expected := []string{"D", "E", "B", "F", "C", "A"}
	for i, label := range expected {
		if it.nodes[i+1].Label != label {
			t.Errorf("post-order[%d]: expected %s, got %s", i, label, it.nodes[i+1].Label)
		}
	}
}

func TestNewIndexedTreeNil(t *testing.T) {
	it := newIndexedTree(nil)
	if it.size != 0 {
		t.Errorf("expected size 0, got %d", it.size)
	}
}
