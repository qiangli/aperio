package trace

import (
	"testing"
	"time"
)

func TestClassifyExecCommand(t *testing.T) {
	tests := []struct {
		command    string
		wantCat    ExecCategory
		wantSubcat string
	}{
		// Git
		{"git commit -m 'msg'", ExecCatGit, "commit"},
		{"git push origin main", ExecCatGit, "push"},
		{"git status", ExecCatGit, "status"},
		{"git diff --cached", ExecCatGit, "diff"},
		{"git clone https://github.com/repo.git", ExecCatGit, "clone"},
		{"/usr/bin/git log --oneline", ExecCatGit, "log"},

		// Filesystem
		{"mkdir -p /tmp/dir", ExecCatFilesystem, "-p"},
		{"rm -rf /tmp/old", ExecCatFilesystem, "-rf"},
		{"cp src dst", ExecCatFilesystem, "src"},
		{"mv old new", ExecCatFilesystem, "old"},
		{"ls -la", ExecCatFilesystem, "-la"},
		{"find . -name '*.go'", ExecCatFilesystem, "."},
		{"chmod 755 script.sh", ExecCatFilesystem, "755"},
		{"touch newfile.txt", ExecCatFilesystem, "newfile.txt"},
		{"cd /tmp", ExecCatFilesystem, "/tmp"},
		{"tree src/", ExecCatFilesystem, "src/"},

		// Editor / text processing
		{"cat file.txt", ExecCatEditor, "file.txt"},
		{"grep -r pattern .", ExecCatEditor, "-r"},
		{"sed -i 's/old/new/g' file", ExecCatEditor, "-i"},
		{"head -20 log.txt", ExecCatEditor, "-20"},
		{"wc -l file.txt", ExecCatEditor, "-l"},
		{"sort names.txt", ExecCatEditor, "names.txt"},
		{"jq .data response.json", ExecCatEditor, ".data"},

		// Network
		{"curl -s https://api.example.com", ExecCatNetwork, "-s"},
		{"wget https://example.com/file.tar.gz", ExecCatNetwork, "https://example.com/file.tar.gz"},
		{"ssh user@host", ExecCatNetwork, "user@host"},
		{"ping google.com", ExecCatNetwork, "google.com"},
		{"dig example.com", ExecCatNetwork, "example.com"},

		// Package managers
		{"pip install requests", ExecCatPackageManager, "install"},
		{"npm install express", ExecCatPackageManager, "install"},
		{"yarn add react", ExecCatPackageManager, "add"},
		{"brew install go", ExecCatPackageManager, "install"},

		// Two-word: package managers
		{"go get github.com/pkg/errors", ExecCatPackageManager, "github.com/pkg/errors"},
		{"go install golang.org/x/tools/...", ExecCatPackageManager, "golang.org/x/tools/..."},
		{"go mod tidy", ExecCatPackageManager, "tidy"},

		// Two-word: build
		{"go build ./cmd/app", ExecCatBuild, "./cmd/app"},
		{"cargo build --release", ExecCatBuild, "--release"},
		{"docker build -t myapp .", ExecCatBuild, "-t"},

		// Build (single-word)
		{"make build", ExecCatBuild, "build"},
		{"cmake ..", ExecCatBuild, ".."},
		{"gcc -o main main.c", ExecCatBuild, "-o"},
		{"tsc --build", ExecCatBuild, "--build"},

		// Two-word: runtime
		{"go run main.go", ExecCatRuntime, "main.go"},
		{"cargo run -- --flag", ExecCatRuntime, "--"},

		// Runtime (single-word)
		{"python3 script.py", ExecCatRuntime, "script.py"},
		{"node server.js", ExecCatRuntime, "server.js"},
		{"bash script.sh", ExecCatRuntime, "script.sh"},

		// Docker
		{"docker run -it ubuntu", ExecCatDocker, "run"},
		{"docker-compose up -d", ExecCatDocker, "up"},
		{"kubectl apply -f deploy.yaml", ExecCatDocker, "apply"},

		// Two-word: test
		{"go test ./...", ExecCatTest, "./..."},
		{"cargo test", ExecCatTest, ""},
		{"npm test", ExecCatTest, ""},

		// Test (single-word)
		{"pytest tests/", ExecCatTest, "tests/"},
		{"jest --coverage", ExecCatTest, "--coverage"},

		// Lint
		{"eslint src/", ExecCatLint, "src/"},
		{"prettier --write .", ExecCatLint, "--write"},
		{"black .", ExecCatLint, "."},
		{"ruff check .", ExecCatLint, "check"},
		{"golangci-lint run", ExecCatLint, "run"},

		// Env
		{"echo hello", ExecCatEnv, "hello"},
		{"which python", ExecCatEnv, "python"},
		{"env", ExecCatEnv, ""},
		{"export PATH=/usr/bin", ExecCatEnv, "PATH=/usr/bin"},

		// Other / unknown
		{"custom-tool --flag", ExecCatOther, ""},
		{"/opt/bin/mystery arg1 arg2", ExecCatOther, ""},
		{"", ExecCatOther, ""},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			cat, subcat := ClassifyExecCommand(tt.command)
			if cat != tt.wantCat {
				t.Errorf("ClassifyExecCommand(%q) category = %s, want %s", tt.command, cat, tt.wantCat)
			}
			if subcat != tt.wantSubcat {
				t.Errorf("ClassifyExecCommand(%q) subcategory = %q, want %q", tt.command, subcat, tt.wantSubcat)
			}
		})
	}
}

// TestParseAndClassifyCompound tests pipe, &&, ||, and ; compound commands.
func TestParseAndClassifyCompound(t *testing.T) {
	tests := []struct {
		command    string
		wantCount  int
		wantFirst  ExecCategory
		wantCats   []ExecCategory // expected categories in order
	}{
		// Pipe: ls | grep
		{"ls -la | grep test", 2, ExecCatFilesystem, []ExecCategory{ExecCatFilesystem, ExecCatEditor}},

		// Chain: cd && touch
		{"cd /tmp && touch x", 2, ExecCatFilesystem, []ExecCategory{ExecCatFilesystem, ExecCatFilesystem}},

		// Chain: git add && git commit
		{"git add . && git commit -m 'fix'", 2, ExecCatGit, []ExecCategory{ExecCatGit, ExecCatGit}},

		// Semicolon
		{"mkdir -p dir; cd dir; touch file", 3, ExecCatFilesystem, []ExecCategory{ExecCatFilesystem, ExecCatFilesystem, ExecCatFilesystem}},

		// Mixed categories with pipe
		{"curl -s https://api.com | jq .data", 2, ExecCatNetwork, []ExecCategory{ExecCatNetwork, ExecCatEditor}},

		// Mixed categories with &&
		{"npm install && npm test", 2, ExecCatPackageManager, []ExecCategory{ExecCatPackageManager, ExecCatTest}},

		// Or chain
		{"git pull || git clone https://repo.git", 2, ExecCatGit, []ExecCategory{ExecCatGit, ExecCatGit}},

		// Complex pipe chain
		{"cat file.txt | sort | uniq -c | head -10", 4, ExecCatEditor, []ExecCategory{ExecCatEditor, ExecCatEditor, ExecCatEditor, ExecCatEditor}},

		// Mixed: git diff piped to grep
		{"git diff HEAD~1 | grep '+.*TODO'", 2, ExecCatGit, []ExecCategory{ExecCatGit, ExecCatEditor}},

		// Build then test
		{"go build ./... && go test ./...", 2, ExecCatBuild, []ExecCategory{ExecCatBuild, ExecCatTest}},

		// Single command (no compound)
		{"git status", 1, ExecCatGit, []ExecCategory{ExecCatGit}},

		// Find with exec (complex)
		{"find . -name '*.tmp' -exec rm {} \\;", 1, ExecCatFilesystem, []ExecCategory{ExecCatFilesystem}},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			results := ParseAndClassify(tt.command)
			if len(results) != tt.wantCount {
				cmds := make([]string, len(results))
				for i, r := range results {
					cmds[i] = r.Command + " [" + string(r.Category) + "]"
				}
				t.Fatalf("ParseAndClassify(%q): got %d commands %v, want %d", tt.command, len(results), cmds, tt.wantCount)
			}

			if results[0].Category != tt.wantFirst {
				t.Errorf("first command category = %s, want %s", results[0].Category, tt.wantFirst)
			}

			for i, wantCat := range tt.wantCats {
				if i >= len(results) {
					break
				}
				if results[i].Category != wantCat {
					t.Errorf("command[%d] (%s) category = %s, want %s",
						i, results[i].Command, results[i].Category, wantCat)
				}
			}
		})
	}
}

// TestParseAndClassifyQuotedArgs verifies shell quoting is handled correctly.
func TestParseAndClassifyQuotedArgs(t *testing.T) {
	tests := []struct {
		command string
		wantCat ExecCategory
		wantSub string
	}{
		{`git commit -m "fix: multi word message"`, ExecCatGit, "commit"},
		{`echo "hello world"`, ExecCatEnv, "hello world"},
		{`grep 'pattern with spaces' file.txt`, ExecCatEditor, "pattern with spaces"},
		{`sed 's/old text/new text/g' file.txt`, ExecCatEditor, "s/old text/new text/g"},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			results := ParseAndClassify(tt.command)
			if len(results) == 0 {
				t.Fatal("no results")
			}
			if results[0].Category != tt.wantCat {
				t.Errorf("category = %s, want %s", results[0].Category, tt.wantCat)
			}
			if results[0].Subcategory != tt.wantSub {
				t.Errorf("subcategory = %q, want %q", results[0].Subcategory, tt.wantSub)
			}
		})
	}
}

// TestParseAndClassifyEnvPrefix handles commands with env var prefixes.
func TestParseAndClassifyEnvPrefix(t *testing.T) {
	// GOOS=linux go build → the shell parser sees env assignment + command
	results := ParseAndClassify("GOOS=linux go build ./cmd/app")
	found := false
	for _, r := range results {
		if r.Category == ExecCatBuild {
			found = true
		}
	}
	if !found {
		cats := make([]string, len(results))
		for i, r := range results {
			cats[i] = string(r.Category) + ":" + r.Command
		}
		t.Errorf("expected build category in %v", cats)
	}
}

func TestClassifyAndAnnotate(t *testing.T) {
	now := time.Now()
	tr := &Trace{
		ID: "test",
		Spans: []*Span{
			{ID: "1", Type: SpanExec, Name: "exec: git commit", Attributes: map[string]any{"command": "git commit -m 'test'"}},
			{ID: "2", Type: SpanExec, Name: "exec: ls", Attributes: map[string]any{"command": "ls -la"}},
			{ID: "3", Type: SpanExec, Name: "exec: curl", Attributes: map[string]any{"command": "curl https://api.com"}},
			{ID: "4", Type: SpanExec, Name: "exec: npm install", Attributes: map[string]any{"command": "npm install express"}},
			{ID: "5", Type: SpanFunction, Name: "main.run", StartTime: now, EndTime: now, Attributes: map[string]any{}},
			{ID: "6", Type: SpanExec, Name: "exec: unknown", Attributes: map[string]any{"command": "mystery-tool"}},
			{ID: "7", Type: SpanExec, Name: "exec: no-command", Attributes: map[string]any{}},
			{ID: "8", Type: SpanExec, Name: "exec: pipe", Attributes: map[string]any{"command": "ls -la | grep test && echo done"}},
		},
	}

	ClassifyAndAnnotate(tr)

	checks := map[string]string{
		"1": "git",
		"2": "filesystem",
		"3": "network",
		"4": "package_manager",
		"6": "other",
	}

	for id, expectedCat := range checks {
		for _, s := range tr.Spans {
			if s.ID == id {
				cat, _ := s.Attributes["exec.category"].(string)
				if cat != expectedCat {
					t.Errorf("span %s: exec.category = %s, want %s", id, cat, expectedCat)
				}
			}
		}
	}

	// Span 5 (FUNCTION) should not have exec.category
	for _, s := range tr.Spans {
		if s.ID == "5" {
			if _, ok := s.Attributes["exec.category"]; ok {
				t.Error("FUNCTION span should not have exec.category")
			}
		}
	}

	// Span 7 (no command) should not have exec.category
	for _, s := range tr.Spans {
		if s.ID == "7" {
			if _, ok := s.Attributes["exec.category"]; ok {
				t.Error("span with no command should not have exec.category")
			}
		}
	}

	// Span 8 (compound) should have exec.compound = true
	for _, s := range tr.Spans {
		if s.ID == "8" {
			if compound, ok := s.Attributes["exec.compound"].(bool); !ok || !compound {
				t.Error("compound command should have exec.compound = true")
			}
			if cmds, ok := s.Attributes["exec.commands"].([]map[string]string); ok {
				if len(cmds) < 2 {
					t.Errorf("compound command should have 2+ sub-commands, got %d", len(cmds))
				}
			}
			if cats, ok := s.Attributes["exec.categories"].([]string); ok {
				if len(cats) < 2 {
					t.Errorf("compound command should have 2+ categories, got %d", len(cats))
				}
			}
		}
	}
}

func TestClassifyExecSubcategory(t *testing.T) {
	gitTests := []struct {
		command string
		subcat  string
	}{
		{"git commit -m 'msg'", "commit"},
		{"git push origin main", "push"},
		{"git pull --rebase", "pull"},
		{"git diff HEAD~1", "diff"},
		{"git log --oneline", "log"},
		{"git status", "status"},
		{"git checkout -b feature", "checkout"},
		{"git branch -d old", "branch"},
		{"git merge main", "merge"},
		{"git rebase main", "rebase"},
		{"git stash", "stash"},
		{"git add .", "add"},
		{"git reset HEAD~1", "reset"},
	}

	for _, tt := range gitTests {
		cat, subcat := ClassifyExecCommand(tt.command)
		if cat != ExecCatGit {
			t.Errorf("%q: expected git category, got %s", tt.command, cat)
		}
		if subcat != tt.subcat {
			t.Errorf("%q: subcategory = %q, want %q", tt.command, subcat, tt.subcat)
		}
	}
}
