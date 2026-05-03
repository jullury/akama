# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What Akama Is

Akama is an AI coding agent orchestration system. It fetches issues from GitHub/GitLab/Trello, runs `opencode` or `claude` CLI to fix them in a cloned workspace, pushes a branch, creates a PR, and notifies the user via Telegram — all triggered through a Telegram bot.

**n8n is the orchestration backbone.** All workflow logic lives in n8n JSON workflows. There is no custom application binary.

## Stack

- **n8n** (self-hosted) — workflow engine, exposes web UI at `:5678`
- **PostgreSQL 16** — n8n state + custom `akama_jobs` / `akama_conversations` tables
- **worker container** — Alpine with openssh-server, `opencode` and `claude` CLI pre-installed, exposes SSH on `:2222`; n8n connects via SSH to run agent commands
- **Shell scripts** in `scripts/` — git clone/push helpers and agent runner, called from n8n "Execute Command" (SSH) nodes

## Architecture: Two Containers

The `docker-compose.yml` defines three services:

| Service | Image | Role |
|---|---|---|
| `postgres` | postgres:16-alpine | shared DB for n8n + Akama tables |
| `n8n` | built from `Dockerfile` | n8n + git + entrypoint that auto-imports workflows |
| `worker` | built from `Dockerfile.worker` | opencode + claude + SSH server; executes agent work |

n8n does NOT run agents directly — it SSH-executes `run-agent.sh` on the `worker` container.

Workflows are auto-imported at startup: `entrypoint.sh` calls `n8n import:workflow` for every JSON in `/workflows/` before launching `n8n start`.

## Workflow Design

```
01-telegram-handler   ← main bot entry point (Telegram webhook)
02-run-job            ← clone → agent → push → PR → notify (called async by 01)
03-follow-up          ← re-run agent on existing workspace (called by 01 on reply)
04-setup-db           ← one-time manual run to create custom Postgres tables
```

**Job state machine:** `pending → running → pr_created ⇄ updating → done / failed`

Workspaces live at `/workspaces/{job_id}` and are deleted only on `done` or `failed`.

Token security: git tokens are passed to scripts via a temporary `GIT_ASKPASS` script, never injected into the URL.

## Setup

```bash
cp .env.example .env
# Fill in: N8N_ENCRYPTION_KEY (openssl rand -hex 32), WEBHOOK_URL, ANTHROPIC_API_KEY
docker compose up --build
```

After first startup:
1. Open `http://localhost:5678`, complete n8n account setup
2. Add credentials via n8n UI: Telegram Bot, GitHub PAT, Akama DB (`host=postgres`, `db/user/pw=akama`)
3. Run Workflow 04 manually once to create `akama_jobs` / `akama_conversations` tables
4. Activate all workflows (Telegram webhook registers automatically)

For dev/local testing, WEBHOOK_URL must be a publicly reachable URL (e.g. via ngrok).

## Rebuilding

```bash
docker compose up --build          # rebuild all images
docker compose build worker        # rebuild worker only (agent version updates)
docker compose logs -f n8n         # watch n8n logs
docker compose exec worker ssh     # not needed; SSH is for n8n→worker communication
```

## Pinned Agent Versions

Agent versions are baked into `Dockerfile.worker` at build time — no runtime drift:

```dockerfile
ARG OPENCODE_VERSION=1.1.4
ARG CLAUDECODE_VERSION=2.1.116
```

To upgrade, change the ARG and rebuild the worker image.

## Custom Postgres Tables

`migrations/001_akama.sql` creates `akama_jobs` and `akama_conversations`. Run it by executing Workflow 04 in the n8n UI. The migration is idempotent (`CREATE TABLE IF NOT EXISTS`).

Key columns in `akama_jobs`: `status`, `workspace_path`, `notification_msg_id` (used for reply-threading in Workflow 03), `agent`, `agent_output`, `git_token`.

## Modifying Workflows

Workflows are edited in the n8n UI, then exported as JSON back to `workflows/`. The n8n container mounts `/workflows/` from the image (baked in at build) — to pick up exported changes on the next restart, the image must be rebuilt or the JSON files must be updated before `docker compose up --build`.

When editing workflow JSON directly, maintain the n8n JSON schema (nodes array with `id`, `type`, `parameters`, `position`).
