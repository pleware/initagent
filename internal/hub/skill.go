package hub

import (
	"errors"
	"net/http"
	"strings"

	"github.com/pleware/initagent/internal/authz"
)

// MCPConfig is the optional MCP-server companion of a skill. It is stored as
// a JSON string in skills.mcp; a skill without one keeps the column empty
// (""), so there is no second table for the config.
type MCPConfig struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
}

// Skill is an installable capability of this hub. The store is
// installation-scoped: skills have no org_id and no version pin.
type Skill struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Body        string     `json:"body"`
	MCP         *MCPConfig `json:"mcp,omitempty"`
	Enabled     bool       `json:"enabled"`
	CreatedBy   string     `json:"createdBy"`
	CreatedAt   int64      `json:"createdAt"`
	UpdatedAt   int64      `json:"updatedAt"`
}

// --- public views ---

// skillSummary is the public listing view of a skill: enough to render a
// catalog entry, nothing more.
type skillSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// skillDownload is the public install view of one skill: everything the
// installer needs, none of the administration (enabled, createdBy,
// timestamps). The MCP config travels without Env, which may hold secrets.
type skillDownload struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Body        string     `json:"body"`
	MCP         *MCPConfig `json:"mcp,omitempty"`
}

// publicMCP copies an MCP config without its Env.
func publicMCP(mcp *MCPConfig) *MCPConfig {
	if mcp == nil {
		return nil
	}
	return &MCPConfig{Command: mcp.Command, Args: mcp.Args, URL: mcp.URL}
}

// --- public handlers ---

// handleListSkillsPublic serves the enabled skills of this installation as a
// catalog, ordered by name (the store's order). No middleware: reading what
// an installation offers is pre-auth, like the installer it feeds.
func (s *Server) handleListSkillsPublic(w http.ResponseWriter, r *http.Request) {
	skills, err := s.store.ListSkills()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]skillSummary, 0, len(skills))
	for _, sk := range skills {
		if !sk.Enabled {
			continue
		}
		out = append(out, skillSummary{ID: sk.ID, Name: sk.Name, Description: sk.Description})
	}
	writeJSON(w, out)
}

// handleGetSkillPublic serves one enabled skill for installation. A disabled
// skill and a missing one answer the same 404, so whether a skill exists
// leaks nothing while it is off.
func (s *Server) handleGetSkillPublic(w http.ResponseWriter, r *http.Request) {
	sk, err := s.store.SkillById(r.PathValue("id"))
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sk == nil || !sk.Enabled {
		httpError(w, http.StatusNotFound, "no such skill")
		return
	}
	writeJSON(w, skillDownload{
		ID:          sk.ID,
		Name:        sk.Name,
		Description: sk.Description,
		Body:        sk.Body,
		MCP:         publicMCP(sk.MCP),
	})
}

// --- admin handlers ---

// skillInput is the editable shape of a skill, shared by create and update.
type skillInput struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Body        string     `json:"body"`
	MCP         *MCPConfig `json:"mcp,omitempty"`
	Enabled     *bool      `json:"enabled,omitempty"`
}

// refusal reports why a submission must be rejected, or "" when it passes.
func (in skillInput) refusal() string {
	if strings.TrimSpace(in.Name) == "" {
		return "name is required"
	}
	if strings.TrimSpace(in.Body) == "" {
		return "body is required"
	}
	if in.MCP != nil && strings.TrimSpace(in.MCP.Command) == "" && strings.TrimSpace(in.MCP.URL) == "" {
		return "mcp requires a command or a url"
	}
	return ""
}

// handleAdminListSkills serves every skill on the installation, disabled
// included: the operator's own view of the store they manage.
func (s *Server) handleAdminListSkills(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminSkill, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	skills, err := s.store.ListSkills()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, skills)
}

// handleCreateSkill mints a new skill. The name is trimmed and both it and
// the body are required; an MCP config has to name a command or a url. A
// nil enabled means on.
func (s *Server) handleCreateSkill(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminSkill, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	var in skillInput
	if err := readJSON(r, &in); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	if msg := in.refusal(); msg != "" {
		httpError(w, http.StatusBadRequest, msg)
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	sk, err := s.store.CreateSkill(strings.TrimSpace(in.Name), in.Description, in.Body, in.MCP, enabled, cred.Requester.Account)
	if errors.Is(err, ErrSkillNameTaken) {
		httpError(w, http.StatusConflict, ErrSkillNameTaken.Error())
		return
	}
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, sk)
}

// handleUpdateSkill replaces the editable fields of one skill. A missing id
// is a 404; validation and the name-collision answer match create.
func (s *Server) handleUpdateSkill(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminSkill, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	existing, err := s.store.SkillById(r.PathValue("id"))
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		httpError(w, http.StatusNotFound, "no such skill")
		return
	}
	var in skillInput
	if err := readJSON(r, &in); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	if msg := in.refusal(); msg != "" {
		httpError(w, http.StatusBadRequest, msg)
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	updated, err := s.store.UpdateSkill(existing.ID, strings.TrimSpace(in.Name), in.Description, in.Body, in.MCP, enabled)
	if errors.Is(err, ErrSkillNameTaken) {
		httpError(w, http.StatusConflict, ErrSkillNameTaken.Error())
		return
	}
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, updated)
}

// handleDeleteSkill removes one skill. Deleting a missing skill is not an
// error, matching the store.
func (s *Server) handleDeleteSkill(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminSkill, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	if err := s.store.DeleteSkill(r.PathValue("id")); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
