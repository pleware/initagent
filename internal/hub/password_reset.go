package hub

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/pleware/initagent/internal/auth"
	"github.com/pleware/initagent/internal/id"
	"github.com/pleware/initagent/internal/store"
)

// CreatePasswordReset stores a hashed one-time secret and retires any
// unused token this account already had.
func (s *Store) CreatePasswordReset(accountID, tokenHash string, now, expires time.Time) error {
	rowID, err := id.New(id.Reset)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE password_resets SET used_at = ? WHERE account_id = ? AND used_at = 0`,
		now.Unix(), accountID); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO password_resets (id, account_id, token_hash, expires_at, used_at, created_at)
		VALUES (?, ?, ?, ?, 0, ?)`,
		rowID, accountID, tokenHash, expires.Unix(), now.Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

// ResetAccountPassword redeems a hashed token, writes a new argon2id hash,
// and retires every unused reset for that account. Missing, used, or
// expired tokens are ErrResetToken.
func (s *Store) ResetAccountPassword(tokenHash, passwordHash string, now time.Time) (*Account, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var accountID string
	err = tx.QueryRow(`SELECT account_id FROM password_resets
		WHERE token_hash = ? AND used_at = 0 AND expires_at > ?`,
		tokenHash, now.Unix()).Scan(&accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, auth.ErrResetToken
	}
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE accounts SET password_hash = ? WHERE id = ?`, passwordHash, accountID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE password_resets SET used_at = ? WHERE account_id = ? AND used_at = 0`,
		now.Unix(), accountID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.AccountById(accountID)
}

func (s *Store) accountByResetHash(tokenHash string, now time.Time) (*Account, error) {
	var accountID string
	err := s.db.QueryRow(`SELECT account_id FROM password_resets
		WHERE token_hash = ? AND used_at = 0 AND expires_at > ?`,
		tokenHash, now.Unix()).Scan(&accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.AccountById(accountID)
}

// lastMail is the newest outbox row. Tests use it to read the reset link.
func (s *Store) lastMail() (*Mail, error) {
	m, err := scanMail(s.db.QueryRow(`SELECT id, kind, to_addr, subject, text_body, html_body, status, attempts,
		last_error, provider_id, available_at, claimed_at, created_at, sent_at
		FROM mail_outbox ORDER BY created_at DESC, id DESC LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return m, err
}

func (s *Store) countMail() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM mail_outbox`).Scan(&n)
	return n, err
}

func (s *Store) ensurePasswordResets() error {
	expiresAt := "INTEGER NOT NULL"
	usedAt := "INTEGER NOT NULL DEFAULT 0"
	createdAt := "INTEGER NOT NULL"
	if s.db.Dialect() == store.Postgres {
		expiresAt = "BIGINT NOT NULL"
		usedAt = "BIGINT NOT NULL DEFAULT 0"
		createdAt = "BIGINT NOT NULL"
	}
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS password_resets (
		id          TEXT PRIMARY KEY,
		account_id  TEXT NOT NULL,
		token_hash  TEXT NOT NULL UNIQUE,
		expires_at  ` + expiresAt + `,
		used_at     ` + usedAt + `,
		created_at  ` + createdAt + `
	)`); err != nil {
		return fmt.Errorf("password_resets: %w", err)
	}
	_, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS password_resets_account ON password_resets(account_id)`)
	return err
}
