package funnel

import "time"

// Event is one append-only funnel row.
type Event struct {
	ID         string
	Kind       string
	OccurredAt time.Time
	OrgID      string
	AccountID  string
	ProjectID  string
	DeviceID   string
	Wall       string
}

// AccountFact is the live account row the snapshot needs.
type AccountFact struct {
	ID        string
	CreatedAt time.Time
	IsAdmin   bool
}

// OrgFact is the live org row plus activation derived from projects,
// enrolled machines, and finished-task streams.
type OrgFact struct {
	ID          string
	CreatedAt   time.Time
	Plan        string
	HasProject  bool
	HasDevice   bool
	FirstTaskAt time.Time
}

// Facts is the live side of a snapshot. Historical CTA / login / wall /
// idle counts come from events; activation uses these so Administration
// is not empty until the first new click after deploy.
type Facts struct {
	Accounts []AccountFact
	Orgs     []OrgFact
}
