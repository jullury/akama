# Contract: Ollama Embeddings (Knowledge Base)

## Endpoint (preferred)
- `POST /api/embed`
- Request body: `{ "model": "nomic-embed-text", "input": "<text>" }` (or `prompt`)
- Response: `{ "embeddings": [[...]] }` (array of vectors)

## Endpoint (legacy fallback)
- `POST /api/embeddings`
- Request body: `{ "model": "nomic-embed-text", "prompt": "<text>" }`
- Response: `{ "embedding": [...] }` (single vector)

## Client Behavior
1. Try `/api/embed` first.
2. On 404 or parse failure, fall back to `/api/embeddings`.
3. Return a single `[]float32` vector (first element if array-of-arrays).
4. On total failure, return error; caller logs and degrades gracefully.

## Reachability
- `GET /api/tags` must return 200 for Ollama to be considered reachable.
- Reachability wait is bounded to 60 seconds.

## Model Ensure
- On startup, if `nomic-embed-text` is not in `/api/tags`, POST `/api/pull` with `{ "name": "nomic-embed-text" }` and read the streaming response to completion.
- Run in background; do not block daemon startup.