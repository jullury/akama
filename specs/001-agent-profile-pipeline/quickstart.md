# Quickstart: Agent Profile Pipeline

1. Build: `go build ./...`
2. Run daemon, create a job from an issue URL.
3. After completion, inspect job: `SELECT profile_outputs FROM jobs WHERE id = <id>;`
4. Verify all seven keys present: spec, plan, task, analyze, implement, converge, reviewer.
5. Reviewer output appears as a comment on the created PR.