# Implementation Plan: Agent Profile Pipeline

**Branch**: `001-agent-profile-pipeline` | **Date**: 2026-10-18 | **Spec**: specs/001-agent-profile-pipeline/spec.md

**Input**: Feature specification from `/specs/001-agent-profile-pipeline/spec.md`

## Summary

Replace the single undifferentiated agent run per job with a structured, sequential agent profile pipeline (spec → plan → task → analyze → implement → converge → reviewer). Each phase runs as a distinct agent invocation with accumulated context, persists its output in a new `profile_outputs` JSON column, and the reviewer output is posted as a PR comment. Implementation already landed in `internal/agent/profiles.go`, storage, and `internal/job/runner.go`.

## Technical Context

**Language/Version**: Go 1.26+
**Primary Dependencies**: cobra, tgbotapi, pgx, modernc/sqlite (per AGENTS.md)
**Storage**: PostgreSQL (pgx) — `jobs` table
**Testing**: none (build gate `go build ./...`)
**Target Platform**: Linux container (daemon) + host CLI
**Project Type**: two-binary (host CLI + in-container daemon)
**Performance Goals**: N/A (batch per job)
**Constraints**: no test suite; build gate only; non-root worker (uid 1000); must support parallel jobs with per-job isolation
**Scale/Scope**: single-job path; grouped (multi-repo) deferred to v1

## Constitution Check

- I. Two-Binary Architecture: changes confined to daemon-side agent/job/storage packages. PASS
- II. Build-Time Secrets: untouched. PASS
- III. Conventional Commits: to be followed on commit. PASS
- IV. Build Verification Gate: `go build ./...` passes. PASS
- V. Schema Ownership: schema change made in `migrate()` + ALTER migration. PASS
- VI. Daemon Lifecycle: untouched. PASS
- VII. Security Defaults: untouched. PASS

## Project Structure

### Documentation (this feature)

```text
specs/001-agent-profile-pipeline/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
└── tasks.md
```

### Source Code (repository root)

```text
internal/agent/profiles.go        # new: Profile type, prompts, runner
internal/storage/db.go            # profile_outputs column + migration
internal/storage/jobs.go          # Job.ProfileOutputs + SetJobProfileOutputs
internal/job/runner.go            # pipeline integration in runJob
```

**Structure Decision**: Single Go module; changes localized to existing packages. No new packages beyond `agent` helpers.

## Complexity Tracking

No constitution violations.