package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	dockerclient "github.com/docker/docker/client"
	"github.com/jullury/akama/internal/agent"
	"github.com/jullury/akama/internal/config"
	docker "github.com/jullury/akama/internal/docker"
	"github.com/jullury/akama/internal/storage"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the daemon and all containers (pulls images if needed)",
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

	configDir := resolveConfigDir(cfgPath)

	// Apply any binary update staged by a Telegram /update from inside Docker.
	applyPendingHostUpdate(configDir)

	// Record host OS/arch/binary-path so the daemon container can stage the
	// correct host binary when the user runs /update from Telegram.
	writeHostInfo(configDir)

	for _, s := range agent.BuiltinSkills {
		if s.Required {
			if err := agent.InstallSkill(s); err != nil {
				fmt.Fprintf(os.Stderr, "Install skill %s: %v\n", s.Name, err)
			}
		}
	}

	dcli, err := docker.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connect to Docker: %v\n", err)
		os.Exit(1)
	}

	// Pull missing images and rebuild the daemon image from the current binary.
	ensureImages(dcli)

	infraCtx, infraCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer infraCancel()

	fmt.Println("Starting infrastructure containers...")

	if err := docker.EnsureNetwork(infraCtx, dcli); err != nil {
		fmt.Fprintf(os.Stderr, "Create network: %v\n", err)
		os.Exit(1)
	}
	if err := docker.EnsureVolume(infraCtx, dcli, docker.WorkspacesVolume); err != nil {
		fmt.Fprintf(os.Stderr, "Create workspace volume: %v\n", err)
		os.Exit(1)
	}

	if _, err := resolveAndStartPostgres(infraCtx, dcli, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	if err := docker.EnsureOllamaContainer(infraCtx, dcli); err != nil {
		fmt.Fprintf(os.Stderr, "Start ollama: %v\n", err)
		os.Exit(1)
	}

	fmt.Print("Waiting for PostgreSQL...")
	pgCtx, pgCancel := context.WithTimeout(infraCtx, 90*time.Second)
	defer pgCancel()
	if err := docker.WaitHealthy(pgCtx, dcli, docker.PostgresContainer, func(ctx context.Context) error {
		db, err := storage.OpenNoMigrate(cfg.PostgresURL)
		if err != nil {
			return err
		}
		db.Close()
		return nil
	}); err != nil {
		fmt.Fprintf(os.Stderr, "\nPostgreSQL did not become healthy: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(" ready.")

	ctx := context.Background()

	healthy, err := docker.ContainerHealthy(ctx, dcli, docker.DaemonContainer)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Check daemon container: %v\n", err)
		os.Exit(1)
	}
	ensureOllamaModels(ctx, dcli)

	if healthy {
		fmt.Println("akama daemon is already running")
		return
	}

	logsDir := filepath.Join(configDir, "logs")

	if err := docker.EnsureDaemonContainer(ctx, dcli, configDir, logsDir); err != nil {
		fmt.Fprintf(os.Stderr, "Start daemon container: %v\n", err)
		os.Exit(1)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if running, _ := docker.ContainerRunning(ctx, dcli, docker.DaemonContainer); running {
			fmt.Println("akama daemon started")
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Fprintln(os.Stderr, "akama daemon container did not start in time")
	os.Exit(1)
}

// ensureImages ensures all required Docker images are present.
// The daemon image is built locally from source when it doesn't exist or
// the CLI binary has been updated since the image was last built.
func ensureImages(dcli *dockerclient.Client) {
	ctx := context.Background()

	// Infrastructure images: pull if missing.
	for _, img := range []struct{ ref, label string }{
		{docker.PostgresImage, "PostgreSQL"},
		{docker.OllamaImage, "Ollama"},
	} {
		if !docker.ImageExists(ctx, dcli, img.ref) {
			fmt.Printf("Pulling %s image...\n", img.label)
			if err := docker.PullImage(ctx, dcli, img.ref, os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Pull %s: %v\n", img.label, err)
				os.Exit(1)
			}
		}
	}

	// Daemon image: build only when missing or when the CLI binary is newer
	// than the existing image (indicating a binary update has occurred).
	exePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Get executable path: %v\n", err)
		os.Exit(1)
	}

	needsBuild := true
	if docker.ImageExists(ctx, dcli, docker.DaemonImage) {
		exeInfo, err := os.Stat(exePath)
		if err == nil {
			imgCreated := docker.ImageCreatedAt(ctx, dcli, docker.DaemonImage)
			if !imgCreated.IsZero() && !exeInfo.ModTime().After(imgCreated) {
				needsBuild = false
			}
		}
	}

	if needsBuild {
		fmt.Println("Building akama-daemon image...")
		if err := docker.BuildDaemonImage(ctx, dcli, exePath, config.Version, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Build daemon image: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Println("Daemon image is up to date.")
	}
}

// resolveAndStartPostgres brings the postgres container up, discovers its bound
// port, saves it to config if it changed, and returns the port string.
func resolveAndStartPostgres(ctx context.Context, dcli *dockerclient.Client, cfg *config.Config) (string, error) {
	running, _ := docker.ContainerRunning(ctx, dcli, docker.PostgresContainer)

	var pgPort string
	if running {
		if p := docker.GetContainerHostPort(ctx, dcli, docker.PostgresContainer, "5432/tcp"); p != "" {
			pgPort = p
		} else {
			pgPort = cfg.PostgresPort
		}
	} else {
		if err := docker.RemoveContainer(ctx, dcli, docker.PostgresContainer); err != nil {
			return "", fmt.Errorf("remove stale postgres container: %w", err)
		}
		var err error
		pgPort, err = docker.FindFreePort(docker.PostgresPort)
		if err != nil {
			return "", fmt.Errorf("find free port: %w", err)
		}
		if pgPort != docker.PostgresPort {
			fmt.Printf("Port %s in use; using port %s for PostgreSQL.\n", docker.PostgresPort, pgPort)
		}
		if err := docker.EnsurePostgresContainer(ctx, dcli, pgPort); err != nil {
			return "", fmt.Errorf("start postgres: %w", err)
		}
	}

	newURL := fmt.Sprintf("postgres://akama:akama@127.0.0.1:%s/akama", pgPort)
	if pgPort != cfg.PostgresPort || newURL != cfg.PostgresURL {
		cfg.PostgresPort = pgPort
		cfg.PostgresURL = newURL
		cfg.Save(cfgPath)
	}
	return pgPort, nil
}

func ensureOllamaModels(ctx context.Context, dcli *dockerclient.Client) {
	pullCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if err := docker.PullAndEnsureModel(pullCtx, dcli, "nomic-embed-text"); err != nil {
		fmt.Fprintf(os.Stderr, "Ensure ollama model: %v\n", err)
	}
}

// resolveConfigDir returns the absolute directory containing the config file.
func resolveConfigDir(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[2:])
	}
	return filepath.Dir(path)
}

// writeHostInfo saves GOOS, GOARCH, and the current binary path to
// <configDir>/.host_info so the daemon container can download the right
// binary when the user triggers /update from Telegram.
func writeHostInfo(configDir string) {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	content := fmt.Sprintf("GOOS=%s\nGOARCH=%s\nBINARY_PATH=%s\n", runtime.GOOS, runtime.GOARCH, exePath)
	os.WriteFile(filepath.Join(configDir, ".host_info"), []byte(content), 0644)
}

// applyPendingHostUpdate checks whether a Telegram /update staged a new host
// binary in <configDir>/akama-update.  If so, it replaces the running binary
// and removes the staging files so the next ensureImages call picks up the
// fresh binary.
func applyPendingHostUpdate(configDir string) {
	sentinelPath := filepath.Join(configDir, ".pending-host-update")
	updatePath := filepath.Join(configDir, "akama-update")

	if _, err := os.Stat(sentinelPath); err != nil {
		return // no pending update
	}

	data, err := os.ReadFile(updatePath)
	if err != nil {
		os.Remove(sentinelPath)
		return
	}

	exePath, err := os.Executable()
	if err != nil {
		os.Remove(sentinelPath)
		return
	}

	if err := os.WriteFile(exePath, data, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Apply pending update: %v\n", err)
		os.Remove(sentinelPath)
		return
	}

	os.Remove(updatePath)
	os.Remove(sentinelPath)
	fmt.Printf("Applied pending update to %s\n", exePath)
}
