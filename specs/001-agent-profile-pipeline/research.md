# Research: Agent Profile Pipeline

## Decisions

- **Profile set**: spec, plan, task, analyze, implement, converge, reviewer (fixed order). Rationale: matches spec-driven development flow already partially present (plan phase).
- **Storage**: single `profile_outputs TEXT` JSON column rather than per-phase columns. Rationale: avoids wide table; flexible; queryable via JSON. Alternatives: 7 columns (rejected: migration churn).
- **Failure handling**: non-fatal per phase; only implement failure aborts PR creation. Rationale: partial context better than full abort.
- **Grouped jobs**: deferred to v1; keep existing single-run behavior. Rationale: multi-repo prompt assembly differs; risk reduction.
- **Parallel jobs**: supported via existing concurrency semaphore and per-job state isolation; `profile_outputs` updates are keyed by job ID, no shared mutable state.

## Alternatives considered

- Per-phase DB columns: rejected for migration complexity.
- Parallel phases: rejected; phases depend on prior outputs.