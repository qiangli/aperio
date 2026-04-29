package runner

import (
	"os"
	"path/filepath"
	"strings"
)

// Language represents a supported agent programming language.
type Language string

const (
	LangPython  Language = "python"
	LangGo      Language = "go"
	LangNode    Language = "node"
	LangUnknown Language = "unknown"
)

// DetectLanguage determines the agent's language from the command arguments.
// It examines the executable name and file extensions.
func DetectLanguage(args []string) Language {
	if len(args) == 0 {
		return LangUnknown
	}

	exe := filepath.Base(args[0])
	exe = strings.TrimSuffix(exe, filepath.Ext(exe))

	switch {
	case isPython(exe):
		return LangPython
	case isGo(exe, args):
		return LangGo
	case isNode(exe):
		return LangNode
	}

	// Check file extension of the first non-flag argument
	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		switch filepath.Ext(arg) {
		case ".py":
			return LangPython
		case ".go":
			return LangGo
		case ".js", ".ts", ".mjs", ".mts":
			return LangNode
		}
		break
	}

	return LangUnknown
}

func isPython(exe string) bool {
	return exe == "python" || exe == "python3" || strings.HasPrefix(exe, "python3.")
}

func isGo(exe string, args []string) bool {
	if exe == "go" {
		// `go run` is an agent invocation
		if len(args) > 1 && args[1] == "run" {
			return true
		}
		return false
	}
	return false
}

func isNode(exe string) bool {
	return exe == "node" || exe == "nodejs" ||
		exe == "npx" || exe == "tsx" || exe == "ts-node" ||
		exe == "bun" || exe == "deno"
}

// detectAgentDir finds the directory containing the agent's source code.
// It looks for a script file in the command args and returns its parent directory.
// Falls back to the provided working directory.
func detectAgentDir(args []string, fallback string) string {
	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		ext := filepath.Ext(arg)
		switch ext {
		case ".py", ".js", ".ts", ".mjs", ".mts", ".go":
			absPath, err := filepath.Abs(arg)
			if err == nil {
				if info, err := os.Stat(absPath); err == nil && !info.IsDir() {
					return filepath.Dir(absPath)
				}
			}
		default:
			// Could be a directory (e.g., "go run ./cmd/agent")
			absPath, err := filepath.Abs(arg)
			if err == nil {
				if info, err := os.Stat(absPath); err == nil && info.IsDir() {
					return absPath
				}
			}
		}
	}
	return fallback
}
