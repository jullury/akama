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
		default_branch      TEXT    NOT NULL DEFAULT 'main',
		images              TEXT    NOT NULL DEFAULT '',
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
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		chat_id        INTEGER NOT NULL,
		provider       TEXT    NOT NULL DEFAULT '',
		repo_url       TEXT    NOT NULL DEFAULT '',
		git_token      TEXT    NOT NULL DEFAULT '',
		default_branch TEXT    NOT NULL DEFAULT '',
		created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_jobs_chat  ON jobs(chat_id, status);
	CREATE INDEX IF NOT EXISTS idx_jobs_notif ON jobs(notification_msg_id);

	CREATE TABLE IF NOT EXISTS user_config (
		chat_id      INTEGER PRIMARY KEY,
		git_name     TEXT    NOT NULL DEFAULT '',
		git_email    TEXT    NOT NULL DEFAULT '',
		agent_model  TEXT    NOT NULL DEFAULT '',
		agent        TEXT    NOT NULL DEFAULT '',
		updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS authorized_users (
		chat_id    INTEGER PRIMARY KEY,
		role       TEXT    NOT NULL DEFAULT 'user',
		added_by   INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	if _, err := db.Exec(schema); err != nil {
		return err
	}
	// Add columns to existing DBs that predate these migrations.
	db.Exec(`ALTER TABLE user_config ADD COLUMN agent TEXT NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE connections ADD COLUMN default_branch TEXT NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE jobs ADD COLUMN default_branch TEXT NOT NULL DEFAULT 'main'`)
	db.Exec(`ALTER TABLE jobs ADD COLUMN images TEXT NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE jobs ADD COLUMN group_id TEXT NOT NULL DEFAULT ''`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_jobs_group ON jobs(group_id)`)
	db.Exec(`ALTER TABLE jobs ADD COLUMN plan TEXT NOT NULL DEFAULT ''`)
	return nil
}

func FindConnectionsByChat(db *sql.DB, chatID int64) ([]*Connection, error) {
	rows, err := db.Query(`SELECT id, chat_id, provider, repo_url, git_token, default_branch FROM connections WHERE chat_id = ? ORDER BY id`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Connection
	for rows.Next() {
		c := &Connection{}
		if err := rows.Scan(&c.ID, &c.ChatID, &c.Provider, &c.RepoURL, &c.GitToken, &c.DefaultBranch); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func DeleteConnection(db *sql.DB, id int) error {
	_, err := db.Exec(`DELETE FROM connections WHERE id = ?`, id)
	return err
}

func ResetConversationState(db *sql.DB, chatID int64) error {
	_, err := db.Exec(`UPDATE conversations SET state = 'idle', data = '{}' WHERE chat_id = ?`, chatID)
	return err
}
