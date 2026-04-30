// Package eval provides trace evaluation and similarity comparison using
// tree edit distance algorithms.
//
// This package re-exports types and functions from the internal eval package:
//
//	import (
//	    "github.com/qiangli/aperio/eval"
//	    "github.com/qiangli/aperio/trace"
//	)
//
//	result := eval.Evaluate(leftGraph, rightGraph, nil)
//	fmt.Println(eval.FormatText(result, true))
package eval

import (
	ieval "github.com/qiangli/aperio/internal/eval"
)

// Core types.
type (
	// EvalResult holds the complete evaluation output.
	EvalResult = ieval.EvalResult

	// TypeEvalResult shows per-type evaluation details.
	TypeEvalResult = ieval.TypeEvalResult

	// EditOperation is the user-facing representation of an edit operation.
	EditOperation = ieval.EditOperation

	// EvalConfig configures the evaluation.
	EvalConfig = ieval.EvalConfig

	// SemanticEvalResult holds the results of semantic text comparison.
	SemanticEvalResult = ieval.SemanticEvalResult

	// TextPairScore holds text similarity scores for a single matched pair.
	TextPairScore = ieval.TextPairScore

	// TextMetrics holds individual text similarity metrics.
	TextMetrics = ieval.TextMetrics
)

// Functions.
var (
	// Evaluate compares two trace graphs and returns a detailed evaluation result.
	Evaluate = ieval.Evaluate

	// FormatText returns a human-readable text representation of the evaluation result.
	FormatText = ieval.FormatText

	// FormatJSON serializes the evaluation result to JSON.
	FormatJSON = ieval.FormatJSON
)
