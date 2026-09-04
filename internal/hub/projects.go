package hub

import (
	"net/http"
	"strings"

	"github.com/pleware/initagent/internal/authz"
)

const (
	maxProjectName = 80
	maxProjectPath = 4096
)

type projectInput struct {
	Name     string `json:"name"`
	OrgId    string `json:"orgId"`
	DeviceId string `json:"deviceId"`
	Path     string `json:"path"`
}

func cleanProjectInput(input projectInput) (projectInput, string) {
	input.Name = strings.TrimSpace(input.Name)
	input.OrgId = strings.TrimSpace(input.OrgId)
	input.DeviceId = strings.TrimSpace(input.DeviceId)
	input.Path = strings.TrimSpace(input.Path)
	if input.Name == "" || input.DeviceId == "" || input.Path == "" {
		return input, "name, deviceId, and path are required"
	}
	if len(input.Name) > maxProjectName {
		return input, "project name is too long"
	}
	if len(input.Path) > maxProjectPath || strings.ContainsRune(input.Path, '\x00') {
		return input, "project path is invalid"
	}
	return input, ""
}

func (s *Server) validateProjectDevice(w http.ResponseWriter, deviceId string) bool {
	device, err := s.store.DeviceById(deviceId)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if device == nil {
		httpError(w, http.StatusBadRequest, "device does not exist")
		return false
	}
	return true
}

// resolveProjectOrg picks the organization a project request acts in.
//
// An explicit orgId wins. A hub with one membership — self-host, and the
// first hosted claim — can omit it, which is how the inherited create form
// keeps working. Two or more memberships without an orgId is a 400, not a
// guess: inventing a "current org" on the server is how a contractor's
// project lands in the wrong company.
func resolveProjectOrg(actor authz.Actor, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		return requested, nil
	}
	if only := actor.SoleOrg(); only != "" {
		return only, nil
	}
	return "", errOrgRequired
}

var errOrgRequired = errBadRequest("orgId is required when you belong to more than one organization")

type errBadRequest string

func (e errBadRequest) Error() string { return string(e) }

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request, actor authz.Actor) {
	requested := strings.TrimSpace(r.URL.Query().Get("org"))
	if requested != "" {
		if !actor.Can(authz.ReadProject, requested) {
			forbid(w, authz.ErrForbidden)
			return
		}
		projects, err := s.store.ListProjectsByOrg(requested)
		if err != nil {
			httpError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, projects)
		return
	}
	var orgIds []string
	for orgId := range actor.Orgs {
		if actor.Can(authz.ReadProject, orgId) {
			orgIds = append(orgIds, orgId)
		}
	}
	projects, err := s.store.ListProjectsForOrgs(orgIds)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, projects)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request, actor authz.Actor) {
	var input projectInput
	if err := readJSON(r, &input); err != nil {
		httpError(w, http.StatusBadRequest, "invalid project")
		return
	}
	var message string
	if input, message = cleanProjectInput(input); message != "" {
		httpError(w, http.StatusBadRequest, message)
		return
	}
	orgId, err := resolveProjectOrg(actor, input.OrgId)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !actor.Can(authz.CreateProject, orgId) {
		forbid(w, authz.ErrForbidden)
		return
	}
	if !s.validateProjectDevice(w, input.DeviceId) {
		return
	}
	project, err := s.store.CreateProject(orgId, input.Name, input.DeviceId, input.Path, s.opts.GatewayURL)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, project)
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request, actor authz.Actor) {
	existing, ok := s.projectForActor(w, r.PathValue("id"), actor, authz.CreateProject)
	if !ok {
		return
	}
	var input projectInput
	if err := readJSON(r, &input); err != nil {
		httpError(w, http.StatusBadRequest, "invalid project")
		return
	}
	var message string
	if input, message = cleanProjectInput(input); message != "" {
		httpError(w, http.StatusBadRequest, message)
		return
	}
	if !s.validateProjectDevice(w, input.DeviceId) {
		return
	}
	project, err := s.store.UpdateProject(existing.Id, input.Name, input.DeviceId, input.Path)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if project == nil {
		httpError(w, http.StatusNotFound, "project not found")
		return
	}
	writeJSON(w, project)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request, actor authz.Actor) {
	project, ok := s.projectForActor(w, r.PathValue("id"), actor, authz.DeleteProject)
	if !ok {
		return
	}
	if err := s.store.DeleteProject(project.Id); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// handleProjectExec is the narrow host boundary used by browser-hosted fx.
// The browser supplies only a command; the hub owns the selected node and cwd.
func (s *Server) handleProjectExec(w http.ResponseWriter, r *http.Request, actor authz.Actor) {
	project, ok := s.projectForActor(w, r.PathValue("id"), actor, authz.ReadProject)
	if !ok {
		return
	}
	c := s.registry.get(project.DeviceId)
	if c == nil {
		httpError(w, http.StatusServiceUnavailable, "project device is offline")
		return
	}
	var input struct {
		Command   string `json:"command"`
		TimeoutMs int    `json:"timeoutMs"`
	}
	if err := readJSON(r, &input); err != nil || strings.TrimSpace(input.Command) == "" {
		httpError(w, http.StatusBadRequest, "command required")
		return
	}
	if len(input.Command) > 64*1024 {
		httpError(w, http.StatusBadRequest, "command is too long")
		return
	}
	timeoutSec := input.TimeoutMs / 1000
	if timeoutSec < 1 {
		timeoutSec = 30
	}
	if timeoutSec > 600 {
		timeoutSec = 600
	}
	result, err := s.execOnDevice(c, input.Command, project.Path, timeoutSec)
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	_ = s.store.TouchProject(project.Id)
	writeJSON(w, result)
}

// projectForActor loads a project and checks the capability inside its org.
// A missing project and a project in someone else's org both answer 404, so
// the catalogue cannot be used to discover ids you cannot read.
func (s *Server) projectForActor(w http.ResponseWriter, id string, actor authz.Actor, c authz.Capability) (*Project, bool) {
	project, err := s.store.ProjectById(id)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	if project == nil || !actor.Can(c, project.OrgId) {
		httpError(w, http.StatusNotFound, "project not found")
		return nil, false
	}
	return project, true
}
