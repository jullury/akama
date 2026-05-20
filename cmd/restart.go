package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jullury/akama/internal/config"
	docker "github.com/jullury/akama/internal/docker"
	"github.com/spf13/cobra"
)

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the daemon (stop if running, recreate, then start)",
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

	configDir := resolveConfigDir(cfgPath)
	applyPendingHostUpdate(configDir)
	writeHostInfo(configDir)

	dcli, err := docker.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connect to Docker: %v\n", err)
		os.Exit(1)
	}

	ensureImages(dcli)

	ctx := context.Background()

	// Stop daemon container if running.
	running, _ := docker.ContainerRunning(ctx, dcli, docker.DaemonContainer)
	if running {
		fmt.Println("Stopping akama daemon...")
		if err := docker.StopContainer(ctx, dcli, docker.DaemonContainer); err != nil {
			fmt.Fprintf(os.Stderr, "Stop daemon: %v\n", err)
			os.Exit(1)
		}
	}

	// Always remove the daemon container to pick up config changes.
	if err := docker.RemoveContainer(ctx, dcli, docker.DaemonContainer); err != nil {
		fmt.Fprintf(os.Stderr, "Remove daemon container: %v\n", err)
		os.Exit(1)
	}

	// Ensure infra containers are running.
	infraCtx, infraCancel := context.WithTimeout(ctx, 3*time.Minute)
	defer infraCancel()

	pgPort := cfg.PostgresPort
	if pgPort == "" {
		pgPort = docker.PostgresPort
	}
	if err := docker.EnsureInfraContainers(infraCtx, dcli, pgPort); err != nil {
		fmt.Fprintf(os.Stderr, "Start infrastructure: %v\n", err)
		os.Exit(1)
	}
	if err := docker.EnsureVolume(infraCtx, dcli, docker.WorkspacesVolume); err != nil {
		fmt.Fprintf(os.Stderr, "Create workspace volume: %v\n", err)
		os.Exit(1)
	}

	logsDir := filepath.Join(configDir, "logs")

	if err := docker.EnsureDaemonContainer(ctx, dcli, configDir, logsDir); err != nil {
		fmt.Fprintf(os.Stderr, "Start daemon container: %v\n", err)
		os.Exit(1)
	}

	// Wait up to 10s for the daemon container to be running.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if r, _ := docker.ContainerRunning(ctx, dcli, docker.DaemonContainer); r {
			fmt.Println("akama daemon restarted")
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Fprintln(os.Stderr, "akama daemon container did not start in time")
	os.Exit(1)
}
