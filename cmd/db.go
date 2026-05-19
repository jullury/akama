package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jullury/akama/internal/config"
	docker "github.com/jullury/akama/internal/docker"
	"github.com/jullury/akama/internal/storage"
	"github.com/spf13/cobra"
)

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Manage PostgreSQL database containers",
}

var dbStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start PostgreSQL container",
	Run:   runDBStart,
}

var dbStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop PostgreSQL container",
	Run:   runDBStop,
}

var dbResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Drop and recreate the database",
	Run:   runDBReset,
}

var dbStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show database connectivity status",
	Run:   runDBStatus,
}

func init() {
	rootCmd.AddCommand(dbCmd)
	dbCmd.AddCommand(dbStartCmd)
	dbCmd.AddCommand(dbStopCmd)
	dbCmd.AddCommand(dbResetCmd)
	dbCmd.AddCommand(dbStatusCmd)
}

func runDBStart(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	dcli, err := docker.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connect to Docker: %v\n", err)
		os.Exit(1)
	}

	if err := docker.EnsureNetwork(ctx, dcli); err != nil {
		fmt.Fprintf(os.Stderr, "Ensure network: %v\n", err)
		os.Exit(1)
	}

	if err := docker.EnsurePostgresContainer(ctx, dcli, "5432"); err != nil {
		fmt.Fprintf(os.Stderr, "Start postgres: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("PostgreSQL is ready.")
}

func runDBStop(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	dcli, err := docker.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connect to Docker: %v\n", err)
		os.Exit(1)
	}

	status, _ := docker.ContainerStatus(ctx, dcli, docker.PostgresContainer)
	if status != "running" {
		fmt.Println("PostgreSQL is not running.")
		return
	}

	fmt.Println("Stopping PostgreSQL...")
	if err := docker.StopContainer(ctx, dcli, docker.PostgresContainer); err != nil {
		fmt.Fprintf(os.Stderr, "Stop postgres: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("PostgreSQL stopped.")
}

func runDBReset(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load config: %v\n", err)
		os.Exit(1)
	}

	fmt.Print("This will drop the akama database. Continue? (y/N): ")
	var resp string
	fmt.Scanln(&resp)
	if resp != "y" {
		fmt.Println("Aborted.")
		return
	}

	// Connect to postgres default database to drop akama (can't drop the one we're connected to)
	adminURL := cfg.PostgresURL
	if idx := strings.LastIndex(adminURL, "/"); idx >= 0 {
		adminURL = adminURL[:idx] + "/postgres"
	}
	db, err := storage.OpenNoMigrate(adminURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Open admin DB: %v\n", err)
		os.Exit(1)
	}
	if _, err := db.Exec("DROP DATABASE IF EXISTS akama"); err != nil {
		fmt.Fprintf(os.Stderr, "Drop database: %v\n", err)
		os.Exit(1)
	}
	if _, err := db.Exec("CREATE DATABASE akama"); err != nil {
		fmt.Fprintf(os.Stderr, "Create database: %v\n", err)
		os.Exit(1)
	}
	db.Close()

	// Reconnect and run migrations on the fresh database
	db, err = storage.Open(cfg.PostgresURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Reconnect: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := storage.Migrate(db); err != nil {
		fmt.Fprintf(os.Stderr, "Migrate: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Database reset complete.")
}

func runDBStatus(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load config: %v\n", err)
		os.Exit(1)
	}

	dcli, err := docker.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connect to Docker: %v\n", err)
		os.Exit(1)
	}

	pgStatus, _ := docker.ContainerStatus(context.Background(), dcli, docker.PostgresContainer)
	fmt.Printf("postgres container: %s\n", pgStatus)

	if pgStatus == "running" {
		db, err := storage.Open(cfg.PostgresURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Open DB: %v\n", err)
			return
		}
		defer db.Close()

		// Quick connectivity test
		var n int
		if err := db.QueryRow("SELECT 1").Scan(&n); err != nil {
			fmt.Fprintf(os.Stderr, "DB ping failed: %v\n", err)
			return
		}
		fmt.Println("database connectivity: ok")
	}
}
