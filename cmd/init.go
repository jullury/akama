package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jullury/akama/internal/agent"
	"github.com/jullury/akama/internal/config"
	"github.com/jullury/akama/internal/storage"
	"golang.org/x/term"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Interactive first-run setup",
	Run:   runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) {
	path := cfgPath
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[2:])
	}

	if _, err := os.Stat(path); err == nil {
		fmt.Print("Config already exists. Overwrite? (y/N): ")
		var resp string
		fmt.Scanln(&resp)
		if strings.ToLower(resp) != "y" {
			fmt.Println("Aborted.")
			return
		}
	}

	cfg := config.DefaultConfig()

	fmt.Print("Telegram bot token: ")
	token, _ := term.ReadPassword(int(os.Stdin.Fd()))
	cfg.TelegramToken = strings.TrimSpace(string(token))
	fmt.Println()

	fmt.Print("Anthropic API key (required for claude): ")
	key, _ := term.ReadPassword(int(os.Stdin.Fd()))
	keyStr := strings.TrimSpace(string(key))
	if keyStr != "" {
		cfg.SetAPIKey("anthropic", keyStr)
	}
	fmt.Println()

	fmt.Print("OpenAI API key (optional, for opencode): ")
	key, _ = term.ReadPassword(int(os.Stdin.Fd()))
	keyStr = strings.TrimSpace(string(key))
	if keyStr != "" {
		cfg.SetAPIKey("openai", keyStr)
	}
	fmt.Println()

	fmt.Print("Default agent [claude/opencode] (default: claude): ")
	var agent string
	fmt.Scanln(&agent)
	if agent == "" {
		agent = "claude"
	}
	cfg.DefaultAgent = agent

	switch agent {
	case "opencode":
		installOpencode()
	default:
		installClaude()
	}
	installSkills()

	fmt.Print("Workspace directory (default ~/.akama/workspaces): ")
	var ws string
	fmt.Scanln(&ws)
	if ws == "" {
		ws = "~/.akama/workspaces"
	}
	cfg.WorkspaceDir = ws

	if err := cfg.Save(path); err != nil {
		fmt.Fprintf(os.Stderr, "Save config: %v\n", err)
		return
	}

	if err := os.Chmod(path, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Chmod config: %v\n", err)
	}

	dbPath := cfg.DBPath
	if strings.HasPrefix(dbPath, "~/") {
		home, _ := os.UserHomeDir()
		dbPath = filepath.Join(home, dbPath[2:])
	}
	os.MkdirAll(filepath.Dir(dbPath), 0700)
	db, err := storage.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Open DB: %v\n", err)
		return
	}
	db.Close()

	fmt.Println("Config saved. Run `akama start` to start the bot.")
}

func installClaude() {
	fmt.Print("Installing claude... ")
	err := agent.InstallClaudeCmd()
	if err != nil {
		fmt.Printf("failed: %v\n", err)
		return
	}
	if _, err := exec.LookPath("claude"); err != nil {
		fmt.Println("installed but 'claude' not found in PATH — you may need to restart your shell or add the install location to PATH")
		return
	}
	fmt.Println("done.")
}

func installOpencode() {
	fmt.Print("Installing opencode... ")
	err := agent.InstallOpencodeCmd()
	if err != nil {
		fmt.Printf("failed: %v\n", err)
		return
	}
	if _, err := exec.LookPath("opencode"); err != nil {
		fmt.Println("installed but 'opencode' not found in PATH — you may need to restart your shell or add the install location to PATH")
		return
	}
	fmt.Println("done.")
}

func installSkills() {
	fmt.Println("\nAvailable skills (press Enter to skip):")
	for i, s := range agent.BuiltinSkills {
		fmt.Printf("  %d. %-25s — %s\n", i+1, s.Name, s.Description)
	}
	fmt.Print("\nSelect skills to install [1,2,... or 'all']: ")
	var input string
	fmt.Scanln(&input)
	input = strings.TrimSpace(input)
	if input == "" {
		return
	}

	var toInstall []agent.Skill
	if strings.ToLower(input) == "all" {
		toInstall = agent.BuiltinSkills
	} else {
		for _, part := range strings.Split(input, ",") {
			var idx int
			fmt.Sscanf(strings.TrimSpace(part), "%d", &idx)
			s := agent.SkillByIndex(idx - 1)
			if s != nil {
				toInstall = append(toInstall, *s)
			}
		}
	}

	for _, s := range toInstall {
		fmt.Printf("Installing %s... ", s.Name)
		if err := agent.InstallSkill(s.ID); err != nil {
			fmt.Printf("failed: %v\n", err)
		} else {
			fmt.Println("done.")
		}
	}
}

func UpdateAgents() {
	fmt.Println("Updating agents...")
	results := agent.UpdateAll()
	for name, err := range results {
		if err != nil {
			fmt.Printf("%s: %v\n", name, err)
		} else {
			fmt.Printf("%s: updated\n", name)
		}
	}
	fmt.Println("Agent update complete.")
}
