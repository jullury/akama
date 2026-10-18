# Quickstart: Fix Ollama Knowledge Base Flow

## Prerequisites
- Docker and Docker Compose installed.
- Repository cloned; `.env` populated with Telegram token and API keys.

## Validate (Docker path)
1. Start the stack: `docker compose up -d`
2. Confirm Ollama container is running: `docker compose ps` shows `ollama` healthy.
3. Check daemon logs for "Ollama is reachable" and model pull progress: `docker compose logs -f akama`.
4. Run a job via Telegram. With at least one previously completed job in the same repo, confirm `.akama-knowledge.md` appears in the workspace and the agent prompt references it.
5. Complete a job; verify an embedding row exists: `docker exec akama-postgres psql -U akama -d akama -c "SELECT count(*) FROM job_embeddings;"`.
6. Disable Ollama (`OLLAMA_URL=`) and re-run a job; confirm the job still completes and no repeated connection errors appear.

## Validate (host path)
1. `akama start` (starts Ollama container and pulls model).
2. Confirm `akama status` shows ollama running.
3. Run a job; verify knowledge injection and embedding as above.

## Expected Outcomes
- No connection-refused errors in daemon logs when Ollama is enabled.
- Knowledge file present for jobs with similar prior art.
- Every successful job has an embedding within 60 seconds of completion.
- Jobs complete normally when Ollama is disabled.