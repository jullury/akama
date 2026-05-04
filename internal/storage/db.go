package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func Open(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS jobs (
		id                  INTEGER PRIMARY KEY AUTOINCREMENT,
		chat_id             INTEGER NOT NULL,
		issue_id            TEXT    NOT NULL DEFAULT '',
		issue_title         TEXT    NOT NULL DEFAULT '',
		issue_body          TEXT    NOT NULL DEFAULT '',
		issue_url           TEXT    NOT NULL DEFAULT '',
		repo_url            TEXT    NOT NULL DEFAULT '',
		provider            TEXT    NOT NULL DEFAULT '',
		git_token           TEXT    NOT NULL DEFAULT '',
		agent               TEXT    NOT NULL DEFAULT 'claude',
		agent_model         TEXT    NOT NULL DEFAULT '',
		status              TEXT    NOT NULL DEFAULT 'pending',
		workspace_path      TEXT    NOT NULL DEFAULT '',
		branch_name         TEXT    NOT NULL DEFAULT '',
		pr_url              TEXT    NOT NULL DEFAULT '',
		notification_msg_id INTEGER NOT NULL DEFAULT 0,
		error_msg           TEXT    NOT NULL DEFAULT '',
		agent_output        TEXT    NOT NULL DEFAULT '',
		created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS conversations (
		chat_id    INTEGER NOT NULL,
		platform   TEXT    NOT NULL DEFAULT 'telegram',
		state      TEXT    NOT NULL DEFAULT 'idle',
		data       TEXT    NOT NULL DEFAULT '{}',
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (chat_id, platform)
	);

	CREATE TABLE IF NOT EXISTS connections (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		chat_id    INTEGER NOT NULL,
		provider   TEXT    NOT NULL DEFAULT '',
		repo_url   TEXT    NOT NULL DEFAULT '',
		git_token  TEXT    NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_jobs_chat  ON jobs(chat_id, status);
	CREATE INDEX IF NOT EXISTS idx_jobs_notif ON jobs(notification_msg_id);

	CREATE TABLE IF NOT EXISTS user_config (
		chat_id      INTEGER PRIMARY KEY,
		git_name     TEXT    NOT NULL DEFAULT '',
		git_email    TEXT    NOT NULL DEFAULT '',
		agent_model  TEXT    NOT NULL DEFAULT '',
		updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := db.Exec(schema)
	return err
}
