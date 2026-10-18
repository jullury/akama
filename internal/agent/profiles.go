package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
	ProfileReviewer,
}

// ProfilePrompt builds the user-facing prompt for a given profile. prior holds
// the accumulated outputs of earlier phases (may be empty).
func ProfilePrompt(profile Profile, title, body, workspacePath, prior string) string {
	base := fmt.Sprintf("Issue Title: %s\n\nDescription:\n%s\n", title, body)
	if prior != "" {
		base += "\n--- Prior phase outputs ---\n" + prior + "\n"
	}
	switch profile {
	case ProfileSpec:
		return base + "\nYou are a software architect. Produce a concise technical specification: goals, non-goals, affected components, and acceptance criteria. Do not write code.\n"
	case ProfilePlan:
		return base + "\nProduce a step-by-step implementation plan. List files to change and the approach for each.\n"
	case ProfileTask:
		return base + "\nBreak the plan into an ordered list of small, actionable tasks with the files each touches.\n"
	case ProfileAnalyze:
		return fmt.Sprintf("Repository workspace: %s\n\n%s\nAnalyze the repository structure relevant to the issue and summarize key modules, entry points, and risks.\n", workspacePath, base)
	case ProfileImplement:
		return base + "\nImplement the complete fix in the repository. Make all necessary code changes. Do NOT create pull requests or push.\n"
	case ProfileConverge:
		return base + "\nReview all changes made so far, resolve inconsistencies, and ensure the project builds. Report what you fixed.\n"
	case ProfileReviewer:
		return base + "\nReview the git diff of the changes. Report bugs, missing tests, and improvements. End with a verdict: APPROVE or CHANGES_REQUESTED.\n"
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
