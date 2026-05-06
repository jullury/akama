package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/jullury/akama/internal/config"
	"github.com/jullury/akama/internal/daemon"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for newer version and update binary",
	Run:   runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

type githubRelease struct {
	TagName string `json:"tag_name"`
}

func runUpdate(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load config: %v\n", err)
		os.Exit(1)
	}

	currentVersion := config.Version
	if currentVersion == "dev" {
		fmt.Println("Running dev build, cannot check for updates")
		return
	}

	fmt.Printf("Current version: %s\n", currentVersion)
	fmt.Print("Checking for updates...")

	latest, err := getLatestVersion()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nFailed to check latest version: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf(" %s\n", latest)

	if !isNewerVersion(latest, currentVersion) {
		fmt.Println("Already running the latest version")
		return
	}

	fmt.Printf("New version available: %s\n", latest)

	if daemon.IsRunning(cfg.PIDPath) {
		fmt.Println("Stopping running daemon...")
		if err := daemon.StopDaemon(cfg.PIDPath); err != nil {
			fmt.Fprintf(os.Stderr, "Stop daemon: %v\n", err)
		} else {
			fmt.Println("Waiting for daemon to stop...")
			deadline := time.After(35 * time.Second)
			for {
				select {
				case <-deadline:
					fmt.Fprintln(os.Stderr, "timed out waiting for daemon to exit")
				case <-time.After(300 * time.Millisecond):
					if !daemon.IsRunning(cfg.PIDPath) {
						fmt.Println("Daemon stopped")
						goto download
					}
				}
			}
		}
	}

download:
	fmt.Println("Downloading and installing...")
	if err := downloadUpdate(); err != nil {
		fmt.Fprintf(os.Stderr, "Update failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Update installed successfully")

	fmt.Println("Starting daemon...")
	pid, err := daemon.ForkDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Start daemon: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("akama daemon started (pid %d)\n", pid)
}

func getLatestVersion() (string, error) {
	resp, err := http.Get("https://api.github.com/repos/jullury/akama/releases/latest")
	if err != nil {
		return "", fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return strings.TrimPrefix(release.TagName, "v"), nil
}

func isNewerVersion(latest, current string) bool {
	latest = strings.TrimPrefix(latest, "v")
	current = strings.TrimPrefix(current, "v")

	if latest == current {
		return false
	}

	latestParts := strings.Split(latest, ".")
	currentParts := strings.Split(current, ".")

	for i := 0; i < len(latestParts) && i < len(currentParts); i++ {
		l, _ := strconv.Atoi(latestParts[i])
		c, _ := strconv.Atoi(currentParts[i])
		if l > c {
			return true
		}
		if l < c {
			return false
		}
	}

	return len(latestParts) > len(currentParts)
}

func downloadUpdate() error {
	goos := runtime.GOOS
	arch := runtime.GOARCH

	asset := fmt.Sprintf("akama-%s-%s", goos, arch)
	url := fmt.Sprintf("https://github.com/jullury/akama/releases/latest/download/%s", asset)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %d", resp.StatusCode)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	tmpPath := filepath.Join(os.TempDir(), "akama-update")
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	out.Close()

	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	if goos == "windows" {
		exePath = exePath + ".exe"
	}

	installDir := filepath.Dir(exePath)
	if err := checkWriteAccess(installDir); err != nil {
		return fmt.Errorf("cannot write to %s: %w", installDir, err)
	}

	if err := os.Rename(tmpPath, exePath); err != nil {
		cmd := exec.Command("mv", tmpPath, exePath)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("replace binary: %w", err)
		}
	}

	return nil
}

func checkWriteAccess(dir string) error {
	testFile := filepath.Join(dir, ".write-test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return err
	}
	os.Remove(testFile)
	return nil
}
