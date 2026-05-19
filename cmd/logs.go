package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	docker "github.com/jullury/akama/internal/docker"
	"github.com/spf13/cobra"
)

var logsFollow bool
var logsTail string

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show or tail daemon container logs",
	Run:   runLogs,
}

func init() {
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
	logsCmd.Flags().StringVar(&logsTail, "tail", "100", "Number of lines to show from the end (default 100, use 'all' for everything)")
	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) {
	dcli, err := docker.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connect to Docker: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	reader, err := docker.ContainerLogs(ctx, dcli, docker.DaemonContainer, logsFollow, logsTail)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Get container logs: %v\n", err)
		os.Exit(1)
	}
	defer reader.Close()

	if _, err := io.Copy(os.Stdout, reader); err != nil && err != io.EOF {
		fmt.Fprintf(os.Stderr, "Read logs: %v\n", err)
		os.Exit(1)
	}
}
