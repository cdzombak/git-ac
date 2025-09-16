package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Provider ProviderConfig `yaml:"provider"`
	Commit   CommitConfig   `yaml:"commit"`
}

type ProviderConfig struct {
	Type    string        `yaml:"type"`    // "ollama" or "openai"
	Timeout time.Duration `yaml:"timeout"`

	// Ollama-specific config
	Ollama *OllamaConfig `yaml:"ollama,omitempty"`

	// OpenAI-compatible config
	OpenAI *OpenAIConfig `yaml:"openai,omitempty"`
}

type OllamaConfig struct {
	Host    string        `yaml:"host"`
	Model   string        `yaml:"model"`
	Timeout time.Duration `yaml:"-"` // Not serialized, passed from provider config
}

type OpenAIConfig struct {
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
	Model   string `yaml:"model"`
}

type CommitConfig struct {
	MaxLength int `yaml:"max_length"`
}

func Load() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".config", "git-ac.yaml")

	// Start with defaults
	cfg := &Config{
		Provider: ProviderConfig{
			Type:    "ollama",
			Timeout: 30 * time.Second,
			Ollama: &OllamaConfig{
				Host:  "http://localhost:11434",
				Model: "llama2",
			},
		},
		Commit: CommitConfig{
			MaxLength: 72,
		},
	}

	// Try to load config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Config file doesn't exist, use defaults
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate config
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	switch c.Provider.Type {
	case "ollama":
		if c.Provider.Ollama == nil {
			return fmt.Errorf("ollama config required when provider type is 'ollama'")
		}
		if c.Provider.Ollama.Host == "" {
			return fmt.Errorf("ollama host is required")
		}
		if c.Provider.Ollama.Model == "" {
			return fmt.Errorf("ollama model is required")
		}
	case "openai":
		if c.Provider.OpenAI == nil {
			return fmt.Errorf("openai config required when provider type is 'openai'")
		}
		if c.Provider.OpenAI.BaseURL == "" {
			return fmt.Errorf("openai base_url is required")
		}
		if c.Provider.OpenAI.APIKey == "" {
			return fmt.Errorf("openai api_key is required")
		}
		if c.Provider.OpenAI.Model == "" {
			return fmt.Errorf("openai model is required")
		}
	default:
		return fmt.Errorf("unsupported provider type: %s (supported: ollama, openai)", c.Provider.Type)
	}

	return nil
}
