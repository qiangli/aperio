package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/qiangli/aperio/internal/trace"
)

func TestRecordEmptyCommand(t *testing.T) {
	err := Record(context.Background(), RecordOptions{})
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestRecordPythonAgent(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found")
	}

	agentDir := t.TempDir()
	agentScript := filepath.Join(agentDir, "agent.py")
	os.WriteFile(agentScript, []byte(`
def process(query):
    return f"Answer: {query}"

if __name__ == "__main__":
    import sys
    q = sys.argv[1] if len(sys.argv) > 1 else "test"
    print(process(q))
`), 0644)

	outputPath := filepath.Join(t.TempDir(), "trace.json")

	err := Record(context.Background(), RecordOptions{
		Command:    []string{"python3", agentScript, "hello"},
		OutputPath: outputPath,
		WorkingDir: agentDir,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	// Verify trace file created
	tr, err := trace.ReadTrace(outputPath)
	if err != nil {
		t.Fatalf("ReadTrace: %v", err)
	}

	if tr.Metadata.Language != "python" {
		t.Errorf("language: got %s, want python", tr.Metadata.Language)
	}

	// Should have function spans
	funcSpans := tr.SpansByType(trace.SpanFunction)
	if len(funcSpans) == 0 {
		t.Error("expected function spans from Python tracer")
	}

	// Should contain process function
	found := false
	for _, s := range funcSpans {
		if s.Name == "__main__.process" {
			found = true
		}
	}
	if !found {
		names := []string{}
		for _, s := range funcSpans {
			names = append(names, s.Name)
		}
		t.Errorf("expected __main__.process, got: %v", names)
	}
}

func TestReplayMissingInputFile(t *testing.T) {
	err := Replay(context.Background(), ReplayOptions{
		Command:   []string{"python3", "agent.py"},
		InputPath: "/nonexistent/trace.json",
	})
	if err == nil {
		t.Fatal("expected error for missing input file")
	}
}

func TestRecordDefaultOutputPath(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found")
	}

	agentDir := t.TempDir()
	os.WriteFile(filepath.Join(agentDir, "a.py"), []byte("print('hi')"), 0644)

	// Record with no explicit output path — should auto-generate
	err := Record(context.Background(), RecordOptions{
		Command:    []string{"python3", filepath.Join(agentDir, "a.py")},
		WorkingDir: agentDir,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	// Should have created a trace-*.json file in current dir
	matches, _ := filepath.Glob("trace-*.json")
	if len(matches) > 0 {
		// Clean up
		for _, m := range matches {
			os.Remove(m)
		}
	}
}
