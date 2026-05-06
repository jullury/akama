# Akama Wiki

Welcome to the Akama wiki. This guide explains what Akama is, how it works, and how to use it effectively.

## What is Akama?

Akama is a self-hosted coding agent orchestration system controlled via Telegram. It automates the full workflow of fixing issues in your GitHub or GitLab repositories:

1. Paste a GitHub or GitLab issue URL into Telegram, or use the `/newissue` command
2. Akama clones the repository into a local workspace
3. Runs a coding agent (Claude or Opencode) to analyze and fix the issue
4. Pushes a fix branch and opens a pull request
5. Notifies you via Telegram with the PR link when complete

All processing happens on your own machine. The only external communication is with the GitHub/GitLab API on your behalf and Telegram for notifications.

## Key Features

- **Telegram-Native Control**: Manage everything from a Telegram chat with simple commands
- **Multi-Provider Support**: Works with both GitHub and GitLab repositories
- **Flexible Agent Choice**: Use Claude (Anthropic API) or Opencode (OpenAI API) as your coding agent
- **Automatic Agent Setup**: Agents are installed automatically on first use via Homebrew, npm, or direct download
- **Persistent State**: SQLite database tracks jobs, conversations, repository connections, and settings
- **Background Operation**: Runs as a daemon with proper logging, PID management, and graceful shutdown
- **Conversation Follow-Up**: Agents can ask for clarification via Telegram if they need more context

## Architecture Overview

Akama operates in two modes, detected at startup in `main.go`:

### CLI Mode
Runs interactive commands for setup and management:
- `akama init` - Interactive setup wizard
- `akama start` - Start the background daemon
- `akama stop` - Stop the daemon
- `akama status` - Check daemon status and active jobs
- `akama logs` - View application logs
- `akama update` - Update to the latest release

### Daemon Mode
Triggered when running as a background service (via `akama start` which forks itself):
- Loads configuration from `~/.akama/config.yaml`
- Opens the SQLite database and recovers any interrupted jobs
- Starts the Telegram bot to listen for updates
- Manages job execution, retries, and follow-ups

### Core Components
| Package | Purpose |
|---------|---------|
| `cmd/` | Cobra CLI commands for all user-facing operations |
| `bot/` | Telegram bot, message routing, and command handling |
| `agent/` | Coding agent management, prompt building, output parsing |
| `storage/` | SQLite database access, migrations, and CRUD operations |
| `job/` | Job execution logic, retry wrapper, and follow-up handling |
| `config/` | Configuration loading and OAuth variable management |
| `git/` | Git operations including clone, commit, push, and askpass helpers |
| `provider/` | GitHub and GitLab API clients |

## Getting Started

### 1. Install Akama
One-line install for macOS and Linux:
```sh
curl -fsSL https://raw.githubusercontent.com/jullury/akama/refs/heads/main/install.sh | sh
```

Or download a pre-built binary for your platform from the [releases page](https://github.com/jullury/akama/releases/latest).

### 2. Initial Setup
Run the interactive setup wizard:
```sh
akama init
```

You will be prompted for:
- Telegram bot token (create one via [@BotFather](https://t.me/BotFather))
- Agent choice: `claude` (requires Anthropic API key) or `opencode` (requires OpenAI API key)
- API key for your chosen agent
- Workspace directory for cloned repositories (default: `~/.akama/workspaces`)

Configuration is saved to `~/.akama/config.yaml`.

### 3. Start the Daemon
```sh
akama start
```

### 4. Connect a Repository
In your Telegram chat with the Akama bot, run:
```
/connect
```

Follow the OAuth prompts to authorize access to your GitHub or GitLab account.

### 5. Fix Your First Issue
Paste any GitHub or GitLab issue URL into the Telegram chat. Akama will:
- Clone the repository
- Run the coding agent to fix the issue
- Push a branch and open a pull request
- Send you a Telegram message with the PR link

## Telegram Commands

| Command | Description |
|---------|-------------|
| `/cancel` | Reset conversation state |
| `/config` | Configure git name, email, and model |
| `/connect` | Connect a repository account via OAuth |
| `/connections` | List all saved repository connections |
| `/delete_connection` | Delete a specific repository connection |
| `/disconnect` | Delete all connections for the current chat |
| `/done <id>` | Mark a job as done and clean up workspace |
| `/followup` | Continue working on an existing job |
| `/issues` | List completed issues |
| `/logs` | View agent output for a specific job |
| `/newissue` | Create a new issue in a connected repository |
| `/queue` | List all pending and running jobs |
| `/retry <id>` | Retry a failed job |
| `/start` | Show welcome message |
| `/status` | Show the last 10 jobs |
| `/update` | Update Akama to the latest version |
| `/update_agents` | Update coding agents to the latest version |

## Configuration Reference

Akama stores all configuration in `~/.akama/config.yaml`:
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

## Building from Source

Requires Go 1.21+ and OAuth app credentials:
```sh
git clone https://github.com/jullury/akama.git
cd akama
cp .env.example .env
# Fill in your OAuth credentials in .env:
# GITHUB_CLIENT_ID, GITHUB_CLIENT_SECRET, GITLAB_CLIENT_ID, GITLAB_CLIENT_SECRET
make build
```

OAuth credentials are baked into the binary at compile time via ldflags. Pre-built releases use Akama's registered OAuth apps.

## Viewing Logs

Akama writes logs to `~/.akama/logs/` with daily rotation (or 10 MB trigger) and keeps 7 gzip archives:
```sh
# Show today's log
akama logs

# Follow log output in real-time
akama logs -f

# Show all historical logs then follow
akama logs -a -f
```

## Troubleshooting

### Agent Not Found
After auto-installation, you must complete the agent's login process before running jobs. For example, run `claude` or `opencode` once manually to authenticate.

### Daemon Won't Start
Check the PID file and logs:
```sh
cat ~/.akama/akama.pid
akama logs
```

### OAuth Errors
If building from source, verify your OAuth credentials in `.env` are correct and that the app has the necessary repository permissions.

### Job Failures
View agent output for a failed job:
```
/logs <job_id>
```

Retry a failed job:
```
/retry <job_id>
```

## Additional Resources
- [README.md](https://github.com/jullury/akama/blob/main/README.md) - Installation and basic usage
- [AGENTS.md](https://github.com/jullury/akama/blob/main/AGENTS.md) - Details for coding agents working in this repository
- [GitHub Issues](https://github.com/jullury/akama/issues) - Report bugs or request features
