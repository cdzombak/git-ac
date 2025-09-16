package provider

import (
	"fmt"
	"git-ac/internal/config"
)

// LLMProvider defines the interface for language model providers
type LLMProvider interface {
	// HealthCheck verifies the provider is accessible and configured correctly
	HealthCheck() error

	// GenerateCommitMessage generates a commit message from the given diff and readme content
	GenerateCommitMessage(diff, readme string) (string, error)
}

// NewProvider creates a new LLM provider based on the config
func NewProvider(cfg *config.Config) (LLMProvider, error) {
	switch cfg.Provider.Type {
	case "ollama":
		return NewOllamaProvider(cfg.Provider.Ollama, cfg.Provider.Timeout, cfg.Commit)
	case "openai":
		return NewOpenAIProvider(cfg.Provider.OpenAI, cfg.Provider.Timeout, cfg.Commit)
	default:
		// This should never happen due to config validation, but defensive programming
		return nil, fmt.Errorf("unsupported provider type: %s", cfg.Provider.Type)
	}
}