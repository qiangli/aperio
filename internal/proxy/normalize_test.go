package proxy

import (
	"encoding/json"
	"testing"
)

func TestNormalizeResponseStripTimestamp(t *testing.T) {
	body := []byte(`{"id":"chatcmpl-abc123","created":1700000000,"model":"gpt-4","choices":[]}`)
	cfg := NormalizeConfig{StripTimestamps: true}

	result := NormalizeResponse(body, cfg)

	var data map[string]any
	if err := json.Unmarshal(result, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, ok := data["created"]; ok {
		t.Error("expected 'created' field to be stripped")
	}
	if _, ok := data["model"]; !ok {
		t.Error("expected 'model' field to be preserved")
	}
}

func TestNormalizeResponseStripIDs(t *testing.T) {
	body := []byte(`{"id":"chatcmpl-abc123","choices":[{"message":{"tool_calls":[{"id":"call_xyz789"}]}}]}`)
	cfg := NormalizeConfig{StripIDs: true}

	result := NormalizeResponse(body, cfg)

	var data map[string]any
	if err := json.Unmarshal(result, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data["id"] != "<normalized>" {
		t.Errorf("expected ID to be normalized, got %v", data["id"])
	}
}

func TestNormalizeResponseNonJSON(t *testing.T) {
	body := []byte("not json")
	cfg := NormalizeConfig{StripTimestamps: true}

	result := NormalizeResponse(body, cfg)
	if string(result) != "not json" {
		t.Error("non-JSON body should be returned unchanged")
	}
}

func TestNormalizeHeaders(t *testing.T) {
	headers := map[string]string{
		"Content-Type":           "application/json",
		"X-Request-Id":           "abc123",
		"X-Ratelimit-Remaining": "99",
	}

	cfg := DefaultNormalizeConfig()
	result := NormalizeHeaders(headers, cfg)

	if _, ok := result["X-Request-Id"]; ok {
		t.Error("expected X-Request-Id to be stripped")
	}
	if _, ok := result["Content-Type"]; !ok {
		t.Error("expected Content-Type to be preserved")
	}
}

func TestCanonicalizeRequestBody(t *testing.T) {
	body1 := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hello"}],"stream_options":{"stream_id":"abc"}}`)
	body2 := []byte(`{"messages":[{"role":"user","content":"hello"}],"model":"gpt-4"}`)

	c1 := CanonicalizeRequestBody(body1)
	c2 := CanonicalizeRequestBody(body2)

	// After canonicalization (stream_options stripped, keys sorted by json.Marshal),
	// both should produce the same output
	if string(c1) != string(c2) {
		t.Errorf("canonical bodies should match:\n  %s\n  %s", c1, c2)
	}
}

func TestCanonicalizeRequestBodyNonJSON(t *testing.T) {
	body := []byte("not json")
	result := CanonicalizeRequestBody(body)
	if string(result) != "not json" {
		t.Error("non-JSON should be returned unchanged")
	}
}
