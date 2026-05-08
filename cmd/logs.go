package cmd

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jullury/akama/internal/config"
	"github.com/spf13/cobra"
)

const archiveSuffix = ".gz"

var (
	logsFollow bool
	logsAll     bool
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show or tail application logs",
	Run:   runLogs,
}

func init() {
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
	logsCmd.Flags().BoolVarP(&logsAll, "all", "a", false, "Show all log files, not just today's")
	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load config: %v\n", err)
		os.Exit(1)
	}

	logsDir := filepath.Join(filepath.Dir(cfg.LogPath), "logs")
	base := filepath.Base(cfg.LogPath)
	ext := filepath.Ext(base)
	name := base[:len(base)-len(ext)]

	today := time.Now().Format("2006-01-02")
	curLog := filepath.Join(logsDir, fmt.Sprintf("%s-%s%s", name, today, ext))

	if logsAll {
		// Print all archived log files first, then fall through to today's log.
		entries, err := os.ReadDir(logsDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Read log dir: %v\n", err)
			os.Exit(1)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			fpath := filepath.Join(logsDir, e.Name())
			if fpath == curLog {
				continue // today's log is handled below
			}
			if strings.HasSuffix(fpath, archiveSuffix) {
				printGzipFile(fpath)
			} else {
				printFile(fpath)
			}
		}
	}

	if !logsFollow {
		// Print today's log and exit.
		printFile(curLog)
		return
	}

	// Follow mode: open today's log once and stream from the beginning,
	// then keep polling for new content. This captures everything written
	// before and after the tail starts (including daemon startup messages).
	f, err := os.Open(curLog)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Open log: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	buf := make([]byte, 4096)
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

func printFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Open %s: %v\n", path, err)
		return
	}
	defer f.Close()

	io.Copy(os.Stdout, f)
}

func printGzipFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Open %s: %v\n", path, err)
		return
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Read %s: %v\n", path, err)
		return
	}
	defer gz.Close()

	io.Copy(os.Stdout, gz)
}
