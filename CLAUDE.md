# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What Akama Is

Akama is a Go CLI that acts as an AI coding agent orchestration system. A Telegram bot receives commands, fetches issues from GitHub/GitLab, runs `claude` or `opencode` locally to fix them in a cloned workspace, pushes a branch, creates a PR/MR, and notifies the user via Telegram.

## Build and Run

OAuth credentials are baked in at compile time via `-ldflags`. Always build through `make`:

```bash
make build          # build with OAuth creds from .env
make start          # build + stop any running instance + start daemon
make clean          # remove binary
go build ./...      # quick compile check (no OAuth creds, Version = "dev")
```

`.env` file must contain `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`, `GITLAB_CLIENT_ID`, `GITLAB_CLIENT_SECRET`.

`config.Version` defaults to `"dev"` unless injected via `make build`. The `akama update` command refuses to run on dev builds.

CLI subcommands (normal mode):
```bash
./akama init        # interactive setup → ~/.akama/config.yaml
./akama start       # fork daemon to background
./akama stop        # SIGTERM daemon, blocks until it exits (up to 35s)
./akama status      # check if running + active job count
./akama logs        # show today's log; -f to follow, -a to show all archived logs
./akama restart     # stop + start daemon (starts fresh if not running)
./akama update      # download latest release binary and restart daemon
```

No test suite exists. Build verification: `go build ./...`

## Architecture

The binary has two modes, detected in `main.go` before cobra runs:

- **Normal mode** (`cmd.Execute()`): handles `init`, `start`, `stop`, `status`, `logs`, `restart`, `update`.
- **Daemon mode** (`runDaemon()` in `main.go`): triggered when `os.Args` contains `--daemon`. Loads config, opens SQLite, calls `storage.RecoverInterruptedJobs` (marks stale `running`/`awaiting_input` jobs as failed), creates the bot, blocks on `bot.RunCtx(ctx)`.

`akama start` forks itself via `daemon.ForkDaemon`. The daemon writes its own PID file; `akama stop` sends SIGTERM and polls until the PID file is gone (the daemon removes it via `defer` on clean exit) — this prevents a race where `make start` would launch a second instance while the first is still draining.

## Request Flow

```
Telegram update
  └── bot.RunCtx (long-polling, AllowedUpdates: message + callback_query)
        ├── Message  → router.handleMessage → command switch or handleText (state machine)
        └── Callback → router.handleCallback → OAuth / config field setter / newissue flow
```

**Bot commands** (handled before the state machine in `router.handleMessage`):
- `/start` — welcome message
- `/connect` — OAuth device flow (GitHub or GitLab inline button)
- `/connections` — list saved repo connections
- `/disconnect` — delete all connections for this chat
- `/config` — inline keyboard to set git name, email, and AI model
- `/newissue` — pick a connected repo, then send issue title + body; bot creates the issue and immediately starts a job
- `/issues` — list jobs with status `pr_created`
- `/status` — show last 5 jobs
- `/done <id>` — mark job done, clean up workspace
- `/cancel` — reset conversation state to `idle`

**`/connect` OAuth device flow:**
1. User taps provider button → `startDeviceFlow` calls GitHub/GitLab device code endpoint
2. Bot sends short code + URL; goroutine polls for token approval
3. On approval: token stored in conversation state (`await_repo`), user prompted for repo URL
4. User sends repo URL → saved to `connections` table, state reset to `idle`

**Issue job flow** (triggered when user sends a GitHub/GitLab issue URL in `idle` state):
```
router.processIssue
  → FindActiveJobByIssue — block duplicate submissions
  → fetch issue title/body from provider API
  → storage.CreateJob
  → job.Run(ctx, jobID, ...)           ← goroutine tracked by sync.WaitGroup
        git.Clone (retry ×3)
        agent.Run(ctx, ...)            ← exec.CommandContext — killed on ctx cancel
        if agent output ends with "?":
            SetJobAwaitingInput, set conversation state await_agent_input, return
        git.CommitPush (retry ×3)
        provider.CreatePR/MR (retry ×3, handles "already exists")
        → Telegram notification with PR URL
        → storage.SetJobNotifMsgID     ← enables reply-threading
```

**Agent question flow** (`awaiting_input` status):
- Conversation state set to `await_agent_input` with `{job_id}` in data
- Any plain-text message (not just a quoted reply) routes to `RunFollowUp`
- Commands (`/connect`, `/config`, etc.) still work — they're dispatched before `handleText`
- On daemon restart: `RecoverInterruptedJobs` resets `await_agent_input` conversation state

**Follow-up** (user replies to a PR notification message, or answers agent question):
```
router.handleReply / handleText(await_agent_input)
  → job.RunFollowUp(ctx, jobID, userText, ...)
        agent.Run → git.CommitPush
        if status was awaiting_input: also CreatePR/MR
        → Telegram update with PR URL
```

## Key Design Decisions

**GIT_ASKPASS**: `writeAskpass` uses `os.CreateTemp("", "git-askpass-*")` — file lands in OS temp dir, never inside the repo. Git identity (`user.name`/`user.email`) is only set if the user has configured it via `/config`; otherwise git falls back to system config.

**No bot identity in repo**: Agent prompts forbid mentioning AI/bots in code comments. Branch names and commit messages are derived from the actual changes (see below), not hardcoded.

**Prompt file**: `agent.WritePrompt` writes to `<workspacePath>/.akama-prompt.txt` (inside the cloned workspace), not OS temp. It is passed to the agent via `-p` and cleaned up after the run.

**Commit message, branch name, and PR description** are generated by a second focused agent call (`agent.GenerateSummary`) after the main fix is complete:
1. `git diff HEAD` is run in Go and passed to the agent with a minimal prompt
2. Agent outputs `COMMIT_MESSAGE: <text>` and `PR_DESCRIPTION: <text>` — both parsed from the response
3. `agent.BranchFromCommit` converts the commit message to a branch name: `feat: implement OWASP 2025 top 10` → `feat/implement-owasp-2025-top-10`; falls back to `fix/issue-X` if unparseable
4. Raw agent output is saved to `jobs.agent_output` for debugging

**Agent execution**:
- `claude`: `claude -p <promptFile> --dangerously-skip-permissions --output-format json` — outputs a single JSON envelope `{"result":"..."}`
- `opencode`: reads prompt file content, passes as message string arg with `--dir <workspacePath> --dangerously-skip-permissions --format json` — outputs NDJSON event stream; `agent.ParseOutput` collects text from `"text"` events and `message.content[].text` blocks; opencode exits 0 on API errors, so `extractOpencodeError` scans for `{"type":"api_error"}` events
- Both use `exec.CommandContext` so the subprocess is killed when the daemon context is cancelled (SIGTERM) or the per-job timeout (`agent_timeout_mins`, default 30) expires.

**Question detection**: `agent.IsQuestion(text)` returns true when the last non-empty line of agent output ends with `?`. This is the only signal used to enter `awaiting_input` state.

**Retry logic** (`internal/job/retry.go`): `withRetry(ctx, label, 3, fn)` retries git clone, push, and PR creation with 5s/15s/45s backoff. Returns immediately on context cancellation. PR creation handles "already exists" by fetching the existing PR URL via `provider.FindExistingPR`.

**Daemon self-check**: `runDaemon()` calls `daemon.IsRunning(pidPath)` before writing its PID — prevents duplicate Telegram polling sessions (which cause `409 Conflict`).

**Conversation state machine** (`conversations` table, `data` column is JSON):
- `idle` — default
- `await_repo` — after OAuth approval; `data`: `{provider, token}`
- `await_agent_input` — agent asked a question pre-commit; `data`: `{job_id}`
- `await_config` — user tapped a `/config` inline button; `data`: `{field: "git_name"|"git_email"|"model"}`
- `await_new_issue_title` / `await_new_issue_body` — `/newissue` multi-step flow; `data`: `{connection_id}`

## Config (`~/.akama/config.yaml`)

```yaml
telegram_token: ""
anthropic_api_key: ""
openai_api_key: ""
default_agent: "claude"          # claude | opencode
default_model: ""
agent_timeout_mins: 30           # kill agent subprocess after this many minutes
workspace_dir: "~/.akama/workspaces"
db_path:        "~/.akama/akama.db"
log_path:       "~/.akama/akama.log"
pid_path:       "~/.akama/akama.pid"
```

OAuth client IDs/secrets are **not** in config.yaml — they are baked in at build time via `make build` from `.env`.

## Logging

`log_path` in config is the base path. `logger.RotatingWriter` writes to `<log_dir>/logs/akama-YYYY-MM-DD.log` (a `logs/` subdirectory next to the base path). Rotation triggers daily or at 10 MB; old files are gzip-archived and the 7 most recent archives are kept. `akama logs -a` decompresses and prints all archives.

## SQLite Schema

The canonical schema is inline in `storage.migrate()` (`internal/storage/db.go`), auto-applied on `storage.Open()`. The file `migrations/001_akama.sql` is a stale Postgres draft and is not used.

Four tables:

- **`jobs`**: one row per issue fix job; `status`: `pending → running → pr_created ⇄ updating → done/failed`; also `awaiting_input` when agent asks a question before committing. `notification_msg_id` links reply messages to jobs.
- **`conversations`**: one row per chat; `state` + `data` (JSON) drive the multi-turn flow.
- **`connections`**: `(chat_id, provider, repo_url, git_token)` — queried by `FindConnectionByRepo`.
- **`user_config`**: per-chat `(git_name, git_email, agent_model)` set via `/config` command.

## Telegram Library

Uses `github.com/go-telegram-bot-api/telegram-bot-api/v5`. Key v5 notes:
- `GetUpdatesChan` returns only `UpdatesChannel` (no error)
- Webhook deletion: `api.Request(tgbotapi.DeleteWebhookConfig{})`
- Callback acknowledgement: `api.Request(tgbotapi.NewCallback(query.ID, ""))`
- `AllowedUpdates` must explicitly include `"callback_query"` or inline button taps are silently dropped
- `Bot.ctx` is set in `RunCtx` and threaded through to all job goroutines

## Release / CI

`.github/workflows/release.yml` runs semantic-release first (creates the git tag and GitHub release), then builds binaries with the version injected via `-X config.Version=$VERSION`, and uploads them to the release via `gh release upload`. Binaries are named `akama-<os>-<arch>`. The build matrix only runs when semantic-release actually cuts a new release.
