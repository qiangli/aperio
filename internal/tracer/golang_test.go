package tracer

import (
	"testing"
	"time"

	"github.com/qiangli/aperio/internal/trace"
)

func TestParseDlvOutput(t *testing.T) {
	output := `> goroutine(1): main.processQuery(query = "hello world")
=> "the answer is 42"
> goroutine(1): main.callLLM(prompt = "What is 6*7?")
=> ("response data", nil)
> goroutine(5): main.executeTool(name = "calculator", args = {"op": "multiply"})
=> "42"
`

	spans, err := parseDlvOutput([]byte(output))
	if err != nil {
		t.Fatalf("parseDlvOutput: %v", err)
	}

	if len(spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(spans))
	}

	// Check first span
	if spans[0].Name != "main.processQuery" {
		t.Errorf("span[0].Name: got %s, want main.processQuery", spans[0].Name)
	}
	if spans[0].Type != trace.SpanFunction {
		t.Errorf("span[0].Type: got %s, want FUNCTION", spans[0].Type)
	}
	if spans[0].Attributes["goroutine"] != "1" {
		t.Errorf("span[0].goroutine: got %v, want 1", spans[0].Attributes["goroutine"])
	}
	if spans[0].Attributes["return_value"] != `"the answer is 42"` {
		t.Errorf("span[0].return_value: got %v", spans[0].Attributes["return_value"])
	}

	// Check end time was set
	if spans[0].EndTime.Equal(time.Time{}) {
		t.Error("span[0].EndTime should be set")
	}

	// Check second span
	if spans[1].Name != "main.callLLM" {
		t.Errorf("span[1].Name: got %s", spans[1].Name)
	}

	// Check goroutine 5
	if spans[2].Attributes["goroutine"] != "5" {
		t.Errorf("span[2].goroutine: got %v", spans[2].Attributes["goroutine"])
	}
}

func TestBuildTracePattern(t *testing.T) {
	cfg := Config{}
	pattern := buildTracePattern(cfg)
	if pattern != `main\..+` {
		t.Errorf("default pattern: got %s", pattern)
	}

	cfg.IncludePatterns = []string{"mypackage\\..*", "main\\..*"}
	pattern = buildTracePattern(cfg)
	if pattern != `mypackage\..*|main\..*` {
		t.Errorf("custom pattern: got %s", pattern)
	}
}

func TestSplitGoRunArgs(t *testing.T) {
	pkg, progArgs := splitGoRunArgs([]string{"go", "run", "./cmd/agent", "hello", "world"})
	if pkg != "./cmd/agent" {
		t.Errorf("pkg: got %s", pkg)
	}
	if len(progArgs) != 2 || progArgs[0] != "hello" {
		t.Errorf("progArgs: got %v", progArgs)
	}
}

func TestSplitGoRunArgsNoProgArgs(t *testing.T) {
	pkg, progArgs := splitGoRunArgs([]string{"go", "run", "./cmd/agent"})
	if pkg != "./cmd/agent" {
		t.Errorf("pkg: got %s", pkg)
	}
	if len(progArgs) != 0 {
		t.Errorf("expected no prog args, got %v", progArgs)
	}
}

func TestParseDlvOutputEmpty(t *testing.T) {
	spans, err := parseDlvOutput([]byte(""))
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 0 {
		t.Errorf("expected 0 spans, got %d", len(spans))
	}
}

func TestParseDlvOutputCallWithoutReturn(t *testing.T) {
	output := `> goroutine(1): main.longRunning(ctx = context.Background)
`
	spans, err := parseDlvOutput([]byte(output))
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if !spans[0].EndTime.IsZero() {
		t.Error("span without return should have zero EndTime")
	}
}

func TestParseDlvOutputLongReturnValue(t *testing.T) {
	longVal := "\"" + string(make([]byte, 300)) + "\""
	output := "> goroutine(1): main.big()\n=> " + longVal + "\n"
	spans, err := parseDlvOutput([]byte(output))
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 1 {
		t.Fatal("expected 1 span")
	}
	retVal, _ := spans[0].Attributes["return_value"].(string)
	if len(retVal) > 204 { // 200 + "..."
		t.Errorf("return value should be truncated, length: %d", len(retVal))
	}
}

func TestIsGoRunCommand(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{[]string{"go", "run", "./cmd/agent"}, true},
		{[]string{"go", "build", "./cmd/agent"}, false},
		{[]string{"go"}, false},
		{[]string{"go", "run"}, false}, // len < 3 but still "go run"
		{[]string{"python3", "agent.py"}, false},
	}
	for _, tt := range tests {
		got := isGoRunCommand(tt.args)
		if got != tt.want {
			t.Errorf("isGoRunCommand(%v) = %v, want %v", tt.args, got, tt.want)
		}
	}
}

func TestGoTracerSetupNoDlv(t *testing.T) {
	// If dlv is not installed, Setup should return original args without error
	gt := NewGoTracer()
	cfg := Config{TraceOutputPath: "/tmp/test.json", WorkingDir: "/tmp"}
	_, args, err := gt.Setup(nil, cfg, []string{"./my-agent", "hello"})
	if err != nil {
		t.Fatalf("Setup should not error: %v", err)
	}
	// Args should be returned (either original or dlv-wrapped)
	if len(args) == 0 {
		t.Error("args should not be empty")
	}
}

func TestGoTracerCleanup(t *testing.T) {
	gt := NewGoTracer()
	// Cleanup on fresh tracer should not error
	if err := gt.Cleanup(); err != nil {
		t.Errorf("Cleanup: %v", err)
	}
}
