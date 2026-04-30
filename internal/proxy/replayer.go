package proxy

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"github.com/elazarl/goproxy"
	"github.com/rs/zerolog/log"

	"github.com/qiangli/aperio/internal/trace"
)

func setupReplayer(p *Proxy, server *goproxy.ProxyHttpServer, opts Options) {
	matcher := newMatcher(opts)

	server.OnRequest().DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		host := req.URL.Host
		if !p.shouldIntercept(host) {
			return req, nil
		}

		// Read request body for matching
		var body []byte
		if req.Body != nil {
			var err error
			body, err = io.ReadAll(req.Body)
			if err != nil {
				log.Error().Err(err).Msg("failed to read request body")
				return req, nil
			}
			req.Body = io.NopCloser(bytes.NewReader(body))
		}

		recorded := matcher.match(req.Method, req.URL.String(), body)
		if recorded == nil {
			if opts.Strict {
				log.Warn().
					Str("method", req.Method).
					Str("url", req.URL.String()).
					Msg("no recorded response found (strict mode)")
				return req, goproxy.NewResponse(req, "text/plain", http.StatusBadGateway,
					"aperio: no recorded response found for this request")
			}
			// Non-strict: forward to real upstream
			return req, nil
		}

		// Build response from recorded data
		resp := &http.Response{
			StatusCode: recorded.statusCode,
			Status:     http.StatusText(recorded.statusCode),
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(recorded.body)),
			Request:    req,
		}

		for k, v := range recorded.headers {
			resp.Header.Set(k, v)
		}

		log.Debug().
			Str("method", req.Method).
			Str("url", req.URL.String()).
			Int("status", recorded.statusCode).
			Msg("replayed response")

		return req, resp
	})
}

// recordedResponse holds a pre-recorded HTTP response.
type recordedResponse struct {
	statusCode int
	headers    map[string]string
	body       []byte
}

// matcher finds recorded responses for incoming requests.
type matcher struct {
	responses []*matchEntry
	index     int
	strategy  string
}

type matchEntry struct {
	method   string
	url      string
	body     []byte
	response *recordedResponse
}

func newMatcher(opts Options) *matcher {
	m := &matcher{
		strategy: opts.MatchStrategy,
	}

	// Load from cassette if provided
	if opts.CassettePath != "" {
		entries, err := ReadCassette(opts.CassettePath)
		if err == nil {
			m.responses = entries
		}
		return m
	}

	if opts.RecordedTrace == nil {
		return m
	}

	// Build response entries from recorded trace spans
	for _, span := range opts.RecordedTrace.Spans {
		if span.Type != trace.SpanLLMRequest {
			continue
		}

		attrs := span.Attributes
		statusCode := 200
		if sc, ok := attrs["http.status_code"]; ok {
			if scf, ok := sc.(float64); ok {
				statusCode = int(scf)
			}
		}

		headers := make(map[string]string)
		if rh, ok := attrs["http.response_headers"]; ok {
			if rhMap, ok := rh.(map[string]any); ok {
				for k, v := range rhMap {
					if vs, ok := v.(string); ok {
						headers[k] = vs
					}
				}
			}
		}

		var body []byte
		if rb, ok := attrs["http.response_body"]; ok {
			if rbs, ok := rb.(string); ok {
				body = []byte(rbs)
			}
		}

		method, _ := attrs["http.method"].(string)
		url, _ := attrs["http.url"].(string)

		var reqBody []byte
		if rb, ok := attrs["http.request_body"]; ok {
			if rbs, ok := rb.(string); ok {
				reqBody = []byte(rbs)
			}
		}

		m.responses = append(m.responses, &matchEntry{
			method: method,
			url:    url,
			body:   reqBody,
			response: &recordedResponse{
				statusCode: statusCode,
				headers:    headers,
				body:       body,
			},
		})
	}

	return m
}

func (m *matcher) match(method, url string, body []byte) *recordedResponse {
	if len(m.responses) == 0 {
		return nil
	}

	switch m.strategy {
	case "fingerprint":
		return m.fingerprintMatch(method, url)
	case "body":
		return m.bodyMatch(method, url, body)
	default:
		return m.sequentialMatch()
	}
}

func (m *matcher) sequentialMatch() *recordedResponse {
	if m.index >= len(m.responses) {
		return nil
	}
	entry := m.responses[m.index]
	m.index++
	return entry.response
}

func (m *matcher) fingerprintMatch(method, url string) *recordedResponse {
	// Normalize URL for matching (strip query params that vary)
	for i, entry := range m.responses {
		if entry.method == method && urlMatch(entry.url, url) {
			// Remove matched entry to prevent re-use
			m.responses = append(m.responses[:i], m.responses[i+1:]...)
			return entry.response
		}
	}
	return nil
}

func (m *matcher) bodyMatch(method, url string, body []byte) *recordedResponse {
	canonBody := CanonicalizeRequestBody(body)

	for i, entry := range m.responses {
		if entry.method != method {
			continue
		}
		if !urlMatch(entry.url, url) {
			continue
		}
		// Compare canonicalized bodies
		canonEntry := CanonicalizeRequestBody(entry.body)
		if string(canonBody) == string(canonEntry) {
			m.responses = append(m.responses[:i], m.responses[i+1:]...)
			return entry.response
		}
	}

	// Fall back to URL-only matching if no body match found
	return m.fingerprintMatch(method, url)
}

// urlMatch compares URLs ignoring minor variations.
func urlMatch(recorded, incoming string) bool {
	// Strip scheme differences and normalize
	r := strings.TrimPrefix(strings.TrimPrefix(recorded, "https://"), "http://")
	i := strings.TrimPrefix(strings.TrimPrefix(incoming, "https://"), "http://")

	// Compare path portion
	rParts := strings.SplitN(r, "?", 2)
	iParts := strings.SplitN(i, "?", 2)

	return rParts[0] == iParts[0]
}
