package trace

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func generateTrace(numSpans int) *Trace {
	t := &Trace{
		ID:        "bench-trace-id",
		CreatedAt: time.Now(),
		Metadata: Metadata{
			Command:  "python agent.py",
			Language: "python",
		},
	}

	for i := range numSpans {
		s := &Span{
			ID:        fmt.Sprintf("span-%d", i),
			Type:      SpanFunction,
			Name:      fmt.Sprintf("function_%d", i),
			StartTime: time.Now(),
			EndTime:   time.Now().Add(time.Duration(i) * time.Millisecond),
			Attributes: map[string]any{
				"filename": fmt.Sprintf("file_%d.py", i%10),
				"lineno":   i * 10,
				"module":   fmt.Sprintf("module_%d", i%5),
			},
		}
		if i > 0 {
			s.ParentID = fmt.Sprintf("span-%d", (i-1)/2)
		}
		t.Spans = append(t.Spans, s)
	}
	return t
}

func BenchmarkWriteTrace(b *testing.B) {
	for _, size := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("spans=%d", size), func(b *testing.B) {
			t := generateTrace(size)
			dir := b.TempDir()

			b.ResetTimer()
			for i := range b.N {
				path := filepath.Join(dir, fmt.Sprintf("trace-%d.json", i))
				if err := WriteTrace(path, t); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkWriteTraceCompressed(b *testing.B) {
	for _, size := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("spans=%d", size), func(b *testing.B) {
			t := generateTrace(size)
			dir := b.TempDir()

			b.ResetTimer()
			for i := range b.N {
				path := filepath.Join(dir, fmt.Sprintf("trace-%d.json.gz", i))
				if err := WriteTrace(path, t); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkReadTrace(b *testing.B) {
	for _, size := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("spans=%d", size), func(b *testing.B) {
			t := generateTrace(size)
			dir := b.TempDir()
			path := filepath.Join(dir, "trace.json")
			if err := WriteTrace(path, t); err != nil {
				b.Fatal(err)
			}

			b.ResetTimer()
			for range b.N {
				if _, err := ReadTrace(path); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkReadTraceCompressed(b *testing.B) {
	for _, size := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("spans=%d", size), func(b *testing.B) {
			t := generateTrace(size)
			dir := b.TempDir()
			path := filepath.Join(dir, "trace.json.gz")
			if err := WriteTrace(path, t); err != nil {
				b.Fatal(err)
			}

			b.ResetTimer()
			for range b.N {
				if _, err := ReadTrace(path); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkStreamWriteSpan(b *testing.B) {
	for _, size := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("spans=%d", size), func(b *testing.B) {
			t := generateTrace(size)
			dir := b.TempDir()

			b.ResetTimer()
			for i := range b.N {
				path := filepath.Join(dir, fmt.Sprintf("stream-%d.ndjson", i))
				sw, err := NewStreamWriter(path, t.ID, t.Metadata)
				if err != nil {
					b.Fatal(err)
				}
				for _, s := range t.Spans {
					if err := sw.WriteSpan(s); err != nil {
						b.Fatal(err)
					}
				}
				if _, err := sw.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestWriteReadCompressed(t *testing.T) {
	tr := generateTrace(50)
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json.gz")

	if err := WriteTrace(path, tr); err != nil {
		t.Fatalf("WriteTrace compressed: %v", err)
	}

	// Verify the file is actually smaller than uncompressed
	info, _ := os.Stat(path)
	uncompressedPath := filepath.Join(dir, "test.json")
	WriteTrace(uncompressedPath, tr)
	uncompressedInfo, _ := os.Stat(uncompressedPath)

	if info.Size() >= uncompressedInfo.Size() {
		t.Errorf("compressed (%d) should be smaller than uncompressed (%d)", info.Size(), uncompressedInfo.Size())
	}

	// Read back and verify
	got, err := ReadTrace(path)
	if err != nil {
		t.Fatalf("ReadTrace compressed: %v", err)
	}

	if len(got.Spans) != len(tr.Spans) {
		t.Errorf("span count: got %d, want %d", len(got.Spans), len(tr.Spans))
	}
	if got.ID != tr.ID {
		t.Errorf("trace ID: got %q, want %q", got.ID, tr.ID)
	}
}

func TestStreamWriteRead(t *testing.T) {
	tr := generateTrace(50)
	dir := t.TempDir()
	path := filepath.Join(dir, "test.ndjson")

	sw, err := NewStreamWriter(path, tr.ID, tr.Metadata)
	if err != nil {
		t.Fatalf("NewStreamWriter: %v", err)
	}

	for _, s := range tr.Spans {
		if err := sw.WriteSpan(s); err != nil {
			t.Fatalf("WriteSpan: %v", err)
		}
	}

	got, err := sw.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}

	if len(got.Spans) != len(tr.Spans) {
		t.Errorf("span count: got %d, want %d", len(got.Spans), len(tr.Spans))
	}
	if got.ID != tr.ID {
		t.Errorf("trace ID: got %q, want %q", got.ID, tr.ID)
	}
}

func TestEncryptDecrypt(t *testing.T) {
	tr := generateTrace(20)
	dir := t.TempDir()
	path := filepath.Join(dir, "encrypted.json")
	key := []byte("01234567890123456789012345678901") // 32 bytes

	if err := WriteTraceEncrypted(path, tr, key); err != nil {
		t.Fatalf("WriteTraceEncrypted: %v", err)
	}

	// ReadTrace should detect encrypted file
	_, err := ReadTrace(path)
	if err == nil {
		t.Fatal("ReadTrace should fail on encrypted file")
	}

	// ReadTraceEncrypted should succeed
	got, err := ReadTraceEncrypted(path, key)
	if err != nil {
		t.Fatalf("ReadTraceEncrypted: %v", err)
	}

	if len(got.Spans) != len(tr.Spans) {
		t.Errorf("span count: got %d, want %d", len(got.Spans), len(tr.Spans))
	}

	// Wrong key should fail
	badKey := []byte("99999999999999999999999999999999")
	_, err = ReadTraceEncrypted(path, badKey)
	if err == nil {
		t.Fatal("ReadTraceEncrypted should fail with wrong key")
	}
}
