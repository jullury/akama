# Tasks: Fix Ollama Knowledge Base Flow

**Input**: Design documents from `/specs/002-fix-ollama-knowledge-base/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: Not requested; verification gate is `go build ./...`.

**Organization**: Tasks grouped by user story for independent implementation.

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Restore Ollama in the containerized deployment and wire its URL into daemon config.

- [x] T001 Re-enable the `ollama` service in `docker-compose.yml` (image `docker.io/ollama/ollama:latest`, volume `akama-ollama`, healthcheck) and set `OLLAMA_URL=http://ollama:11434` in the `akama` service environment.
- [x] T002 Update `entrypoint.sh` generated `config.yaml` to include `ollama_url: "${OLLAMA_URL:-http://localhost:11434}"`.
- [x] T003 [P] Verify host-side flow in `cmd/start.go` starts the Ollama container and pulls `nomic-embed-text` via `ensureOllamaModels` (no code change expected; confirm only).

**Checkpoint**: Compose stack includes a reachable Ollama service.

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core knowledge reliability improvements required by all user stories.

- [x] T004 Bound the reachability wait in `internal/knowledge/embedder.go` `EnsureModel` to 60 seconds (loop with 2s sleeps, exit on timeout with a clear log line); keep model pull in background.
- [x] T005 [P] Update `getEmbedding` in `internal/knowledge/embedder.go` to try `/api/embed` first, then fall back to `/api/embeddings`, normalizing both response shapes into `[]float32`.
- [x] T006 [P] Make `FindSimilarForRepo` in `internal/knowledge/retriever.go` return an empty slice (not error) when no embeddings exist or query embedding fails recoverably, logging a warning.
- [x] T007 Add clean disabled-state handling in `internal/job/runner.go`: when `pkgOllamaURL == ""`, log a single line and skip knowledge lookup/injection and embedding without repeated errors.
- [x] T008 Ensure `EmbedJob` in `internal/knowledge/embedder.go` uses upsert on `job_id` (already present) and logs failures without panicking.

**Checkpoint**: Foundation ready — knowledge flow degrades gracefully and supports both endpoints.

## Phase 3: User Story 1 - Knowledge base reachable at runtime (Priority: P1) 🎯 MVP

**Goal**: Ollama is available and configured in supported deployment modes.

**Independent Test**: Start the stack, confirm daemon logs "Ollama is reachable" and model pull, and a job triggers a lookup without connection error.

- [x] T009 [US1] Verify `cmd/akama-daemon/main.go` calls `knowledge.EnsureModel` in a background goroutine and overrides `cfg.OllamaURL` from `OLLAMA_URL` env (already present; confirm).
- [x] T010 [US1] Confirm daemon logs reachability and pull status lines from `EnsureModel` are emitted (no change if already present).
- [x] T011 [US1] Run `go build ./...` and `make build-daemon` to verify compilation.

**Checkpoint**: User Story 1 functional — Ollama reachable in Docker.

## Phase 4: User Story 2 - Knowledge retrieved and injected (Priority: P1)

**Goal**: Prior art is written and referenced in the agent prompt.

**Independent Test**: Run a job with existing embeddings in the same repo; confirm `.akama-knowledge.md` exists and prompt references it.

- [x] T012 [US2] Verify `internal/job/runner.go` writes the knowledge file via `knowledge.WriteKnowledgeFile` when similar jobs are found.
- [x] T013 [US2] Verify the agent prompt references `filepath.Base(knowledgePath)` in `agent.BuildPrompt` call (already present).
- [x] T014 [US2] Ensure no knowledge file is created and no prior-art reference is added when zero similar jobs are found (already handled; verify).
- [x] T015 [US2] Confirm `knowledge.TrackUsage` is called with similar job IDs (already present; verify).

**Checkpoint**: User Story 2 functional — prior art injected.

## Phase 5: User Story 3 - Completed jobs embedded for reuse (Priority: P2)

**Goal**: Every successful job is embedded for future retrieval.

**Independent Test**: Complete a job; verify an embedding row exists and a later similar job retrieves it.

- [x] T016 [US3] Verify the post-completion goroutine in `internal/job/runner.go` calls `knowledge.EmbedJob` after `SetJobPRCreated`.
- [x] T017 [US3] Ensure embedding failures are logged and do not abort the job (verify error handling in goroutine).
- [x] T018 [US3] Confirm `storage.GetJob` fetches updated job data before embedding (already present; verify).

**Checkpoint**: User Story 3 functional — knowledge base grows.

## Phase 6: User Story 4 - Graceful degradation when Ollama unavailable (Priority: P2)

**Goal**: Job flow continues normally when Ollama is down.

**Independent Test**: Disable Ollama, run a job end-to-end, confirm completion and at most warning-level logs.

- [x] T019 [US4] Verify disabled-state single log line behavior (T007) covers both lookup and embedding paths.
- [x] T020 [US4] Ensure `EnsureModel` bounded wait (T004) logs a clear timeout message and returns without blocking daemon startup.
- [x] T021 [US4] Confirm `FindSimilarForRepo` and `EmbedJob` failures in `runner.go` are logged but do not fail the job (verify error paths).

**Checkpoint**: User Story 4 functional — graceful degradation.

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Final verification and docs.

- [ ] T022 [P] Update `AGENTS.md`/`README.md` if Ollama setup steps changed (optional).
- [x] T023 Run full verification: `go build ./...`, `make build`, `make build-daemon`.
- [ ] T024 Validate against `quickstart.md` scenarios manually.

## Dependencies & Execution Order

### Phase Dependencies
- Phase 1 (Setup) → Phase 2 (Foundational) → User Stories (Phases 3-6) → Polish (Phase 7).
- User stories can proceed in parallel after Phase 2.

### Task Dependencies
- T004 blocks T019, T020.
- T005 blocks T006.
- T007 blocks T019.
- All user-story verification tasks depend on Phase 2 completion.

## Parallel Opportunities
- T001 and T003 can run in parallel (different files).
- T005 and T006 can run in parallel (different files).
- T012-T015 (US2) and T016-T018 (US3) can run in parallel after Phase 2.

## Implementation Strategy

### MVP First (User Story 1)
1. Complete Phase 1 + Phase 2.
2. Complete Phase 3 (US1).
3. Validate Ollama reachable in Docker.
4. Proceed to US2/US3/US4.

### Incremental Delivery
Each user story phase is independently verifiable via quickstart.md scenarios.