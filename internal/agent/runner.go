package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Config struct {
	APIKeys     map[string]string
	TimeoutMins int
	// WorkspaceBaseDir is the directory under which temporary plan workspaces
	// are created inside the daemon container.
	WorkspaceBaseDir string
}

// AgentRunner defines the interface for executing a provider command.
type AgentRunner interface {
	Name() string
	Run(ctx context.Context, model, workspacePath, promptPath string, cfg *Config) (string, error)
	FetchModels(apiKeys map[string]string) []string
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
	return r.Run(ctx, model, workspacePath, promptPath, cfg)
}

// RunDetached is like Run but calls onProcess with the agent's OS process
// immediately after it starts. The callback can be used by the caller to
// monitor whether the process is still alive (e.g. from a heartbeat goroutine).
// Pass nil for onProcess if you don't need process monitoring.
func RunDetached(ctx context.Context, agentName, model, workspacePath, promptPath string, cfg *Config, onProcess func(*os.Process)) (string, error) {
	r := Get(agentName)
	if r == nil {
		return "", fmt.Errorf("unknown agent: %s", agentName)
	}
	if dr, ok := r.(detachedRunner); ok {
		return dr.RunDetached(ctx, model, workspacePath, promptPath, cfg, onProcess)
	}
	// Fallback for runners that don't support detached mode.
	return r.Run(ctx, model, workspacePath, promptPath, cfg)
}

// detachedRunner is implemented by agents that can expose their OS process.
type detachedRunner interface {
	RunDetached(ctx context.Context, model, workspacePath, promptPath string, cfg *Config, onProcess func(*os.Process)) (string, error)
}

// claudeRunner implements AgentRunner for the claude CLI.
type claudeRunner struct{}

func (r *claudeRunner) Name() string { return "claude" }

func (r *claudeRunner) Run(ctx context.Context, model, workspacePath, promptPath string, cfg *Config) (string, error) {
	return r.RunDetached(ctx, model, workspacePath, promptPath, cfg, nil)
}

func (r *claudeRunner) RunDetached(ctx context.Context, model, workspacePath, promptPath string, cfg *Config, onProcess func(*os.Process)) (string, error) {
	args := []string{"-p", promptPath, "--dangerously-skip-permissions", "--output-format", "json"}
	if model != "" {
		args = append(args, "--model", model)
	}
	cmd := agentCmd(ctx, "claude", args, workspacePath, cfg)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("agent claude start: %w", err)
	}
	if onProcess != nil {
		onProcess(cmd.Process)
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("agent claude cancelled: %w", ctx.Err())
		}
		return "", fmt.Errorf("agent claude failed: %w\nstderr: %s\nstdout: %s", err, stderr.String(), stdout.String())
	}
	return stdout.String() + stderr.String(), nil
}

func (r *claudeRunner) FetchModels(apiKeys map[string]string) []string {
	key := apiKeys["anthropic"]
	if key == "" {
		return []string{"claude-haiku-4-5-20251001", "claude-sonnet-4-6", "claude-opus-4-7"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.anthropic.com/v1/models", nil)
	if err != nil {
		return []string{"claude-haiku-4-5-20251001", "claude-sonnet-4-6", "claude-opus-4-7"}
	}
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return []string{"claude-haiku-4-5-20251001", "claude-sonnet-4-6", "claude-opus-4-7"}
	}
	defer resp.Body.Close()
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return []string{"claude-haiku-4-5-20251001", "claude-sonnet-4-6", "claude-opus-4-7"}
	}
	models := make([]string, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, m.ID)
	}
	if len(models) == 0 {
		return []string{"claude-haiku-4-5-20251001", "claude-sonnet-4-6", "claude-opus-4-7"}
	}
	return models
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
	return r.RunDetached(ctx, model, workspacePath, promptPath, cfg, nil)
}

func (r *opencodeRunner) RunDetached(ctx context.Context, model, workspacePath, promptPath string, cfg *Config, onProcess func(*os.Process)) (string, error) {
	promptContent, readErr := os.ReadFile(promptPath)
	if readErr != nil {
		return "", fmt.Errorf("read prompt: %w", readErr)
	}

	// Pass prompt via stdin to avoid ARG_MAX when content is large.
	args := []string{"run", "--dir", workspacePath, "--dangerously-skip-permissions", "--format", "json"}
	if model != "" {
		args = append(args, "-m", model)
	}
	cmd := agentCmd(ctx, "opencode", args, workspacePath, cfg)
	cmd.Stdin = bytes.NewReader(promptContent)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("agent opencode start: %w", err)
	}
	if onProcess != nil {
		onProcess(cmd.Process)
	}
	if err := cmd.Wait(); err != nil {
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

func (r *opencodeRunner) FetchModels(_ map[string]string) []string {
	cmd := exec.Command("opencode", "models")
	cmd.Env = []string{
		"HOME=/home/worker",
		"PATH=/home/worker/.npm-global/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
	}
	dropPrivileges(cmd)
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

func BuildPrompt(title, url, body, knowledgePath string) string {
	truncated := body
	if len(body) > 50000 {
		truncated = body[:50000]
	}

	prompt := fmt.Sprintf(`You are a developer fixing an issue in this repository.

Issue Title: %s
Issue URL:   %s
Description:
%s

## CRITICAL INSTRUCTIONS — READ BEFORE DOING ANYTHING

1. **EXECUTE IMMEDIATELY** — Do NOT ask questions. Do NOT request clarification. Do NOT output INPUT_REQUIRED. The issue description has everything you need.
2. **FOLLOW THE PLAN** — If an Implementation Plan is included below, follow it step-by-step. Do NOT deviate. Do NOT regenerate the plan. Do NOT create your own plan.
3. **NO PLANNING PHASE** — Do NOT write a plan. Do NOT outline steps. Do NOT say "here's what I'll do". Just DO it. Write the code.
4. **NO QUESTIONS ALLOWED** — Even if something is ambiguous, make a reasonable assumption and proceed. The user has already reviewed and approved this plan.
5. **COMMIT TO CODE** — Your first response should be code changes, not analysis. Start editing files immediately.

## What To Do

Implement a complete fix. Make all necessary code changes.
Do NOT create pull requests or push branches — that is handled separately.
Do NOT mention AI, bots, automation, or any tool in code comments.
Write as a human developer would.

## What NOT To Do

- ❌ Do NOT ask clarifying questions (use INPUT_REQUIRED only for truly blocking issues)
- ❌ Do NOT write implementation plans (the plan is already provided)
- ❌ Do NOT explain what you're going to do (just do it)
- ❌ Do NOT request confirmation before proceeding (you are already confirmed)
- ❌ Do NOT output meta-commentary about the task
`, title, url, truncated)

	if knowledgePath != "" {
		prompt += fmt.Sprintf(`
## Prior Art Analysis Required

Before implementing, you MUST read and analyze the knowledge base at: %s

This file contains similar issues that were previously resolved. Use it to:

1. **Identify Root Cause**: Understand WHY similar issues occurred, not just HOW they were fixed
2. **Recognize Patterns**: Look for recurring patterns across similar issues
3. **Apply Proven Solutions**: Use approaches that worked for similar problems
4. **Prevent Recurrence**: Consider what can prevent similar issues

Analyze the knowledge base thoroughly, then implement your fix while applying insights from prior art.
Reference specific patterns or approaches from the knowledge base in your implementation.
`, knowledgePath)
	}

	return prompt
}

func BuildFollowUpPrompt(userText string) string {
	return fmt.Sprintf(`You are a developer continuing work on this repository.
Additional instructions:

%s

## CRITICAL INSTRUCTIONS

1. **EXECUTE IMMEDIATELY** — Apply the changes requested. Do NOT ask questions. Do NOT request clarification.
2. **NO PLANNING** — Do NOT write a plan. Do NOT outline steps. Just make the code changes.
3. **NO QUESTIONS** — Even if something is ambiguous, make a reasonable assumption and proceed.
4. **COMMIT TO CODE** — Your first response should be code changes, not analysis.

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

	promptPath, err := WritePrompt(workspacePath, agentName, prompt)
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

// FetchModels returns the available models for the given agent by querying the
// provider's API directly using the supplied API keys.
func FetchModels(agentName string, apiKeys map[string]string) []string {
	r := Get(agentName)
	if r == nil {
		return nil
	}
	return r.FetchModels(apiKeys)
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
// ParseOutput extracts the human-readable text from agent output.
// Delegates to the specific agent's parser.
func ParseOutput(agentName, output string) string {
	r := Get(agentName)
	if r != nil {
		return r.ParseOutput(output)
	}
	return output
}

// BuildClarifyingQuestionsPrompt returns a prompt that asks the agent to generate
// 3-5 clarifying questions about the issue.
func BuildClarifyingQuestionsPrompt(title, body string, repoSources ...string) string {
	truncated := body
	if len(body) > 30000 {
		truncated = body[:30000]
	}
	multiRepoBlock := ""
	if len(repoSources) > 0 {
		multiRepoBlock = "\n\nThis issue spans multiple repositories. The workspace contains:\n" +
			strings.Join(repoSources, "\n") +
			"\n\nConsider cross-repo dependencies when generating questions.\n"
	}
	return fmt.Sprintf(`You are reviewing an issue that needs to be fixed.

Issue Title: %s
Description:
%s
%s
Generate 3-5 clarifying questions that would help you understand the issue better before creating an implementation plan.
Ask about requirements, expected behavior, edge cases, or anything unclear.
Output ONLY the questions, one per line, starting with "Q: ".`, title, truncated, multiRepoBlock)
}

// ParseClarifyingQuestions extracts questions from agent output.
func ParseClarifyingQuestions(output string) []string {
	var questions []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "q:") {
			q := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(line), "q:"))
			if q != "" {
				questions = append(questions, q)
			}
		}
	}
	if len(questions) == 0 {
		fallback := strings.Split(output, "\n")
		for _, line := range fallback {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "*") || (len(line) > 0 && line[0] >= '0' && line[0] <= '9') {
				q := strings.TrimSpace(strings.TrimLeft(line, "-* 1234567890."))
				if q != "" && strings.Contains(q, "?") {
					questions = append(questions, q)
				}
			}
		}
	}
	return questions
}

// BuildPlanFromAnswers creates a plan prompt using the issue details and user
// answers to clarifying questions.
func BuildPlanFromAnswers(title, body, answers string, repoSources ...string) string {
	truncated := body
	if len(body) > 30000 {
		truncated = body[:30000]
	}
	multiRepoBlock := ""
	if len(repoSources) > 0 {
		multiRepoBlock = "\n\nThis issue spans multiple repositories. The workspace contains:\n" +
			strings.Join(repoSources, "\n") +
			"\n\nCreate a detailed plan that covers ALL repositories.\n"
	}
	return fmt.Sprintf(`You are a developer planning how to fix an issue in a codebase.

Issue Title: %s
Issue Description:
%s

Additional context from the user:
%s
%s
Create a detailed, step-by-step implementation plan for fixing this issue.
Include:
1. Files that need to be modified
2. What changes each file needs
3. Any new files that need to be created
4. Potential risks or edge cases
5. Testing strategy

Be specific and actionable. Do NOT implement the changes — only describe what needs to be done.
Do NOT mention AI, bots, or automation tools.`, title, truncated, answers, multiRepoBlock)
}

func IsQuestion(text string) bool {
	if !strings.Contains(text, "INPUT_REQUIRED:") {
		return false
	}
	// Guard against false positives: the marker must have non-empty question text after it.
	q := ExtractQuestion(text)
	return strings.TrimSpace(q) != ""
}

func ExtractQuestion(text string) string {
	idx := strings.Index(text, "INPUT_REQUIRED:")
	if idx == -1 {
		return text
	}
	q := strings.TrimSpace(text[idx+len("INPUT_REQUIRED:"):])
	if nl := strings.Index(q, "\n"); nl != -1 {
		q = q[:nl]
	}
	return strings.TrimSpace(q)
}

// RunPlanAgent runs the agent to generate plan-related content.
// workspacePath must be provided — it is the cloned repository workspace.
func RunPlanAgent(ctx context.Context, agentName, model, workspacePath, promptContent string, cfg *Config) (string, error) {
	if workspacePath == "" {
		return "", fmt.Errorf("workspace path is required for plan agent")
	}

	promptPath, err := WritePrompt(workspacePath, agentName, promptContent)
	if err != nil {
		return "", fmt.Errorf("write plan prompt: %w", err)
	}
	defer os.Remove(promptPath)

	output, err := Run(ctx, agentName, model, workspacePath, promptPath, cfg)
	if err != nil {
		return "", err
	}

	return ParseOutput(agentName, output), nil
}

func WritePrompt(workspacePath, agentName, content string) (string, error) {
	promptPath := filepath.Join(workspacePath, ".akama-prompt.txt")
	var injected string
	if agentName == "opencode" {
		injected = OpencodeInjectedContent()
	} else {
		injected = InjectedSkillsContent()
	}
	full := injected + content
	instruction := "\n\n---\nIf you need to ask the user a question before proceeding, your response must end with exactly:\nINPUT_REQUIRED: <your question here>\nDo not use INPUT_REQUIRED for any other purpose."
	if err := os.WriteFile(promptPath, []byte(full+instruction), 0644); err != nil {
		return "", fmt.Errorf("write prompt: %w", err)
	}
	return promptPath, nil
}

// init registers the built-in agent providers.
func init() {
	Register(&claudeRunner{})
	Register(&opencodeRunner{})
}

// InstallClaudeCmd installs the claude agent using available package managers.
func InstallClaudeCmd() error {
	// brew --cask is macOS-only; skip it on Linux even if brew is installed.
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("brew"); err == nil {
			return exec.Command("brew", "install", "--cask", "claude-code").Run()
		}
	}
	if _, err := exec.LookPath("npm"); err == nil {
		return exec.Command("npm", "install", "-g", "@anthropic-ai/claude-code").Run()
	}
	if _, err := exec.LookPath("curl"); err == nil {
		tmpScript := filepath.Join(os.TempDir(), "claude-install.sh")
		if err := exec.Command("curl", "-fsSL", "https://claude.ai/install.sh", "-o", tmpScript).Run(); err != nil {
			return err
		}
		err := exec.Command("bash", tmpScript).Run()
		os.Remove(tmpScript)
		return err
	}
	return fmt.Errorf("no supported package manager found (brew, npm, or curl required)")
}

// InstallOpencodeCmd installs the opencode agent using available package managers.
func InstallOpencodeCmd() error {
	// brew tap formulas may not have Linux bottles; use npm/curl on Linux.
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("brew"); err == nil {
			return exec.Command("brew", "install", "anomalyco/tap/opencode").Run()
		}
	}
	if _, err := exec.LookPath("npm"); err == nil {
		return exec.Command("npm", "install", "-g", "--force", "opencode-ai@latest").Run()
	}
	if _, err := exec.LookPath("curl"); err == nil {
		tmpScript := filepath.Join(os.TempDir(), "opencode-install.sh")
		if err := exec.Command("curl", "-fsSL", "https://opencode.ai/install", "-o", tmpScript).Run(); err != nil {
			return err
		}
		err := exec.Command("bash", tmpScript).Run()
		os.Remove(tmpScript)
		return err
	}
	return fmt.Errorf("no supported package manager found (brew, npm, or curl required)")
}

// UpdateClaude updates the claude agent to the latest version.
// Installs it if not present.
func UpdateClaude() error {
	if _, err := exec.LookPath("claude"); err == nil {
		// brew --cask upgrade is macOS-only.
		if runtime.GOOS == "darwin" {
			if _, err := exec.LookPath("brew"); err == nil {
				return exec.Command("brew", "upgrade", "--cask", "claude-code").Run()
			}
		}
		if _, err := exec.LookPath("npm"); err == nil {
			return exec.Command("npm", "update", "-g", "@anthropic-ai/claude-code").Run()
		}
		return fmt.Errorf("no supported package manager found (brew or npm required)")
	}
	return InstallClaudeCmd()
}

// UpdateOpencode updates the opencode agent to the latest version.
// Installs it if not present.
func UpdateOpencode() error {
	if _, err := exec.LookPath("opencode"); err == nil {
		if runtime.GOOS == "darwin" {
			if _, err := exec.LookPath("brew"); err == nil {
				return exec.Command("brew", "upgrade", "anomalyco/tap/opencode").Run()
			}
		}
	if _, err := exec.LookPath("npm"); err == nil {
		return exec.Command("npm", "install", "-g", "--force", "opencode-ai@latest").Run()
	}
		return fmt.Errorf("no supported package manager found (brew or npm required)")
	}
	return InstallOpencodeCmd()
}

// UpdateAll updates all registered agents to their latest versions.
func UpdateAll() map[string]error {
	results := make(map[string]error)
	results["claude"] = UpdateClaude()
	results["opencode"] = UpdateOpencode()
	return results
}
