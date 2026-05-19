package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jullury/akama/internal/storage"
)

func WriteKnowledgeFile(workspacePath string, jobs []storage.Job) (string, error) {
	if len(jobs) == 0 {
		return "", nil
	}

	var b strings.Builder
	b.WriteString("# Prior Art — Similar Issues Resolved Previously\n\n")
	b.WriteString("Use these as reference for patterns, conventions, and approaches.\n")
	b.WriteString("Do NOT copy blindly — adapt to the current codebase state.\n\n")

	for i, j := range jobs {
		title := j.IssueTitle
		if title == "" {
			title = "(no title)"
		}

		b.WriteString("---\n\n")
		b.WriteString(fmt.Sprintf("## [%d] %s\n", i+1, title))

		meta := ""
		if j.RepoURL != "" {
			meta += "Repo: " + j.RepoURL
		}
		if j.PRURL != "" {
			meta += " | PR: " + j.PRURL
		}
		if j.BranchName != "" {
			meta += " | Branch: " + j.BranchName
		}
		if meta != "" {
			b.WriteString(meta + "\n\n")
		}

		if j.IssueBody != "" {
			body := j.IssueBody
			if len(body) > 4000 {
				body = body[:4000] + "\n... (truncated)"
			}
			b.WriteString("### Issue\n")
			b.WriteString(body + "\n\n")
		}

		if j.Plan != "" {
			plan := j.Plan
			if len(plan) > 3000 {
				plan = plan[:3000] + "\n... (truncated)"
			}
			b.WriteString("### Implementation Plan\n")
			b.WriteString(plan + "\n\n")
		}

		if j.AgentOutput != "" {
			output := j.AgentOutput
			if len(output) > 3000 {
				output = output[:3000] + "\n... (truncated)"
			}
			b.WriteString("### Agent Reasoning & Steps\n")
			b.WriteString(output + "\n\n")
		}
	}

	filename := filepath.Join(workspacePath, ".akama-knowledge.md")
	if err := os.WriteFile(filename, []byte(b.String()), 0644); err != nil {
		return "", fmt.Errorf("write knowledge file: %w", err)
	}
	return filename, nil
}
