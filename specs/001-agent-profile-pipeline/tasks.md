# Tasks: Agent Profile Pipeline

**Input**: Design documents from `/specs/001-agent-profile-pipeline/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md

## Phase 1: Setup

- [x] T001 Verify build gate passes: `go build ./...`

## Phase 2: Foundational

- [x] T002 Add `profile_outputs` column and migration in `internal/storage/db.go`
- [x] T003 Add `ProfileOutputs` field and `SetJobProfileOutputs` in `internal/storage/jobs.go`

## Phase 3: User Story 1 - Run full agent profile pipeline per job (Priority: P1)

**Goal**: Execute spec/plan/task/analyze/implement/converge/reviewer profiles in order per job.
**Independent Test**: Completed job has all seven keys in `profile_outputs`.

- [x] T004 [US1] Create `internal/agent/profiles.go` with Profile type, constants, ProfilePrompt, RunProfileAgent, ProfileOutputs
- [x] T005 [US1] Integrate pre-implementation profiles into `runJob` in `internal/job/runner.go`
- [x] T006 [US1] Run implement phase and store output in `internal/job/runner.go`
- [x] T007 [US1] Run converge and reviewer phases in `internal/job/runner.go`
- [x] T008 [US1] Persist reviewer output as PR comment in `internal/job/runner.go`

## Phase 4: User Story 2 - Inspect phase outputs (Priority: P2)

**Goal**: Operators can inspect each phase output.
**Independent Test**: Query job and confirm seven keys.

- [x] T009 [US2] Ensure `SetJobProfileOutputs` called after each phase in `internal/job/runner.go`

## Phase 5: User Story 3 - Configure profile agent/model (Priority: P3)

**Goal**: Reuse existing user/connection agent/model config for all phases.
**Independent Test**: Set config and confirm all phases use it.

- [x] T010 [US3] Pass `j.Agent`/`j.AgentModel` to every `RunProfileAgent` call in `internal/job/runner.go`

## Final Phase: Polish

- [x] T011 Update docs (AGENTS.md/CLAUDE.md) if needed
- [x] T012 Add/verify parallel-job safety: per-job `profile_outputs` isolation and no shared mutable state in `internal/job/runner.go`
- [x] T013 Re-run `go build ./...` and verify no regressions