package runner

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RuntimeInfo holds the detected runtime information for a binary.
type RuntimeInfo struct {
	Runtime    Language // python, node, go, unknown
	Interpreter string  // full path to interpreter (e.g., "/usr/bin/node")
	BinaryPath string  // resolved absolute path of the binary
	Confidence string  // "shebang", "file_type", "known_name"
}

// knownAgentBinaries maps popular AI agent binary names to their runtime.
var knownAgentBinaries = map[string]Language{
	"claude":      LangNode,
	"claude-code": LangNode,
	"copilot":     LangNode,
	"continue":    LangNode,
	"cody":        LangNode,
	"aider":       LangPython,
	"open-interpreter": LangPython,
	"gpt-engineer":     LangPython,
	"mentat":      LangPython,
	"smol-developer":   LangPython,
}

// DetectRuntime inspects a binary to determine its runtime language.
// It checks (in order): shebang line, file type, known binary names.
func DetectRuntime(binary string) (*RuntimeInfo, error) {
	// Resolve the binary path
	binaryPath, err := exec.LookPath(binary)
	if err != nil {
		return nil, err
	}
	binaryPath, err = filepath.Abs(binaryPath)
	if err != nil {
		return nil, err
	}

	// Follow symlinks
	resolved, err := filepath.EvalSymlinks(binaryPath)
	if err == nil {
		binaryPath = resolved
	}

	info := &RuntimeInfo{
		Runtime:    LangUnknown,
		BinaryPath: binaryPath,
	}

	// 1. Check shebang
	if detectShebang(binaryPath, info) {
		return info, nil
	}

	// 2. Check file type (magic bytes)
	if detectFileType(binaryPath, info) {
		return info, nil
	}

	// 3. Check known binary names
	if detectKnownName(binary, info) {
		return info, nil
	}

	return info, nil
}

// detectShebang reads the first line of a file to check for a shebang (#!) line.
func detectShebang(path string, info *RuntimeInfo) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	// Read first line
	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return false
	}
	line := scanner.Text()

	if !strings.HasPrefix(line, "#!") {
		return false
	}

	shebang := strings.TrimPrefix(line, "#!")
	shebang = strings.TrimSpace(shebang)

	// Parse the interpreter
	// Handle: "#!/usr/bin/env node", "#!/usr/bin/node", "#!/usr/bin/env python3"
	parts := strings.Fields(shebang)
	if len(parts) == 0 {
		return false
	}

	interpreter := parts[0]
	// If using /usr/bin/env, the actual interpreter is the next arg
	if filepath.Base(interpreter) == "env" && len(parts) > 1 {
		interpreter = parts[1]
	}

	interpreterBase := filepath.Base(interpreter)

	switch {
	case isNodeInterpreter(interpreterBase):
		info.Runtime = LangNode
		info.Interpreter = interpreter
		info.Confidence = "shebang"
		return true
	case isPythonInterpreter(interpreterBase):
		info.Runtime = LangPython
		info.Interpreter = interpreter
		info.Confidence = "shebang"
		return true
	case isShellInterpreter(interpreterBase):
		// Shell scripts — scan content for clues about what they launch
		return detectShellScriptContent(path, info)
	}

	return false
}

// detectShellScriptContent scans a shell script for clues about what runtime it wraps.
// Many npm/pip installed binaries are shell wrapper scripts.
func detectShellScriptContent(path string, info *RuntimeInfo) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineCount := 0
	for scanner.Scan() {
		lineCount++
		if lineCount > 50 { // only scan first 50 lines
			break
		}
		line := strings.TrimSpace(scanner.Text())

		// Look for exec node, exec python patterns
		if strings.Contains(line, "exec") && strings.Contains(line, "node") {
			info.Runtime = LangNode
			info.Interpreter = "node"
			info.Confidence = "file_type"
			return true
		}
		if strings.Contains(line, "exec") && (strings.Contains(line, "python") || strings.Contains(line, "python3")) {
			info.Runtime = LangPython
			info.Interpreter = "python3"
			info.Confidence = "file_type"
			return true
		}
		// npm wrapper pattern: node "$basedir/../lib/node_modules/..."
		if strings.Contains(line, "node_modules") {
			info.Runtime = LangNode
			info.Interpreter = "node"
			info.Confidence = "file_type"
			return true
		}
		// pip wrapper pattern: from module import ...
		if strings.HasPrefix(line, "from ") || strings.HasPrefix(line, "import ") {
			info.Runtime = LangPython
			info.Interpreter = "python3"
			info.Confidence = "file_type"
			return true
		}
	}

	return false
}

// detectFileType checks magic bytes to identify compiled binaries.
func detectFileType(path string, info *RuntimeInfo) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	header := make([]byte, 4)
	n, err := f.Read(header)
	if err != nil || n < 4 {
		return false
	}

	// ELF binary (Linux)
	if header[0] == 0x7f && header[1] == 'E' && header[2] == 'L' && header[3] == 'F' {
		info.Runtime = LangGo // assume Go for now — most Go binaries are ELF
		info.Confidence = "file_type"
		return true
	}

	// Mach-O binary (macOS)
	// MH_MAGIC: 0xFEEDFACE (32-bit), MH_MAGIC_64: 0xFEEDFACF (64-bit)
	// Also FAT_MAGIC: 0xCAFEBABE (universal)
	if (header[0] == 0xFE && header[1] == 0xED && header[2] == 0xFA) ||
		(header[0] == 0xCF && header[1] == 0xFA && header[2] == 0xED) ||
		(header[0] == 0xCA && header[1] == 0xFE && header[2] == 0xBA && header[3] == 0xBE) {
		info.Runtime = LangGo // assume Go for compiled binaries
		info.Confidence = "file_type"
		return true
	}

	return false
}

// detectKnownName checks the binary name against a table of known AI agents.
func detectKnownName(binary string, info *RuntimeInfo) bool {
	name := filepath.Base(binary)
	if lang, ok := knownAgentBinaries[name]; ok {
		info.Runtime = lang
		info.Confidence = "known_name"
		return true
	}
	return false
}

func isNodeInterpreter(name string) bool {
	return name == "node" || name == "nodejs" || name == "tsx" ||
		name == "ts-node" || name == "bun" || name == "deno"
}

func isPythonInterpreter(name string) bool {
	return name == "python" || name == "python3" ||
		strings.HasPrefix(name, "python3.")
}

func isShellInterpreter(name string) bool {
	return name == "bash" || name == "sh" || name == "zsh" ||
		name == "dash" || name == "fish"
}
