# Contract: QA Profile

## Output contract (markdown)

The QA phase produces a markdown report with these sections, ending with a verdict line:

```markdown
## Checks Performed
- Build
- Automated tests
- Acceptance scenario: <title>

## Results
- Build: PASS | FAIL
- Automated tests: PASS | FAIL | N/A (no tests)
- Acceptance scenario: <title>: PASS | FAIL

## Defects Found
- <defect description> (file/line if known)

## Verdict
Verdict: PASS
```

The final non-empty line's verdict token (`PASS`/`FAIL`/`BLOCKING`) is the machine-readable signal.

## Verdict parsing (runner)

- Scan the QA output for the last line matching `Verdict:` (case-insensitive).
- Token after the colon, trimmed, uppercased.
- `FAIL` or `BLOCKING` → blocking defect → triggers fix loop or job failure.
- `PASS` or absent → proceed.

## Fix-loop contract

- Trigger: QA verdict is `FAIL`/`BLOCKING`.
- Action: relay QA report to implement phase via prior outputs; re-run implement → converge → QA.
- Cap: `qaMaxFixIterations` (default 2) additional iterations.
- On cap exceeded or final failure: fail job without PR; surface accumulated QA findings via notification and (where applicable) issue comment.

## Notification contract

- Start: `🤖 [<provider>] running: QA (Quality Assurance)` (same cadence as other non-implement phases).
- Completion: QA report posted as issue/PR comment when non-empty (prefix `## QA Report\n\n`), consistent with review posting.