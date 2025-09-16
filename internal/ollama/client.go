package ollama

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git-ac/internal/config"

	"github.com/ollama/ollama/api"
)

type Client struct {
	client       *api.Client
	config       config.OllamaConfig
	commitConfig config.CommitConfig
}

func NewClient(cfg config.OllamaConfig, commitCfg config.CommitConfig) *Client {
	httpClient := &http.Client{
		Timeout: cfg.Timeout,
	}

	client := api.NewClient(&url.URL{Scheme: "http", Host: "localhost:11434"}, httpClient)
	if cfg.Host != "" {
		if u, err := url.Parse(cfg.Host); err == nil {
			client = api.NewClient(u, httpClient)
		}
	}

	return &Client{
		client:       client,
		config:       cfg,
		commitConfig: commitCfg,
	}
}

func (c *Client) HealthCheck() error {
	// Test connection with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try to list models to verify connection and get available models
	resp, err := c.client.List(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			return fmt.Errorf("cannot connect to Ollama at %s - make sure Ollama is running with 'ollama serve'", c.config.Host)
		}
		return fmt.Errorf("failed to connect to Ollama: %w", err)
	}

	// Check if the requested model is available
	modelFound := false
	var availableModels []string
	for _, model := range resp.Models {
		availableModels = append(availableModels, model.Name)
		if model.Name == c.config.Model {
			modelFound = true
			break
		}
	}

	if !modelFound {
		return fmt.Errorf("model '%s' not found - available models: %s\nPull the model with: ollama pull %s",
			c.config.Model, strings.Join(availableModels, ", "), c.config.Model)
	}

	return nil
}

func (c *Client) GenerateCommitMessage(diff, readme string) (string, error) {
	// First, check if Ollama is reachable and the model exists
	if err := c.HealthCheck(); err != nil {
		return "", err
	}

	fmt.Printf("Generating commit message using model '%s' (timeout: %v)...\n", c.config.Model, c.config.Timeout)

	// Check if diff is too large for direct processing
	if c.isDiffTooLarge(diff) {
		fmt.Println("Large diff detected, using two-stage approach...")
		return c.generateCommitMessageTwoStage(diff, readme)
	}

	// Direct approach for smaller diffs
	prompt := c.buildPrompt(diff, readme)
	return c.generateFromPrompt(prompt)
}

func (c *Client) isDiffTooLarge(diff string) bool {
	return len(diff) > c.commitConfig.LargeDiffThreshold
}

func (c *Client) generateCommitMessageTwoStage(diff, readme string) (string, error) {
	// Stage 1: Summarize changes per file
	fmt.Print("Stage 1: Analyzing file changes")
	fileSummaries, err := c.summarizeFileChanges(diff)
	if err != nil {
		return "", fmt.Errorf("failed to summarize file changes: %w", err)
	}

	// Stage 2: Generate commit message from summaries
	fmt.Print("Stage 2: Generating commit message")
	prompt := c.buildCommitPromptFromSummaries(fileSummaries, readme)
	return c.generateFromPrompt(prompt)
}

func (c *Client) summarizeFileChanges(diff string) (string, error) {
	prompt := fmt.Sprintf(`List file changes in this format:
- filename: brief description

DIFF:
%s

OUTPUT:`, diff)

	req := &api.GenerateRequest{
		Model:  c.config.Model,
		Prompt: prompt,
		Stream: new(bool),
		Options: map[string]interface{}{
			"temperature": 0.3, // Lower temperature for more focused analysis
			"top_p":       0.8,
			"num_ctx":     4096,
			// Remove num_predict limit for thinking models
			"stop": []string{"\n\nDIFF:", "\n\nCOMMIT"},
		},
	}

	return c.generateFromRequest(req)
}

func (c *Client) buildCommitPromptFromSummaries(summaries, readme string) string {
	return c.buildPromptInternal(summaries, readme, true)
}

func (c *Client) generateFromPrompt(prompt string) (string, error) {
	// Remove strict limits for thinking models
	req := &api.GenerateRequest{
		Model:  c.config.Model,
		Prompt: prompt,
		Stream: new(bool),
		Options: map[string]interface{}{
			"temperature": 0.7,
			"top_p":       0.9,
			"num_ctx":     4096,
			// Remove num_predict limit to allow thinking models to work
		},
	}

	return c.generateFromRequest(req)
}

func (c *Client) generateFromRequest(req *api.GenerateRequest) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.Timeout)
	defer cancel()

	var fullResponse strings.Builder
	fmt.Print(".")

	err := c.client.Generate(ctx, req, func(response api.GenerateResponse) error {
		fmt.Print(".")
		fullResponse.WriteString(response.Response)
		return nil
	})

	fmt.Println() // New line after dots

	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return "", fmt.Errorf("request timed out after %v - try increasing timeout in config or check if model '%s' is available", c.config.Timeout, c.config.Model)
		}
		if strings.Contains(err.Error(), "connection refused") {
			return "", fmt.Errorf("cannot connect to Ollama at %s - make sure Ollama is running", c.config.Host)
		}
		return "", fmt.Errorf("failed to generate response: %w", err)
	}

	message := strings.TrimSpace(fullResponse.String())
	if message == "" {
		return "", fmt.Errorf("received empty response from Ollama")
	}

	fmt.Printf("Raw response: %q\n", message)

	// Clean up the message
	cleanedMessage := c.cleanCommitMessage(message)

	fmt.Printf("Cleaned message: %q\n", cleanedMessage)

	if cleanedMessage == "" {
		return "", fmt.Errorf("commit message became empty after cleaning - raw response was: %q", message)
	}

	return cleanedMessage, nil
}

func (c *Client) buildPrompt(diff, readme string) string {
	return c.buildPromptInternal(diff, readme, false)
}

func (c *Client) buildPromptInternal(content, readme string, isFileSummary bool) string {
	var prompt strings.Builder

	prompt.WriteString("You are a Git commit message generator. Analyze the following changes and output ONLY a conventional commit message Your commit message must summarize the most important and significant changes present. You may optionally include an extended description if the changes are large or complex.\n\n")

	prompt.WriteString("REQUIRED FORMAT:\ntype(scope): description\n\noptional extended description\n\n")

	prompt.WriteString("VALID TYPES:\n")
	prompt.WriteString("feat - new feature\n")
	prompt.WriteString("fix - bug fix\n")
	prompt.WriteString("refactor - code refactoring\n")
	prompt.WriteString("docs - documentation\n")
	prompt.WriteString("style - formatting\n")
	prompt.WriteString("test - testing\n")
	prompt.WriteString("chore - maintenance\n\n")

	prompt.WriteString("GOOD FIRST-LINE EXAMPLES:\n")
	prompt.WriteString("feat(auth): add JWT token validation\n")
	prompt.WriteString("fix(parser): handle empty input strings\n")
	prompt.WriteString("refactor(config): simplify YAML loading\n")
	prompt.WriteString("docs: update installation guide\n\n")

	prompt.WriteString("REQUIREMENTS:\n")
	prompt.WriteString("- First line under 72 characters\n") // TODO(cdzombak): take from config
	prompt.WriteString("- Present tense (add, not added)\n")
	prompt.WriteString("- No explanations or reasoning\n")
	prompt.WriteString("- Output ONLY the commit message\n")
	prompt.WriteString("- Start immediately with type(scope):\n\n")
	prompt.WriteString("- SCOPE is not a file path/name, but one or two words summarizing the area of code that was changed. If multiple areas, exclude the scope. Scope should be meaningful to a human knowledgeable about the codebase. Be specific, not generic.\n\n")

	prompt.WriteString("GOOD SCOPE EXAMPLES: auth, parser, config, tests, api client\n")
	prompt.WriteString("BAD SCOPE EXAMPLES: internal, pkg, deps\n\n")

	if readme != "" {
		prompt.WriteString("PROJECT CONTEXT:\n")
		// Limit README content to avoid token limits
		readmeLines := strings.Split(readme, "\n")
		if len(readmeLines) > 20 {
			readmeLines = readmeLines[:20]
			readme = strings.Join(readmeLines, "\n") + "\n... (truncated)"
		}
		prompt.WriteString(readme)
		prompt.WriteString("\n\n")
	}

	if isFileSummary {
		prompt.WriteString("FILE CHANGES SUMMARY:\n")
	} else {
		prompt.WriteString("STAGED CHANGES:\n")
	}
	prompt.WriteString(content)

	return prompt.String()
}

func (c *Client) cleanCommitMessage(message string) string {
	cleaned := strings.TrimSpace(message)

	// For thinking models, look for the actual answer after </think>
	if strings.Contains(cleaned, "</think>") {
		parts := strings.Split(cleaned, "</think>")
		if len(parts) > 1 {
			// Take everything after the last </think>
			cleaned = strings.TrimSpace(parts[len(parts)-1])
		}
	}

	// Look for commit message in backticks
	if strings.Contains(cleaned, "`") && strings.Count(cleaned, "`") >= 2 {
		start := strings.Index(cleaned, "`")
		end := strings.LastIndex(cleaned, "`")
		if start != end && start >= 0 && end > start {
			cleaned = strings.TrimSpace(cleaned[start+1 : end])
		}
	}

	// Remove everything after common stopping phrases, but only if they appear after some content
	stopPhrases := []string{
		"We are generating",
		"We have the following",
		"The purpose of this",
		"This change",
		"The changes include",
		"Summary:",
		"Changes:",
		"Analysis:",
		"This commit message follows",
		"Note: If you'd like",
		"Here is a conventional",
		"The commit message",
	}

	for _, phrase := range stopPhrases {
		if idx := strings.Index(cleaned, phrase); idx > 10 { // Only cut if there's some content before
			cleaned = strings.TrimSpace(cleaned[:idx])
		}
	}

	// Remove common prefixes
	prefixes := []string{
		"commit message:",
		"commit:",
		"message:",
		"git commit:",
		"output:",
		"```",
	}

	// Remove thinking patterns
	thinkingPatterns := []string{
		"<think>",
		"</think>",
	}

	for _, pattern := range thinkingPatterns {
		cleaned = strings.ReplaceAll(cleaned, pattern, "")
	}

	// Remove text between <think> tags
	for strings.Contains(cleaned, "<think>") && strings.Contains(cleaned, "</think>") {
		start := strings.Index(cleaned, "<think>")
		end := strings.Index(cleaned, "</think>") + len("</think>")
		if start >= 0 && end > start {
			cleaned = cleaned[:start] + cleaned[end:]
		} else {
			break
		}
	}

	cleaned = strings.TrimSpace(cleaned)
	lowerCleaned := strings.ToLower(cleaned)

	// Remove prefixes
	for _, prefix := range prefixes {
		if strings.HasPrefix(lowerCleaned, prefix) {
			cleaned = strings.TrimSpace(cleaned[len(prefix):])
			lowerCleaned = strings.ToLower(cleaned)
		}
	}

	// Remove trailing artifacts
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	// If the message doesn't start with a valid type, try to find the first line that does
	validTypes := []string{"feat", "fix", "refactor", "docs", "style", "test", "chore"}
	lines := strings.Split(cleaned, "\n")

	// Check if we already have a valid start
	firstLine := strings.TrimSpace(lines[0])
	hasValidStart := false
	for _, typ := range validTypes {
		if strings.HasPrefix(firstLine, typ+"(") || strings.HasPrefix(firstLine, typ+":") {
			hasValidStart = true
			break
		}
	}

	// Only search for valid lines if the first line isn't already valid
	if !hasValidStart {
		for _, line := range lines {
			line = strings.TrimSpace(line)
			for _, typ := range validTypes {
				if strings.HasPrefix(line, typ+"(") || strings.HasPrefix(line, typ+":") {
					// Found a valid commit message line, use this as starting point
					if idx := strings.Index(cleaned, line); idx >= 0 {
						cleaned = cleaned[idx:]
					}
					goto processMultiLine
				}
			}
		}
	}

processMultiLine:

	// Handle multi-line commits based on config
	if !c.commitConfig.IncludeBody {
		// Take only the first line if body is not allowed
		lines := strings.Split(cleaned, "\n")
		if len(lines) > 0 {
			cleaned = strings.TrimSpace(lines[0])
		}
	} else {
		// For multi-line commits, ensure proper formatting
		lines := strings.Split(cleaned, "\n")
		if len(lines) > 0 {
			// Ensure subject line is under max length
			subject := strings.TrimSpace(lines[0])
			if c.commitConfig.MaxLength > 0 && len(subject) > c.commitConfig.MaxLength {
				if spaceIdx := strings.LastIndex(subject[:c.commitConfig.MaxLength], " "); spaceIdx > 0 {
					subject = subject[:spaceIdx]
				} else {
					subject = subject[:c.commitConfig.MaxLength]
				}
				lines[0] = subject
			}
			cleaned = strings.Join(lines, "\n")
		}
	}

	return cleaned
}
