package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	if healthy {
		fmt.Println("akama daemon is already running")
		return
	}

	configDir := resolveConfigDir(cfgPath)
	logsDir := filepath.Join(configDir, "logs")

	if err := docker.EnsureDaemonContainer(ctx, dcli, configDir, logsDir); err != nil {
		fmt.Fprintf(os.Stderr, "Start daemon container: %v\n", err)
		os.Exit(1)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if running, _ := docker.ContainerRunning(ctx, dcli, docker.DaemonContainer); running {
			fmt.Println("akama daemon started")
			go func() {
				pullCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
				defer cancel()
				docker.PullAndEnsureModel(pullCtx, dcli, "nomic-embed-text") //nolint:errcheck
			}()
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Fprintln(os.Stderr, "akama daemon container did not start in time")
	os.Exit(1)
}

// ensureImages ensures all required Docker images are present.
// The daemon image is always built locally from source so it stays in sync
// with the running binary. The worker image is pulled from GHCR when available.
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

	// Daemon image: always build locally from current source so that the
	// container binary stays in sync with the CLI binary and any local changes
	// are picked up on every start.
	fmt.Println("Building akama-daemon image...")
	exePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Get executable path: %v\n", err)
		os.Exit(1)
	}
	if err := docker.BuildDaemonImage(ctx, dcli, exePath, config.Version, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Build daemon image: %v\n", err)
		os.Exit(1)
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

// resolveConfigDir returns the absolute directory containing the config file.
func resolveConfigDir(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[2:])
	}
	return filepath.Dir(path)
}
