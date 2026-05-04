package storage

import "database/sql"

type UserConfig struct {
	ChatID     int64
	GitName    string
	GitEmail   string
	AgentModel string
}

func GetUserConfig(db *sql.DB, chatID int64) (*UserConfig, error) {
	row := db.QueryRow(`SELECT chat_id, git_name, git_email, agent_model FROM user_config WHERE chat_id = ?`, chatID)
	c := &UserConfig{}
	err := row.Scan(&c.ChatID, &c.GitName, &c.GitEmail, &c.AgentModel)
	if err == sql.ErrNoRows {
		return &UserConfig{ChatID: chatID}, nil
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func SetUserConfig(db *sql.DB, cfg *UserConfig) error {
	_, err := db.Exec(`
		INSERT INTO user_config (chat_id, git_name, git_email, agent_model)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(chat_id) DO UPDATE SET
			git_name    = excluded.git_name,
			git_email   = excluded.git_email,
			agent_model = excluded.agent_model,
			updated_at  = CURRENT_TIMESTAMP`,
		cfg.ChatID, cfg.GitName, cfg.GitEmail, cfg.AgentModel)
	return err
}
