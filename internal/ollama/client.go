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
	// Count words in the diff (split by whitespace)
	words := strings.Fields(diff)
	wordCount := len(words)

	// Context window is 4096 tokens, use half as threshold
	// Rough approximation: 1 word ≈ 1.3 tokens
	maxWords := (4096 / 2) / 1.3 // ~1575 words

	return wordCount > int(maxWords)
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
	prompt := fmt.Sprintf(`Summarize the changes in the following diff in several sentences. Pay attention to detail. The result should be a summary that is meaningful to a human knowledgeable about the codebase.

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

	prompt.WriteString("You are a Git commit message generator. " +
		"Analyze the following changes and output ONLY a conventional commit message. Your commit message must summarize the most important and significant changes present. " +
		"Be as specific as possible within the given constraints; saying 'change maximum character limit to 72' is better than 'update commit message rules'. " +
		"You may optionally include an extended description of the changes, ONLY if the changes are large or complex. Focus on the changes themselves; do not explain why you chose the type you did.\n\n")

	prompt.WriteString("REQUIRED FORMAT:\ntype(scope): summary line\n\noptional description\n\n")

	prompt.WriteString("VALID TYPES:\n")
	prompt.WriteString("feat - new or improved feature work\n")
	prompt.WriteString("fix - fixing bugs or shortcomings\n")
	prompt.WriteString("refactor - internal refactoring that improves quality, is not user-facing, and does not affect program behavior\n")
	prompt.WriteString("docs - documentation\n")
	prompt.WriteString("style - formatting\n")
	prompt.WriteString("test - testing\n")
	prompt.WriteString("chore - maintenance that is not feature-related or user-facing\n\n")

	prompt.WriteString("GOOD FIRST-LINE EXAMPLES:\n")
	prompt.WriteString("feat(auth): add JWT token validation\n")
	prompt.WriteString("fix(parser): handle empty input strings\n")
	prompt.WriteString("refactor(config): simplify YAML loading\n")
	prompt.WriteString("docs: update installation guide\n\n")

	prompt.WriteString("REQUIREMENTS:\n")
	prompt.WriteString(fmt.Sprintf("- First line of the commit message MUST be concise and under %d characters\n", c.commitConfig.MaxLength))
	prompt.WriteString("- Present tense (add, not added)\n")
	prompt.WriteString("- No explanations, reasoning, or headings\n")
	prompt.WriteString("- Output ONLY the commit message\n")
	prompt.WriteString("- Focus on the most important changes present rather than inconsequential details. Be extremely concise.\n")
	prompt.WriteString("- Start immediately with 'type(scope):'\n")
	prompt.WriteString("- SCOPE is not a file path/name, but one or two words summarizing the area of code that was changed. If multiple areas are changed, exclude the scope. Scope should be meaningful to a human knowledgeable about the codebase.\n\n")
	prompt.WriteString("GOOD SCOPE EXAMPLES: auth, parser, config, tests, api client\n")
	prompt.WriteString("BAD SCOPE EXAMPLES: internal, pkg, deps\n")
	prompt.WriteString("- If you include an extended description, it must be specific and concise. Do not include excess verbiage like 'note:' or 'these changes relate to...'. Do not prefix it with 'extended description'.\n")

	if readme != "" {
		prompt.WriteString("PROJECT README:\n")
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
		prompt.WriteString("FILE CHANGES SUMMARIZED:\n")
	} else {
		prompt.WriteString("STAGED DIFF:\n")
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

	// Remove thinking patterns
	for strings.Contains(cleaned, "<think>") && strings.Contains(cleaned, "</think>") {
		start := strings.Index(cleaned, "<think>")
		end := strings.Index(cleaned, "</think>") + len("</think>")
		if start >= 0 && end > start {
			cleaned = cleaned[:start] + cleaned[end:]
		} else {
			break
		}
	}

	// Remove remaining thinking tags
	cleaned = strings.ReplaceAll(cleaned, "<think>", "")
	cleaned = strings.ReplaceAll(cleaned, "</think>", "")
	cleaned = strings.TrimSpace(cleaned)

	// Handle multi-line commits based on config
	lines := strings.Split(cleaned, "\n")
	if len(lines) > 0 {
		// Handle first line length - split with ellipsis if too long, never truncate
		subject := strings.TrimSpace(lines[0])
		if c.commitConfig.MaxLength > 0 && len(subject) > c.commitConfig.MaxLength {
			// Find a good break point
			maxLen := c.commitConfig.MaxLength - 1 // Reserve space for "…"
			if spaceIdx := strings.LastIndex(subject[:maxLen], " "); spaceIdx > 0 {
				// Split at word boundary
				lines[0] = subject[:spaceIdx] + "…"
				// Add remainder as new line
				remainder := strings.TrimSpace(subject[spaceIdx:])
				if remainder != "" {
					lines = append([]string{lines[0], "…" + remainder}, lines[1:]...)
				}
			} else {
				// No good break point, split at character boundary
				lines[0] = subject[:maxLen] + "…"
				remainder := subject[maxLen:]
				if remainder != "" {
					lines = append([]string{lines[0], "…" + remainder}, lines[1:]...)
				}
			}
		}

		// Always allow multi-line commits - let the LLM decide
		cleaned = strings.Join(lines, "\n")
	}

	return cleaned
}
