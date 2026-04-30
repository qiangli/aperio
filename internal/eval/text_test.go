package eval

import (
	"math"
	"testing"
)

func approxEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"Hello, World!", []string{"hello", "world"}},
		{"the weather is 72°F", []string{"the", "weather", "is", "72", "f"}},
		{"", nil},
		{"  spaces  ", []string{"spaces"}},
	}
	for _, tt := range tests {
		got := Tokenize(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("Tokenize(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("Tokenize(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestNGrams(t *testing.T) {
	tokens := []string{"the", "cat", "sat", "on", "the", "mat"}
	bigrams := NGrams(tokens, 2)
	if len(bigrams) != 5 {
		t.Errorf("expected 5 bigrams, got %d", len(bigrams))
	}
	if bigrams[0] != "the cat" {
		t.Errorf("first bigram: %q", bigrams[0])
	}

	unigrams := NGrams(tokens, 1)
	if len(unigrams) != 6 {
		t.Errorf("expected 6 unigrams, got %d", len(unigrams))
	}

	empty := NGrams(tokens, 10)
	if len(empty) != 0 {
		t.Errorf("expected 0 ngrams for n > len, got %d", len(empty))
	}
}

func TestBLEUIdentical(t *testing.T) {
	score := BLEU("The weather is sunny", "The weather is sunny")
	if !approxEqual(score, 1.0, 0.01) {
		t.Errorf("BLEU of identical texts: expected ~1.0, got %f", score)
	}
}

func TestBLEUCompleteDifference(t *testing.T) {
	score := BLEU("cat dog bird", "apple banana cherry")
	if score != 0 {
		t.Errorf("BLEU of completely different texts: expected 0, got %f", score)
	}
}

func TestBLEUPartialOverlap(t *testing.T) {
	// High overlap sentence pair where all n-gram levels have matches
	score := BLEU("the weather in new york is sunny and warm today",
		"the weather in new york is clear and warm today")
	if score <= 0 || score >= 1.0 {
		t.Errorf("BLEU of partial overlap: expected between 0 and 1, got %f", score)
	}
}

func TestBLEUEmpty(t *testing.T) {
	if BLEU("", "") != 1.0 {
		t.Error("BLEU of two empty strings should be 1.0")
	}
	if BLEU("hello", "") != 0 {
		t.Error("BLEU with empty reference should be 0")
	}
	if BLEU("", "hello") != 0 {
		t.Error("BLEU with empty candidate should be 0")
	}
}

func TestROUGENIdentical(t *testing.T) {
	score := ROUGEN("The weather is sunny", "The weather is sunny", 1)
	if !approxEqual(score, 1.0, 0.01) {
		t.Errorf("ROUGE-1 of identical texts: expected ~1.0, got %f", score)
	}
}

func TestROUGENPartial(t *testing.T) {
	score := ROUGEN("the cat sat on mat", "the cat is on the mat", 1)
	if score <= 0 || score >= 1.0 {
		t.Errorf("ROUGE-1 partial: expected between 0 and 1, got %f", score)
	}
}

func TestROUGENEmpty(t *testing.T) {
	if ROUGEN("", "", 1) != 1.0 {
		t.Error("ROUGE-N of two empty strings should be 1.0")
	}
}

func TestROUGELIdentical(t *testing.T) {
	score := ROUGEL("The weather is sunny", "The weather is sunny")
	if !approxEqual(score, 1.0, 0.01) {
		t.Errorf("ROUGE-L of identical texts: expected ~1.0, got %f", score)
	}
}

func TestROUGELPartial(t *testing.T) {
	score := ROUGEL("the cat sat on the mat", "the dog sat on the mat")
	if score <= 0 || score >= 1.0 {
		t.Errorf("ROUGE-L partial: expected between 0 and 1, got %f", score)
	}
}

func TestROUGELEmpty(t *testing.T) {
	if ROUGEL("", "") != 1.0 {
		t.Error("ROUGE-L of two empty strings should be 1.0")
	}
	if ROUGEL("hello", "") != 0 {
		t.Error("ROUGE-L with one empty should be 0")
	}
}

func TestLevenshteinIdentical(t *testing.T) {
	score := Levenshtein("hello", "hello")
	if score != 1.0 {
		t.Errorf("Levenshtein identical: expected 1.0, got %f", score)
	}
}

func TestLevenshteinCompleteDiff(t *testing.T) {
	score := Levenshtein("abc", "xyz")
	if score >= 1.0 || score < 0 {
		t.Errorf("Levenshtein different: expected between 0 and 1, got %f", score)
	}
}

func TestLevenshteinEmpty(t *testing.T) {
	if Levenshtein("", "") != 1.0 {
		t.Error("Levenshtein of two empty strings should be 1.0")
	}
	if Levenshtein("hello", "") != 0 {
		t.Error("Levenshtein with one empty should be 0")
	}
}

func TestLevenshteinOneDiff(t *testing.T) {
	score := Levenshtein("kitten", "sitten")
	// 1 edit out of 6 chars → similarity = 5/6 ≈ 0.833
	if !approxEqual(score, 5.0/6.0, 0.01) {
		t.Errorf("Levenshtein one diff: expected ~0.833, got %f", score)
	}
}

func TestCosineSimilarityIdentical(t *testing.T) {
	score := CosineSimilarity("the cat sat on the mat", "the cat sat on the mat")
	if !approxEqual(score, 1.0, 0.01) {
		t.Errorf("Cosine identical: expected ~1.0, got %f", score)
	}
}

func TestCosineSimilarityDifferent(t *testing.T) {
	score := CosineSimilarity("apple banana", "cat dog")
	if score != 0 {
		t.Errorf("Cosine completely different: expected 0, got %f", score)
	}
}

func TestCosineSimilarityPartial(t *testing.T) {
	score := CosineSimilarity("the cat sat", "the dog sat")
	if score <= 0 || score >= 1.0 {
		t.Errorf("Cosine partial: expected between 0 and 1, got %f", score)
	}
}

func TestCosineSimilarityEmpty(t *testing.T) {
	if CosineSimilarity("", "") != 1.0 {
		t.Error("Cosine of two empty strings should be 1.0")
	}
	if CosineSimilarity("hello", "") != 0 {
		t.Error("Cosine with one empty should be 0")
	}
}
