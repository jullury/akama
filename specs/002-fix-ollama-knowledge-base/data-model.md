# Data Model: Fix Ollama Knowledge Base Flow

No new tables or migrations are introduced. This feature reuses the existing schema in `internal/storage/db.go`.

## Entities

### Job Embedding (`job_embeddings`)
- `job_id` (BIGINT, PK, FK → jobs.id, ON DELETE CASCADE): the completed job.
- `embedding` (vector(768)): vector representation of the job's issue/plan/output text.
- `indexed_at` (TIMESTAMPTZ): when the embedding was stored.

Validation: embedding dimension must be 768 (matches `nomic-embed-text` and `vector(768)` column). Insert uses upsert on `job_id` so re-running a job refreshes its embedding.

### Knowledge Usage (`knowledge_usage`)
- `id` (BIGSERIAL, PK)
- `job_id` (BIGINT, UNIQUE, FK → jobs.id, ON DELETE CASCADE): the job that used prior art.
- `knowledge_file_path` (TEXT): path to the generated `.akama-knowledge.md`.
- `similar_jobs_found` (INT): count of similar jobs retrieved.
- `similar_job_ids` (TEXT): comma-separated list of similar job IDs.
- `agent_referenced` (BOOLEAN): whether the agent referenced the knowledge.
- `created_at` (TIMESTAMPTZ).

### Job (existing, referenced)
- Fields used by knowledge flow: `id`, `issue_title`, `issue_body`, `plan`, `agent_output`, `error_msg`, `repo_url`, `provider`, `pr_url`, `branch_name`, `status`.

## Relationships
- `job_embeddings.job_id` → `jobs.id` (1:1, cascade delete).
- `knowledge_usage.job_id` → `jobs.id` (1:1, cascade delete, upsert on conflict).

## State Transitions
- Embedding is created after a job reaches a terminal success state (`done`/`pr_created`).
- Retrieval filters to jobs with status in (`done`, `pr_created`) and a non-null embedding.

## Notes
- No schema migration required; existing tables suffice.
- Existing jobs without embeddings are not backfilled.