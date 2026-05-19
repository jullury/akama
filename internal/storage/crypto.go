package storage

import (
	"database/sql"

	"github.com/jullury/akama/internal/crypto"
)

var encKey []byte

func SetEncryptionKey(key []byte) {
	encKey = key
}

func encryptToken(tok string) string {
	if len(encKey) == 0 || tok == "" {
		return tok
	}
	enc, err := crypto.Encrypt(encKey, tok)
	if err != nil {
		return tok
	}
	return enc
}

func decryptToken(tok string) string {
	if len(encKey) == 0 || tok == "" {
		return tok
	}
	dec, err := crypto.Decrypt(encKey, tok)
	if err != nil {
		return tok // tolerate plaintext tokens (pre-migration)
	}
	return dec
}

func MigrateTokenEncryption(db *sql.DB) error {
	if len(encKey) == 0 {
		return nil
	}
	var version string
	err := db.QueryRow(`SELECT value FROM meta WHERE key = 'encryption_version'`).Scan(&version)
	if err == nil && version == "1" {
		return nil // already migrated
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Migrate connections
	rows, err := tx.Query(`SELECT id, git_token FROM connections`)
	if err != nil {
		return err
	}
	var connIDs []int64
	var connToks []string
	for rows.Next() {
		var id int64
		var tok string
		if err := rows.Scan(&id, &tok); err != nil {
			rows.Close()
			return err
		}
		connIDs = append(connIDs, id)
		connToks = append(connToks, tok)
	}
	rows.Close()
	for i, id := range connIDs {
		enc := encryptToken(connToks[i])
		if _, err := tx.Exec(`UPDATE connections SET git_token = ? WHERE id = ?`, enc, id); err != nil {
			return err
		}
	}

	// Migrate jobs
	rows, err = tx.Query(`SELECT id, git_token FROM jobs`)
	if err != nil {
		return err
	}
	var jobIDs []int64
	var jobToks []string
	for rows.Next() {
		var id int64
		var tok string
		if err := rows.Scan(&id, &tok); err != nil {
			rows.Close()
			return err
		}
		jobIDs = append(jobIDs, id)
		jobToks = append(jobToks, tok)
	}
	rows.Close()
	for i, id := range jobIDs {
		enc := encryptToken(jobToks[i])
		if _, err := tx.Exec(`UPDATE jobs SET git_token = ? WHERE id = ?`, enc, id); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`INSERT INTO meta (key, value) VALUES ('encryption_version', '1') ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`); err != nil {
		return err
	}

	return tx.Commit()
}
