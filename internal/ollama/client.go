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
	client *api.Client
	config config.OllamaConfig
}

func NewClient(cfg config.OllamaConfig) *Client {
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
		client: client,
		config: cfg,
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
	// Consider a diff "large" if it's over 2000 characters or has many files
	const maxDirectDiffSize = 2000
	lineCount := strings.Count(diff, "\n")
	fileCount := strings.Count(diff, "diff --git")

	return len(diff) > maxDirectDiffSize || fileCount > 5 || lineCount > 100
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
	prompt := fmt.Sprintf(`Analyze this git diff and summarize the changes for each file in 1-2 lines each.

FORMAT:
- filename: brief description of changes

DIFF:
%s

SUMMARIES:`, diff)

	req := &api.GenerateRequest{
		Model:  c.config.Model,
		Prompt: prompt,
		Stream: new(bool),
		Options: map[string]interface{}{
			"temperature": 0.3, // Lower temperature for more focused analysis
			"top_p":       0.8,
			"num_ctx":     4096,
			"num_predict": 300, // More tokens for summaries
			"stop":        []string{"\n\nDIFF:", "\n\nCOMMIT"},
		},
	}

	return c.generateFromRequest(req)
}

func (c *Client) buildCommitPromptFromSummaries(summaries, readme string) string {
	var prompt strings.Builder

	prompt.WriteString("Generate a Git commit message in conventional commit format.\n\n")
	prompt.WriteString("FORMAT: type(scope): description\n")
	prompt.WriteString("TYPES: feat, fix, refactor, docs, style, test, chore\n\n")

	if readme != "" {
		prompt.WriteString("PROJECT CONTEXT:\n")
		readmeLines := strings.Split(readme, "\n")
		if len(readmeLines) > 10 {
			readmeLines = readmeLines[:10]
			readme = strings.Join(readmeLines, "\n") + "\n... (truncated)"
		}
		prompt.WriteString(readme)
		prompt.WriteString("\n\n")
	}

	prompt.WriteString("FILE CHANGES SUMMARY:\n")
	prompt.WriteString(summaries)
	prompt.WriteString("\n\nCOMMIT MESSAGE:")

	return prompt.String()
}

func (c *Client) generateFromPrompt(prompt string) (string, error) {
	req := &api.GenerateRequest{
		Model:  c.config.Model,
		Prompt: prompt,
		Stream: new(bool),
		Options: map[string]interface{}{
			"temperature": 0.7,
			"top_p":       0.9,
			"num_ctx":     4096,
			"num_predict": 100,
			"stop":        []string{"\n\n"},
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

	// Clean up the message
	message = c.cleanCommitMessage(message)
	return message, nil
}

func (c *Client) buildPrompt(diff, readme string) string {
	var prompt strings.Builder

	prompt.WriteString("Generate a Git commit message in conventional commit format.\n\n")

	prompt.WriteString("FORMAT: type(scope): description\n")
	prompt.WriteString("TYPES: feat, fix, refactor, docs, style, test, chore\n")
	prompt.WriteString("EXAMPLES:\n")
	prompt.WriteString("- feat(auth): add JWT token validation\n")
	prompt.WriteString("- fix(parser): handle empty input strings\n")
	prompt.WriteString("- refactor(client): improve error handling\n")
	prompt.WriteString("- docs: update installation instructions\n\n")

	prompt.WriteString("RULES:\n")
	prompt.WriteString("- Under 72 characters\n")
	prompt.WriteString("- Use present tense\n")
	prompt.WriteString("- Be specific but concise\n")
	prompt.WriteString("- Output ONLY the commit message\n\n")

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

	prompt.WriteString("STAGED CHANGES:\n")
	prompt.WriteString(diff)
	prompt.WriteString("\n\n")

	prompt.WriteString("COMMIT MESSAGE:")

	return prompt.String()
}

func (c *Client) cleanCommitMessage(message string) string {
	// Remove common prefixes that might be added by the model
	prefixes := []string{
		"commit message:",
		"commit:",
		"message:",
		"git commit:",
		"```",
	}

	// Remove thinking tags and content
	thinkingPatterns := []string{
		"<think>",
		"</think>",
	}

	cleaned := strings.TrimSpace(message)

	// Remove thinking patterns
	for _, pattern := range thinkingPatterns {
		cleaned = strings.ReplaceAll(cleaned, pattern, "")
	}

	// Remove text between <think> tags if any remain
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

	for _, prefix := range prefixes {
		if strings.HasPrefix(lowerCleaned, prefix) {
			cleaned = strings.TrimSpace(cleaned[len(prefix):])
			lowerCleaned = strings.ToLower(cleaned)
		}
	}

	// Remove trailing ```
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	// Take only the first line if multiple lines (commit subject should be one line)
	lines := strings.Split(cleaned, "\n")
	if len(lines) > 0 {
		cleaned = strings.TrimSpace(lines[0])
	}

	return cleaned
}