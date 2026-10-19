# Quickstart: QA Profile

## Prerequisites

- Build succeeds: `go build ./...`
- A configured agent (claude or opencode) and a test repository with an open issue.

## Validation scenarios

1. **Happy path**: Submit an issue that the agent can fix cleanly. Observe phases run in order including `QA (Quality Assurance)` between converge and review. Confirm a `## QA Report` comment is posted and the PR is created only after QA passes.

2. **No test suite**: Use a repo with no tests. QA report should show `Automated tests: N/A (no tests)` and still pass on build + acceptance scenarios; job should not fail solely for missing tests.

3. **Fix loop**: Inject a defect the implement phase initially misses (e.g., a failing test). Confirm QA reports FAIL, implement is re-invoked with the findings, and QA passes on the second iteration; PR is created after the passing QA.

4. **Cap exceeded**: Construct a case where QA keeps failing. Confirm the job fails without PR after 2 additional iterations and surfaces accumulated findings.

5. **Resume**: Interrupt a job during QA (kill daemon). Restart and confirm earlier phases are reused and QA re-runs; after a passing QA the job completes.

6. **Grouped flow**: Submit a multi-repo issue. Confirm QA runs once on the shared workspace and a single QA report comment is posted.