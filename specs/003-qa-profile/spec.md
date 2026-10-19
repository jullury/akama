# Feature Specification: QA Profile

**Feature Branch**: `[###-qa-profile]`

**Created**: 2026-10-19

**Status**: Draft

**Input**: User description: "add QA profile"

## Clarifications

### Session 2026-10-19

- Q: When QA finds a blocking defect, should the job fail and stop before creating a PR, or post the findings and still create the PR? → A: Fail before PR creation is the baseline, but instead of stopping outright, relay the QA findings to the implement profile so it can fix the defects, then re-run converge and QA in a bounded fix-and-reverify loop; only if the iteration cap is exceeded (or QA still fails) does the job fail without creating a PR.
- Q: May the QA phase add or modify automated tests to cover the spec's acceptance scenarios, or must it stay strictly read-only? → A: QA may add or modify automated tests only (never product code), so acceptance scenarios can be exercised; product behavior remains untouched.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - QA gate verifies implementation before PR (Priority: P1)

As an operator of the issue-to-PR pipeline, I want a dedicated QA phase to actively verify the implemented change (build, automated tests, and acceptance scenarios) before a pull request is created, so that PRs reaching me have passed an independent quality check.

**Why this priority**: This is the core value of the feature — without it, there is no QA gate at all.

**Independent Test**: Can be fully tested by submitting an issue and observing a QA phase notification plus a QA report in the job's phase history, with PR creation only occurring after QA completes.

**Acceptance Scenarios**:

1. **Given** a job has completed the implementation and converge phases, **When** the pipeline proceeds, **Then** a QA phase runs and the user is notified that QA is running.
2. **Given** QA completes successfully, **When** all checks pass, **Then** the pipeline proceeds to the review phase and PR creation, and a QA report is available in the job's phase outputs.
3. **Given** QA finds a blocking defect, **When** QA reports it, **Then** the findings are relayed to the implement phase to attempt a fix, and converge plus QA are re-run in a bounded loop; the job does not proceed to PR creation until QA passes or the iteration cap is exceeded.

---

### User Story 2 - QA report available to operators (Priority: P2)

As an operator, I want the QA findings posted alongside the review so I can see exactly what was verified and what failed.

**Why this priority**: Visibility of QA results is necessary for the gate to be actionable, but it depends on the gate existing (P1).

**Independent Test**: Can be tested by completing a job and confirming a QA report comment appears on the issue/PR in addition to the review comment.

**Acceptance Scenarios**:

1. **Given** QA produces a non-empty report, **When** the PR is created, **Then** the QA report is posted as a comment on the issue/PR.
2. **Given** QA produces an empty report, **When** the PR is created, **Then** no empty QA comment is posted.

---

### User Story 3 - Interrupted jobs resume at QA (Priority: P3)

As an operator, if a job is interrupted while QA is running, resuming the job must continue from QA without re-running earlier phases, consistent with how every other phase behaves.

**Why this priority**: Consistency and reliability of the pipeline; lower priority because it is a robustness concern rather than core value.

**Independent Test**: Can be tested by interrupting a job during QA and resuming it, then confirming earlier phase outputs are reused and QA re-runs.

**Acceptance Scenarios**:

1. **Given** a job is interrupted during the QA phase, **When** it is resumed, **Then** QA re-runs and earlier phase outputs are reused from the checkpoint.
2. **Given** a job already completed QA, **When** it is resumed, **Then** QA is not re-run.

### Edge Cases

- What happens when the target project has no automated test suite? QA reports the absence and verifies the build plus acceptance scenarios; it does not fail solely because tests are missing.
- What happens when the QA phase itself errors (for example, the agent crashes)? The job fails with a clear error, consistent with how other phases fail.
- What happens in a grouped (multi-repository) job? QA runs per repository workspace, consistent with the implementation and converge phases.
- What happens when QA cannot execute the build (missing tooling)? QA reports the limitation in its report rather than silently passing.
- What happens when QA keeps finding defects across fix iterations? After the iteration cap is exceeded, the job fails without creating a PR and surfaces all accumulated findings.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST include a QA profile as a named phase in the agent profile pipeline, with a user-friendly display name.
- **FR-002**: The QA phase MUST run after the converge phase and before the review phase in the default pipeline order.
- **FR-003**: The QA phase MUST participate in the existing checkpoint mechanism (phase outputs), including skip-when-passed and re-enter-on-failure resume semantics, so interrupted jobs resume at QA without re-running earlier phases (FR-003 governs resume participation; FR-010 governs storage of the report artifact).
- **FR-004**: The QA phase MUST produce a structured report containing: checks performed, pass/fail result per check, defects found, and an overall verdict.
- **FR-005**: The QA phase MUST verify at minimum that the project build succeeds and that available automated tests pass; the QA prompt MUST instruct the agent to detect build/test tooling from repository files (e.g., Makefile, go.mod, package.json) rather than assuming a fixed command.
- **FR-006**: The QA phase MUST verify the implementation against the acceptance scenarios derived during the specification phase. Scenarios that cannot be automated MUST be recorded in the Results section as "Manual/Not automated" with a reason, rather than silently skipped.
- **FR-007**: The system MUST notify the user when the QA phase starts (via the standard phase-start notification) and on completion (via the QA report comment, or an explicit completion notification when the report is empty), consistent with other phases.
- **FR-008**: In grouped (multi-repository) jobs, the QA phase MUST run for each repository workspace, consistent with the implementation and converge phases.
- **FR-009**: The QA phase MUST be verification-only with respect to product behavior: it MUST NOT change product behavior, but MAY add or modify automated tests to exercise the acceptance scenarios. Product code fixes are left to the implement phase (including via the fix-and-reverify loop).
- **FR-010**: The QA report MUST be persisted in the job's phase outputs alongside the other phases' outputs (storage location of the report artifact; see FR-003 for resume semantics).
- **FR-011**: The QA report SHOULD be posted as a comment on the issue/PR when non-empty, consistent with how the review output is posted.
- **FR-012**: The QA phase MUST use the same agent and model selection as the other phases of the job.
- **FR-013**: On a blocking defect, the system MUST relay the QA findings to the implement phase to attempt a fix, then re-run converge and QA, up to a bounded number of iterations (default 2); if QA still fails or the cap is exceeded, the job MUST fail without creating a PR.

### Key Entities *(include if feature involves data)*

- **QA Profile**: A named agent role in the pipeline responsible for active verification of an implemented change.
- **QA Report**: A structured artifact produced by the QA phase listing checks performed, their results, defects found, and an overall verdict.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of jobs processed after this feature include a QA phase in their phase history.
- **SC-002**: Operators receive a QA start notification for every job, within the same notification cadence as other phases.
- **SC-003**: 100% of PRs created after this feature include a QA report comment when the report is non-empty.
- **SC-004**: Interrupted jobs resume at the QA phase without re-running earlier phases, verified by reuse of prior phase outputs.
- **SC-005**: QA reports include a build result and a test result for every job where those checks are applicable.

## Assumptions

- The QA phase is positioned after converge and before review; the review phase remains the final static quality gate.
- QA is verification-only and does not modify product code; defects found are surfaced for follow-up rather than auto-fixed.
- Projects without a test suite are not failed solely for lacking tests; QA reports the absence and verifies build and acceptance scenarios.
- Grouped flows run QA per repository workspace, consistent with implementation and converge.
- Heartbeat and notification cadence for QA matches other non-implementation phases.
- QA uses the same agent/model selection as the other phases of the job.
- The fix-and-reverify loop is bounded to 2 additional iterations by default to prevent unbounded runs.
- QA may add or modify automated tests but must never change product behavior; test changes are part of verification, not implementation.
- Stale failed QA reports may remain in prior outputs across fix iterations; this is acceptable and provides context to the reviewer.