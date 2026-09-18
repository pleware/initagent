package hub

import (
	"net/http"
	"strconv"
	"strings"
)

// handleBoxChanges is the box sync-down endpoint (58): a box polls its own
// config manifest, presenting its box token as a Bearer credential. The
// route is deliberately plain — not behind requireCredential/requireSession —
// because the credential is a machine secret: BoxTokenAuth is the only gate,
// an api token or a session is refused, and the token must name the box in
// the path.
//
// `since` is the config_version the box already applied. A poll at or past
// the current version answers 304 with no body; an earlier version serves
// the fresh manifest.
func (s *Server) handleBoxChanges(w http.ResponseWriter, r *http.Request) {
	secret, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		httpError(w, http.StatusUnauthorized, "missing box token")
		return
	}
	boxID, ok, err := s.store.BoxTokenAuth(secret)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		httpError(w, http.StatusUnauthorized, "invalid or revoked box token")
		return
	}
	if boxID != r.PathValue("id") {
		httpError(w, http.StatusForbidden, "box token does not match this box")
		return
	}
	box, err := s.store.GetBox(boxID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if box == nil {
		httpError(w, http.StatusNotFound, "no such box")
		return
	}
	since := 0
	if raw := r.URL.Query().Get("since"); raw != "" {
		since, err = strconv.Atoi(raw)
		if err != nil || since < 0 {
			httpError(w, http.StatusBadRequest, "since must be a non-negative integer")
			return
		}
	}
	if int64(since) >= box.ConfigVersion {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	manifest, err := s.store.BuildBoxManifest(boxID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, manifest)
}
