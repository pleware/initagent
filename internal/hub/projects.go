package hub

import (
	"cmp"
	"errors"
	"net/http"
	"strings"

	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/offering"
	"github.com/pleware/initagent/internal/projecttemplate"
	"github.com/pleware/initagent/internal/repo"
)

const (
	maxProjectName   = 80
	maxProjectPath   = 4096
	maxProjectRemote = 2048
)

type projectInput struct {
	Name       string `json:"name"`
	OrgId      string `json:"orgId"`
	DeviceId   string `json:"deviceId"`
	Path       string `json:"path"`
	TemplateId string `json:"templateId"`
	RepoRemote string `json:"repoRemote"`
}

func cleanProjectFields(input projectInput) (projectInput, string) {
	input.Name = strings.TrimSpace(input.Name)
	input.OrgId = strings.TrimSpace(input.OrgId)
	input.DeviceId = strings.TrimSpace(input.DeviceId)
	input.Path = strings.TrimSpace(input.Path)
	input.TemplateId = strings.TrimSpace(input.TemplateId)
	input.RepoRemote = strings.TrimSpace(input.RepoRemote)
	if len(input.Name) > maxProjectName {
		return input, "project name is too long"
	}
	if len(input.Path) > maxProjectPath || strings.ContainsRune(input.Path, '\x00') {
		return input, "project path is invalid"
	}
	if len(input.RepoRemote) > maxProjectRemote {
		return input, "repository address is too long"
	}
	if input.TemplateId != "" {
		tmpl, ok := projecttemplate.Lookup(input.TemplateId)
		if !ok {
			return input, "unknown project template"
		}
		if !tmpl.Live {
			return input, "that template is not available yet"
		}
	}
	return input, ""
}

func repoFields(remote string) (string, string, string) {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return "", "", ""
	}
	host, err := repo.HostFromRemote(remote)
	if err != nil {
		return "", "", err.Error()
	}
	return remote, string(host), ""
}

func (s *Server) validateProjectDevice(w http.ResponseWriter, deviceId string) bool {
	if deviceId == "" {
		return true
	}
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
	if len(actor.Orgs) == 0 {
		return "", authz.ErrForbidden
	}
	return "", errOrgRequired
}

var errOrgRequired = errBadRequest("orgId is required when you belong to more than one organization")

type errBadRequest string

func (e errBadRequest) Error() string { return string(e) }

func (s *Server) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, projecttemplate.Catalogue())
}

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
	if s.opts.Offering == offering.Hosted && actor.Platform {
		forbid(w, authz.ErrForbidden)
		return
	}
	var input projectInput
	if err := readJSON(r, &input); err != nil {
		httpError(w, http.StatusBadRequest, "invalid project")
		return
	}
	var message string
	if input, message = cleanProjectFields(input); message != "" {
		httpError(w, http.StatusBadRequest, message)
		return
	}
	if input.Name == "" {
		httpError(w, http.StatusBadRequest, "name is required")
		return
	}
	orgId, err := resolveProjectOrg(actor, input.OrgId)
	if err != nil {
		if errors.Is(err, authz.ErrForbidden) {
			forbid(w, err)
			return
		}
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !actor.Can(authz.CreateProject, orgId) {
		forbid(w, authz.ErrForbidden)
		return
	}
	if s.refuseAnotherProject(w, orgId) {
		return
	}
	if !s.validateProjectDevice(w, input.DeviceId) {
		return
	}
	if s.refuseAnotherMachine(w, orgId, "", input.DeviceId) {
		return
	}
	remote, host, message := repoFields(input.RepoRemote)
	if message != "" {
		httpError(w, http.StatusBadRequest, message)
		return
	}
	project, err := s.store.CreateProject(orgId, input.Name, input.DeviceId, input.Path, s.opts.GatewayURL, input.TemplateId, remote, host)
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
	if input, message = cleanProjectFields(input); message != "" {
		httpError(w, http.StatusBadRequest, message)
		return
	}
	name := cmp.Or(input.Name, existing.Name)
	deviceId := existing.DeviceId
	if input.DeviceId != "" {
		deviceId = input.DeviceId
	}
	path := existing.Path
	if input.Path != "" {
		path = input.Path
	}
	templateId := existing.TemplateId
	if input.TemplateId != "" {
		templateId = input.TemplateId
	}
	if !s.validateProjectDevice(w, deviceId) {
		return
	}
	if s.refuseAnotherMachine(w, existing.OrgId, existing.Id, deviceId) {
		return
	}
	remote, host := existing.RepoRemote, existing.RepoHost
	if input.RepoRemote != "" {
		remote, host, message = repoFields(input.RepoRemote)
		if message != "" {
			httpError(w, http.StatusBadRequest, message)
			return
		}
	}
	project, err := s.store.UpdateProject(existing.Id, name, deviceId, path, templateId, remote, host)
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

func (s *Server) handleAttachProjectDevice(w http.ResponseWriter, r *http.Request, actor authz.Actor) {
	existing, ok := s.projectForActor(w, r.PathValue("id"), actor, authz.CreateProject)
	if !ok {
		return
	}
	var input struct {
		DeviceId string `json:"deviceId"`
	}
	if err := readJSON(r, &input); err != nil {
		httpError(w, http.StatusBadRequest, "invalid device")
		return
	}
	deviceId := strings.TrimSpace(input.DeviceId)
	if deviceId == "" {
		httpError(w, http.StatusBadRequest, "deviceId is required")
		return
	}
	if !s.validateProjectDevice(w, deviceId) {
		return
	}
	if s.refuseAnotherMachine(w, existing.OrgId, existing.Id, deviceId) {
		return
	}
	added, err := s.store.AttachProjectDevice(existing.Id, deviceId)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	project, err := s.store.ProjectById(existing.Id)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if project == nil {
		httpError(w, http.StatusNotFound, "project not found")
		return
	}
	if added {
		w.WriteHeader(http.StatusCreated)
	}
	writeJSON(w, project)
}

func (s *Server) handleDetachProjectDevice(w http.ResponseWriter, r *http.Request, actor authz.Actor) {
	existing, ok := s.projectForActor(w, r.PathValue("id"), actor, authz.CreateProject)
	if !ok {
		return
	}
	deviceId := strings.TrimSpace(r.PathValue("deviceId"))
	if deviceId == "" {
		httpError(w, http.StatusBadRequest, "deviceId is required")
		return
	}
	enrolled, err := s.store.ProjectHasDevice(existing.Id, deviceId)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !enrolled {
		httpError(w, http.StatusNotFound, "device is not on this project")
		return
	}
	if err := s.store.DetachProjectDevice(existing.Id, deviceId); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	project, err := s.store.ProjectById(existing.Id)
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
	if project.DeviceId == "" {
		httpError(w, http.StatusServiceUnavailable, "project has no device")
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
