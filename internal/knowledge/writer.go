package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jullury/akama/internal/storage"
)

// RootCauseCategory represents common categories of issues
type RootCauseCategory string

const (
	CategoryBug           RootCauseCategory = "Bug Fix"
	CategoryFeature       RootCauseCategory = "Feature Implementation"
	CategoryRefactor      RootCauseCategory = "Refactoring"
	CategoryPerformance   RootCauseCategory = "Performance"
	CategorySecurity      RootCauseCategory = "Security"
	CategoryDependency    RootCauseCategory = "Dependency Update"
	CategoryConfiguration RootCauseCategory = "Configuration"
	CategoryDocumentation RootCauseCategory = "Documentation"
	CategoryTesting       RootCauseCategory = "Testing"
	CategoryUnknown       RootCauseCategory = "Other"
)

// classifyIssue attempts to classify the issue based on title and body keywords
func classifyIssue(title, body string) RootCauseCategory {
	text := strings.ToLower(title + " " + body)

	// Bug indicators
	bugKeywords := []string{"bug", "fix", "error", "crash", "broken", "failing", "issue", "problem", "exception", "panic", "undefined", "null pointer"}
	for _, kw := range bugKeywords {
		if strings.Contains(text, kw) {
			return CategoryBug
		}
	}

	// Security indicators
	securityKeywords := []string{"security", "vulnerability", "cve", "exploit", "injection", "xss", "csrf", "auth", "permission", "access control"}
	for _, kw := range securityKeywords {
		if strings.Contains(text, kw) {
			return CategorySecurity
		}
	}

	// Performance indicators
	perfKeywords := []string{"performance", "slow", "optimize", "memory", "cpu", "latency", "cache", "speed"}
	for _, kw := range perfKeywords {
		if(strings.Contains(text, kw)) {
			return CategoryPerformance
		}
	}

	// Feature indicators
	featureKeywords := []string{"feature", "add", "implement", "new", "create", "support", "enhance"}
	for _, kw := range featureKeywords {
		if strings.Contains(text, kw) {
			return CategoryFeature
		}
	}

	// Refactoring indicators
	refactorKeywords := []string{"refactor", "cleanup", "reorganize", "simplify", "extract", "rename"}
	for _, kw := range refactorKeywords {
		if strings.Contains(text, kw) {
			return CategoryRefactor
		}
	}

	// Dependency indicators
	depKeywords := []string{"upgrade", "update", "dependency", "package", "version", "npm", "pip", "cargo"}
	for _, kw := range depKeywords {
		if strings.Contains(text, kw) {
			return CategoryDependency
		}
	}

	return CategoryUnknown
}

// extractRootCause attempts to extract root cause from agent output
func extractRootCause(agentOutput string) string {
	if agentOutput == "" {
		return ""
	}

	// Look for common root cause patterns
	lowerOutput := strings.ToLower(agentOutput)

	// Pattern: "root cause" or "cause"
	rootCausePatterns := []string{
		"root cause",
		"cause of",
		"caused by",
		"because",
		"due to",
		"reason for",
	}

	for _, pattern := range rootCausePatterns {
		if idx := strings.Index(lowerOutput, pattern); idx != -1 {
			// Extract surrounding context (up to 200 chars)
			start := idx
			if start > 50 {
				start -= 50
			}
			end := idx + len(pattern) + 200
			if end > len(agentOutput) {
				end = len(agentOutput)
			}
			excerpt := agentOutput[start:end]
			// Clean up excerpt
			excerpt = strings.TrimSpace(excerpt)
			if len(excerpt) > 300 {
				excerpt = excerpt[:300] + "..."
			}
			return excerpt
		}
	}

	// If no explicit root cause found, return first 200 chars of output as context
	if len(agentOutput) > 200 {
		return agentOutput[:200] + "..."
	}
	return agentOutput
}

// extractResolutionPattern extracts the resolution approach from agent output
func extractResolutionPattern(agentOutput, plan string) string {
	var patterns []string

	// Extract from plan if available
	if plan != "" {
		planLower := strings.ToLower(plan)
		resolutionKeywords := []string{
			"fixed by",
			"resolved by",
			"changed",
			"modified",
			"updated",
			"replaced",
			"added",
			"removed",
		}
		for _, kw := range resolutionKeywords {
			if idx := strings.Index(planLower, kw); idx != -1 {
				start := idx
				if start > 30 {
					start -= 30
				}
				end := idx + len(kw) + 150
				if end > len(plan) {
					end = len(plan)
				}
				patterns = append(patterns, plan[start:end])
			}
		}
	}

	if len(patterns) > 0 {
		result := strings.Join(patterns, "; ")
		if len(result) > 500 {
			result = result[:500] + "..."
		}
		return result
	}

	// Fallback to agent output
	if agentOutput != "" {
		// Look for code changes mentioned
		codeChangePatterns := []string{
			"changed file",
			"modified file",
			"updated file",
			"added file",
			"removed file",
		}
		lowerOutput := strings.ToLower(agentOutput)
		for _, pattern := range codeChangePatterns {
			if idx := strings.Index(lowerOutput, pattern); idx != -1 {
				start := idx
				if start > 20 {
					start -= 20
				}
				end := idx + len(pattern) + 100
				if end > len(agentOutput) {
					end = len(agentOutput)
				}
				return agentOutput[start:end]
			}
		}
	}

	return ""
}

// extractFilesModified attempts to extract files modified from agent output
func extractFilesModified(agentOutput string) []string {
	if agentOutput == "" {
		return nil
	}

	var files []string
	seen := make(map[string]bool)

	// Common patterns for file paths in code
	filePatterns := []string{
		`\b[\w/\\.-]+\.\w{1,5}\b`, // General file paths
		`(?:file|path):\s*([^\s,]+)`,
		`(?:changed|modified|updated|added|removed)\s+([^\s,]+)`,
	}

	for _, pattern := range filePatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllString(agentOutput, -1)
		for _, match := range matches {
			// Filter out common false positives
			match = strings.TrimSpace(match)
			if match != "" && !seen[match] && len(match) > 3 && len(match) < 200 {
				// Check if it looks like a file path
				if strings.Contains(match, "/") || strings.Contains(match, ".") {
					files = append(files, match)
					seen[match] = true
				}
			}
		}
	}

	if len(files) > 10 {
		files = files[:10]
	}
	return files
}

func WriteKnowledgeFile(workspacePath string, jobs []storage.Job) (string, error) {
	if len(jobs) == 0 {
		return "", nil
	}

	var b strings.Builder
	b.WriteString("# Prior Art — Similar Issues Resolved Previously\n\n")
	b.WriteString("Use these as reference for patterns, conventions, and approaches.\n")
	b.WriteString("Do NOT copy blindly — adapt to the current codebase state.\n\n")
	b.WriteString("## Key Insights for Current Issue\n\n")
	b.WriteString("Before implementing, analyze these similar issues to:\n")
	b.WriteString("1. **Understand Root Cause**: Why did this issue occur? What were the underlying factors?\n")
	b.WriteString("2. **Identify Patterns**: Are there recurring patterns across similar issues?\n")
	b.WriteString("3. **Apply Proven Solutions**: What approaches worked for similar problems?\n")
	b.WriteString("4. **Prevent Recurrence**: What can be done to prevent similar issues in the future?\n\n")

	for i, j := range jobs {
		title := j.IssueTitle
		if title == "" {
			title = "(no title)"
		}

		b.WriteString("---\n\n")
		b.WriteString(fmt.Sprintf("## [%d] %s\n", i+1, title))

		// Repository context
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

		// Classify the issue
		category := classifyIssue(j.IssueTitle, j.IssueBody)
		b.WriteString(fmt.Sprintf("**Category**: %s\n\n", category))

		// Issue body
		if j.IssueBody != "" {
			body := j.IssueBody
			if len(body) > 4000 {
				body = body[:4000] + "\n... (truncated)"
			}
			b.WriteString("### Issue\n")
			b.WriteString(body + "\n\n")
		}

		// Root cause analysis
		rootCause := extractRootCause(j.AgentOutput)
		if rootCause != "" {
			b.WriteString("### Root Cause Analysis\n")
			b.WriteString(fmt.Sprintf("**Detected Root Cause**: %s\n\n", rootCause))
		}

		// Implementation plan
		if j.Plan != "" {
			plan := j.Plan
			if len(plan) > 3000 {
				plan = plan[:3000] + "\n... (truncated)"
			}
			b.WriteString("### Implementation Plan\n")
			b.WriteString(plan + "\n\n")
		}

		// Resolution pattern
		resolution := extractResolutionPattern(j.AgentOutput, j.Plan)
		if resolution != "" {
			b.WriteString("### Resolution Pattern\n")
			b.WriteString(fmt.Sprintf("**Approach**: %s\n\n", resolution))
		}

		// Files modified (if detectable)
		filesModified := extractFilesModified(j.AgentOutput)
		if len(filesModified) > 0 {
			b.WriteString("### Files Modified\n")
			for _, file := range filesModified {
				b.WriteString(fmt.Sprintf("- `%s`\n", file))
			}
			b.WriteString("\n")
		}

		// Agent reasoning and steps
		if j.AgentOutput != "" {
			output := j.AgentOutput
			if len(output) > 3000 {
				output = output[:3000] + "\n... (truncated)"
			}
			b.WriteString("### Agent Reasoning & Steps\n")
			b.WriteString(output + "\n\n")
		}

		// Learning points
		b.WriteString("### Key Takeaways\n")
		b.WriteString("- What worked: [Analyze the resolution approach]\n")
		b.WriteString("- What to avoid: [Note any pitfalls or failed approaches]\n")
		b.WriteString("- Patterns to reuse: [Identify reusable patterns]\n\n")
	}

	filename := filepath.Join(workspacePath, ".akama-knowledge.md")
	if err := os.WriteFile(filename, []byte(b.String()), 0644); err != nil {
		return "", fmt.Errorf("write knowledge file: %w", err)
	}
	return filename, nil
}
