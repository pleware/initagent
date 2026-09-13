package hub

import (
	"net/http"

	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/connectorops"
)

// handleSetupStatus reports the setup tool catalogue and its install state on
// one connector. The catalogue and the probing live in connectorops so the gateway
// answers the same surface for a connector connected there (10/16).
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	c := s.liveConnector(w, r, cred)
	if c == nil {
		return
	}
	writeJSON(w, connectorops.SetupStatus(r.Context(), c, c.hello.OS, c.hello.Arch))
}
