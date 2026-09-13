package hub

import (
	"cmp"
	"errors"
	"net/http"
	"strings"

	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/funnel"
	"github.com/pleware/initagent/internal/offering"
	"github.com/pleware/initagent/internal/projecttemplate"
	"github.com/pleware/initagent/internal/protocol"
	"github.com/pleware/initagent/internal/repo"
)

const (
	maxProjectName   = 80
	maxProjectPath   = 4096
	maxProjectRemote = 2048
)

type projectInput struct {
	Name        string `json:"name"`
	OrgId       string `json:"orgId"`
	ConnectorId string `json:"connectorId"`
	Path        string `json:"path"`
	TemplateId  string `json:"templateId"`
	RepoRemote  string `json:"repoRemote"`
}

func cleanProjectFields(input projectInput) (projectInput, string) {
	input.Name = strings.TrimSpace(input.Name)
	input.OrgId = strings.TrimSpace(input.OrgId)
	input.ConnectorId = strings.TrimSpace(input.ConnectorId)
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

func (s *Server) validateProjectConnector(w http.ResponseWriter, connectorId string) bool {
	if connectorId == "" {
		return true
	}
	connector, err := s.store.ConnectorById(connectorId)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if connector == nil {
		httpError(w, http.StatusBadRequest, "connector does not exist")
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
func resolveProjectOrg(cred authz.Credential, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		return requested, nil
	}
	// A token already names its tenant, so it never has to guess — and it
	// must not fall through to its author's memberships, which may be wider
	// than the boundary the token was given.
	if cred.Grant != nil {
		return cred.Grant.Org, nil
	}
	if only := cred.Actor.SoleOrg(); only != "" {
		return only, nil
	}
	if len(cred.Actor.Orgs) == 0 {
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

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	requested := strings.TrimSpace(r.URL.Query().Get("org"))
	if requested != "" {
		// A project-scoped token is refused here rather than narrowed: it
		// asked for a whole organization, which is wider than it holds.
		// Omitting ?org= gets it the project it does hold.
		if !cred.Can(authz.ReadProject, requested, "") {
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
	// Same resolution the gateway placement uses, so the catalogue a caller
	// can list and the projects it can route to cannot drift apart.
	projects, err := s.readableProjects(cred)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if projects == nil {
		projects = []Project{}
	}
	writeJSON(w, projects)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if s.opts.Offering == offering.Hosted && cred.Actor.Platform {
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
	orgId, err := resolveProjectOrg(cred, input.OrgId)
	if err != nil {
		if errors.Is(err, authz.ErrForbidden) {
			forbid(w, err)
			return
		}
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !cred.Can(authz.CreateProject, orgId, "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	if s.refuseAnotherProject(w, orgId, cred.Actor.Account) {
		return
	}
	if !s.validateProjectConnector(w, input.ConnectorId) {
		return
	}
	if s.refuseAnotherMachine(w, orgId, "", cred.Actor.Account, input.ConnectorId) {
		return
	}
	remote, host, message := repoFields(input.RepoRemote)
	if message != "" {
		httpError(w, http.StatusBadRequest, message)
		return
	}
	project, err := s.store.CreateProject(orgId, input.Name, input.ConnectorId, input.Path, s.opts.GatewayURL, input.TemplateId, remote, host)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.recordEvent(funnel.Event{
		Kind:      funnel.KindProjectCreated,
		OrgID:     orgId,
		AccountID: cred.Actor.Account,
		ProjectID: project.Id,
	})
	if input.ConnectorId != "" {
		s.recordEvent(funnel.Event{
			Kind:        funnel.KindConnectorEnrolled,
			OrgID:       orgId,
			AccountID:   cred.Actor.Account,
			ProjectID:   project.Id,
			ConnectorID: input.ConnectorId,
		})
	}
	project = s.bindSelfhostWorker(r.Context(), project)
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, project)
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	existing, ok := s.projectFor(w, r.PathValue("id"), cred, authz.AdminProject)
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
	connectorId := existing.ConnectorId
	if input.ConnectorId != "" {
		connectorId = input.ConnectorId
	}
	path := existing.Path
	if input.Path != "" {
		path = input.Path
	}
	templateId := existing.TemplateId
	if input.TemplateId != "" {
		templateId = input.TemplateId
	}
	if !s.validateProjectConnector(w, connectorId) {
		return
	}
	if s.refuseAnotherMachine(w, existing.OrgId, existing.Id, cred.Actor.Account, connectorId) {
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
	project, err := s.store.UpdateProject(existing.Id, name, connectorId, path, templateId, remote, host)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if project == nil {
		httpError(w, http.StatusNotFound, "project not found")
		return
	}
	s.stampProjectActivity(project.Id)
	writeJSON(w, project)
}

func (s *Server) handleAttachProjectConnector(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	existing, ok := s.projectFor(w, r.PathValue("id"), cred, authz.AdminProject)
	if !ok {
		return
	}
	var input struct {
		ConnectorId string `json:"connectorId"`
	}
	if err := readJSON(r, &input); err != nil {
		httpError(w, http.StatusBadRequest, "invalid connector")
		return
	}
	connectorId := strings.TrimSpace(input.ConnectorId)
	if connectorId == "" {
		httpError(w, http.StatusBadRequest, "connectorId is required")
		return
	}
	if !s.validateProjectConnector(w, connectorId) {
		return
	}
	if s.refuseAnotherMachine(w, existing.OrgId, existing.Id, cred.Actor.Account, connectorId) {
		return
	}
	added, err := s.store.AttachProjectConnector(existing.Id, connectorId)
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
		s.recordEvent(funnel.Event{
			Kind:        funnel.KindConnectorEnrolled,
			OrgID:       existing.OrgId,
			AccountID:   cred.Actor.Account,
			ProjectID:   existing.Id,
			ConnectorID: connectorId,
		})
		w.WriteHeader(http.StatusCreated)
	}
	writeJSON(w, project)
}

func (s *Server) handleDetachProjectConnector(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	existing, ok := s.projectFor(w, r.PathValue("id"), cred, authz.AdminProject)
	if !ok {
		return
	}
	connectorId := strings.TrimSpace(r.PathValue("connectorId"))
	if connectorId == "" {
		httpError(w, http.StatusBadRequest, "connectorId is required")
		return
	}
	enrolled, err := s.store.ProjectHasConnector(existing.Id, connectorId)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !enrolled {
		httpError(w, http.StatusNotFound, "connector is not on this project")
		return
	}
	if err := s.store.DetachProjectConnector(existing.Id, connectorId); err != nil {
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

func (s *Server) handleProjectActivity(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	project, ok := s.projectFor(w, r.PathValue("id"), cred, authz.ReadProject)
	if !ok {
		return
	}
	s.stampProjectActivity(project.Id)
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	project, ok := s.projectFor(w, r.PathValue("id"), cred, authz.DeleteProject)
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
func (s *Server) handleProjectExec(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	project, ok := s.projectFor(w, r.PathValue("id"), cred, authz.ExecConnector)
	if !ok {
		return
	}
	if project.ConnectorId == "" {
		httpError(w, http.StatusServiceUnavailable, "project has no connector")
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
	if c := s.registry.get(project.ConnectorId); c != nil {
		result, err := s.execOnConnector(c, input.Command, project.Path, timeoutSec)
		if err != nil {
			httpError(w, http.StatusBadGateway, err.Error())
			return
		}
		_ = s.store.TouchProject(project.Id)
		s.stampProjectActivity(project.Id)
		writeJSON(w, result)
		return
	}
	// The worker lives on the gateway (self-host enroll, 10/16): rewrite the
	// command into the connector-exec shape and hop, exactly as the session and
	// connector-exec routes do.
	target := s.projectGateway(project)
	if target == "" {
		httpError(w, http.StatusServiceUnavailable, "project connector is offline")
		return
	}
	s.stampProjectActivity(project.Id)
	s.proxyGatewayJSON(w, r, placement{projectID: project.Id, gatewayURL: target},
		http.MethodPost, "/api/connectors/"+project.ConnectorId+"/exec", connectorProxyTimeout,
		protocol.Exec{Command: input.Command, Cwd: project.Path, TimeoutSec: timeoutSec})
}

// projectFor loads a project and checks the capability against it.
//
// The single choke point for every project route, which is why the boundary
// axis arrives here for free: passing the project's own id as the target is
// what stops a project-scoped token operating on its neighbour.
//
// A missing project and a project outside the credential's reach both answer
// 404, so the catalogue cannot be used to discover ids you cannot read.
func (s *Server) projectFor(w http.ResponseWriter, id string, cred authz.Credential, c authz.Capability) (*Project, bool) {
	project, err := s.store.ProjectById(id)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	if project == nil {
		httpError(w, http.StatusNotFound, "project not found")
		return nil, false
	}
	if !cred.Can(c, project.OrgId, project.Id) {
		hideOrRefuse(w, cred, c, "project not found", bound{org: project.OrgId, project: project.Id})
		return nil, false
	}
	return project, true
}
