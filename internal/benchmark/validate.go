package benchmark

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// ValidationResult holds the outcome of a single validation check.
type ValidationResult struct {
	Check  ValidationCheck `json:"check"`
	Passed bool            `json:"passed"`
	Detail string          `json:"detail,omitempty"`
}

// RunChecks executes all validation checks and returns results.
func RunChecks(checks []ValidationCheck, workDir string, exitCode int, outputText string) []ValidationResult {
	var results []ValidationResult
	for _, check := range checks {
		result := runCheck(check, workDir, exitCode, outputText)
		results = append(results, result)
	}
	return results
}

// ComputePassRate calculates the fraction of passed checks.
func ComputePassRate(results []ValidationResult) float64 {
	if len(results) == 0 {
		return 1.0 // no checks = pass
	}
	passed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		}
	}
	return float64(passed) / float64(len(results))
}

func runCheck(check ValidationCheck, workDir string, exitCode int, outputText string) ValidationResult {
	switch check.Type {
	case "exit_code":
		return checkExitCode(check, exitCode)
	case "file_exists":
		return checkFileExists(check, workDir)
	case "regex":
		return checkRegex(check, outputText)
	case "command":
		return checkCommand(check, workDir)
	case "git_diff":
		return checkGitDiff(check, workDir)
	default:
		return ValidationResult{
			Check:  check,
			Passed: false,
			Detail: fmt.Sprintf("unknown check type: %s", check.Type),
		}
	}
}

func checkExitCode(check ValidationCheck, exitCode int) ValidationResult {
	expected := "0"
	if check.Expect != "" {
		expected = check.Expect
	}
	passed := fmt.Sprintf("%d", exitCode) == expected
	return ValidationResult{
		Check:  check,
		Passed: passed,
		Detail: fmt.Sprintf("exit code %d (expected %s)", exitCode, expected),
	}
}

func checkFileExists(check ValidationCheck, workDir string) ValidationResult {
	path := check.Value
	if !filepath.IsAbs(path) {
		path = filepath.Join(workDir, path)
	}
	_, err := os.Stat(path)
	passed := err == nil
	detail := "file exists"
	if !passed {
		detail = fmt.Sprintf("file not found: %s", path)
	}
	return ValidationResult{Check: check, Passed: passed, Detail: detail}
}

func checkRegex(check ValidationCheck, outputText string) ValidationResult {
	re, err := regexp.Compile(check.Value)
	if err != nil {
		return ValidationResult{
			Check:  check,
			Passed: false,
			Detail: fmt.Sprintf("invalid regex: %s", err),
		}
	}
	passed := re.MatchString(outputText)
	detail := "pattern matched"
	if !passed {
		detail = "pattern not found in output"
	}
	return ValidationResult{Check: check, Passed: passed, Detail: detail}
}

func checkCommand(check ValidationCheck, workDir string) ValidationResult {
	cmd := exec.Command("sh", "-c", check.Value)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	passed := err == nil
	detail := strings.TrimSpace(string(output))
	if len(detail) > 200 {
		detail = detail[:200] + "..."
	}
	if !passed {
		detail = fmt.Sprintf("command failed: %s", detail)
	}
	return ValidationResult{Check: check, Passed: passed, Detail: detail}
}

func checkGitDiff(check ValidationCheck, workDir string) ValidationResult {
	cmd := exec.Command("git", "diff", "--name-only", "HEAD")
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ValidationResult{
			Check:  check,
			Passed: false,
			Detail: fmt.Sprintf("git diff failed: %s", err),
		}
	}

	changedFiles := strings.Split(strings.TrimSpace(string(output)), "\n")
	expectedFiles := strings.Split(check.Value, ",")

	for _, expected := range expectedFiles {
		expected = strings.TrimSpace(expected)
		found := false
		for _, changed := range changedFiles {
			if strings.TrimSpace(changed) == expected {
				found = true
				break
			}
		}
		if !found {
			return ValidationResult{
				Check:  check,
				Passed: false,
				Detail: fmt.Sprintf("expected file %q not in git diff", expected),
			}
		}
	}

	return ValidationResult{
		Check:  check,
		Passed: true,
		Detail: fmt.Sprintf("all expected files found in diff"),
	}
}
