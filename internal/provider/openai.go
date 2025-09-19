package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"git-ac/internal/color"
	"git-ac/internal/config"
	"git-ac/internal/llm"
)

type OpenAIProvider struct {
	config       *config.OpenAIConfig
	timeout      time.Duration
	commitConfig config.CommitConfig
	client       *http.Client
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature"`
	TopP        float64       `json:"top_p,omitempty"`
	Stop        []string      `json:"stop,omitempty"`
	Stream      bool          `json:"stream"`
}

type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func NewOpenAIProvider(cfg *config.OpenAIConfig, timeout time.Duration, commitCfg config.CommitConfig) (*OpenAIProvider, error) {
	return &OpenAIProvider{
		config:       cfg,
		timeout:      timeout,
		commitConfig: commitCfg,
		client: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

func (p *OpenAIProvider) HealthCheck() error {
	// Simple health check by making a minimal request
	req := ChatCompletionRequest{
		Model: p.config.Model,
		Messages: []ChatMessage{
			{Role: "user", Content: "test"},
		},
		MaxTokens:   1,
		Temperature: 0.1,
		Stream:      false,
	}

	_, err := p.makeRequest(req)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "no such host") {
			return fmt.Errorf("cannot connect to OpenAI API at %s - check your network connection and base_url", p.config.BaseURL)
		}
		if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "authentication") {
			return fmt.Errorf("authentication failed - check your API key")
		}
		if strings.Contains(err.Error(), "404") {
			return fmt.Errorf("model '%s' not found - check if the model exists and you have access", p.config.Model)
		}
		return fmt.Errorf("health check failed: %w", err)
	}

	return nil
}

func (p *OpenAIProvider) GenerateCommitMessage(diff, readme string) (string, error) {
	color.FaintPrintf("Generating commit message using model '%s' (timeout: %v)...\n", p.config.Model, p.timeout)

	// Check if diff is too large for direct processing
	if p.isDiffTooLarge(diff) {
		fmt.Println("Large diff detected, using two-stage approach...")
		return p.generateCommitMessageTwoStage(diff, readme)
	}

	// Direct approach for smaller diffs
	prompt := p.buildPrompt(diff, readme)
	return p.generateFromPrompt(prompt)
}

func (p *OpenAIProvider) isDiffTooLarge(diff string) bool {
	return llm.IsDiffTooLarge(diff)
}

func (p *OpenAIProvider) generateCommitMessageTwoStage(diff, readme string) (string, error) {
	// Stage 1: Summarize changes per file
	fileSummaries, err := p.summarizeFileChanges(diff)
	if err != nil {
		return "", fmt.Errorf("failed to summarize file changes: %w", err)
	}

	// Stage 2: Generate commit message from summaries
	prompt := p.buildCommitPromptFromSummaries(fileSummaries, readme)
	return p.generateFromPrompt(prompt)
}

func (p *OpenAIProvider) summarizeFileChanges(diff string) (string, error) {
	prompt := llm.BuildSummarizePrompt(diff)

	req := ChatCompletionRequest{
		Model: p.config.Model,
		Messages: []ChatMessage{
			{Role: "user", Content: prompt},
		},
		MaxTokens:   4096,                                // Match Ollama's num_ctx
		Temperature: 0.3,                                 // Lower temperature for more focused analysis
		TopP:        0.8,                                 // Match Ollama's top_p
		Stop:        []string{"\n\nDIFF:", "\n\nCOMMIT"}, // Match Ollama's stop sequences
		Stream:      false,
	}

	return p.generateFromRequest(req)
}

func (p *OpenAIProvider) buildCommitPromptFromSummaries(summaries, readme string) string {
	return llm.BuildCommitPrompt(summaries, readme, true, p.commitConfig)
}

func (p *OpenAIProvider) generateFromPrompt(prompt string) (string, error) {
	req := ChatCompletionRequest{
		Model: p.config.Model,
		Messages: []ChatMessage{
			{Role: "user", Content: prompt},
		},
		MaxTokens:   4096, // Match Ollama's num_ctx
		Temperature: 0.7,  // Match Ollama's generation temperature
		TopP:        0.9,  // Match Ollama's generation top_p
		Stream:      false,
	}

	return p.generateFromRequest(req)
}

func (p *OpenAIProvider) generateFromRequest(req ChatCompletionRequest) (string, error) {
	resp, err := p.makeRequest(req)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	message := strings.TrimSpace(resp.Choices[0].Message.Content)
	if message == "" {
		return "", fmt.Errorf("received empty response from OpenAI")
	}

	// Clean up the message
	cleanedMessage := llm.CleanCommitMessage(message, p.commitConfig)

	if cleanedMessage == "" {
		return "", fmt.Errorf("commit message became empty after cleaning - raw response was: %q", message)
	}

	return cleanedMessage, nil
}

func (p *OpenAIProvider) makeRequest(req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), "POST", p.config.BaseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") || strings.Contains(err.Error(), "timeout") {
			return nil, fmt.Errorf("request timed out after %v - try increasing timeout in config or check if the API is accessible", p.timeout)
		}
		if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "no such host") {
			return nil, fmt.Errorf("cannot connect to OpenAI API at %s - check your network connection and base_url", p.config.BaseURL)
		}
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		switch resp.StatusCode {
		case 401:
			return nil, fmt.Errorf("authentication failed (401) - check your API key")
		case 404:
			return nil, fmt.Errorf("model '%s' not found (404) - check if the model exists and you have access", p.config.Model)
		case 429:
			return nil, fmt.Errorf("rate limit exceeded (429) - try again later or increase timeout")
		case 500, 502, 503, 504:
			return nil, fmt.Errorf("server error (%d) - the API service may be experiencing issues", resp.StatusCode)
		default:
			return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
		}
	}

	var chatResp ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &chatResp, nil
}

func (p *OpenAIProvider) buildPrompt(diff, readme string) string {
	return llm.BuildCommitPrompt(diff, readme, false, p.commitConfig)
}
