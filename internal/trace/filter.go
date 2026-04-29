package trace

import (
	"path/filepath"
	"strings"
)

// FilterConfig controls which function spans to keep.
type FilterConfig struct {
	// IncludeDirs are directories whose files should be traced.
	IncludeDirs []string

	// ExcludePatterns are substrings to exclude from function names or file paths.
	ExcludePatterns []string

	// MaxDepth limits the call tree depth (0 = unlimited).
	MaxDepth int
}

// Filter removes function spans that don't match the filter criteria.
// Non-FUNCTION spans are always kept.
func Filter(spans []*Span, cfg FilterConfig) []*Span {
	if len(cfg.IncludeDirs) == 0 && len(cfg.ExcludePatterns) == 0 && cfg.MaxDepth == 0 {
		return spans
	}

	var result []*Span
	depths := make(map[string]int) // span ID -> depth

	for _, s := range spans {
		if s.Type != SpanFunction {
			result = append(result, s)
			continue
		}

		// Check depth
		if cfg.MaxDepth > 0 {
			depth := 1
			if s.ParentID != "" {
				if pd, ok := depths[s.ParentID]; ok {
					depth = pd + 1
				}
			}
			depths[s.ID] = depth
			if depth > cfg.MaxDepth {
				continue
			}
		}

		// Check include dirs
		if len(cfg.IncludeDirs) > 0 {
			filename, _ := s.Attributes["filename"].(string)
			if filename != "" && !matchesAnyDir(filename, cfg.IncludeDirs) {
				continue
			}
		}

		// Check exclude patterns
		if matchesAnyPattern(s.Name, cfg.ExcludePatterns) {
			continue
		}
		filename, _ := s.Attributes["filename"].(string)
		if filename != "" && matchesAnyPattern(filename, cfg.ExcludePatterns) {
			continue
		}

		result = append(result, s)
	}

	return result
}

func matchesAnyDir(filename string, dirs []string) bool {
	for _, dir := range dirs {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		absFile, err := filepath.Abs(filename)
		if err != nil {
			continue
		}
		if strings.HasPrefix(absFile, absDir) {
			return true
		}
	}
	return false
}

func matchesAnyPattern(s string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}
