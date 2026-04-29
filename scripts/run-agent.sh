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
