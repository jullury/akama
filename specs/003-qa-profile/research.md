# Research: QA Profile

## Decisions

### D1 — Detecting a blocking defect from QA output

- **Decision**: The QA profile's output contract requires a final line `Verdict: PASS` or `Verdict: FAIL` (optionally `Verdict: BLOCKING`). The runner detects failure by scanning the QA output for the last `Verdict:` line and treating `FAIL`/`BLOCKING` as a blocking defect.
- **Rationale**: Matches the existing reviewer profile's `Verdict: APPROVE | CHANGES_REQUESTED` convention, so agents and parsers already follow the pattern. Keeps QA output human-readable markdown.
- **Alternatives considered**: JSON envelope (rejected — inconsistent with other profiles and harder for agents to emit reliably); heuristic keyword scan (rejected — too many false positives).

### D2 — Fix-and-reverify loop cap

- **Decision**: A constant `qaMaxFixIterations = 2` additional iterations. The loop re-runs implement → converge → QA while the QA verdict is FAIL and iterations remain.
- **Rationale**: Prevents unbounded agent runs and cost. 2 matches the plan-phase retry precedent (`retries < 2`).
- **Alternatives considered**: User-configurable cap (deferred — no config schema change now; constant keeps scope minimal and can be promoted later).

### D3 — Persisting loop progress across interrupts

- **Decision**: Store the iteration counter in the existing `profile_outputs` JSON map under the reserved key `qa_fix_iterations` (string value). No database schema change.
- **Rationale**: Consistent with how every other phase persists state; avoids a migration and keeps resume logic uniform.
- **Alternatives considered**: New `jobs.qa_fix_iterations` column (rejected — unnecessary schema change for transient runtime state; constitution requires migrate() bump).

### D4 — QA placement and ordering

- **Decision**: Insert QA after converge and before reviewer in `ProfileOrder` and in both runner flows (`runJob`, `RunGrouped`).
- **Rationale**: Code must exist and be reconciled before verification; review remains the final static gate afterward.
- **Alternatives considered**: Before converge (rejected — converge reconciles half-finished edits that QA would otherwise flag); after reviewer (rejected — review would comment on unverified code).

### D5 — Test modifications by QA

- **Decision**: QA may add or modify automated tests only; it MUST NOT change product behavior. Product fixes go through implement (including the fix loop).
- **Rationale**: Lets QA exercise acceptance scenarios that lack coverage, while preserving a single owner for product changes.

### D6 — Grouped (multi-repo) flow

- **Decision**: QA runs once on the shared workspace (like converge), producing shared output stored on the first job; the fix loop re-runs the multi-repo implement block + converge + QA. The QA report is posted as a single comment on the issue.
- **Rationale**: Matches how converge and review already operate in grouped flows.