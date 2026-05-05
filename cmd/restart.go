package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/jullury/akama/internal/config"
	"github.com/jullury/akama/internal/daemon"
	"github.com/spf13/cobra"
)

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Stop and restart daemon",
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

	if !daemon.IsRunning(cfg.PIDPath) {
		fmt.Println("akama is not running, starting fresh...")
		pid, err := daemon.ForkDaemon()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Start daemon: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("akama daemon started (pid %d)\n", pid)
		return
	}

	fmt.Println("Stopping daemon...")
	if err := daemon.StopDaemon(cfg.PIDPath); err != nil {
		fmt.Fprintf(os.Stderr, "Stop daemon: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Waiting for daemon to stop...")
	deadline := time.After(35 * time.Second)
	for {
		select {
		case <-deadline:
			fmt.Fprintln(os.Stderr, "timed out waiting for daemon to exit")
			os.Exit(1)
		case <-time.After(300 * time.Millisecond):
			if !daemon.IsRunning(cfg.PIDPath) {
				fmt.Println("Daemon stopped")
				goto start
			}
		}
	}

start:
	fmt.Println("Starting daemon...")
	pid, err := daemon.ForkDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Start daemon: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("akama daemon started (pid %d)\n", pid)
}
