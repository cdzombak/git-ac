package git

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func ValidateRepository() error {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("not a git repository")
	}
	return nil
}

func GetStagedDiff() (string, error) {
	cmd := exec.Command("git", "diff", "--cached")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get staged diff: %w", err)
	}

	// Transform diff format for better LLM readability
	diff := string(output)
	return transformDiffForLLM(diff), nil
}

func transformDiffForLLM(diff string) string {
	lines := strings.Split(diff, "\n")
	var transformedLines []string

	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			// Replace + with ADDED: (preserve the rest of the line)
			transformedLines = append(transformedLines, "ADDED: "+line[1:])
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			// Replace - with REMOVED: (preserve the rest of the line)
			transformedLines = append(transformedLines, "REMOVED: "+line[1:])
		} else if strings.HasPrefix(line, " ") && len(line) > 1 {
			// Context lines (unchanged code) start with space
			transformedLines = append(transformedLines, "UNCHANGED:"+line)
		} else {
			// Keep other lines as-is (headers, file markers, etc.)
			transformedLines = append(transformedLines, line)
		}
	}

	return strings.Join(transformedLines, "\n")
}

func GetReadmeContent() string {
	readmeFiles := []string{"README.md", "readme.md", "Readme.md", "README", "readme"}

	for _, filename := range readmeFiles {
		if content, err := os.ReadFile(filename); err == nil {
			return string(content)
		}
	}

	return ""
}

func Commit(message string) error {
	// Write commit message to temporary file to handle multiline messages properly
	tmpFile, err := os.CreateTemp("", "git-ac-commit-*.txt")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(message); err != nil {
		return fmt.Errorf("failed to write commit message: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	cmd := exec.Command("git", "commit", "-F", tmpFile.Name())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}

	return nil
}

func StageAllChanges() error {
	cmd := exec.Command("git", "add", ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	return nil
}

func GetRepositoryRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get repository root: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}