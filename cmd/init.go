package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jullury/akama/internal/agent"
	"github.com/jullury/akama/internal/config"
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

	fmt.Print("Telegram admin user ID (your Telegram user ID): ")
	var adminID int64
	fmt.Scanln(&adminID)
	cfg.AdminUserID = adminID
	fmt.Println()

	fmt.Print("Default agent [claude/opencode] (default: claude): ")
	var ag string
	fmt.Scanln(&ag)
	if ag == "" {
		ag = "claude"
	}
	cfg.DefaultAgent = ag

	switch ag {
	case "opencode":
		installOpencode()
	default:
		installClaude()
	}

	if err := cfg.Save(path); err != nil {
		fmt.Fprintf(os.Stderr, "Save config: %v\n", err)
		return
	}

	if err := os.Chmod(path, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Chmod config: %v\n", err)
	}

	fmt.Println("\nConfig saved. Run `akama start` to start the daemon.")
}

func installClaude() {
	if _, err := exec.LookPath("claude"); err == nil {
		fmt.Println("claude already installed, skipping.")
		return
	}
	fmt.Print("Installing claude... ")
	if err := agent.InstallClaudeCmd(); err != nil {
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
	if _, err := exec.LookPath("opencode"); err == nil {
		fmt.Println("opencode already installed, skipping.")
		return
	}
	fmt.Print("Installing opencode... ")
	if err := agent.InstallOpencodeCmd(); err != nil {
		fmt.Printf("failed: %v\n", err)
		return
	}
	if _, err := exec.LookPath("opencode"); err != nil {
		fmt.Println("installed but 'opencode' not found in PATH — you may need to restart your shell or add the install location to PATH")
		return
	}
	fmt.Println("done.")
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
