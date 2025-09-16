package editor

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func Edit(initialContent string) (string, error) {
	editor := getEditor()
	if editor == "" {
		return "", fmt.Errorf("no editor found - set $EDITOR environment variable")
	}

	// Create temporary file with initial content
	tmpFile, err := os.CreateTemp("", "git-ac-edit-*.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer func() {
		_ = os.Remove(tmpFile.Name())
	}()

	// Write initial content to file
	if _, err := tmpFile.WriteString(initialContent); err != nil {
		_ = tmpFile.Close()
		return "", fmt.Errorf("failed to write initial content: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("failed to close temporary file: %w", err)
	}

	// Parse editor command and arguments
	editorParts := strings.Fields(editor)
	if len(editorParts) == 0 {
		return "", fmt.Errorf("empty editor command")
	}

	// Build command with arguments and add the temp file at the end
	args := append(editorParts[1:], tmpFile.Name())
	cmd := exec.Command(editorParts[0], args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("editor failed: %w", err)
	}

	// Read the edited content
	editedContent, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		return "", fmt.Errorf("failed to read edited content: %w", err)
	}

	result := strings.TrimSpace(string(editedContent))
	if result == "" {
		return "", fmt.Errorf("commit message cannot be empty")
	}

	return result, nil
}

func getEditor() string {
	// Check EDITOR environment variable first
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}

	// Check VISUAL as fallback
	if visual := os.Getenv("VISUAL"); visual != "" {
		return visual
	}

	// Try common editors as last resort
	editors := []string{"nano", "vim", "vi", "emacs"}
	for _, editor := range editors {
		if _, err := exec.LookPath(editor); err == nil {
			return editor
		}
	}

	return ""
}
