package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/jullury/akama/internal/config"
	"github.com/jullury/akama/internal/daemon"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Send SIGTERM to daemon via PID file",
	Run:   runStop,
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

func runStop(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load config: %v\n", err)
		os.Exit(1)
	}

	pid, err := daemon.ReadPID(cfg.PIDPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "akama is not running")
		os.Exit(1)
	}

	if err := daemon.StopDaemon(cfg.PIDPath); err != nil {
		fmt.Fprintf(os.Stderr, "Stop daemon: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("akama daemon stopping (pid %d), waiting...\n", pid)
	deadline := time.After(35 * time.Second)
	for {
		select {
		case <-deadline:
			fmt.Fprintln(os.Stderr, "timed out waiting for daemon to exit")
			os.Exit(1)
		case <-time.After(300 * time.Millisecond):
			if !daemon.IsProcessAlive(pid) {
				fmt.Println("akama daemon stopped")
				return
			}
		}
	}
}
