# Akama

Akama is an AI coding agent orchestration system controlled via Telegram. Send it a GitHub or GitLab issue URL, and it clones the repo, runs an AI agent to fix the issue, pushes a branch, and opens a pull request — notifying you when done.

## How It Works

1. Connect a repository via `/connect` (OAuth with GitHub or GitLab)
2. Paste any issue URL into the chat
3. Akama clones the repo, runs `claude` or `opencode` to fix it, and creates a PR
4. You get a Telegram message with the PR link

## Installation

### One-line install (macOS and Linux)

```sh
curl -fsSL https://raw.githubusercontent.com/jullury/akama/main/install.sh | sh
```

This downloads the latest binary for your OS and architecture to `/usr/local/bin/akama`.

### Manual download

Download the binary for your platform from the [releases page](https://github.com/jullury/akama/releases/latest):

| Platform       | Binary                    |
|----------------|---------------------------|
| Linux x86_64   | `akama-linux-amd64`       |
| Linux ARM64    | `akama-linux-arm64`       |
| macOS x86_64   | `akama-darwin-amd64`      |
| macOS ARM64    | `akama-darwin-arm64`      |

```sh
chmod +x akama-<os>-<arch>
sudo mv akama-<os>-<arch> /usr/local/bin/akama
```

## Setup

Run the interactive setup wizard:

```sh
akama init
```

You will be prompted for:

- **Telegram bot token** — create a bot via [@BotFather](https://t.me/BotFather)
- **AI agent** — `claude` (requires Anthropic API key) or `opencode` (requires OpenAI API key)
- **API key** — for the chosen agent
- **Workspace directory** — where repos are cloned (default: `~/.akama/workspaces`)

Configuration is saved to `~/.akama/config.yaml`.

## Usage

### Start / Stop

```sh
akama start    # start the daemon in the background
akama stop     # stop the daemon
akama status   # check if running and how many jobs are active
akama logs     # tail the log file
```

### In Telegram

| Command      | Description                                      |
|--------------|--------------------------------------------------|
| `/connect`   | Connect a GitHub or GitLab repository via OAuth  |
| `/config`    | Set git identity or override the AI model        |
| `/status`    | Check if the daemon is running                   |

**Fixing an issue:** paste a GitHub or GitLab issue URL into the chat while in `idle` state. Akama will start working on it immediately.

**Agent follow-up:** if the agent needs clarification before committing, it will ask you a question directly in Telegram. Reply with your answer to continue.

## Building from Source

Requires Go and OAuth app credentials for GitHub and GitLab baked in at compile time.

```sh
# Clone
git clone https://github.com/jullury/akama.git
cd akama

# Copy and fill in your OAuth credentials
cp .env.example .env

# Build
make build

# Build for all platforms (output to ./dist/)
make dist
```

### Required environment variables (`.env`)

```
GITHUB_CLIENT_ID=...
GITHUB_CLIENT_SECRET=...
GITLAB_CLIENT_ID=...
GITLAB_CLIENT_SECRET=...
```

## Creating a Release

Releases are created automatically on every push to `main`. The version tag is date-based (e.g. `v2026.05.04`). If multiple pushes happen on the same day, the release for that date is updated in place.

To publish a release manually:

```sh
make release            # tags today: v2026.05.04
make release VERSION=v2026.05.10  # override the tag
```

The workflow requires these repository secrets to be set in GitHub:

| Secret                    | Description                   |
|---------------------------|-------------------------------|
| `OAUTH_GITHUB_CLIENT_ID`     | GitHub OAuth app client ID    |
| `OAUTH_GITHUB_CLIENT_SECRET` | GitHub OAuth app client secret |
| `OAUTH_GITLAB_CLIENT_ID`     | GitLab OAuth app client ID    |
| `OAUTH_GITLAB_CLIENT_SECRET` | GitLab OAuth app client secret |

> **Note:** GitHub reserves the `GITHUB_` prefix for built-in secrets. Use the `OAUTH_GITHUB_*` names shown above.

## Configuration Reference

`~/.akama/config.yaml`:

```yaml
telegram_token: ""
anthropic_api_key: ""
openai_api_key: ""
default_agent: "claude"        # claude | opencode
default_model: ""              # leave blank to use agent default
agent_timeout_mins: 30
workspace_dir: "~/.akama/workspaces"
db_path:        "~/.akama/akama.db"
log_path:       "~/.akama/akama.log"
pid_path:       "~/.akama/akama.pid"
```
