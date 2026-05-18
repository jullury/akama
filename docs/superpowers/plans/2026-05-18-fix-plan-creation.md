# Fix Plan Creation (Issue #85) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the hang in `await_plan_review` (plan follow-up blocks forever with no feedback) and give the plan agent real codebase context by cloning the repository before generating questions/plan.

**Architecture:** Two changes: (1) move plan regeneration to a goroutine with a heartbeat + a guard state (`await_plan_regen`) to block duplicate submissions; (2) extend `RunPlanAgent` to accept an optional workspace path so callers can pass a cloned repo directory instead of an empty temp dir, and update `startPlanMode` / `processMultiIssue` to clone before calling the plan agent.

**Tech Stack:** Go, `exec.CommandContext`, `tgbotapi`, SQLite via `modernc.org/sqlite`

---

## File Map

| File | Change |
|------|--------|
| `internal/agent/runner.go` | Add `workspacePath` param to `RunPlanAgent`; create temp dir only when param is empty |
| `internal/bot/router.go` | `await_plan_review`: persist state first, async goroutine + heartbeat; add `await_plan_regen` guard case; `startPlanMode` + `processMultiIssue`: clone repo before `RunPlanAgent`; `proceedWithPlan` + `proceedWithMultiPlan` + `plan:cancel` callback: clean up plan workspace |

---

### Task 1: Extend `RunPlanAgent` to accept an optional workspace path

**Files:**
- Modify: `internal/agent/runner.go:622-641`

The current signature creates an empty temp dir unconditionally. We add a `workspacePath string` parameter: if non-empty, use it directly; if empty, create a temp dir (old behaviour, all existing callers pass `""` initially — updated in Task 3).

- [ ] **Step 1: Update `RunPlanAgent` signature and body**

In `internal/agent/runner.go`, replace the function at line 622:

```go
// RunPlanAgent runs the agent to generate plan-related content.
// If workspacePath is non-empty it is used directly (caller owns cleanup).
// If empty, a fresh temporary directory is created and removed on return.
func RunPlanAgent(ctx context.Context, agentName, model, workspacePath, promptContent string, cfg *Config) (string, error) {
	ownDir := workspacePath == ""
	if ownDir {
		var err error
		workspacePath, err = os.MkdirTemp("", "akama-plan-*")
		if err != nil {
			return "", fmt.Errorf("create temp dir: %w", err)
		}
		defer os.RemoveAll(workspacePath)
	}

	promptPath, err := WritePrompt(workspacePath, promptContent)
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
```

- [ ] **Step 2: Build to verify no compile errors**

```bash
cd /Users/Jullury/Projects/Jullury/akama && go build ./...
```

Expected: no output (clean build).

- [ ] **Step 3: Fix all existing `RunPlanAgent` call sites in `router.go` to add the new `""` argument**

There are four call sites. Each currently looks like:
```go
agent.RunPlanAgent(b.ctx, agentName, agentModel, prompt, agentCfg)
```
Change every occurrence to:
```go
agent.RunPlanAgent(b.ctx, agentName, agentModel, "", prompt, agentCfg)
```

Lines to update (search for `RunPlanAgent` in `router.go`): lines ~827, ~887, ~1335, ~1650.

- [ ] **Step 4: Build again to confirm**

```bash
go build ./...
```

Expected: clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/runner.go internal/bot/router.go
git commit -m "refactor(agent): add optional workspacePath param to RunPlanAgent"
```

---

### Task 2: Fix the `await_plan_review` hang — persist state + async goroutine + heartbeat + guard state

**Files:**
- Modify: `internal/bot/router.go:860-907` (the `await_plan_review` case)
- Modify: `internal/bot/router.go` (add `await_plan_regen` case in the same switch)

Two bugs fixed here:
1. User modifications were only saved in-memory (lost on restart) — now persisted before the agent runs.
2. `RunPlanAgent` blocked the goroutine with no feedback — now async with 30-second heartbeat ticks.

The new transient state `await_plan_regen` acts as a mutex: if the user sends another message while regen is in progress, they get a polite "still thinking" response instead of spawning a second agent.

- [ ] **Step 1: Replace the `await_plan_review` case body**

In `internal/bot/router.go`, replace the entire `case "await_plan_review":` block (lines 860–907) with:

```go
case "await_plan_review":
	mods := strings.TrimSpace(text)

	title, _ := conv.Data["title"].(string)
	body, _ := conv.Data["body"].(string)
	answers, _ := conv.Data["answers"].(string)
	agentName, _ := conv.Data["agent_name"].(string)
	agentModel, _ := conv.Data["agent_model"].(string)

	if agentName == "" {
		agentName = b.Config.DefaultAgent
	}

	agentCfg := &agent.Config{
		APIKeys:     b.Config.APIKeys,
		TimeoutMins: b.Config.AgentTimeoutMins,
	}

	updatedAnswers := answers
	if mods != "" {
		updatedAnswers = fmt.Sprintf("%s\n\nUser requested changes:\n%s", answers, mods)
		conv.Data["answers"] = updatedAnswers
	}

	// Persist modifications before running the agent so a daemon restart
	// doesn't lose the user's changes.
	storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_plan_regen", conv.Data)

	b.send(chatID, "⏳ Regenerating plan with your changes...")

	savedData := conv.Data
	go func() {
		heartbeatStop := make(chan struct{})
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-heartbeatStop:
					return
				case <-ticker.C:
					b.send(chatID, "⏳ Still thinking...")
				}
			}
		}()

		prompt := agent.BuildPlanFromAnswers(title, body, updatedAnswers)
		planOutput, agentErr := agent.RunPlanAgent(b.ctx, agentName, agentModel, "", prompt, agentCfg)
		close(heartbeatStop)

		if agentErr != nil {
			log.Printf("[await_plan_review] Failed to regenerate plan: %v", agentErr)
			// Restore state so the user can try again
			storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_plan_review", savedData)
			b.send(chatID, fmt.Sprintf("❌ Failed to regenerate plan: %v\n\nReply again to retry.", agentErr))
			return
		}

		savedData["plan"] = planOutput
		storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_plan_review", savedData)

		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Confirm", "plan:confirm"),
				tgbotapi.NewInlineKeyboardButtonData("Cancel", "plan:cancel"),
			),
		)
		msgText := fmt.Sprintf("Updated plan:\n\n%s\n\nReply with further changes or tap Confirm.", planOutput)
		msg := tgbotapi.NewMessage(chatID, msgText)
		msg.ReplyMarkup = keyboard
		b.API.Send(msg)
	}()
```

- [ ] **Step 2: Add the `await_plan_regen` guard case**

In the same `handleText` switch, after the `await_plan_review` block, add:

```go
case "await_plan_regen":
	b.send(chatID, "⏳ Still regenerating the plan — please wait a moment.")
```

- [ ] **Step 3: Build to verify**

```bash
go build ./...
```

Expected: clean build. Fix any import errors (ensure `time` is imported — it already is in `router.go`).

- [ ] **Step 4: Commit**

```bash
git add internal/bot/router.go
git commit -m "fix(bot): fix plan review hang — async regen with heartbeat and guard state"
```

---

### Task 3: Clone repository before plan generation for codebase context

**Files:**
- Modify: `internal/bot/router.go` — `startPlanMode` (line ~1258) and `processMultiIssue` (line ~1580)
- Modify: `internal/bot/router.go` — `proceedWithPlan`, `proceedWithMultiPlan`, and the `plan:cancel` callback

The plan workspace path is stored in `conv.Data["plan_workspace"]` so it can be cleaned up on confirm/cancel.

- [ ] **Step 1: Update `startPlanMode` to clone the repo into a temp dir before `RunPlanAgent`**

In `startPlanMode` (starting around line 1258), after the issue body is enriched and the user config is loaded, add a repo clone before the clarifying-questions agent call. Replace the section from `b.send(chatID, "Analyzing issue...")` through the `output, agentErr := agent.RunPlanAgent(...)` call with:

```go
b.send(chatID, "🔍 Cloning repository for analysis...")

planWorkspace, cloneErr := os.MkdirTemp("", "akama-plan-*")
if cloneErr != nil {
	b.send(chatID, fmt.Sprintf("❌ Failed to create workspace: %v", cloneErr))
	return
}

if cloneErr := git.Clone(repoURL, gitToken, planWorkspace, defaultBranch); cloneErr != nil {
	os.RemoveAll(planWorkspace)
	log.Printf("[startPlanMode] Failed to clone repo for plan context: %v", cloneErr)
	// Non-fatal: continue without codebase context
	planWorkspace = ""
}

b.send(chatID, "🤔 Analyzing issue to generate clarifying questions...")

prompt := agent.BuildClarifyingQuestionsPrompt(title, body)
output, agentErr := agent.RunPlanAgent(b.ctx, agentName, agentModel, planWorkspace, prompt, agentCfg)
if agentErr != nil {
	log.Printf("[startPlanMode] Failed to generate questions: %v", agentErr)
	// planWorkspace cleanup on non-fatal path handled below via conv.Data
}
```

Then add `"plan_workspace": planWorkspace` to the `storage.SetConversationState` data map at the end of `startPlanMode`:

```go
storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_clarifying_questions", map[string]interface{}{
	"issue_url":      issueURL,
	"git_token":      gitToken,
	"default_branch": defaultBranch,
	"images":         images,
	"provider":       providerName,
	"repo_url":       repoURL,
	"title":          title,
	"body":           body,
	"issue_id":       issueID,
	"agent_name":     agentName,
	"agent_model":    agentModel,
	"plan_workspace": planWorkspace, // ← add this
})
```

- [ ] **Step 2: Update `await_clarifying_questions` handler to pass plan_workspace to the plan agent**

In the `await_clarifying_questions` case (line ~802), read `planWorkspace` from conv.Data and pass it to `RunPlanAgent`:

```go
case "await_clarifying_questions":
	// ... existing code ...
	planWorkspace, _ := conv.Data["plan_workspace"].(string)
	// ... existing code building prompt ...
	planOutput, agentErr := agent.RunPlanAgent(b.ctx, agentName, agentModel, planWorkspace, prompt, agentCfg)
```

Also propagate `plan_workspace` when transitioning to `await_plan_review`:

```go
conv.Data["plan"] = planOutput
// plan_workspace is already in conv.Data; it carries forward automatically
storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_plan_review", conv.Data)
```

(No explicit action needed if `conv.Data` is passed through unchanged — verify this is the case in the existing code.)

- [ ] **Step 3: Update `await_plan_review` async goroutine to pass plan_workspace**

In the goroutine added in Task 2, read `planWorkspace` from `savedData` and pass it to `RunPlanAgent`:

```go
planWorkspace, _ := savedData["plan_workspace"].(string)
planOutput, agentErr := agent.RunPlanAgent(b.ctx, agentName, agentModel, planWorkspace, prompt, agentCfg)
```

- [ ] **Step 4: Clean up plan workspace in `proceedWithPlan`**

In `proceedWithPlan` (line ~1372), after `storage.ResetConversation`, add:

```go
if ws, _ := conv.Data["plan_workspace"].(string); ws != "" {
	os.RemoveAll(ws)
}
```

- [ ] **Step 5: Clean up plan workspace in `proceedWithMultiPlan`**

In `proceedWithMultiPlan` (line ~1423), after `storage.ResetConversation`, add:

```go
if ws, _ := conv.Data["plan_workspace"].(string); ws != "" {
	os.RemoveAll(ws)
}
```

- [ ] **Step 6: Clean up plan workspace in the `plan:cancel` callback**

In the `plan:cancel` branch of `handleCallback` (line ~604), after `storage.ResetConversation`, add:

```go
case "plan:cancel":
	conv, convErr := storage.GetConversation(b.JobsDB, chatID, "telegram")
	if convErr == nil {
		if ws, _ := conv.Data["plan_workspace"].(string); ws != "" {
			os.RemoveAll(ws)
		}
	}
	storage.ResetConversation(b.JobsDB, chatID, "telegram")
	b.send(chatID, "Plan cancelled. You can send another issue URL to start over.")
	return
```

- [ ] **Step 7: Update `processMultiIssue` to clone the first repo before plan generation**

In `processMultiIssue` (line ~1580), mirror the same clone pattern as `startPlanMode`. After loading `agentCfg`, add:

```go
b.send(chatID, "🔍 Cloning repository for analysis...")

planWorkspace, cloneErr := os.MkdirTemp("", "akama-plan-*")
if cloneErr != nil {
	log.Printf("[processMultiIssue] Failed to create workspace: %v", cloneErr)
	planWorkspace = ""
} else if cloneErr := git.Clone(repos[0]["repo_url"].(string), repos[0]["token"].(string), planWorkspace, repos[0]["default_branch"].(string)); cloneErr != nil {
	os.RemoveAll(planWorkspace)
	log.Printf("[processMultiIssue] Failed to clone for plan context: %v", cloneErr)
	planWorkspace = ""
}

b.send(chatID, "🤔 Analyzing issue to generate clarifying questions...")

prompt := agent.BuildClarifyingQuestionsPrompt(title, body)
output, agentErr := agent.RunPlanAgent(b.ctx, agentName, agentModel, planWorkspace, prompt, agentCfg)
```

Add `"plan_workspace": planWorkspace` to the `SetConversationState` call at the end of `processMultiIssue`.

- [ ] **Step 8: Build to verify**

```bash
go build ./...
```

Expected: clean build. The `git` package is already imported in `router.go`; verify `os` is imported (it already is).

- [ ] **Step 9: Commit**

```bash
git add internal/bot/router.go internal/agent/runner.go
git commit -m "feat(bot): clone repo before plan generation for codebase context"
```

---

### Task 4: Final build verification and smoke check

- [ ] **Step 1: Full clean build**

```bash
cd /Users/Jullury/Projects/Jullury/akama && go build ./...
```

Expected: no output.

- [ ] **Step 2: Verify `make build` works (builds with OAuth creds)**

```bash
make build
```

Expected: binary produced without errors.

- [ ] **Step 3: Final commit (if any stray files)**

```bash
git status
# If clean, nothing to do. Otherwise:
git add -p
git commit -m "chore: cleanup after plan fix"
```
