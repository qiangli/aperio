package trace

import "sort"

// Merge combines function spans (from language-specific tracers) with API spans
// (from the proxy) into a unified trace. It establishes parent-child relationships
// by finding function spans whose time window contains API spans.
func Merge(apiSpans, funcSpans []*Span) []*Span {
	if len(funcSpans) == 0 {
		return apiSpans
	}
	if len(apiSpans) == 0 {
		return funcSpans
	}

	// Sort function spans by start time
	sort.Slice(funcSpans, func(i, j int) bool {
		return funcSpans[i].StartTime.Before(funcSpans[j].StartTime)
	})

	// For each API span, find the innermost function span that contains it
	for _, apiSpan := range apiSpans {
		if apiSpan.ParentID != "" {
			continue // already has a parent (e.g., from enrichment)
		}

		var bestParent *Span
		for _, funcSpan := range funcSpans {
			if funcSpan.StartTime.IsZero() || funcSpan.EndTime.IsZero() {
				continue
			}
			// Function span must contain the API span's time window
			if !funcSpan.StartTime.After(apiSpan.StartTime) && !funcSpan.EndTime.Before(apiSpan.EndTime) {
				// Prefer the innermost (most recent start time) containing span
				if bestParent == nil || funcSpan.StartTime.After(bestParent.StartTime) {
					bestParent = funcSpan
				}
			}
		}

		if bestParent != nil {
			apiSpan.ParentID = bestParent.ID
		}
	}

	// Combine all spans
	all := make([]*Span, 0, len(apiSpans)+len(funcSpans))
	all = append(all, funcSpans...)
	all = append(all, apiSpans...)

	// Sort by start time
	sort.Slice(all, func(i, j int) bool {
		return all[i].StartTime.Before(all[j].StartTime)
	})

	return all
}
