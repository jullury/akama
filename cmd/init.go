package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	if _, err := exec.LookPath("claude"); err == nil {
		fmt.Println("already installed.")
		return
	}
	if _, err := exec.LookPath("brew"); err == nil {
		cmd := exec.Command("brew", "install", "--cask", "claude-code")
		if err := cmd.Run(); err != nil {
			fmt.Printf("failed: %v\n", err)
			return
		}
	} else if _, err := exec.LookPath("npm"); err == nil {
		cmd := exec.Command("npm", "install", "-g", "@anthropic-ai/claude-code")
		if err := cmd.Run(); err != nil {
			fmt.Printf("failed: %v\n", err)
			return
		}
	} else if _, err := exec.LookPath("curl"); err == nil {
		cmd := exec.Command("curl", "-fsSL", "https://claude.ai/install.sh", "-o", "/tmp/claude-install.sh")
		if err := cmd.Run(); err != nil {
			fmt.Println("failed: curl download failed")
			return
		}
		cmd = exec.Command("bash", "/tmp/claude-install.sh")
		err := cmd.Run()
		os.Remove("/tmp/claude-install.sh")
		if err != nil {
			fmt.Printf("failed: %v\n", err)
			return
		}
	} else {
		fmt.Println("failed: no supported package manager found (brew, npm, or curl required)")
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
	if _, err := exec.LookPath("opencode"); err == nil {
		fmt.Println("already installed.")
		return
	}
	if _, err := exec.LookPath("brew"); err == nil {
		cmd := exec.Command("brew", "install", "anomalyco/tap/opencode")
		if err := cmd.Run(); err != nil {
			fmt.Printf("failed: %v\n", err)
			return
		}
	} else if _, err := exec.LookPath("npm"); err == nil {
		cmd := exec.Command("npm", "install", "-g", "opencode-ai@latest")
		if err := cmd.Run(); err != nil {
			fmt.Printf("failed: %v\n", err)
			return
		}
	} else if _, err := exec.LookPath("curl"); err == nil {
		cmd := exec.Command("curl", "-fsSL", "https://opencode.ai/install", "-o", "/tmp/opencode-install.sh")
		if err := cmd.Run(); err != nil {
			fmt.Println("failed: curl download failed")
			return
		}
		cmd = exec.Command("bash", "/tmp/opencode-install.sh")
		err := cmd.Run()
		os.Remove("/tmp/opencode-install.sh")
		if err != nil {
			fmt.Printf("failed: %v\n", err)
			return
		}
	} else {
		fmt.Println("failed: no supported package manager found (brew, npm, or curl required)")
		return
	}
	if _, err := exec.LookPath("opencode"); err != nil {
		fmt.Println("installed but 'opencode' not found in PATH — you may need to restart your shell or add the install location to PATH")
		return
	}
	fmt.Println("done.")
}
