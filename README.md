# git-ac

AI-powered commit message generator for Git repositories.

## Overview

`git-ac` analyzes your staged Git changes and generates conventional commit messages using AI. It supports multiple LLM providers including Ollama (local), OpenAI, Anthropic, and other OpenAI-compatible services.

## Features

- **Multiple AI providers**: Ollama (local/private), OpenAI, Anthropic, or any OpenAI-compatible API
- **Conventional commits**: Follows conventional commit format (type: description)
- **Context-aware**: Includes README.md content for better project understanding
- **Interactive editing**: Edit generated messages before committing with `-e`
- **Automatic staging**: Stage all changes before generating with `-a`
- **Smart diff handling**: Two-stage processing for large changesets

## Installation

### Prerequisites

- Go 1.19 or later
- Git
- One of: Ollama (local), OpenAI API key, or other compatible API

### Install git-ac

```bash
go install github.com/cdzombak/git-ac@latest
```

### Provider Setup

**For Ollama (local/private):**
```bash
# Install Ollama from ollama.ai, then:
ollama pull llama2
```

**For OpenAI/Anthropic:**
Get an API key from your provider.

## Configuration

Create `~/.config/git-ac.yaml`:

### Ollama (Local)
```yaml
provider:
  type: "ollama"
  timeout: 60s
  ollama:
    host: "http://localhost:11434"
    model: "llama2"

commit:
  max_length: 72
```

### OpenAI
```yaml
provider:
  type: "openai"
  timeout: 30s
  openai:
    base_url: "https://api.openai.com/v1"
    api_key: "sk-your-key-here"
    model: "gpt-4"

commit:
  max_length: 72
```

### Anthropic Claude
```yaml
provider:
  type: "openai"
  timeout: 30s
  openai:
    base_url: "https://api.anthropic.com/v1"
    api_key: "your-anthropic-key"
    model: "claude-3-sonnet-20240229"

commit:
  max_length: 72
```

### Local AI Server (LM Studio, vLLM, etc.)
```yaml
provider:
  type: "openai"
  timeout: 60s
  openai:
    base_url: "http://localhost:1234/v1"
    api_key: "not-needed"
    model: "local-model"

commit:
  max_length: 72
```

## Usage

```bash
# Generate and commit
git add .
git-ac

# Stage all changes and generate
git-ac -a

# Edit message before committing
git-ac -e

# Combine flags
git-ac -a -e
```

### Options

- `-a`: Stage modified files (like `git commit -a`)
- `-e`: Edit message in `$EDITOR` before committing
- `-h`: Show help

## Examples

Generated commit messages follow conventional commit format:

```
feat: add JWT token validation
fix: handle empty input strings
refactor: simplify YAML loading
docs: update installation guide
chore: update dependencies
```

## License

MIT License
