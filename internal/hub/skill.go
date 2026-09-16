package hub

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
