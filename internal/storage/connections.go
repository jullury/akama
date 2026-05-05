package storage

import (
	"database/sql"
)

type Connection struct {
	ID            int64
	ChatID        int64
	Provider      string
	RepoURL       string
	GitToken      string
	DefaultBranch string
}

func SaveConnection(db *sql.DB, chatID int64, provider, repoURL, gitToken, defaultBranch string) error {
	_, err := db.Exec(`INSERT INTO connections (chat_id, provider, repo_url, git_token, default_branch) VALUES (?, ?, ?, ?, ?)`,
		chatID, provider, repoURL, gitToken, defaultBranch)
	return err
}

func ListConnections(db *sql.DB, chatID int64) ([]*Connection, error) {
	rows, err := db.Query(`SELECT id, chat_id, provider, repo_url, git_token, default_branch FROM connections WHERE chat_id = ?`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var conns []*Connection
	for rows.Next() {
		c := &Connection{}
		err := rows.Scan(&c.ID, &c.ChatID, &c.Provider, &c.RepoURL, &c.GitToken, &c.DefaultBranch)
		if err != nil {
			return nil, err
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
	row := db.QueryRow(`SELECT id, chat_id, provider, repo_url, git_token, default_branch FROM connections WHERE id = ?`, id)
	c := &Connection{}
	err := row.Scan(&c.ID, &c.ChatID, &c.Provider, &c.RepoURL, &c.GitToken, &c.DefaultBranch)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

func UpdateConnectionDefaultBranch(db *sql.DB, chatID int64, repoURL, defaultBranch string) error {
	_, err := db.Exec(`UPDATE connections SET default_branch = ? WHERE chat_id = ? AND repo_url = ?`,
		defaultBranch, chatID, repoURL)
	return err
}

func FindConnectionByRepo(db *sql.DB, chatID int64, repoURL string) (*Connection, error) {
	row := db.QueryRow(`SELECT id, chat_id, provider, repo_url, git_token, default_branch FROM connections WHERE chat_id = ? AND repo_url = ?`,
		chatID, repoURL)
	c := &Connection{}
	err := row.Scan(&c.ID, &c.ChatID, &c.Provider, &c.RepoURL, &c.GitToken, &c.DefaultBranch)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}
