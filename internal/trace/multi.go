package trace

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MergeTraces combines multiple correlated traces into a single unified trace.
// It resolves cross-process parent-child links using correlation metadata and
// namespaces span IDs to avoid collisions.
func MergeTraces(traces []*Trace) (*Trace, error) {
	if len(traces) == 0 {
		return nil, fmt.Errorf("no traces to merge")
	}
	if len(traces) == 1 {
		return traces[0], nil
	}

	// Determine correlation ID (prefer the one most traces share)
	correlationID := findCorrelationID(traces)

	// Create the merged trace
	merged := &Trace{
		ID:        uuid.New().String(),
		CreatedAt: time.Now(),
		Metadata: Metadata{
			Command:       "merged",
			Language:      "multi",
			WorkingDir:    traces[0].Metadata.WorkingDir,
			CorrelationID: correlationID,
		},
	}

	// Build a map of trace ID → process span ID for cross-linking
	traceProcessSpans := make(map[string]string) // traceID → namespaced process span ID
	for _, t := range traces {
		for _, s := range t.Spans {
			if s.Type == SpanProcess {
				traceProcessSpans[t.ID] = namespaceID(t.ID, s.ID)
				break
			}
		}
	}

	// Namespace all span IDs and resolve cross-process links
	for _, t := range traces {
		for _, s := range t.Spans {
			newSpan := &Span{
				ID:         namespaceID(t.ID, s.ID),
				ParentID:   namespaceID(t.ID, s.ParentID),
				Type:       s.Type,
				Name:       s.Name,
				StartTime:  s.StartTime,
				EndTime:    s.EndTime,
				Attributes: copyAttrs(s.Attributes),
			}

			// Add source agent info
			if newSpan.Attributes == nil {
				newSpan.Attributes = make(map[string]any)
			}
			newSpan.Attributes["agent.source_trace"] = t.ID
			if t.Metadata.AgentRole != "" {
				newSpan.Attributes["agent.role"] = t.Metadata.AgentRole
			}

			// Resolve cross-process parent links
			if s.Type == SpanProcess && t.Metadata.ParentTraceID != "" && t.Metadata.ParentSpanID != "" {
				// This process was spawned by another trace — link to parent's span
				parentNS := namespaceID(t.Metadata.ParentTraceID, t.Metadata.ParentSpanID)
				// Check if the parent span exists in our merged set
				if _, ok := traceProcessSpans[t.Metadata.ParentTraceID]; ok {
					newSpan.ParentID = parentNS
				}
			}

			merged.Spans = append(merged.Spans, newSpan)
		}
	}

	// If there are multiple root PROCESS spans with no parent, create a synthetic root
	roots := findRootSpans(merged.Spans)
	if len(roots) > 1 {
		rootSpan := &Span{
			ID:        uuid.New().String(),
			Type:      SpanProcess,
			Name:      "multi-agent execution",
			StartTime: earliestStart(merged.Spans),
			EndTime:   latestEnd(merged.Spans),
			Attributes: map[string]any{
				"correlation.id": correlationID,
				"agent.count":    len(traces),
			},
		}
		for _, s := range roots {
			s.ParentID = rootSpan.ID
		}
		merged.Spans = append([]*Span{rootSpan}, merged.Spans...)
	}

	return merged, nil
}

func namespaceID(traceID, spanID string) string {
	if spanID == "" {
		return ""
	}
	// Use short prefix to keep IDs readable
	prefix := traceID
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	return prefix + ":" + spanID
}

func findCorrelationID(traces []*Trace) string {
	counts := make(map[string]int)
	for _, t := range traces {
		if t.Metadata.CorrelationID != "" {
			counts[t.Metadata.CorrelationID]++
		}
	}
	best := ""
	bestCount := 0
	for id, count := range counts {
		if count > bestCount {
			best = id
			bestCount = count
		}
	}
	if best == "" {
		return uuid.New().String()
	}
	return best
}

func findRootSpans(spans []*Span) []*Span {
	var roots []*Span
	for _, s := range spans {
		if s.ParentID == "" {
			roots = append(roots, s)
		}
	}
	return roots
}

func earliestStart(spans []*Span) time.Time {
	if len(spans) == 0 {
		return time.Now()
	}
	earliest := spans[0].StartTime
	for _, s := range spans[1:] {
		if s.StartTime.Before(earliest) {
			earliest = s.StartTime
		}
	}
	return earliest
}

func latestEnd(spans []*Span) time.Time {
	if len(spans) == 0 {
		return time.Now()
	}
	latest := spans[0].EndTime
	for _, s := range spans[1:] {
		if s.EndTime.After(latest) {
			latest = s.EndTime
		}
	}
	return latest
}

func copyAttrs(attrs map[string]any) map[string]any {
	if attrs == nil {
		return nil
	}
	cp := make(map[string]any, len(attrs))
	for k, v := range attrs {
		cp[k] = v
	}
	return cp
}
