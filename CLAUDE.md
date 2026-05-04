# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What Akama Is

Akama is a Go CLI that acts as an AI coding agent orchestration system. A Telegram bot receives commands, fetches issues from GitHub/GitLab, runs `claude` or `opencode` locally to fix them in a cloned workspace, pushes a branch, creates a PR/MR, and notifies the user via Telegram.

## Build and Run

```bash
go build -o akama .          # compile
./akama init                 # first-run interactive setup → ~/.akama/config.yaml
./akama start                # fork daemon to background
./akama stop                 # send SIGTERM to daemon
./akama status               # check if running + active job count
./akama logs                 # tail ~/.akama/akama.log
```

No test suite exists yet. Build verification: `go build ./...`

## Architecture

The binary has two modes, detected in `main.go` before cobra runs:

- **Normal mode** (`cmd.Execute()`): handles `init`, `start`, `stop`, `status`, `logs` subcommands.
- **Daemon mode** (`runDaemon()` in `main.go`): triggered when `os.Args` contains `--daemon`. This is the re-exec'd child process launched by `akama start`. It loads config, opens SQLite, creates the bot, and blocks on `bot.RunCtx(ctx)`.

`akama start` forks itself via `daemon.ForkDaemon`, redirecting stdout/stderr to `~/.akama/akama.log`. The daemon writes its own PID file on startup (not the parent) to avoid race conditions between fork and PID write.

## Request Flow

```
Telegram update
  └── bot.RunCtx (long-polling, AllowedUpdates: message + callback_query)
        ├── Message  → router.handleMessage → command switch or handleText (state machine)
        └── Callback → router.handleCallback → startDeviceFlow (OAuth) or other
```

**`/connect` OAuth device flow** (GitHub or GitLab):
1. User taps provider button → `startDeviceFlow` calls GitHub/GitLab device code endpoint
2. Bot sends user a short code + URL; a goroutine polls for token approval
3. On approval: token stored in conversation state (`await_repo`), user prompted for repo URL
4. User sends repo URL → connection saved to `connections` table, state reset to `idle`

**Issue job flow** (triggered when user sends a GitHub/GitLab issue URL in `idle` state):
```
router.processIssue
  → look up token from connections table
  → fetch issue title/body from provider API
  → storage.CreateJob
  → go job.Run(jobID, ...)          ← goroutine tracked by sync.WaitGroup
        git.Clone → agent.Run → git.CommitPush → provider.CreatePR/MR
        → Telegram notification with PR URL
        → storage.SetJobNotifMsgID   ← enables reply-threading
```

**Follow-up** (user replies to a PR notification message):
```
router.handleReply
  → storage.GetJobByNotifMsgID(replyMsgID)
  → go job.RunFollowUp(jobID, userText, ...)
        agent.Run in existing workspace → git.CommitPush → Telegram update
        → storage.SetJobNotifMsgID (new message ID for next follow-up)
```

## Key Design Decisions

**GIT_ASKPASS for tokens**: `git.Clone` and `git.CommitPush` write a temporary executable script that echoes the token, set as `GIT_ASKPASS`. Tokens are never injected into the git remote URL (would appear in `ps aux` and git logs).

**Daemon self-check**: `runDaemon()` calls `daemon.IsRunning(pidPath)` before writing its own PID. A second instance launched while one is running logs a fatal error and exits — prevents duplicate Telegram polling sessions (which cause `409 Conflict` from Telegram).

**Conversation state machine** (`conversations` table, `data` column is JSON):
- `idle` — default; handles issue URLs and commands
- `await_repo` — after OAuth approval; expects a repo URL; `data` contains `{provider, token}`
- No `await_token` state — tokens come from OAuth device flow, not manual PAT entry

**Job `sync.WaitGroup`** in `internal/job/runner.go`: `job.Run` increments a package-level WaitGroup. `job.WaitForJobs(30)` in `runDaemon` drains it with a 30-second timeout during graceful shutdown.

## Config (`~/.akama/config.yaml`)

All paths support `~/` expansion (handled in `config.expandHome()`). Key fields:

```yaml
telegram_token: ""
anthropic_api_key: ""        # for claude agent
openai_api_key: ""           # for opencode agent
default_agent: "claude"      # claude | opencode
github_client_id: ""         # OAuth App for /connect device flow
github_client_secret: ""
gitlab_client_id: ""
gitlab_client_secret: ""
workspace_dir: "~/.akama/workspaces"
db_path: "~/.akama/akama.db"
log_path: "~/.akama/akama.log"
pid_path: "~/.akama/akama.pid"
```

GitHub OAuth App: enable **Device Flow** at `github.com/settings/developers`.
GitLab Application: tick **Use Device Authorization Grant** at `gitlab.com/-/user_settings/applications`.

## SQLite Schema

Auto-migrated by `storage.Open()`. Three tables:

- **`jobs`**: one row per issue fix job; `status` follows `pending → running → pr_created ⇄ updating → done/failed`; `notification_msg_id` is the Telegram message ID of the PR notification, used to match reply messages to jobs.
- **`conversations`**: one row per chat; `state` + `data` (JSON) drive the multi-turn connect/issue flow.
- **`connections`**: saved `(chat_id, provider, repo_url, git_token)` tuples; queried by `FindConnectionByRepo` to resolve tokens when an issue URL is sent.

## Telegram Library

Uses `github.com/go-telegram-bot-api/telegram-bot-api/v5`. Key v5 differences from v4:
- `GetUpdatesChan` returns only `UpdatesChannel` (no error)
- Webhook deletion: `api.Request(tgbotapi.DeleteWebhookConfig{})` (no `RemoveWebhook`)
- Callback acknowledgement: `api.Request(tgbotapi.NewCallback(query.ID, ""))` (no `AnswerCallbackQuery`)
- `AllowedUpdates` field on `UpdateConfig` — must explicitly include `"callback_query"` or inline button taps are silently dropped

## Agent Execution

`internal/agent/runner.go` execs either `claude` or `opencode` from `PATH` with `--dangerously-skip-permissions`. The agent binary must be installed locally. `ANTHROPIC_API_KEY` / `OPENAI_API_KEY` are injected into the subprocess environment from config.
