package agent

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	return fmt.Sprintf(`You are fixing an issue in the current repository.

Issue Title: %s
Issue URL:   %s
Description:
%s

Implement a complete fix. Make all necessary code changes.
Do NOT create pull requests or push branches — that is handled separately.
`, title, url, truncated)
}

func BuildFollowUpPrompt(userText string) string {
	return fmt.Sprintf(`You are continuing work on the same repository.
Additional instructions from the user:

%s

Apply these changes to the existing code. Commit all changes.
Do NOT open pull requests — only make and commit code changes.
`, userText)
}

func WritePrompt(workspacePath, content string) (string, error) {
	promptPath := filepath.Join(workspacePath, ".akama-prompt.txt")
	if err := os.WriteFile(promptPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write prompt: %w", err)
	}
	return promptPath, nil
}
