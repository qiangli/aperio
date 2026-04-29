package trace

import (
	"fmt"
	"strings"
)

// Difference represents a divergence between two traces.
type Difference struct {
	Type    string // "missing", "extra", "changed"
	SpanID  string
	Field   string
	Left    string
	Right   string
	Message string
}

// Diff compares two traces and returns the differences.
// It compares spans by sequence and type, focusing on LLM and tool call flows.
func Diff(left, right *Trace) []Difference {
	var diffs []Difference

	// Compare metadata
	if left.Metadata.Language != right.Metadata.Language {
		diffs = append(diffs, Difference{
			Type:    "changed",
			Field:   "metadata.language",
			Left:    left.Metadata.Language,
			Right:   right.Metadata.Language,
			Message: fmt.Sprintf("language changed: %s → %s", left.Metadata.Language, right.Metadata.Language),
		})
	}

	// Group spans by type for comparison
	leftByType := groupByType(left.Spans)
	rightByType := groupByType(right.Spans)

	// Compare span counts by type
	allTypes := make(map[SpanType]bool)
	for t := range leftByType {
		allTypes[t] = true
	}
	for t := range rightByType {
		allTypes[t] = true
	}

	for t := range allTypes {
		lCount := len(leftByType[t])
		rCount := len(rightByType[t])
		if lCount != rCount {
			diffs = append(diffs, Difference{
				Type:    "changed",
				Field:   fmt.Sprintf("span_count.%s", t),
				Left:    fmt.Sprintf("%d", lCount),
				Right:   fmt.Sprintf("%d", rCount),
				Message: fmt.Sprintf("%s span count: %d → %d", t, lCount, rCount),
			})
		}
	}

	// Compare LLM request sequence
	leftLLM := leftByType[SpanLLMRequest]
	rightLLM := rightByType[SpanLLMRequest]
	maxLLM := len(leftLLM)
	if len(rightLLM) > maxLLM {
		maxLLM = len(rightLLM)
	}

	for i := 0; i < maxLLM; i++ {
		if i >= len(leftLLM) {
			diffs = append(diffs, Difference{
				Type:    "extra",
				SpanID:  rightLLM[i].ID,
				Message: fmt.Sprintf("extra LLM request in right trace: %s", rightLLM[i].Name),
			})
			continue
		}
		if i >= len(rightLLM) {
			diffs = append(diffs, Difference{
				Type:    "missing",
				SpanID:  leftLLM[i].ID,
				Message: fmt.Sprintf("missing LLM request in right trace: %s", leftLLM[i].Name),
			})
			continue
		}

		if leftLLM[i].Name != rightLLM[i].Name {
			diffs = append(diffs, Difference{
				Type:    "changed",
				SpanID:  leftLLM[i].ID,
				Field:   "name",
				Left:    leftLLM[i].Name,
				Right:   rightLLM[i].Name,
				Message: fmt.Sprintf("LLM request[%d] name: %s → %s", i, leftLLM[i].Name, rightLLM[i].Name),
			})
		}
	}

	// Compare tool call sequence
	leftTools := leftByType[SpanToolCall]
	rightTools := rightByType[SpanToolCall]
	maxTools := len(leftTools)
	if len(rightTools) > maxTools {
		maxTools = len(rightTools)
	}

	for i := 0; i < maxTools; i++ {
		if i >= len(leftTools) {
			diffs = append(diffs, Difference{
				Type:    "extra",
				SpanID:  rightTools[i].ID,
				Message: fmt.Sprintf("extra tool call in right trace: %s", rightTools[i].Name),
			})
			continue
		}
		if i >= len(rightTools) {
			diffs = append(diffs, Difference{
				Type:    "missing",
				SpanID:  leftTools[i].ID,
				Message: fmt.Sprintf("missing tool call in right trace: %s", leftTools[i].Name),
			})
			continue
		}

		lName := getStringAttr(leftTools[i], "tool_name")
		rName := getStringAttr(rightTools[i], "tool_name")
		if lName != rName {
			diffs = append(diffs, Difference{
				Type:    "changed",
				SpanID:  leftTools[i].ID,
				Field:   "tool_name",
				Left:    lName,
				Right:   rName,
				Message: fmt.Sprintf("tool call[%d] name: %s → %s", i, lName, rName),
			})
		}
	}

	// Compare exec sequence
	diffs = append(diffs, compareSpanSequence(leftByType[SpanExec], rightByType[SpanExec], "exec", "command")...)

	// Compare filesystem operations
	diffs = append(diffs, compareSpanSequence(leftByType[SpanFSRead], rightByType[SpanFSRead], "fs_read", "path")...)
	diffs = append(diffs, compareSpanSequence(leftByType[SpanFSWrite], rightByType[SpanFSWrite], "fs_write", "path")...)

	// Compare network I/O
	diffs = append(diffs, compareSpanSequence(leftByType[SpanNetIO], rightByType[SpanNetIO], "net_io", "url")...)

	// Compare database queries
	diffs = append(diffs, compareSpanSequence(leftByType[SpanDBQuery], rightByType[SpanDBQuery], "db_query", "query")...)

	return diffs
}

// compareSpanSequence compares two ordered lists of spans by a key attribute.
func compareSpanSequence(left, right []*Span, label, attrKey string) []Difference {
	var diffs []Difference
	maxLen := len(left)
	if len(right) > maxLen {
		maxLen = len(right)
	}

	for i := 0; i < maxLen; i++ {
		if i >= len(left) {
			diffs = append(diffs, Difference{
				Type:    "extra",
				SpanID:  right[i].ID,
				Message: fmt.Sprintf("extra %s in right trace: %s", label, right[i].Name),
			})
			continue
		}
		if i >= len(right) {
			diffs = append(diffs, Difference{
				Type:    "missing",
				SpanID:  left[i].ID,
				Message: fmt.Sprintf("missing %s in right trace: %s", label, left[i].Name),
			})
			continue
		}

		lVal := getStringAttr(left[i], attrKey)
		rVal := getStringAttr(right[i], attrKey)
		if lVal != "" && rVal != "" && lVal != rVal {
			diffs = append(diffs, Difference{
				Type:    "changed",
				SpanID:  left[i].ID,
				Field:   attrKey,
				Left:    lVal,
				Right:   rVal,
				Message: fmt.Sprintf("%s[%d] %s: %s → %s", label, i, attrKey, lVal, rVal),
			})
		}
	}
	return diffs
}

// FormatDiff returns a human-readable diff summary.
func FormatDiff(diffs []Difference) string {
	if len(diffs) == 0 {
		return "Traces are identical (in terms of LLM and tool call flow)"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Found %d difference(s):\n\n", len(diffs))

	for _, d := range diffs {
		switch d.Type {
		case "missing":
			fmt.Fprintf(&b, "  - MISSING: %s\n", d.Message)
		case "extra":
			fmt.Fprintf(&b, "  + EXTRA:   %s\n", d.Message)
		case "changed":
			fmt.Fprintf(&b, "  ~ CHANGED: %s\n", d.Message)
		}
	}

	return b.String()
}

func groupByType(spans []*Span) map[SpanType][]*Span {
	m := make(map[SpanType][]*Span)
	for _, s := range spans {
		m[s.Type] = append(m[s.Type], s)
	}
	return m
}
