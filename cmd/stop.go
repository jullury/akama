package cmd

import (
	"context"
	"fmt"
	"os"

	docker "github.com/jullury/akama/internal/docker"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the daemon and infrastructure containers",
	Run:   runStop,
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

func runStop(cmd *cobra.Command, args []string) {
	dcli, err := docker.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connect to Docker: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// Stop the daemon container first.
	running, _ := docker.ContainerRunning(ctx, dcli, docker.DaemonContainer)
	if running {
		fmt.Println("Stopping akama daemon...")
		if err := docker.StopContainer(ctx, dcli, docker.DaemonContainer); err != nil {
			fmt.Fprintf(os.Stderr, "Stop daemon: %v\n", err)
		} else {
			fmt.Println("akama daemon stopped")
		}
	} else {
		fmt.Println("akama daemon is not running")
	}

	// Then stop postgres and ollama.
	for _, name := range []string{docker.PostgresContainer, docker.OllamaContainer} {
		running, _ := docker.ContainerRunning(ctx, dcli, name)
		if !running {
			continue
		}
		fmt.Printf("Stopping %s...\n", name)
		if err := docker.StopContainer(ctx, dcli, name); err != nil {
			fmt.Fprintf(os.Stderr, "Stop %s: %v\n", name, err)
		} else {
			fmt.Printf("%s stopped\n", name)
		}
	}
}
