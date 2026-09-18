package hub

import (
	"database/sql"
	"errors"
	"time"

	"github.com/pleware/initagent/internal/id"
	"github.com/pleware/initagent/internal/store"
)

// --- boxes ---

// ErrBoxSlugTaken reports a CreateBox whose slug collides with an existing
// box: the slug is the unique key of the boxes table.
var ErrBoxSlugTaken = errors.New("a box with this slug already exists")

// Box is the configurable PWare OS appliance (`initagent.fleet.box`): the
// logical box, distinct from the physical `fleet.host` machine it runs on.
// A host may run several boxes; each box carries its organizations, its
// narrator and its staff overrides, and syncs them down to the machine (58).
// Edition is the appliance class (ParseEdition); ConfigVersion counts the
// config writes a connector's sync has to pick up.
type Box struct {
	ID            string `json:"id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	HostID        string `json:"hostId,omitempty"`
	Edition       string `json:"edition"`
	ConfigVersion int64  `json:"configVersion"`
	CreatedAt     int64  `json:"createdAt"`
	UpdatedAt     int64  `json:"updatedAt"`
}

// boxScanner is the shared shape of sql.Row and sql.Rows Scan methods, the
// same role staffScanner plays for the staff table.
type boxScanner interface {
	Scan(dest ...any) error
}

// scanBox reads one boxes row selected in schema order:
// id, slug, name, host_id, created_at, updated_at, edition, config_version.
// A missing row is (nil, nil). host_id is NULL until the box is bound to a
// host machine.
func scanBox(row boxScanner) (*Box, error) {
	var b Box
	var hostID sql.NullString
	if err := row.Scan(&b.ID, &b.Slug, &b.Name, &hostID, &b.CreatedAt, &b.UpdatedAt,
		&b.Edition, &b.ConfigVersion); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	b.HostID = hostID.String
	return &b, nil
}

// CreateBox mints and stores a new box. The slug is unique per installation;
// a collision returns ErrBoxSlugTaken. hostID is optional: an empty host
// leaves the box unbound to a machine until a later UpdateBox. The edition
// runs through ParseEdition, so an empty input stores lite; a fresh box
// starts at config_version 1.
func (s *Store) CreateBox(slug, name, hostID, edition string) (*Box, error) {
	edition, err := ParseEdition(edition)
	if err != nil {
		return nil, err
	}
	boxID, err := id.New(id.Box)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	b := &Box{
		ID:            boxID,
		Slug:          slug,
		Name:          name,
		HostID:        hostID,
		Edition:       edition,
		ConfigVersion: 1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	_, err = s.db.Exec(`INSERT INTO boxes (id, slug, name, host_id, created_at, updated_at, edition, config_version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		b.ID, b.Slug, b.Name, nullableHostID(b.HostID), b.CreatedAt, b.UpdatedAt, b.Edition, b.ConfigVersion)
	if uniqueConstraint(err) {
		return nil, ErrBoxSlugTaken
	}
	if err != nil {
		return nil, err
	}
	if err := s.EnsureSeedBoxNarrator(boxID); err != nil {
		return nil, err
	}
	return b, nil
}

// GetBox returns one box by id. A missing box is (nil, nil).
func (s *Store) GetBox(id string) (*Box, error) {
	return scanBox(s.db.QueryRow(`SELECT id, slug, name, host_id, created_at, updated_at, edition, config_version
		FROM boxes WHERE id = ?`, id))
}

// ListBoxes returns every box on this installation, ordered by slug.
func (s *Store) ListBoxes() ([]Box, error) {
	rows, err := s.db.Query(`SELECT id, slug, name, host_id, created_at, updated_at, edition, config_version
		FROM boxes ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Box{}
	for rows.Next() {
		b, err := scanBox(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

// UpdateBox replaces the editable fields of a box, bumps its config_version
// atomically in the same statement, and refreshes its updated_at. A missing
// box is (nil, nil). An empty hostID clears the host binding. The edition
// runs through ParseEdition, so an empty input stores lite.
func (s *Store) UpdateBox(id, name, hostID, edition string) (*Box, error) {
	edition, err := ParseEdition(edition)
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec(`UPDATE boxes SET name = ?, host_id = ?, edition = ?,
		config_version = config_version + 1, updated_at = ?
		WHERE id = ?`, name, nullableHostID(hostID), edition, time.Now().Unix(), id)
	if err != nil {
		return nil, err
	}
	return s.GetBox(id)
}

// SetBoxOrgs replaces the organization set bound to a box: the old rows are
// deleted and the new ones inserted inside one transaction, so a concurrent
// reader never sees a half-written set. Duplicate ids in orgIDs collapse.
// The same transaction bumps the box's config_version so a connector's next
// sync picks the new set up.
func (s *Store) SetBoxOrgs(boxID string, orgIDs []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM box_orgs WHERE box_id = ?`, boxID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE boxes SET config_version = config_version + 1 WHERE id = ?`, boxID); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, orgID := range orgIDs {
		if seen[orgID] {
			continue
		}
		seen[orgID] = true
		if _, err := tx.Exec(`INSERT INTO box_orgs (box_id, org_id) VALUES (?, ?)`, boxID, orgID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// bumpAllBoxes advances config_version on every box. A canonical org-scoped
// staff change reaches the manifest of every box — StaffForOrg serves all
// org-scoped rows regardless of which org the box carries — so the whole
// fleet has to re-sync. It runs inside the caller's transaction so the
// write that caused it and the bump commit together.
func bumpAllBoxes(tx *store.Tx) error {
	_, err := tx.Exec(`UPDATE boxes SET config_version = config_version + 1`)
	return err
}

// bumpConfigForOrg advances config_version on the boxes bound to one
// organization. An org rename or an org staff override only changes the
// manifest of the boxes carrying that org, so unbound boxes keep their
// version. It runs inside the caller's transaction, like bumpAllBoxes.
func bumpConfigForOrg(tx *store.Tx, orgID string) error {
	_, err := tx.Exec(`UPDATE boxes SET config_version = config_version + 1
		WHERE id IN (SELECT box_id FROM box_orgs WHERE org_id = ?)`, orgID)
	return err
}

// ListBoxOrgs returns the organization ids bound to a box, ordered by id.
// A box with no organizations yields an empty slice, not nil.
func (s *Store) ListBoxOrgs(boxID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT org_id FROM box_orgs WHERE box_id = ? ORDER BY org_id`, boxID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var orgID string
		if err := rows.Scan(&orgID); err != nil {
			return nil, err
		}
		out = append(out, orgID)
	}
	return out, rows.Err()
}

// nullableHostID maps an empty host binding to SQL NULL, the column's
// "no host yet" marker. The read side (scanBox) turns NULL back into "".
func nullableHostID(hostID string) any {
	if hostID == "" {
		return nil
	}
	return hostID
}

// DeleteBox removes a box and everything bound to it in one transaction:
// its organization set, its box-scoped staff (the narrator), its sync
// tokens, and the box row itself. The cascade is hand-written because the
// schema carries no foreign keys. A missing box is (false, nil).
func (s *Store) DeleteBox(id string) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM box_orgs WHERE box_id = ?`, id); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`DELETE FROM staff WHERE scope = 'box' AND box_id = ?`, id); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`DELETE FROM box_tokens WHERE box_id = ?`, id); err != nil {
		return false, err
	}
	res, err := tx.Exec(`DELETE FROM boxes WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, nil
	}
	return true, tx.Commit()
}
