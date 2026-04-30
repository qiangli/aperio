package trace

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// streamHeader is the first line in an NDJSON trace stream.
type streamHeader struct {
	TraceID   string   `json:"trace_id"`
	CreatedAt time.Time `json:"created_at"`
	Metadata  Metadata  `json:"metadata"`
}

// StreamWriter writes spans incrementally as NDJSON (newline-delimited JSON).
// The first line is a header with trace ID and metadata.
// Each subsequent line is a serialized Span.
type StreamWriter struct {
	f     *os.File
	w     *bufio.Writer
	mu    sync.Mutex
	meta  Metadata
	id    string
	count int
	path  string
}

// NewStreamWriter creates a new NDJSON stream writer.
func NewStreamWriter(path string, traceID string, meta Metadata) (*StreamWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create stream file: %w", err)
	}

	sw := &StreamWriter{
		f:    f,
		w:    bufio.NewWriter(f),
		meta: meta,
		id:   traceID,
		path: path,
	}

	// Write header as first line
	header := streamHeader{
		TraceID:   traceID,
		CreatedAt: time.Now(),
		Metadata:  meta,
	}
	data, err := json.Marshal(header)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("marshal stream header: %w", err)
	}
	if _, err := sw.w.Write(data); err != nil {
		f.Close()
		return nil, fmt.Errorf("write stream header: %w", err)
	}
	if err := sw.w.WriteByte('\n'); err != nil {
		f.Close()
		return nil, fmt.Errorf("write newline: %w", err)
	}

	return sw, nil
}

// WriteSpan appends a span to the NDJSON stream.
func (sw *StreamWriter) WriteSpan(s *Span) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal span: %w", err)
	}

	sw.mu.Lock()
	defer sw.mu.Unlock()

	if _, err := sw.w.Write(data); err != nil {
		return fmt.Errorf("write span: %w", err)
	}
	if err := sw.w.WriteByte('\n'); err != nil {
		return fmt.Errorf("write newline: %w", err)
	}
	sw.count++

	return nil
}

// Close flushes and closes the stream, returning the complete Trace.
func (sw *StreamWriter) Close() (*Trace, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if err := sw.w.Flush(); err != nil {
		sw.f.Close()
		return nil, fmt.Errorf("flush stream: %w", err)
	}
	if err := sw.f.Close(); err != nil {
		return nil, fmt.Errorf("close stream: %w", err)
	}

	// Read back the stream to construct the full Trace
	return ReadStreamTrace(sw.path)
}

// Count returns the number of spans written so far.
func (sw *StreamWriter) Count() int {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.count
}

// ReadStreamTrace reads an NDJSON trace stream and returns a Trace.
func ReadStreamTrace(path string) (*Trace, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open stream file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024) // up to 10MB per line

	// First line is the header
	if !scanner.Scan() {
		return nil, fmt.Errorf("empty stream file")
	}
	var header streamHeader
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		return nil, fmt.Errorf("parse stream header: %w", err)
	}

	t := &Trace{
		ID:        header.TraceID,
		CreatedAt: header.CreatedAt,
		Metadata:  header.Metadata,
	}

	// Remaining lines are spans
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var s Span
		if err := json.Unmarshal(line, &s); err != nil {
			return nil, fmt.Errorf("parse span: %w", err)
		}
		t.Spans = append(t.Spans, &s)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan stream: %w", err)
	}

	return t, nil
}
