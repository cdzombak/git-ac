package ollama

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git-ac/internal/config"
	"git-ac/internal/llm"

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
	return llm.IsDiffTooLarge(diff)
}

func (c *Client) generateCommitMessageTwoStage(diff, readme string) (string, error) {
	// Stage 1: Summarize changes per file
	fileSummaries, err := c.summarizeFileChanges(diff)
	if err != nil {
		return "", fmt.Errorf("failed to summarize file changes: %w", err)
	}

	// Stage 2: Generate commit message from summaries
	prompt := c.buildCommitPromptFromSummaries(fileSummaries, readme)
	return c.generateFromPrompt(prompt)
}

func (c *Client) summarizeFileChanges(diff string) (string, error) {
	prompt := llm.BuildSummarizePrompt(diff)

	req := &api.GenerateRequest{
		Model:   c.config.Model,
		Prompt:  prompt,
		Stream:  new(bool),
		Context: nil, // Explicitly clear context to prevent cross-invocation contamination
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
	return llm.BuildCommitPrompt(summaries, readme, true, c.commitConfig)
}

func (c *Client) generateFromPrompt(prompt string) (string, error) {
	// Remove strict limits for thinking models
	req := &api.GenerateRequest{
		Model:   c.config.Model,
		Prompt:  prompt,
		Stream:  new(bool),
		Context: nil, // Explicitly clear context to prevent cross-invocation contamination
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

	err := c.client.Generate(ctx, req, func(response api.GenerateResponse) error {
		fullResponse.WriteString(response.Response)
		return nil
	})

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
	cleanedMessage := llm.CleanCommitMessage(message, c.commitConfig)

	if cleanedMessage == "" {
		return "", fmt.Errorf("commit message became empty after cleaning - raw response was: %q", message)
	}

	return cleanedMessage, nil
}

func (c *Client) buildPrompt(diff, readme string) string {
	return llm.BuildCommitPrompt(diff, readme, false, c.commitConfig)
}

