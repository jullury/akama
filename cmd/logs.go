package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	docker "github.com/jullury/akama/internal/docker"
	"github.com/spf13/cobra"
)

var (
	logsFollow bool
	logsTail   string
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show daemon container logs",
	Run:   runLogs,
}

func init() {
	rootCmd.AddCommand(logsCmd)
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
	logsCmd.Flags().StringVar(&logsTail, "tail", "all", "Number of lines to show from the end")
}

func runLogs(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	dcli, err := docker.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connect to Docker: %v\n", err)
		os.Exit(1)
	}

	r, err := docker.ContainerLogs(ctx, dcli, docker.DaemonContainer, logsFollow, logsTail)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Get logs: %v\n", err)
		os.Exit(1)
	}
	defer r.Close()

	if _, err := io.Copy(os.Stdout, r); err != nil {
		fmt.Fprintf(os.Stderr, "Read logs: %v\n", err)
		os.Exit(1)
	}
}
