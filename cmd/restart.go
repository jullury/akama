package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jullury/akama/internal/config"
	docker "github.com/jullury/akama/internal/docker"
	"github.com/spf13/cobra"
)

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the daemon container",
	Run:   runRestart,
}

func init() {
	rootCmd.AddCommand(restartCmd)
}

func runRestart(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load config: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	dcli, err := docker.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connect to Docker: %v\n", err)
		os.Exit(1)
	}

	status, _ := docker.ContainerStatus(ctx, dcli, docker.DaemonContainer)
	if status == "running" {
		fmt.Println("Stopping daemon...")
		if err := docker.StopContainer(ctx, dcli, docker.DaemonContainer); err != nil {
			fmt.Fprintf(os.Stderr, "Stop daemon: %v\n", err)
			os.Exit(1)
		}
	}

	if err := docker.RemoveContainer(ctx, dcli, docker.DaemonContainer); err != nil {
		fmt.Fprintf(os.Stderr, "Remove daemon container: %v\n", err)
		os.Exit(1)
	}

	homeDir, _ := os.UserHomeDir()
	configPath := cfgPath
	if strings.HasPrefix(configPath, "~/") {
		configPath = filepath.Join(homeDir, configPath[2:])
	}
	logDir := filepath.Join(filepath.Dir(cfg.LogPath), "logs")

	fmt.Println("Starting daemon...")
	if err := docker.EnsureDaemonContainer(ctx, dcli, configPath, logDir); err != nil {
		fmt.Fprintf(os.Stderr, "Start daemon: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("akama daemon restarted.")
}
