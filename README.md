# git-ac

AI-powered commit message generator using Ollama.

## Overview

`git-ac` is a command-line tool that automatically generates commit messages for your staged Git changes using a local Ollama server. It analyzes your git diff and optionally includes your project's README.md for context to create meaningful, conventional commit messages.

## Features

- 🤖 AI-powered commit message generation using Ollama
- 📝 Conventional commit format
- ✏️  Optional manual editing with your preferred editor
- 🔧 Configurable via YAML file
- 📖 Uses README.md for project context
- 🚀 Works entirely locally with your Ollama installation

## Prerequisites

- [Ollama](https://ollama.com/) installed and running
- A language model pulled in Ollama (e.g., `ollama pull llama2`)
- Go 1.21+ (for building from source)

## Installation

### From Source

```bash
git clone https://github.com/yourusername/git-ac
cd git-ac
go build -o git-ac
sudo mv git-ac /usr/local/bin/
```

### Using Go Install

```bash
go install github.com/yourusername/git-ac@latest
```

## Configuration

Create a configuration file at `~/.config/git-ac.yaml`:

```yaml
ollama:
  host: "http://localhost:11434"
  model: "llama2"
  timeout: 30s

commit:
  max_length: 72
  include_body: true
```

### Configuration Options

#### Ollama Section
- `host`: Ollama server URL (default: "http://localhost:11434")
- `model`: Model to use for generation (default: "llama2")
- `timeout`: Request timeout (default: 30s)

#### Commit Section
- `max_length`: Maximum commit message length (default: 72)
- `include_body`: Include commit body in longer messages (default: true)

## Usage

### Basic Usage

Stage your changes and run:

```bash
git add .
git-ac
```

This will:
1. Analyze your staged changes
2. Generate a commit message using Ollama
3. Automatically commit with the generated message

### Edit Before Committing

To review and edit the generated message:

```bash
git-ac -e
```

This opens your `$EDITOR` with the proposed commit message, allowing you to modify it before committing.

### Help

```bash
git-ac -h
```

## Examples

### Example 1: Adding a new feature

```bash
$ git add src/auth.go
$ git-ac
Successfully committed with message:
feat(auth): add JWT token validation middleware
```

### Example 2: Bug fix with manual editing

```bash
$ git add src/parser.go
$ git-ac -e
# Opens editor with: "fix(parser): handle empty input strings"
# You can edit the message
Successfully committed with message:
fix(parser): handle empty input strings to prevent panic
```

## How It Works

1. **Repository Validation**: Ensures you're in a Git repository
2. **Staged Changes**: Retrieves `git diff --cached` output
3. **Context Gathering**: Reads README.md (if present) for project context
4. **AI Generation**: Sends diff and context to Ollama for commit message generation
5. **Optional Editing**: Opens editor if `-e` flag is used
6. **Commit**: Executes `git commit` with the final message

## Prompt Engineering

The tool sends a carefully crafted prompt to Ollama that includes:

- Instructions for conventional commit format
- Your project's README.md for context
- The actual git diff of staged changes
- Requirements for message length and style

## Troubleshooting

### "No staged changes found"
Make sure you have staged changes:
```bash
git add <files>
git-ac
```

### "Failed to connect to Ollama"
Ensure Ollama is running:
```bash
ollama serve
```

### "Model not found"
Pull the required model:
```bash
ollama pull llama2
```

### "Request timed out"
This can happen with large models. Try:

1. **Use a faster model**: Smaller models work better for commit messages:
   ```bash
   ollama pull llama2:7b-chat    # Faster than larger models
   ollama pull mistral:7b        # Good for code tasks
   ollama pull codellama:7b      # Optimized for code
   ```

2. **Increase timeout in config**:
   ```yaml
   ollama:
     timeout: 60s  # Increase from default 30s
   ```

3. **Check model size**: Very large models (>10GB) may be too slow:
   ```bash
   ollama list  # Check model sizes
   ```

### "No editor found"
Set your preferred editor:
```bash
export EDITOR=nano
# or add to your shell profile
echo 'export EDITOR=nano' >> ~/.bashrc
```

## Conventional Commit Types

The generated messages follow the [Conventional Commits](https://www.conventionalcommits.org/) specification:

- `feat`: New features
- `fix`: Bug fixes
- `docs`: Documentation changes
- `style`: Code style changes
- `refactor`: Code refactoring
- `perf`: Performance improvements
- `test`: Adding or fixing tests
- `chore`: Maintenance tasks

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## License

MIT License - see LICENSE file for details.

## Related Projects

- [Ollama](https://ollama.com/) - Local language model server
- [Conventional Commits](https://www.conventionalcommits.org/) - Commit message convention