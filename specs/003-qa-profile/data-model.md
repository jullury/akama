# Data Model: QA Profile

## Entities

### Profile (enum, existing)

Extended with one new value:

- `qa` — "QA (Quality Assurance)" — active verification role.

Pipeline order becomes: spec → plan → task → analyze → implement → converge → **qa** → reviewer.

### QA Report (artifact)

A markdown artifact produced by the QA phase and stored as the value of `profile_outputs["qa"]`.

Required sections:
- **Checks Performed** — list of checks (build, tests, acceptance scenarios).
- **Results** — pass/fail per check.
- **Defects Found** — list (may be empty).
- **Verdict** — final line `Verdict: PASS` or `Verdict: FAIL` (or `BLOCKING`).

### Fix Loop State (transient)

Stored inside the existing `profile_outputs` JSON map under the reserved key `qa_fix_iterations` (string-encoded integer). Counts how many fix iterations have been attempted. Absent/empty means 0.

## State transitions

1. QA runs → produces QA Report.
2. If verdict is PASS → proceed to reviewer phase.
3. If verdict is FAIL/BLOCKING and `qa_fix_iterations < qaMaxFixIterations`:
   - increment counter, persist
   - re-run implement (with QA findings in prior outputs)
   - re-run converge
   - re-run QA (overwrite report)
   - repeat from step 2
4. If verdict is FAIL/BLOCKING and counter reached cap → job fails without PR; accumulated findings surfaced.

## Persistence

- All phase outputs (including QA and the counter) persisted via the existing `storage.SetJobProfileOutputs` after each phase, enabling resume.
- No database schema change.