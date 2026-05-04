package cmd

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jullury/akama/internal/config"
	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Tail ~/.akama/akama.log",
	Run:   runLogs,
}

func init() {
	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load config: %v\n", err)
		os.Exit(1)
	}

	f, err := os.Open(cfg.LogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Open log: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Stat log: %v\n", err)
		os.Exit(1)
	}

	if info.Size() > 4096 {
		f.Seek(-4096, 2)
	}

	io.Copy(os.Stdout, f)

	buf := make([]byte, 1024)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			os.Stdout.Write(buf[:n])
		}
		if err != nil {
			time.Sleep(200 * time.Millisecond)
		}
	}
}
