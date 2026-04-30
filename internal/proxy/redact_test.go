package proxy

import (
	"encoding/json"
	"testing"
)

func TestNewRedactor(t *testing.T) {
	cfg := RedactionConfig{
		BuiltinPII: true,
		Rules: []RedactionRule{
			{Name: "custom", Pattern: `CUST-\d{8}`, Replacement: "[REDACTED-CUST]"},
		},
	}
	r, err := NewRedactor(cfg)
	if err != nil {
		t.Fatalf("NewRedactor: %v", err)
	}
	// 7 builtin + 1 custom
	if len(r.rules) != 8 {
		t.Errorf("expected 8 rules, got %d", len(r.rules))
	}
}

func TestNewRedactorInvalidPattern(t *testing.T) {
	cfg := RedactionConfig{
		Rules: []RedactionRule{
			{Name: "bad", Pattern: `[invalid`},
		},
	}
	_, err := NewRedactor(cfg)
	if err == nil {
		t.Fatal("expected error for invalid pattern")
	}
}

func TestRedactString_Email(t *testing.T) {
	r := mustRedactor(t, true, nil)
	got := r.RedactString("contact user@example.com for details")
	want := "contact [REDACTED-EMAIL] for details"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRedactString_Phone(t *testing.T) {
	r := mustRedactor(t, true, nil)
	got := r.RedactString("call 555-123-4567 now")
	want := "call [REDACTED-PHONE] now"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRedactString_IP(t *testing.T) {
	r := mustRedactor(t, true, nil)
	got := r.RedactString("server at 192.168.1.100 is down")
	want := "server at [REDACTED-IP] is down"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRedactString_CreditCard(t *testing.T) {
	r := mustRedactor(t, true, nil)
	got := r.RedactString("card 4111-1111-1111-1111 charged")
	want := "card [REDACTED-CC] charged"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRedactString_SSN(t *testing.T) {
	r := mustRedactor(t, true, nil)
	got := r.RedactString("ssn is 123-45-6789")
	want := "ssn is [REDACTED-SSN]"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRedactString_APIKey(t *testing.T) {
	r := mustRedactor(t, true, nil)
	got := r.RedactString("key: sk-abcdefghijklmnopqrstuvwxyz")
	want := "key: [REDACTED-KEY]"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRedactString_JWT(t *testing.T) {
	r := mustRedactor(t, true, nil)
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc123_signature"
	got := r.RedactString("token: " + jwt)
	want := "token: [REDACTED-JWT]"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRedactString_NoMatch(t *testing.T) {
	r := mustRedactor(t, true, nil)
	input := "nothing sensitive here"
	got := r.RedactString(input)
	if got != input {
		t.Errorf("got %q, want %q", got, input)
	}
}

func TestRedactString_FieldPathRulesSkipped(t *testing.T) {
	rules := []RedactionRule{
		{Name: "targeted", Pattern: `secret`, Replacement: "[GONE]", FieldPaths: []string{"data.key"}},
	}
	r := mustRedactor(t, false, rules)
	// Rules with field paths should not apply in RedactString
	input := "this is secret"
	got := r.RedactString(input)
	if got != input {
		t.Errorf("field-path rule should be skipped in RedactString, got %q", got)
	}
}

func TestRedactJSON_NestedStrings(t *testing.T) {
	r := mustRedactor(t, true, nil)
	input := `{"user": {"email": "test@example.com", "name": "John"}}`
	got, err := r.RedactJSON([]byte(input))
	if err != nil {
		t.Fatalf("RedactJSON: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(got, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	user := result["user"].(map[string]any)
	if user["email"] != "[REDACTED-EMAIL]" {
		t.Errorf("email not redacted: %v", user["email"])
	}
	if user["name"] != "John" {
		t.Errorf("name should not be redacted: %v", user["name"])
	}
}

func TestRedactJSON_Arrays(t *testing.T) {
	r := mustRedactor(t, true, nil)
	input := `{"contacts": ["alice@test.com", "safe text", "bob@test.com"]}`
	got, err := r.RedactJSON([]byte(input))
	if err != nil {
		t.Fatalf("RedactJSON: %v", err)
	}

	var result map[string]any
	json.Unmarshal(got, &result)
	contacts := result["contacts"].([]any)
	if contacts[0] != "[REDACTED-EMAIL]" {
		t.Errorf("first email not redacted: %v", contacts[0])
	}
	if contacts[1] != "safe text" {
		t.Errorf("safe text changed: %v", contacts[1])
	}
	if contacts[2] != "[REDACTED-EMAIL]" {
		t.Errorf("second email not redacted: %v", contacts[2])
	}
}

func TestRedactJSON_FieldPathTargeting(t *testing.T) {
	rules := []RedactionRule{
		{Name: "content_scrub", Pattern: `\w+`, Replacement: "[SCRUBBED]", FieldPaths: []string{"messages[*].content"}},
	}
	r := mustRedactor(t, false, rules)

	input := `{"messages": [{"role": "user", "content": "hello world"}], "model": "gpt-4"}`
	got, err := r.RedactJSON([]byte(input))
	if err != nil {
		t.Fatalf("RedactJSON: %v", err)
	}

	var result map[string]any
	json.Unmarshal(got, &result)
	// model should be untouched
	if result["model"] != "gpt-4" {
		t.Errorf("model should not be redacted: %v", result["model"])
	}
	// content should be scrubbed
	msgs := result["messages"].([]any)
	msg := msgs[0].(map[string]any)
	if msg["content"] == "hello world" {
		t.Error("content should have been redacted")
	}
	// role should be untouched (doesn't match field path)
	if msg["role"] != "user" {
		t.Errorf("role should not be redacted: %v", msg["role"])
	}
}

func TestRedactJSON_InvalidJSON(t *testing.T) {
	r := mustRedactor(t, true, nil)
	input := "not json, has user@example.com"
	got, err := r.RedactJSON([]byte(input))
	if err != nil {
		t.Fatalf("RedactJSON: %v", err)
	}
	want := "not json, has [REDACTED-EMAIL]"
	if string(got) != want {
		t.Errorf("got %q, want %q", string(got), want)
	}
}

func TestRedactHeaders(t *testing.T) {
	r := mustRedactor(t, true, nil)
	headers := map[string]string{
		"Authorization": "Bearer sk-abcdefghijklmnopqrstuvwxyz",
		"X-Api-Key":     "secret-key",
		"Content-Type":  "application/json",
		"X-Custom":      "value with user@test.com",
	}
	got := r.RedactHeaders(headers)

	if got["Authorization"] != "[REDACTED]" {
		t.Errorf("Authorization not redacted: %v", got["Authorization"])
	}
	if got["X-Api-Key"] != "[REDACTED]" {
		t.Errorf("X-Api-Key not redacted: %v", got["X-Api-Key"])
	}
	if got["Content-Type"] != "application/json" {
		t.Errorf("Content-Type changed: %v", got["Content-Type"])
	}
	if got["X-Custom"] != "value with [REDACTED-EMAIL]" {
		t.Errorf("X-Custom email not redacted: %v", got["X-Custom"])
	}
}

func TestDefaultRedactHeaders(t *testing.T) {
	headers := map[string]string{
		"Authorization": "Bearer token",
		"X-Api-Key":     "secret",
		"Accept":        "application/json",
	}
	got := DefaultRedactHeaders(headers)
	if got["Authorization"] != "[REDACTED]" {
		t.Errorf("Authorization not redacted: %v", got["Authorization"])
	}
	if got["X-Api-Key"] != "[REDACTED]" {
		t.Errorf("X-Api-Key not redacted: %v", got["X-Api-Key"])
	}
	if got["Accept"] != "application/json" {
		t.Errorf("Accept changed: %v", got["Accept"])
	}
}

func TestMatchesFieldPath(t *testing.T) {
	tests := []struct {
		path     string
		patterns []string
		want     bool
	}{
		{"messages[*].content", []string{"messages[*].content"}, true},
		{"messages[*].role", []string{"messages[*].content"}, false},
		{"data.nested.value", []string{"data.*.value"}, true},
		{"data.nested.value", []string{"data.other.value"}, false},
		{"top", []string{"top"}, true},
	}
	for _, tt := range tests {
		got := matchesFieldPath(tt.path, tt.patterns)
		if got != tt.want {
			t.Errorf("matchesFieldPath(%q, %v) = %v, want %v", tt.path, tt.patterns, got, tt.want)
		}
	}
}

func mustRedactor(t *testing.T, builtin bool, custom []RedactionRule) *Redactor {
	t.Helper()
	r, err := NewRedactor(RedactionConfig{BuiltinPII: builtin, Rules: custom})
	if err != nil {
		t.Fatalf("NewRedactor: %v", err)
	}
	return r
}
