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

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Rebuild and restart the daemon image",
	Run:   runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) {
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

	// Stop daemon before rebuilding
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

	// Pull latest image
	fmt.Print("Pulling latest daemon image...")
	if err := docker.PullImage(ctx, dcli, docker.DaemonImage, nil); err != nil {
		fmt.Fprintf(os.Stderr, "\nPull image: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(" done.")

	homeDir, _ := os.UserHomeDir()
	configPath := cfgPath
	if strings.HasPrefix(configPath, "~/") {
		configPath = filepath.Join(homeDir, configPath[2:])
	}
	logDir := filepath.Join(filepath.Dir(cfg.LogPath), "logs")

	// Start new daemon
	if err := docker.EnsureDaemonContainer(ctx, dcli, configPath, logDir); err != nil {
		fmt.Fprintf(os.Stderr, "Start daemon: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("akama updated and restarted.")
}
