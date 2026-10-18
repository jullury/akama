# Implementation Plan: Fix Ollama Knowledge Base Flow

**Branch**: `002-fix-ollama-knowledge-base` | **Date**: 2026-10-18 | **Spec**: `/specs/002-fix-ollama-knowledge-base/spec.md`

**Input**: Feature specification from `/specs/002-fix-ollama-knowledge-base/spec.md`

## Summary

The knowledge base flow is broken primarily in the containerized deployment: the Ollama service is commented out of `docker-compose.yml`, `OLLAMA_URL` is empty, and the generated daemon `config.yaml` omits `ollama_url`, leaving the daemon pointed at a wrong `localhost` default. The fix re-enables Ollama as a required service, wires its internal URL through config generation, makes the embedding client compatible with both Ollama embedding endpoints, and ensures model pulling happens in the background with a bounded reachability wait and clean degradation when Ollama is unavailable. No storage-engine change; existing Postgres+pgvector schema is reused.

## Technical Context

**Language/Version**: Go 1.26+ (daemon and host CLI)
**Primary Dependencies**: pgx (PostgreSQL), pgvector (`vector(768)`), Ollama HTTP API, Docker SDK (host CLI), Telegram bot API
**Storage**: PostgreSQL with pgvector (`job_embeddings`, `knowledge_usage` tables), managed by `internal/storage/db.go`
**Testing**: None (project has no test suite); verification gate is `go build ./...`
**Target Platform**: Linux host CLI + Linux container daemon (Docker)
**Project Type**: Two-binary (host CLI `akama`, in-container daemon `akama-daemon`)
**Performance Goals**: No blocking daemon startup; background model pull; bounded 60s reachability wait
**Constraints**: Graceful degradation when Ollama unavailable; no job failure due to knowledge flow; no repeated connection errors
**Scale/Scope**: Single operator/small team; knowledge base grows incrementally with completed jobs

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. Two-Binary Architecture**: PASS — host CLI changes (docker-compose, start.go) manage containers/config; daemon changes (config, knowledge) run bot/jobs. No cross-boundary calls introduced.
- **II. Build-Time Secret Injection**: PASS — no secrets added; Ollama has no auth.
- **III. Conventional Commits**: N/A at plan time; applies at commit.
- **IV. Build Verification Gate**: PASS — `go build ./...` remains the gate; no test infra required.
- **V. SQLite Schema Ownership**: NOTE — constitution states "SQLite only", but the codebase and this feature use PostgreSQL+pgvector (existing `internal/storage/db.go`). This feature makes no schema-engine change and introduces no new storage; it reuses existing tables. Treated as a pre-existing, justified deviation, not introduced here.
- **VI. Daemon Lifecycle Safety**: PASS — background goroutine for model pull uses daemon context; no PID-file changes.
- **VII. Security Defaults**: PASS — container continues to run as non-root; config written 0600.

**Additional Constraints**: Storage stated as SQLite in constitution vs. actual Postgres is a pre-existing doc mismatch (see note). Telegram chunking, heartbeat, agent abstraction unchanged.

**Gate result**: PASS (one pre-existing, justified deviation noted).

## Project Structure

### Documentation (this feature)

```text
specs/002-fix-ollama-knowledge-base/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
└── tasks.md             # Phase 2 output (/speckit.tasks)
```

### Source Code (repository root)

```text
internal/
├── knowledge/
│   ├── embedder.go      # EmbedJob, getEmbedding, EnsureModel (modify: endpoint fallback, bounded wait)
│   ├── retriever.go     # FindSimilar/ForRepo (verify empty-result handling)
│   ├── tracker.go       # TrackUsage (no change expected)
│   └── writer.go        # WriteKnowledgeFile (no change expected)
├── job/
│   └── runner.go        # knowledge lookup/injection + post-completion embed (verify degradation)
├── storage/
│   └── db.go            # schema (no change)
├── docker/
│   └── manager.go       # EnsureOllamaContainer (host side)
└── config/
    └── config.go        # OllamaURL default
cmd/
├── start.go             # host-side: ensure ollama container + pull model
└── akama-daemon/main.go # daemon: EnsureModel goroutine, env override
docker-compose.yml       # re-enable ollama service, set OLLAMA_URL
entrypoint.sh            # write ollama_url into config.yaml
```

**Structure Decision**: Existing single-project Go layout; changes are localized to the knowledge, job, docker, config, and deployment files above. No new packages.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Constitution V (SQLite-only) vs. actual Postgres | Feature depends on existing pgvector infrastructure | Migrating to SQLite would require reimplementing vector search; out of scope and would break existing deployments |