package proxy

import (
	"encoding/json"
	"regexp"
	"strings"
)

// NormalizeConfig specifies which fields to normalize in responses.
type NormalizeConfig struct {
	// StripTimestamps removes "created" fields from JSON responses.
	StripTimestamps bool
	// StripIDs replaces volatile ID fields with a placeholder.
	StripIDs bool
	// StripHeaders removes volatile response headers.
	StripHeaders []string
}

// DefaultNormalizeConfig returns a normalization config suitable for LLM API responses.
func DefaultNormalizeConfig() NormalizeConfig {
	return NormalizeConfig{
		StripTimestamps: true,
		StripIDs:        true,
		StripHeaders:    []string{"x-request-id", "x-ratelimit-remaining", "x-ratelimit-reset", "date"},
	}
}

// NormalizeResponse applies normalization to a response body.
func NormalizeResponse(body []byte, cfg NormalizeConfig) []byte {
	// Try to parse as JSON
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return body
	}

	if cfg.StripTimestamps {
		delete(data, "created")
	}

	if cfg.StripIDs {
		normalizeIDs(data)
	}

	normalized, err := json.Marshal(data)
	if err != nil {
		return body
	}
	return normalized
}

// NormalizeHeaders applies header normalization.
func NormalizeHeaders(headers map[string]string, cfg NormalizeConfig) map[string]string {
	if len(cfg.StripHeaders) == 0 {
		return headers
	}

	result := make(map[string]string, len(headers))
	strip := make(map[string]bool, len(cfg.StripHeaders))
	for _, h := range cfg.StripHeaders {
		strip[strings.ToLower(h)] = true
	}

	for k, v := range headers {
		if !strip[strings.ToLower(k)] {
			result[k] = v
		}
	}
	return result
}

// idPattern matches common volatile ID formats.
var idPattern = regexp.MustCompile(`^(chatcmpl-|msg_|call_)[a-zA-Z0-9]+$`)

func normalizeIDs(data map[string]any) {
	for key, val := range data {
		switch v := val.(type) {
		case string:
			if key == "id" && idPattern.MatchString(v) {
				data[key] = "<normalized>"
			}
		case map[string]any:
			normalizeIDs(v)
		case []any:
			for _, item := range v {
				if m, ok := item.(map[string]any); ok {
					normalizeIDs(m)
				}
			}
		}
	}
}

// CanonicalizeRequestBody produces a canonical form of a JSON request body
// for use in body-aware matching. Keys are sorted and volatile fields are removed.
func CanonicalizeRequestBody(body []byte) []byte {
	var data any
	if err := json.Unmarshal(body, &data); err != nil {
		return body
	}

	stripVolatileFields(data)

	canonical, err := json.Marshal(data)
	if err != nil {
		return body
	}
	return canonical
}

func stripVolatileFields(v any) {
	switch val := v.(type) {
	case map[string]any:
		// Remove fields that vary between requests
		delete(val, "stream_options")
		for _, child := range val {
			stripVolatileFields(child)
		}
	case []any:
		for _, item := range val {
			stripVolatileFields(item)
		}
	}
}
