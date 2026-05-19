package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/jullury/akama/internal/config"
	docker "github.com/jullury/akama/internal/docker"
	"github.com/jullury/akama/internal/storage"
	"github.com/spf13/cobra"

	// SQLite driver for reading old database
	_ "modernc.org/sqlite"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate from SQLite to PostgreSQL",
	Long: `One-shot migration: reads data from SQLite and writes it into PostgreSQL.
Requires PostgreSQL to be running (use 'akama db start' first).`,
	Run: runMigrate,
}

func init() {
	rootCmd.AddCommand(migrateCmd)
}

func runMigrate(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load config: %v\n", err)
		os.Exit(1)
	}

	// Ensure PostgreSQL is running
	ctx := context.Background()
	dcli, derr := docker.NewClient()
	if derr == nil {
		status, _ := docker.ContainerStatus(ctx, dcli, docker.PostgresContainer)
		if status != "running" {
			fmt.Fprintln(os.Stderr, "PostgreSQL container is not running. Run `akama db start` first.")
			os.Exit(1)
		}
	}

	// Open PostgreSQL
	fmt.Print("PostgreSQL URL [" + cfg.PostgresURL + "]: ")
	var url string
	fmt.Scanln(&url)
	if url == "" {
		url = cfg.PostgresURL
	}

	pg, err := storage.Open(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Open PostgreSQL: %v\n", err)
		os.Exit(1)
	}
	defer pg.Close()

	// Find SQLite database
	sqlitePaths := []string{
		"~/.akama/akama.db",
		"/root/.akama/akama.db",
	}
	homeDir, _ := os.UserHomeDir()
	var sqlitePath string
	for _, p := range sqlitePaths {
		expanded := strings.Replace(p, "~", homeDir, 1)
		if _, err := os.Stat(expanded); err == nil {
			sqlitePath = expanded
			break
		}
	}

	if sqlitePath == "" {
		fmt.Fprintln(os.Stderr, "No SQLite database found at ~/.akama/akama.db or /root/.akama/akama.db")
		fmt.Print("Enter SQLite database path: ")
		fmt.Scanln(&sqlitePath)
		if sqlitePath == "" {
			os.Exit(1)
		}
	}

	fmt.Printf("Migrating from %s...\n", sqlitePath)

	sqlite, err := sql.Open("sqlite", sqlitePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Open SQLite: %v\n", err)
		os.Exit(1)
	}
	defer sqlite.Close()

	// Migrate each table
	migrateTable(sqlite, pg, "authorized_users",
		"SELECT chat_id, role, added_by, created_at FROM authorized_users",
		"INSERT INTO authorized_users (chat_id, role, added_by, created_at) VALUES ($1, $2, $3, $4) ON CONFLICT (chat_id) DO NOTHING")

	migrateTable(sqlite, pg, "connections",
		"SELECT id, chat_id, provider, repo_url, git_token, auth_token, refresh_token, token_expiry, created_at, updated_at FROM connections",
		"INSERT INTO connections (chat_id, provider, repo_url, git_token, auth_token, refresh_token, token_expiry, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT (chat_id, provider, repo_url) DO NOTHING")

	migrateTable(sqlite, pg, "conversations",
		"SELECT id, chat_id, platform, thread_id, title, created_at, updated_at FROM conversations",
		"INSERT INTO conversations (chat_id, platform, thread_id, title, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (chat_id, platform) DO NOTHING")

	// Jobs table — use INSERT ... ON CONFLICT DO NOTHING, skip id (let SERIAL assign new ones)
	migrateTable(sqlite, pg, "jobs",
		"SELECT chat_id, message_id, issue_url, repo_url, pr_url, status, plan, result, error_message, model, created_at, started_at, completed_at, branch_name FROM jobs",
		"INSERT INTO jobs (chat_id, message_id, issue_url, repo_url, pr_url, status, plan, result, error_message, model, created_at, started_at, completed_at, branch_name) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)")

	migrateTable(sqlite, pg, "settings",
		"SELECT key, value FROM meta",
		"INSERT INTO meta (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value")

	fmt.Println("\nMigration complete.")
	fmt.Println("You may now remove the old SQLite database: rm", sqlitePath)
}

func migrateTable(sqlite, pg *sql.DB, name, selectSQL, insertSQL string) {
	rows, err := sqlite.Query(selectSQL)
	if err != nil {
		// Skip tables that may not exist in older schemas
		if strings.Contains(err.Error(), "no such table") {
			fmt.Printf("  %s: table not found (skipping)\n", name)
			return
		}
		fmt.Fprintf(os.Stderr, "  %s: query error: %v\n", name, err)
		return
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	values := make([]interface{}, len(cols))
	valuePtrs := make([]interface{}, len(cols))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	count := 0
	tx, err := pg.Begin()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  %s: begin tx: %v\n", name, err)
		return
	}

	stmt, err := tx.Prepare(insertSQL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  %s: prepare: %v\n", name, err)
		tx.Rollback()
		return
	}
	defer stmt.Close()

	for rows.Next() {
		if err := rows.Scan(valuePtrs...); err != nil {
			fmt.Fprintf(os.Stderr, "  %s: scan row: %v\n", name, err)
			tx.Rollback()
			return
		}

		// Convert []byte back to string for PostgreSQL
		args := make([]interface{}, len(values))
		for i, v := range values {
			if b, ok := v.([]byte); ok {
				args[i] = string(b)
			} else {
				args[i] = v
			}
		}

		if _, err := stmt.Exec(args...); err != nil {
			// Skip duplicate key errors
			if strings.Contains(err.Error(), "duplicate key") {
				continue
			}
			fmt.Fprintf(os.Stderr, "  %s: insert: %v\n", name, err)
			tx.Rollback()
			return
		}
		count++
	}

	if err := tx.Commit(); err != nil {
		fmt.Fprintf(os.Stderr, "  %s: commit: %v\n", name, err)
		return
	}

	fmt.Printf("  %s: %d rows migrated\n", name, count)
}
