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
	prompt := c.buildPrompt(diff, readme)

	req := &api.GenerateRequest{
		Model:  c.config.Model,
		Prompt: prompt,
		Stream: new(bool), // false
		Options: map[string]interface{}{
			"temperature": 0.7,
			"top_p":       0.9,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.config.Timeout)
	defer cancel()

	var fullResponse strings.Builder
	fmt.Print("Waiting for response")

	err := c.client.Generate(ctx, req, func(response api.GenerateResponse) error {
		fmt.Print(".")
		fullResponse.WriteString(response.Response)
		return nil
	})

	fmt.Println() // New line after dots

	if err != nil {
		// Provide more helpful error messages
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

	prompt.WriteString("You are an expert Git commit message generator. ")
	prompt.WriteString("Generate a clear, concise commit message based on the following staged changes.\n\n")

	prompt.WriteString("REQUIREMENTS:\n")
	prompt.WriteString("- Use conventional commit format (type(scope): description)\n")
	prompt.WriteString("- Keep the subject line under 72 characters\n")
	prompt.WriteString("- Use present tense (\"add\" not \"added\")\n")
	prompt.WriteString("- Be specific about what changed\n")
	prompt.WriteString("- Do not include diff syntax or file paths in the message\n\n")

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

	prompt.WriteString("Generate only the commit message, no explanations or additional text:")

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

	cleaned := strings.TrimSpace(message)
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

	// Note: MaxLength is handled in the commit config, not here
	// This could be extended to take commit config if needed

	return cleaned
}