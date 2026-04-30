package benchmark

import "math/big"

// ComputePassAtK computes the unbiased pass@k estimator.
// Formula: pass@k = 1 - C(n-c, k) / C(n, k)
// where n = total trials, c = correct trials, k = desired k.
//
// This is the standard estimator from the Codex paper (Chen et al., 2021).
func ComputePassAtK(n, c, k int) float64 {
	if n <= 0 || k <= 0 {
		return 0
	}
	if c >= n {
		return 1.0
	}
	if k > n {
		k = n
	}
	if c == 0 {
		return 0
	}

	// Use big.Int for exact combinatorial computation to avoid overflow.
	// pass@k = 1 - C(n-c, k) / C(n, k)
	numerator := comb(n-c, k)
	denominator := comb(n, k)

	if denominator.Sign() == 0 {
		return 0
	}

	// Convert to float64
	ratio := new(big.Float).Quo(
		new(big.Float).SetInt(numerator),
		new(big.Float).SetInt(denominator),
	)

	f, _ := ratio.Float64()
	return 1.0 - f
}

// ComputePassKFromRuns computes pass@k from a slice of RunResults.
// A run passes if all its validation checks passed (PassRate == 1.0).
func ComputePassKFromRuns(runs []RunResult, k int) float64 {
	n := len(runs)
	if n == 0 {
		return 0
	}

	c := 0
	for _, run := range runs {
		if runPassed(run) {
			c++
		}
	}

	return ComputePassAtK(n, c, k)
}

// ComputeAllPassK computes pass@k for multiple k values.
func ComputeAllPassK(runs []RunResult, ks []int) map[int]float64 {
	result := make(map[int]float64, len(ks))
	for _, k := range ks {
		result[k] = ComputePassKFromRuns(runs, k)
	}
	return result
}

func runPassed(run RunResult) bool {
	if run.Error != "" {
		return false
	}
	return ComputePassRate(run.Validation) == 1.0
}

// comb computes C(n, k) using big.Int for arbitrary precision.
func comb(n, k int) *big.Int {
	if k < 0 || k > n {
		return big.NewInt(0)
	}
	if k == 0 || k == n {
		return big.NewInt(1)
	}
	// Use the smaller of k and n-k for efficiency
	if k > n-k {
		k = n - k
	}

	result := big.NewInt(1)
	for i := 0; i < k; i++ {
		result.Mul(result, big.NewInt(int64(n-i)))
		result.Div(result, big.NewInt(int64(i+1)))
	}
	return result
}
