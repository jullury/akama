package cmd

import (
	"context"
	"fmt"
	"os"

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

	fmt.Println("Starting daemon...")
	if err := docker.EnsureDaemonContainer(ctx, dcli, cfg.WorkspaceDir, cfgPath, cfg.LogPath); err != nil {
		fmt.Fprintf(os.Stderr, "Start daemon: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("akama daemon restarted.")
}
