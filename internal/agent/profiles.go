package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Profile identifies a named agent role in the spec-driven pipeline.
type Profile string

const (
	ProfileSpec      Profile = "spec"
	ProfilePlan      Profile = "plan"
	ProfileTask      Profile = "task"
	ProfileAnalyze   Profile = "analyze"
	ProfileImplement Profile = "implement"
	ProfileConverge  Profile = "converge"
	ProfileQA        Profile = "qa"
	ProfileReviewer  Profile = "reviewer"
)

// ProfileOrder is the default execution order of the pipeline.
var ProfileOrder = []Profile{
	ProfileSpec,
	ProfilePlan,
	ProfileTask,
	ProfileAnalyze,
	ProfileImplement,
	ProfileConverge,
	ProfileQA,
	ProfileReviewer,
}

// ProfilePrompt builds the user-facing prompt for a given profile. prior holds
// the accumulated outputs of earlier phases (may be empty).
//
// Prompts mirror the SpecKit command methodology (specify, clarify, plan, tasks,
// analyze, implement) adapted to akama's issue -> PR workflow: each phase produces
// a structured artifact consumed by the next, with quality gates and bounded
// clarifications, but operates on the codebase/workspace rather than spec files.
func ProfilePrompt(profile Profile, title, body, workspacePath, prior string) string {
	base := fmt.Sprintf("Issue Title: %s\n\nDescription:\n%s\n", title, body)
	if prior != "" {
		base += "\n--- Prior phase outputs (context only; do not repeat verbatim) ---\n" + prior + "\n"
	}
	switch profile {
	case ProfileSpec:
		return base + `
ROLE: You are a senior software architect running /speckit.specify (with /speckit.clarify).

EXECUTION FLOW (adapted to an issue -> PR pipeline):
1. Parse the issue description; extract actors, actions, data, constraints.
2. For unclear aspects, make informed guesses based on context and industry standards.
   Only mark with [NEEDS CLARIFICATION: specific question] when:
     - The choice significantly impacts feature scope or user experience
     - Multiple reasonable interpretations exist with different implications
     - No reasonable default exists
   LIMIT: maximum 3 [NEEDS CLARIFICATION] markers, prioritized scope > security/privacy > UX > technical.
3. If you find CRITICAL ambiguities, ask up to 5 targeted clarification questions and STOP.
   Each question must be answerable with a short answer or 2-5 multiple-choice options.
   End your entire response with a single question mark (?) so the system can detect it.
   Do NOT produce the final spec yet.
4. Otherwise (or once clarified), generate the specification following the structure below.
   Success criteria must be measurable, technology-agnostic, user-focused, verifiable.
   Common areas needing clarification only if no reasonable default: scope/boundaries, user types/permissions, security/compliance.

OUTPUT CONTRACT (markdown, only when producing the final spec):
## User Scenarios & Testing
### User Story 1 - [Title] (Priority: P1)
... (actors, journey, independent test, acceptance scenarios Given/When/Then)
### Edge Cases
## Requirements
### Functional Requirements (FR-001, testable, unambiguous)
### Key Entities (if data involved)
## Success Criteria (measurable, technology-agnostic outcomes SC-001...)
## Assumptions
CONSTRAINTS:
- Do NOT write or modify code.
- Do NOT propose implementation steps (that is the plan phase).
- Focus on WHAT users need and WHY; avoid HOW.
- Remove sections that don't apply (no "N/A").
`
	case ProfilePlan:
		return base + `
ROLE: You are a tech lead running /speckit.plan.

EXECUTION FLOW:
1. Establish Technical Context: language, dependencies, storage, testing, target platform, project type, performance goals, constraints, scale/scope. Mark unknowns as "NEEDS CLARIFICATION".
2. Constitution Check: state the project principles/gates that apply; ERROR if violations are unjustified.
3. Phase 0 (Research): for each unknown/dependency/integration, research best practices and consolidate findings as Decision / Rationale / Alternatives considered.
4. Phase 1 (Design): extract entities (name, fields, relationships, validation, state transitions), define interface contracts (public APIs/CLI/commands), create a quickstart validation guide with prerequisites, setup, run, expected outcomes (no full implementation code).
5. Re-evaluate Constitution Check post-design.

OUTPUT CONTRACT (markdown):
## Technical Context
## Constitution Check
## Phase 0: Research (Decision / Rationale / Alternatives)
## Phase 1: Design
### Data Model
### Interface Contracts
### Quickstart Validation
## Post-Design Constitution Check
CONSTRAINTS:
- Do NOT write code; describe design precisely enough to implement.
- Use absolute paths for filesystem operations; project-relative paths in docs.
- ERROR on gate failures or unresolved clarifications (mark them, don't silently skip).
`
	case ProfileTask:
		return base + `
ROLE: You are an engineering manager running /speckit.tasks.

EXECUTION FLOW:
1. Derive tasks from the spec (user stories) and plan (design), organized by user story so each story is independently implementable and testable.
2. Phase structure: Setup -> Foundational (blocking) -> User Story phases (P1, P2, ...) -> Polish/Cross-cutting.
3. For each task: ID, description, touched files, dependency on prior tasks, parallel marker [P] when independent.
4. Order tasks by dependency (foundations first); mark [P] for tasks that can run in parallel within a phase.
5. Include checkpoints for validating each user story independently.

OUTPUT CONTRACT (markdown):
## Phase 1: Setup
## Phase 2: Foundational (blocking)
## Phase 3: User Story 1 - [Title] (P1)
- [ ] T001 [US1] <task> — files: <paths> — change: <what>
...
## Phase N: Polish
CONSTRAINTS:
- Tasks must be small enough to review in isolation.
- Every task must name the files it touches.
`
	case ProfileAnalyze:
		return fmt.Sprintf(`Repository workspace: %s

%s
ROLE: You are a reviewer running /speckit.analyze (STRICTLY READ-ONLY).

EXECUTION FLOW (cross-artifact consistency & quality analysis):
1. Load the spec, plan, and task outputs from prior phases (already in context).
2. Build semantic models: requirements inventory (FR-/SC- keys), user story/action inventory, task coverage mapping, constitution rule set.
3. Detection passes (token-efficient, high-signal, limit 50 findings):
   A. Duplication: near-duplicate requirements.
   B. Ambiguity: vague adjectives, unresolved placeholders.
   C. Underspecification: requirements with no measurable outcome.
   D. Constitution alignment: conflicts with stated principles (CRITICAL).
   E. Coverage gaps: requirements with zero task coverage.
   F. Inconsistency: terminology drift, conflicting decisions.
4. Severity: CRITICAL (blocks implementation), HIGH, MEDIUM, LOW.
   Constitution conflicts are always CRITICAL.

OUTPUT CONTRACT (markdown, read-only report, no file changes):
## Findings (ID, category, severity, location, summary, recommendation)
## Coverage Summary (requirements vs tasks)
## Constitution Alignment
## Unmapped Tasks
## Metrics (total requirements, total tasks, coverage %%, ambiguity count, duplication count, critical issues)
## Next Actions
CONSTRAINTS:
- Do NOT modify files or code.
- Do NOT silently ignore constitution conflicts.
`, workspacePath, base)
	case ProfileImplement:
		return base + `
ROLE: You are a senior software engineer running /speckit.implement.

EXECUTION FLOW:
1. Prerequisites: spec, plan, tasks are complete (available in prior phase outputs). Check checklists if present; report incomplete items but proceed unless critical.
2. Parse tasks.md structure: phases, task IDs, [P] markers, file paths, dependencies.
3. Execute phase by phase, respecting dependencies (sequential tasks in order; [P] tasks may be done together).
4. Per task: read referenced files, implement the change, mark the task complete, handle errors (fix and retry; if blocked, report and continue).
5. Validation: run available tests/builds after each phase; report failures with context.
6. Progress tracking: report completed tasks vs total.

CONSTRAINTS:
- Implement ALL tasks, not just the first phase, unless explicitly scoped.
- Respect file paths and project structure from the plan.
- Do NOT create pull requests or push branches (handled separately).
- Do NOT mention AI, bots, automation, or tools in code comments.
- Write as a human developer would.
OUTPUT: code changes in the repository + a short completion summary.
`
	case ProfileConverge:
		return base + `
ROLE: You are an integration engineer converging the change set (SpecKit has no direct equivalent; this is akama's reconciliation gate).

EXECUTION FLOW:
1. Review all changes made so far for consistency and completeness.
2. Resolve inconsistencies, missing wiring, or half-finished edits.
3. Ensure the project builds (run build/tests where feasible).
4. Report concretely what you fixed or verified.

CONSTRAINTS:
- Do NOT introduce new features; only reconcile and verify.
- Do NOT create pull requests or push branches.
OUTPUT CONTRACT (markdown):
## Issues Found
## Fixes Applied
## Build/Test Status
`
	case ProfileQA:
		return base + `
ROLE: You are a quality assurance engineer verifying the implemented change (SpecKit has no direct equivalent; this is akama's verification gate).

EXECUTION FLOW:
1. Identify build/test tooling from repository files (Makefile, go.mod, package.json, Cargo.toml, etc.) — do NOT assume a fixed command.
2. Run the project build; record PASS/FAIL.
3. Run available automated tests; record PASS/FAIL (or N/A if none exist — do not fail solely for missing tests).
4. Verify the implementation against the acceptance scenarios derived in the spec phase (prior outputs). For scenarios that cannot be automated, record them as "Manual/Not automated" with a reason.
5. Report defects found (file/line when known).

CONSTRAINTS:
- Verification-only with respect to product behavior: do NOT change product behavior.
- MAY add or modify automated tests to exercise acceptance scenarios; test changes are part of verification.
- Do NOT introduce new features.
- Do NOT create pull requests or push branches.
OUTPUT CONTRACT (markdown):
## Checks Performed
- Build
- Automated tests
- Acceptance scenario: <title>
## Results
- Build: PASS | FAIL
- Automated tests: PASS | FAIL | N/A (no tests)
- Acceptance scenario: <title>: PASS | FAIL | Manual/Not automated
## Defects Found
- <defect description> (file/line if known)
## Verdict
Verdict: PASS
(or Verdict: FAIL / Verdict: BLOCKING when a blocking defect is found)
`
	case ProfileReviewer:
		return base + `
ROLE: You are a strict code reviewer (final quality gate before PR).

EXECUTION FLOW:
1. Review the git diff of the changes.
2. Report bugs, logic errors, security concerns, missing tests, and improvements; cite files/lines.
3. Verify against the spec's acceptance criteria and the plan's design.
4. Do NOT rewrite the implementation; only review.

CONSTRAINTS:
- End with exactly one verdict line.
OUTPUT CONTRACT (markdown):
## Findings
## Missing Tests
## Improvements
## Acceptance Criteria Check
Verdict: APPROVE | CHANGES_REQUESTED
`
	default:
		return base
	}
}

// RunProfileAgent runs a single profile phase, mirroring RunPlanAgent.
func RunProfileAgent(ctx context.Context, profile Profile, agentName, model, workspacePath, promptContent string, cfg *Config) (string, error) {
	if workspacePath == "" {
		return "", fmt.Errorf("workspace path is required for profile %s", profile)
	}
	promptPath, err := WritePrompt(workspacePath, agentName, promptContent)
	if err != nil {
		return "", fmt.Errorf("write %s prompt: %w", profile, err)
	}
	defer os.Remove(promptPath)

	output, err := Run(ctx, agentName, model, workspacePath, promptPath, cfg)
	if err != nil {
		return "", err
	}
	return ParseOutput(agentName, output), nil
}

// DisplayName returns a user-friendly label for a profile, used in notifications.
func (p Profile) DisplayName() string {
	switch p {
	case ProfileSpec:
		return "Spec (Architect)"
	case ProfilePlan:
		return "Plan (Tech Lead)"
	case ProfileTask:
		return "Tasks (Manager)"
	case ProfileAnalyze:
		return "Analyze (Codebase Analyst)"
	case ProfileImplement:
		return "Implement (Engineer)"
	case ProfileConverge:
		return "Converge (Integrator)"
	case ProfileQA:
		return "QA (Quality Assurance)"
	case ProfileReviewer:
		return "Review (Reviewer)"
	}
	return string(p)
}

// IsValidPlan reports whether a plan output looks like a real implementation plan
// rather than an error message, empty output, or truncated noise. It requires a
// minimum length and at least one common plan marker.
func IsValidPlan(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 200 {
		return false
	}
	lower := strings.ToLower(s)
	markers := []string{"##", "phase", "technical context", "implementation", "files to change", "approach"}
	for _, m := range markers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

// QAMaxFixIterations is the maximum number of additional implement->converge->QA
// iterations attempted when QA reports a blocking defect.
const QAMaxFixIterations = 2

// IsQAFailure reports whether a QA profile output indicates a blocking defect.
// It scans for the last "Verdict:" line and treats FAIL or BLOCKING as failure.
func IsQAFailure(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "verdict:") {
			token := strings.TrimSpace(line[len("verdict:"):])
			token = strings.ToUpper(token)
			return strings.Contains(token, "FAIL") || strings.Contains(token, "BLOCKING")
		}
	}
	return false
}

// ProfileOutputs is a JSON-serializable map of phase name -> agent output.
type ProfileOutputs map[string]string

func (p ProfileOutputs) Marshal() string {
	b, _ := json.Marshal(p)
	return string(b)
}

func ParseProfileOutputs(s string) ProfileOutputs {
	out := ProfileOutputs{}
	if s == "" {
		return out
	}
	_ = json.Unmarshal([]byte(s), &out)
	return out
}