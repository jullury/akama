package storage

import (
	"database/sql"
	"time"
)

type Connection struct {
	ID            int64
	ChatID        int64
	Provider      string
	RepoURL       string
	GitToken      string
	DefaultBranch string
	Agent         string
	AgentModel    string
	LastPolledAt  *time.Time
}

func SaveConnection(db *sql.DB, chatID int64, provider, repoURL, gitToken, defaultBranch string) error {
	_, err := db.Exec(`INSERT INTO connections (chat_id, provider, repo_url, git_token, default_branch) VALUES (?, ?, ?, ?, ?)`,
		chatID, provider, repoURL, encryptToken(gitToken), defaultBranch)
	return err
}

func ListConnections(db *sql.DB, chatID int64) ([]*Connection, error) {
	rows, err := db.Query(`SELECT id, chat_id, provider, repo_url, git_token, default_branch, agent, agent_model, last_polled_at FROM connections WHERE chat_id = ?`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var conns []*Connection
	for rows.Next() {
		c := &Connection{}
		var lastPolled sql.NullTime
		err := rows.Scan(&c.ID, &c.ChatID, &c.Provider, &c.RepoURL, &c.GitToken, &c.DefaultBranch, &c.Agent, &c.AgentModel, &lastPolled)
		if err != nil {
			return nil, err
		}
		c.GitToken = decryptToken(c.GitToken)
		if lastPolled.Valid {
			c.LastPolledAt = &lastPolled.Time
		}
		conns = append(conns, c)
	}
	return conns, nil
}

func DeleteAllConnections(db *sql.DB, chatID int64) error {
	_, err := db.Exec(`DELETE FROM connections WHERE chat_id = ?`, chatID)
	return err
}

func GetConnectionByID(db *sql.DB, id int64) (*Connection, error) {
	row := db.QueryRow(`SELECT id, chat_id, provider, repo_url, git_token, default_branch, agent, agent_model, last_polled_at FROM connections WHERE id = ?`, id)
	c := &Connection{}
	var lastPolled sql.NullTime
	err := row.Scan(&c.ID, &c.ChatID, &c.Provider, &c.RepoURL, &c.GitToken, &c.DefaultBranch, &c.Agent, &c.AgentModel, &lastPolled)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.GitToken = decryptToken(c.GitToken)
	if lastPolled.Valid {
		c.LastPolledAt = &lastPolled.Time
	}
	return c, nil
}

func UpdateConnectionDefaultBranch(db *sql.DB, chatID int64, repoURL, defaultBranch string) error {
	_, err := db.Exec(`UPDATE connections SET default_branch = ? WHERE chat_id = ? AND repo_url = ?`,
		defaultBranch, chatID, repoURL)
	return err
}

func FindConnectionByRepo(db *sql.DB, chatID int64, repoURL string) (*Connection, error) {
	row := db.QueryRow(`SELECT id, chat_id, provider, repo_url, git_token, default_branch, agent, agent_model, last_polled_at FROM connections WHERE chat_id = ? AND repo_url = ?`,
		chatID, repoURL)
	c := &Connection{}
	var lastPolled sql.NullTime
	err := row.Scan(&c.ID, &c.ChatID, &c.Provider, &c.RepoURL, &c.GitToken, &c.DefaultBranch, &c.Agent, &c.AgentModel, &lastPolled)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.GitToken = decryptToken(c.GitToken)
	if lastPolled.Valid {
		c.LastPolledAt = &lastPolled.Time
	}
	return c, nil
}

func SetConnectionAgent(db *sql.DB, id int64, agent, agentModel string) error {
	_, err := db.Exec(`UPDATE connections SET agent = ?, agent_model = ? WHERE id = ?`, agent, agentModel, id)
	return err
}

func UpdateConnectionToken(db *sql.DB, chatID int64, repoURL, newToken string) error {
	_, err := db.Exec(`UPDATE connections SET git_token = ? WHERE chat_id = ? AND repo_url = ?`,
		encryptToken(newToken), chatID, repoURL)
	return err
}

func ListAllConnections(db *sql.DB) ([]*Connection, error) {
	rows, err := db.Query(`SELECT id, chat_id, provider, repo_url, git_token, default_branch, agent, agent_model, last_polled_at FROM connections ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var conns []*Connection
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
		conns = append(conns, c)
	}
	return conns, rows.Err()
}

func UpdateConnectionLastPolled(db *sql.DB, id int64, t time.Time) error {
	_, err := db.Exec(`UPDATE connections SET last_polled_at = ? WHERE id = ?`, t, id)
	return err
}
