# Akama — Go CLI Plan

## What It Does

Akama is an AI coding agent orchestration system. It receives issues (GitHub/GitLab) via a Telegram bot, runs `claude` or `opencode` locally to fix them, pushes a branch, creates a PR, and notifies the user via Telegram — all from a single Go binary.

This replaces the previous Docker/n8n implementation with a self-contained CLI that runs directly on the host machine. No Docker, no n8n, no webhook server required.

---

## CLI Commands

```
akama init      # interactive first-run setup → ~/.akama/config.yaml
akama start     # fork daemon to background, returns to shell immediately
akama stop      # send SIGTERM to daemon via PID file
akama status    # show running/stopped + active job count
akama logs      # tail ~/.akama/akama.log (Ctrl-C to exit)
```

---

## Architecture

```
akama (binary)
  ├── init     → prompt for config → write ~/.akama/config.yaml + migrate SQLite DB
  ├── start    → re-exec self as background daemon
  │               ├── Telegram long-polling loop       (mirrors workflow 01)
  │               ├── job goroutines (per job)          (mirrors workflow 02)
  │               └── follow-up goroutines (per reply)  (mirrors workflow 03)
  ├── stop     → SIGTERM via PID file
  ├── status   → PID liveness + DB job count
  └── logs     → tail ~/.akama/akama.log
```

---

## Project Layout

```
akama/
├── main.go
├── go.mod
├── go.sum
├── cmd/
│   ├── root.go           ← cobra root; loads config path flag
│   ├── init.go           ← interactive setup wizard
│   ├── start.go          ← fork daemon; return immediately
│   ├── stop.go           ← SIGTERM via PID file
│   ├── status.go         ← daemon liveness + job count
│   └── logs.go           ← tail log file
└── internal/
    ├── config/
    │   └── config.go     ← Config struct, load/save ~/.akama/config.yaml
    ├── storage/
    │   ├── db.go         ← open SQLite + run schema migration
    │   ├── jobs.go       ← Job struct + CRUD
    │   ├── conversations.go ← Conversation state CRUD
    │   └── connections.go   ← saved repo connections CRUD
    ├── daemon/
    │   └── daemon.go     ← re-exec fork, PID file read/write, IsRunning
    ├── git/
    │   └── git.go        ← Clone, CommitPush via GIT_ASKPASS
    ├── agent/
    │   └── runner.go     ← exec claude / opencode with prompt file
    ├── provider/
    │   ├── github.go     ← fetch issue, create PR
    │   └── gitlab.go     ← fetch issue, create MR
    ├── bot/
    │   ├── bot.go        ← Telegram API init + RunCtx (long-polling)
    │   ├── router.go     ← dispatch messages/callbacks to handlers
    │   └── commands.go   ← /start /connect /disconnect /issues /status /done /cancel
    └── job/
        ├── runner.go     ← workflow 02: clone → agent → push → PR → notify
        └── followup.go   ← workflow 03: re-run agent on existing workspace
```

Files to remove from the repo:
- `docker-compose.yml`
- `Dockerfile`
- `Dockerfile.worker`
- `scripts/` (entire directory)
- `workflows/` (entire directory)
- `.env.example`

`migrations/001_akama.sql` can be kept as schema reference; the SQLite schema lives in `storage/db.go`.

---

## Dependencies

```
github.com/spf13/cobra                            ← CLI framework
github.com/go-telegram-bot-api/telegram-bot-api/v5 ← Telegram long-polling
modernc.org/sqlite                                ← SQLite, pure Go (no CGO)
golang.org/x/term                                 ← password prompt in init
gopkg.in/yaml.v3                                  ← config file
```

---

## Config File: `~/.akama/config.yaml`

Created by `akama init`. All paths support `~/` expansion.

```yaml
telegram_token: ""
anthropic_api_key: ""
openai_api_key: ""        # optional; required for opencode
default_agent: "claude"   # claude | opencode
default_model: ""         # passed to agent -m flag; empty = agent default
workspace_dir: "~/.akama/workspaces"
db_path: "~/.akama/akama.db"
log_path: "~/.akama/akama.log"
pid_path: "~/.akama/akama.pid"
```

---

## SQLite Schema (`internal/storage/db.go`)

Auto-applied on `storage.Open()`. Idempotent (`CREATE TABLE IF NOT EXISTS`).

```sql
CREATE TABLE IF NOT EXISTS jobs (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    chat_id             INTEGER NOT NULL,
    issue_id            TEXT    NOT NULL DEFAULT '',
    issue_title         TEXT    NOT NULL DEFAULT '',
    issue_body          TEXT    NOT NULL DEFAULT '',
    issue_url           TEXT    NOT NULL DEFAULT '',
    repo_url            TEXT    NOT NULL DEFAULT '',
    provider            TEXT    NOT NULL DEFAULT '',   -- github | gitlab
    git_token           TEXT    NOT NULL DEFAULT '',
    agent               TEXT    NOT NULL DEFAULT 'claude',
    agent_model         TEXT    NOT NULL DEFAULT '',
    status              TEXT    NOT NULL DEFAULT 'pending',
    workspace_path      TEXT    NOT NULL DEFAULT '',
    branch_name         TEXT    NOT NULL DEFAULT '',
    pr_url              TEXT    NOT NULL DEFAULT '',
    notification_msg_id INTEGER NOT NULL DEFAULT 0,
    error_msg           TEXT    NOT NULL DEFAULT '',
    agent_output        TEXT    NOT NULL DEFAULT '',
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS conversations (
    chat_id    INTEGER NOT NULL,
    platform   TEXT    NOT NULL DEFAULT 'telegram',
    state      TEXT    NOT NULL DEFAULT 'idle',
    data       TEXT    NOT NULL DEFAULT '{}',  -- JSON object
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (chat_id, platform)
);

CREATE TABLE IF NOT EXISTS connections (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    chat_id    INTEGER NOT NULL,
    provider   TEXT    NOT NULL DEFAULT '',
    repo_url   TEXT    NOT NULL DEFAULT '',
    git_token  TEXT    NOT NULL DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_jobs_chat  ON jobs(chat_id, status);
CREATE INDEX IF NOT EXISTS idx_jobs_notif ON jobs(notification_msg_id);
```

---

## Job State Machine

```
pending → running → pr_created ⇄ updating → done
                 ↘ failed   (workspace deleted on failure)
```

Workspace at `{workspace_dir}/{job_id}` is deleted only on `done` or `failed`.

---

## `akama init` — Interactive Setup

```
1. Check ~/.akama/config.yaml exists → prompt to overwrite
2. Prompt: Telegram bot token     [required, password input]
3. Prompt: Anthropic API key      [required for claude, password input]
4. Prompt: OpenAI API key         [optional, password input]
5. Prompt: Default agent          [select: claude / opencode]
6. Prompt: Workspace directory    [default: ~/.akama/workspaces]
7. Write config to ~/.akama/config.yaml (chmod 600)
8. Open SQLite DB + run migration
9. Print: "Config saved. Run `akama start` to start the bot."
```

Password prompts use `golang.org/x/term.ReadPassword` so the token is hidden.

---

## Daemon Model (`internal/daemon/daemon.go`)

### `akama start` (parent process)

1. Load config; check PID file — error if already running
2. Open log file (`~/.akama/akama.log`) for append
3. Re-exec self: `exec.Command(os.Args[0], "start", "--daemon")` with:
   - `Stdout` / `Stderr` → log file
   - `Stdin` → nil
   - `SysProcAttr.Setsid = true` (detach from terminal)
4. Write `cmd.Process.Pid` to `~/.akama/akama.pid`
5. Print: `"akama daemon started (pid XXXX)"`; parent exits

### Child process (`--daemon` flag)

`main.go` scans `os.Args` for `--daemon` before cobra runs. If found, calls `runDaemon()` directly:

1. Load config, open SQLite DB
2. Create Telegram bot client
3. Set up `context.WithCancel`; install `SIGTERM`/`SIGINT` handler → `cancel()`
4. Call `bot.RunCtx(ctx)` — blocks until context cancelled
5. Wait for in-flight job goroutines (`sync.WaitGroup`), timeout 30s
6. Exit 0

### `akama stop`

1. Read PID from `~/.akama/akama.pid`
2. `kill -TERM <pid>`
3. Remove PID file
4. Print: `"akama daemon stopped (pid XXXX)"`

### `akama status`

1. Read PID file; send `kill -0` to check liveness
2. Query DB: `SELECT COUNT(*) FROM jobs WHERE status IN ('pending','running','updating')`
3. Print: `"running (pid XXXX), N active jobs"` or `"stopped"`

### `akama logs`

1. Open `~/.akama/akama.log`
2. Seek to last 4 KB; print existing lines
3. Poll for new bytes every 200ms; print as they arrive
4. Exit on SIGINT (Ctrl-C)

---

## Telegram Handler (`internal/bot/`)

Long-polling via `go-telegram-bot-api`. All updates handled in `bot.RunCtx(ctx)`.

### Message routing (`router.go`)

```
Incoming message
  └── Has ReplyToMessage?
        YES → look up job by notification_msg_id
              found (status pr_created|updating) → go followup.RunFollowUp(...)
        NO  → dispatch by text:
              /start          → handleStart
              /connect        → handleConnect (send inline keyboard: GitHub | GitLab)
              /connections    → list saved connections
              /disconnect     → delete all connections for chat_id
              /issues         → list open pr_created jobs
              /status         → list last 5 jobs
              /done <id>      → set job status = done
              /cancel         → reset conversation state to idle
              other text      → handleText (state machine)

Inline keyboard callback
  connect:github → state = await_repo, data.provider = github
  connect:gitlab → state = await_repo, data.provider = gitlab
```

### Conversation state machine (`handleText` in `router.go`)

```
idle        + issue URL   → fetch issue → create job → go job.Run(...)
await_repo  + repo URL    → save repo URL → state = await_token
await_token + PAT         → save connection → state = idle → "Connected! Send an issue URL."
```

Issue URL detection: any word in the message containing `github.com/.../issues/` or `gitlab.com/.../issues/`.

---

## Job Runner (`internal/job/runner.go`) — mirrors Workflow 02

Runs as `go job.Run(jobID, jobs, bot, cfg)` goroutine. Uses `wg.Add(1)` / `wg.Done()` for graceful shutdown.

```
1.  jobs.Get(jobID)
2.  jobs.SetRunning(jobID, workspacePath)
3.  bot.Send: "[provider] Working on: {title}..."
4.  os.MkdirAll(workspacePath)
5.  git.Clone(repo_url, token, workspacePath)
6.  os.WriteFile(promptPath, buildPrompt(title, url, body))
7.  agent.Run(agent, model, workspacePath, promptPath, agentCfg)
         ↳ on error → fail(jobs, bot, job, err, workspacePath); return
8.  git.CommitPush(workspacePath, "akama/issue-{issueID}", token)
9.  provider.CreatePR / provider.CreateMR  (detect from repo_url)
10. jobs.SetPRCreated(jobID, branch, prURL)
11. bot.Send: "PR ready — {title}\n{prURL}\n\nReply for follow-up or /done {id}"
12. jobs.SetNotifMsgID(jobID, sent.MessageID)

on fail:
    jobs.SetFailed(jobID, errMsg)
    bot.Send: "❌ Job {id} failed: {errMsg}"
    os.RemoveAll(workspacePath)
```

### Prompt template

```
You are fixing an issue in the current repository.

Issue Title: {title}
Issue URL:   {url}
Description:
{body}  ← truncated to 8000 chars

Implement a complete fix. Make all necessary code changes.
Do NOT create pull requests or push branches — that is handled separately.
```

---

## Follow-up Runner (`internal/job/followup.go`) — mirrors Workflow 03

Triggered by `router.go` when user replies to a PR notification.

```
1.  jobs.Get(jobID)  (found via notification_msg_id lookup)
2.  jobs.SetStatus(jobID, "updating")
3.  os.WriteFile(promptPath, buildFollowUpPrompt(userText))
4.  agent.Run(agent, model, job.WorkspacePath, promptPath, agentCfg)
         ↳ on error → failFollowUp(...)
5.  git.CommitPush(job.WorkspacePath, job.BranchName, token)
6.  jobs.SetStatus(jobID, "pr_created")
7.  bot.Send: "[provider] Updated — {prURL}\n\nReply for more or /done {id}"
8.  jobs.SetNotifMsgID(jobID, sent.MessageID)  ← enables chained follow-ups
```

### Follow-up prompt template

```
You are continuing work on the same repository.
Additional instructions from the user:

{userText}

Apply these changes to the existing code. Commit all changes.
Do NOT open pull requests — only make and commit code changes.
```

---

## Agent Runner (`internal/agent/runner.go`)

```go
// claude:   claude -p <prompt> --dangerously-skip-permissions --output-format json
// opencode: opencode run <prompt> --dangerously-skip-permissions --format json [-m model]
// Env: ANTHROPIC_API_KEY, OPENAI_API_KEY (from config)
// Returns: combined stdout+stderr as string
```

Both agents run with `cmd.Dir = workspacePath` so they operate on the cloned repo.

---

## Git Operations (`internal/git/git.go`)

**Clone** — uses a temp `GIT_ASKPASS` script that echoes the token (same pattern as the old `git-clone.sh`; token never injected into URL):

```bash
#!/bin/sh
echo '<token>'
```

```go
cmd = exec.Command("git", "clone", "--depth=1", repoURL, destPath)
cmd.Env = append(os.Environ(), "GIT_ASKPASS="+askpassPath, "GIT_TERMINAL_PROMPT=0")
```

**CommitPush:**

```go
git -C workspace config user.email akama@bot
git -C workspace config user.name Akama
git -C workspace add -A
git -C workspace commit --allow-empty -m "fix: apply akama agent changes"
git -C workspace checkout -B <branch>
GIT_ASKPASS=<script> git -C workspace push origin <branch> --force-with-lease
```

---

## Provider API Calls (`internal/provider/`)

### GitHub (`github.go`)

```
FetchIssue:  GET  https://api.github.com/repos/{owner}/{repo}/issues/{number}
             → title, body, number
CreatePR:    POST https://api.github.com/repos/{owner}/{repo}/pulls
             Authorization: Bearer {token}
             body: { title, head, base:"main", body }
             → html_url
```

### GitLab (`gitlab.go`)

```
FetchIssue:  GET  https://gitlab.com/api/v4/projects/{url-encoded-path}/issues/{iid}
             PRIVATE-TOKEN: {token}
             → title, description, iid
CreateMR:    POST https://gitlab.com/api/v4/projects/{url-encoded-path}/merge_requests
             PRIVATE-TOKEN: {token}
             body: { title, source_branch, target_branch:"main", description }
             → web_url
```

Provider detection: repo URL contains `github.com` → GitHub; `gitlab.com` → GitLab.

---

## Implementation Order

| # | File(s) | Notes |
|---|---|---|
| 1 | `go.mod` | module `github.com/jullury/akama`, go 1.23 |
| 2 | `internal/config/config.go` | Config struct, Load, Save, ExpandHome |
| 3 | `internal/storage/db.go` | Open, migrate |
| 4 | `internal/storage/jobs.go` | Job struct, CRUD methods |
| 5 | `internal/storage/conversations.go` | Get, Set, Reset |
| 6 | `internal/storage/connections.go` | Save, List, DeleteAll, FindByRepo |
| 7 | `internal/git/git.go` | Clone, CommitPush, writeAskpass, DetectProvider, OwnerRepo |
| 8 | `internal/agent/runner.go` | Run |
| 9 | `internal/provider/github.go` | FetchIssue, CreatePR |
| 10 | `internal/provider/gitlab.go` | FetchIssue, CreateMR |
| 11 | `internal/daemon/daemon.go` | Start, Stop, IsRunning, WritePID, ReadPID |
| 12 | `internal/job/runner.go` | Run goroutine |
| 13 | `internal/job/followup.go` | RunFollowUp goroutine |
| 14 | `internal/bot/bot.go` | New, RunCtx |
| 15 | `internal/bot/router.go` | route, handleCallback, handleText |
| 16 | `internal/bot/commands.go` | command handlers, saveConnection |
| 17 | `cmd/root.go` | cobra root |
| 18 | `cmd/init.go` | runInit |
| 19 | `cmd/start.go` | runStart |
| 20 | `cmd/stop.go` | runStop |
| 21 | `cmd/status.go` | runStatus |
| 22 | `cmd/logs.go` | runLogs (tail) |
| 23 | `main.go` | --daemon check, cmd.Execute() |

---

## Verification Checklist

- [ ] `go build ./...` — clean compile
- [ ] `./akama init` — prompts complete, `~/.akama/config.yaml` written, DB migrated
- [ ] `./akama start` — returns to shell immediately, PID file written
- [ ] `./akama status` — prints `running (pid XXXX), 0 active jobs`
- [ ] `./akama logs` — shows `polling started as @<botname>`; Ctrl-C exits
- [ ] Telegram `/start` → welcome message received
- [ ] Telegram `/connect` → inline buttons appear
- [ ] Select GitHub → send repo URL → send PAT → `"Connected! Send an issue URL."`
- [ ] Send GitHub issue URL → bot replies `"Working on: {title}..."` → job runs
- [ ] After job: bot sends PR URL notification
- [ ] Reply to PR notification → follow-up runs, PR updates (same URL)
- [ ] `/done {id}` → job marked done
- [ ] `/status` → last 5 jobs listed
- [ ] `./akama stop` → daemon exits cleanly; `./akama status` shows `stopped`
- [ ] Failed job → `status=failed`, workspace deleted, error sent to Telegram
- [ ] GIT_ASKPASS used — token not visible in process list
