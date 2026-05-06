package cmd

import (
	"fmt"
	"os"

	"github.com/jullury/akama/internal/agent"
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

	for _, s := range agent.BuiltinSkills {
		if s.Required {
			if err := agent.InstallSkill(s); err != nil {
				fmt.Fprintf(os.Stderr, "Install skill %s: %v\n", s.Name, err)
			}
		}
	}

	if daemon.IsRunning(cfg.PIDPath) {
		fmt.Fprintln(os.Stderr, "akama is already running")
		os.Exit(1)
	}

	pid, err := daemon.ForkDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Start daemon: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("akama daemon started (pid %d)\n", pid)
}
