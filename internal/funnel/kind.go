// Package funnel is the hosted close-out counting surface (backlog KPIs).
//
// It names events and turns stored rows plus live facts into a snapshot.
// It does not decide product behaviour — draft 39 telemetry never does
// either. A missing insert must not fail the caller.
package funnel

// Qualified kind names (draft 05). The row id carries event- (id.Event →
// initagent.hub.event); these strings are what we count.
const (
	KindCTAOpenApp        = "initagent.site.cta.open_app"
	KindCTASelfHost       = "initagent.site.cta.self_host"
	KindSignup            = "initagent.hub.account.signup"
	KindInviteRedeem      = "initagent.hub.account.invite_redeem"
	KindLogin             = "initagent.hub.account.login"
	KindProjectCreated    = "initagent.hub.project.created"
	KindConnectorEnrolled = "initagent.fleet.connector.enrolled"
	KindTaskFinished      = "initagent.project.task.finished"
	KindPlanLimitHit      = "initagent.hub.plan_limit.hit"
	KindIdleWarned        = "initagent.hub.project.idle_warned"
	KindIdleDeleted       = "initagent.hub.project.idle_deleted"
)

var known = map[string]struct{}{
	KindCTAOpenApp:        {},
	KindCTASelfHost:       {},
	KindSignup:            {},
	KindInviteRedeem:      {},
	KindLogin:             {},
	KindProjectCreated:    {},
	KindConnectorEnrolled: {},
	KindTaskFinished:      {},
	KindPlanLimitHit:      {},
	KindIdleWarned:        {},
	KindIdleDeleted:       {},
}

// Known reports whether kind is one we will store.
func Known(kind string) bool {
	_, ok := known[kind]
	return ok
}
