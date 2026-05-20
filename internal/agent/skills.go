package agent

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const skillHubBase = "https://www.skillhub.club/api/v1/skills"

// Skill represents an installable skillhub.club skill.
type Skill struct {
	ID          string
	Name        string
	Description string
	// Required marks skills that are always installed during init (shown pre-checked, cannot be skipped).
	Required bool
	// AlwaysInject means the skill's content is prepended to every agent prompt.
	// ContentFile is the filename inside ~/.claude/commands/ where the skill lands after install.
	AlwaysInject bool
	ContentFile  string
	// RawURL is used for skills not hosted on skillhub.club.
	// When set, the raw file is downloaded directly instead of using the skillhub script.
	RawURL string
}

// BuiltinSkills is the curated list shown during `akama init` and via /skills.
// To add a new skill: append one Skill{} line — ID is the skillhub.club slug.
var BuiltinSkills = []Skill{
	// Core superpowers (orchestration + always-active workflow guidance)
	{
		ID: "obra-superpowers-using-superpowers", Name: "Using Superpowers", Description: "Unlock advanced agent capabilities",
		Required: true, AlwaysInject: true, ContentFile: "obra-superpowers-using-superpowers/SKILL.md",
	},
	{ID: "obra-superpowers-brainstorming", Name: "Brainstorming", Description: "Explore intent and design before implementing any feature or change"},
	{ID: "obra-superpowers-writing-plans", Name: "Writing Plans", Description: "Write a structured implementation plan before touching code"},
	{ID: "obra-superpowers-executing-plans", Name: "Executing Plans", Description: "Structured plan execution for agents"},
	{ID: "obra-superpowers-systematic-debugging", Name: "Systematic Debugging", Description: "Root-cause analysis process for bugs, test failures, and unexpected behavior"},
	{ID: "obra-superpowers-test-driven-development", Name: "Test-Driven Development", Description: "Red-green-refactor TDD workflow"},
	{ID: "obra-superpowers-verification-before-completion", Name: "Verification Before Completion", Description: "Checklist to verify work meets requirements before calling a task done"},
	{ID: "obra-superpowers-finishing-a-development-branch", Name: "Finishing a Branch", Description: "Guides merge/PR/cleanup decisions when implementation is complete"},
	{ID: "obra-superpowers-requesting-code-review", Name: "Requesting Code Review", Description: "Prepare and submit work for review"},
	{ID: "obra-superpowers-receiving-code-review", Name: "Receiving Code Review", Description: "Process review feedback and iterate"},
	{ID: "obra-superpowers-dispatching-parallel-agents", Name: "Dispatching Parallel Agents", Description: "Spawn and coordinate multiple agents for independent subtasks"},
	{ID: "obra-superpowers-subagent-driven-development", Name: "Subagent-Driven Development", Description: "Execute plans via independent subagents in the current session"},
	{ID: "obra-superpowers-using-git-worktrees", Name: "Using Git Worktrees", Description: "Isolate feature work via git worktrees before implementation"},
	// Standalone coding skills
	{ID: "alirezarezvani-claude-skills-focused-fix", Name: "Focused Fix", Description: "Systematic deep-dive repair across files and dependencies",
		RawURL:      "https://raw.githubusercontent.com/alirezarezvani/claude-skills/7d493fed97e4d57553630e1a2432c1c02bf5b2b3/engineering/skills/focused-fix/SKILL.md",
		ContentFile: "focused-fix/SKILL.md"},
	{ID: "tdd-guide", Name: "TDD Guide", Description: "Comprehensive test-driven development guide"},
	{ID: "git-worktree-manager", Name: "Git Worktree Manager", Description: "Manage git worktrees for parallel development"},
	// Engineering depth
	{ID: "senior-architect", Name: "Senior Architect", Description: "System design, architecture decisions, ADRs, and tech stack evaluation"},
	{ID: "senior-fullstack", Name: "Senior Fullstack", Description: "Full-stack development patterns across frontend and backend"},
	{ID: "alirezarezvani-claude-skills-senior-backend", Name: "Senior Backend", Description: "Backend development patterns, API design, database optimization, and security practices"},
	{ID: "alirezarezvani-claude-skills-senior-frontend", Name: "Senior Frontend", Description: "Frontend development patterns, performance optimization, and automation tools for React/Next.js applications"},
	{ID: "alirezarezvani-claude-skills-senior-devops", Name: "Senior DevOps", Description: "Complete toolkit for senior devops with modern tools and best practices"},
	{ID: "karpathy-coder", Name: "Karpathy Coder", Description: "High-quality coding philosophy and best practices"},
	{ID: "api-design-reviewer", Name: "API Design Reviewer", Description: "REST/GraphQL API design review and best practices"},
	{ID: "spec-driven-workflow", Name: "Spec-Driven Workflow", Description: "Implement from specifications with disciplined spec-first approach"},
	// DevOps and infrastructure
	{ID: "docker-development", Name: "Docker Development", Description: "Dockerfile optimization, docker-compose, multi-stage builds, and container security"},
	{ID: "ci-cd-pipeline-builder", Name: "CI/CD Pipeline Builder", Description: "Build and optimize CI/CD pipelines across GitHub Actions, GitLab CI, and others"},
	{ID: "terraform-patterns", Name: "Terraform Patterns", Description: "Infrastructure as code patterns and Terraform best practices"},
	{ID: "helm-chart-builder", Name: "Helm Chart Builder", Description: "Kubernetes Helm chart creation and management"},
	{ID: "aws-solution-architect", Name: "AWS Solution Architect", Description: "AWS architecture patterns, services selection, and cost optimization"},
	{ID: "gcp-cloud-architect", Name: "GCP Cloud Architect", Description: "Google Cloud architecture and service design"},
	{ID: "observability-designer", Name: "Observability Designer", Description: "Logging, metrics, tracing, and alerting design"},
	{ID: "monorepo-navigator", Name: "Monorepo Navigator", Description: "Monorepo structure, tooling, and dependency management"},
	// Quality, testing, and database
	{ID: "senior-qa", Name: "Senior QA", Description: "Test strategy, test automation, and quality engineering practices"},
	{ID: "performance-profiler", Name: "Performance Profiler", Description: "Profiling, benchmarking, and performance optimization"},
	{ID: "migration-architect", Name: "Migration Architect", Description: "Database and system migration strategies"},
	{ID: "dependency-auditor", Name: "Dependency Auditor", Description: "Dependency security auditing and update strategies"},
	{ID: "database-schema-designer", Name: "Database Schema Designer", Description: "ERD design, schema normalization, and migration planning"},
	{ID: "sql-database-assistant", Name: "SQL Database Assistant", Description: "SQL query optimization, indexing, and database administration"},
	// Context and search
	{ID: "massgen-massgen-file-search", Name: "File Search", Description: "Smart file search across codebases"},
	{ID: "muratcankoylan-agent-skills-for-context-engineering-context-optimization", Name: "Context Optimization", Description: "Context engineering for better agent output"},
	// UI/UX and web
	{ID: "bytedance-deer-flow-web-design-guidelines", Name: "Web Design Guidelines", Description: "ByteDance DeerFlow web design patterns"},
	{ID: "nextlevelbuilder-ui-ux-pro-max-skill-ui-ux-pro-max", Name: "UI/UX Pro Max", Description: "Advanced UI/UX design guidance"},
	{ID: "zhayujie-chatgpt-on-wechat-web-fetch", Name: "Web Fetch", Description: "Web content fetching capability"},
}

// InjectedSkillsContent returns the concatenated content of all AlwaysInject skills
// found in ~/.claude/skills/. Missing files are silently skipped.
// findSkillContent resolves a skill's SKILL.md content in priority order:
//  1. ~/.claude/skills/<ContentFile>  (explicit path when ContentFile is set)
//  2. ~/.claude/skills/<s.ID>/SKILL.md  (standard skillhub subdirectory layout)
//  3. ~/.claude/plugins/cache/claude-plugins-official/superpowers/<ver>/skills/<name>/SKILL.md
//     (Claude Code superpowers plugin cache, for skills not yet on skillhub)
//  4. embedded/<s.ID>/SKILL.md  (bundled in the binary — always works inside containers)
//
// Returns nil if the skill is not found anywhere.
func findSkillContent(home string, s Skill) []byte {
	skillsDir := filepath.Join(home, ".claude", "skills")

	// 1. Explicit ContentFile
	if s.ContentFile != "" {
		if data, err := os.ReadFile(filepath.Join(skillsDir, s.ContentFile)); err == nil {
			return data
		}
	}

	// 2. Standard skillhub subdirectory
	if data, err := os.ReadFile(filepath.Join(skillsDir, s.ID, "SKILL.md")); err == nil {
		return data
	}

	// 3. Superpowers plugin cache (strip "obra-superpowers-" prefix for the plugin's name)
	if name, ok := strings.CutPrefix(s.ID, "obra-superpowers-"); ok {
		pattern := filepath.Join(home, ".claude", "plugins", "cache", "claude-plugins-official", "superpowers", "*", "skills", name, "SKILL.md")
		if matches, err := filepath.Glob(pattern); err == nil && len(matches) > 0 {
			if data, err := os.ReadFile(matches[len(matches)-1]); err == nil {
				return data
			}
		}
	}

	// 4. Binary-embedded fallback (always present; used inside containers)
	if data, err := embeddedSkills.ReadFile("embedded/" + s.ID + "/SKILL.md"); err == nil {
		return data
	}
	return nil
}

func InjectedSkillsContent() string {
	home, _ := os.UserHomeDir()
	var sb strings.Builder
	for _, s := range BuiltinSkills {
		if !s.AlwaysInject {
			continue
		}
		data := findSkillContent(home, s)
		if data == nil {
			continue
		}
		sb.Write(data)
		sb.WriteString("\n\n")
	}
	return sb.String()
}

// OpencodeInjectedContent returns skill content adapted for opencode, which has no Skill tool.
// Prepends a context header, then appends content from every BuiltinSkill that is installed.
// Uses findSkillFile which checks ~/.claude/skills/ and the superpowers plugin cache.
// Missing files are silently skipped.
func OpencodeInjectedContent() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Agent Environment: OpenCode\n\n")
	sb.WriteString("You are running as an opencode agent within the Akama orchestration system.\n")
	sb.WriteString("All available skills are pre-loaded directly in this prompt — there is NO Skill tool.\n")
	sb.WriteString("When skill content references \"invoke the Skill tool\", ignore that instruction.\n")
	sb.WriteString("Instead, apply the skill guidance you can already read in this context.\n")
	sb.WriteString("Before responding to any task, review the skills below and follow relevant guidance.\n\n")
	sb.WriteString("---\n\n")

	seen := map[string]bool{}
	for _, s := range BuiltinSkills {
		data := findSkillContent(home, s)
		if data == nil || seen[s.ID] {
			continue
		}
		seen[s.ID] = true
		sb.Write(data)
		sb.WriteString("\n\n")
	}
	return sb.String()
}

// SkillByIndex returns the skill at position i, or nil if out of range.
func SkillByIndex(i int) *Skill {
	if i < 0 || i >= len(BuiltinSkills) {
		return nil
	}
	return &BuiltinSkills[i]
}

// InstallSkill downloads and runs the skillhub install script for skillID across all agents.
// If s.RawURL is set, it downloads the raw file directly and installs it.
func InstallSkill(s Skill) error {
	if s.RawURL != "" {
		return installRawSkill(s)
	}
	url := fmt.Sprintf("%s/%s/install?agents=claude,opencode&format=sh", skillHubBase, s.ID)

	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return fmt.Errorf("fetch install script: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("skillhub returned HTTP %d", resp.StatusCode)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return fmt.Errorf("read script: %w", err)
	}

	cmd := exec.Command("bash")
	cmd.Stdin = &buf
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(out))
	}
	return nil
}

// installRawSkill downloads a raw .md file and installs it into ~/.claude/commands/
func installRawSkill(s Skill) error {
	resp, err := http.Get(s.RawURL) //nolint:noctx
	if err != nil {
		return fmt.Errorf("fetch raw skill: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("raw skill returned HTTP %d", resp.StatusCode)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	skillsDir := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return fmt.Errorf("create skills dir: %w", err)
	}

	filename := s.ContentFile
	if filename == "" {
		filename = filepath.Base(s.RawURL)
	}
	destPath := filepath.Join(skillsDir, filename)

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if err := os.WriteFile(destPath, data, 0o644); err != nil {
		return fmt.Errorf("write skill file: %w", err)
	}
	return nil
}
