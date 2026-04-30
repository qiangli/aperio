package benchmark

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPricing(t *testing.T) {
	pt := DefaultPricing()
	if len(pt.Models) != 9 {
		t.Errorf("expected 9 models, got %d", len(pt.Models))
	}
	if _, ok := pt.Models["gpt-4"]; !ok {
		t.Error("missing gpt-4")
	}
	if _, ok := pt.Models["claude-sonnet-4"]; !ok {
		t.Error("missing claude-sonnet-4")
	}
}

func TestLookupExact(t *testing.T) {
	pt := DefaultPricing()
	price, ok := pt.Lookup("gpt-4")
	if !ok {
		t.Fatal("expected to find gpt-4")
	}
	if price.Input != 0.03 {
		t.Errorf("expected input 0.03, got %f", price.Input)
	}
}

func TestLookupPrefix(t *testing.T) {
	pt := DefaultPricing()
	price, ok := pt.Lookup("gpt-4o-2024-05-13")
	if !ok {
		t.Fatal("expected prefix match for gpt-4o-2024-05-13")
	}
	if price.Input != 0.005 {
		t.Errorf("expected gpt-4o pricing (0.005), got %f", price.Input)
	}
}

func TestLookupLongestPrefix(t *testing.T) {
	pt := DefaultPricing()
	// "gpt-4o-mini" should match "gpt-4o-mini" not "gpt-4o" or "gpt-4"
	price, ok := pt.Lookup("gpt-4o-mini-2024")
	if !ok {
		t.Fatal("expected match")
	}
	if price.Input != 0.00015 {
		t.Errorf("expected gpt-4o-mini pricing (0.00015), got %f", price.Input)
	}
}

func TestLookupNotFound(t *testing.T) {
	pt := DefaultPricing()
	_, ok := pt.Lookup("unknown-model")
	if ok {
		t.Error("expected no match for unknown model")
	}
}

func TestLookupCaseInsensitive(t *testing.T) {
	pt := DefaultPricing()
	_, ok := pt.Lookup("GPT-4")
	if !ok {
		t.Error("expected case-insensitive match")
	}
}

func TestMergePricing(t *testing.T) {
	base := DefaultPricing()
	overlay := PricingTable{
		Models: map[string]ModelPrice{
			"gpt-4":        {Input: 0.01, Output: 0.02}, // override
			"custom-model": {Input: 0.1, Output: 0.2},   // new
		},
	}

	merged := MergePricing(base, overlay)
	if merged.Models["gpt-4"].Input != 0.01 {
		t.Error("overlay should override base")
	}
	if _, ok := merged.Models["custom-model"]; !ok {
		t.Error("new model from overlay should be present")
	}
	if _, ok := merged.Models["claude-3-haiku"]; !ok {
		t.Error("base model should be preserved")
	}
}

func TestLoadPricing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pricing.yaml")
	content := `models:
  my-model:
    input: 0.01
    output: 0.02
`
	os.WriteFile(path, []byte(content), 0644)

	pt, err := LoadPricing(path)
	if err != nil {
		t.Fatalf("LoadPricing: %v", err)
	}
	if pt.Models["my-model"].Input != 0.01 {
		t.Errorf("expected input 0.01, got %f", pt.Models["my-model"].Input)
	}
}

func TestLoadPricingMissing(t *testing.T) {
	_, err := LoadPricing("/nonexistent/pricing.yaml")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestComputeCost(t *testing.T) {
	pt := DefaultPricing()
	// gpt-4: input=0.03/1K, output=0.06/1K
	// 1000 input + 500 output = 0.03 + 0.03 = 0.06
	cost := pt.ComputeCost("gpt-4", 1000, 500)
	if math.Abs(cost-0.06) > 0.0001 {
		t.Errorf("expected cost 0.06, got %f", cost)
	}
}

func TestComputeCostUnknownModel(t *testing.T) {
	pt := DefaultPricing()
	cost := pt.ComputeCost("unknown", 1000, 500)
	if cost != 0 {
		t.Errorf("expected 0 for unknown model, got %f", cost)
	}
}
