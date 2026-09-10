package hub

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/pleware/initagent/internal/auth"
	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/id"
	"github.com/pleware/initagent/internal/offering"
	"github.com/pleware/initagent/internal/orgplan"
	"github.com/pleware/initagent/internal/store"
)

// OrgInvite is a pending invitation. The secret never appears here.
type OrgInvite struct {
	Id        string `json:"id"`
	OrgId     string `json:"orgId"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	ExpiresAt int64  `json:"expiresAt"`
	CreatedAt int64  `json:"createdAt"`
}

// InvitePreview is what a holder of the secret may see before redeeming.
type InvitePreview struct {
	Email     string `json:"email"`
	OrgName   string `json:"orgName"`
	Role      string `json:"role"`
	ExpiresAt int64  `json:"expiresAt"`
}

// CreateOrgInvite stores a hashed one-time secret. A second unused invite
// to the same address in the same org retires the first, like a resend.
func (s *Store) CreateOrgInvite(orgID, email, tokenHash string, role authz.Role, now, expires time.Time) (*OrgInvite, error) {
	already, err := s.orgHasEmail(orgID, email)
	if err != nil {
		return nil, err
	}
	if already {
		return nil, auth.ErrAlreadyMember
	}
	if err := s.refuseAnotherPerson(orgID); err != nil {
		return nil, err
	}
	rowID, err := id.New(id.Invite)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE org_invites SET used_at = ? WHERE org_id = ? AND email = ? AND used_at = 0`,
		now.Unix(), orgID, email); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`INSERT INTO org_invites (id, org_id, email, role, token_hash, expires_at, used_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, ?)`,
		rowID, orgID, email, string(role), tokenHash, expires.Unix(), now.Unix()); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &OrgInvite{
		Id: rowID, OrgId: orgID, Email: email, Role: string(role),
		ExpiresAt: expires.Unix(), CreatedAt: now.Unix(),
	}, nil
}

// ListOrgInvites returns unused, unexpired invitations for an organization.
func (s *Store) ListOrgInvites(orgID string, now time.Time) ([]OrgInvite, error) {
	rows, err := s.db.Query(`SELECT id, org_id, email, role, expires_at, created_at
		FROM org_invites WHERE org_id = ? AND used_at = 0 AND expires_at > ?
		ORDER BY created_at, id`, orgID, now.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OrgInvite{}
	for rows.Next() {
		var inv OrgInvite
		if err := rows.Scan(&inv.Id, &inv.OrgId, &inv.Email, &inv.Role, &inv.ExpiresAt, &inv.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// RevokeOrgInvite retires an unused invite. Missing or already used is nil.
func (s *Store) RevokeOrgInvite(orgID, inviteID string, now time.Time) (bool, error) {
	res, err := s.db.Exec(`UPDATE org_invites SET used_at = ? WHERE id = ? AND org_id = ? AND used_at = 0`,
		now.Unix(), inviteID, orgID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// PeekInvite returns the public facts for a live secret. Missing, used, or
// expired is (nil, nil).
func (s *Store) PeekInvite(tokenHash string, now time.Time) (*InvitePreview, error) {
	var preview InvitePreview
	err := s.db.QueryRow(`SELECT i.email, o.name, i.role, i.expires_at
		FROM org_invites i JOIN orgs o ON o.id = i.org_id
		WHERE i.token_hash = ? AND i.used_at = 0 AND i.expires_at > ?`,
		tokenHash, now.Unix()).Scan(&preview.Email, &preview.OrgName, &preview.Role, &preview.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &preview, err
}

// AcceptOrgInvite redeems a hashed token. A new person is created without an
// organization; an existing account is attached. Invite-join must not mint a
// second org (`08`).
func (s *Store) AcceptOrgInvite(tokenHash, email, passwordHash, locale, existingID string, now time.Time) (*Account, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var inviteID, orgID, role string
	err = tx.QueryRow(`SELECT id, org_id, role FROM org_invites
		WHERE token_hash = ? AND used_at = 0 AND expires_at > ?`,
		tokenHash, now.Unix()).Scan(&inviteID, &orgID, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, auth.ErrInviteToken
	}
	if err != nil {
		return nil, err
	}

	accountID := existingID
	if accountID == "" {
		accountID, err = id.New(id.Account)
		if err != nil {
			return nil, err
		}
		loc, err := auth.NormalizeLocale(locale)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`INSERT INTO accounts (id, email, password_hash, is_admin, locale, created_at)
			VALUES (?, ?, ?, 0, ?, ?)`, accountID, email, passwordHash, loc, now.Unix()); err != nil {
			if uniqueConstraint(err) {
				return nil, auth.ErrEmailTaken
			}
			return nil, err
		}
	}

	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM org_members WHERE org_id = ? AND account_id = ?`,
		orgID, accountID).Scan(&n); err != nil {
		return nil, err
	}
	if n == 0 {
		if err := s.refuseAnotherPersonTx(tx, orgID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`INSERT INTO org_members (org_id, account_id, role, created_at)
			VALUES (?, ?, ?, ?)`, orgID, accountID, role, now.Unix()); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(`UPDATE org_invites SET used_at = ? WHERE id = ? AND used_at = 0`,
		now.Unix(), inviteID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.AccountById(accountID)
}

func (s *Store) refuseAnotherPersonTx(tx *store.Tx, orgID string) error {
	if s.offering != offering.Hosted {
		return nil
	}
	var plan string
	var members int
	if err := tx.QueryRow(`SELECT plan FROM orgs WHERE id = ?`, orgID).Scan(&plan); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM org_members WHERE org_id = ?`, orgID).Scan(&members); err != nil {
		return err
	}
	limit := orgplan.Caps(s.offering, orgplan.ID(plan)).People
	if orgplan.AllowsAnother(members, limit) {
		return nil
	}
	return planLimitError{Wall: "people", Limit: limit}
}

func (s *Store) orgHasEmail(orgID, email string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM org_members m JOIN accounts a ON a.id = m.account_id
		WHERE m.org_id = ? AND a.email = ?`, orgID, email).Scan(&n)
	return n > 0, err
}

func (s *Store) ensureOrgInvites() error {
	expiresAt := "INTEGER NOT NULL"
	usedAt := "INTEGER NOT NULL DEFAULT 0"
	createdAt := "INTEGER NOT NULL"
	if s.db.Dialect() == store.Postgres {
		expiresAt = "BIGINT NOT NULL"
		usedAt = "BIGINT NOT NULL DEFAULT 0"
		createdAt = "BIGINT NOT NULL"
	}
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS org_invites (
		id          TEXT PRIMARY KEY,
		org_id      TEXT NOT NULL,
		email       TEXT NOT NULL,
		role        TEXT NOT NULL,
		token_hash  TEXT NOT NULL UNIQUE,
		expires_at  ` + expiresAt + `,
		used_at     ` + usedAt + `,
		created_at  ` + createdAt + `
	)`); err != nil {
		return fmt.Errorf("org_invites: %w", err)
	}
	_, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS org_invites_org ON org_invites(org_id)`)
	return err
}
