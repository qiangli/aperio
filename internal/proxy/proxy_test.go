package proxy

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/qiangli/aperio/internal/trace"
)

func TestRecordProxy(t *testing.T) {
	// Create a mock LLM server
	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		t.Logf("Mock LLM received: %s %s body=%s", r.Method, r.URL.Path, string(body))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"model": "gpt-4",
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": "Hello! How can I help you?",
					},
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 8,
				"total_tokens":      18,
			},
		})
	}))
	defer mockLLM.Close()

	// Create recording proxy
	p, err := New(Options{
		Mode: ModeRecord,
		Port: 0, // random port
	})
	if err != nil {
		t.Fatalf("New proxy: %v", err)
	}
	defer p.Close()

	go p.Serve()

	// Make a request through the proxy to the mock LLM
	proxyURL, _ := url.Parse(fmt.Sprintf("http://%s", p.Addr()))
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	reqBody := `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}`
	resp, err := client.Post(mockLLM.URL+"/v1/chat/completions", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("request through proxy: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	t.Logf("Response: %s", string(respBody))

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Check recorded spans
	spans := p.Spans()
	if len(spans) == 0 {
		t.Fatal("no spans recorded")
	}

	found := false
	for _, s := range spans {
		if s.Type == trace.SpanLLMRequest {
			found = true
			if s.Attributes["http.method"] != "POST" {
				t.Errorf("expected method POST, got %v", s.Attributes["http.method"])
			}
			t.Logf("Recorded span: %s %s", s.Type, s.Name)
		}
	}

	if !found {
		t.Error("no LLM_REQUEST span recorded")
	}
}

func TestReplayProxy(t *testing.T) {
	// Create a trace with recorded responses
	recorded := &trace.Trace{
		ID: "test-trace",
		Spans: []*trace.Span{
			{
				ID:   "span-1",
				Type: trace.SpanLLMRequest,
				Name: "POST /v1/chat/completions",
				Attributes: map[string]any{
					"http.method":      "POST",
					"http.url":         "http://localhost/v1/chat/completions",
					"http.status_code": float64(200),
					"http.response_headers": map[string]any{
						"Content-Type": "application/json",
					},
					"http.response_body": `{"model":"gpt-4","choices":[{"message":{"role":"assistant","content":"Replayed!"}}]}`,
				},
			},
		},
	}

	// Create replay proxy
	p, err := New(Options{
		Mode:          ModeReplay,
		Port:          0,
		RecordedTrace: recorded,
		MatchStrategy: "sequential",
	})
	if err != nil {
		t.Fatalf("New proxy: %v", err)
	}
	defer p.Close()

	go p.Serve()

	// Make a request through the proxy
	proxyURL, _ := url.Parse(fmt.Sprintf("http://%s", p.Addr()))
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	// The target doesn't matter — the replay proxy will intercept
	reqBody := `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}`
	resp, err := client.Post("http://api.openai.com/v1/chat/completions", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("request through proxy: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	respStr := string(respBody)

	if !strings.Contains(respStr, "Replayed!") {
		t.Errorf("expected replayed response, got: %s", respStr)
	}
}

func TestRecordAuthHeaderRedaction(t *testing.T) {
	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer mockLLM.Close()

	p, err := New(Options{Mode: ModeRecord, Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	go p.Serve()

	proxyURL, _ := url.Parse(fmt.Sprintf("http://%s", p.Addr()))
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}

	req, _ := http.NewRequest("POST", mockLLM.URL+"/v1/chat", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer sk-secret-key-12345")
	req.Header.Set("X-Api-Key", "anthropic-key-secret")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	spans := p.Spans()
	if len(spans) == 0 {
		t.Fatal("no spans recorded")
	}

	for _, s := range spans {
		if headers, ok := s.Attributes["http.request_headers"].(map[string]string); ok {
			if auth := headers["Authorization"]; auth != "[REDACTED]" {
				t.Errorf("Authorization should be redacted, got: %s", auth)
			}
			if apiKey := headers["X-Api-Key"]; apiKey != "[REDACTED]" {
				t.Errorf("X-Api-Key should be redacted, got: %s", apiKey)
			}
		}
	}
}

func TestRecordSSEStreaming(t *testing.T) {
	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Transfer-Encoding", "chunked")
		flusher, _ := w.(http.Flusher)
		for _, chunk := range []string{
			"data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n",
			"data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n",
			"data: [DONE]\n\n",
		} {
			w.Write([]byte(chunk))
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer mockLLM.Close()

	p, err := New(Options{Mode: ModeRecord, Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	go p.Serve()

	proxyURL, _ := url.Parse(fmt.Sprintf("http://%s", p.Addr()))
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}

	resp, err := client.Post(mockLLM.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	// Verify client received the SSE stream
	if !strings.Contains(string(body), "Hello") {
		t.Errorf("client should receive streamed data, got: %s", string(body))
	}

	// Verify proxy recorded it
	spans := p.Spans()
	if len(spans) == 0 {
		t.Fatal("no spans recorded for SSE stream")
	}

	// Check content type was recorded
	for _, s := range spans {
		ct, _ := s.Attributes["http.content_type"].(string)
		if strings.Contains(ct, "event-stream") {
			return // success
		}
	}
	t.Error("SSE content type not recorded")
}

func TestRecordTargetFiltering(t *testing.T) {
	p, err := New(Options{Mode: ModeRecord, Port: 0, Targets: []string{"api.openai.com"}})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	if !p.shouldIntercept("api.openai.com") {
		t.Error("should intercept api.openai.com")
	}
	if p.shouldIntercept("api.anthropic.com") {
		t.Error("should NOT intercept api.anthropic.com")
	}
}

func TestShouldInterceptNoTargets(t *testing.T) {
	p := &Proxy{targets: nil}
	if !p.shouldIntercept("anything.com") {
		t.Error("empty targets should intercept everything")
	}
}

func TestUrlMatch(t *testing.T) {
	tests := []struct {
		recorded string
		incoming string
		want     bool
	}{
		{"https://api.openai.com/v1/chat/completions", "http://api.openai.com/v1/chat/completions", true},
		{"http://api.openai.com/v1/chat/completions", "https://api.openai.com/v1/chat/completions", true},
		{"https://api.openai.com/v1/chat/completions?key=1", "https://api.openai.com/v1/chat/completions?key=2", true},
		{"https://api.openai.com/v1/chat/completions", "https://api.openai.com/v1/messages", false},
		{"api.openai.com/v1/chat", "api.openai.com/v1/chat", true},
	}
	for _, tt := range tests {
		got := urlMatch(tt.recorded, tt.incoming)
		if got != tt.want {
			t.Errorf("urlMatch(%q, %q) = %v, want %v", tt.recorded, tt.incoming, got, tt.want)
		}
	}
}

func TestSequentialMatcherExhaustion(t *testing.T) {
	m := &matcher{
		strategy: "sequential",
		responses: []*matchEntry{
			{response: &recordedResponse{statusCode: 200, body: []byte("first")}},
		},
	}

	// First match succeeds
	r1 := m.match("POST", "/v1/chat", nil)
	if r1 == nil {
		t.Fatal("first match should succeed")
	}

	// Second match returns nil (exhausted)
	r2 := m.match("POST", "/v1/chat", nil)
	if r2 != nil {
		t.Error("should return nil when exhausted")
	}
}

func TestFingerprintMatcherRemovesUsed(t *testing.T) {
	m := &matcher{
		strategy: "fingerprint",
		responses: []*matchEntry{
			{method: "POST", url: "http://api.openai.com/v1/chat", response: &recordedResponse{statusCode: 200, body: []byte("resp1")}},
			{method: "POST", url: "http://api.openai.com/v1/chat", response: &recordedResponse{statusCode: 200, body: []byte("resp2")}},
		},
	}

	r1 := m.match("POST", "http://api.openai.com/v1/chat", nil)
	if r1 == nil {
		t.Fatal("first match should succeed")
	}
	if string(r1.body) != "resp1" {
		t.Errorf("expected resp1, got %s", string(r1.body))
	}

	if len(m.responses) != 1 {
		t.Errorf("should have removed matched entry, got %d", len(m.responses))
	}

	r2 := m.match("POST", "http://api.openai.com/v1/chat", nil)
	if r2 == nil {
		t.Fatal("second match should succeed")
	}
	if string(r2.body) != "resp2" {
		t.Errorf("expected resp2, got %s", string(r2.body))
	}

	r3 := m.match("POST", "http://api.openai.com/v1/chat", nil)
	if r3 != nil {
		t.Error("third match should fail (exhausted)")
	}
}

func TestReplayStrictMode(t *testing.T) {
	// Empty trace — no recorded responses
	p, err := New(Options{
		Mode:          ModeReplay,
		Port:          0,
		RecordedTrace: &trace.Trace{},
		Strict:        true,
		MatchStrategy: "sequential",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	go p.Serve()

	proxyURL, _ := url.Parse(fmt.Sprintf("http://%s", p.Addr()))
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}

	resp, err := client.Post("http://api.openai.com/v1/chat/completions", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("strict mode should return 502, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "no recorded response") {
		t.Errorf("expected error message, got: %s", string(body))
	}
}

func TestMatcherEmptyTrace(t *testing.T) {
	m := newMatcher(Options{RecordedTrace: &trace.Trace{}, MatchStrategy: "sequential"})
	r := m.match("POST", "/v1/chat", nil)
	if r != nil {
		t.Error("empty trace should return nil")
	}
}

func TestMatcherNilTrace(t *testing.T) {
	m := newMatcher(Options{RecordedTrace: nil, MatchStrategy: "sequential"})
	r := m.match("POST", "/v1/chat", nil)
	if r != nil {
		t.Error("nil trace should return nil")
	}
}
