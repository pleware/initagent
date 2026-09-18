package hub

import (
	"database/sql"
	"log"
	"time"

	"github.com/pleware/initagent/internal/brand"
	"github.com/pleware/initagent/internal/id"
)

// --- box tokens ---

// BoxToken is the credential a box's sync client presents to the hub
// (58): a machine secret with no account behind it. One box holds one
// active token at a time; minting a new one revokes the old in the same
// transaction, so rotation is atomic.
type BoxToken struct {
	Id         string `json:"id"`
	BoxId      string `json:"boxId"`
	CreatedAt  int64  `json:"createdAt"`
	LastUsedAt int64  `json:"lastUsedAt"`
}

// CreateBoxToken mints the box's sync credential and returns the secret
// exactly once, alongside the row the cockpit will list. The old active
// token for the box, if any, is revoked in the same transaction: a box
// never carries two live secrets.
func (s *Store) CreateBoxToken(boxID string) (string, BoxToken, error) {
	rowId, err := id.New(id.Token)
	if err != nil {
		return "", BoxToken{}, err
	}
	secret := brand.TokenPrefix + randomToken()
	now := time.Now().Unix()
	tx, err := s.db.Begin()
	if err != nil {
		return "", BoxToken{}, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE box_tokens SET revoked_at = ?
		WHERE box_id = ? AND revoked_at = 0`, now, boxID); err != nil {
		return "", BoxToken{}, err
	}
	if _, err := tx.Exec(`INSERT INTO box_tokens (id, box_id, token_hash, created_at)
		VALUES (?, ?, ?, ?)`, rowId, boxID, hashToken(secret), now); err != nil {
		return "", BoxToken{}, err
	}
	if err := tx.Commit(); err != nil {
		return "", BoxToken{}, err
	}
	return secret, BoxToken{Id: rowId, BoxId: boxID, CreatedAt: now}, nil
}

// ListBoxTokens returns a box's active (non-revoked) tokens, oldest first.
// A box with no live token yields an empty slice, not nil.
func (s *Store) ListBoxTokens(boxID string) ([]BoxToken, error) {
	rows, err := s.db.Query(`SELECT id, box_id, created_at, last_used_at
		FROM box_tokens WHERE box_id = ? AND revoked_at = 0 ORDER BY created_at, id`, boxID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BoxToken{}
	for rows.Next() {
		var t BoxToken
		if err := rows.Scan(&t.Id, &t.BoxId, &t.CreatedAt, &t.LastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeBoxToken stops a box token and reports whether it found a live
// one. The box_id in the predicate keeps a token id that belongs to
// another box out of reach, and stamps rather than deletes so the row
// stays in the audit trail.
func (s *Store) RevokeBoxToken(tokenId, boxID string) (bool, error) {
	res, err := s.db.Exec(`UPDATE box_tokens SET revoked_at = ?
		WHERE id = ? AND box_id = ? AND revoked_at = 0`,
		time.Now().Unix(), tokenId, boxID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// BoxTokenAuth resolves a presented secret into the box it belongs to.
//
// A revoked row is not found rather than returned-and-flagged: revocation
// has to be a hard stop here, not a field some later caller might forget
// to test. Use stamps last_used_at at minute granularity.
func (s *Store) BoxTokenAuth(secret string) (string, bool, error) {
	var (
		tokenId  string
		boxID    string
		lastUsed int64
	)
	err := s.db.QueryRow(`SELECT id, box_id, last_used_at
		FROM box_tokens WHERE token_hash = ? AND revoked_at = 0`, hashToken(secret)).
		Scan(&tokenId, &boxID, &lastUsed)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	s.touchBoxToken(tokenId, lastUsed)
	return boxID, true, nil
}

// touchBoxToken records use at minute granularity, best effort — the same
// shape touchApiToken follows: a person deciding whether to revoke does
// not need a database write on every sync, nor a failed stamp on an
// entitled one.
func (s *Store) touchBoxToken(tokenId string, lastUsed int64) {
	now := time.Now().Unix()
	if now-lastUsed < 60 {
		return
	}
	if _, err := s.db.Exec(`UPDATE box_tokens SET last_used_at = ? WHERE id = ?`, now, tokenId); err != nil {
		log.Printf("stamping last use on box token %s: %v", tokenId, err)
	}
}
