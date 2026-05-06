# Akama

<p align="center">
  <img src="logo.png" alt="Akama" width="480" />
</p>

<p align="center">
  <a href="https://github.com/jullury/akama/releases/latest"><img src="https://img.shields.io/github/v/release/jullury/akama" alt="Release"></a>
  <a href="https://golang.org"><img src="https://img.shields.io/badge/go-1.21+-00ADD8?logo=go" alt="Go"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/jullury/akama" alt="License"></a>
  <a href="CODE_OF_CONDUCT.md"><img src="https://img.shields.io/badge/Contributor%20Covenant-2.1-4baaaa" alt="Code of Conduct"></a>
</p>

Akama is a coding agent orchestration system controlled via Telegram. Send it a GitHub or GitLab issue URL — Akama clones the repo, runs an agent to fix the issue, pushes a branch, and opens a pull request, then notifies you when done.

---

## Table of Contents

- [How It Works](#how-it-works)
- [Installation](#installation)
- [Agent Auto-Installation](#agent-auto-installation)
- [Setup](#setup)
- [Usage](#usage)
  - [CLI Commands](#cli-commands)
  - [Logs](#logs)
  - [Telegram Commands](#telegram-commands)
- [Building from Source](#building-from-source)
- [Docker Deployment](#docker-deployment)
- [Creating a Release](#creating-a-release)
- [Configuration Reference](#configuration-reference)

---

## How It Works

1. Connect a repository via `/connect` (OAuth with GitHub or GitLab)
2. Paste any issue URL into the Telegram chat
3. Akama clones the repo, runs `claude` or `opencode` to fix it, and creates a PR
4. You receive a Telegram message with the PR link

---

## Installation

### One-line install (macOS and Linux)

```sh
curl -fsSL https://raw.githubusercontent.com/jullury/akama/refs/heads/main/install.sh | sh
```

Downloads the latest binary for your OS and architecture to `/usr/local/bin/akama`.

### Manual download

Download the binary for your platform from the [releases page](https://github.com/jullury/akama/releases/latest):

| Platform     | Binary               |
|--------------|----------------------|
| Linux x86_64 | `akama-linux-amd64`  |
| Linux ARM64  | `akama-linux-arm64`  |
| macOS x86_64 | `akama-darwin-amd64` |
| macOS ARM64  | `akama-darwin-arm64` |

```sh
chmod +x akama-<os>-<arch>
sudo mv akama-<os>-<arch> /usr/local/bin/akama
```

### OAuth App Notice

The pre-built binaries released in this repository have GitHub and GitLab OAuth app credentials baked in at compile time. This means the OAuth flow uses Akama's registered app identity, which is granted access to your repositories when you run `/connect`.

**This access is used exclusively for the operations the app is designed to perform:**
- Cloning the repository to a local workspace
- Pushing a fix branch
- Opening a pull request or merge request

No repository data is transmitted to any third party. Akama runs entirely on your own machine — the only outbound calls are to the GitHub/GitLab API on your behalf and to Telegram for notifications.

If you prefer to use your own OAuth app credentials, [build from source](#building-from-source) with your own `.env` values.

---

## Agent Auto-Installation

`opencode` and `claude` are automatically installed on first use. Akama selects the best method based on your system:

| Method   | Requires      | When used                         |
|----------|---------------|-----------------------------------|
| Homebrew | `brew`        | macOS/Linux with Homebrew         |
| npm      | `npm`         | Systems with Node.js installed    |
| curl     | `curl`        | Fallback — direct binary download |

> **Note:** After auto-installation, you must manually complete the login/authentication for your chosen agent (`opencode` or `claude`) before running jobs. Model selection is handled entirely through Telegram via the `/config` command — no need to configure models manually.

---

## Setup

Run the interactive setup wizard:

```sh
akama init
```

You will be prompted for:

| Prompt              | Description                                                            |
|---------------------|------------------------------------------------------------------------|
| Telegram bot token  | Create a bot via [@BotFather](https://t.me/BotFather)                 |
| Agent             | `claude` (Anthropic API key) or `opencode` (OpenAI API key)           |
| API key             | Key for the chosen agent                                               |
| Workspace directory | Where repos are cloned — default: `~/.akama/workspaces`               |

Configuration is saved to `~/.akama/config.yaml`.

---

## Usage

### CLI Commands

| Command          | Description                                       |
|------------------|---------------------------------------------------|
| `akama init`     | Interactive setup wizard                          |
| `akama start`    | Start the daemon in the background                |
| `akama stop`     | Stop the daemon                                   |
| `akama restart`  | Stop and restart the daemon                       |
| `akama status`   | Check if running and how many jobs are active     |
| `akama logs`     | Show today's log file                             |
| `akama update`   | Download the latest release binary and restart    |

### Logs

```sh
akama logs [flags]
```

| Flag             | Short | Description                                      |
|------------------|-------|--------------------------------------------------|
| `--follow`       | `-f`  | Follow log output (tail mode)                    |
| `--all`          | `-a`  | Show all log files, including gzip archives      |

**Examples:**

```sh
# Show today's log
akama logs

# Stream new log lines as they are written
akama logs -f

# Print all historical logs (including rotated archives) then stream
akama logs -a -f
```

Logs are written to `<log_dir>/logs/akama-YYYY-MM-DD.log`. Files are rotated daily or at 10 MB; old files are gzip-archived and the 7 most recent archives are kept.

### Telegram Commands

| Command         | Description                                      |
|-----------------|--------------------------------------------------|
| `/cancel`       | Reset conversation state                         |
| `/config`       | Configure git name, email, and model             |
| `/connect`      | Connect repository account                       |
| `/connections`  | List saved repo connections                      |
| `/delete_connection` | Delete a specific connection                |
| `/disconnect`   | Delete all connections for this chat             |
| `/done <id>`    | Mark job done and clean up                      |
| `/followup`     | Continue working on a job                        |
| `/issues`       | List completed issues                            |
| `/logs`         | View agent output for a job                      |
| `/newissue`     | Create a new issue                               |
| `/queue`        | List pending and running jobs                    |
| `/retry <id>`   | Retry a failed job                               |
| `/start`        | Welcome message                                  |
| `/status`       | Show last 10 jobs                                |
| `/update`       | Update Akama server binary to the latest version |
| `/update_agents`| Update agents to latest version                  |

**Fixing an issue:** paste a GitHub or GitLab issue URL into the chat while in `idle` state. Akama starts working on it immediately.

**Agent follow-up:** if the agent needs clarification before committing, it asks you directly in Telegram. Reply with your answer to continue.

---

## Building from Source

Requires Go and OAuth app credentials baked in at compile time.

```sh
# Clone
git clone https://github.com/jullury/akama.git
cd akama

# Copy and fill in your OAuth credentials
cp .env.example .env

# Build binary
make build

# Build for all platforms (output to ./dist/)
make dist
```

### Required environment variables (`.env`)

```env
GITHUB_CLIENT_ID=...
GITHUB_CLIENT_SECRET=...
GITLAB_CLIENT_ID=...
GITLAB_CLIENT_SECRET=...
```

> OAuth credentials are **not** stored in `config.yaml` — they are embedded at build time via `make build`.

---

## Docker Deployment

The included `Dockerfile` and `docker-compose.yml` provide an isolated environment for running Akama in production mode without affecting your host system.

### Prerequisites

Copy the example environment file and fill in your credentials:

```sh
cp .env.example .env
```

Edit `.env` with your values:
- `TELEGRAM_BOT_TOKEN` — from [@BotFather](https://t.me/BotFather)
- `OPENAI_API_KEY` — required for opencode (default agent)
- `ANTHROPIC_API_KEY` — required for claude agent

### Start with docker-compose

```sh
docker-compose up -d
```

This will:
- Build the image using `install.sh` to download the latest akama binary
- Install `opencode` as the default agent globally
- Generate `config.yaml` from environment variables at runtime
- Start akama as a background daemon
- Persist state (config, database, logs) via the `akama-data` volume
- Automatically restart the container unless explicitly stopped

### View logs

```sh
docker-compose logs -f
```

### Stop the container

```sh
docker-compose down
```

### Notes

- The container includes `git`, `curl`, `bash`, `nodejs`, and `npm` — sufficient for agent auto-installation and repo operations.
- State is persisted in the `akama-data` Docker volume (mounted to `/root/.akama` inside the container).
- `opencode` is set as the default agent; change `default_agent` in your `.env` if needed.
- No build from source is required — the image downloads the pre-built binary via `install.sh`.
- Configuration is generated at runtime from environment variables — no need to bake in OAuth credentials.

---

## Creating a Release

Releases are created automatically by [semantic-release](https://semantic-release.gitbook.io) on every push to `main`. The version is derived from [Conventional Commits](https://www.conventionalcommits.org):

| Commit prefix  | Release type |
|----------------|--------------|
| `fix:`         | Patch        |
| `feat:`        | Minor        |
| `BREAKING CHANGE:` | Major   |

No manual tagging is required. Simply merge to `main` with a properly-formatted commit message and the pipeline handles versioning, changelog generation, and binary uploads automatically.

### Required repository secrets

| Secret                       | Description                    |
|------------------------------|--------------------------------|
| `OAUTH_GITHUB_CLIENT_ID`     | GitHub OAuth app client ID     |
| `OAUTH_GITHUB_CLIENT_SECRET` | GitHub OAuth app client secret |
| `OAUTH_GITLAB_CLIENT_ID`     | GitLab OAuth app client ID     |
| `OAUTH_GITLAB_CLIENT_SECRET` | GitLab OAuth app client secret |

> **Note:** GitHub reserves the `GITHUB_` prefix for built-in secrets. Use the `OAUTH_GITHUB_*` names shown above.

---

## Configuration Reference

`~/.akama/config.yaml`:

```yaml
telegram_token: ""
api_keys:
  anthropic: ""
  openai: ""
default_agent: "claude"        # claude | opencode
default_model: ""              # leave blank to use the agent's default
agent_timeout_mins: 30         # kill the agent subprocess after this many minutes
workspace_dir: "~/.akama/workspaces"
db_path:        "~/.akama/akama.db"
log_path:       "~/.akama/akama.log"
pid_path:       "~/.akama/akama.pid"
```
