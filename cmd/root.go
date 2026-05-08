package cmd

import (
	"fmt"
	"os"

	"github.com/jullury/akama/internal/config"
	"github.com/spf13/cobra"
)

var cfgPath string

var rootCmd = &cobra.Command{
	Use:     "akama",
	Short:   "Akama — AI coding agent orchestration via Telegram",
	Version: config.Version,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgPath, "config", "~/.akama/config.yaml", "config file path")
}
