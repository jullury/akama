package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/jullury/akama/internal/config"
	docker "github.com/jullury/akama/internal/docker"
	"github.com/jullury/akama/internal/storage"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon, database, and job status",
	Run:   runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load config: %v\n", err)
		os.Exit(1)
	}

	dcli, err := docker.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connect to Docker: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	daemonStatus, _ := docker.ContainerStatus(ctx, dcli, docker.DaemonContainer)
	postgresStatus, _ := docker.ContainerStatus(ctx, dcli, docker.PostgresContainer)
	ollamaStatus, _ := docker.ContainerStatus(ctx, dcli, docker.OllamaContainer)

	fmt.Printf("daemon:    %s\n", daemonStatus)
	fmt.Printf("postgres:  %s\n", postgresStatus)
	fmt.Printf("ollama:    %s\n", ollamaStatus)

	if daemonStatus == "running" {
		db, err := storage.OpenNoMigrate(cfg.PostgresURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Open DB: %v\n", err)
			return
		}
		defer db.Close()

		count, err := storage.CountActiveJobs(db)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Count jobs: %v\n", err)
			return
		}
		fmt.Printf("active jobs: %d\n", count)
	} else {
		fmt.Println("active jobs: 0 (daemon not running)")
	}
}
