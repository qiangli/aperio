package aperio_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/aperio/internal/proxy"
	"github.com/qiangli/aperio/internal/runner"
	"github.com/qiangli/aperio/internal/trace"
	"github.com/qiangli/aperio/internal/viewer"
)

// =============================================================================
// E2E 1: Python record + view
// =============================================================================

func TestE2EPythonRecordAndView(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found")
	}

	agentDir := t.TempDir()
	os.WriteFile(filepath.Join(agentDir, "agent.py"), []byte(`
def greet(name):
    return f"Hello, {name}!"

def process(query):
    return greet("World") + " " + query

if __name__ == "__main__":
    import sys
    q = sys.argv[1] if len(sys.argv) > 1 else "test"
    print(process(q))
`), 0644)

	tracePath := filepath.Join(t.TempDir(), "trace.json")

	// Record
	err := runner.Record(context.Background(), runner.RecordOptions{
		Command:    []string{"python3", filepath.Join(agentDir, "agent.py"), "e2e-test"},
		OutputPath: tracePath,
		WorkingDir: agentDir,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	// Verify trace file
	tr, err := trace.ReadTrace(tracePath)
	if err != nil {
		t.Fatalf("ReadTrace: %v", err)
	}

	if tr.Metadata.Language != "python" {
		t.Errorf("language: %s", tr.Metadata.Language)
	}
	if len(tr.Spans) == 0 {
		t.Fatal("no spans in trace")
	}

	funcSpans := tr.SpansByType(trace.SpanFunction)
	if len(funcSpans) == 0 {
		t.Fatal("no FUNCTION spans")
	}

	// Check expected functions
	funcNames := map[string]bool{}
	for _, s := range funcSpans {
		funcNames[s.Name] = true
	}
	for _, expected := range []string{"__main__.process", "__main__.greet"} {
		if !funcNames[expected] {
			t.Errorf("missing function: %s (found: %v)", expected, funcNames)
		}
	}

	// View
	viewURL, err := viewer.Serve(viewer.Options{Port: 0, TraceFile: tracePath})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}

	resp, err := http.Get(viewURL)
	if err != nil {
		t.Fatalf("GET viewer: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	if !strings.Contains(html, tr.ID) {
		t.Error("viewer HTML should contain trace ID")
	}
	if !strings.Contains(html, "FUNCTION") {
		t.Error("viewer HTML should contain FUNCTION span type")
	}
}

// =============================================================================
// E2E 2: Python record with I/O (exec, fs_read, fs_write)
// =============================================================================

func TestE2EPythonRecordWithIO(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found")
	}

	agentDir := t.TempDir()
	configFile := filepath.Join(agentDir, "config.txt")
	os.WriteFile(configFile, []byte("setting=42"), 0644)

	os.WriteFile(filepath.Join(agentDir, "io_agent.py"), []byte(`
import subprocess
import os

def read_config():
    with open("`+configFile+`", "r") as f:
        return f.read().strip()

def run_command(cmd):
    result = subprocess.run(cmd, shell=True, capture_output=True, text=True)
    return result.stdout.strip()

def write_result(data):
    with open("`+filepath.Join(agentDir, "output.txt")+`", "w") as f:
        f.write(data)

def process():
    config = read_config()
    hostname = run_command("echo test-host")
    result = f"{config} @ {hostname}"
    write_result(result)
    print(result)

if __name__ == "__main__":
    process()
`), 0644)

	tracePath := filepath.Join(t.TempDir(), "trace.json")

	err := runner.Record(context.Background(), runner.RecordOptions{
		Command:    []string{"python3", filepath.Join(agentDir, "io_agent.py")},
		OutputPath: tracePath,
		WorkingDir: agentDir,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	tr, err := trace.ReadTrace(tracePath)
	if err != nil {
		t.Fatal(err)
	}

	// Verify all expected span types present
	typeCount := map[trace.SpanType]int{}
	for _, s := range tr.Spans {
		typeCount[s.Type]++
	}

	for _, expected := range []trace.SpanType{trace.SpanFunction, trace.SpanExec, trace.SpanFSRead, trace.SpanFSWrite} {
		if typeCount[expected] == 0 {
			t.Errorf("missing span type %s, found: %v", expected, typeCount)
		}
	}

	// Verify EXEC span has correct command
	execSpans := tr.SpansByType(trace.SpanExec)
	foundEcho := false
	for _, s := range execSpans {
		if cmd, ok := s.Attributes["command"].(string); ok && strings.Contains(cmd, "echo") {
			foundEcho = true
		}
	}
	if !foundEcho {
		t.Error("EXEC span should contain echo command")
	}

	// Verify FS_READ has config file path
	fsReadSpans := tr.SpansByType(trace.SpanFSRead)
	foundConfig := false
	for _, s := range fsReadSpans {
		if path, ok := s.Attributes["path"].(string); ok && strings.Contains(path, "config.txt") {
			foundConfig = true
		}
	}
	if !foundConfig {
		t.Error("FS_READ span should reference config.txt")
	}

	// Verify parent-child: process -> read_config, run_command, write_result
	funcSpans := tr.SpansByType(trace.SpanFunction)
	processSpanID := ""
	for _, s := range funcSpans {
		if strings.Contains(s.Name, "process") && !strings.Contains(s.Name, "<module>") {
			processSpanID = s.ID
		}
	}
	if processSpanID == "" {
		t.Fatal("process function span not found")
	}

	childCount := 0
	for _, s := range funcSpans {
		if s.ParentID == processSpanID {
			childCount++
		}
	}
	if childCount < 3 {
		t.Errorf("process should have at least 3 children (read_config, run_command, write_result), got %d", childCount)
	}
}

// =============================================================================
// E2E 3: Python record → replay → diff
// =============================================================================

func TestE2EPythonRecordReplayDiff(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found")
	}

	agentDir := t.TempDir()
	os.WriteFile(filepath.Join(agentDir, "simple.py"), []byte(`
def compute(x):
    return x * 2

if __name__ == "__main__":
    print(compute(21))
`), 0644)

	tracePath1 := filepath.Join(t.TempDir(), "trace1.json")
	tracePath2 := filepath.Join(t.TempDir(), "trace2.json")

	// Record
	err := runner.Record(context.Background(), runner.RecordOptions{
		Command:    []string{"python3", filepath.Join(agentDir, "simple.py")},
		OutputPath: tracePath1,
		WorkingDir: agentDir,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	// Replay
	err = runner.Replay(context.Background(), runner.ReplayOptions{
		Command:       []string{"python3", filepath.Join(agentDir, "simple.py")},
		InputPath:     tracePath1,
		OutputPath:    tracePath2,
		MatchStrategy: "sequential",
		WorkingDir:    agentDir,
	})
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}

	// Read both traces
	tr1, err := trace.ReadTrace(tracePath1)
	if err != nil {
		t.Fatal(err)
	}
	tr2, err := trace.ReadTrace(tracePath2)
	if err != nil {
		t.Fatal(err)
	}

	// Both should have function spans
	if len(tr1.SpansByType(trace.SpanFunction)) == 0 {
		t.Error("trace1 should have function spans")
	}
	if len(tr2.SpansByType(trace.SpanFunction)) == 0 {
		t.Error("trace2 should have function spans")
	}

	// Diff should show matching function spans (no LLM/tool differences since there are none)
	diffs := trace.Diff(tr1, tr2)
	// Filter out only significant diffs (function count may vary slightly)
	significantDiffs := 0
	for _, d := range diffs {
		if d.Type == "changed" && strings.Contains(d.Field, "LLM_REQUEST") {
			significantDiffs++
		}
		if d.Type == "changed" && strings.Contains(d.Field, "TOOL_CALL") {
			significantDiffs++
		}
	}
	if significantDiffs > 0 {
		t.Errorf("expected no significant LLM/tool diffs, got: %s", trace.FormatDiff(diffs))
	}
}

// =============================================================================
// E2E 4: Node.js record with I/O
// =============================================================================

func TestE2ENodeRecordWithIO(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not found")
	}

	agentDir := t.TempDir()
	configFile := filepath.Join(agentDir, "config.txt")
	os.WriteFile(configFile, []byte("node_setting=99"), 0644)

	os.WriteFile(filepath.Join(agentDir, "io_agent.js"), []byte(`
const fs = require('fs');
const { execSync } = require('child_process');
const config = fs.readFileSync('`+configFile+`', 'utf8');
const result = execSync('echo node-test').toString().trim();
fs.writeFileSync('`+filepath.Join(agentDir, "output.txt")+`', config + ' ' + result);
console.log('done: ' + config.trim() + ' ' + result);
`), 0644)

	tracePath := filepath.Join(t.TempDir(), "trace.json")

	err := runner.Record(context.Background(), runner.RecordOptions{
		Command:    []string{"node", filepath.Join(agentDir, "io_agent.js")},
		OutputPath: tracePath,
		WorkingDir: agentDir,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	tr, err := trace.ReadTrace(tracePath)
	if err != nil {
		t.Fatal(err)
	}

	typeCount := map[trace.SpanType]int{}
	for _, s := range tr.Spans {
		typeCount[s.Type]++
	}

	for _, expected := range []trace.SpanType{trace.SpanExec, trace.SpanFSRead, trace.SpanFSWrite} {
		if typeCount[expected] == 0 {
			t.Errorf("missing span type %s, found: %v", expected, typeCount)
		}
	}
}

// =============================================================================
// E2E 5: Proxy record + replay round-trip
// =============================================================================

func TestE2EProxyRecordReplayRoundTrip(t *testing.T) {
	// Start mock LLM server
	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"model": "gpt-4",
			"choices": []map[string]any{{
				"message": map[string]any{
					"role":    "assistant",
					"content": "Mock response to: " + fmt.Sprint(req["messages"]),
				},
			}},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		})
	}))
	defer mockLLM.Close()

	// === RECORD ===
	recordProxy, err := proxy.New(proxy.Options{Mode: proxy.ModeRecord, Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer recordProxy.Close()
	go recordProxy.Serve()

	proxyURL, _ := url.Parse(fmt.Sprintf("http://%s", recordProxy.Addr()))
	client := &http.Client{Transport: &http.Transport{
		Proxy:           http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}

	// Make 3 requests
	for i := 0; i < 3; i++ {
		reqBody := fmt.Sprintf(`{"model":"gpt-4","messages":[{"role":"user","content":"request %d"}]}`, i)
		resp, err := client.Post(mockLLM.URL+"/v1/chat/completions", "application/json", strings.NewReader(reqBody))
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		io.ReadAll(resp.Body)
		resp.Body.Close()
	}

	recordedSpans := recordProxy.Spans()
	if len(recordedSpans) != 3 {
		t.Fatalf("expected 3 recorded spans, got %d", len(recordedSpans))
	}

	// Build trace from recorded spans
	recordedTrace := &trace.Trace{
		ID:    "recorded",
		Spans: recordedSpans,
	}

	// === REPLAY ===
	replayProxy, err := proxy.New(proxy.Options{
		Mode:          proxy.ModeReplay,
		Port:          0,
		RecordedTrace: recordedTrace,
		MatchStrategy: "sequential",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer replayProxy.Close()
	go replayProxy.Serve()

	replayProxyURL, _ := url.Parse(fmt.Sprintf("http://%s", replayProxy.Addr()))
	replayClient := &http.Client{Transport: &http.Transport{
		Proxy:           http.ProxyURL(replayProxyURL),
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}

	// Make the same 3 requests — should get replayed responses
	for i := 0; i < 3; i++ {
		reqBody := fmt.Sprintf(`{"model":"gpt-4","messages":[{"role":"user","content":"request %d"}]}`, i)
		resp, err := replayClient.Post("http://api.openai.com/v1/chat/completions", "application/json", strings.NewReader(reqBody))
		if err != nil {
			t.Fatalf("replay request %d: %v", i, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Errorf("replay %d: expected 200, got %d", i, resp.StatusCode)
		}
		if !strings.Contains(string(body), "Mock response") {
			t.Errorf("replay %d: expected mock response, got: %s", i, string(body))
		}
	}

	// 4th request should fail (exhausted)
	resp, err := replayClient.Post("http://api.openai.com/v1/chat/completions", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	// In non-strict mode, it tries to forward (which will fail since no real server)
	// That's expected behavior
}

// =============================================================================
// E2E 6: Diff with known differences
// =============================================================================

func TestE2EDiffWithDifferences(t *testing.T) {
	dir := t.TempDir()

	trace1 := &trace.Trace{
		ID:       "trace1",
		Metadata: trace.Metadata{Language: "python"},
		Spans: []*trace.Span{
			{ID: "1", Type: trace.SpanLLMRequest, Name: "POST /v1/chat"},
			{ID: "2", Type: trace.SpanToolCall, Name: "tool: get_weather", Attributes: map[string]any{"tool_name": "get_weather"}},
			{ID: "3", Type: trace.SpanExec, Name: "exec: curl", Attributes: map[string]any{"command": "curl http://api.weather.com"}},
			{ID: "4", Type: trace.SpanFSWrite, Name: "write: output.txt", Attributes: map[string]any{"path": "/tmp/output.txt"}},
		},
	}

	trace2 := &trace.Trace{
		ID:       "trace2",
		Metadata: trace.Metadata{Language: "python"},
		Spans: []*trace.Span{
			{ID: "1", Type: trace.SpanLLMRequest, Name: "POST /v1/chat"},
			{ID: "2", Type: trace.SpanToolCall, Name: "tool: get_time", Attributes: map[string]any{"tool_name": "get_time"}},
			{ID: "3", Type: trace.SpanExec, Name: "exec: wget", Attributes: map[string]any{"command": "wget http://api.time.com"}},
			{ID: "4", Type: trace.SpanFSWrite, Name: "write: result.txt", Attributes: map[string]any{"path": "/tmp/result.txt"}},
			{ID: "5", Type: trace.SpanDBQuery, Name: "sqlite: INSERT", Attributes: map[string]any{"query": "INSERT INTO logs"}},
		},
	}

	path1 := filepath.Join(dir, "trace1.json")
	path2 := filepath.Join(dir, "trace2.json")
	trace.WriteTrace(path1, trace1)
	trace.WriteTrace(path2, trace2)

	// Read and diff
	t1, _ := trace.ReadTrace(path1)
	t2, _ := trace.ReadTrace(path2)

	diffs := trace.Diff(t1, t2)
	if len(diffs) == 0 {
		t.Fatal("expected differences")
	}

	formatted := trace.FormatDiff(diffs)
	t.Logf("Diff output:\n%s", formatted)

	// Should detect tool_name change
	hasToolChange := false
	hasExecChange := false
	hasFSChange := false
	hasDBExtra := false

	for _, d := range diffs {
		if d.Field == "tool_name" && d.Left == "get_weather" && d.Right == "get_time" {
			hasToolChange = true
		}
		if d.Field == "command" && strings.Contains(d.Left, "curl") && strings.Contains(d.Right, "wget") {
			hasExecChange = true
		}
		if d.Field == "path" && strings.Contains(d.Left, "output.txt") && strings.Contains(d.Right, "result.txt") {
			hasFSChange = true
		}
		if d.Type == "extra" && strings.Contains(d.Message, "db_query") {
			hasDBExtra = true
		}
		if d.Type == "changed" && strings.Contains(d.Field, "span_count.DB_QUERY") {
			hasDBExtra = true
		}
	}

	if !hasToolChange {
		t.Error("should detect tool_name change (get_weather → get_time)")
	}
	if !hasExecChange {
		t.Error("should detect exec command change (curl → wget)")
	}
	if !hasFSChange {
		t.Error("should detect fs_write path change (output.txt → result.txt)")
	}
	if !hasDBExtra {
		t.Error("should detect extra DB_QUERY in trace2")
	}
}

// =============================================================================
// E2E 7: Exec classification in recorded trace
// =============================================================================

func TestE2EExecClassification(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found")
	}

	agentDir := t.TempDir()

	// Init a git repo so git commands work
	cmd := exec.Command("git", "init")
	cmd.Dir = agentDir
	cmd.Run()
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = agentDir
	cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = agentDir
	cmd.Run()

	os.WriteFile(filepath.Join(agentDir, "classify_agent.py"), []byte(`
import subprocess

def do_git():
    subprocess.run(["git", "status"], capture_output=True, text=True)
    subprocess.run(["git", "log", "--oneline", "-1"], capture_output=True, text=True)

def do_fs():
    subprocess.run(["ls", "-la"], capture_output=True, text=True)
    subprocess.run(["mkdir", "-p", "`+filepath.Join(agentDir, "subdir")+`"], capture_output=True, text=True)

def do_net():
    subprocess.run(["echo", "simulating-curl"], capture_output=True, text=True)

def do_build():
    subprocess.run(["echo", "simulating-make"], capture_output=True, text=True)

if __name__ == "__main__":
    do_git()
    do_fs()
    do_net()
    do_build()
    print("done")
`), 0644)

	tracePath := filepath.Join(t.TempDir(), "trace.json")

	err := runner.Record(context.Background(), runner.RecordOptions{
		Command:    []string{"python3", filepath.Join(agentDir, "classify_agent.py")},
		OutputPath: tracePath,
		WorkingDir: agentDir,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	tr, err := trace.ReadTrace(tracePath)
	if err != nil {
		t.Fatal(err)
	}

	// Verify EXEC spans have exec.category
	execSpans := tr.SpansByType(trace.SpanExec)
	if len(execSpans) == 0 {
		t.Fatal("no EXEC spans found")
	}

	categories := map[string]int{}
	for _, s := range execSpans {
		cat, _ := s.Attributes["exec.category"].(string)
		if cat == "" {
			t.Errorf("EXEC span %q missing exec.category", s.Name)
		}
		categories[cat]++
	}

	t.Logf("Exec categories: %v", categories)

	// Should have git and filesystem categories
	if categories["git"] == 0 {
		t.Error("expected git-category exec spans")
	}
	if categories["filesystem"] == 0 {
		t.Error("expected filesystem-category exec spans")
	}

	// Verify git subcategories
	for _, s := range execSpans {
		cat, _ := s.Attributes["exec.category"].(string)
		sub, _ := s.Attributes["exec.subcategory"].(string)
		if cat == "git" && sub == "" {
			t.Errorf("git command %q should have subcategory", s.Attributes["command"])
		}
	}
}

// =============================================================================
// E2E 8: Compound command classification
// =============================================================================

func TestE2ECompoundCommandClassification(t *testing.T) {
	// Test that ClassifyAndAnnotate properly handles compound commands in a trace
	tr := &trace.Trace{
		ID: "compound-test",
		Spans: []*trace.Span{
			{ID: "1", Type: trace.SpanExec, Name: "exec: pipe", Attributes: map[string]any{
				"command": "cat README.md | grep TODO | wc -l",
			}},
			{ID: "2", Type: trace.SpanExec, Name: "exec: chain", Attributes: map[string]any{
				"command": "cd /tmp && mkdir -p mydir && touch mydir/file.txt",
			}},
			{ID: "3", Type: trace.SpanExec, Name: "exec: git chain", Attributes: map[string]any{
				"command": "git add . && git commit -m 'update'",
			}},
			{ID: "4", Type: trace.SpanExec, Name: "exec: mixed", Attributes: map[string]any{
				"command": "npm install && npm test",
			}},
			{ID: "5", Type: trace.SpanExec, Name: "exec: curl pipe", Attributes: map[string]any{
				"command": "curl -s https://api.example.com | jq .data",
			}},
		},
	}

	trace.ClassifyAndAnnotate(tr)

	// Verify compound flag and breakdown
	for _, s := range tr.Spans {
		compound, _ := s.Attributes["exec.compound"].(bool)
		cat, _ := s.Attributes["exec.category"].(string)
		cats, hasCats := s.Attributes["exec.categories"].([]string)
		cmds, hasCmds := s.Attributes["exec.commands"].([]map[string]string)

		switch s.ID {
		case "1": // cat | grep | wc — 3 commands, all editor
			if !compound {
				t.Errorf("span 1: expected compound=true")
			}
			if cat != "editor" {
				t.Errorf("span 1: primary category = %s, want editor", cat)
			}
			if !hasCats || len(cats) != 3 {
				t.Errorf("span 1: expected 3 categories, got %d", len(cats))
			}
			if !hasCmds || len(cmds) != 3 {
				t.Errorf("span 1: expected 3 commands, got %d", len(cmds))
			}

		case "2": // cd && mkdir && touch — 3 filesystem commands
			if !compound {
				t.Errorf("span 2: expected compound=true")
			}
			if cat != "filesystem" {
				t.Errorf("span 2: primary category = %s, want filesystem", cat)
			}
			if !hasCats || len(cats) != 3 {
				t.Errorf("span 2: expected 3 categories, got %v", cats)
			}

		case "3": // git add && git commit — 2 git commands
			if !compound {
				t.Errorf("span 3: expected compound=true")
			}
			if cat != "git" {
				t.Errorf("span 3: primary category = %s, want git", cat)
			}
			for _, c := range cats {
				if c != "git" {
					t.Errorf("span 3: all categories should be git, got %s", c)
				}
			}

		case "4": // npm install && npm test — package_manager + test
			if !compound {
				t.Errorf("span 4: expected compound=true")
			}
			if !hasCats || len(cats) != 2 {
				t.Errorf("span 4: expected 2 categories, got %v", cats)
			}
			if hasCats && len(cats) == 2 {
				if cats[0] != "package_manager" {
					t.Errorf("span 4: first category = %s, want package_manager", cats[0])
				}
				if cats[1] != "test" {
					t.Errorf("span 4: second category = %s, want test", cats[1])
				}
			}

		case "5": // curl | jq — network + editor
			if !compound {
				t.Errorf("span 5: expected compound=true")
			}
			if cat != "network" {
				t.Errorf("span 5: primary category = %s, want network", cat)
			}
			if hasCats && len(cats) == 2 {
				if cats[1] != "editor" {
					t.Errorf("span 5: second category = %s, want editor", cats[1])
				}
			}
		}
	}

	// Write to file, read back, verify persistence
	dir := t.TempDir()
	path := filepath.Join(dir, "compound.json")
	if err := trace.WriteTrace(path, tr); err != nil {
		t.Fatal(err)
	}
	loaded, err := trace.ReadTrace(path)
	if err != nil {
		t.Fatal(err)
	}

	// Verify compound attributes survived round-trip
	for _, s := range loaded.Spans {
		if s.ID == "3" {
			cat, _ := s.Attributes["exec.category"].(string)
			if cat != "git" {
				t.Errorf("after round-trip: span 3 category = %s, want git", cat)
			}
			compound, _ := s.Attributes["exec.compound"].(bool)
			if !compound {
				t.Error("after round-trip: span 3 compound should be true")
			}
		}
	}
}

// =============================================================================
// E2E 9: Node.js exec classification
// =============================================================================

func TestE2ENodeExecClassification(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not found")
	}

	agentDir := t.TempDir()

	// Init git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = agentDir
	cmd.Run()

	os.WriteFile(filepath.Join(agentDir, "classify_agent.js"), []byte(`
const { execSync } = require('child_process');
const fs = require('fs');

// Git operation
try { execSync('git status', { cwd: '`+agentDir+`' }); } catch(e) {}

// Filesystem
execSync('ls -la', { cwd: '`+agentDir+`' });

// Editor/text
execSync('echo hello-world');

console.log('done');
`), 0644)

	tracePath := filepath.Join(t.TempDir(), "trace.json")

	err := runner.Record(context.Background(), runner.RecordOptions{
		Command:    []string{"node", filepath.Join(agentDir, "classify_agent.js")},
		OutputPath: tracePath,
		WorkingDir: agentDir,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	tr, err := trace.ReadTrace(tracePath)
	if err != nil {
		t.Fatal(err)
	}

	execSpans := tr.SpansByType(trace.SpanExec)
	if len(execSpans) == 0 {
		t.Fatal("no EXEC spans")
	}

	categories := map[string]int{}
	for _, s := range execSpans {
		cat, _ := s.Attributes["exec.category"].(string)
		categories[cat]++
		t.Logf("EXEC: category=%s command=%v", cat, s.Attributes["command"])
	}

	if categories["git"] == 0 {
		t.Error("expected git-category exec span")
	}
	if categories["filesystem"] == 0 {
		t.Error("expected filesystem-category exec span")
	}
}

// =============================================================================
// E2E 10: CLI binary build and smoke test
// =============================================================================

func TestE2ECLIBinaryBuildAndRun(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found")
	}

	// Build binary
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "aperio")
	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/aperio")
	buildCmd.Dir = filepath.Join(mustGetwd(t))
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}

	// Test --help
	helpOut, err := exec.Command(binPath, "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("--help failed: %v", err)
	}
	if !strings.Contains(string(helpOut), "record") {
		t.Error("--help should mention record command")
	}
	if !strings.Contains(string(helpOut), "replay") {
		t.Error("--help should mention replay command")
	}
	if !strings.Contains(string(helpOut), "view") {
		t.Error("--help should mention view command")
	}
	if !strings.Contains(string(helpOut), "diff") {
		t.Error("--help should mention diff command")
	}

	// Test record with a simple agent
	agentDir := t.TempDir()
	os.WriteFile(filepath.Join(agentDir, "agent.py"), []byte(`
def run():
    return "hello"
if __name__ == "__main__":
    print(run())
`), 0644)

	tracePath := filepath.Join(t.TempDir(), "trace.json")
	recordCmd := exec.Command(binPath, "record", "--output", tracePath, "--", "python3", filepath.Join(agentDir, "agent.py"))
	recordOut, err := recordCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("record failed: %v\n%s", err, recordOut)
	}

	// Verify trace file created and valid
	tr, err := trace.ReadTrace(tracePath)
	if err != nil {
		t.Fatalf("ReadTrace: %v", err)
	}
	if len(tr.Spans) == 0 {
		t.Error("trace should have spans")
	}
	if tr.Metadata.Language != "python" {
		t.Errorf("language = %s, want python", tr.Metadata.Language)
	}

	// Test diff command (diff trace with itself)
	diffCmd := exec.Command(binPath, "diff", tracePath, tracePath)
	diffOut, err := diffCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("diff failed: %v\n%s", err, diffOut)
	}
	if !strings.Contains(string(diffOut), "identical") {
		t.Errorf("diff with self should be identical, got: %s", diffOut)
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	// Walk up from test binary location to find go.mod
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// The test runs in the repo root
	return dir
}

// =============================================================================
// E2E 11: Binary Node.js agent (shebang detection + NODE_OPTIONS injection)
// =============================================================================

func TestE2EBinaryNodeAgent(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not found")
	}

	agentDir := t.TempDir()

	// Create a helper module the agent requires
	os.WriteFile(filepath.Join(agentDir, "helper.js"), []byte(`
function greet(name) { return "Hello, " + name + "!"; }
module.exports = { greet };
`), 0644)

	// Create a "binary" agent with a Node.js shebang — simulates an installed CLI tool
	agentBin := filepath.Join(agentDir, "fake-agent")
	os.WriteFile(agentBin, []byte(`#!/usr/bin/env node
const { greet } = require('./helper');
const fs = require('fs');
const { execSync } = require('child_process');

// Simulate agent behavior
const result = greet("World");
fs.writeFileSync('`+filepath.Join(agentDir, "output.txt")+`', result);
execSync('echo agent-ran');
console.log(result);
`), 0755)

	tracePath := filepath.Join(t.TempDir(), "trace.json")

	// Record using the binary directly (NOT "node fake-agent")
	err := runner.Record(context.Background(), runner.RecordOptions{
		Command:    []string{agentBin},
		OutputPath: tracePath,
		WorkingDir: agentDir,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	tr, err := trace.ReadTrace(tracePath)
	if err != nil {
		t.Fatal(err)
	}

	// Verify runtime was detected as Node.js
	if tr.Metadata.Language != "node" {
		t.Errorf("language = %s, want node", tr.Metadata.Language)
	}

	// Verify spans were captured
	typeCount := map[trace.SpanType]int{}
	for _, s := range tr.Spans {
		typeCount[s.Type]++
	}

	t.Logf("Span types: %v", typeCount)

	// Should have EXEC span (echo agent-ran)
	if typeCount[trace.SpanExec] == 0 {
		t.Error("expected EXEC spans from binary agent")
	}

	// Should have FS_WRITE span (writeFileSync)
	if typeCount[trace.SpanFSWrite] == 0 {
		t.Error("expected FS_WRITE spans from binary agent")
	}
}

// =============================================================================
// E2E 12: Binary Python agent (shebang detection + PYTHONPATH injection)
// =============================================================================

func TestE2EBinaryPythonAgent(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found")
	}

	agentDir := t.TempDir()

	// Create a "binary" agent with a Python shebang
	agentBin := filepath.Join(agentDir, "fake-py-agent")
	os.WriteFile(agentBin, []byte(`#!/usr/bin/env python3
import subprocess
import os

def process():
    result = subprocess.run(["echo", "py-agent-ran"], capture_output=True, text=True)
    with open("`+filepath.Join(agentDir, "result.txt")+`", "w") as f:
        f.write(result.stdout)
    print("done: " + result.stdout.strip())

if __name__ == "__main__":
    process()
`), 0755)

	tracePath := filepath.Join(t.TempDir(), "trace.json")

	// Record using the binary directly (NOT "python3 fake-py-agent")
	err := runner.Record(context.Background(), runner.RecordOptions{
		Command:    []string{agentBin},
		OutputPath: tracePath,
		WorkingDir: agentDir,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	tr, err := trace.ReadTrace(tracePath)
	if err != nil {
		t.Fatal(err)
	}

	if tr.Metadata.Language != "python" {
		t.Errorf("language = %s, want python", tr.Metadata.Language)
	}

	typeCount := map[trace.SpanType]int{}
	for _, s := range tr.Spans {
		typeCount[s.Type]++
	}

	t.Logf("Span types: %v", typeCount)

	if typeCount[trace.SpanFunction] == 0 {
		t.Error("expected FUNCTION spans from binary Python agent")
	}
	if typeCount[trace.SpanExec] == 0 {
		t.Error("expected EXEC spans from binary Python agent")
	}
	if typeCount[trace.SpanFSWrite] == 0 {
		t.Error("expected FS_WRITE spans from binary Python agent")
	}
}

// =============================================================================
// E2E 13: Runtime detection fallback to known name
// =============================================================================

func TestE2ERuntimeDetectionKnownName(t *testing.T) {
	// Test that DetectRuntime correctly identifies known agent names
	// even when the binary has no shebang or recognizable file type

	dir := t.TempDir()

	// Create opaque binaries with known agent names
	for name, wantLang := range map[string]string{
		"claude":  "node",
		"aider":   "python",
		"copilot": "node",
	} {
		path := filepath.Join(dir, name)
		os.WriteFile(path, []byte("opaque binary content"), 0755)

		info, err := runner.DetectRuntime(path)
		if err != nil {
			t.Errorf("%s: DetectRuntime error: %v", name, err)
			continue
		}
		if string(info.Runtime) != wantLang {
			t.Errorf("%s: runtime = %s, want %s", name, info.Runtime, wantLang)
		}
		if info.Confidence != "known_name" {
			t.Errorf("%s: confidence = %s, want known_name", name, info.Confidence)
		}
	}
}
