package eval

import (
	"strings"
	"unicode"
)

// Tokenize splits text into lowercase tokens, splitting on whitespace and punctuation.
func Tokenize(text string) []string {
	text = strings.ToLower(text)
	var tokens []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// NGrams generates n-grams from a slice of tokens.
func NGrams(tokens []string, n int) []string {
	if n <= 0 || len(tokens) < n {
		return nil
	}
	grams := make([]string, 0, len(tokens)-n+1)
	for i := 0; i <= len(tokens)-n; i++ {
		grams = append(grams, strings.Join(tokens[i:i+n], " "))
	}
	return grams
}
