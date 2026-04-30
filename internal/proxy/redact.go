package proxy

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// RedactionRule defines a single redaction pattern.
type RedactionRule struct {
	Name        string   `yaml:"name" json:"name"`
	Pattern     string   `yaml:"pattern" json:"pattern"`
	Replacement string   `yaml:"replacement" json:"replacement"`
	FieldPaths  []string `yaml:"field_paths,omitempty" json:"field_paths,omitempty"`

	compiled *regexp.Regexp
}

// RedactionConfig defines the complete redaction configuration.
type RedactionConfig struct {
	Rules      []RedactionRule `yaml:"rules" json:"rules"`
	BuiltinPII bool           `yaml:"builtin_pii" json:"builtin_pii"`
}

// Redactor applies redaction rules to strings, JSON bodies, and headers.
type Redactor struct {
	rules []compiledRule
}

type compiledRule struct {
	name        string
	pattern     *regexp.Regexp
	replacement string
	fieldPaths  []string
}

// Built-in PII patterns compiled once.
var builtinPatterns = []RedactionRule{
	{
		Name:        "email",
		Pattern:     `[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`,
		Replacement: "[REDACTED-EMAIL]",
	},
	{
		Name:        "phone",
		Pattern:     `\b\d{3}[-.]?\d{3}[-.]?\d{4}\b`,
		Replacement: "[REDACTED-PHONE]",
	},
	{
		Name:        "ip_address",
		Pattern:     `\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`,
		Replacement: "[REDACTED-IP]",
	},
	{
		Name:        "credit_card",
		Pattern:     `\b\d{4}[-\s]?\d{4}[-\s]?\d{4}[-\s]?\d{4}\b`,
		Replacement: "[REDACTED-CC]",
	},
	{
		Name:        "ssn",
		Pattern:     `\b\d{3}-\d{2}-\d{4}\b`,
		Replacement: "[REDACTED-SSN]",
	},
	{
		Name:        "api_key",
		Pattern:     `(sk-[a-zA-Z0-9]{20,}|AKIA[A-Z0-9]{16})`,
		Replacement: "[REDACTED-KEY]",
	},
	{
		Name:        "jwt",
		Pattern:     `eyJ[a-zA-Z0-9_-]+\.eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+`,
		Replacement: "[REDACTED-JWT]",
	},
}

// NewRedactor creates a Redactor from the given configuration.
func NewRedactor(cfg RedactionConfig) (*Redactor, error) {
	var rules []compiledRule

	if cfg.BuiltinPII {
		for _, bp := range builtinPatterns {
			re, err := regexp.Compile(bp.Pattern)
			if err != nil {
				return nil, fmt.Errorf("compile builtin pattern %q: %w", bp.Name, err)
			}
			rules = append(rules, compiledRule{
				name:        bp.Name,
				pattern:     re,
				replacement: bp.Replacement,
			})
		}
	}

	for _, r := range cfg.Rules {
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return nil, fmt.Errorf("compile rule %q pattern %q: %w", r.Name, r.Pattern, err)
		}
		replacement := r.Replacement
		if replacement == "" {
			replacement = "[REDACTED]"
		}
		rules = append(rules, compiledRule{
			name:        r.Name,
			pattern:     re,
			replacement: replacement,
			fieldPaths:  r.FieldPaths,
		})
	}

	return &Redactor{rules: rules}, nil
}

// RedactString applies all redaction rules to a plain string.
func (r *Redactor) RedactString(s string) string {
	for _, rule := range r.rules {
		// Rules with field paths only apply to targeted JSON fields
		if len(rule.fieldPaths) > 0 {
			continue
		}
		s = rule.pattern.ReplaceAllString(s, rule.replacement)
	}
	return s
}

// RedactJSON parses JSON data, walks all string values, and applies redaction patterns.
// Rules with FieldPaths are only applied to matching paths; rules without FieldPaths
// are applied to all string values.
func (r *Redactor) RedactJSON(data []byte) ([]byte, error) {
	var obj any
	if err := json.Unmarshal(data, &obj); err != nil {
		// Not valid JSON; apply string-level redaction
		return []byte(r.RedactString(string(data))), nil
	}

	r.walkJSON(obj, "")

	return json.Marshal(obj)
}

func (r *Redactor) walkJSON(v any, path string) {
	switch val := v.(type) {
	case map[string]any:
		for k, child := range val {
			childPath := path + "." + k
			if path == "" {
				childPath = k
			}
			if s, ok := child.(string); ok {
				val[k] = r.redactField(s, childPath)
			} else {
				r.walkJSON(child, childPath)
			}
		}
	case []any:
		for i, child := range val {
			childPath := fmt.Sprintf("%s[*]", path)
			if s, ok := child.(string); ok {
				val[i] = r.redactField(s, childPath)
			} else {
				r.walkJSON(child, childPath)
			}
		}
	}
}

func (r *Redactor) redactField(s, path string) string {
	for _, rule := range r.rules {
		if len(rule.fieldPaths) > 0 {
			if !matchesFieldPath(path, rule.fieldPaths) {
				continue
			}
		}
		s = rule.pattern.ReplaceAllString(s, rule.replacement)
	}
	return s
}

// matchesFieldPath checks if the current JSON path matches any of the given patterns.
// Supports wildcards: [*] matches any array index, * matches any key segment.
func matchesFieldPath(path string, patterns []string) bool {
	for _, pattern := range patterns {
		if pathMatchesPattern(path, pattern) {
			return true
		}
	}
	return false
}

func pathMatchesPattern(path, pattern string) bool {
	// Normalize: split on '.' but preserve [*]
	pathParts := splitPath(path)
	patternParts := splitPath(pattern)

	if len(pathParts) != len(patternParts) {
		return false
	}

	for i, pp := range patternParts {
		if pp == "*" {
			continue
		}
		if pp != pathParts[i] {
			return false
		}
	}
	return true
}

func splitPath(p string) []string {
	// Split on '.' but keep [*] attached to the previous segment
	var parts []string
	for _, seg := range strings.Split(p, ".") {
		parts = append(parts, seg)
	}
	return parts
}

// RedactHeaders applies redaction to HTTP header values.
// Always redacts authorization and x-api-key headers.
// Applies configured rules to all other header values.
func (r *Redactor) RedactHeaders(headers map[string]string) map[string]string {
	result := make(map[string]string, len(headers))
	for k, v := range headers {
		lower := strings.ToLower(k)
		if lower == "authorization" || lower == "x-api-key" {
			result[k] = "[REDACTED]"
		} else {
			result[k] = r.RedactString(v)
		}
	}
	return result
}

// LoadRedactionConfig reads a redaction config from a YAML file.
func LoadRedactionConfig(path string) (*RedactionConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read redaction config: %w", err)
	}

	var cfg RedactionConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse redaction config: %w", err)
	}

	return &cfg, nil
}

// DefaultRedactHeaders applies the default header-only redaction
// (authorization and x-api-key) without requiring a Redactor instance.
func DefaultRedactHeaders(headers map[string]string) map[string]string {
	result := make(map[string]string, len(headers))
	for k, v := range headers {
		lower := strings.ToLower(k)
		if lower == "authorization" || lower == "x-api-key" {
			result[k] = "[REDACTED]"
		} else {
			result[k] = v
		}
	}
	return result
}
