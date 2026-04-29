package trace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.json")

	now := time.Now().Truncate(time.Millisecond)

	original := &Trace{
		ID:        "trace-123",
		CreatedAt: now,
		Metadata: Metadata{
			Command:    "python agent.py hello",
			Language:   "python",
			WorkingDir: "/tmp/agent",
		},
		Spans: []*Span{
			{
				ID:        "span-1",
				Type:      SpanUserInput,
				Name:      "user query",
				StartTime: now,
				EndTime:   now.Add(time.Millisecond),
				Attributes: map[string]any{
					"query": "hello",
				},
			},
			{
				ID:        "span-2",
				ParentID:  "span-1",
				Type:      SpanLLMRequest,
				Name:      "POST /v1/chat/completions",
				StartTime: now.Add(time.Millisecond),
				EndTime:   now.Add(2 * time.Millisecond),
				Attributes: map[string]any{
					"url":    "https://api.openai.com/v1/chat/completions",
					"method": "POST",
				},
			},
		},
	}

	if err := WriteTrace(path, original); err != nil {
		t.Fatalf("WriteTrace: %v", err)
	}

	loaded, err := ReadTrace(path)
	if err != nil {
		t.Fatalf("ReadTrace: %v", err)
	}

	if loaded.ID != original.ID {
		t.Errorf("ID: got %s, want %s", loaded.ID, original.ID)
	}
	if loaded.Metadata.Command != original.Metadata.Command {
		t.Errorf("Command: got %s, want %s", loaded.Metadata.Command, original.Metadata.Command)
	}
	if loaded.Metadata.Language != original.Metadata.Language {
		t.Errorf("Language: got %s, want %s", loaded.Metadata.Language, original.Metadata.Language)
	}
	if len(loaded.Spans) != len(original.Spans) {
		t.Fatalf("Spans: got %d, want %d", len(loaded.Spans), len(original.Spans))
	}

	for i, span := range loaded.Spans {
		orig := original.Spans[i]
		if span.ID != orig.ID {
			t.Errorf("Span[%d].ID: got %s, want %s", i, span.ID, orig.ID)
		}
		if span.Type != orig.Type {
			t.Errorf("Span[%d].Type: got %s, want %s", i, span.Type, orig.Type)
		}
		if span.ParentID != orig.ParentID {
			t.Errorf("Span[%d].ParentID: got %s, want %s", i, span.ParentID, orig.ParentID)
		}
	}
}

func TestReadTraceNotFound(t *testing.T) {
	_, err := ReadTrace("/nonexistent/path/trace.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestReadTraceInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")

	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ReadTrace(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestWriteTraceEmptySpans(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")

	tr := &Trace{ID: "empty", Metadata: Metadata{Language: "python"}}
	if err := WriteTrace(path, tr); err != nil {
		t.Fatalf("WriteTrace: %v", err)
	}
	loaded, err := ReadTrace(path)
	if err != nil {
		t.Fatalf("ReadTrace: %v", err)
	}
	if len(loaded.Spans) != 0 {
		t.Errorf("expected 0 spans, got %d", len(loaded.Spans))
	}
}

func TestWriteTraceNonExistentDir(t *testing.T) {
	err := WriteTrace("/nonexistent/dir/trace.json", &Trace{ID: "test"})
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

func TestReadTraceEmptyObject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	os.WriteFile(path, []byte("{}"), 0644)

	tr, err := ReadTrace(path)
	if err != nil {
		t.Fatalf("ReadTrace: %v", err)
	}
	if tr.ID != "" {
		t.Errorf("expected empty ID, got %s", tr.ID)
	}
}

func TestReadTraceZeroLengthFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "zero.json")
	os.WriteFile(path, []byte(""), 0644)

	_, err := ReadTrace(path)
	if err == nil {
		t.Fatal("expected error for zero-length file")
	}
}

func TestWriteReadAllSpanTypes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "all-types.json")
	now := time.Now().Truncate(time.Millisecond)

	original := &Trace{
		ID:        "all-types",
		CreatedAt: now,
		Metadata:  Metadata{Command: "test", Language: "python", WorkingDir: "/tmp"},
		Spans: []*Span{
			{ID: "1", Type: SpanFunction, Name: "main.run", StartTime: now, EndTime: now},
			{ID: "2", Type: SpanLLMRequest, Name: "POST /v1/chat", StartTime: now, EndTime: now},
			{ID: "3", Type: SpanToolCall, Name: "tool: get_weather", StartTime: now, EndTime: now},
			{ID: "4", Type: SpanExec, Name: "exec: ls", StartTime: now, EndTime: now, Attributes: map[string]any{"command": "ls", "exit_code": float64(0)}},
			{ID: "5", Type: SpanFSRead, Name: "read: config.txt", StartTime: now, EndTime: now, Attributes: map[string]any{"path": "/tmp/config.txt", "size": float64(100)}},
			{ID: "6", Type: SpanFSWrite, Name: "write: out.txt", StartTime: now, EndTime: now},
			{ID: "7", Type: SpanNetIO, Name: "connect: api.com:443", StartTime: now, EndTime: now},
			{ID: "8", Type: SpanDBQuery, Name: "sqlite: SELECT *", StartTime: now, EndTime: now, Attributes: map[string]any{"query": "SELECT * FROM users"}},
		},
	}

	if err := WriteTrace(path, original); err != nil {
		t.Fatalf("WriteTrace: %v", err)
	}
	loaded, err := ReadTrace(path)
	if err != nil {
		t.Fatalf("ReadTrace: %v", err)
	}
	if len(loaded.Spans) != len(original.Spans) {
		t.Fatalf("span count: got %d, want %d", len(loaded.Spans), len(original.Spans))
	}
	for i, s := range loaded.Spans {
		if s.Type != original.Spans[i].Type {
			t.Errorf("span[%d].Type: got %s, want %s", i, s.Type, original.Spans[i].Type)
		}
	}
}

func TestWriteReadUnicode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unicode.json")
	now := time.Now().Truncate(time.Millisecond)

	original := &Trace{
		ID:        "unicode-test",
		CreatedAt: now,
		Spans: []*Span{
			{ID: "1", Type: SpanFunction, Name: "处理查询", StartTime: now, EndTime: now,
				Attributes: map[string]any{"content": "日本語テスト 🎉"}},
		},
	}

	if err := WriteTrace(path, original); err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadTrace(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Spans[0].Name != "处理查询" {
		t.Errorf("unicode name not preserved: %s", loaded.Spans[0].Name)
	}
	if loaded.Spans[0].Attributes["content"] != "日本語テスト 🎉" {
		t.Errorf("unicode attribute not preserved: %v", loaded.Spans[0].Attributes["content"])
	}
}
