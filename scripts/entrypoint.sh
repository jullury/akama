#!/bin/bash
set -e

# Import workflows from directory - import each file individually
if [ -d /workflows ] && [ "$(ls -A /workflows/*.json 2>/dev/null)" ]; then
  echo "Importing Akama workflows from /workflows..."
  for f in /workflows/*.json; do
    echo "  Importing $(basename $f)..."
    n8n import:workflow --input "$f" || echo "  Warning: failed to import $f, continuing..."
  done
  echo "Workflow import done."
else
  echo "No workflows directory or no JSON files found, skipping import."
fi

# Start n8n
exec n8n start
