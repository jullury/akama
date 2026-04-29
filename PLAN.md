# Akama — Implementation Plan (n8n edition)

## What It Does

Akama is an orchestration system that fetches issues from code/project trackers, runs an AI coding agent (opencode or claude) to fix them autonomously in a cloned workspace, creates a PR, and communicates with the user via Telegram.

**n8n is the orchestration backbone.** All workflow logic lives in n8n — no custom Go binary. The deliverable is a `docker compose` stack: a custom n8n image (with git + opencode pre-installed) + PostgreSQL.

Configuration (API keys, tokens, agent choice) is done entirely through the n8n web UI — no terminal wizard.

---

## Architecture

```
docker compose
  ├── n8n          ← custom image: n8n + git + opencode (pinned)
  └── postgres     ← n8n's own DB + custom job/conversation tables

n8n workflows (imported from JSON):
  01-telegram-handler   ← Telegram webhook: routes commands + messages
  02-run-job            ← clone → opencode → push → PR → notify
  03-follow-up          ← re-runs opencode on existing workspace
  04-setup-db           ← one-time: creates custom Postgres tables
```

---

## Tech Stack

| Layer | Choice |
|---|---|
| Orchestration | n8n (self-hosted, `n8nio/n8n` base) |
| Database | PostgreSQL 16 — n8n's own + custom tables |
| Telegram | n8n built-in Telegram node (webhook mode) |
| GitHub / GitLab | n8n built-in GitHub / GitLab nodes |
| Trello | n8n built-in Trello node |
| Git operations | Shell scripts via n8n "Execute Command" node |
| AI agent (default) | `opencode` CLI — pre-installed in Docker image |
| AI agent (alt) | `claude` CLI — pre-installed in Docker image |
| Agent auth | Env vars passed to n8n container (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`) |

---

## Project Structure

```
akama/
├── Dockerfile                     # n8n + git + opencode + claude (pinned versions)
├── docker-compose.yml             # n8n + postgres
├── .env.example                   # N8N_ENCRYPTION_KEY, DB vars, ANTHROPIC_API_KEY etc.
├── scripts/
│   ├── git-clone.sh              # GIT_ASKPASS clone: git clone --depth=1
│   ├── git-commit-push.sh        # stage + commit + force-push (GIT_ASKPASS)
│   └── run-agent.sh              # opencode or claude subprocess with HOME=/data/home
├── migrations/
│   └── 001_akama.sql             # custom tables: jobs, conversations
└── workflows/
    ├── 01-telegram-handler.json  # main bot entry point
    ├── 02-run-job.json           # job execution sub-workflow
    ├── 03-follow-up.json         # follow-up / updating cycle
    └── 04-setup-db.json          # one-time DB init (run manually once)
```

---

## Docker Setup

### Dockerfile

```dockerfile
FROM n8nio/n8n:latest

USER root

# Install git (n8n base is Alpine-based)
RUN apk add --no-cache git bash

# Install pinned agent versions via npm (npm is available in the n8n image)
ARG OPENCODE_VERSION=0.3.14
ARG CLAUDECODE_VERSION=1.0.17
RUN npm install -g opencode-ai@${OPENCODE_VERSION} \
    && npm install -g @anthropic-ai/claude-code@${CLAUDECODE_VERSION}

# Copy helper scripts
COPY scripts/ /usr/local/bin/akama-scripts/
RUN chmod +x /usr/local/bin/akama-scripts/*.sh

USER node

VOLUME ["/home/node/.n8n", "/workspaces", "/data/home"]
```

### docker-compose.yml

```yaml
services:
  postgres:
    image: postgres:16-alpine
    restart: unless-stopped
    environment:
      POSTGRES_USER: akama
      POSTGRES_PASSWORD: akama
      POSTGRES_DB: akama
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U akama"]
      interval: 5s
      retries: 5

  n8n:
    build: .
    restart: unless-stopped
    ports:
      - "5678:5678"           # n8n web UI
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      # n8n DB (n8n's own state)
      DB_TYPE: postgresdb
      DB_POSTGRESDB_HOST: postgres
      DB_POSTGRESDB_PORT: 5432
      DB_POSTGRESDB_DATABASE: akama
      DB_POSTGRESDB_USER: akama
      DB_POSTGRESDB_PASSWORD: akama
      # Security
      N8N_ENCRYPTION_KEY: ${N8N_ENCRYPTION_KEY}
      # Webhook URL for Telegram (must be publicly reachable, e.g. via ngrok in dev)
      WEBHOOK_URL: ${WEBHOOK_URL}
      # Agent API keys (passed through to shell scripts)
      ANTHROPIC_API_KEY: ${ANTHROPIC_API_KEY}
      OPENAI_API_KEY: ${OPENAI_API_KEY}
      # Agent HOME dir for opencode/claude config persistence
      AGENT_HOME: /data/home
    volumes:
      - n8n_data:/home/node/.n8n
      - workspaces:/workspaces
      - agent_home:/data/home

volumes:
  postgres_data:
  n8n_data:
  workspaces:
  agent_home:
```

### .env.example

```bash
# Required
N8N_ENCRYPTION_KEY=        # generate: openssl rand -hex 32
WEBHOOK_URL=               # public URL where n8n is reachable (e.g. https://akama.example.com)
ANTHROPIC_API_KEY=         # for opencode/claude with Anthropic models
OPENAI_API_KEY=            # optional: for OpenAI models
```

---

## Custom Postgres Tables (`migrations/001_akama.sql`)

n8n uses the same Postgres instance for its own tables. Akama's custom tables coexist in the same DB.

```sql
CREATE TABLE IF NOT EXISTS akama_jobs (
    id                 BIGSERIAL PRIMARY KEY,
    chat_id            TEXT   NOT NULL,
    platform           TEXT   NOT NULL DEFAULT 'telegram',
    provider           TEXT   NOT NULL,  -- 'github' | 'gitlab' | 'trello'
    repo_url           TEXT   NOT NULL,
    issue_id           TEXT   NOT NULL,
    issue_title        TEXT   NOT NULL,
    issue_body         TEXT   NOT NULL DEFAULT '',
    status             TEXT   NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending','running','pr_created','updating','done','failed')),
    branch_name        TEXT   NOT NULL DEFAULT '',
    pr_url             TEXT   NOT NULL DEFAULT '',
    workspace_path     TEXT   NOT NULL DEFAULT '',
    notification_msg_id TEXT  NOT NULL DEFAULT '',
    agent              TEXT   NOT NULL DEFAULT 'opencode',
    agent_model        TEXT   NOT NULL DEFAULT '',
    agent_output       TEXT   NOT NULL DEFAULT '',
    error_msg          TEXT   NOT NULL DEFAULT '',
    git_token          TEXT   NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS akama_conversations (
    chat_id     TEXT NOT NULL,
    platform    TEXT NOT NULL DEFAULT 'telegram',
    state       TEXT NOT NULL DEFAULT 'idle',
    data        JSONB NOT NULL DEFAULT '{}',
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (chat_id, platform)
);

CREATE INDEX IF NOT EXISTS idx_akama_jobs_chat ON akama_jobs(chat_id, platform, status);
CREATE INDEX IF NOT EXISTS idx_akama_jobs_notif ON akama_jobs(notification_msg_id);
```

**Token storage:** PATs are stored in n8n's encrypted credential store (via the n8n UI). The `git_token` column is a fallback; prefer credential vault references where possible.

---

## Shell Scripts (`scripts/`)

### `git-clone.sh`

Token is passed via a temporary `GIT_ASKPASS` script — never injected into the URL (avoids leaking in `ps aux`).

```bash
#!/bin/bash
# Usage: git-clone.sh <repo_url> <token> <dest_path>
set -euo pipefail
REPO_URL=$1; TOKEN=$2; DEST=$3

ASKPASS=$(mktemp /tmp/akama-askpass-XXXXXX)
chmod 700 "$ASKPASS"
printf '#!/bin/sh\necho "%s"\n' "$TOKEN" > "$ASKPASS"
trap "rm -f $ASKPASS" EXIT

GIT_ASKPASS="$ASKPASS" GIT_TERMINAL_PROMPT=0 git clone --depth=1 "$REPO_URL" "$DEST"
```

### `git-commit-push.sh`

```bash
#!/bin/bash
# Usage: git-commit-push.sh <workspace> <branch> <token> [commit_msg]
set -euo pipefail
WORKSPACE=$1; BRANCH=$2; TOKEN=$3; MSG=${4:-"fix: apply akama agent changes"}

ASKPASS=$(mktemp /tmp/akama-askpass-XXXXXX)
chmod 700 "$ASKPASS"
printf '#!/bin/sh\necho "%s"\n' "$TOKEN" > "$ASKPASS"
trap "rm -f $ASKPASS" EXIT

git -C "$WORKSPACE" add -A
git -C "$WORKSPACE" -c user.email=akama@bot -c user.name=Akama commit -m "$MSG" --allow-empty
GIT_ASKPASS="$ASKPASS" GIT_TERMINAL_PROMPT=0 \
  git -C "$WORKSPACE" push origin "$BRANCH" --force-with-lease
```

### `run-agent.sh`

```bash
#!/bin/bash
# Usage: run-agent.sh <agent> <model> <workspace> <prompt_file>
set -euo pipefail
AGENT=$1; MODEL=$2; WORKSPACE=$3; PROMPT_FILE=$4

export HOME="${AGENT_HOME:-/data/home}"
export ANTHROPIC_API_KEY="${ANTHROPIC_API_KEY:-}"
export OPENAI_API_KEY="${OPENAI_API_KEY:-}"

PROMPT=$(cat "$PROMPT_FILE")

case "$AGENT" in
  opencode)
    opencode run "$PROMPT" \
      --dangerously-skip-permissions \
      --format json \
      --dir "$WORKSPACE" \
      -m "$MODEL"
    ;;
  claudecode)
    cd "$WORKSPACE"
    claude -p "$PROMPT" \
      --dangerously-skip-permissions \
      --output-format json
    ;;
  *)
    echo "Unknown agent: $AGENT" >&2; exit 1 ;;
esac
```

---

## n8n Credentials Setup (via UI)

After `docker compose up`, open `http://localhost:5678` and add credentials:

| Credential name | Type | Fields |
|---|---|---|
| `GitHub PAT` | GitHub API | Personal Access Token |
| `GitLab PAT` | GitLab API | Personal Access Token |
| `Trello` | Trello API | API Key + Token |
| `Telegram Bot` | Telegram API | Bot Token |
| `Akama DB` | PostgreSQL | host=postgres, db=akama, user=akama, pw=akama |

---

## n8n Workflow Design

### Workflow 01: Telegram Handler (`01-telegram-handler.json`)

**Trigger:** Telegram Trigger node (webhook mode)

```
Telegram Trigger
  └── Switch (message type)
        ├── /start       → send welcome + command list
        ├── /connect     → start connection flow (state = await_provider)
        ├── /connections → list saved connections
        ├── /disconnect  → remove connection
        ├── /issues      → fetch issues → inline keyboard (max 10)
        ├── /status      → query akama_jobs for recent jobs
        ├── /done        → update job status='done', delete workspace
        ├── /cancel      → reset conversation state
        └── plain text   → check conversation state
              ├── await_* states → advance /connect flow (read/write akama_conversations)
              ├── reply to PR msg → call Workflow 03 (follow-up)
              └── no context → show help
```

**Issue selection callback:** user taps issue → insert `akama_jobs` (status=pending) → call Workflow 02 async → reply "Starting work on: {title}..."

### Workflow 02: Run Job (`02-run-job.json`)

**Trigger:** Called by Workflow 01 via "Execute Workflow" (async, receives job ID)

```
Read job from akama_jobs
Update status = 'running'
Send Telegram: "[org/repo] Working on: {title}..."
Execute Command: git-clone.sh {repo_url} {token} /workspaces/{job_id}
Update job: workspace_path
Write prompt to /tmp/prompt-{job_id}.txt
Execute Command: run-agent.sh {agent} {model} /workspaces/{job_id} /tmp/prompt-{job_id}.txt
Execute Command: git-commit-push.sh /workspaces/{job_id} akama/issue-{issue_id} {token}
GitHub/GitLab node: list PRs on branch → create if none (FindOrCreate)
Update job: status='pr_created', pr_url, branch_name, agent_output
Send Telegram: "[org/repo] PR ready — {title}\n{pr_url}\n\nReply for follow-up or /done {job_id}"
Update job: notification_msg_id = sent message ID
[on error] → status='failed', send error via Telegram, rm -rf workspace
```

### Workflow 03: Follow-up (`03-follow-up.json`)

**Trigger:** Called by Workflow 01 when reply to PR notification detected

```
Query akama_jobs WHERE notification_msg_id = {reply_msg_id}
Update status = 'updating'
Write follow-up prompt to /tmp/prompt-{job_id}.txt
Execute Command: run-agent.sh (reuses existing workspace — no re-clone)
Execute Command: git-commit-push.sh (force-with-lease, PR link unchanged)
Update status = 'pr_created', agent_output
Send Telegram: "[org/repo] Updated — {pr_url}\n\nReply for more or /done {job_id}"
Update notification_msg_id
```

### Workflow 04: Setup DB (`04-setup-db.json`)

**Trigger:** Manual (run once after first `docker compose up`)

```
Postgres node: execute migrations/001_akama.sql
Send response: "Akama tables created successfully"
```

---

## Prompt Template

Written to `/tmp/prompt-{job_id}.txt` by an n8n "Set" node:

```
You are fixing an issue in the current repository.

Issue Title: {issue_title}
Issue URL:   {issue_url}

Description:
{issue_body}   ← truncated to 8000 chars if needed

Instructions:
- Read the codebase to understand the context.
- Implement the minimal fix for this issue.
- Commit all changes with a descriptive message.
- Do NOT open pull requests — just commit.
[if follow_up]
Additional instructions from the user:
{follow_up_text}
```

---

## Job State Machine

```
pending → running → pr_created ⇄ updating → done
                 ↘ failed (workspace deleted)
```

Workspace at `/workspaces/{job_id}` is deleted only on `done` or `failed`.

---

## Follow-up Routing

In Workflow 01, when a plain-text message arrives:

```
IF message.reply_to_message.message_id IS SET:
    SELECT * FROM akama_jobs WHERE notification_msg_id = '{reply_id}'
    found → call Workflow 03
    not found → ignore

ELSE:
    SELECT * FROM akama_jobs WHERE chat_id='{id}' AND status='pr_created'
    0 jobs → show help
    1 job  → call Workflow 03
    2+ jobs → send inline keyboard "Which repo? Reply to the PR notification directly."
```

---

## Telegram /connect Flow

State tracked in `akama_conversations.state` + `data` JSONB.

**GitHub / GitLab:**
```
/connect → [GitHub][GitLab][Trello]       state=await_provider
tap GitHub → "Paste repo URL:"            state=await_repo_url
URL → "Paste PAT (repo+PR scopes):"      state=await_token
token → validate → store in n8n vault
       → "Connected: org/repo (GitHub, branch: main)"
                                          state=idle
```

**Trello:**
```
tap Trello → "Paste Trello API key:"      state=await_trello_apikey
→ "Paste Trello OAuth token:"             state=await_trello_token
→ "Paste board ID:"                       state=await_board
→ "Paste target git repo URL:"            state=await_repo_url
→ "Paste PAT for that repo:"              state=await_repo_token
→ store → "Connected!"                    state=idle
```

---

## Pinned Agent Versions

Versions are baked into the Docker image at build time:

```dockerfile
ARG OPENCODE_VERSION=0.3.14
ARG CLAUDECODE_VERSION=1.0.17
```

Updating requires rebuilding the image — no runtime version drift possible.

---

## First-Time Setup

1. `docker compose up --build`
2. Open `http://localhost:5678`, complete n8n account setup
3. Add credentials via n8n UI (Telegram Bot, GitHub PAT, Akama DB, etc.)
4. Import workflow JSONs from `workflows/` via Settings → Workflows → Import
5. Run Workflow 04 manually once to create custom Postgres tables
6. Activate all workflows (Telegram webhook registered automatically)

---

## Implementation Order

| Step | Deliverable |
|---|---|
| 1 | `Dockerfile` — n8n + git + opencode + claude (pinned) |
| 2 | `docker-compose.yml` + `.env.example` |
| 3 | `migrations/001_akama.sql` |
| 4 | `scripts/git-clone.sh` |
| 5 | `scripts/git-commit-push.sh` |
| 6 | `scripts/run-agent.sh` |
| 7 | `workflows/04-setup-db.json` |
| 8 | `workflows/02-run-job.json` |
| 9 | `workflows/03-follow-up.json` |
| 10 | `workflows/01-telegram-handler.json` |

---

## Verification Checklist

- [ ] `docker compose up --build` — postgres healthy, n8n reachable at `:5678`
- [ ] `opencode --version` in n8n container shows pinned version
- [ ] `claude --version` in n8n container shows pinned version
- [ ] Workflow 04 creates `akama_jobs` + `akama_conversations` tables
- [ ] Telegram bot responds to `/start`
- [ ] `/connect github` → multi-step flow, credential saved in n8n vault
- [ ] `/issues` shows real issues as inline keyboard
- [ ] Tapping an issue → Workflow 02 runs, opencode commits, PR created
- [ ] Telegram notified: `[org/repo] PR ready — {title}\n{pr_url}`
- [ ] Reply to PR notification → Workflow 03 runs, force-pushes, same PR URL
- [ ] `/done {id}` → job=done, workspace deleted
- [ ] Failed job → status=failed, workspace deleted, error sent via Telegram
- [ ] GIT_ASKPASS used — token not visible in `ps aux`
- [ ] n8n container restart → workflows re-activate, volumes intact
- [ ] `claudecode` path works via `run-agent.sh`
