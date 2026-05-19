package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jullury/akama/internal/agent"
	"github.com/jullury/akama/internal/config"
	docker "github.com/jullury/akama/internal/docker"
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
	installSkills()

	fmt.Print("Workspace directory (default /workspaces): ")
	var ws string
	fmt.Scanln(&ws)
	if ws == "" {
		ws = "/workspaces"
	}
	cfg.WorkspaceDir = ws

	if err := cfg.Save(path); err != nil {
		fmt.Fprintf(os.Stderr, "Save config: %v\n", err)
		return
	}

	if err := os.Chmod(path, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Chmod config: %v\n", err)
	}

	// Docker phase: build images, pull containers, provision infra
	fmt.Println("\nSetting up Docker infrastructure...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	dcli, err := docker.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connect to Docker daemon: %v\n", err)
		fmt.Println("Make sure Docker is running and try again.")
		return
	}

	dockerClient := dcli

	// Pull daemon image
	fmt.Println("Pulling akama-daemon image...")
	if err := docker.PullImage(ctx, dockerClient, docker.DaemonImage, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Pull daemon image: %v\n", err)
		return
	}
	fmt.Println("Daemon image ready.")

	// Pull infrastructure images
	fmt.Println("Pulling PostgreSQL (pgvector)...")
	if err := docker.PullImage(ctx, dockerClient, docker.PostgresImage, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Pull postgres: %v\n", err)
		return
	}
	fmt.Println("PostgreSQL image ready.")

	fmt.Println("Pulling Ollama...")
	if err := docker.PullImage(ctx, dockerClient, docker.OllamaImage, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Pull ollama: %v\n", err)
		return
	}
	fmt.Println("Ollama image ready.")

	// Create network and start containers
	logDir := filepath.Join(filepath.Dir(cfg.LogPath), "logs")
	os.MkdirAll(logDir, 0700)

	if err := docker.EnsureNetwork(ctx, dockerClient); err != nil {
		fmt.Fprintf(os.Stderr, "Create network: %v\n", err)
		return
	}
	fmt.Println("Network akama-net ready.")

	if err := docker.EnsurePostgresContainer(ctx, dockerClient, "5432"); err != nil {
		fmt.Fprintf(os.Stderr, "Start postgres: %v\n", err)
		return
	}
	fmt.Print("Waiting for PostgreSQL to become healthy...")
	dbHealthCtx, dbCancel := context.WithTimeout(ctx, 30*time.Second)
	defer dbCancel()
	if err := docker.WaitHealthy(dbHealthCtx, dockerClient, docker.PostgresContainer, func(ctx context.Context) error {
		db, err := storage.Open(cfg.PostgresURL)
		if err != nil {
			return err
		}
		db.Close()
		return nil
	}); err != nil {
		fmt.Fprintf(os.Stderr, "\nPostgreSQL health check: %v\n", err)
		return
	}
	fmt.Println(" ready.")

	if err := docker.EnsureOllamaContainer(ctx, dockerClient); err != nil {
		fmt.Fprintf(os.Stderr, "Start ollama: %v\n", err)
		return
	}
	fmt.Print("Waiting for Ollama to become healthy...")
	ollamaCtx, ollamaCancel := context.WithTimeout(ctx, 30*time.Second)
	defer ollamaCancel()
	if err := docker.WaitHealthy(ollamaCtx, dockerClient, docker.OllamaContainer, func(ctx context.Context) error {
		running, err := docker.ContainerRunning(ctx, dockerClient, docker.OllamaContainer)
		if err != nil {
			return err
		}
		if !running {
			return fmt.Errorf("not running")
		}
		return nil
	}); err != nil {
		fmt.Fprintf(os.Stderr, "\nOllama health check: %v\n", err)
		return
	}
	fmt.Println(" ready.")

	// Run migrations
	db, err := storage.Open(cfg.PostgresURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Open DB: %v\n", err)
		return
	}
	defer db.Close()

	if cfg.AdminUserID != 0 {
		if err := storage.AddAuthorizedUser(db, cfg.AdminUserID, "admin", cfg.AdminUserID); err != nil {
			fmt.Fprintf(os.Stderr, "Add admin user: %v\n", err)
		}
	}

	// Pull embedding model
	fmt.Println("Pulling embedding model (nomic-embed-text)...")
	if err := docker.PullAndEnsureModel(ctx, dockerClient, "nomic-embed-text"); err != nil {
		fmt.Fprintf(os.Stderr, "Pull embedding model: %v\n", err)
	}
	fmt.Println("Embedding model ready.")

	db.Close()

	fmt.Println("\nSetup complete. Run `akama start` to start the daemon.")
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

func installSkills() {
	for _, s := range agent.BuiltinSkills {
		if s.Required {
			fmt.Printf("Installing %s (required)... ", s.Name)
			if err := agent.InstallSkill(s); err != nil {
				fmt.Printf("failed: %v\n", err)
			} else {
				fmt.Println("done.")
			}
		}
	}

	fmt.Println("\nOptional skills (press Enter to skip):")
	optionalIdx := 0
	indexMap := map[int]int{}
	for i, s := range agent.BuiltinSkills {
		if s.Required {
			continue
		}
		optionalIdx++
		indexMap[optionalIdx] = i
		fmt.Printf("  %d. %-25s — %s\n", optionalIdx, s.Name, s.Description)
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
		for _, i := range indexMap {
			toInstall = append(toInstall, agent.BuiltinSkills[i])
		}
	} else {
		for _, part := range strings.Split(input, ",") {
			var n int
			fmt.Sscanf(strings.TrimSpace(part), "%d", &n)
			if i, ok := indexMap[n]; ok {
				toInstall = append(toInstall, agent.BuiltinSkills[i])
			}
		}
	}

	for _, s := range toInstall {
		fmt.Printf("Installing %s... ", s.Name)
		if err := agent.InstallSkill(s); err != nil {
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
