package storage

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func Open(postgresURL string) (*sql.DB, error) {
	db, err := sql.Open("pgx", postgresURL)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	if _, err := db.Exec("CREATE EXTENSION IF NOT EXISTS vector"); err != nil {
		return nil, fmt.Errorf("enable vector extension: %w", err)
	}
	if err := Migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func OpenNoMigrate(postgresURL string) (*sql.DB, error) {
	db, err := sql.Open("pgx", postgresURL)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return db, nil
}

func Migrate(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS jobs (
		id                  BIGSERIAL PRIMARY KEY,
		chat_id             BIGINT NOT NULL,
		issue_id            TEXT NOT NULL DEFAULT '',
		issue_title         TEXT NOT NULL DEFAULT '',
		issue_body          TEXT NOT NULL DEFAULT '',
		issue_url           TEXT NOT NULL DEFAULT '',
		repo_url            TEXT NOT NULL DEFAULT '',
		provider            TEXT NOT NULL DEFAULT '',
		git_token           TEXT NOT NULL DEFAULT '',
		agent               TEXT NOT NULL DEFAULT 'claude',
		agent_model         TEXT NOT NULL DEFAULT '',
		status              TEXT NOT NULL DEFAULT 'pending',
		workspace_path      TEXT NOT NULL DEFAULT '',
		branch_name         TEXT NOT NULL DEFAULT '',
		pr_url              TEXT NOT NULL DEFAULT '',
		notification_msg_id BIGINT NOT NULL DEFAULT 0,
		error_msg           TEXT NOT NULL DEFAULT '',
		agent_output        TEXT NOT NULL DEFAULT '',
		default_branch      TEXT NOT NULL DEFAULT 'main',
		images              TEXT NOT NULL DEFAULT '',
		group_id            TEXT NOT NULL DEFAULT '',
		plan                TEXT NOT NULL DEFAULT '',
		question_count      INTEGER NOT NULL DEFAULT 0,
		last_review_check_at TIMESTAMPTZ,
		created_at          TIMESTAMPTZ DEFAULT NOW(),
		updated_at          TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS conversations (
		chat_id    BIGINT NOT NULL,
		platform   TEXT NOT NULL DEFAULT 'telegram',
		state      TEXT NOT NULL DEFAULT 'idle',
		data       TEXT NOT NULL DEFAULT '{}',
		updated_at TIMESTAMPTZ DEFAULT NOW(),
		PRIMARY KEY (chat_id, platform)
	);

	CREATE TABLE IF NOT EXISTS connections (
		id             BIGSERIAL PRIMARY KEY,
		chat_id        BIGINT NOT NULL,
		provider       TEXT NOT NULL DEFAULT '',
		repo_url       TEXT NOT NULL DEFAULT '',
		git_token      TEXT NOT NULL DEFAULT '',
		default_branch TEXT NOT NULL DEFAULT '',
		agent          TEXT NOT NULL DEFAULT '',
		agent_model    TEXT NOT NULL DEFAULT '',
		last_polled_at TIMESTAMPTZ,
		created_at     TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_jobs_chat  ON jobs(chat_id, status);
	CREATE INDEX IF NOT EXISTS idx_jobs_notif ON jobs(notification_msg_id);
	CREATE INDEX IF NOT EXISTS idx_jobs_group ON jobs(group_id);

	CREATE TABLE IF NOT EXISTS user_config (
		chat_id     BIGINT PRIMARY KEY,
		git_name    TEXT NOT NULL DEFAULT '',
		git_email   TEXT NOT NULL DEFAULT '',
		agent_model TEXT NOT NULL DEFAULT '',
		agent       TEXT NOT NULL DEFAULT '',
		updated_at  TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS authorized_users (
		chat_id    BIGINT PRIMARY KEY,
		role       TEXT NOT NULL DEFAULT 'user',
		added_by   BIGINT NOT NULL DEFAULT 0,
		created_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS meta (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT ''
	);

	CREATE TABLE IF NOT EXISTS job_embeddings (
		job_id     BIGINT PRIMARY KEY REFERENCES jobs(id) ON DELETE CASCADE,
		embedding  vector(768),
		indexed_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_job_embeddings_vec
		ON job_embeddings USING ivfflat (embedding vector_cosine_ops)
		WITH (lists = 100);

	CREATE TABLE IF NOT EXISTS knowledge_usage (
		id                 BIGSERIAL PRIMARY KEY,
		job_id             BIGINT UNIQUE REFERENCES jobs(id) ON DELETE CASCADE,
		knowledge_file_path TEXT,
		similar_jobs_found  INT DEFAULT 0,
		similar_job_ids     TEXT,
		agent_referenced    BOOLEAN DEFAULT FALSE,
		created_at         TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_knowledge_usage_job_id
		ON knowledge_usage (job_id);
	`
	if _, err := db.Exec(schema); err != nil {
		return err
	}

	// Migrations for existing databases
	if _, err := db.Exec(`ALTER TABLE jobs ADD COLUMN IF NOT EXISTS question_count INTEGER NOT NULL DEFAULT 0`); err != nil {
		log.Printf("[migrate] adding question_count column: %v (may already exist)", err)
	}

	return nil
}

// IncrementQuestionCount increments the question count for a job and returns the new count.
func IncrementQuestionCount(db *sql.DB, jobID int64) (int, error) {
	var count int
	err := db.QueryRow(`UPDATE jobs SET question_count = question_count + 1, updated_at = NOW() WHERE id = $1 RETURNING question_count`, jobID).Scan(&count)
	return count, err
}

// GetQuestionCount returns the current question count for a job.
func GetQuestionCount(db *sql.DB, jobID int64) (int, error) {
	var count int
	err := db.QueryRow(`SELECT question_count FROM jobs WHERE id = $1`, jobID).Scan(&count)
	return count, err
}

func FindConnectionsByChat(db *sql.DB, chatID int64) ([]*Connection, error) {
	rows, err := db.Query(`SELECT id, chat_id, provider, repo_url, git_token, default_branch, agent, agent_model, last_polled_at FROM connections WHERE chat_id = $1 ORDER BY id`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Connection
	for rows.Next() {
		c := &Connection{}
		var lastPolled sql.NullTime
		if err := rows.Scan(&c.ID, &c.ChatID, &c.Provider, &c.RepoURL, &c.GitToken, &c.DefaultBranch, &c.Agent, &c.AgentModel, &lastPolled); err != nil {
			return nil, err
		}
		c.GitToken = decryptToken(c.GitToken)
		if lastPolled.Valid {
			c.LastPolledAt = &lastPolled.Time
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func DeleteConnection(db *sql.DB, id int) error {
	_, err := db.Exec(`DELETE FROM connections WHERE id = $1`, id)
	return err
}

func ResetConversationState(db *sql.DB, chatID int64) error {
	_, err := db.Exec(`UPDATE conversations SET state = 'idle', data = '{}' WHERE chat_id = $1`, chatID)
	return err
}
