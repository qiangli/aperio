package eval

import (
	"math"
	"testing"
)

func TestSplitLabel(t *testing.T) {
	tests := []struct {
		label    string
		wantType string
		wantName string
	}{
		{"FUNCTION:main", "FUNCTION", "main"},
		{"LLM_REQUEST:POST https://api.openai.com", "LLM_REQUEST", "POST https://api.openai.com"},
		{"TOOL_CALL:", "TOOL_CALL", ""},
		{"nocolon", "nocolon", ""},
	}

	for _, tt := range tests {
		gotType, gotName := splitLabel(tt.label)
		if gotType != tt.wantType || gotName != tt.wantName {
			t.Errorf("splitLabel(%q) = (%q, %q), want (%q, %q)",
				tt.label, gotType, gotName, tt.wantType, tt.wantName)
		}
	}
}

func TestDefaultCostFnMatchingLabels(t *testing.T) {
	costFn := DefaultCostFn()
	a := &TreeNode{Label: "FUNCTION:main"}
	b := &TreeNode{Label: "FUNCTION:main"}

	if costFn.Rename(a, b) != 0 {
		t.Errorf("expected 0 for matching labels, got %f", costFn.Rename(a, b))
	}
}

func TestDefaultCostFnDifferentTypes(t *testing.T) {
	costFn := DefaultCostFn()
	a := &TreeNode{Label: "FUNCTION:main"}
	b := &TreeNode{Label: "LLM_REQUEST:api"}

	rc := costFn.Rename(a, b)
	if rc != 1.0 {
		t.Errorf("expected 1.0 for different types, got %f", rc)
	}
}

func TestDefaultCostFnSameTypeDiffName(t *testing.T) {
	costFn := DefaultCostFn()
	a := &TreeNode{Label: "FUNCTION:main"}
	b := &TreeNode{Label: "FUNCTION:helper"}

	rc := costFn.Rename(a, b)
	if rc < 0.2 || rc > 0.6 {
		t.Errorf("expected rename cost between 0.2-0.6 for same type diff name, got %f", rc)
	}
}

func TestDefaultCostFnInsertDelete(t *testing.T) {
	costFn := DefaultCostFn()

	llm := &TreeNode{Label: "LLM_REQUEST:api"}
	fn := &TreeNode{Label: "FUNCTION:main"}

	if math.Abs(costFn.Insert(llm)-1.0) > 1e-10 {
		t.Errorf("expected insert cost 1.0 for LLM_REQUEST, got %f", costFn.Insert(llm))
	}
	if math.Abs(costFn.Delete(fn)-0.3) > 1e-10 {
		t.Errorf("expected delete cost 0.3 for FUNCTION, got %f", costFn.Delete(fn))
	}
}

func TestCompareSemanticAttrs(t *testing.T) {
	a := map[string]any{"tool_name": "get_weather", "command": "curl"}
	b := map[string]any{"tool_name": "get_weather", "command": "wget"}

	diff := compareSemanticAttrs(a, b)
	if diff != 0.5 {
		t.Errorf("expected 0.5 (1/2 differ), got %f", diff)
	}
}

func TestCompareSemanticAttrsAllSame(t *testing.T) {
	a := map[string]any{"tool_name": "x"}
	b := map[string]any{"tool_name": "x"}

	diff := compareSemanticAttrs(a, b)
	if diff != 0 {
		t.Errorf("expected 0, got %f", diff)
	}
}

func TestCompareSemanticAttrsNoOverlap(t *testing.T) {
	a := map[string]any{"foo": "bar"}
	b := map[string]any{"baz": "qux"}

	diff := compareSemanticAttrs(a, b)
	if diff != 0 {
		t.Errorf("expected 0 (no semantic keys), got %f", diff)
	}
}

func TestTypeWeight(t *testing.T) {
	llm := &TreeNode{Label: "LLM_REQUEST:api"}
	fn := &TreeNode{Label: "FUNCTION:main"}
	unknown := &TreeNode{Label: "UNKNOWN:x"}

	if math.Abs(typeWeight(llm)-1.0) > 1e-10 {
		t.Errorf("LLM_REQUEST weight: expected 1.0, got %f", typeWeight(llm))
	}
	if math.Abs(typeWeight(fn)-0.3) > 1e-10 {
		t.Errorf("FUNCTION weight: expected 0.3, got %f", typeWeight(fn))
	}
	if math.Abs(typeWeight(unknown)-0.5) > 1e-10 {
		t.Errorf("UNKNOWN weight: expected 0.5, got %f", typeWeight(unknown))
	}
}
