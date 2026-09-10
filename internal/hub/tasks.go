package hub

import (
	"bytes"
	"cmp"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/brand"
)

// taskProxyTimeout gives the gateway time to run a one-shot exec (60s cap)
// plus slack for the hub↔gateway round trip. The cockpit never decides when a
// task is done; the gateway's resolver does (12).
const taskProxyTimeout = 75 * time.Second

type taskView struct {
	ID               string `json:"id"`
	ProjectID        string `json:"projectId,omitempty"`
	State            string `json:"state"`
	Command          string `json:"command,omitempty"`
	Launch           string `json:"launch,omitempty"`
	AssignedWorkerID string `json:"assignedWorkerId,omitempty"`
	ExitCode         int    `json:"exitCode"`
	Reason           string `json:"reason,omitempty"`
	Stdout           string `json:"stdout,omitempty"`
	Stderr           string `json:"stderr,omitempty"`
}

// handleCreateTask proxies a task submission to the project's gateway, which
// enqueues, claims, runs, and resolves it synchronously and returns the
// finished row. The streams are copied onto the hub so a later GET and the
// hosted log window can still see them (26).
func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	p, ok := s.gatewayFor(w, r, cred)
	if !ok {
		return
	}
	status, body, ct, err := s.fetchGateway(r, p, http.MethodPost, "/api/tasks", taskProxyTimeout)
	if err != nil {
		httpError(w, http.StatusBadGateway, "gateway unreachable: "+err.Error())
		return
	}
	if status == http.StatusOK {
		s.rememberTaskOutput(p, body)
		s.stampProjectActivity(p.projectID)
	}
	writeGatewayCopy(w, status, ct, body)
}

// handleGetTask proxies a single task's status from the project's gateway
// and overlays stored streams when the gateway no longer has them.
func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	p, ok := s.gatewayFor(w, r, cred)
	if !ok {
		return
	}
	status, body, ct, err := s.fetchGateway(r, p, http.MethodGet, "/api/tasks/"+r.PathValue("id"), taskProxyTimeout)
	if err != nil {
		httpError(w, http.StatusBadGateway, "gateway unreachable: "+err.Error())
		return
	}
	if status == http.StatusOK {
		body = s.overlayTaskOutput(body)
	}
	writeGatewayCopy(w, status, ct, body)
}

func (s *Server) rememberTaskOutput(p placement, body []byte) {
	if p.projectID == "" {
		return
	}
	var view taskView
	if err := json.Unmarshal(body, &view); err != nil || strings.TrimSpace(view.ID) == "" {
		return
	}
	project, err := s.store.ProjectById(p.projectID)
	if err != nil || project == nil || project.OrgId == "" {
		return
	}
	if err := s.store.SaveTaskOutput(TaskOutput{
		TaskID:    view.ID,
		OrgID:     project.OrgId,
		ProjectID: project.Id,
		Stdout:    view.Stdout,
		Stderr:    view.Stderr,
	}); err != nil {
		log.Printf("task output: %v", err)
	}
}

func (s *Server) overlayTaskOutput(body []byte) []byte {
	var view map[string]any
	if err := json.Unmarshal(body, &view); err != nil {
		return body
	}
	id, _ := view["id"].(string)
	if strings.TrimSpace(id) == "" {
		return body
	}
	out, err := s.store.TaskOutputByID(id)
	if err != nil || out == nil {
		return body
	}
	// Empty stored streams stay omitted, matching the gateway TaskView
	// omitempty contract. Do not write "" — that is a new observable.
	// Leave the gateway bytes alone when there is nothing to add, so a
	// GET without stored streams does not change key order or number types.
	changed := false
	if out.Stdout != "" {
		view["stdout"] = out.Stdout
		changed = true
	}
	if out.Stderr != "" {
		view["stderr"] = out.Stderr
		changed = true
	}
	if !changed {
		return body
	}
	merged, err := json.Marshal(view)
	if err != nil {
		return body
	}
	return merged
}

func (s *Server) fetchGateway(r *http.Request, p placement, method, path string, timeout time.Duration) (int, []byte, string, error) {
	u := strings.TrimRight(p.gatewayURL, "/") + path
	req, err := http.NewRequestWithContext(r.Context(), method, u, r.Body)
	if err != nil {
		return 0, nil, "", err
	}
	req.Header.Set("Content-Type", cmp.Or(r.Header.Get("Content-Type"), "application/json"))
	if p.projectID != "" {
		req.Header.Set(brand.ProjectHeader, p.projectID)
	}
	if s.opts.GatewaySecret != "" {
		req.Header.Set("Authorization", "Bearer "+s.opts.GatewaySecret)
	}
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return 0, nil, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, nil, resp.Header.Get("Content-Type"), err
	}
	return resp.StatusCode, body, resp.Header.Get("Content-Type"), nil
}

func writeGatewayCopy(w http.ResponseWriter, status int, contentType string, body []byte) {
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(status)
	_, _ = io.Copy(w, bytes.NewReader(body))
}
