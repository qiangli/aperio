package eval

import "math"

// TreeNode represents a node in a labeled ordered tree for tree edit distance computation.
type TreeNode struct {
	Label    string
	Attrs    map[string]any
	Children []*TreeNode
}

// CostFn defines the cost functions for tree edit distance operations.
type CostFn struct {
	// Insert returns the cost of inserting a node.
	Insert func(n *TreeNode) float64
	// Delete returns the cost of deleting a node.
	Delete func(n *TreeNode) float64
	// Rename returns the cost of renaming node a to node b (0 if they match).
	Rename func(a, b *TreeNode) float64
}

// EditOp represents a single edit operation in the edit script.
type EditOp struct {
	Op   string // "insert", "delete", "rename", "match"
	IdxA int    // post-order index in tree A (-1 for insert)
	IdxB int    // post-order index in tree B (-1 for delete)
	Cost float64
}

// indexedTree holds the pre-computed data structures for the Zhang-Shasha algorithm.
type indexedTree struct {
	nodes    []*TreeNode // nodes in post-order (1-indexed, index 0 unused)
	l        []int       // l[i] = leftmost leaf descendant of node i (post-order index)
	keyRoots []int       // key root nodes
	size     int         // number of nodes
}

// zhangShasha computes the tree edit distance between two trees
// using the Zhang-Shasha algorithm with the given cost functions.
// It returns the edit distance and the edit script.
func zhangShasha(t1, t2 *TreeNode, costFn CostFn) (float64, []EditOp) {
	if t1 == nil && t2 == nil {
		return 0, nil
	}

	it1 := newIndexedTree(t1)
	it2 := newIndexedTree(t2)

	if it1.size == 0 && it2.size == 0 {
		return 0, nil
	}

	// Handle one tree being empty
	if it1.size == 0 {
		var cost float64
		var ops []EditOp
		for i := 1; i <= it2.size; i++ {
			c := costFn.Insert(it2.nodes[i])
			cost += c
			ops = append(ops, EditOp{Op: "insert", IdxA: -1, IdxB: i, Cost: c})
		}
		return cost, ops
	}
	if it2.size == 0 {
		var cost float64
		var ops []EditOp
		for i := 1; i <= it1.size; i++ {
			c := costFn.Delete(it1.nodes[i])
			cost += c
			ops = append(ops, EditOp{Op: "delete", IdxA: i, IdxB: -1, Cost: c})
		}
		return cost, ops
	}

	n := it1.size
	m := it2.size

	// td[i][j] = tree distance between subtree rooted at post-order node i in t1
	// and subtree rooted at post-order node j in t2
	td := make([][]float64, n+1)
	for i := range td {
		td[i] = make([]float64, m+1)
	}

	for _, x := range it1.keyRoots {
		for _, y := range it2.keyRoots {
			lx := it1.l[x]
			ly := it2.l[y]

			fdRows := x - lx + 2
			fdCols := y - ly + 2
			fd := make([][]float64, fdRows)
			for i := range fd {
				fd[i] = make([]float64, fdCols)
			}

			// Base cases
			fd[0][0] = 0
			for i := lx; i <= x; i++ {
				fi := i - lx + 1
				fd[fi][0] = fd[fi-1][0] + costFn.Delete(it1.nodes[i])
			}
			for j := ly; j <= y; j++ {
				fj := j - ly + 1
				fd[0][fj] = fd[0][fj-1] + costFn.Insert(it2.nodes[j])
			}

			for i := lx; i <= x; i++ {
				fi := i - lx + 1
				for j := ly; j <= y; j++ {
					fj := j - ly + 1

					deleteCost := fd[fi-1][fj] + costFn.Delete(it1.nodes[i])
					insertCost := fd[fi][fj-1] + costFn.Insert(it2.nodes[j])

					if it1.l[i] == lx && it2.l[j] == ly {
						renameCost := fd[fi-1][fj-1] + costFn.Rename(it1.nodes[i], it2.nodes[j])
						fd[fi][fj] = min3(deleteCost, insertCost, renameCost)
						td[i][j] = fd[fi][fj]
					} else {
						li := it1.l[i] - lx + 1
						lj := it2.l[j] - ly + 1
						subtreeCost := fd[li-1][lj-1] + td[i][j]
						fd[fi][fj] = min3(deleteCost, insertCost, subtreeCost)
					}
				}
			}
		}
	}

	ops := buildEditScript(it1, it2, td, costFn)
	return td[n][m], ops
}

// buildEditScript reconstructs the edit operations from the distance matrix.
func buildEditScript(it1, it2 *indexedTree, td [][]float64, costFn CostFn) []EditOp {
	var ops []EditOp

	var backtrack func(i, j int)
	backtrack = func(i, j int) {
		if i == 0 && j == 0 {
			return
		}
		if i == 0 {
			for k := 1; k <= j; k++ {
				ops = append(ops, EditOp{
					Op:   "insert",
					IdxA: -1,
					IdxB: k,
					Cost: costFn.Insert(it2.nodes[k]),
				})
			}
			return
		}
		if j == 0 {
			for k := 1; k <= i; k++ {
				ops = append(ops, EditOp{
					Op:   "delete",
					IdxA: k,
					IdxB: -1,
					Cost: costFn.Delete(it1.nodes[k]),
				})
			}
			return
		}

		deleteCost := td[i-1][j] + costFn.Delete(it1.nodes[i])
		renameCost := td[i-1][j-1] + costFn.Rename(it1.nodes[i], it2.nodes[j])

		const epsilon = 1e-10

		if math.Abs(td[i][j]-renameCost) < epsilon {
			backtrack(i-1, j-1)
			rc := costFn.Rename(it1.nodes[i], it2.nodes[j])
			if rc == 0 {
				ops = append(ops, EditOp{Op: "match", IdxA: i, IdxB: j, Cost: 0})
			} else {
				ops = append(ops, EditOp{Op: "rename", IdxA: i, IdxB: j, Cost: rc})
			}
		} else if math.Abs(td[i][j]-deleteCost) < epsilon {
			backtrack(i-1, j)
			ops = append(ops, EditOp{Op: "delete", IdxA: i, IdxB: -1, Cost: costFn.Delete(it1.nodes[i])})
		} else {
			backtrack(i, j-1)
			ops = append(ops, EditOp{Op: "insert", IdxA: -1, IdxB: j, Cost: costFn.Insert(it2.nodes[j])})
		}
	}

	backtrack(it1.size, it2.size)
	return ops
}

// newIndexedTree creates the indexed representation of a tree for Zhang-Shasha.
func newIndexedTree(root *TreeNode) *indexedTree {
	if root == nil {
		return &indexedTree{size: 0}
	}

	it := &indexedTree{}

	// Post-order traversal to index nodes (1-indexed)
	it.nodes = []*TreeNode{nil} // placeholder for 0-index
	var postOrder func(n *TreeNode)
	postOrder = func(n *TreeNode) {
		for _, child := range n.Children {
			postOrder(child)
		}
		it.nodes = append(it.nodes, n)
	}
	postOrder(root)
	it.size = len(it.nodes) - 1

	// Compute leftmost leaf descendants
	it.l = make([]int, it.size+1)
	nodeIdx := make(map[*TreeNode]int)
	for i := 1; i <= it.size; i++ {
		nodeIdx[it.nodes[i]] = i
	}

	var computeL func(n *TreeNode) int
	computeL = func(n *TreeNode) int {
		if len(n.Children) == 0 {
			return nodeIdx[n]
		}
		return computeL(n.Children[0])
	}

	for i := 1; i <= it.size; i++ {
		it.l[i] = computeL(it.nodes[i])
	}

	// Compute key roots: a node is a key root if no other node to its right
	// shares the same leftmost leaf descendant
	visited := make(map[int]bool)
	for i := it.size; i >= 1; i-- {
		if !visited[it.l[i]] {
			it.keyRoots = append(it.keyRoots, i)
			visited[it.l[i]] = true
		}
	}

	sortInts(it.keyRoots)
	return it
}

func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		key := a[i]
		j := i - 1
		for j >= 0 && a[j] > key {
			a[j+1] = a[j]
			j--
		}
		a[j+1] = key
	}
}

func min3(a, b, c float64) float64 {
	if a <= b && a <= c {
		return a
	}
	if b <= c {
		return b
	}
	return c
}
