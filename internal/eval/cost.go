package eval

import "github.com/qiangli/aperio/internal/trace"

// Default type weights: higher = more important to match.
var defaultTypeWeights = map[trace.SpanType]float64{
	trace.SpanLLMRequest:  1.0,
	trace.SpanLLMResponse: 1.0,
	trace.SpanToolCall:    1.0,
	trace.SpanToolResult:  1.0,
	trace.SpanUserInput:   1.0,
	trace.SpanAgentOutput: 1.0,
	trace.SpanExec:        0.5,
	trace.SpanFSRead:      0.5,
	trace.SpanFSWrite:     0.5,
	trace.SpanNetIO:       0.5,
	trace.SpanDBQuery:     0.5,
	trace.SpanFunction:    0.3,
}

// Semantic attributes that are compared for rename cost.
var semanticAttrs = []string{
	"tool_name",
	"llm.model",
	"command",
	"path",
	"http.url",
	"query",
}

// DefaultCostFn returns cost functions using domain-specific weights for trace spans.
// Nodes carry a composite label: "TYPE:name" where TYPE is the span type.
func DefaultCostFn() CostFn {
	return CostFn{
		Insert: func(n *TreeNode) float64 {
			return typeWeight(n)
		},
		Delete: func(n *TreeNode) float64 {
			return typeWeight(n)
		},
		Rename: func(a, b *TreeNode) float64 {
			if a.Label == b.Label {
				return 0
			}

			aType, aName := splitLabel(a.Label)
			bType, bName := splitLabel(b.Label)

			if aType != bType {
				return 1.0
			}

			// Same type, different name
			cost := 0.3

			// Additional cost for key attribute differences
			if a.Attrs != nil && b.Attrs != nil {
				attrDiff := compareSemanticAttrs(a.Attrs, b.Attrs)
				cost += attrDiff * 0.2
			}

			// Reduce cost if names are similar
			if aName == bName {
				cost = 0.1
			}

			return cost
		},
	}
}

// typeWeight returns the insert/delete cost for a node based on its span type.
func typeWeight(n *TreeNode) float64 {
	spanType, _ := splitLabel(n.Label)
	if w, ok := defaultTypeWeights[trace.SpanType(spanType)]; ok {
		return w
	}
	return 0.5
}

// splitLabel extracts type and name from a composite label "TYPE:name".
func splitLabel(label string) (string, string) {
	for i := 0; i < len(label); i++ {
		if label[i] == ':' {
			return label[:i], label[i+1:]
		}
	}
	return label, ""
}

// compareSemanticAttrs compares key semantic attributes between two nodes.
// Returns a value in [0, 1] representing the fraction of differing attributes.
func compareSemanticAttrs(a, b map[string]any) float64 {
	compared := 0
	differing := 0

	for _, key := range semanticAttrs {
		va, okA := a[key]
		vb, okB := b[key]
		if !okA && !okB {
			continue
		}
		compared++
		if !okA || !okB || va != vb {
			differing++
		}
	}

	if compared == 0 {
		return 0
	}
	return float64(differing) / float64(compared)
}
