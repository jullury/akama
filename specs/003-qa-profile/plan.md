# Implementation Plan: QA Profile

**Branch**: `003-qa-profile` | **Date**: 2026-10-19 | **Spec**: `/specs/003-qa-profile/spec.md`

**Input**: Feature specification from `/specs/003-qa-profile/spec.md`

## Summary

Add a new `qa` profile to the agent profile pipeline, positioned after converge and before reviewer. The QA phase actively verifies the implemented change (build, automated tests, acceptance scenarios) and produces a structured QA report. On a blocking defect, findings are relayed to the implement phase in a bounded fix-and-reverify loop (max 2 additional iterations) before failing the job. QA may add/modify tests but never product code. Integrates into both single (`runJob`) and grouped (`RunGrouped`) flows, reusing the existing checkpoint, notification, and heartbeat machinery.

## Technical Context

**Language/Version**: Go 1.26.1 (module `github.com/jullury/akama`)
**Primary Dependencies**: cobra, go-telegram-bot-api, modernc.org/sqlite (daemon); agent CLIs (claude, opencode) invoked as subprocesses
**Storage**: SQLite, schema owned by `internal/storage/db.go` `migrate()`; no schema change for this feature (counter stored in `profile_outputs` JSON)
**Testing**: No test suite; mandatory gate is `go build ./...`. Existing `internal/agent/runner_test.go` covers `IsValidPlan`; add unit tests for the new verdict parser
**Target Platform**: Linux daemon in Docker (non-root `akama` user); macOS/Windows host CLI
**Project Type**: Two binaries (`akama` host CLI, `akama-daemon` in-container)
**Performance Goals**: N/A (agent runs dominate; QA adds one agent invocation per phase/iteration)
**Constraints**: Telegram 4096-char message cap (chunk at 4000 via `notifyChunked`); agent output formats differ (claude JSON envelope vs opencode NDJSON); no plan mode; profile phases must be checkpointed/resumable
**Scale/Scope**: Single feature — one new profile constant, one prompt case, one display name, one helper, integration in two runner functions, one new test file addition

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. Two-Binary Architecture**: Changes confined to `internal/agent` and `internal/job` (daemon-side). Host CLI untouched. PASS.
- **II. Build-Time Secret Injection**: No secrets involved. PASS.
- **III. Conventional Commits**: Commit will follow `feat:` prefix. PASS (process).
- **IV. Build Verification Gate**: Must run `go build ./...` after changes. PASS (will verify).
- **V. SQLite Schema Ownership**: No schema change; counter stored in `profile_outputs` JSON. PASS.
- **VI. Daemon Lifecycle Safety**: No changes to PID/lifecycle. PASS.
- **VII. Security Defaults**: No changes. PASS.

**Result**: All gates pass; no violations to justify.

## Project Structure

### Documentation (this feature)

```text
specs/003-qa-profile/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── qa-profile.md
└── tasks.md          # Phase 2 output (/speckit.tasks — not created here)
```

### Source Code (repository root)

```text
internal/
├── agent/
│   ├── profiles.go        # add ProfileQA, ProfileOrder entry, prompt case, display name, IsQAFailure helper
│   └── runner_test.go     # add TestIsQAFailure
└── job/
    └── runner.go          # runJob + RunGrouped: QA phase + fix loop + comment posting
```

**Structure Decision**: Single Go module; changes are localized to the agent profile definitions and the two job runner functions, mirroring how converge/reviewer are already wired. No new packages.

## Implementation Notes

- The grouped implement block should be extracted into a reusable `runImplement()` closure invoked both initially and by the fix loop, to avoid duplicating ~120 lines.
- Fix-loop implement re-runs must repeat the agent question check (`IsQuestion`) and pause logic, and update `agentText`/`priorOutputs`.
- Stale failed QA reports may remain in `priorOutputs` across iterations (acceptable; provides reviewer context).

## Complexity Tracking

> No constitution violations; nothing to justify.