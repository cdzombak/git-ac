package main

import (
	"flag"
	"fmt"
	"log"

	"git-ac/internal/config"
	"git-ac/internal/git"
	"git-ac/internal/ollama"
	"git-ac/internal/editor"
)

var (
	editFlag = flag.Bool("e", false, "Edit the generated commit message in $EDITOR before committing")
	helpFlag = flag.Bool("h", false, "Show help")
)

func main() {
	flag.Parse()

	if *helpFlag {
		showHelp()
		return
	}

	if err := run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate we're in a git repository
	if err := git.ValidateRepository(); err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	// Check for staged changes
	diff, err := git.GetStagedDiff()
	if err != nil {
		return fmt.Errorf("failed to get staged changes: %w", err)
	}

	if diff == "" {
		return fmt.Errorf("no staged changes found")
	}

	// Get README.md content for context (if it exists)
	readme := git.GetReadmeContent()

	// Generate commit message using Ollama
	client := ollama.NewClient(cfg.Ollama, cfg.Commit)
	commitMsg, err := client.GenerateCommitMessage(diff, readme)
	if err != nil {
		return fmt.Errorf("failed to generate commit message: %w", err)
	}

	// If edit flag is set, open editor
	if *editFlag {
		editedMsg, err := editor.Edit(commitMsg)
		if err != nil {
			return fmt.Errorf("failed to edit commit message: %w", err)
		}
		commitMsg = editedMsg
	}

	// Perform the commit
	if err := git.Commit(commitMsg); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	fmt.Printf("Successfully committed with message:\n%s\n", commitMsg)
	return nil
}

func showHelp() {
	fmt.Println("git-ac - AI-powered commit message generator")
	fmt.Println()
	fmt.Println("USAGE:")
	fmt.Println("  git-ac [flags]")
	fmt.Println()
	fmt.Println("FLAGS:")
	fmt.Println("  -e    Edit the generated commit message in $EDITOR before committing")
	fmt.Println("  -h    Show this help message")
	fmt.Println()
	fmt.Println("DESCRIPTION:")
	fmt.Println("  git-ac generates commit messages for staged changes using Ollama.")
	fmt.Println("  It analyzes git diff output and optionally includes README.md context.")
	fmt.Println()
	fmt.Println("CONFIGURATION:")
	fmt.Println("  Configuration is read from ~/.config/git-ac.yaml")
	fmt.Println("  See git-ac.yaml.sample for an example configuration.")
}