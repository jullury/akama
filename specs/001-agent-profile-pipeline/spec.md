# Feature Specification: Agent Profile Pipeline

**Feature Branch**: `001-agent-profile-pipeline`
**Created**: 2026-10-18
**Status**: Draft
**Input**: User description: "implement agent profile for each task: spec, plan, tast, analyze, implement, converge, reviewer"

## Clarifications

### Session 2026-10-18

- Q: Should the schema and workflow support parallel jobs? → A: Yes, concurrent jobs must be supported without cross-job interference.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Run a full agent profile pipeline per job (Priority: P1)

As an operator, when a job is created from an issue, the system runs a sequence of agent profiles (spec, plan, task, analyze, implement, converge, reviewer) in order, storing each phase's output, so the final implementation is informed by structured spec/plan/task/analysis and reviewed before PR creation.

**Why this priority**: This is the core value — replacing the single undifferentiated agent run with a structured, reviewable pipeline.

**Independent Test**: Create a job from a small issue and verify that all seven phase outputs are persisted on the job and the reviewer verdict is included in the PR comment.

**Acceptance Scenarios**:

1. **Given** a pending job, **When** it runs, **Then** each of the seven profiles executes in order and its output is stored.
2. **Given** a phase fails, **When** the pipeline continues, **Then** the failure is logged and recorded, and subsequent phases still run with available context.
3. **Given** the reviewer profile returns CHANGES_REQUESTED, **When** the pipeline finishes, **Then** the review notes are posted as a comment on the PR.

### User Story 2 - Inspect phase outputs (Priority: P2)

As an operator, I can view each phase's output for a job (via /logs or DB) to understand decisions and debug failures.

**Why this priority**: Observability for the multi-phase pipeline.

**Independent Test**: Query a completed job and confirm `profile_outputs` contains all seven keys.

**Acceptance Scenarios**:

1. **Given** a completed job, **When** inspecting `profile_outputs`, **Then** keys spec/plan/task/analyze/implement/converge/reviewer are present.

### User Story 3 - Configure profile agent/model (Priority: P3)

As an operator, I can override which agent/model is used for the whole pipeline via existing user config.

**Why this priority**: Reuses existing config; low risk.

**Independent Test**: Set agent/model in user config and confirm pipeline uses them for every phase.

**Acceptance Scenarios**:

1. **Given** a user config with agent/model, **When** a job runs, **Then** every profile phase uses that agent/model.

### Edge Cases

- What happens when a phase produces no output? The pipeline records an empty string and continues.
- How does system handle a phase timeout? The phase is marked failed and the pipeline continues with available context.
- What happens on grouped (multi-repo) jobs? The grouped path keeps the existing single-run behavior for v1; pipeline integration is deferred.
- How does the system handle concurrent jobs? Each job operates on its own workspace and `profile_outputs` row; no shared mutable state between jobs.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST define a named Profile type with constants for spec, plan, task, analyze, implement, converge, reviewer.
- **FR-002**: System MUST execute the seven profiles in the fixed order: spec → plan → task → analyze → implement → converge → reviewer.
- **FR-003**: System MUST persist each phase output in a `profile_outputs` JSON column on the job.
- **FR-004**: System MUST pass accumulated prior-phase outputs into each subsequent phase's prompt.
- **FR-005**: System MUST continue the pipeline when an individual phase fails, recording the error and continuing.
- **FR-006**: System MUST post the reviewer phase output as a comment on the created PR.
- **FR-007**: System MUST keep the existing commit/push/PR creation flow after the implement phase.
- **FR-008**: System MUST reuse the existing agent/model selection from user and connection config for every phase.
- **FR-009**: System MUST migrate existing databases to add the `profile_outputs` column.
- **FR-010**: System MUST expose `agent.ProfilePrompt` and `agent.RunProfileAgent` helpers.
- **FR-011**: System MUST support parallel job execution: concurrent jobs MUST NOT interfere with each other's `profile_outputs` or workspace state; per-job state updates are isolated by job ID.

### Key Entities *(include if feature involves data)*

- **Profile**: Named agent role (spec, plan, task, analyze, implement, converge, reviewer).
- **ProfileOutputs**: Map of profile name to agent output text, stored as JSON on a Job.
- **Job**: Extended with `profile_outputs` column.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A completed job has all seven profile keys in `profile_outputs`.
- **SC-002**: Phase failures do not abort the pipeline; 100% of jobs reach PR creation when implement succeeds.
- **SC-003**: Reviewer output is posted as a PR comment in 100% of successful runs.
- **SC-004**: No regression in existing single-job execution path (build passes).

## Assumptions

- The pipeline runs all phases sequentially in one job; no parallelism between phases.
- Grouped (multi-repo) jobs retain existing behavior in v1.
- Phase prompts are built-in templates; user customization is out of scope for v1.
- A phase failure is non-fatal; only implement-phase failure aborts PR creation.