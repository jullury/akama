#!/bin/bash
set -e

# If running as root, seed binaries to volume on first run, fix ownership,
# then drop to the akama user for the rest of the process
if [ "$(id -u)" = "0" ]; then
    # Remove stale PID file left by a previous container instance.
    # The old container's processes are gone; keeping the file makes the new
    # daemon think another instance is running (the PID may be reused).
    rm -f /home/akama/.akama/akama.pid

    # Seed or upgrade the akama binary in the volume.
    # Compare versions so a rebuilt image (newer seed) replaces a stale volume binary,
    # while a user-updated binary (newer in volume) is left untouched.
    SEED_VER=$(/opt/akama/bin/akama --version 2>/dev/null || echo "seed-unknown")
    VOL_VER=$(/home/akama/.akama/bin/akama --version 2>/dev/null || echo "none")
    if [ "$SEED_VER" != "$VOL_VER" ]; then
        echo "Updating akama binary: $VOL_VER -> $SEED_VER"
        mkdir -p /home/akama/.akama/bin
        cp /opt/akama/bin/akama /home/akama/.akama/bin/akama
        chmod +x /home/akama/.akama/bin/akama
    fi

    # Seed npm packages (opencode, claude etc.) into volume on first run
    if [ ! -d /home/akama/.akama/.npm-global ]; then
        cp -r /opt/akama/.npm-global /home/akama/.akama/.npm-global
    fi

    chown -R akama:akama /home/akama/.akama
    exec gosu akama "$0" "$@"
fi

# Running as akama from here

# Install the npm package if the binary is missing; update it if outdated.
# NPM_CONFIG_PREFIX is already set to ~/.akama/.npm-global so writes go to the volume.
ensure_npm_pkg() {
    local pkg="$1"
    local bin="$2"
    if ! command -v "$bin" > /dev/null 2>&1; then
        echo "Installing $bin..."
        npm install -g "$pkg"
        return
    fi
    local outdated
    outdated=$(npm outdated -g "$pkg" --json 2>/dev/null || echo "{}")
    if [ "$outdated" != "{}" ] && [ -n "$outdated" ]; then
        echo "Updating $bin..."
        npm install -g "$pkg"
    fi
}

ensure_npm_pkg "opencode-ai" "opencode"
ensure_npm_pkg "@anthropic-ai/claude-code" "claude"

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

# Run the daemon in the foreground as PID 1 so docker logs captures its output directly.
# `akama --daemon` skips the fork that `akama start` normally performs.
exec akama --daemon
