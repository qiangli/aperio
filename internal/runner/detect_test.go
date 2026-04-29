package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want Language
	}{
		{"python", []string{"python", "agent.py", "hello"}, LangPython},
		{"python3", []string{"python3", "agent.py"}, LangPython},
		{"python3.11", []string{"python3.11", "agent.py"}, LangPython},
		{"go run", []string{"go", "run", "./cmd/agent"}, LangGo},
		{"go bare", []string{"go", "version"}, LangUnknown},
		{"node", []string{"node", "agent.js"}, LangNode},
		{"npx", []string{"npx", "tsx", "agent.ts"}, LangNode},
		{"tsx", []string{"tsx", "agent.ts"}, LangNode},
		{"bun", []string{"bun", "run", "agent.ts"}, LangNode},
		{"deno", []string{"deno", "run", "agent.ts"}, LangNode},
		{"file ext py", []string{"/usr/bin/custom-runner", "agent.py"}, LangPython},
		{"file ext js", []string{"/usr/bin/custom-runner", "agent.js"}, LangNode},
		{"file ext ts", []string{"/usr/bin/custom-runner", "agent.ts"}, LangNode},
		{"file ext mjs", []string{"/usr/bin/custom-runner", "agent.mjs"}, LangNode},
		{"file ext go", []string{"/usr/bin/custom-runner", "agent.go"}, LangGo},
		{"empty", []string{}, LangUnknown},
		{"unknown", []string{"/usr/bin/mystery", "something"}, LangUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectLanguage(tt.args)
			if got != tt.want {
				t.Errorf("DetectLanguage(%v) = %s, want %s", tt.args, got, tt.want)
			}
		})
	}
}

func TestDetectAgentDirPythonScript(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "agent.py")
	os.WriteFile(script, []byte("pass"), 0644)

	result := detectAgentDir([]string{"python3", script}, "/fallback")
	if result != dir {
		t.Errorf("expected %s, got %s", dir, result)
	}
}

func TestDetectAgentDirJSScript(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "agent.js")
	os.WriteFile(script, []byte(""), 0644)

	result := detectAgentDir([]string{"node", script}, "/fallback")
	if result != dir {
		t.Errorf("expected %s, got %s", dir, result)
	}
}

func TestDetectAgentDirTSScript(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "agent.ts")
	os.WriteFile(script, []byte(""), 0644)

	result := detectAgentDir([]string{"tsx", script}, "/fallback")
	if result != dir {
		t.Errorf("expected %s, got %s", dir, result)
	}
}

func TestDetectAgentDirDirectory(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "cmd", "agent")
	os.MkdirAll(subdir, 0755)

	result := detectAgentDir([]string{"go", "run", subdir}, "/fallback")
	if result != subdir {
		t.Errorf("expected %s, got %s", subdir, result)
	}
}

func TestDetectAgentDirNonExistent(t *testing.T) {
	result := detectAgentDir([]string{"python3", "/nonexistent/agent.py"}, "/fallback")
	if result != "/fallback" {
		t.Errorf("expected /fallback for nonexistent file, got %s", result)
	}
}

func TestDetectAgentDirSkipsFlags(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "agent.py")
	os.WriteFile(script, []byte("pass"), 0644)

	result := detectAgentDir([]string{"python3", "-u", script}, "/fallback")
	if result != dir {
		t.Errorf("expected %s (flag skipped), got %s", dir, result)
	}
}

func TestDetectAgentDirFallback(t *testing.T) {
	result := detectAgentDir([]string{"mystery-binary", "--arg"}, "/my/fallback")
	if result != "/my/fallback" {
		t.Errorf("expected /my/fallback, got %s", result)
	}
}
