# Research: Fix Ollama Knowledge Base Flow

## Decisions

### D1: Ollama embedding endpoint compatibility
- **Decision**: Support both `/api/embed` (newer) and `/api/embeddings` (legacy) in `getEmbedding`, trying the newer first and falling back to legacy on 404/error.
- **Rationale**: Deployed Ollama versions vary; a single endpoint risks breakage. Fallback maximizes compatibility at small cost.
- **Alternatives considered**: Single legacy endpoint (current code) — rejected because newer Ollama deprecates `/api/embeddings`; single new endpoint — rejected for older deployments.

### D2: Model pull timing
- **Decision**: Pull in a background goroutine at daemon startup; daemon starts immediately. Reachability wait is bounded to 60 seconds; the pull itself runs until completion or context cancellation.
- **Rationale**: Keeps startup fast; first job degrades gracefully if model not yet present; respects variable network speeds for pulling.
- **Alternatives considered**: Blocking startup until pulled — rejected (delays startup by minutes); no pull at all — rejected (knowledge never works).

### D3: Ollama as required service in Docker Compose
- **Decision**: Re-enable the `ollama` service in the default compose stack and set `OLLAMA_URL` to the internal URL (`http://ollama:11434`).
- **Rationale**: Feature goal is out-of-the-box working knowledge flow; optional profile would leave default broken.
- **Alternatives considered**: Optional profile — rejected per clarification.

### D4: Clean disabled state
- **Decision**: When `OLLAMA_URL` is empty, log a single line disabling knowledge flow and skip all lookups/embeddings without repeated errors.
- **Rationale**: Avoids log spam and confusion; matches graceful-degradation requirement.
- **Alternatives considered**: Repeated per-job warnings — rejected (noise).

### D5: Empty result handling
- **Decision**: `FindSimilarForRepo` returns empty slice (not error) when no embeddings exist or query embedding fails recoverably; log and continue.
- **Rationale**: Retrieval is an enhancement; must not fail jobs.
- **Alternatives considered**: Propagating errors — rejected (breaks job flow).

## Resolved Unknowns

All `NEEDS CLARIFICATION` items from the spec's Technical Context are resolved via clarifications recorded in `spec.md` (Session 2026-10-18). No outstanding unknowns.