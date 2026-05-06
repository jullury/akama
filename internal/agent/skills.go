package agent

import (
	"bytes"
	"fmt"
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
}

// BuiltinSkills is the curated list shown during `akama init` and via /skills.
// To add a new skill: append one Skill{} line — ID is the skillhub.club slug.
var BuiltinSkills = []Skill{
	{
		ID: "obra-superpowers-using-superpowers", Name: "Using Superpowers", Description: "Unlock advanced agent capabilities",
		Required: true, AlwaysInject: true, ContentFile: "using-superpowers.md",
	},
	{ID: "massgen-massgen-file-search", Name: "File Search", Description: "Smart file search across codebases"},
	{ID: "muratcankoylan-agent-skills-for-context-engineering-context-optimization", Name: "Context Optimization", Description: "Context engineering for better agent output"},
	{ID: "bytedance-deer-flow-web-design-guidelines", Name: "Web Design Guidelines", Description: "ByteDance DeerFlow web design patterns"},
	{ID: "obra-superpowers-executing-plans", Name: "Executing Plans", Description: "Structured plan execution for agents"},
	{ID: "nextlevelbuilder-ui-ux-pro-max-skill-ui-ux-pro-max", Name: "UI/UX Pro Max", Description: "Advanced UI/UX design guidance"},
	{ID: "zhayujie-chatgpt-on-wechat-web-fetch", Name: "Web Fetch", Description: "Web content fetching capability"},
	{ID: "alirezarezvani-claude-skills-senior-backend", Name: "Senior Backend", Description: "Backend development patterns, API design, database optimization, and security practices"},
	{ID: "alirezarezvani-claude-skills-senior-frontend", Name: "Senior Frontend", Description: "Frontend development patterns, performance optimization, and automation tools for React/Next.js applications"},
	{ID: "alirezarezvani-claude-skills-senior-devops", Name: "Senior DevOps", Description: "Complete toolkit for senior devops with modern tools and best practices"},
}

// InjectedSkillsContent returns the concatenated content of all AlwaysInject skills
// found in ~/.claude/commands/. Missing files are silently skipped.
func InjectedSkillsContent() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	commandsDir := filepath.Join(home, ".claude", "commands")
	var sb strings.Builder
	for _, s := range BuiltinSkills {
		if !s.AlwaysInject || s.ContentFile == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(commandsDir, s.ContentFile))
		if err != nil {
			continue
		}
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
func InstallSkill(skillID string) error {
	url := fmt.Sprintf("%s/%s/install?agents=claude,opencode&format=sh", skillHubBase, skillID)

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
