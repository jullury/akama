<!--
Sync Impact Report
==================
Version change: (unresolved template placeholder) -> 1.0.0
Modified principles: N/A (initial ratification; all principle placeholders replaced)
Added sections: Core Principles (I-VII), Additional Constraints, Development Workflow, Governance
Removed sections: none
Follow-up TODOs:
- none (RATIFICATION_DATE set to first constitution adoption date 2026-10-18)
-->

# Akama Constitution

## Core Principles

### I. Two-Binary Architecture (NON-NEGOTIABLE)

The product ships two distinct binaries with separate build targets: `akama`
(host CLI, root `main.go`, `make build`) and `akama-daemon` (in-container,
`cmd/akama-daemon/main.go`, `make build-daemon`). The host CLI MUST manage
containers, config, and init; it MUST NOT run the Telegram bot or execute jobs.
The daemon MUST run the bot and job execution; it MUST NOT perform host
container management. Cross-package calls across this boundary are forbidden
except through the documented CLI/daemon contract.

Rationale: keeps Docker lifecycle, OAuth bake-in, and runtime concerns
isolated and independently buildable.

### II. Build-Time Secret Injection (NON-NEGOTIABLE)

GitHub/GitLab OAuth client secrets MUST be injected only via `-ldflags` at
build time (`make build`, `make dist`, `make docker-build`). They MUST NOT
appear in `config.yaml`, source code, committed files, or container images'
runtime environment. `.env` is gitignored; CI supplies `OAUTH_GITHUB_*` /
`OAUTH_GITLAB_*` secrets (GitHub reserves the `GITHUB_` prefix). Runtime
config stores only user tokens obtained through device flow.

Rationale: prevents credential leakage in runtime config and source control.

### III. Conventional Commits Drive Releases (NON-NEGOTIABLE)

All commits merged to `main` MUST follow Conventional Commits
(`fix:` -> patch, `feat:` -> minor, `BREAKING CHANGE:` -> major). Releases,
version tags, and binary publication are automated by semantic-release in
`.github/workflows/release.yml`; manual version edits and ad-hoc tags are
forbidden. Commit subjects MUST be imperative and scoped where useful.

Rationale: the release pipeline parses commit prefixes to decide version bumps.

### IV. Build Verification Gate

The project has no test suite. The mandatory verification gate is a clean
`go build ./...`; release binaries additionally require `make build` and
`make build-daemon` to succeed. The daemon build MUST use `CGO_ENABLED=0`.
New code MUST compile across all packages before merge. Adding tests is
encouraged but not required by this constitution.

Rationale: matches the project's actual verification reality (no test infra).

### V. SQLite Schema Ownership (NON-NEGOTIABLE)

The canonical database schema lives inline in `internal/storage/db.go`
(`migrate()`), applied automatically by `storage.Open()`. The file
`migrations/001_akama.sql` is a stale PostgreSQL draft and MUST NOT be
executed, imported, or treated as authoritative. All schema changes MUST be
made in `migrate()` and recorded with a schema-version bump comment.

Rationale: a single source of truth prevents divergent schema definitions.

### VI. Daemon Lifecycle Safety (NON-NEGOTIABLE)

The daemon PID file MUST be claimed atomically via `O_EXCL`
(`daemon.ClaimPIDFile()`). `akama stop` MUST poll `IsProcessAlive(pid)`, not
the PID file, with a 35s timeout. On SIGTERM the daemon MUST drain in-flight
jobs through the `sync.WaitGroup` with a 30s hard timeout before removing the
PID file. A new daemon MUST NOT start while a previous one is still draining.

Rationale: prevents overlapping daemons and loss of in-flight work.

### VII. Security Defaults (NON-NEGOTIABLE)

The container MUST run as the non-root `akama` user; in-container commands
MUST use `gosu akama`. `~/.akama/config.yaml` MUST be written with mode 0600.
`GIT_ASKPASS` helper files MUST be created in the OS temp directory (never
inside a cloned workspace) with `GIT_TERMINAL_PROMPT=0`. Agent prompts and
secrets MUST NOT be logged.

Rationale: least-privilege runtime and secret hygiene.

## Additional Constraints

- **Toolchain**: Go 1.26+ and Node managed via `mise` (`.mise.toml`); `make setup`
  bootstraps both. Do not commit toolchain binaries.
- **Storage**: SQLite only; time stored as `DATETIME` text
  (`"2006-01-02 15:04:05"`). No external DB dependency in the daemon image.
- **Config paths**: `~/` prefixes in config are expanded to absolute paths at
  runtime (`expandHome()`); `Save()` persists expanded paths.
- **Telegram limits**: outbound messages MUST be chunked at 4000 chars
  (Telegram cap 4096) via `notifyChunked()`, breaking on newlines.
- **Heartbeat**: "Agent still working..." notification fires only every 5
  minutes; no notification in the first 5 minutes of a run.
- **Agent abstraction**: agent output formats differ (claude single JSON,
  opencode NDJSON); parsing MUST use the dedicated extractors
  (`extractOpencodeError()`). Branch names and commit messages are generated
  by a second agent call (`agent.GenerateSummary()`), never hardcoded.
- **Question detection**: `agent.IsQuestion()` is the sole signal for entering
  `awaiting_input` state (last non-empty line ends with `?`).
- **Workspace hygiene**: prompt file written to
  `<workspace>/.akama-prompt.txt`, appended to workspace `.gitignore`, cleaned
  up after run. `CleanOldWorkspaces` and `/done` reclaim disk.

## Development Workflow

- **Build**: use `make build` / `make build-daemon` / `make dist`; never
  invoke `go build` with hand-rolled ldflags for releases.
- **Daemon updates**: download the new binary BEFORE stopping the daemon
  (stopping PID 1 restarts the container). Applies to both CLI `akama update`
  and Telegram `/update`.
- **Container runtime**: image runs `akama-daemon` as PID 1 as user `akama`;
  `entrypoint.sh` seeds the volume binary only when the seed is strictly newer.
  Force recreation with `docker compose up -d --force-recreate`.
- **Docker exec**: all in-container commands use `gosu akama`
  (e.g. `docker exec akama gosu akama akama init`).
- **Code review**: changes MUST be reviewed against `AGENTS.md` and `CLAUDE.md`
  guidance; PRs MUST verify the build gate and lifecycle invariants.
- **Agent tooling**: agent installers live in `internal/agent/runner.go`
  (`InstallClaudeCmd`, `InstallOpencodeCmd`); invoked via `akama init` or
  `/update_agents`. There is no `install` CLI subcommand.
- **Logging**: rotating writer, daily or 10 MB trigger, 7 gzip archives.

## Governance

This constitution supersedes all other practice documents for the Akama project.
Amendments MUST be proposed via pull request, update this file, bump
`CONSTITUTION_VERSION` per the semantic rules below, and update
`LAST_AMENDED_DATE`. Breaking governance changes require a MAJOR bump and a
migration note in the PR.

Versioning policy:
- MAJOR: principle removal or backward-incompatible redefinition.
- MINOR: new principle or section, or materially expanded guidance.
- PATCH: clarification, wording, or non-semantic refinement.

Compliance review: every PR MUST be checked against the Core Principles and
Additional Constraints; reviewers MUST reject changes that violate
NON-NEGOTIABLE principles without an approved amendment. Runtime development
guidance lives in `AGENTS.md` and `CLAUDE.md`.

**Version**: 1.0.0 | **Ratified**: 2026-10-18 | **Last Amended**: 2026-10-18
