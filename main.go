package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"git-ac/internal/config"
	"git-ac/internal/git"
	"git-ac/internal/provider"
	"git-ac/internal/editor"
)

var version = "<dev>"

var (
	editFlag    = flag.Bool("e", false, "Edit the generated commit message in $EDITOR before committing")
	allFlag     = flag.Bool("a", false, "Stage modified files before generating commit message")
	helpFlag    = flag.Bool("h", false, "Show help")
	versionFlag = flag.Bool("version", false, "Show version")
)

func main() {
	flag.Parse()

	if *helpFlag {
		showHelp()
		return
	}

	if *versionFlag {
		fmt.Println(version)
		os.Exit(0)
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

	// Stage all changes if -a flag is provided
	if *allFlag {
		if err := git.StageAllChanges(); err != nil {
			return fmt.Errorf("failed to stage all changes: %w", err)
		}
	}

	// Check for staged changes
	diff, err := git.GetStagedDiff()
	if err != nil {
		return fmt.Errorf("failed to get staged changes: %w", err)
	}

	if diff == "" {
		if *allFlag {
			return fmt.Errorf("no changes to stage")
		}
		return fmt.Errorf("no staged changes found (use -a to stage modified files)")
	}

	// Get README.md content for context (if it exists)
	readme := git.GetReadmeContent()

	// Generate commit message using configured provider
	llmProvider, err := provider.NewProvider(cfg)
	if err != nil {
		return fmt.Errorf("failed to create LLM provider: %w", err)
	}

	commitMsg, err := llmProvider.GenerateCommitMessage(diff, readme)
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
	fmt.Println("  -a    Stage modified files before generating commit message")
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
