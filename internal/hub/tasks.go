package hub

import (
	"net/http"
	"time"
)

// taskProxyTimeout gives the gateway time to run a one-shot exec (60s cap)
// plus slack for the hub↔gateway round trip. The cockpit never decides when a
// task is done; the gateway's resolver does (12).
const taskProxyTimeout = 75 * time.Second

// handleCreateTask proxies a task submission to the project's gateway, which
// enqueues, claims, runs, and resolves it synchronously and returns the
// finished row.
func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	p, ok := s.gatewayFor(w, r)
	if !ok {
		return
	}
	s.proxyGateway(w, r, p, http.MethodPost, "/api/tasks", taskProxyTimeout)
}

// handleGetTask proxies a single task's status from the project's gateway.
func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	p, ok := s.gatewayFor(w, r)
	if !ok {
		return
	}
	s.proxyGateway(w, r, p, http.MethodGet, "/api/tasks/"+r.PathValue("id"), taskProxyTimeout)
}
