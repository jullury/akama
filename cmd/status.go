package cmd

import (
	"fmt"
	"os"

	"github.com/jullury/akama/internal/config"
	"github.com/jullury/akama/internal/daemon"
	"github.com/jullury/akama/internal/storage"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show running/stopped + active job count",
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

	if !daemon.IsRunning(cfg.PIDPath) {
		fmt.Println("stopped")
		return
	}

	pid, _ := daemon.ReadPID(cfg.PIDPath)
	db, err := storage.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Open DB: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	count, err := storage.CountActiveJobs(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Count jobs: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("running (pid %d), %d active jobs\n", pid, count)
}
