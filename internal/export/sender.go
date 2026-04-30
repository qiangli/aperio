package export

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SendOptions configures the OTLP HTTP sender.
type SendOptions struct {
	Endpoint string
	Timeout  time.Duration
}

// Send posts an OTLP export request to the configured endpoint.
func Send(req *OTLPExportRequest, opts SendOptions) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal OTLP request: %w", err)
	}

	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}

	client := &http.Client{Timeout: opts.Timeout}

	httpReq, err := http.NewRequest("POST", opts.Endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("send OTLP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OTLP endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// FormatJSON returns the OTLP export request as formatted JSON.
func FormatJSON(req *OTLPExportRequest) (string, error) {
	data, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal OTLP request: %w", err)
	}
	return string(data), nil
}
