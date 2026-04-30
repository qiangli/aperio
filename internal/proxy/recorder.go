package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/elazarl/goproxy"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/qiangli/aperio/internal/trace"
)

// requestContext holds data captured in OnRequest for use in OnResponse.
type requestContext struct {
	spanID    string
	startTime time.Time
	method    string
	url       string
	headers   map[string]string
	body      []byte
}

func setupRecorder(p *Proxy, server *goproxy.ProxyHttpServer, redactor *Redactor) {
	server.OnRequest().DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		host := req.URL.Host
		if !p.shouldIntercept(host) {
			return req, nil
		}

		// Read and buffer the request body
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

		// Inject multi-agent correlation headers
		if traceID := os.Getenv("APERIO_TRACE_ID"); traceID != "" {
			req.Header.Set("X-Aperio-Trace-ID", traceID)
			if parentSpanID := os.Getenv("APERIO_PARENT_SPAN_ID"); parentSpanID != "" {
				req.Header.Set("X-Aperio-Parent-Span-ID", parentSpanID)
			}
			// W3C traceparent for OTEL interop
			spanID := uuid.New().String()
			traceHex := strings.ReplaceAll(traceID, "-", "")
			if len(traceHex) > 32 {
				traceHex = traceHex[:32]
			}
			spanHex := strings.ReplaceAll(spanID, "-", "")
			if len(spanHex) > 16 {
				spanHex = spanHex[len(spanHex)-16:]
			}
			req.Header.Set("traceparent", fmt.Sprintf("00-%s-%s-01", traceHex, spanHex))
		}

		// Capture headers (with redaction)
		rawHeaders := make(map[string]string, len(req.Header))
		for k, v := range req.Header {
			rawHeaders[k] = strings.Join(v, ", ")
		}
		var headers map[string]string
		if redactor != nil {
			headers = redactor.RedactHeaders(rawHeaders)
		} else {
			headers = DefaultRedactHeaders(rawHeaders)
		}

		rctx := &requestContext{
			spanID:    uuid.New().String(),
			startTime: time.Now(),
			method:    req.Method,
			url:       req.URL.String(),
			headers:   headers,
			body:      body,
		}

		ctx.UserData = rctx
		return req, nil
	})

	server.OnResponse().DoFunc(func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		rctx, ok := ctx.UserData.(*requestContext)
		if !ok || rctx == nil {
			return resp
		}

		if resp == nil {
			return resp
		}

		// Read and buffer the response body
		var respBody []byte
		if resp.Body != nil {
			var err error
			respBody, err = io.ReadAll(resp.Body)
			if err != nil {
				log.Error().Err(err).Msg("failed to read response body")
				return resp
			}
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
		}

		// Capture response headers
		respHeaders := make(map[string]string)
		for k, v := range resp.Header {
			respHeaders[k] = strings.Join(v, ", ")
		}

		endTime := time.Now()

		// Apply body-level redaction if configured
		reqBodyBytes := rctx.body
		respBodyBytes := respBody
		if redactor != nil {
			if len(reqBodyBytes) > 0 {
				if redacted, err := redactor.RedactJSON(reqBodyBytes); err == nil {
					reqBodyBytes = redacted
				}
			}
			if len(respBodyBytes) > 0 {
				if redacted, err := redactor.RedactJSON(respBodyBytes); err == nil {
					respBodyBytes = redacted
				}
			}
		}

		// Parse request body as JSON if possible for cleaner storage
		var reqBodyParsed any
		reqBodyParsed = string(reqBodyBytes)
		if len(reqBodyBytes) > 0 && reqBodyBytes[0] == '{' {
			reqBodyParsed = jsonRawOrString(reqBodyBytes)
		}

		// Parse response body as JSON if possible
		var respBodyParsed any
		respBodyParsed = string(respBodyBytes)
		contentType := resp.Header.Get("Content-Type")

		if strings.Contains(contentType, "text/event-stream") {
			// SSE streaming response — store as raw text
			respBodyParsed = string(respBodyBytes)
		} else if len(respBodyBytes) > 0 && respBodyBytes[0] == '{' {
			respBodyParsed = jsonRawOrString(respBodyBytes)
		}

		// Create LLM_REQUEST span
		reqSpan := &trace.Span{
			ID:        rctx.spanID,
			Type:      trace.SpanLLMRequest,
			Name:      rctx.method + " " + rctx.url,
			StartTime: rctx.startTime,
			EndTime:   endTime,
			Attributes: map[string]any{
				"http.method":           rctx.method,
				"http.url":              rctx.url,
				"http.request_headers":  rctx.headers,
				"http.request_body":     reqBodyParsed,
				"http.status_code":      resp.StatusCode,
				"http.response_headers": respHeaders,
				"http.response_body":    respBodyParsed,
				"http.content_type":     contentType,
			},
		}

		p.addSpan(reqSpan)

		log.Debug().
			Str("method", rctx.method).
			Str("url", rctx.url).
			Int("status", resp.StatusCode).
			Msg("recorded request/response")

		return resp
	})
}

// jsonRawOrString attempts to preserve JSON structure, falling back to string.
func jsonRawOrString(data []byte) any {
	// Return as json.RawMessage-compatible string for clean serialization
	return string(data)
}
