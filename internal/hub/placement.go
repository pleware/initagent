package hub

import (
	"errors"
	"net/http"
	"strings"

	"github.com/pleware/initagent/internal/authz"
)

// placement is where a gateway-bound request goes and which project it acts
// on. Both come from the project row, not from the process: `gateway_url` has
// been a column since projects existed, and reading it is what makes "one
// gateway per project" a value rather than a deployment (18).
type placement struct {
	projectID  string
	gatewayURL string
}

var (
	errProjectRequired = errBadRequest("project is required when this hub has more than one project (?project=prj-…)")
	errNoProject       = errBadRequest("this hub has no project yet")
)

// resolveProject picks the project a gateway-bound request acts on.
//
// The shape matches resolveProjectOrg deliberately: an explicit prj- wins, a
// hub with one readable project — self-host, and the free plan — can omit it,
// and two projects without one is a 400 rather than a guess. Guessing here
// would run a contractor's command against the wrong company's machine.
//
// A nil actor means the caller authenticated with an API token, which carries
// no scope today (09). Such a caller may name any project on the hub, which
// is the same reach it already has; it is not widened here, and scoping it is
// 09's job, not this function's.
func (s *Server) resolveProject(actor *authz.Actor, requested string) (*Project, error) {
	if requested = strings.TrimSpace(requested); requested != "" {
		project, err := s.store.ProjectById(requested)
		if err != nil {
			return nil, err
		}
		// A project in someone else's org answers the same 404 as a missing
		// one, so the router cannot be used to discover ids you cannot read.
		if project == nil || (actor != nil && !actor.Can(authz.ReadProject, project.OrgId)) {
			return nil, errProjectNotFound
		}
		return project, nil
	}

	projects, err := s.readableProjects(actor)
	if err != nil {
		return nil, err
	}
	switch len(projects) {
	case 0:
		return nil, errNoProject
	case 1:
		return &projects[0], nil
	default:
		return nil, errProjectRequired
	}
}

func (s *Server) readableProjects(actor *authz.Actor) ([]Project, error) {
	if actor == nil {
		return s.store.ListAllProjects()
	}
	var orgIds []string
	for orgId := range actor.Orgs {
		if actor.Can(authz.ReadProject, orgId) {
			orgIds = append(orgIds, orgId)
		}
	}
	return s.store.ListProjectsForOrgs(orgIds)
}

var errProjectNotFound = errNotFound("project not found")

type errNotFound string

func (e errNotFound) Error() string { return string(e) }

// gatewayFor resolves the placement for a request, answering the request
// itself on failure. It falls back to the hub's --gateway-url when the column
// is empty, which is what rows written before placement was read look like,
// and what a single-box self-host looks like when the flag is the only
// configuration there is.
func (s *Server) gatewayFor(w http.ResponseWriter, r *http.Request) (placement, bool) {
	actor, err := s.optionalActor(r)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return placement{}, false
	}
	project, err := s.resolveProject(actor, r.URL.Query().Get("project"))
	if err != nil {
		// A hub with no project row yet still has its flag: that is a
		// self-host box before anyone created a project, and the gateway
		// answers for the project it was started with. No header, so the
		// gateway falls back to that project exactly as it does today.
		if errors.Is(err, errNoProject) && s.opts.GatewayURL != "" {
			return placement{gatewayURL: s.opts.GatewayURL}, true
		}
		var notFound errNotFound
		var badRequest errBadRequest
		switch {
		case errors.Is(err, errNoProject):
			httpError(w, http.StatusServiceUnavailable, "gateway URL is required (--gateway-url); tasks and enroll run on the project gateway")
		case errors.As(err, &notFound):
			httpError(w, http.StatusNotFound, err.Error())
		case errors.As(err, &badRequest):
			httpError(w, http.StatusBadRequest, err.Error())
		default:
			httpError(w, http.StatusInternalServerError, err.Error())
		}
		return placement{}, false
	}
	target := strings.TrimSpace(project.GatewayURL)
	if target == "" {
		target = s.opts.GatewayURL
	}
	if target == "" {
		httpError(w, http.StatusServiceUnavailable, "project "+project.Id+" has no gateway; set --gateway-url or the project's gateway")
		return placement{}, false
	}
	return placement{projectID: project.Id, gatewayURL: target}, true
}
