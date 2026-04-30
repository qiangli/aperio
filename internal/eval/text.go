package eval

import (
	"math"
)

// BLEU computes the BLEU score between a candidate and reference text.
// Uses modified n-gram precision for n=1..4 with brevity penalty.
// Returns a score in [0, 1].
func BLEU(candidate, reference string) float64 {
	candTokens := Tokenize(candidate)
	refTokens := Tokenize(reference)

	if len(candTokens) == 0 || len(refTokens) == 0 {
		if len(candTokens) == 0 && len(refTokens) == 0 {
			return 1.0
		}
		return 0.0
	}

	// Compute modified precision for n=1..4
	maxN := 4
	if len(candTokens) < maxN {
		maxN = len(candTokens)
	}
	if len(refTokens) < maxN {
		maxN = len(refTokens)
	}
	if maxN == 0 {
		return 0.0
	}

	logPrecision := 0.0
	weight := 1.0 / float64(maxN)

	for n := 1; n <= maxN; n++ {
		p := modifiedPrecision(candTokens, refTokens, n)
		if p == 0 {
			return 0.0
		}
		logPrecision += weight * math.Log(p)
	}

	// Brevity penalty
	bp := 1.0
	if len(candTokens) < len(refTokens) {
		bp = math.Exp(1.0 - float64(len(refTokens))/float64(len(candTokens)))
	}

	return bp * math.Exp(logPrecision)
}

// modifiedPrecision computes clipped n-gram precision.
func modifiedPrecision(candidate, reference []string, n int) float64 {
	candGrams := NGrams(candidate, n)
	refGrams := NGrams(reference, n)

	if len(candGrams) == 0 {
		return 0
	}

	// Count n-grams in reference
	refCounts := make(map[string]int)
	for _, g := range refGrams {
		refCounts[g]++
	}

	// Count clipped matches
	clipped := 0
	candCounts := make(map[string]int)
	for _, g := range candGrams {
		candCounts[g]++
	}

	for gram, candCount := range candCounts {
		refCount := refCounts[gram]
		if refCount < candCount {
			clipped += refCount
		} else {
			clipped += candCount
		}
	}

	return float64(clipped) / float64(len(candGrams))
}

// ROUGEN computes ROUGE-N (recall-oriented n-gram overlap).
// Returns a score in [0, 1].
func ROUGEN(candidate, reference string, n int) float64 {
	candTokens := Tokenize(candidate)
	refTokens := Tokenize(reference)

	if len(refTokens) == 0 {
		if len(candTokens) == 0 {
			return 1.0
		}
		return 0.0
	}

	candGrams := NGrams(candTokens, n)
	refGrams := NGrams(refTokens, n)

	if len(refGrams) == 0 {
		return 0
	}

	// Count n-grams in candidate
	candCounts := make(map[string]int)
	for _, g := range candGrams {
		candCounts[g]++
	}

	// Count overlapping n-grams (recall-oriented: denominator is reference)
	overlap := 0
	refCounted := make(map[string]int)
	for _, g := range refGrams {
		refCounted[g]++
		if refCounted[g] <= candCounts[g] {
			overlap++
		}
	}

	return float64(overlap) / float64(len(refGrams))
}

// ROUGEL computes ROUGE-L using the longest common subsequence.
// Returns a score in [0, 1].
func ROUGEL(candidate, reference string) float64 {
	candTokens := Tokenize(candidate)
	refTokens := Tokenize(reference)

	if len(candTokens) == 0 && len(refTokens) == 0 {
		return 1.0
	}
	if len(candTokens) == 0 || len(refTokens) == 0 {
		return 0.0
	}

	lcsLen := lcs(candTokens, refTokens)

	// F-measure with equal weight to precision and recall
	precision := float64(lcsLen) / float64(len(candTokens))
	recall := float64(lcsLen) / float64(len(refTokens))

	if precision+recall == 0 {
		return 0
	}

	return 2 * precision * recall / (precision + recall)
}

// lcs computes the length of the longest common subsequence.
func lcs(a, b []string) int {
	m := len(a)
	n := len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				if dp[i-1][j] > dp[i][j-1] {
					dp[i][j] = dp[i-1][j]
				} else {
					dp[i][j] = dp[i][j-1]
				}
			}
		}
	}

	return dp[m][n]
}

// Levenshtein computes the normalized Levenshtein similarity between two strings.
// Returns a value in [0, 1] where 1 means identical.
func Levenshtein(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}

	runesA := []rune(a)
	runesB := []rune(b)
	m := len(runesA)
	n := len(runesB)

	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
		dp[i][0] = i
	}
	for j := 0; j <= n; j++ {
		dp[0][j] = j
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			cost := 1
			if runesA[i-1] == runesB[j-1] {
				cost = 0
			}
			dp[i][j] = minInt(
				dp[i-1][j]+1,
				minInt(dp[i][j-1]+1, dp[i-1][j-1]+cost),
			)
		}
	}

	maxLen := m
	if n > maxLen {
		maxLen = n
	}
	return 1.0 - float64(dp[m][n])/float64(maxLen)
}

// CosineSimilarity computes cosine similarity between two texts using TF-IDF vectors.
// Returns a value in [0, 1].
func CosineSimilarity(a, b string) float64 {
	tokensA := Tokenize(a)
	tokensB := Tokenize(b)

	if len(tokensA) == 0 && len(tokensB) == 0 {
		return 1.0
	}
	if len(tokensA) == 0 || len(tokensB) == 0 {
		return 0.0
	}

	// Build term frequency vectors
	tfA := termFreq(tokensA)
	tfB := termFreq(tokensB)

	// Compute cosine similarity using TF vectors (skip IDF for two-document comparison)
	var dotProduct, normA, normB float64
	allTerms := make(map[string]bool)
	for t := range tfA {
		allTerms[t] = true
	}
	for t := range tfB {
		allTerms[t] = true
	}

	for term := range allTerms {
		a := tfA[term]
		b := tfB[term]
		dotProduct += a * b
		normA += a * a
		normB += b * b
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

func termFreq(tokens []string) map[string]float64 {
	counts := make(map[string]int)
	for _, t := range tokens {
		counts[t]++
	}
	tf := make(map[string]float64, len(counts))
	for t, c := range counts {
		tf[t] = float64(c) / float64(len(tokens))
	}
	return tf
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
