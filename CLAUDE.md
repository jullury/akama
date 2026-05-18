# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What Akama Is

Akama is a Go CLI that acts as a coding agent orchestration system. A Telegram bot receives commands, fetches issues from GitHub/GitLab, runs `claude` or `opencode` locally to fix them in a cloned workspace, pushes a branch, creates a PR/MR, and notifies the user via Telegram.

## Build and Run

Toolchain is managed by [mise](https://mise.run/) via `.mise.toml`. After cloning:

```bash
curl https://mise.run/ | sh   # install mise if not present
eval "$(mise activate bash)"   # activate in current shell
mise install                   # install Go, Node, etc.
```

Or use the Make shortcut:

```bash
make setup      # install mise + toolchain
```

OAuth credentials are baked in at compile time via `-ldflags`. Always build through `make`:

```bash
make build          # build with OAuth creds from .env
make start          # build + stop any running instance + start daemon
make dist           # cross-compile for linux/darwin × amd64/arm64 → ./dist/
make clean          # remove binary and dist/
go build ./...      # quick compile check (no OAuth creds, Version = "dev")
```

`.env` file must contain `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`, `GITLAB_CLIENT_ID`, `GITLAB_CLIENT_SECRET`.

`config.Version` defaults to `"dev"` unless injected via `make build`. The `akama update` command refuses to run on dev builds. `make dist` strips debug symbols (`-s -w`) and uses `CGO_ENABLED=0` for static binaries.

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

**Platform**: Unix only. `daemon.ForkDaemon` uses `syscall.Setsid` to detach from the parent process group; Windows is not supported.

## Request Flow

```
Telegram update
  └── bot.RunCtx (long-polling, AllowedUpdates: message + callback_query)
        ├── Message  → router.handleMessage → command switch or handleText (state machine)
        └── Callback → router.handleCallback → OAuth / config field setter / newissue flow
```

**Bot commands** (handled before the state machine in `router.handleMessage`):
- `/start` / `/help` — welcome message and command list
- `/connect` — OAuth device flow (GitHub or GitLab inline button)
- `/connections` — list saved repo connections
- `/delete_connection` — delete a specific connection (prompts for selection)
- `/disconnect` — delete all connections for this chat
- `/config` — inline keyboard to set git name, email, agent, and model (model list from `agent.FetchModels`, paginated)
- `/newissue` — select one or more connected repos, then send issue title + body + optional images; creates the issue and starts jobs
- `/issues` — show job filter keyboard (Open / Running / Failed / Pending / All)
- `/queue` — list jobs with status `pending` or `running`
- `/status` — show last 10 jobs
- `/logs` — prompt for job ID, show that job's agent output (`agent_output` column)
- `/done` — prompt for job ID, mark done and clean up workspace
- `/retry` — prompt for job ID, reset a `failed` job to `pending` and requeue it
- `/followup` — prompt for job ID, resume working on a completed job
- `/cancel` — reset conversation state to `idle`
- `/skills` — browse and install skillhub.club skills
- `/update` — check for latest Akama release; prompts to download and restart
- `/update_agents` — update `claude` and `opencode` CLI tools to latest version
- `/users` — list authorized users (admin only)
- `/add_user` — add a user by Telegram ID (admin only)
- `/delete_user` — remove a user by Telegram ID (admin only)

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
      provider.EnrichIssueBody — downloads images from issue body + comments for agent context
  → agent.BuildClarifyingQuestionsPrompt → agent.RunPlanAgent — generates 3-5 clarifying questions
  → state: await_clarifying_questions
  → user answers → agent.BuildPlanFromAnswers → agent.RunPlanAgent — generates implementation plan
  → state: await_plan_review; user confirms or requests modifications (→ await_plan_regen)
  → on confirmation: storage.CreateJob (plan stored in jobs.plan)
  → job.Run(ctx, jobID, ...)           ← goroutine tracked by sync.WaitGroup + cancelFuncs map
        git.Clone (retry ×3)
        setupMise — runs `mise install` in cloned repo (silent, installs runtime deps)
        agent.Run(ctx, ...)            ← exec.CommandContext — killed on ctx cancel
        heartbeat goroutine sends "still working..." every 5 min while agent runs
        if agent output ends with "?":
            SetJobAwaitingInput, set conversation state await_agent_input, return
        git.Commit + git.Push (retry ×3)
        provider.CreatePR/MR (retry ×3, handles "already exists")
        → Telegram notification with PR URL
        → storage.SetJobNotifMsgID     ← enables reply-threading
        → pollCI goroutine: polls provider.GetCIStatus until branch merged or CI fails
```

**Multi-repo issue flow** (when multiple repos selected via `/newissue`):
```
b.processMultiIssue
  → jobs created with shared group_id = "group_<chatID>_<issueID>"
  → job.RunGrouped — workspace at workspaceDir/multi/<groupID>/
      each repo cloned as <owner>-<repo>/ subdirectory
      agent runs across all repos simultaneously
      CreatePR/MR per repo in parallel
  → consolidated Telegram notification after all repos complete
```

**Image attachment** (via `/newissue`): after issue body, state enters `await_issue_images`. User sends photos or `/done` to skip. Images downloaded from Telegram, uploaded via `provider.UploadGitHubImage` / `provider.UploadGitLabImage`, embedded as `![image](url)` in the issue body. 64KB body limit enforced; failed uploads silently skipped.

**Agent question flow** (`awaiting_input` status):
- Conversation state set to `await_agent_input` with `{job_id}` in data
- Any plain-text message (not just a quoted reply) routes to `RunFollowUp`
- Commands (`/connect`, `/config`, etc.) still work — they're dispatched before `handleText`
- On daemon restart: `RecoverInterruptedJobs` resets `await_agent_input` conversation state

**Follow-up** (user replies to a PR notification message, or answers agent question):
```
router.handleReply / handleText(await_agent_input)
  → job.RunFollowUp(ctx, jobID, userText, ...)
        agent.Run → git.Commit + git.Push
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
- Agent providers are registered via a registry pattern in `internal/agent/runner.go` — `agent.Register()` adds providers, `agent.Get()` retrieves them
- Built-in providers: `claude` and `opencode`; new providers just need to implement `agent.AgentRunner` interface and call `agent.Register()`
- `claude`: `claude -p <promptFile> --dangerously-skip-permissions --output-format json` — outputs a single JSON envelope `{"result":"..."}`
- `opencode`: reads prompt file content, passes as message string arg with `--dir <workspacePath> --dangerously-skip-permissions --format json` — outputs NDJSON event stream; exits 0 on API errors, so `extractOpencodeError` scans for `{"type":"api_error"}` events
- Both use `exec.CommandContext` so the subprocess is killed when the daemon context is cancelled (SIGTERM) or the per-job timeout (`agent_timeout_mins`, default 30) expires.
- `agent.FetchModels(name)` calls the provider's API to list available models; used to populate the paginated model picker in `/config`.
- Agent binaries are installed on first use via `agent.Install()` — tries Homebrew, then npm, then curl as fallback. Users must authenticate the installed CLI manually after installation.

**Question detection**: `agent.IsQuestion(text)` returns true when the last non-empty line of agent output ends with `?`. This is the only signal used to enter `awaiting_input` state.

**Retry logic** (`internal/job/retry.go`): `withRetry(ctx, label, 3, fn)` retries git clone, push, and PR creation with 5s/15s/45s backoff. Returns immediately on context cancellation. PR creation handles "already exists" by fetching the existing PR URL via `provider.FindExistingPR`.

**Daemon self-check**: `runDaemon()` calls `daemon.IsRunning(pidPath)` before writing its PID — prevents duplicate Telegram polling sessions (which cause `409 Conflict`).

**Concurrency model**: Each job goroutine is registered in two structures: a `sync.WaitGroup` (for graceful shutdown) and a `cancelFuncs` map keyed by jobID (for per-job cancellation). On SIGTERM, the daemon context is cancelled, then `wg.Wait()` drains with a 30s hard timeout. `/cancel <id>` looks up the job's `CancelFunc` in the map and calls it directly. Message handlers are dispatched as goroutines; long-polling itself runs in the main goroutine of `RunCtx`.

**Branch confirmation**: The first issue submitted for a given repo triggers the `await_branch_confirm` state — the bot asks the user to confirm or override the default branch before cloning. Subsequent issues for that repo skip this step and use the persisted default branch.

**HTTP timeouts**: Telegram long-poll client timeout is 90 seconds. Provider API calls (GitHub/GitLab) use a 30-second timeout. Agent subprocess timeout is `agent_timeout_mins` (default 30 minutes), enforced via `exec.CommandContext`.

**SQLite driver**: Uses `modernc.org/sqlite` — a pure-Go port with no CGO dependency. This is what makes `CGO_ENABLED=0` static builds possible via `make dist`.

**Provider helpers** (`internal/provider/client.go`): `GetCIStatus(repoURL, token, branch, providerName)` checks pipeline/check-run status and returns `"pending"`, `"success"`, `"failure"`, or `"none"`. `GetDefaultBranch(repoURL, token, providerName)` queries the repo API to resolve the default branch name used during `await_branch_confirm`.

**Plan mode**: Issue URLs trigger a two-phase pre-execution flow. `agent.BuildClarifyingQuestionsPrompt` → `agent.RunPlanAgent` generates 3-5 clarifying questions; user answers → `agent.BuildPlanFromAnswers` → `agent.RunPlanAgent` generates an implementation plan stored in `jobs.plan` and included in the job prompt. User can request modifications (`await_plan_regen`) or confirm to proceed. `proceedWithPlan` / `proceedWithMultiPlan` create jobs and start execution. A temporary clone is created for plan generation and cleaned up after confirmation.

**Multi-repo jobs**: `/newissue` opens a checkbox selector (`await_repo_select` state) for picking multiple repos. Per-repo branch overrides use `await_branch_select`. Jobs share `group_id` (`"group_<chatID>_<issueID>"`); `job.RunGrouped` (`internal/job/runner.go`) clones all repos into `workspaceDir/multi/<groupID>/<owner>-<repo>/` and runs the agent across them simultaneously. PRs created in parallel; single consolidated notification sent after all complete.

**Authorization**: When `admin_user_id` is non-zero, all incoming messages are checked against `authorized_users` — unrecognized users are silently ignored. Admin can add/remove users via `/add_user` and `/delete_user`. `admin_user_id: 0` (default) disables the check entirely.

**Skills system**: `agent.InjectedSkillsContent()` prepends `AlwaysInject: true` skill content to every agent prompt. `/skills` lists and installs skills from skillhub.club or a `RawURL` (GitHub raw). `Required: true` skills are always pre-checked and cannot be skipped. `agent.InstallSkill` handles both download methods.

**Image enrichment**: `provider.EnrichIssueBody` downloads images referenced in existing issue bodies and comments for agent context. For bot-created issues, `await_issue_images` state collects user-sent photos, uploads them to GitHub/GitLab, and embeds them as `![image](url)` in the issue body (64KB limit; failed uploads silently skipped). Send `/done` to skip.

**Token refresh**: When a provider API call returns 401, conversation state switches to `await_token_refresh` with `pending_action` data (`"process_issue"` or `"create_issue"`). After re-OAuth, `retryAfterTokenRefresh` replays the interrupted action automatically.

**Update mechanism**: `/update` calls `isNewerVersion` against GitHub releases. On non-Docker hosts, a detached helper script replaces the binary and restarts the daemon. On Docker (PID 1), the process exits and the container restarts.

**Mise in workspace**: `setupMise` runs `mise install` in each cloned repo before the agent runs, to install runtime dependencies declared in `.mise.toml`. Silent and non-blocking on failure.

**Conversation state machine** (`conversations` table, `data` column is JSON):
- `idle` — default
- `await_repo` — after OAuth approval; `data`: `{provider, token}`
- `await_agent_input` — agent asked a question pre-commit; `data`: `{job_id}`
- `await_config` — user tapped a `/config` inline button; `data`: `{field: "git_name"|"git_email"|"agent"|"model"}`
- `await_new_issue_title` / `await_new_issue_body` — `/newissue` multi-step flow
- `await_issue_images` — collecting optional images after issue body; `data`: `{title, body, repo_url, provider, token}` (or `repos` array for multi-repo)
- `await_repo_select` — multi-repo checkbox selection for `/newissue`; `data`: `{title, body, repos}`
- `await_branch_select` — per-repo branch selection in multi-repo flow
- `await_branch_confirm` — first job for a repo; bot asks user to confirm default branch; `data`: `{connection_id, branch}`
- `await_clarifying_questions` — user answering agent's questions about an issue before plan generation
- `await_plan_review` — user reviewing generated implementation plan; `data`: `{plan, workspace_path, connection_id, ...}`
- `await_plan_regen` — plan is regenerating with user's modification request
- `await_token_refresh` — OAuth re-auth required after 401; `data`: `{pending_action: "process_issue"|"create_issue", ...}` — action replayed after re-auth via `retryAfterTokenRefresh`
- `await_skill_id` — user entering a custom skill ID to install
- `await_logs` — user entering a job ID to view agent output
- `await_retry` — user entering a job ID to retry
- `await_followup_id` — user entering a job ID for follow-up
- `await_add_user` / `await_delete_user` — admin user management

## Config (`~/.akama/config.yaml`)

```yaml
telegram_token: ""
api_keys:
  anthropic: ""
  openai: ""
default_agent: "claude"          # registered agent name (claude, opencode, or any newly added provider)
default_model: ""
agent_timeout_mins: 30           # kill agent subprocess after this many minutes
workspace_dir: "~/.akama/workspaces"
db_path:        "~/.akama/akama.db"
log_path:       "~/.akama/akama.log"
pid_path:       "~/.akama/akama.pid"
admin_user_id:  0                # Telegram user ID of admin; 0 = no auth required (all users allowed)
```

OAuth client IDs/secrets are **not** in config.yaml — they are baked in at build time via `make build` from `.env`.

API keys are stored in the `api_keys` map (keyed by provider name like `anthropic`, `openai`). `config.Load` automatically migrates configs that still use the old flat `anthropic_api_key` / `openai_api_key` fields into the `api_keys` map on first load.

## Logging

`log_path` in config is the base path. `logger.RotatingWriter` writes to `<log_dir>/logs/akama-YYYY-MM-DD.log` (a `logs/` subdirectory next to the base path). Rotation triggers daily or at 10 MB; old files are gzip-archived and the 7 most recent archives are kept. `akama logs -a` decompresses and prints all archives.

## SQLite Schema

The canonical schema is inline in `storage.migrate()` (`internal/storage/db.go`), auto-applied on `storage.Open()`. The file `migrations/001_akama.sql` is a stale Postgres draft and is not used.

Five tables:

- **`jobs`**: one row per issue fix job; `status`: `pending → running → pr_created ⇄ updating → done/failed`; also `awaiting_input` when agent asks a question. Key columns: `group_id` (links multi-repo jobs; indexed), `plan` (generated implementation plan included in agent prompt), `images` (attached image URLs), `agent_output` (raw agent output for debugging), `notification_msg_id` (enables reply-threading).
- **`conversations`**: one row per chat; `state` + `data` (JSON) drive the multi-turn flow.
- **`connections`**: `(chat_id, provider, repo_url, git_token, default_branch)` — `default_branch` persisted after first `await_branch_confirm`.
- **`user_config`**: per-chat `(git_name, git_email, agent_model, agent)` — `agent` stores the preferred agent provider name.
- **`authorized_users`**: `(chat_id, role, added_by)` — access control; role is `"admin"` or `"user"`. `admin_user_id` from config is inserted as admin on daemon startup. `admin_user_id: 0` disables the check.

## Telegram Library

Uses `github.com/go-telegram-bot-api/telegram-bot-api/v5`. Key v5 notes:
- `GetUpdatesChan` returns only `UpdatesChannel` (no error)
- Webhook deletion: `api.Request(tgbotapi.DeleteWebhookConfig{})`
- Callback acknowledgement: `api.Request(tgbotapi.NewCallback(query.ID, ""))`
- `AllowedUpdates` must explicitly include `"callback_query"` or inline button taps are silently dropped
- `Bot.ctx` is set in `RunCtx` and threaded through to all job goroutines

## Release / CI

`.github/workflows/release.yml` runs semantic-release first (creates the git tag and GitHub release), then builds binaries with the version injected via `-X github.com/jullury/akama/internal/config.Version=$VERSION`, and uploads them to the release via `gh release upload`. Binaries are named `akama-<os>-<arch>`. The build matrix only runs when semantic-release actually cuts a new release.

Required repository secrets: `OAUTH_GITHUB_CLIENT_ID`, `OAUTH_GITHUB_CLIENT_SECRET`, `OAUTH_GITLAB_CLIENT_ID`, `OAUTH_GITLAB_CLIENT_SECRET`. These are prefixed with `OAUTH_` because GitHub reserves the `GITHUB_*` namespace for its own built-in secrets.

Commits must follow [Conventional Commits](https://www.conventionalcommits.org): `fix:` → patch, `feat:` → minor, `BREAKING CHANGE:` → major.
