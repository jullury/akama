package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

type Conversation struct {
	ChatID   int64
	Platform string
	State    string
	Data     map[string]interface{}
}

func GetConversation(db *sql.DB, chatID int64, platform string) (*Conversation, error) {
	row := db.QueryRow(`SELECT chat_id, platform, state, data FROM conversations WHERE chat_id = $1 AND platform = $2`,
		chatID, platform)
	var dataStr string
	c := &Conversation{}
	err := row.Scan(&c.ChatID, &c.Platform, &c.State, &dataStr)
	if err == sql.ErrNoRows {
		return &Conversation{ChatID: chatID, Platform: platform, State: "idle", Data: make(map[string]interface{})}, nil
	}
	if err != nil {
		return nil, err
	}
	c.Data = make(map[string]interface{})
	json.Unmarshal([]byte(dataStr), &c.Data)
	return c, nil
}

func SetConversationState(db *sql.DB, chatID int64, platform, state string, data map[string]interface{}) error {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}
	_, err = db.Exec(`INSERT INTO conversations (chat_id, platform, state, data) VALUES ($1, $2, $3, $4)
		ON CONFLICT(chat_id, platform) DO UPDATE SET state = EXCLUDED.state, data = EXCLUDED.data, updated_at = NOW()`,
		chatID, platform, state, string(dataBytes))
	return err
}

func ResetConversation(db *sql.DB, chatID int64, platform string) error {
	return SetConversationState(db, chatID, platform, "idle", make(map[string]interface{}))
}
