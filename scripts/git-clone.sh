#!/bin/bash
# Usage: git-clone.sh <repo_url> <token> <dest_path>
set -euo pipefail
REPO_URL=$1; TOKEN=$2; DEST=$3

ASKPASS=$(mktemp /tmp/akama-askpass-XXXXXX)
chmod 700 "$ASKPASS"
printf '#!/bin/sh\necho "%s"\n' "$TOKEN" > "$ASKPASS"
trap "rm -f $ASKPASS" EXIT

GIT_ASKPASS="$ASKPASS" GIT_TERMINAL_PROMPT=0 git clone --depth=1 "$REPO_URL" "$DEST"
