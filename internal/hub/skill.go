package hub

import (
	"net/http"
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
