package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	docker "github.com/jullury/akama/internal/docker"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the daemon container gracefully",
	Run:   runStop,
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

func runStop(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	dcli, err := docker.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connect to Docker: %v\n", err)
		os.Exit(1)
	}

	status, err := docker.ContainerStatus(ctx, dcli, docker.DaemonContainer)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Check daemon: %v\n", err)
		os.Exit(1)
	}

	if status != "running" {
		fmt.Println("akama daemon is not running")
		return
	}

	fmt.Println("Stopping akama daemon...")
	if err := docker.StopContainer(ctx, dcli, docker.DaemonContainer); err != nil {
		fmt.Fprintf(os.Stderr, "Stop daemon: %v\n", err)
		os.Exit(1)
	}

	// Wait for container to stop
	deadline := time.After(30 * time.Second)
	for {
		select {
		case <-deadline:
			fmt.Fprintln(os.Stderr, "timed out waiting for daemon to stop")
			os.Exit(1)
		case <-time.After(500 * time.Millisecond):
			s, _ := docker.ContainerStatus(ctx, dcli, docker.DaemonContainer)
			if s != "running" {
				fmt.Println("akama daemon stopped")
				return
			}
		}
	}
}
