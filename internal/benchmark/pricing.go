package benchmark

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ModelPrice holds per-1K-token pricing for a model.
type ModelPrice struct {
	Input  float64 `yaml:"input" json:"input"`
	Output float64 `yaml:"output" json:"output"`
}

// PricingTable holds pricing for multiple models.
type PricingTable struct {
	Models map[string]ModelPrice `yaml:"models" json:"models"`
}

// DefaultPricing returns the built-in pricing table.
func DefaultPricing() PricingTable {
	return PricingTable{
		Models: map[string]ModelPrice{
			"gpt-4":            {Input: 0.03, Output: 0.06},
			"gpt-4-turbo":      {Input: 0.01, Output: 0.03},
			"gpt-4o":           {Input: 0.005, Output: 0.015},
			"gpt-4o-mini":      {Input: 0.00015, Output: 0.0006},
			"gpt-3.5-turbo":    {Input: 0.0005, Output: 0.0015},
			"claude-3-opus":    {Input: 0.015, Output: 0.075},
			"claude-sonnet-4":  {Input: 0.003, Output: 0.015},
			"claude-3.5-sonnet": {Input: 0.003, Output: 0.015},
			"claude-3-haiku":   {Input: 0.00025, Output: 0.00125},
		},
	}
}

// LoadPricing reads a pricing table from a YAML file.
func LoadPricing(path string) (PricingTable, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PricingTable{}, fmt.Errorf("read pricing file: %w", err)
	}

	var pt PricingTable
	if err := yaml.Unmarshal(data, &pt); err != nil {
		return PricingTable{}, fmt.Errorf("parse pricing: %w", err)
	}
	if pt.Models == nil {
		// Try parsing as flat map
		var models map[string]ModelPrice
		if err := yaml.Unmarshal(data, &models); err == nil && len(models) > 0 {
			pt.Models = models
		}
	}
	return pt, nil
}

// MergePricing combines two pricing tables, with overlay taking precedence.
func MergePricing(base, overlay PricingTable) PricingTable {
	result := PricingTable{Models: make(map[string]ModelPrice)}
	for k, v := range base.Models {
		result.Models[k] = v
	}
	for k, v := range overlay.Models {
		result.Models[k] = v
	}
	return result
}

// Lookup finds the price for a model using longest-prefix matching.
// For example, "gpt-4o-2024-05-13" matches the "gpt-4o" entry.
func (pt PricingTable) Lookup(modelName string) (ModelPrice, bool) {
	modelName = strings.ToLower(modelName)

	// Exact match first
	if price, ok := pt.Models[modelName]; ok {
		return price, true
	}

	// Longest prefix match
	keys := make([]string, 0, len(pt.Models))
	for k := range pt.Models {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) > len(keys[j]) // longest first
	})

	for _, key := range keys {
		if strings.HasPrefix(modelName, key) {
			return pt.Models[key], true
		}
	}

	return ModelPrice{}, false
}

// ComputeCost calculates the cost for a given model and token counts.
func (pt PricingTable) ComputeCost(model string, inputTokens, outputTokens int) float64 {
	price, ok := pt.Lookup(model)
	if !ok {
		return 0
	}
	return (float64(inputTokens)/1000)*price.Input + (float64(outputTokens)/1000)*price.Output
}
