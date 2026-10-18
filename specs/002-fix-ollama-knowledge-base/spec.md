# Feature Specification: Fix Ollama Knowledge Base Flow

**Feature Branch**: `002-fix-ollama-knowledge-base`

**Created**: 2026-10-18

**Status**: Draft

**Input**: User description: "fix the ollama knowledge base flow, it is not working currently"

## Clarifications

### Session 2026-10-18

- Q: Should the embedding client support both the legacy Ollama `/api/embeddings` endpoint and the newer `/api/embed` endpoint, or only the endpoint matching the Ollama version shipped in this project? → A: Support both endpoints with fallback (try `/api/embed` first, then `/api/embeddings`).
- Q: Should the daemon block startup until the embedding model is pulled, or pull it in the background while allowing the daemon to start immediately? → A: Pull in background; daemon starts immediately.
- Q: Should the Ollama service be a required part of the default Docker Compose stack, or an optional profile that operators enable explicitly? → A: Required in default stack (always starts).
- Q: What maximum wait duration should `EnsureModel` use while waiting for Ollama to become reachable before giving up? → A: Reachability wait bounded to 60 seconds; model pull runs until completion or context cancellation (pull time varies by network).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Knowledge base is reachable at runtime (Priority: P1)

As an operator running akama (host or Docker), I want the Ollama embedding service to be available and configured so that the knowledge base flow can run instead of silently no-oping.

**Why this priority**: Without a reachable Ollama endpoint, every knowledge lookup and embedding fails, so prior-art retrieval never happens. This is the foundational blocker.

**Independent Test**: Start akama in a supported deployment mode, confirm the daemon reports Ollama reachable and the embedding model present, and verify a job triggers a knowledge lookup without a connection error.

**Acceptance Scenarios**:

1. **Given** a fresh Docker Compose deployment, **When** the stack starts, **Then** an Ollama container is running and reachable by the daemon at the configured internal URL.
2. **Given** the daemon starts, **When** it initializes, **Then** the embedding model is ensured (pulled if missing) and the daemon logs Ollama as reachable.
3. **Given** Ollama is reachable, **When** a job runs, **Then** the knowledge lookup does not log a connection error and either returns similar jobs or an empty result set.

### User Story 2 - Knowledge is actually retrieved and injected (Priority: P1)

As a developer fixing an issue, I want relevant prior-art from similar resolved jobs to be written into my workspace and referenced by the agent prompt, so the fix benefits from past resolutions.

**Why this priority**: Even with Ollama reachable, the flow must produce and inject the knowledge file; otherwise the feature delivers no value.

**Independent Test**: Run a job in an environment with at least one previously embedded completed job in the same repo, confirm `.akama-knowledge.md` is created in the workspace and the agent prompt references it.

**Acceptance Scenarios**:

1. **Given** completed jobs with embeddings exist for the same repository, **When** a new job starts, **Then** a knowledge file is written to the workspace containing prior-art entries.
2. **Given** a knowledge file is written, **When** the agent prompt is built, **Then** the prompt references the knowledge file path so the agent can read it.
3. **Given** no similar jobs exist, **When** a job starts, **Then** no knowledge file is created and the prompt omits the prior-art reference without error.

### User Story 3 - Completed jobs are embedded for future reuse (Priority: P2)

As the system, I want each completed job to be embedded and stored so future jobs can retrieve it as prior art.

**Why this priority**: Without embedding completed jobs, the knowledge base never grows and retrieval returns nothing forever.

**Independent Test**: Complete a job successfully, then verify an embedding row exists for that job and a subsequent similar job retrieves it.

**Acceptance Scenarios**:

1. **Given** a job completes successfully, **When** the post-completion embedding step runs, **Then** an embedding is stored for that job.
2. **Given** an embedding is stored, **When** a later job with similar text runs, **Then** the earlier job appears in the similar-jobs result set.

### User Story 4 - Graceful degradation when Ollama is unavailable (Priority: P2)

As an operator, I want the job flow to continue normally when Ollama is unavailable, with clear logging, rather than failing or hanging.

**Why this priority**: Knowledge is an enhancement, not a blocker; the core fix flow must never break because of it.

**Independent Test**: Disable Ollama, run a job end-to-end, confirm the job still completes and knowledge-related failures are logged at most as warnings.

**Acceptance Scenarios**:

1. **Given** Ollama is unreachable, **When** a job runs, **Then** the job proceeds without a knowledge file and completes normally.
2. **Given** Ollama is unreachable, **When** embedding is attempted, **Then** the failure is logged and does not abort the job.
3. **Given** Ollama is unreachable, **When** `EnsureModel` runs, **Then** it does not block daemon startup indefinitely (bounded wait).

### Edge Cases

- What happens when the embedding model is not yet pulled when the first job runs? The flow should wait or skip gracefully, not crash.
- How does the system handle an Ollama endpoint that returns an unexpected response shape (e.g., newer `/api/embed` format)?
- What happens when the knowledge file write fails (read-only workspace)? The job should continue without prior art.
- What happens when `job_embeddings` table is empty? Retrieval returns empty, no error.
- What happens when Ollama URL is empty/unconfigured? Knowledge flow is disabled entirely with a single clear log line, not repeated errors.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The Docker Compose stack MUST include an Ollama service reachable by the daemon container, and the daemon MUST be configured with its internal URL.
- **FR-002**: The container entrypoint/config generation MUST write the Ollama URL into the daemon config so it is not left at an incorrect default.
- **FR-003**: The daemon MUST verify Ollama reachability at startup (bounded wait of 60 seconds) and pull the embedding model if missing in the background, so startup is not blocked and pull time may vary by network.
- **FR-004**: The daemon MUST disable the knowledge flow cleanly (single log line, no repeated errors) when Ollama is unconfigured.
- **FR-005**: Job execution MUST perform a knowledge lookup when Ollama is configured and inject the resulting file reference into the agent prompt.
- **FR-006**: Job execution MUST write the prior-art knowledge file into the workspace when similar jobs are found.
- **FR-007**: Job execution MUST NOT fail or abort when the knowledge lookup, file write, or embedding step fails; failures MUST be logged.
- **FR-008**: On successful job completion, the system MUST embed and store the job for future retrieval.
- **FR-009**: The embedding client MUST support both the legacy `/api/embeddings` endpoint and the newer `/api/embed` endpoint, trying the newer first and falling back to the legacy endpoint.
- **FR-010**: The retrieval query MUST return an empty result (not an error) when no embeddings exist.
- **FR-011**: Knowledge usage tracking MUST record which jobs used prior art and how many similar jobs were found.
- **FR-012**: The host-side (`akama start`) flow MUST continue to start Ollama and pull the embedding model as it does today.

### Key Entities

- **Job Embedding**: A vector representation of a completed job's issue/plan/output, stored per job, used for similarity search.
- **Knowledge File**: A markdown file written into a job's workspace summarizing prior-art from similar jobs.
- **Knowledge Usage Record**: A tracking entry linking a job to the knowledge file and similar job IDs it used.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In a fresh Docker Compose deployment, 100% of daemon startups reach a reachable Ollama endpoint (no connection-refused errors in logs) when the Ollama service is enabled.
- **SC-002**: At least 90% of jobs in repositories with prior completed jobs retrieve and inject prior-art (knowledge file present in workspace).
- **SC-003**: Zero jobs fail solely due to a knowledge-base or Ollama error.
- **SC-004**: Every successfully completed job has an embedding stored within 60 seconds of completion.
- **SC-005**: When Ollama is disabled, job end-to-end duration and success rate are unchanged versus a run with Ollama disabled prior to the fix.

## Assumptions

- Ollama remains the embedding provider; no alternative embedding backend is introduced in this fix.
- The embedding model remains `nomic-embed-text` (768 dimensions), matching the `vector(768)` column.
- Docker Compose is the primary broken deployment path; the host-side path already starts Ollama but may share config gaps.
- Existing completed jobs without embeddings are not backfilled; only new completions are embedded.
- The fix preserves backward compatibility with existing databases (no destructive migrations).