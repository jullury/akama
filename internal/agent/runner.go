package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Config struct {
	APIKeys    map[string]string
	TimeoutMins int
}

// AgentRunner defines the interface for executing a provider command.
type AgentRunner interface {
	Name() string
	Run(ctx context.Context, model, workspacePath, promptPath string, cfg *Config) (string, error)
	FetchModels() []string
	ParseOutput(output string) string
}

var (
	registry   = make(map[string]AgentRunner)
	registryMu sync.RWMutex
)

// Register adds a provider implementation to the registry.
func Register(r AgentRunner) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[r.Name()] = r
}

// Get retrieves a provider by name; returns nil if not found.
func Get(name string) AgentRunner {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[name]
}

// List returns the names of all registered providers.
func List() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	return names
}

func Run(ctx context.Context, agentName, model, workspacePath, promptPath string, cfg *Config) (string, error) {
	r := Get(agentName)
	if r == nil {
		return "", fmt.Errorf("unknown agent: %s", agentName)
	}
	if cfg.TimeoutMins > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(cfg.TimeoutMins)*time.Minute)
		defer cancel()
	}
	return r.Run(ctx, model, workspacePath, promptPath, cfg)
}

// claudeRunner implements AgentRunner for the claude CLI.
type claudeRunner struct{}

func (r *claudeRunner) Name() string { return "claude" }

func (r *claudeRunner) Run(ctx context.Context, model, workspacePath, promptPath string, cfg *Config) (string, error) {
	args := []string{"-p", promptPath, "--dangerously-skip-permissions", "--output-format", "json"}
	cmd := exec.CommandContext(ctx, "claude", args...)

	cmd.Dir = workspacePath
	cmd.Env = os.Environ()
	if key, ok := cfg.APIKeys["anthropic"]; ok && key != "" {
		cmd.Env = append(cmd.Env, "ANTHROPIC_API_KEY="+key)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("agent claude cancelled: %w", ctx.Err())
		}
		return "", fmt.Errorf("agent claude failed: %w\nstderr: %s", err, stderr.String())
	}
	return stdout.String() + stderr.String(), nil
}

func (r *claudeRunner) FetchModels() []string {
	cmd := exec.Command("claude", "-p", "/model", "--output-format", "text")
	out, err := cmd.Output()
	if err == nil {
		if models := parseModelLines(string(out)); len(models) > 0 {
			return models
		}
	}
	return []string{"claude-haiku-4-5-20251001", "claude-sonnet-4-6", "claude-opus-4-7"}
}

func (r *claudeRunner) ParseOutput(output string) string {
	output = strings.TrimSpace(output)
	var envelope struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal([]byte(output), &envelope); err == nil && envelope.Result != "" {
		return strings.TrimSpace(envelope.Result)
	}
	return output
}

// opencodeRunner implements AgentRunner for the opencode CLI.
type opencodeRunner struct{}

func (r *opencodeRunner) Name() string { return "opencode" }

func (r *opencodeRunner) Run(ctx context.Context, model, workspacePath, promptPath string, cfg *Config) (string, error) {
	promptContent, readErr := os.ReadFile(promptPath)
	if readErr != nil {
		return "", fmt.Errorf("read prompt: %w", readErr)
	}

	args := []string{"run", string(promptContent), "--dir", workspacePath, "--dangerously-skip-permissions", "--format", "json"}
	if model != "" {
		args = append(args, "-m", model)
	}
	cmd := exec.CommandContext(ctx, "opencode", args...)

	cmd.Dir = workspacePath
	cmd.Env = os.Environ()
	if key, ok := cfg.APIKeys["anthropic"]; ok && key != "" {
		cmd.Env = append(cmd.Env, "ANTHROPIC_API_KEY="+key)
	}
	if key, ok := cfg.APIKeys["openai"]; ok && key != "" {
		cmd.Env = append(cmd.Env, "OPENAI_API_KEY="+key)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("agent opencode cancelled: %w", ctx.Err())
		}
		return "", fmt.Errorf("agent opencode failed: %w\nstderr: %s", err, stderr.String())
	}

	out := stdout.String() + stderr.String()
	if err := extractOpencodeError(out); err != nil {
		return "", err
	}
	return out, nil
}

func (r *opencodeRunner) FetchModels() []string {
	cmd := exec.Command("opencode", "models")
	out, err := cmd.Output()
	if err == nil {
		if models := parseModelLines(string(out)); len(models) > 0 {
			return models
		}
	}
	return []string{
		"anthropic/claude-sonnet-4-6",
		"anthropic/claude-haiku-4-5",
		"openai/gpt-4o",
		"openai/gpt-4o-mini",
	}
}

func (r *opencodeRunner) ParseOutput(output string) string {
	return parseOpencodeOutput(output)
}

// extractOpencodeError scans opencode's NDJSON output for error events.
func extractOpencodeError(output string) error {
	var msgs []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || len(line) == 0 || line[0] != '{' {
			continue
		}
		var evt struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			continue
		}
		if evt.Type == "api_error" || evt.Type == "error" {
			msg := evt.Message
			if msg == "" {
				msg = evt.Error
			}
			if msg == "" {
				msg = "unknown error"
			}
			msgs = append(msgs, msg)
		}
	}
	if len(msgs) > 0 {
		return fmt.Errorf("opencode: %s", strings.Join(msgs, "; "))
	}
	return nil
}

// parseOpencodeOutput extracts human-readable text from opencode's NDJSON event stream.
// It formats text messages and tool call results like a terminal session.
func parseOpencodeOutput(output string) string {
	output = strings.TrimSpace(output)
	var parts []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || len(line) == 0 || line[0] != '{' {
			continue
		}

		// Try to parse as a generic JSON event
		evt := map[string]json.RawMessage{}
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			continue
		}

		// Get event type
		var evtType string
		if t, ok := evt["type"]; ok {
			json.Unmarshal(t, &evtType)
		}

		// Handle text events
		if evtType == "text" {
			// Try part.text format
			if part, ok := evt["part"]; ok {
				var p struct {
					Text string `json:"text"`
				}
				if json.Unmarshal(part, &p) == nil && p.Text != "" {
					parts = append(parts, p.Text)
					continue
				}
			}
			// Try flat text format
			var flat struct {
				Text string `json:"text"`
			}
			if json.Unmarshal([]byte(line), &flat) == nil && flat.Text != "" {
				parts = append(parts, flat.Text)
			}
			continue
		}

		// Handle tool_use events - show tool name and output
		if evtType == "tool_use" {
			var tu struct {
				Part struct {
					Type   string `json:"type"`
					Tool   string `json:"tool"`
					State  struct {
						Status string `json:"status"`
						Input  json.RawMessage `json:"input"`
						Output string `json:"output"`
					} `json:"state"`
				} `json:"part"`
			}
			if json.Unmarshal([]byte(line), &tu) == nil && tu.Part.Tool != "" {
				// Show what tool was used
				output := strings.TrimSpace(tu.Part.State.Output)
				if output != "" {
					// Truncate very long output
					if len(output) > 500 {
						output = output[:500] + "..."
					}
					parts = append(parts, output)
				}
			}
			continue
		}

		// Skip step_start, step_finish, and other metadata events
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
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
Do NOT mention AI, bots, automation, or any tool in code comments.
Write as a human developer would.
`, title, url, truncated)
}

func BuildFollowUpPrompt(userText string) string {
	return fmt.Sprintf(`You are a developer continuing work on this repository.
Additional instructions:

%s

Apply these changes to the existing code.
Do NOT open pull requests — only make code changes.
Do NOT mention AI, bots, automation, or any tool in code comments.
`, userText)
}

// GenerateSummary runs a focused agent call on the current git diff to produce
// a conventional commit message and a PR description.
// Falls back to generic strings if the agent call fails.
func GenerateSummary(ctx context.Context, agentName, model, workspacePath, issueURL string, cfg *Config) (commitMsg, prDesc string) {
	diff := gitDiff(workspacePath)
	if len(diff) > 8000 {
		diff = diff[:8000] + "\n... (truncated)"
	}
	if diff == "" {
		return "fix: apply changes", fmt.Sprintf("Fixes %s", issueURL)
	}

	prompt := fmt.Sprintf(`You are reviewing the following git diff.

%s

Based solely on these changes, output EXACTLY two lines and nothing else:
COMMIT_MESSAGE: <conventional commit message, max 72 chars>
PR_DESCRIPTION: <2-3 sentences describing what changed and why, no mention of AI or bots>`, diff)

	promptPath, err := WritePrompt(workspacePath, prompt)
	if err != nil {
		return "fix: apply changes", fmt.Sprintf("Fixes %s", issueURL)
	}
	defer os.Remove(promptPath)

	output, err := Run(ctx, agentName, model, workspacePath, promptPath, cfg)
	if err != nil {
		return "fix: apply changes", fmt.Sprintf("Fixes %s", issueURL)
	}

	text := ParseOutput(agentName, output)
	for _, line := range strings.Split(text, "\n") {
		if msg, ok := strings.CutPrefix(line, "COMMIT_MESSAGE:"); ok {
			commitMsg = truncate(strings.TrimSpace(msg), 72)
		}
		if desc, ok := strings.CutPrefix(line, "PR_DESCRIPTION:"); ok {
			prDesc = strings.TrimSpace(desc)
		}
	}
	// Also search raw output in case content is JSON-encoded
	if commitMsg == "" {
		if idx := strings.Index(text, "COMMIT_MESSAGE:"); idx != -1 {
			rest := text[idx+len("COMMIT_MESSAGE:"):]
			end := strings.IndexAny(rest, "\n\\\"")
			if end == -1 {
				end = len(rest)
			}
			commitMsg = truncate(strings.TrimSpace(rest[:end]), 72)
		}
	}
	if commitMsg == "" {
		commitMsg = "fix: apply changes"
	}
	if prDesc == "" {
		prDesc = fmt.Sprintf("Fixes %s", issueURL)
	} else {
		prDesc += fmt.Sprintf("\n\nFixes %s", issueURL)
	}
	return commitMsg, prDesc
}

// BranchFromCommit converts a conventional commit message to a git branch name.
// "feat: implement OWASP 2025 top 10" → "feat/implement-owasp-2025-top-10"
// Falls back to fallback if the message can't be parsed.
func BranchFromCommit(commitMsg, fallback string) string {
	commitMsg = strings.TrimSpace(commitMsg)
	// Split on first colon to get type and description
	colon := strings.IndexByte(commitMsg, ':')
	if colon < 1 {
		return fallback
	}
	rawType := strings.TrimSpace(commitMsg[:colon])
	desc := strings.TrimSpace(commitMsg[colon+1:])
	if desc == "" {
		return fallback
	}
	// Strip scope from type: "feat(auth)" → "feat"
	if idx := strings.IndexByte(rawType, '('); idx != -1 {
		rawType = rawType[:idx]
	}
	slug := slugify(desc)
	if slug == "" {
		return fallback
	}
	branch := rawType + "/" + slug
	if len(branch) > 60 {
		branch = branch[:60]
		branch = strings.TrimRight(branch, "-")
	}
	return branch
}

func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prevHyphen := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevHyphen = false
		} else if !prevHyphen && b.Len() > 0 {
			b.WriteByte('-')
			prevHyphen = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// FetchModels returns the available models for the given agent.
func FetchModels(agentName string) []string {
	r := Get(agentName)
	if r == nil {
		return nil
	}
	return r.FetchModels()
}

func parseModelLines(output string) []string {
	var models []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.ContainsAny(line, " \t") {
			models = append(models, line)
		}
	}
	return models
}

func gitDiff(workspacePath string) string {
	// Try staged changes first, fall back to unstaged, then stat only
	for _, args := range [][]string{
		{"diff", "--cached"},
		{"diff", "HEAD"},
		{"diff"},
		{"diff", "--stat", "HEAD"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = workspacePath
		out, err := cmd.Output()
		if err == nil && len(out) > 0 {
			return string(out)
		}
	}
	return ""
}

// BuildCommitMessage extracts the COMMIT_MESSAGE line the agent was asked to produce.
// Searches both decoded text (newlines) and raw output (JSON-encoded \n sequences).
// Falls back to the first non-JSON, non-markdown line, then to a generic message.
func BuildCommitMessage(agentName, text string) string {
	// Pass 1: clean decoded text — look for COMMIT_MESSAGE: at line start
	for _, line := range strings.Split(text, "\n") {
		if msg, ok := strings.CutPrefix(line, "COMMIT_MESSAGE:"); ok {
			msg = strings.TrimSpace(msg)
			if msg != "" {
				return truncate(msg, 72)
			}
		}
	}
	// Pass 2: raw output — COMMIT_MESSAGE: may be inside a JSON string where
	// newlines are encoded as \n (two chars), so search the raw text as well.
	if idx := strings.Index(text, "COMMIT_MESSAGE:"); idx != -1 {
		rest := text[idx+len("COMMIT_MESSAGE:"):]
		// Read until a JSON string boundary or actual newline
		end := strings.IndexAny(rest, "\n\\\"")
		if end == -1 {
			end = len(rest)
		}
		if msg := strings.TrimSpace(rest[:end]); msg != "" {
			return truncate(msg, 72)
		}
	}
	// Fallback: first non-JSON, non-markdown line
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "{") {
			continue
		}
		line = strings.TrimSpace(strings.TrimLeft(line, "#*- "))
		if line != "" {
			return truncate(line, 72)
		}
	}
	return "fix: apply changes"
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// ParseOutput extracts the human-readable text from agent output.
// Delegates to the specific agent's parser.
func ParseOutput(agentName, output string) string {
	r := Get(agentName)
	if r != nil {
		return r.ParseOutput(output)
	}
	return output
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

// init registers the built-in agent providers.
func init() {
	Register(&claudeRunner{})
	Register(&opencodeRunner{})
}
