package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Provider ProviderConfig `yaml:"provider"`
	Commit   CommitConfig   `yaml:"commit"`
}

type ProviderConfig struct {
	Type    string        `yaml:"type"` // "ollama" or "openai"
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
	// Validate provider type
	if c.Provider.Type == "" {
		return fmt.Errorf("provider type is required (supported: ollama, openai)")
	}

	// Validate timeout
	if c.Provider.Timeout <= 0 {
		return fmt.Errorf("provider timeout must be positive (got %v)", c.Provider.Timeout)
	}
	if c.Provider.Timeout > 10*time.Minute {
		return fmt.Errorf("provider timeout is too large (got %v, maximum 10m)", c.Provider.Timeout)
	}

	// Validate commit config
	if err := c.validateCommitConfig(); err != nil {
		return fmt.Errorf("commit config validation failed: %w", err)
	}

	// Validate provider-specific config
	switch c.Provider.Type {
	case "ollama":
		return c.validateOllamaConfig()
	case "openai":
		return c.validateOpenAIConfig()
	default:
		return fmt.Errorf("unsupported provider type '%s' (supported: ollama, openai)", c.Provider.Type)
	}
}

func (c *Config) validateCommitConfig() error {
	if c.Commit.MaxLength <= 0 {
		return fmt.Errorf("max_length must be positive (got %d)", c.Commit.MaxLength)
	}
	if c.Commit.MaxLength < 20 {
		return fmt.Errorf("max_length is too small (got %d, minimum 20)", c.Commit.MaxLength)
	}
	if c.Commit.MaxLength > 200 {
		return fmt.Errorf("max_length is too large (got %d, maximum 200)", c.Commit.MaxLength)
	}
	return nil
}

func (c *Config) validateOllamaConfig() error {
	if c.Provider.Ollama == nil {
		return fmt.Errorf("ollama config section is required when provider type is 'ollama'")
	}

	cfg := c.Provider.Ollama
	if cfg.Host == "" {
		return fmt.Errorf("ollama host is required")
	}

	// Validate host URL format
	if !strings.HasPrefix(cfg.Host, "http://") && !strings.HasPrefix(cfg.Host, "https://") {
		return fmt.Errorf("ollama host must be a valid URL starting with http:// or https:// (got %q)", cfg.Host)
	}

	if cfg.Model == "" {
		return fmt.Errorf("ollama model is required")
	}

	return nil
}

func (c *Config) validateOpenAIConfig() error {
	if c.Provider.OpenAI == nil {
		return fmt.Errorf("openai config section is required when provider type is 'openai'")
	}

	cfg := c.Provider.OpenAI
	if cfg.BaseURL == "" {
		return fmt.Errorf("openai base_url is required")
	}

	// Validate base URL format
	if !strings.HasPrefix(cfg.BaseURL, "http://") && !strings.HasPrefix(cfg.BaseURL, "https://") {
		return fmt.Errorf("openai base_url must be a valid URL starting with http:// or https:// (got %q)", cfg.BaseURL)
	}

	if cfg.APIKey == "" {
		return fmt.Errorf("openai api_key is required")
	}

	// Basic API key format validation
	if len(cfg.APIKey) < 10 {
		return fmt.Errorf("openai api_key appears to be too short (got %d characters)", len(cfg.APIKey))
	}

	if cfg.Model == "" {
		return fmt.Errorf("openai model is required")
	}

	return nil
}
