package benchmark

import (
	"math"
	"testing"
)

func TestComputePassAtK(t *testing.T) {
	tests := []struct {
		name     string
		n, c, k  int
		expected float64
	}{
		{"all pass k=1", 10, 10, 1, 1.0},
		{"none pass", 10, 0, 1, 0.0},
		{"7/10 pass@1", 10, 7, 1, 0.7},
		{"7/10 pass@3", 10, 7, 3, 0.9916666666666667},
		{"1/10 pass@1", 10, 1, 1, 0.1},
		{"1/10 pass@5", 10, 1, 5, 0.5},
		{"5/10 pass@1", 10, 5, 1, 0.5},
		{"5/10 pass@5", 10, 5, 5, 0.9960317460317460},
		{"k > n", 5, 3, 10, 1.0},
		{"n=0", 0, 0, 1, 0.0},
		{"k=0", 10, 5, 0, 0.0},
		{"1/1 pass@1", 1, 1, 1, 1.0},
		{"0/1 pass@1", 1, 0, 1, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputePassAtK(tt.n, tt.c, tt.k)
			if math.Abs(got-tt.expected) > 1e-10 {
				t.Errorf("ComputePassAtK(%d, %d, %d) = %v, want %v", tt.n, tt.c, tt.k, got, tt.expected)
			}
		})
	}
}

func TestComputePassKFromRuns(t *testing.T) {
	// 3 runs, 2 pass
	runs := []RunResult{
		{Validation: []ValidationResult{{Passed: true}, {Passed: true}}},
		{Validation: []ValidationResult{{Passed: true}, {Passed: false}}},
		{Validation: []ValidationResult{{Passed: true}, {Passed: true}}},
	}

	got := ComputePassKFromRuns(runs, 1)
	// n=3, c=2, k=1 → pass@1 = 1 - C(1,1)/C(3,1) = 1 - 1/3 = 0.6667
	expected := 2.0 / 3.0
	if math.Abs(got-expected) > 1e-10 {
		t.Errorf("ComputePassKFromRuns = %v, want %v", got, expected)
	}
}

func TestComputeAllPassK(t *testing.T) {
	// 5 runs, 3 pass
	runs := []RunResult{
		{Validation: []ValidationResult{{Passed: true}}},
		{Validation: []ValidationResult{{Passed: true}}},
		{Validation: []ValidationResult{{Passed: true}}},
		{Validation: []ValidationResult{{Passed: false}}},
		{Error: "failed"},
	}

	result := ComputeAllPassK(runs, []int{1, 3, 5})

	if math.Abs(result[1]-0.6) > 1e-10 {
		t.Errorf("pass@1 = %v, want 0.6", result[1])
	}
	if result[5] != 1.0 {
		t.Errorf("pass@5 = %v, want 1.0 (k=n and c>0)", result[5])
	}
}

func TestComb(t *testing.T) {
	tests := []struct {
		n, k     int
		expected int64
	}{
		{10, 3, 120},
		{5, 2, 10},
		{5, 0, 1},
		{5, 5, 1},
		{0, 0, 1},
		{10, 11, 0},
	}

	for _, tt := range tests {
		got := comb(tt.n, tt.k).Int64()
		if got != tt.expected {
			t.Errorf("comb(%d, %d) = %d, want %d", tt.n, tt.k, got, tt.expected)
		}
	}
}
