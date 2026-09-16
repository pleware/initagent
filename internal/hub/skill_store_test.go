package hub

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/pleware/initagent/internal/id"
)

// testMCP is a fully-populated MCP config used where the test needs a set one.
func testMCP() *MCPConfig {
	return &MCPConfig{
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
		Env:     map[string]string{"HOME": "/tmp"},
		URL:     "http://127.0.0.1:9876",
	}
}

func TestCreateSkillRoundTrip(t *testing.T) {
	tests := []struct {
		name        string
		description string
		body        string
		mcp         *MCPConfig
		enabled     bool
		createdBy   string
	}{
		{
			name:        "disabled without mcp",
			description: "turn a light off",
			body:        "step one: walk away",
			mcp:         nil,
			enabled:     false,
			createdBy:   "acc-tester",
		},
		{
			name:        "enabled with mcp",
			description: "serve files",
			body:        "ask the mcp server",
			mcp:         testMCP(),
			enabled:     true,
			createdBy:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testStore(t)
			created, err := s.CreateSkill("hello", tt.description, tt.body, tt.mcp, tt.enabled, tt.createdBy)
			if err != nil {
				t.Fatal(err)
			}

			got, err := s.SkillById(created.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got == nil {
				t.Fatal("SkillById returned nil for a created skill")
			}
			if !id.Is(id.Skill, got.ID) {
				t.Errorf("minted id %q is not a skill identifier", got.ID)
			}
			if got.Name != "hello" {
				t.Errorf("Name = %q, want hello", got.Name)
			}
			if got.Description != tt.description {
				t.Errorf("Description = %q, want %q", got.Description, tt.description)
			}
			if got.Body != tt.body {
				t.Errorf("Body = %q, want %q", got.Body, tt.body)
			}
			if !reflect.DeepEqual(got.MCP, tt.mcp) {
				t.Errorf("MCP = %+v, want %+v", got.MCP, tt.mcp)
			}
			if got.Enabled != tt.enabled {
				t.Errorf("Enabled = %v, want %v", got.Enabled, tt.enabled)
			}
			if got.CreatedBy != tt.createdBy {
				t.Errorf("CreatedBy = %q, want %q", got.CreatedBy, tt.createdBy)
			}
			if got.CreatedAt == 0 || got.UpdatedAt == 0 {
				t.Errorf("timestamps not set: %+v", got)
			}
			if got.CreatedAt != created.CreatedAt || got.UpdatedAt != created.UpdatedAt {
				t.Errorf("read-back timestamps %d/%d differ from created %d/%d",
					got.CreatedAt, got.UpdatedAt, created.CreatedAt, created.UpdatedAt)
			}
		})
	}
}

func TestCreateSkillDuplicateName(t *testing.T) {
	s := testStore(t)
	if _, err := s.CreateSkill("dup", "", "", nil, true, ""); err != nil {
		t.Fatal(err)
	}
	_, err := s.CreateSkill("dup", "second try", "", nil, true, "")
	if !errors.Is(err, ErrSkillNameTaken) {
		t.Fatalf("err = %v, want ErrSkillNameTaken", err)
	}
	list, err := s.ListSkills()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("ListSkills has %d rows after refused duplicate, want 1", len(list))
	}
}

func TestSkillByIdMissing(t *testing.T) {
	s := testStore(t)
	got, err := s.SkillById("skill-00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("SkillById = %+v, want nil", got)
	}
}

func TestUpdateSkill(t *testing.T) {
	s := testStore(t)
	created, err := s.CreateSkill("before", "first body", "echo one", nil, false, "acc-tester")
	if err != nil {
		t.Fatal(err)
	}
	// Backdate updated_at so the bump below is observable even within the
	// same wall-clock second.
	if _, err := s.db.Exec(`UPDATE skills SET updated_at = 1 WHERE id = ?`, created.ID); err != nil {
		t.Fatal(err)
	}

	got, err := s.UpdateSkill(created.ID, "after", "second body", "echo two", testMCP(), true)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("UpdateSkill returned nil for an existing skill")
	}
	if got.Name != "after" {
		t.Errorf("Name = %q, want after", got.Name)
	}
	if got.Body != "echo two" {
		t.Errorf("Body = %q, want echo two", got.Body)
	}
	if got.Enabled != true {
		t.Errorf("Enabled = %v, want true", got.Enabled)
	}
	if !reflect.DeepEqual(got.MCP, testMCP()) {
		t.Errorf("MCP = %+v, want %+v", got.MCP, testMCP())
	}
	if got.UpdatedAt <= 1 {
		t.Errorf("UpdatedAt = %d, want bumped past the backdated 1", got.UpdatedAt)
	}
	// Update touches only the editable fields.
	if got.CreatedAt != created.CreatedAt {
		t.Errorf("CreatedAt = %d, want %d", got.CreatedAt, created.CreatedAt)
	}
	if got.CreatedBy != created.CreatedBy {
		t.Errorf("CreatedBy = %q, want %q", got.CreatedBy, created.CreatedBy)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q", got.ID, created.ID)
	}
}

func TestUpdateSkillMissing(t *testing.T) {
	s := testStore(t)
	got, err := s.UpdateSkill("skill-00000000-0000-0000-0000-000000000000", "nope", "", "", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("UpdateSkill = %+v, want nil", got)
	}
}

func TestUpdateSkillDuplicateName(t *testing.T) {
	s := testStore(t)
	if _, err := s.CreateSkill("taken", "", "", nil, true, ""); err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateSkill("free", "", "", nil, true, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.UpdateSkill(other.ID, "taken", "", "", nil, true)
	if !errors.Is(err, ErrSkillNameTaken) {
		t.Fatalf("err = %v, want ErrSkillNameTaken", err)
	}
}

func TestDeleteSkill(t *testing.T) {
	s := testStore(t)
	created, err := s.CreateSkill("doomed", "", "", nil, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSkill(created.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.SkillById(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("SkillById after delete = %+v, want nil", got)
	}
	// Deleting a missing skill is not an error.
	if err := s.DeleteSkill(created.ID); err != nil {
		t.Fatalf("second DeleteSkill = %v, want nil", err)
	}
}

func TestSkillMCPColumn(t *testing.T) {
	tests := []struct {
		name    string
		mcp     *MCPConfig
		wantCol string
	}{
		{"nil stores empty column", nil, ""},
		{"set mcp stores json", testMCP(), ""}, // JSON asserted separately below.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testStore(t)
			created, err := s.CreateSkill("mcp-skill", "", "", tt.mcp, true, "")
			if err != nil {
				t.Fatal(err)
			}
			var col string
			if err := s.db.QueryRow(`SELECT mcp FROM skills WHERE id = ?`, created.ID).Scan(&col); err != nil {
				t.Fatal(err)
			}
			if tt.mcp == nil {
				if col != "" {
					t.Errorf("mcp column = %q, want empty", col)
				}
				got, err := s.SkillById(created.ID)
				if err != nil {
					t.Fatal(err)
				}
				if got.MCP != nil {
					t.Errorf("MCP = %+v, want nil", got.MCP)
				}
				return
			}
			want, err := json.Marshal(tt.mcp)
			if err != nil {
				t.Fatal(err)
			}
			if col != string(want) {
				t.Errorf("mcp column = %q, want %q", col, string(want))
			}
		})
	}
}

func TestSkillEnabledColumn(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		wantCol int
	}{
		{"enabled stores one", true, 1},
		{"disabled stores zero", false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testStore(t)
			created, err := s.CreateSkill("toggle", "", "", nil, tt.enabled, "")
			if err != nil {
				t.Fatal(err)
			}
			var col int
			if err := s.db.QueryRow(`SELECT enabled FROM skills WHERE id = ?`, created.ID).Scan(&col); err != nil {
				t.Fatal(err)
			}
			if col != tt.wantCol {
				t.Errorf("enabled column = %d, want %d", col, tt.wantCol)
			}
			got, err := s.SkillById(created.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Enabled != tt.enabled {
				t.Errorf("Enabled = %v, want %v", got.Enabled, tt.enabled)
			}
		})
	}
}

func TestListSkillsOrderedByName(t *testing.T) {
	s := testStore(t)
	for _, name := range []string{"zeta", "alpha", "mike"} {
		if _, err := s.CreateSkill(name, "", "", nil, true, ""); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.ListSkills()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"alpha", "mike", "zeta"}
	if len(list) != len(want) {
		t.Fatalf("ListSkills has %d rows, want %d", len(list), len(want))
	}
	for i, sk := range list {
		if sk.Name != want[i] {
			t.Errorf("row %d name = %q, want %q", i, sk.Name, want[i])
		}
	}
}

func TestListSkillsEmpty(t *testing.T) {
	s := testStore(t)
	list, err := s.ListSkills()
	if err != nil {
		t.Fatal(err)
	}
	if list == nil || len(list) != 0 {
		t.Fatalf("ListSkills on empty store = %#v, want empty non-nil slice", list)
	}
}
