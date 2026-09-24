package hub

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/pleware/initagent/internal/store"
)

// --- box narrator ---

// A box's narrator is its own 1:1 being, not an installation-scoped staff
// member. It lives in box_narrator keyed by box_id — the box's identity is
// the narrator's identity — so there is no minted id and no scope column.
// The slug is the fixed marker narratorSlug, emitted by the manifest, never
// stored as a key.

// scanNarrator reads one box_narrator row selected in schema order:
// name, locale, age, big_five, brief, word_budget, avatar_model_3d,
// soul_core, voice, biological_gender, created_at, updated_at.
// A missing row is (nil, nil).
func scanNarrator(row staffScanner) (*Narrator, error) {
	var n Narrator
	var bigFive string
	if err := row.Scan(&n.Name, &n.Locale, &n.Age, &bigFive, &n.Brief, &n.WordBudget,
		&n.AvatarModel3D, &n.SoulCore, &n.Voice, &n.BiologicalGender, &n.CreatedAt, &n.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	n.Slug = narratorSlug
	if bigFive != "" {
		if err := json.Unmarshal([]byte(bigFive), &n.BigFive); err != nil {
			return nil, fmt.Errorf("narrator: decode big_five: %w", err)
		}
	}
	return &n, nil
}

// GetBoxNarrator returns one box's narrator. A box whose narrator has not
// been seeded yet is (nil, nil).
func (s *Store) GetBoxNarrator(boxID string) (*Narrator, error) {
	return scanNarrator(s.db.QueryRow(`SELECT name, locale, age, big_five, brief, word_budget, avatar_model_3d, soul_core, voice, biological_gender, created_at, updated_at
		FROM box_narrator WHERE box_id = ?`, boxID))
}

// upsertNarratorTx is the SELECT/UPDATE/INSERT core of a narrator write. It
// runs inside the caller's transaction: an existing row is updated (and nil
// is returned so the caller re-reads the committed row), while a missing row
// is inserted and the freshly built narrator returned. The config_version
// bump the write causes is the caller's business.
func upsertNarratorTx(tx *store.Tx, boxID, name, locale, avatarModel3D, brief, soulCore, voice, biologicalGender string, age, wordBudget int, bigFive Character) (*Narrator, error) {
	bigFiveJSON, err := encodeBigFive(bigFive)
	if err != nil {
		return nil, err
	}
	var existing string
	err = tx.QueryRow(`SELECT box_id FROM box_narrator WHERE box_id = ?`, boxID).Scan(&existing)
	if err == nil {
		if _, err = tx.Exec(`UPDATE box_narrator SET name = ?, locale = ?, avatar_model_3d = ?, brief = ?, age = ?, word_budget = ?, soul_core = ?, voice = ?, biological_gender = ?, big_five = ?, updated_at = ?
			WHERE box_id = ?`, name, locale, avatarModel3D, brief, age, wordBudget, soulCore, voice, biologicalGender, bigFiveJSON, time.Now().Unix(), boxID); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	now := time.Now().Unix()
	n := &Narrator{
		Slug:             narratorSlug,
		Name:             name,
		Locale:           locale,
		AvatarModel3D:    avatarModel3D,
		Brief:            brief,
		Age:              age,
		WordBudget:       wordBudget,
		SoulCore:         soulCore,
		Voice:            voice,
		BiologicalGender: biologicalGender,
		BigFive:          bigFive,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if _, err = tx.Exec(`INSERT INTO box_narrator (box_id, name, locale, age, big_five, brief, word_budget, avatar_model_3d, soul_core, voice, biological_gender, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		boxID, n.Name, n.Locale, n.Age, bigFiveJSON, n.Brief, n.WordBudget, n.AvatarModel3D, n.SoulCore, n.Voice, n.BiologicalGender, n.CreatedAt, n.UpdatedAt); err != nil {
		return nil, err
	}
	return n, nil
}

// EnsureSeedNarrator creates the narrator of a box — the box's own "Ania"
// (st_b_pi) — when it is missing and leaves it alone otherwise, so content
// written over the seed survives a restart. Idempotent; the seed does not
// bump the box's config_version (CreateBox already starts at version 1).
func (s *Store) EnsureSeedNarrator(boxID string) error {
	var existing string
	err := s.db.QueryRow(`SELECT box_id FROM box_narrator WHERE box_id = ?`, boxID).Scan(&existing)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// The seed's age is 30 — a grown, credible voice — and its word budget is
	// 120, the box's shipped default for how many words a spoken answer may
	// take.
	if _, err := upsertNarratorTx(tx, boxID, "Ania", "pl", "", "", "", "pl_PL-gosia-medium", "female", 30, 120, neutralCharacter()); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateBoxNarrator writes the narrator of one box and bumps the box's
// config_version in the same transaction, so a connector's next sync picks
// the edited narrator up. CreateBox seeds the row, so the normal path is an
// update; the upsert core also creates the row when it is missing. The bump
// is single-box (bumpBoxConfig): a narrator edit changes only this box's
// manifest.
func (s *Store) UpdateBoxNarrator(boxID, name, locale, avatarModel3D, brief, soulCore, voice, biologicalGender string, age, wordBudget int, bigFive Character) (*Narrator, error) {
	if err := validateBiologicalGender(biologicalGender); err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	n, err := upsertNarratorTx(tx, boxID, name, locale, avatarModel3D, brief, soulCore, voice, biologicalGender, age, wordBudget, bigFive)
	if err != nil {
		return nil, err
	}
	if err := bumpBoxConfig(tx, boxID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if n != nil {
		return n, nil
	}
	return s.GetBoxNarrator(boxID)
}
