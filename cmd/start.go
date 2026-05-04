package cmd

import (
	"fmt"
	"os"

	"github.com/jullury/akama/internal/config"
	"github.com/jullury/akama/internal/daemon"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Fork daemon to background",
	Run:   runStart,
}

func init() {
	rootCmd.AddCommand(startCmd)
}

func runStart(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load config: %v\n", err)
		os.Exit(1)
	}

	if daemon.IsRunning(cfg.PIDPath) {
		fmt.Fprintln(os.Stderr, "akama is already running")
		os.Exit(1)
	}

	pid, err := daemon.ForkDaemon(cfg.LogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Start daemon: %v\n", err)
		os.Exit(1)
	}

	if err := daemon.WritePID(cfg.PIDPath, pid); err != nil {
		fmt.Fprintf(os.Stderr, "Write PID: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("akama daemon started (pid %d)\n", pid)
}
