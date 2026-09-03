package gateway

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// EnrollOffer is the JSON the hub forwards to the cockpit: one paste command
// whose URL is this gateway, never the hub's r.Host.
type EnrollOffer struct {
	Token          string `json:"token"`
	Command        string `json:"command"`
	WindowsCommand string `json:"windowsCommand"`
	ProjectID      string `json:"project_id"`
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CreateEnrollToken mints a single-use token valid for EnrollTTL.
func (s *Store) CreateEnrollToken(ctx context.Context, projectID string, ttl time.Duration) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO enroll_tokens (token_hash, project_id, created_at, expires_at)
		VALUES (?, ?, ?, ?)
	`, hashToken(token), projectID, unixTime(now), unixTime(now.Add(ttl)))
	if err != nil {
		return "", fmt.Errorf("enroll token: %w", err)
	}
	return token, nil
}

// ConsumeEnrollToken atomically validates and burns a token. The project id
// is returned so enroll can bind the device to the same prj-.
func (s *Store) ConsumeEnrollToken(ctx context.Context, token string) (projectID string, ok bool, err error) {
	now := unixTime(time.Now().UTC())
	res, err := s.db.ExecContext(ctx, `
		UPDATE enroll_tokens SET used = 1
		WHERE token_hash = ? AND used = 0 AND expires_at > ?
	`, hashToken(token), now)
	if err != nil {
		return "", false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return "", false, err
	}
	if n != 1 {
		return "", false, nil
	}
	err = s.db.QueryRowContext(ctx, `
		SELECT project_id FROM enroll_tokens WHERE token_hash = ?
	`, hashToken(token)).Scan(&projectID)
	if err != nil {
		return "", false, err
	}
	return projectID, true, nil
}
