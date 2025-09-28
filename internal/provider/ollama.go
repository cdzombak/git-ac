package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git-ac/internal/color"
	"git-ac/internal/config"
	"git-ac/internal/llm"

	"github.com/ollama/ollama/api"
)

type OllamaProvider struct {
	client       *api.Client
	config       *config.OllamaConfig
	timeout      time.Duration
	commitConfig config.CommitConfig
}

func NewOllamaProvider(cfg *config.OllamaConfig, timeout time.Duration, commitCfg config.CommitConfig) (*OllamaProvider, error) {
	httpClient := &http.Client{
		Timeout: timeout,
	}

	client := api.NewClient(&url.URL{Scheme: "http", Host: "localhost:11434"}, httpClient)
	if cfg.Host != "" {
		if u, err := url.Parse(cfg.Host); err == nil {
			client = api.NewClient(u, httpClient)
		}
	}

	return &OllamaProvider{
		client:       client,
		config:       cfg,
		timeout:      timeout,
		commitConfig: commitCfg,
	}, nil
}

func (p *OllamaProvider) HealthCheck() error {
	// Test connection with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try to list models to verify connection and get available models
	resp, err := p.client.List(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			return fmt.Errorf("cannot connect to Ollama at %s - make sure Ollama is running with 'ollama serve'", p.config.Host)
		}
		return fmt.Errorf("failed to connect to Ollama: %w", err)
	}

	// Check if the requested model is available
	modelFound := false
	var availableModels []string
	for _, model := range resp.Models {
		availableModels = append(availableModels, model.Name)
		if model.Name == p.config.Model {
			modelFound = true
			break
		}
	}

	if !modelFound {
		return fmt.Errorf("model '%s' not found - available models: %s\nPull the model with: ollama pull %s",
			p.config.Model, strings.Join(availableModels, ", "), p.config.Model)
	}

	return nil
}

func (p *OllamaProvider) GenerateCommitMessage(diff, readme string) (string, error) {
	// First, check if Ollama is reachable and the model exists
	if err := p.HealthCheck(); err != nil {
		return "", err
	}

	color.FaintPrintf("Generating commit message using model '%s' (timeout: %v)...\n", p.config.Model, p.timeout)

	// Check if diff is too large for direct processing
	if llm.IsDiffTooLarge(diff, p.commitConfig) {
		return p.generateCommitMessageTwoStage(diff, readme)
	}

	// Direct approach for smaller diffs
	prompt := llm.BuildCommitPrompt(diff, readme, false, p.commitConfig)
	return p.generateFromPrompt(prompt)
}

func (p *OllamaProvider) generateCommitMessageTwoStage(diff, readme string) (string, error) {
	// Stage 1: Summarize changes per file
	fileSummaries, err := p.summarizeFileChanges(diff)
	if err != nil {
		return "", fmt.Errorf("failed to summarize file changes: %w", err)
	}

	// Stage 2: Generate commit message from summaries
	prompt := llm.BuildCommitPrompt(fileSummaries, readme, true, p.commitConfig)
	return p.generateFromPrompt(prompt)
}

func (p *OllamaProvider) summarizeFileChanges(diff string) (string, error) {
	prompt := llm.BuildSummarizePrompt(diff)

	req := &api.GenerateRequest{
		Model:   p.config.Model,
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

	return p.generateFromRequest(req)
}

func (p *OllamaProvider) generateFromPrompt(prompt string) (string, error) {
	// Remove strict limits for thinking models
	req := &api.GenerateRequest{
		Model:   p.config.Model,
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

	return p.generateFromRequest(req)
}

func (p *OllamaProvider) generateFromRequest(req *api.GenerateRequest) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	var fullResponse strings.Builder

	err := p.client.Generate(ctx, req, func(response api.GenerateResponse) error {
		fullResponse.WriteString(response.Response)
		return nil
	})

	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return "", fmt.Errorf("request timed out after %v - try increasing timeout in config or check if model '%s' is available", p.timeout, p.config.Model)
		}
		if strings.Contains(err.Error(), "connection refused") {
			return "", fmt.Errorf("cannot connect to Ollama at %s - make sure Ollama is running", p.config.Host)
		}
		return "", fmt.Errorf("failed to generate response: %w", err)
	}

	message := strings.TrimSpace(fullResponse.String())
	if message == "" {
		return "", fmt.Errorf("received empty response from Ollama")
	}

	// Clean up the message
	cleanedMessage := llm.CleanCommitMessage(message, p.commitConfig)

	if cleanedMessage == "" {
		return "", fmt.Errorf("commit message became empty after cleaning - raw response was: %q", message)
	}

	return cleanedMessage, nil
}
