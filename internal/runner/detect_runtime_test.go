package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func createTestBinary(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDetectRuntimeShebangNode(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		shebang string
	}{
		{"env node", "#!/usr/bin/env node\nconsole.log('hi');\n"},
		{"direct node", "#!/usr/bin/node\nconsole.log('hi');\n"},
		{"env tsx", "#!/usr/bin/env tsx\nconsole.log('hi');\n"},
		{"env bun", "#!/usr/bin/env bun\nconsole.log('hi');\n"},
		{"env deno", "#!/usr/bin/env deno\nconsole.log('hi');\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := createTestBinary(t, dir, "agent-"+tt.name, tt.shebang)
			info, err := DetectRuntime(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Runtime != LangNode {
				t.Errorf("expected node, got %s", info.Runtime)
			}
			if info.Confidence != "shebang" {
				t.Errorf("expected shebang confidence, got %s", info.Confidence)
			}
		})
	}
}

func TestDetectRuntimeShebangPython(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		shebang string
	}{
		{"env python3", "#!/usr/bin/env python3\nprint('hi')\n"},
		{"env python", "#!/usr/bin/env python\nprint('hi')\n"},
		{"direct python3", "#!/usr/bin/python3\nprint('hi')\n"},
		{"env python3.11", "#!/usr/bin/env python3.11\nprint('hi')\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := createTestBinary(t, dir, "agent-"+tt.name, tt.shebang)
			info, err := DetectRuntime(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Runtime != LangPython {
				t.Errorf("expected python, got %s", info.Runtime)
			}
			if info.Confidence != "shebang" {
				t.Errorf("expected shebang confidence, got %s", info.Confidence)
			}
		})
	}
}

func TestDetectRuntimeShellWrapperNode(t *testing.T) {
	dir := t.TempDir()

	// npm-installed binary pattern (bash wrapper that execs node)
	content := `#!/bin/bash
basedir=$(dirname "$(echo "$0" | sed -e 's,\\\\,/,g')")
exec node "$basedir/../lib/node_modules/claude-code/cli.js" "$@"
`
	path := createTestBinary(t, dir, "claude", content)
	info, err := DetectRuntime(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Runtime != LangNode {
		t.Errorf("expected node from shell wrapper, got %s", info.Runtime)
	}
}

func TestDetectRuntimeShellWrapperNodeModules(t *testing.T) {
	dir := t.TempDir()

	// Another npm pattern with node_modules reference
	content := `#!/bin/sh
node "/usr/local/lib/node_modules/@anthropic-ai/claude-code/cli.js" "$@"
`
	path := createTestBinary(t, dir, "claude", content)
	info, err := DetectRuntime(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Runtime != LangNode {
		t.Errorf("expected node from node_modules pattern, got %s", info.Runtime)
	}
}

func TestDetectRuntimeShellWrapperPython(t *testing.T) {
	dir := t.TempDir()

	// pip-installed binary pattern (python shebang with imports)
	content := `#!/bin/sh
'''exec' python3 "$0" "$@"
' '''
from aider.main import main
main()
`
	path := createTestBinary(t, dir, "aider", content)
	info, err := DetectRuntime(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Runtime != LangPython {
		t.Errorf("expected python from shell wrapper with imports, got %s", info.Runtime)
	}
}

func TestDetectRuntimeKnownNames(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name string
		want Language
	}{
		{"claude", LangNode},
		{"claude-code", LangNode},
		{"aider", LangPython},
		{"copilot", LangNode},
		{"cody", LangNode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a binary with no shebang and no recognizable file type
			path := createTestBinary(t, dir, tt.name, "some opaque content\n")
			info, err := DetectRuntime(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Runtime != tt.want {
				t.Errorf("known name %q: expected %s, got %s", tt.name, tt.want, info.Runtime)
			}
			if info.Confidence != "known_name" {
				t.Errorf("expected known_name confidence, got %s", info.Confidence)
			}
		})
	}
}

func TestDetectRuntimeUnknownBinary(t *testing.T) {
	dir := t.TempDir()
	path := createTestBinary(t, dir, "mystery-tool", "opaque binary content\x00\x01\x02\n")

	info, err := DetectRuntime(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Runtime != LangUnknown {
		t.Errorf("expected unknown, got %s", info.Runtime)
	}
}

func TestDetectRuntimeNonExistent(t *testing.T) {
	_, err := DetectRuntime("/nonexistent/binary")
	if err == nil {
		t.Fatal("expected error for nonexistent binary")
	}
}

func TestDetectRuntimeSymlink(t *testing.T) {
	dir := t.TempDir()

	// Create real binary
	realPath := createTestBinary(t, dir, "real-agent", "#!/usr/bin/env node\nconsole.log('hi');\n")

	// Create symlink
	linkPath := filepath.Join(dir, "agent-link")
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Skip("symlinks not supported")
	}

	info, err := DetectRuntime(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Runtime != LangNode {
		t.Errorf("expected node through symlink, got %s", info.Runtime)
	}
	// BinaryPath should be the resolved real path (may differ due to /var → /private/var on macOS)
	resolvedReal, _ := filepath.EvalSymlinks(realPath)
	if info.BinaryPath != resolvedReal {
		t.Errorf("BinaryPath should be resolved: got %s, want %s", info.BinaryPath, resolvedReal)
	}
}

func TestDetectRuntimeELFBinary(t *testing.T) {
	dir := t.TempDir()
	// Create fake ELF binary (magic bytes: 0x7f ELF)
	content := []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00}
	path := filepath.Join(dir, "go-binary")
	os.WriteFile(path, content, 0755)

	info, err := DetectRuntime(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Runtime != LangGo {
		t.Errorf("expected go for ELF binary, got %s", info.Runtime)
	}
	if info.Confidence != "file_type" {
		t.Errorf("expected file_type confidence, got %s", info.Confidence)
	}
}

func TestDetectRuntimeMachOBinary(t *testing.T) {
	dir := t.TempDir()
	// Mach-O 64-bit magic: 0xCF 0xFA 0xED 0xFE (little-endian FEEDFACF)
	content := []byte{0xCF, 0xFA, 0xED, 0xFE, 0x07, 0x00, 0x00, 0x01}
	path := filepath.Join(dir, "macho-binary")
	os.WriteFile(path, content, 0755)

	info, err := DetectRuntime(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Runtime != LangGo {
		t.Errorf("expected go for Mach-O binary, got %s", info.Runtime)
	}
}

func TestIsNodeInterpreter(t *testing.T) {
	for _, name := range []string{"node", "nodejs", "tsx", "ts-node", "bun", "deno"} {
		if !isNodeInterpreter(name) {
			t.Errorf("%s should be node interpreter", name)
		}
	}
	if isNodeInterpreter("python3") {
		t.Error("python3 should not be node interpreter")
	}
}

func TestIsPythonInterpreter(t *testing.T) {
	for _, name := range []string{"python", "python3", "python3.11"} {
		if !isPythonInterpreter(name) {
			t.Errorf("%s should be python interpreter", name)
		}
	}
	if isPythonInterpreter("node") {
		t.Error("node should not be python interpreter")
	}
}

func TestIsShellInterpreter(t *testing.T) {
	for _, name := range []string{"bash", "sh", "zsh", "dash", "fish"} {
		if !isShellInterpreter(name) {
			t.Errorf("%s should be shell interpreter", name)
		}
	}
}
