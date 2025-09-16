package provider

import (
	"time"

	"git-ac/internal/config"
	"git-ac/internal/ollama"
)

type OllamaProvider struct {
	client *ollama.Client
}

func NewOllamaProvider(cfg *config.OllamaConfig, timeout time.Duration, commitCfg config.CommitConfig) (*OllamaProvider, error) {
	// Create a copy of the config with timeout set
	ollamaConfig := config.OllamaConfig{
		Host:    cfg.Host,
		Model:   cfg.Model,
		Timeout: timeout,
	}

	client := ollama.NewClient(ollamaConfig, commitCfg)

	return &OllamaProvider{
		client: client,
	}, nil
}

func (p *OllamaProvider) HealthCheck() error {
	return p.client.HealthCheck()
}

func (p *OllamaProvider) GenerateCommitMessage(diff, readme string) (string, error) {
	return p.client.GenerateCommitMessage(diff, readme)
}