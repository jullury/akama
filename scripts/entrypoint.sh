#!/bin/bash
set -e

# Auto-import credentials from environment variables so no UI setup is needed.
# Uses node (always available in the n8n image) to safely JSON-encode values.
echo "Importing Akama credentials from environment..."
node -e "
const creds = [];

creds.push({
  id: '1',
  name: 'Postgres account',
  type: 'postgres',
  data: {
    host:     process.env.DB_POSTGRESDB_HOST     || 'postgres',
    port:     parseInt(process.env.DB_POSTGRESDB_PORT || '5432'),
    database: process.env.DB_POSTGRESDB_DATABASE || 'akama',
    user:     process.env.DB_POSTGRESDB_USER     || 'akama',
    password: process.env.DB_POSTGRESDB_PASSWORD || 'akama',
    ssl: false,
  },
});

if (process.env.TELEGRAM_BOT_TOKEN) {
  creds.push({
    id: '2',
    name: 'Telegram account',
    type: 'telegramApi',
    data: { accessToken: process.env.TELEGRAM_BOT_TOKEN },
  });
}

creds.push({
  id: '3',
  name: 'Worker SSH',
  type: 'sshPassword',
  data: {
    host:     'worker',
    port:     22,
    username: process.env.SSH_USER     || 'root',
    password: process.env.SSH_PASSWORD || '',
  },
});

require('fs').writeFileSync('/tmp/akama-credentials.json', JSON.stringify(creds));
"
n8n import:credentials --input /tmp/akama-credentials.json \
  || echo "Warning: credential import failed, continuing..."
rm -f /tmp/akama-credentials.json

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
