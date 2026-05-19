package storage

import "database/sql"

type UserConfig struct {
	ChatID     int64
	GitName    string
	GitEmail   string
	AgentModel string
	Agent      string
}

func GetUserConfig(db *sql.DB, chatID int64) (*UserConfig, error) {
	row := db.QueryRow(`SELECT chat_id, git_name, git_email, agent_model, agent FROM user_config WHERE chat_id = $1`, chatID)
	c := &UserConfig{}
	err := row.Scan(&c.ChatID, &c.GitName, &c.GitEmail, &c.AgentModel, &c.Agent)
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
		INSERT INTO user_config (chat_id, git_name, git_email, agent_model, agent)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT(chat_id) DO UPDATE SET
			git_name    = EXCLUDED.git_name,
			git_email   = EXCLUDED.git_email,
			agent_model = EXCLUDED.agent_model,
			agent       = EXCLUDED.agent,
			updated_at  = NOW()`,
		cfg.ChatID, cfg.GitName, cfg.GitEmail, cfg.AgentModel, cfg.Agent)
	return err
}
