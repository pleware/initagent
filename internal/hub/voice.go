package hub

import (
	"net/http"

	"github.com/pleware/initagent/internal/voices"
)

// handleListVoices serves the TTS voice catalog of this installation, in the
// voices package's display order (pl_PL first). No middleware: like the skill
// catalog, what an installation offers is pre-auth.
func (s *Server) handleListVoices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, voices.List())
}
