package hub

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/pleware/initagent/internal/id"
)

// --- skills ---

// ErrSkillNameTaken reports a CreateSkill or UpdateSkill whose name collides
// with an existing skill: the name is the unique key of the skills table.
var ErrSkillNameTaken = errors.New("a skill with this name already exists")

// encodeMCP turns an optional MCP config into its column form: the empty
// string when absent, JSON otherwise. The explicit nil branch matters because
// json.Marshal((*MCPConfig)(nil)) yields "null", while the schema's convention
// is the empty string; the read side treats "null" as a stored config, so the
// two sides must agree.
func encodeMCP(mcp *MCPConfig) (string, error) {
	if mcp == nil {
		return "", nil
	}
	b, err := json.Marshal(mcp)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// skillScanner is the shared shape of sql.Row and sql.Rows Scan methods.
type skillScanner interface {
	Scan(dest ...any) error
}

// scanSkill reads one skills row selected in schema order:
// id, name, description, body, mcp, enabled, created_by, created_at, updated_at.
// A missing row is (nil, nil). The mcp column is ” when the skill has no
// config, and enabled travels as an int, like is_admin elsewhere in the store.
func scanSkill(row skillScanner) (*Skill, error) {
	var sk Skill
	var mcp string
	var enabled int
	if err := row.Scan(&sk.ID, &sk.Name, &sk.Description, &sk.Body, &mcp, &enabled,
		&sk.CreatedBy, &sk.CreatedAt, &sk.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	sk.Enabled = enabled == 1
	if mcp != "" {
		if err := json.Unmarshal([]byte(mcp), &sk.MCP); err != nil {
			return nil, fmt.Errorf("skills: decode mcp: %w", err)
		}
	}
	return &sk, nil
}

// ListSkills returns every skill on this installation, ordered by name.
func (s *Store) ListSkills() ([]Skill, error) {
	rows, err := s.db.Query(`SELECT id, name, description, body, mcp, enabled, created_by, created_at, updated_at
		FROM skills ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Skill{}
	for rows.Next() {
		sk, err := scanSkill(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *sk)
	}
	return out, rows.Err()
}

// SkillById looks up one skill. A missing skill is (nil, nil).
func (s *Store) SkillById(id string) (*Skill, error) {
	return scanSkill(s.db.QueryRow(`SELECT id, name, description, body, mcp, enabled, created_by, created_at, updated_at
		FROM skills WHERE id = ?`, id))
}

// CreateSkill mints and stores a new skill. The name is unique per
// installation; a collision returns ErrSkillNameTaken. createdBy is best
// effort: an empty actor still persists.
func (s *Store) CreateSkill(name, description, body string, mcp *MCPConfig, enabled bool, createdBy string) (*Skill, error) {
	skillId, err := id.New(id.Skill)
	if err != nil {
		return nil, err
	}
	mcpJSON, err := encodeMCP(mcp)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	sk := &Skill{
		ID:          skillId,
		Name:        name,
		Description: description,
		Body:        body,
		MCP:         mcp,
		Enabled:     enabled,
		CreatedBy:   createdBy,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err = s.db.Exec(`INSERT INTO skills (id, name, description, body, mcp, enabled, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sk.ID, sk.Name, sk.Description, sk.Body, mcpJSON, boolInt(sk.Enabled), sk.CreatedBy, sk.CreatedAt, sk.UpdatedAt)
	if uniqueConstraint(err) {
		return nil, ErrSkillNameTaken
	}
	if err != nil {
		return nil, err
	}
	return sk, nil
}

// UpdateSkill replaces the editable fields of a skill and refreshes its
// updated_at. A missing skill is (nil, nil); a name collision returns
// ErrSkillNameTaken.
func (s *Store) UpdateSkill(id, name, description, body string, mcp *MCPConfig, enabled bool) (*Skill, error) {
	mcpJSON, err := encodeMCP(mcp)
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec(`UPDATE skills SET name = ?, description = ?, body = ?, mcp = ?, enabled = ?, updated_at = ?
		WHERE id = ?`, name, description, body, mcpJSON, boolInt(enabled), time.Now().Unix(), id)
	if uniqueConstraint(err) {
		return nil, ErrSkillNameTaken
	}
	if err != nil {
		return nil, err
	}
	return s.SkillById(id)
}

// DeleteSkill removes a skill. Deleting a missing skill is not an error.
func (s *Store) DeleteSkill(id string) error {
	_, err := s.db.Exec(`DELETE FROM skills WHERE id = ?`, id)
	return err
}
