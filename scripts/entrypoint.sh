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

# Activate workflows via n8n public API once n8n is ready.
# Requires N8N_API_KEY — create one in n8n Settings > API after first login.
# Uses node (always available) — no curl/wget needed.
if [ -n "$N8N_API_KEY" ]; then
  node -e "
const http = require('http');
const fs   = require('fs');

function req(path, method, body) {
  return new Promise((resolve, reject) => {
    const payload = body ? JSON.stringify(body) : '';
    const headers = {
      'Content-Type': 'application/json',
      'Content-Length': Buffer.byteLength(payload),
      'X-N8N-API-KEY': process.env.N8N_API_KEY,
    };
    const r = http.request({ hostname: 'localhost', port: 5678, path, method, headers }, res => {
      let d = '';
      res.on('data', c => d += c);
      res.on('end', () => resolve({ status: res.statusCode, body: d }));
    });
    r.on('error', reject);
    if (payload) r.write(payload);
    r.end();
  });
}

async function wait() {
  for (;;) {
    try { const h = await req('/healthz', 'GET'); if (h.status === 200) return; } catch (_) {}
    await new Promise(r => setTimeout(r, 2000));
  }
}

(async () => {
  await wait();
  console.log('[setup] n8n ready, activating workflows...');

  const files = fs.readdirSync('/workflows')
    .filter(f => f.endsWith('.json') && !/^04-/.test(f));

  for (const file of files) {
    const wf = JSON.parse(fs.readFileSync('/workflows/' + file, 'utf8'));
    if (!wf.id) continue;
    const r = await req('/api/v1/workflows/' + wf.id + '/activate', 'POST', {});
    console.log('[setup] activate', file, ':', r.status);
  }

  console.log('[setup] done');
})().catch(e => console.error('[setup] error:', e.message));
" &
fi

# Start n8n
exec n8n start
