#!/bin/bash
set -e

# Create ~/.akama directory if it doesn't exist
mkdir -p ~/.akama/workspaces

# Generate config from environment variables
cat > ~/.akama/config.yaml << EOF
telegram_token: "${TELEGRAM_BOT_TOKEN:-}"
api_keys:
  openai: "${OPENAI_API_KEY:-}"
  anthropic: "${ANTHROPIC_API_KEY:-}"
default_agent: opencode
default_model: ""
agent_timeout_mins: 30
workspace_dir: ~/.akama/workspaces
db_path: ~/.akama/akama.db
log_path: ~/.akama/akama.log
pid_path: ~/.akama/akama.pid
EOF

chmod 600 ~/.akama/config.yaml

# Initialize DB (create if not exists)
akama status > /dev/null 2>&1 || true

echo "Config ready. Starting akama..."

# Execute the command
exec "$@"
