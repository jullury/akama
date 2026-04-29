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
