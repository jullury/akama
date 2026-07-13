package cmd

import (
	"context"
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
	"syscall"
	"time"

	"github.com/jullury/akama/internal/config"
	"github.com/jullury/akama/internal/daemon"
	docker "github.com/jullury/akama/internal/docker"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for newer version and update (Docker container or native binary)",
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

	if !isNewerVersionStr(latest, currentVersion) {
		fmt.Println("Already running the latest version")
		return
	}

	fmt.Printf("New version available: %s\n", latest)

	// Docker mode: if the daemon runs as a container, pull the new image
	// and recreate the container instead of doing a native binary update.
	if dockerUpdate(latest) {
		return
	}

	fmt.Println("Downloading and installing...")
	if err := downloadUpdate(); err != nil {
		fmt.Fprintf(os.Stderr, "Update failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Update installed successfully")

	pid, _ := daemon.ReadPID(cfg.PIDPath)
	if daemon.IsProcessAlive(pid) {
		fmt.Printf("Stopping daemon (pid %d)...\n", pid)
		proc, _ := os.FindProcess(pid)
		if err := proc.Signal(syscall.SIGTERM); err != nil {
			fmt.Fprintf(os.Stderr, "Send SIGTERM: %v\n", err)
		} else {
			deadline := time.After(35 * time.Second)
		wait:
			for {
				select {
				case <-deadline:
					fmt.Fprintln(os.Stderr, "timed out waiting for daemon to exit")
					break wait
				case <-time.After(300 * time.Millisecond):
					if !daemon.IsProcessAlive(pid) {
						fmt.Println("Daemon stopped")
						break wait
					}
				}
			}
		}
	}

	fmt.Println("Starting daemon...")
	if _, err := daemon.ForkDaemon(); err != nil {
		fmt.Fprintf(os.Stderr, "Start daemon: %v\n", err)
		os.Exit(1)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if daemon.IsRunning(cfg.PIDPath) {
			pid, _ := daemon.ReadPID(cfg.PIDPath)
			fmt.Printf("akama daemon started (pid %d)\n", pid)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Fprintln(os.Stderr, "akama daemon did not start in time — check logs with `akama logs`")
	os.Exit(1)
}

// dockerUpdate checks whether the daemon is running as a Docker container.
// If so, it pulls the latest image, recreates the container, and returns true.
// If Docker is unavailable or the daemon container is not running, it returns
// false so the caller falls through to the native binary update path.
func dockerUpdate(newVersion string) bool {
	dcli, err := docker.NewClient()
	if err != nil {
		return false // Docker not available, fall through to native update
	}

	ctx := context.Background()

	running, _ := docker.ContainerRunning(ctx, dcli, docker.DaemonContainer)
	if !running {
		return false // daemon container not running, fall through to native update
	}

	fmt.Println("Daemon is running in Docker — pulling new image...")

	// Pull the latest daemon image from GHCR.
	if err := docker.PullImage(ctx, dcli, docker.DaemonImageRef, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Pull daemon image: %v\n", err)
		fmt.Fprintln(os.Stderr, "Falling back to native update...")
		return false
	}

	// Tag the pulled image as the local name the daemon container expects.
	if err := dcli.ImageTag(ctx, docker.DaemonImageRef, docker.DaemonImage); err != nil {
		fmt.Fprintf(os.Stderr, "Tag daemon image: %v\n", err)
		fmt.Fprintln(os.Stderr, "Falling back to native update...")
		return false
	}

	configDir := resolveConfigDir(cfgPath)
	logsDir := filepath.Join(configDir, "logs")

	// Recreate the daemon container with the new image.
	fmt.Println("Recreating daemon container...")
	if err := docker.EnsureDaemonContainer(ctx, dcli, configDir, logsDir); err != nil {
		fmt.Fprintf(os.Stderr, "Recreate daemon container: %v\n", err)
		os.Exit(1)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if running, _ := docker.ContainerRunning(ctx, dcli, docker.DaemonContainer); running {
			fmt.Printf("akama daemon updated and restarted (Docker container %s)\n", docker.DaemonContainer)
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Fprintln(os.Stderr, "akama daemon container did not start in time — check logs with `akama logs`")
	os.Exit(1)
	return false
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

func isNewerVersionStr(latest, current string) bool {
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
