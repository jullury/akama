package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Config struct {
	AnthropicAPIKey string
	OpenAIAPIKey    string
}

func Run(agent, model, workspacePath, promptPath string, cfg *Config) (string, error) {
	var cmd *exec.Cmd

	switch agent {
	case "claude":
		args := []string{"-p", promptPath, "--dangerously-skip-permissions", "--output-format", "json"}
		cmd = exec.Command("claude", args...)
	case "opencode":
		args := []string{"run", promptPath, "--dangerously-skip-permissions", "--format", "json"}
		if model != "" {
			args = append(args, "-m", model)
		}
		cmd = exec.Command("opencode", args...)
	default:
		return "", fmt.Errorf("unknown agent: %s", agent)
	}

	cmd.Dir = workspacePath
	cmd.Env = os.Environ()
	if cfg.AnthropicAPIKey != "" {
		cmd.Env = append(cmd.Env, "ANTHROPIC_API_KEY="+cfg.AnthropicAPIKey)
	}
	if cfg.OpenAIAPIKey != "" {
		cmd.Env = append(cmd.Env, "OPENAI_API_KEY="+cfg.OpenAIAPIKey)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("agent %s failed: %w\nstderr: %s", agent, err, stderr.String())
	}

	return stdout.String() + stderr.String(), nil
}

func BuildPrompt(title, url, body string) string {
	truncated := body
	if len(body) > 8000 {
		truncated = body[:8000]
	}
	return fmt.Sprintf(`You are a developer fixing an issue in this repository.

Issue Title: %s
Issue URL:   %s
Description:
%s

Implement a complete fix. Make all necessary code changes.
Do NOT create pull requests or push branches — that is handled separately.
Do NOT mention AI, bots, automation, or any tool in commit messages, code comments, or PR descriptions.
Write as a human developer would.

At the very end of your response, on its own line, write exactly:
COMMIT_MESSAGE: <conventional commit message describing the change>
`, title, url, truncated)
}

func BuildFollowUpPrompt(userText string) string {
	return fmt.Sprintf(`You are a developer continuing work on this repository.
Additional instructions:

%s

Apply these changes to the existing code.
Do NOT open pull requests — only make code changes.
Do NOT mention AI, bots, automation, or any tool in commit messages or code comments.

At the very end of your response, on its own line, write exactly:
COMMIT_MESSAGE: <conventional commit message describing the change>
`, userText)
}

// BuildPRDescription extracts the human-readable description from the agent output,
// stripping the COMMIT_MESSAGE line so it reads as a natural PR body.
func BuildPRDescription(agentText, issueURL string) string {
	var lines []string
	for _, line := range strings.Split(agentText, "\n") {
		if strings.HasPrefix(line, "COMMIT_MESSAGE:") {
			continue
		}
		lines = append(lines, line)
	}
	desc := strings.TrimSpace(strings.Join(lines, "\n"))
	if desc == "" {
		desc = fmt.Sprintf("Fixes %s", issueURL)
	} else {
		desc += fmt.Sprintf("\n\nFixes %s", issueURL)
	}
	return desc
}

// BuildCommitMessage extracts the COMMIT_MESSAGE line the agent was asked to produce.
// Falls back to the first non-empty, non-markdown line, then to a generic message.
func BuildCommitMessage(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "COMMIT_MESSAGE:") {
			msg := strings.TrimSpace(strings.TrimPrefix(line, "COMMIT_MESSAGE:"))
			if msg != "" {
				if len(msg) > 72 {
					msg = msg[:69] + "..."
				}
				return msg
			}
		}
	}
	// Fallback: first non-empty line stripped of markdown decorators
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(line, "#*- "))
		if line != "" {
			if len(line) > 72 {
				line = line[:69] + "..."
			}
			return line
		}
	}
	return "fix: apply agent changes"
}

// ParseOutput extracts the human-readable text from a claude/opencode JSON result.
// Falls back to the raw string if it is not valid JSON.
func ParseOutput(output string) string {
	var envelope struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &envelope); err == nil && envelope.Result != "" {
		return strings.TrimSpace(envelope.Result)
	}
	return strings.TrimSpace(output)
}

// IsQuestion returns true when the agent's last non-empty line ends with "?",
// indicating it needs user input before proceeding.
func IsQuestion(text string) bool {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return strings.HasSuffix(line, "?")
		}
	}
	return false
}

func WritePrompt(workspacePath, content string) (string, error) {
	promptPath := filepath.Join(workspacePath, ".akama-prompt.txt")
	if err := os.WriteFile(promptPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write prompt: %w", err)
	}
	return promptPath, nil
}
