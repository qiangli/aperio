package proxy

import (
	"fmt"
	"os"

	"github.com/qiangli/aperio/internal/trace"

	"gopkg.in/yaml.v3"
)

// Cassette represents a collection of recorded HTTP interactions in VCR format.
type Cassette struct {
	Version      int           `yaml:"version"`
	Interactions []Interaction `yaml:"interactions"`
}

// Interaction represents a single recorded HTTP request-response pair.
type Interaction struct {
	Request  CassetteRequest  `yaml:"request"`
	Response CassetteResponse `yaml:"response"`
}

// CassetteRequest is the request portion of an interaction.
type CassetteRequest struct {
	Method string            `yaml:"method"`
	URL    string            `yaml:"url"`
	Body   string            `yaml:"body,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
}

// CassetteResponse is the response portion of an interaction.
type CassetteResponse struct {
	StatusCode int               `yaml:"status_code"`
	Body       string            `yaml:"body,omitempty"`
	Headers    map[string]string `yaml:"headers,omitempty"`
}

// WriteCassette writes recorded interactions to a YAML cassette file.
func WriteCassette(path string, spans []*trace.Span) error {
	cassette := SpansToCassette(spans)

	data, err := yaml.Marshal(cassette)
	if err != nil {
		return fmt.Errorf("marshal cassette: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write cassette: %w", err)
	}
	return nil
}

// ReadCassette reads a YAML cassette file and returns match entries.
func ReadCassette(path string) ([]*matchEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read cassette: %w", err)
	}

	var cassette Cassette
	if err := yaml.Unmarshal(data, &cassette); err != nil {
		return nil, fmt.Errorf("unmarshal cassette: %w", err)
	}

	var entries []*matchEntry
	for _, interaction := range cassette.Interactions {
		entries = append(entries, &matchEntry{
			method: interaction.Request.Method,
			url:    interaction.Request.URL,
			body:   []byte(interaction.Request.Body),
			response: &recordedResponse{
				statusCode: interaction.Response.StatusCode,
				headers:    interaction.Response.Headers,
				body:       []byte(interaction.Response.Body),
			},
		})
	}

	return entries, nil
}

// SpansToCassette converts trace spans to a cassette format.
func SpansToCassette(spans []*trace.Span) *Cassette {
	cassette := &Cassette{Version: 1}

	for _, span := range spans {
		if span.Type != trace.SpanLLMRequest {
			continue
		}

		attrs := span.Attributes
		method, _ := attrs["http.method"].(string)
		url, _ := attrs["http.url"].(string)
		reqBody, _ := attrs["http.request_body"].(string)
		respBody, _ := attrs["http.response_body"].(string)

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

		cassette.Interactions = append(cassette.Interactions, Interaction{
			Request: CassetteRequest{
				Method: method,
				URL:    url,
				Body:   reqBody,
			},
			Response: CassetteResponse{
				StatusCode: statusCode,
				Body:       respBody,
				Headers:    headers,
			},
		})
	}

	return cassette
}
