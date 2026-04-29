package trace

import (
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// ExecCategory represents the classification of a shell command.
type ExecCategory string

const (
	ExecCatGit            ExecCategory = "git"
	ExecCatFilesystem     ExecCategory = "filesystem"
	ExecCatEditor         ExecCategory = "editor"
	ExecCatNetwork        ExecCategory = "network"
	ExecCatPackageManager ExecCategory = "package_manager"
	ExecCatBuild          ExecCategory = "build"
	ExecCatRuntime        ExecCategory = "runtime"
	ExecCatDocker         ExecCategory = "docker"
	ExecCatTest           ExecCategory = "test"
	ExecCatLint           ExecCategory = "lint"
	ExecCatEnv            ExecCategory = "env"
	ExecCatOther          ExecCategory = "other"
)

// twoWordRules are checked first — they override single-word matches.
var twoWordRules = map[string]ExecCategory{
	"go get":      ExecCatPackageManager,
	"go install":  ExecCatPackageManager,
	"go mod":      ExecCatPackageManager,
	"go build":    ExecCatBuild,
	"cargo build": ExecCatBuild,
	"docker build": ExecCatBuild,
	"go run":      ExecCatRuntime,
	"cargo run":   ExecCatRuntime,
	"bun run":     ExecCatRuntime,
	"go test":     ExecCatTest,
	"cargo test":  ExecCatTest,
	"npm test":    ExecCatTest,
	"yarn test":   ExecCatTest,
}

// singleWordRules map executable base names to categories.
var singleWordRules = map[string]ExecCategory{
	// Git
	"git": ExecCatGit,

	// Filesystem
	"cd": ExecCatFilesystem, "mkdir": ExecCatFilesystem, "rmdir": ExecCatFilesystem,
	"rm": ExecCatFilesystem, "cp": ExecCatFilesystem, "mv": ExecCatFilesystem,
	"ls": ExecCatFilesystem, "find": ExecCatFilesystem, "chmod": ExecCatFilesystem,
	"chown": ExecCatFilesystem, "ln": ExecCatFilesystem, "touch": ExecCatFilesystem,
	"stat": ExecCatFilesystem, "realpath": ExecCatFilesystem, "readlink": ExecCatFilesystem,
	"basename": ExecCatFilesystem, "dirname": ExecCatFilesystem, "mktemp": ExecCatFilesystem,
	"install": ExecCatFilesystem, "rsync": ExecCatFilesystem, "tree": ExecCatFilesystem,
	"du": ExecCatFilesystem, "df": ExecCatFilesystem,

	// Editor / text processing
	"sed": ExecCatEditor, "awk": ExecCatEditor, "cat": ExecCatEditor,
	"head": ExecCatEditor, "tail": ExecCatEditor, "tee": ExecCatEditor,
	"grep": ExecCatEditor, "rg": ExecCatEditor, "sort": ExecCatEditor,
	"uniq": ExecCatEditor, "wc": ExecCatEditor, "cut": ExecCatEditor,
	"tr": ExecCatEditor, "xargs": ExecCatEditor, "diff": ExecCatEditor,
	"patch": ExecCatEditor, "jq": ExecCatEditor, "yq": ExecCatEditor,
	"less": ExecCatEditor, "more": ExecCatEditor, "vi": ExecCatEditor,
	"vim": ExecCatEditor, "nano": ExecCatEditor, "ed": ExecCatEditor,

	// Network
	"curl": ExecCatNetwork, "wget": ExecCatNetwork, "ssh": ExecCatNetwork,
	"scp": ExecCatNetwork, "sftp": ExecCatNetwork, "nc": ExecCatNetwork,
	"ncat": ExecCatNetwork, "dig": ExecCatNetwork, "nslookup": ExecCatNetwork,
	"host": ExecCatNetwork, "ping": ExecCatNetwork, "traceroute": ExecCatNetwork,
	"openssl": ExecCatNetwork, "http": ExecCatNetwork,

	// Package managers
	"pip": ExecCatPackageManager, "pip3": ExecCatPackageManager,
	"npm": ExecCatPackageManager, "npx": ExecCatPackageManager,
	"yarn": ExecCatPackageManager, "pnpm": ExecCatPackageManager,
	"bun": ExecCatPackageManager, "cargo": ExecCatPackageManager,
	"apt": ExecCatPackageManager, "apt-get": ExecCatPackageManager,
	"brew": ExecCatPackageManager, "conda": ExecCatPackageManager,
	"mamba": ExecCatPackageManager, "gem": ExecCatPackageManager,
	"composer": ExecCatPackageManager, "poetry": ExecCatPackageManager,
	"pdm": ExecCatPackageManager, "uv": ExecCatPackageManager,
	"pipx": ExecCatPackageManager,

	// Build
	"make": ExecCatBuild, "cmake": ExecCatBuild, "ninja": ExecCatBuild,
	"gcc": ExecCatBuild, "g++": ExecCatBuild, "cc": ExecCatBuild,
	"clang": ExecCatBuild, "tsc": ExecCatBuild, "esbuild": ExecCatBuild,
	"vite": ExecCatBuild, "webpack": ExecCatBuild, "rollup": ExecCatBuild,
	"javac": ExecCatBuild, "rustc": ExecCatBuild, "bazel": ExecCatBuild,
	"gradle": ExecCatBuild, "mvn": ExecCatBuild,

	// Runtime
	"python": ExecCatRuntime, "python3": ExecCatRuntime,
	"node": ExecCatRuntime, "java": ExecCatRuntime, "ruby": ExecCatRuntime,
	"deno": ExecCatRuntime, "php": ExecCatRuntime, "perl": ExecCatRuntime,
	"bash": ExecCatRuntime, "sh": ExecCatRuntime, "zsh": ExecCatRuntime,

	// Docker / container
	"docker": ExecCatDocker, "docker-compose": ExecCatDocker,
	"podman": ExecCatDocker, "kubectl": ExecCatDocker,
	"helm": ExecCatDocker, "skaffold": ExecCatDocker,

	// Test
	"pytest": ExecCatTest, "jest": ExecCatTest, "vitest": ExecCatTest,
	"mocha": ExecCatTest, "phpunit": ExecCatTest, "rspec": ExecCatTest,
	"unittest": ExecCatTest,

	// Lint
	"eslint": ExecCatLint, "prettier": ExecCatLint, "black": ExecCatLint,
	"ruff": ExecCatLint, "isort": ExecCatLint, "mypy": ExecCatLint,
	"pyright": ExecCatLint, "flake8": ExecCatLint, "pylint": ExecCatLint,
	"golangci-lint": ExecCatLint, "clippy": ExecCatLint,
	"shellcheck": ExecCatLint, "stylelint": ExecCatLint, "biome": ExecCatLint,

	// Env
	"export": ExecCatEnv, "env": ExecCatEnv, "printenv": ExecCatEnv,
	"source": ExecCatEnv, "which": ExecCatEnv, "where": ExecCatEnv,
	"echo": ExecCatEnv, "printf": ExecCatEnv, "set": ExecCatEnv,
	"unset": ExecCatEnv, "type": ExecCatEnv, "command": ExecCatEnv,
}

// ClassifiedCommand holds the classification result for a single simple command.
type ClassifiedCommand struct {
	Command     string       `json:"command"`
	Category    ExecCategory `json:"category"`
	Subcategory string       `json:"subcategory,omitempty"`
}

// ClassifyExecCommand determines the category and subcategory of a shell command.
// For simple commands, returns a single classification.
// For compound commands (pipes, &&, ||, ;), returns the classification of the
// first command. Use ParseAndClassify for full compound command decomposition.
func ClassifyExecCommand(command string) (ExecCategory, string) {
	command = strings.TrimSpace(command)
	if command == "" {
		return ExecCatOther, ""
	}

	cmds := ParseAndClassify(command)
	if len(cmds) == 0 {
		return ExecCatOther, ""
	}
	return cmds[0].Category, cmds[0].Subcategory
}

// ParseAndClassify parses a shell command string (which may contain pipes,
// &&, ||, ;, subshells, etc.) and returns classifications for each individual
// command. Uses mvdan.cc/sh to properly handle quoting and shell syntax.
func ParseAndClassify(command string) []ClassifiedCommand {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}

	// Parse with mvdan/sh
	parser := syntax.NewParser(syntax.KeepComments(false), syntax.Variant(syntax.LangPOSIX))
	prog, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		// Fall back to simple split if shell parse fails
		cat, sub := classifySimple(command)
		return []ClassifiedCommand{{Command: command, Category: cat, Subcategory: sub}}
	}

	var results []ClassifiedCommand
	syntax.Walk(prog, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}

		// Extract words from the call, unquoting shell literals
		var words []string
		for _, word := range call.Args {
			lit := unquoteWord(word)
			words = append(words, lit)
		}

		if len(words) == 0 {
			return true
		}

		cmdStr := strings.Join(words, " ")
		cat, sub := classifyWords(words)
		results = append(results, ClassifiedCommand{
			Command:     cmdStr,
			Category:    cat,
			Subcategory: sub,
		})

		return true
	})

	if len(results) == 0 {
		cat, sub := classifySimple(command)
		return []ClassifiedCommand{{Command: command, Category: cat, Subcategory: sub}}
	}

	return results
}

// unquoteWord extracts the literal string value from a parsed shell Word,
// stripping quotes and joining parts.
func unquoteWord(word *syntax.Word) string {
	var parts []string
	for _, part := range word.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			parts = append(parts, p.Value)
		case *syntax.SglQuoted:
			parts = append(parts, p.Value)
		case *syntax.DblQuoted:
			// Recursively extract from double-quoted parts
			for _, inner := range p.Parts {
				if lit, ok := inner.(*syntax.Lit); ok {
					parts = append(parts, lit.Value)
				}
			}
		default:
			// For complex expressions (command substitution, etc.), use the printer
			var buf strings.Builder
			syntax.NewPrinter().Print(&buf, part)
			parts = append(parts, buf.String())
		}
	}
	return strings.Join(parts, "")
}

// classifyWords classifies a parsed list of command words.
func classifyWords(words []string) (ExecCategory, string) {
	if len(words) == 0 {
		return ExecCatOther, ""
	}

	exe := filepath.Base(words[0])

	// Try two-word match first
	if len(words) >= 2 {
		twoWord := exe + " " + words[1]
		if cat, ok := twoWordRules[twoWord]; ok {
			subcategory := ""
			if len(words) > 2 {
				subcategory = words[2]
			}
			return cat, subcategory
		}
	}

	// Single-word match
	if cat, ok := singleWordRules[exe]; ok {
		subcategory := ""
		if len(words) > 1 {
			subcategory = words[1]
		}
		return cat, subcategory
	}

	return ExecCatOther, ""
}

// classifySimple is the fallback classifier using strings.Fields (no shell parsing).
func classifySimple(command string) (ExecCategory, string) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return ExecCatOther, ""
	}
	return classifyWords(parts)
}

// ClassifyAndAnnotate adds exec.category, exec.subcategory, and exec.commands
// attributes to all EXEC spans in a trace.
// For compound commands, exec.commands contains the full breakdown.
func ClassifyAndAnnotate(t *Trace) {
	for _, s := range t.Spans {
		if s.Type != SpanExec {
			continue
		}
		if s.Attributes == nil {
			continue
		}
		cmd, _ := s.Attributes["command"].(string)
		if cmd == "" {
			continue
		}

		cmds := ParseAndClassify(cmd)
		if len(cmds) == 0 {
			continue
		}

		// Primary classification is the first command
		s.Attributes["exec.category"] = string(cmds[0].Category)
		if cmds[0].Subcategory != "" {
			s.Attributes["exec.subcategory"] = cmds[0].Subcategory
		}

		// For compound commands, include the full breakdown
		if len(cmds) > 1 {
			breakdown := make([]map[string]string, len(cmds))
			categories := make([]string, len(cmds))
			for i, c := range cmds {
				breakdown[i] = map[string]string{
					"command":     c.Command,
					"category":    string(c.Category),
					"subcategory": c.Subcategory,
				}
				categories[i] = string(c.Category)
			}
			s.Attributes["exec.commands"] = breakdown
			s.Attributes["exec.categories"] = categories
			s.Attributes["exec.compound"] = true
		}
	}
}
