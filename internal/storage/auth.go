package storage

import (
	"database/sql"
)

type AuthorizedUser struct {
	ChatID    int64
	Role      string
	AddedBy   int64
}

func IsAuthorized(db *sql.DB, chatID int64) bool {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM authorized_users WHERE chat_id = ?`, chatID).Scan(&count)
	if err != nil {
		return false
	}
	return count > 0
}

func IsAdmin(db *sql.DB, chatID int64) bool {
	var role string
	err := db.QueryRow(`SELECT role FROM authorized_users WHERE chat_id = ?`, chatID).Scan(&role)
	if err != nil {
		return false
	}
	return role == "admin"
}

func ListAuthorizedUsers(db *sql.DB) ([]*AuthorizedUser, error) {
	rows, err := db.Query(`SELECT chat_id, role, added_by FROM authorized_users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AuthorizedUser
	for rows.Next() {
		u := &AuthorizedUser{}
		if err := rows.Scan(&u.ChatID, &u.Role, &u.AddedBy); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func AddAuthorizedUser(db *sql.DB, chatID int64, role string, addedBy int64) error {
	_, err := db.Exec(`INSERT OR IGNORE INTO authorized_users (chat_id, role, added_by) VALUES (?, ?, ?)`, chatID, role, addedBy)
	return err
}

func RemoveAuthorizedUser(db *sql.DB, chatID int64) error {
	_, err := db.Exec(`DELETE FROM authorized_users WHERE chat_id = ?`, chatID)
	return err
}

func CountAuthorizedUsers(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM authorized_users`).Scan(&count)
	return count, err
}
