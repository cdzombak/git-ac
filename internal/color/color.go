package color

import (
	"fmt"
	"os"
	"runtime"
)

// ANSI color codes
const (
	Reset = "\033[0m"
	Gray  = "\033[90m" // Bright black (gray)
	Dim   = "\033[2m"  // Dim/faint
)

// isTerminal checks if the output is going to a terminal
func isTerminal() bool {
	// Check if stdout is a terminal
	fileInfo, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	// On Unix-like systems, check if it's a character device
	if runtime.GOOS != "windows" {
		return (fileInfo.Mode() & os.ModeCharDevice) != 0
	}

	// On Windows, we're more conservative and just check for basic cases
	return !fileInfo.Mode().IsRegular()
}

// supportsColor checks if the terminal supports color output
func supportsColor() bool {
	// Check common environment variables that indicate color support
	term := os.Getenv("TERM")
	colorTerm := os.Getenv("COLORTERM")

	// Most modern terminals support color
	if term != "" && term != "dumb" {
		return true
	}

	if colorTerm != "" {
		return true
	}

	// Check for specific CI environments that support color
	if os.Getenv("CI") != "" {
		return true
	}

	return false
}

// Faint returns text in a lighter/dimmed color if the terminal supports it
func Faint(text string) string {
	if isTerminal() && supportsColor() {
		return Gray + text + Reset
	}
	return text
}

// Printf prints formatted text in a lighter/dimmed color if the terminal supports it
func FaintPrintf(format string, args ...interface{}) {
	text := fmt.Sprintf(format, args...)
	fmt.Print(Faint(text))
}
