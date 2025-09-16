package llm

import (
	"fmt"
	"strings"

	"git-ac/internal/config"
)

// IsDiffTooLarge determines if a diff is too large for direct processing
func IsDiffTooLarge(diff string) bool {
	// Count words in the diff (split by whitespace)
	words := strings.Fields(diff)
	wordCount := len(words)

	// Context window is 4096 tokens, use half as threshold
	// Rough approximation: 1 word ≈ 1.3 tokens
	maxWords := (4096 / 2) / 1.3 // ~1575 words

	return wordCount > int(maxWords)
}

// BuildSummarizePrompt creates the prompt for file change summarization
func BuildSummarizePrompt(diff string) string {
	return fmt.Sprintf(`Summarize the changes in the following diff in several sentences. Pay attention to detail. The result should be a summary that is meaningful to a human knowledgeable about the codebase.

DIFF:
%s

OUTPUT:`, diff)
}

// BuildCommitPrompt creates the commit message generation prompt
func BuildCommitPrompt(content, readme string, isFileSummary bool, commitConfig config.CommitConfig) string {
	var prompt strings.Builder

	prompt.WriteString("You are a Git commit message generator. " +
		"Analyze the following changes and output ONLY a conventional commit message. Your commit message must summarize the most important and significant changes present. " +
		"Be as specific as possible within the given constraints; saying 'change maximum character limit to 72' is better than 'update commit message rules'. " +
		"You may optionally include an extended description of the changes, ONLY if the changes are large or complex. Focus on the changes themselves; do not explain why you chose the type you did.\n\n")

	prompt.WriteString("REQUIRED FORMAT:\ntype(scope): summary line\n\noptional description\n\n")

	prompt.WriteString("VALID TYPES:\n")
	prompt.WriteString("feat - new or improved feature work\n")
	prompt.WriteString("fix - fixing bugs or shortcomings\n")
	prompt.WriteString("refactor - internal refactoring that improves quality, is not user-facing, and does not affect program behavior\n")
	prompt.WriteString("docs - documentation\n")
	prompt.WriteString("style - formatting\n")
	prompt.WriteString("test - testing\n")
	prompt.WriteString("chore - maintenance that is not feature-related or user-facing\n\n")

	prompt.WriteString("GOOD FIRST-LINE EXAMPLES:\n")
	prompt.WriteString("feat(auth): add JWT token validation\n")
	prompt.WriteString("fix(parser): handle empty input strings\n")
	prompt.WriteString("refactor(config): simplify YAML loading\n")
	prompt.WriteString("docs: update installation guide\n\n")

	prompt.WriteString("REQUIREMENTS:\n")
	prompt.WriteString(fmt.Sprintf("- First line of the commit message MUST be concise and under %d characters\n", commitConfig.MaxLength))
	prompt.WriteString("- Present tense (add, not added)\n")
	prompt.WriteString("- No explanations, reasoning, or headings\n")
	prompt.WriteString("- Output ONLY the commit message\n")
	prompt.WriteString("- Focus on the most important changes present rather than inconsequential details. Be extremely concise.\n")
	prompt.WriteString("- Start immediately with 'type(scope):'\n")
	prompt.WriteString("- SCOPE is not a file path/name, but one or two words summarizing the area of code that was changed. If multiple areas are changed, exclude the scope. Scope should be meaningful to a human knowledgeable about the codebase.\n\n")
	prompt.WriteString("GOOD SCOPE EXAMPLES: auth, parser, config, tests, api client\n")
	prompt.WriteString("BAD SCOPE EXAMPLES: internal, pkg, deps\n")
	prompt.WriteString("- If you include an extended description, it must be specific and concise. Do not include excess verbiage like 'note:' or 'these changes relate to...'. Do not prefix it with 'extended description'.\n")
	prompt.WriteString("- If you do not include an extended description, no additional output is required. DO NOT write 'No extended description'. Your output should only include words that are meaningful to describe the diff itself.\n\n")

	if readme != "" {
		prompt.WriteString("PROJECT README:\n")
		// Limit README content to avoid token limits
		readmeLines := strings.Split(readme, "\n")
		if len(readmeLines) > 20 {
			readmeLines = readmeLines[:20]
			readme = strings.Join(readmeLines, "\n") + "\n... (truncated)"
		}
		prompt.WriteString(readme)
		prompt.WriteString("\n\n")
	}

	if isFileSummary {
		prompt.WriteString("FILE CHANGES SUMMARIZED:\n")
	} else {
		prompt.WriteString("STAGED DIFF:\n")
	}
	prompt.WriteString(content)

	return prompt.String()
}

// CleanCommitMessage removes thinking tags and handles message formatting
func CleanCommitMessage(message string, commitConfig config.CommitConfig) string {
	cleaned := strings.TrimSpace(message)

	// For thinking models, look for the actual answer after </think>
	if strings.Contains(cleaned, "</think>") {
		parts := strings.Split(cleaned, "</think>")
		if len(parts) > 1 {
			// Take everything after the last </think>
			cleaned = strings.TrimSpace(parts[len(parts)-1])
		}
	}

	// Remove thinking patterns
	for strings.Contains(cleaned, "<think>") && strings.Contains(cleaned, "</think>") {
		start := strings.Index(cleaned, "<think>")
		end := strings.Index(cleaned, "</think>") + len("</think>")
		if start >= 0 && end > start {
			cleaned = cleaned[:start] + cleaned[end:]
		} else {
			break
		}
	}

	// Remove remaining thinking tags
	cleaned = strings.ReplaceAll(cleaned, "<think>", "")
	cleaned = strings.ReplaceAll(cleaned, "</think>", "")
	cleaned = strings.TrimSpace(cleaned)

	// Handle multi-line commits based on config
	lines := strings.Split(cleaned, "\n")
	if len(lines) > 0 {
		// Handle first line length - split with ellipsis if too long, never truncate
		subject := strings.TrimSpace(lines[0])
		if commitConfig.MaxLength > 0 && len(subject) > commitConfig.MaxLength {
			// Find a good break point
			maxLen := commitConfig.MaxLength - 1 // Reserve space for "…"
			if spaceIdx := strings.LastIndex(subject[:maxLen], " "); spaceIdx > 0 {
				// Split at word boundary
				lines[0] = subject[:spaceIdx] + "…"
				// Add remainder as new line
				remainder := strings.TrimSpace(subject[spaceIdx:])
				if remainder != "" {
					lines = append([]string{lines[0], "…" + remainder}, lines[1:]...)
				}
			} else {
				// No good break point, split at character boundary
				lines[0] = subject[:maxLen] + "…"
				remainder := subject[maxLen:]
				if remainder != "" {
					lines = append([]string{lines[0], "…" + remainder}, lines[1:]...)
				}
			}
		}

		// Always allow multi-line commits - let the LLM decide
		cleaned = strings.Join(lines, "\n")
	}

	return cleaned
}