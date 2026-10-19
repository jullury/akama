# Tasks: QA Profile

**Input**: Design documents from `/specs/003-qa-profile/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Unit tests included for the new verdict parser (per plan.md); no integration test harness exists in this project.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- Single Go module at repository root
- Agent profiles: `internal/agent/profiles.go`
- Job runners: `internal/job/runner.go`
- Agent tests: `internal/agent/runner_test.go`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Confirm baseline before making changes.

- [x] T001 Verify baseline build passes with `go build ./...` from repository root

**Checkpoint**: Baseline green before edits.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core profile definitions that ALL user stories depend on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [x] T002 Add `ProfileQA Profile = "qa"` constant and insert `ProfileQA` into `ProfileOrder` between `ProfileConverge` and `ProfileReviewer` in `internal/agent/profiles.go`
- [x] T003 Add `case ProfileQA` to `ProfilePrompt` returning the QA role, execution flow, output contract (Checks Performed / Results / Defects Found / `Verdict: PASS|FAIL`), constraints (verification-only; may add/modify tests, never product code), and instructions to detect build/test tooling from repo files and to record unautomatable acceptance scenarios as "Manual/Not automated" in `internal/agent/profiles.go`
- [x] T004 Add `case ProfileQA` to `DisplayName` returning `"QA (Quality Assurance)"` in `internal/agent/profiles.go`
- [x] T005 Add exported helper `IsQAFailure(output string) bool` that scans the last `Verdict:` line and returns true for `FAIL`/`BLOCKING` in `internal/agent/profiles.go`
- [x] T006 Add exported constant `QAMaxFixIterations = 2` in `internal/agent/profiles.go`
- [x] T007 Add unit tests for `IsQAFailure` (PASS, FAIL, BLOCKING, missing verdict, empty input) in `internal/agent/runner_test.go`

**Checkpoint**: Profile defined and tested; runner wiring can begin.

---

## Phase 3: User Story 1 - QA gate verifies implementation before PR (Priority: P1) 🎯 MVP

**Goal**: A QA phase runs after converge and before review in both single and grouped flows, with a bounded fix-and-reverify loop on blocking defects; the job does not silently proceed to PR creation on failure.

**Independent Test**: Submit an issue; observe `QA (Quality Assurance)` phase notification between converge and review; confirm PR is created only after QA passes (or job fails after the iteration cap).

### Implementation for User Story 1

- [x] T008 [US1] Wire the QA phase into `runJob` in `internal/job/runner.go`: after the converge block, send the standard phase-start notification, run `agent.ProfileQA` via `runProfileWithHeartbeat`, store output in `profileOutputs`/`priorOutputs`, persist via `storage.SetJobProfileOutputs`, guarded by presence check
- [x] T009 [US1] Add the fix-and-reverify loop to `runJob` after the QA block: while `agent.IsQAFailure(qaOutput)` and `qa_fix_iterations < agent.QAMaxFixIterations`, increment and persist the counter in `profileOutputs["qa_fix_iterations"]`, re-run implement (repeating the question-check and updating `agentText`) → converge → QA (overwriting outputs and appending to `priorOutputs`); on cap exceeded or final failure call `failJob` and return before PR creation; send an explicit completion notification when the report is empty
- [x] T010 [US1] Wire the QA phase into `RunGrouped` in `internal/job/runner.go`: after the converge block, send the standard phase-start notification, run `agent.ProfileQA` on the shared workspace, store output on the first job, guarded by presence check
- [x] T011 [US1] Add the fix-and-reverify loop to `RunGrouped` after the QA block, re-running implement → converge → QA, with the same cap and failure handling as T009, including question-check

**Checkpoint**: QA gate functional in both flows; fix loop bounded.

---

## Phase 4: User Story 2 - QA report available to operators (Priority: P2)

**Goal**: QA findings are posted alongside the review so operators see what was verified.

**Independent Test**: Complete a job and confirm a `## QA Report` comment appears on the issue/PR when the report is non-empty, and no empty comment is posted.

### Implementation for User Story 2

- [x] T013 [US2] In `runJob` (`internal/job/runner.go`), after the QA phase (and fix loop) completes successfully, post the QA report as an issue comment with prefix `## QA Report\n\n` when non-empty (mirroring the review comment posting) and send an explicit completion notification when the report is empty
- [x] T014 [US2] In `RunGrouped` (`internal/job/runner.go`), post the shared QA report as a single issue comment with the same prefix when non-empty, and send an explicit completion notification when empty

**Checkpoint**: QA report visible to operators.

---

## Phase 5: User Story 3 - Interrupted jobs resume at QA (Priority: P3)

**Goal**: Interrupted jobs resume at QA (or the fix loop) without re-running earlier phases, consistent with other phases.

**Independent Test**: Interrupt a job during QA, restart, confirm earlier phases are reused and QA re-runs; after a passing QA the job completes.

### Implementation for User Story 3

- [x] T016 [US3] In both `runJob` and `RunGrouped` (`internal/job/runner.go`), ensure the phase-skip/resume logic treats QA like other phases: skip when present and verdict is PASS; re-enter the fix loop when QA is present but verdict is FAIL and the iteration cap is not exhausted
- [x] T017 [US3] Ensure `qa_fix_iterations` is read from `profileOutputs` on resume so the cap is honored across restarts in both flows

**Checkpoint**: Resume behavior consistent with existing phases.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final verification.

- [x] T019 Run `go build ./...` and confirm zero errors
- [x] T020 Run the unit tests in `internal/agent` (`go test ./internal/agent/...`) and confirm pass
- [ ] T021 Validate against `specs/003-qa-profile/quickstart.md` scenarios (manual/agent run)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies.
- **Foundational (Phase 2)**: Depends on Phase 1; BLOCKS all user stories.
- **User Stories (Phase 3+)**: All depend on Phase 2 completion.
  - US1 (P1) → US2 (P2) → US3 (P3) sequentially (US2/US3 build on US1 wiring).
- **Polish (Phase 6)**: Depends on all stories complete.

### Within Each User Story

- Foundational profile definitions before runner wiring.
- Runner wiring before comment posting (US2) and resume logic (US3).
- Build verification after each story.

### Parallel Opportunities

- T002–T006 are sequential (same file).
- T007 can run in parallel with T002–T006 once `IsQAFailure` signature is settled (different file), but depends on T005; sequence after T005.
- T008 and T010 (single vs grouped wiring) touch the same file (`runner.go`) → sequential.

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 + Phase 2.
2. Complete Phase 3 (US1) — QA gate + fix loop in both flows.
3. **STOP and VALIDATE**: build passes; manual run shows QA phase.
4. Proceed to US2/US3.

### Incremental Delivery

1. Setup + Foundational → profile defined.
2. US1 → QA gate functional.
3. US2 → report visible.
4. US3 → resume safe.
5. Polish → final verification.
## Phase 7: Convergence

- [x] T022 Check error from `runProfileWithHeartbeat` in the initial QA phase block in `runJob` and `RunGrouped` (`internal/job/runner.go`); on error call `failJob`/`failGroupedJob` and return before PR creation (contradicts, spec Edge Cases)
- [x] T023 Check errors from `runProfileWithHeartbeat` in fix-loop converge and QA re-runs in both flows (`internal/job/runner.go`); on error fail the job and return (contradicts, spec Edge Cases)
- [x] T024 Send an explicit completion notification when the QA report is empty in `runJob` and `RunGrouped` (`internal/job/runner.go`) (partial, FR-007)
- [x] T025 Re-run implement in the grouped fix loop with multi-repo context appended to the prompt (`internal/job/runner.go`) (partial, plan D6)
- [x] T026 Extract the grouped implement block into a reusable `runGroupedImplement()` closure, used by both the first pass and the fix loop (`internal/job/runner.go`) (partial, plan Implementation Notes)
