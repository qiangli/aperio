package export

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"
)

// SendOptions configures the OTLP HTTP sender.
type SendOptions struct {
	Endpoint  string
	Timeout   time.Duration
	AuthToken string // Bearer token for Authorization header
	APIKey    string // API key for X-API-Key header
	Compress  bool   // Enable gzip compression of request body
	MaxRetries int   // Maximum retry attempts (default: 3)
	UserAgent  string // User-Agent header (default: "aperio")
}

// SendError provides structured error information from OTLP send failures.
type SendError struct {
	StatusCode int
	Body       string
	Attempts   int
	Err        error
}

func (e *SendError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("OTLP endpoint returned %d after %d attempt(s): %s", e.StatusCode, e.Attempts, e.Body)
	}
	return fmt.Sprintf("OTLP send failed after %d attempt(s): %v", e.Attempts, e.Err)
}

func (e *SendError) Unwrap() error {
	return e.Err
}

// Send posts an OTLP export request to the configured endpoint.
// Supports retry with exponential backoff, gzip compression, and auth headers.
func Send(req *OTLPExportRequest, opts SendOptions) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal OTLP request: %w", err)
	}

	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.MaxRetries <= 0 {
		opts.MaxRetries = 3
	}
	if opts.UserAgent == "" {
		opts.UserAgent = "aperio"
	}

	// Optionally compress the payload
	var body []byte
	var contentEncoding string
	if opts.Compress {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, err := gz.Write(data); err != nil {
			return fmt.Errorf("gzip compress: %w", err)
		}
		if err := gz.Close(); err != nil {
			return fmt.Errorf("gzip close: %w", err)
		}
		body = buf.Bytes()
		contentEncoding = "gzip"
	} else {
		body = data
	}

	client := &http.Client{Timeout: opts.Timeout}

	var lastErr error
	for attempt := 1; attempt <= opts.MaxRetries; attempt++ {
		httpReq, err := http.NewRequest("POST", opts.Endpoint, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create HTTP request: %w", err)
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("User-Agent", opts.UserAgent)
		if contentEncoding != "" {
			httpReq.Header.Set("Content-Encoding", contentEncoding)
		}
		if opts.AuthToken != "" {
			httpReq.Header.Set("Authorization", "Bearer "+opts.AuthToken)
		}
		if opts.APIKey != "" {
			httpReq.Header.Set("X-API-Key", opts.APIKey)
		}

		resp, err := client.Do(httpReq)
		if err != nil {
			lastErr = err
			if attempt < opts.MaxRetries {
				backoff(attempt)
				continue
			}
			return &SendError{Attempts: attempt, Err: err}
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}

		// Retry on transient errors
		if isRetryable(resp.StatusCode) && attempt < opts.MaxRetries {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
			// Respect Retry-After header if present
			if retryAfter := parseRetryAfter(resp.Header.Get("Retry-After")); retryAfter > 0 {
				time.Sleep(retryAfter)
			} else {
				backoff(attempt)
			}
			continue
		}

		return &SendError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
			Attempts:   attempt,
		}
	}

	return &SendError{Attempts: opts.MaxRetries, Err: lastErr}
}

func isRetryable(statusCode int) bool {
	return statusCode == 429 || statusCode == 502 || statusCode == 503 || statusCode == 504
}

func backoff(attempt int) {
	delay := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	time.Sleep(delay)
}

func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 0
	}
	// Try as seconds
	if seconds, err := strconv.Atoi(header); err == nil {
		return time.Duration(seconds) * time.Second
	}
	// Try as HTTP date
	if t, err := time.Parse(time.RFC1123, header); err == nil {
		delay := time.Until(t)
		if delay > 0 {
			return delay
		}
	}
	return 0
}

// FormatJSON returns the OTLP export request as formatted JSON.
func FormatJSON(req *OTLPExportRequest) (string, error) {
	data, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal OTLP request: %w", err)
	}
	return string(data), nil
}
