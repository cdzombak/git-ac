package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Ollama OllamaConfig `yaml:"ollama"`
	Commit CommitConfig `yaml:"commit"`
}

type OllamaConfig struct {
	Host    string        `yaml:"host"`
	Model   string        `yaml:"model"`
	Timeout time.Duration `yaml:"timeout"`
}

type CommitConfig struct {
	MaxLength       int  `yaml:"max_length"`
	IncludeBody     bool `yaml:"include_body"`
	LargeDiffThreshold int `yaml:"large_diff_threshold"`
}

func Load() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".config", "git-ac.yaml")

	// Start with defaults
	cfg := &Config{
		Ollama: OllamaConfig{
			Host:    "http://localhost:11434",
			Model:   "llama2",
			Timeout: 30 * time.Second,
		},
		Commit: CommitConfig{
			MaxLength:          72,
			IncludeBody:        true,
			LargeDiffThreshold: 2000,
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

	return cfg, nil
}