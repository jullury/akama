package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jullury/akama/internal/agent"
	"github.com/jullury/akama/internal/config"
	docker "github.com/jullury/akama/internal/docker"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the daemon container and ensure infrastructure is running",
	Run:   runStart,
}

func init() {
	rootCmd.AddCommand(startCmd)
}

func runStart(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load config: %v\n", err)
		os.Exit(1)
	}

	for _, s := range agent.BuiltinSkills {
		if s.Required {
			if err := agent.InstallSkill(s); err != nil {
				fmt.Fprintf(os.Stderr, "Install skill %s: %v\n", s.Name, err)
			}
		}
	}

	ctx := context.Background()
	dcli, err := docker.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connect to Docker: %v\n", err)
		os.Exit(1)
	}

	dockerClient := dcli

	// Ensure network and infrastructure containers are running
	if err := docker.EnsureNetwork(ctx, dockerClient); err != nil {
		fmt.Fprintf(os.Stderr, "Ensure network: %v\n", err)
		os.Exit(1)
	}

	if err := docker.EnsurePostgresContainer(ctx, dockerClient, "5432"); err != nil {
		fmt.Fprintf(os.Stderr, "Ensure postgres: %v\n", err)
		os.Exit(1)
	}

	if err := docker.EnsureOllamaContainer(ctx, dockerClient); err != nil {
		fmt.Fprintf(os.Stderr, "Ensure ollama: %v\n", err)
		os.Exit(1)
	}

	// Build and start daemon container
	homeDir, _ := os.UserHomeDir()
	workspaceDir := cfg.WorkspaceDir
	if strings.HasPrefix(workspaceDir, "~/") {
		workspaceDir = filepath.Join(homeDir, workspaceDir[2:])
	}
	os.MkdirAll(workspaceDir, 0700)

	configPath := cfgPath
	if strings.HasPrefix(configPath, "~/") {
		configPath = filepath.Join(homeDir, configPath[2:])
	}

	logDir := filepath.Join(filepath.Dir(cfg.LogPath), "logs")

	// Build image if not present
	if err := docker.BuildImage(ctx, dockerClient, "Dockerfile", docker.DaemonImage, nil, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Build daemon image: %v\n", err)
		os.Exit(1)
	}

	if err := docker.EnsureDaemonContainer(ctx, dockerClient, workspaceDir, configPath, logDir); err != nil {
		fmt.Fprintf(os.Stderr, "Ensure daemon: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("akama daemon started.")
}
