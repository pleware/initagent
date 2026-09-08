package hub

import (
	"net/http"

	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/deviceops"
)

// handleSetupStatus reports the setup tool catalogue and its install state on
// one device. The catalogue and the probing live in deviceops so the gateway
// answers the same surface for a device connected there (10/16).
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	c := s.liveDevice(w, r, cred)
	if c == nil {
		return
	}
	writeJSON(w, deviceops.SetupStatus(r.Context(), c, c.hello.OS, c.hello.Arch))
}
