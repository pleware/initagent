package funnel

import (
	"slices"
	"time"
)

// Snapshot is what GET /api/admin/kpis returns.
type Snapshot struct {
	Acquisition Acquisition `json:"acquisition"`
	Activation  Activation  `json:"activation"`
	Hygiene     Hygiene     `json:"hygiene"`
	Retention   Retention   `json:"retention"`
	Conversion  Conversion  `json:"conversion"`
	Cost        Cost        `json:"cost"`
}

// Acquisition is CTA landings and new accounts.
type Acquisition struct {
	CTAOpenApp    int      `json:"ctaOpenApp"`
	CTASelfHost   int      `json:"ctaSelfHost"`
	Signups       int      `json:"signups"`
	InviteRedeems int      `json:"inviteRedeems"`
	SignupRate    *float64 `json:"signupRate,omitempty"`
}

// Activation is first project / worker / finished task.
type Activation struct {
	OrgsWithProject  int      `json:"orgsWithProject"`
	OrgsWithWorker   int      `json:"orgsWithWorker"`
	OrgsWithTask     int      `json:"orgsWithTask"`
	ProjectRate      *float64 `json:"projectRate,omitempty"`
	WorkerRate       *float64 `json:"workerRate,omitempty"`
	TaskRate         *float64 `json:"taskRate,omitempty"`
	TimeToValueHours *float64 `json:"timeToValueHours,omitempty"`
}

// Hygiene is walls and the idle pipeline.
type Hygiene struct {
	PlanLimitHits int `json:"planLimitHits"`
	IdleWarned    int `json:"idleWarned"`
	IdleDeleted   int `json:"idleDeleted"`
}

// Retention is D7 / D30 login return among customer accounts old enough
// to be eligible.
type Retention struct {
	D7Eligible  int      `json:"d7Eligible"`
	D7Returned  int      `json:"d7Returned"`
	D7Rate      *float64 `json:"d7Rate,omitempty"`
	D30Eligible int      `json:"d30Eligible"`
	D30Returned int      `json:"d30Returned"`
	D30Rate     *float64 `json:"d30Rate,omitempty"`
}

// Conversion stays structured zeros until Stripe (close-out step 10).
// Paid orgs still count starter/team rows that an operator set by hand.
type Conversion struct {
	PaidOrgs           int      `json:"paidOrgs"`
	ConvertedRate      *float64 `json:"convertedRate,omitempty"`
	TimeToPayAvailable bool     `json:"timeToPayAvailable"`
	MRRAvailable       bool     `json:"mrrAvailable"`
}

// Cost is the thin GATE-1 view the hub can see without a metrics stack.
type Cost struct {
	OnlineWorkers int `json:"onlineWorkers"`
}

// SnapshotFrom folds live facts and stored events. Rates are omitted
// when the denominator is zero. now is the clock for D7/D30 eligibility.
func SnapshotFrom(facts Facts, events []Event, now time.Time) Snapshot {
	var snap Snapshot
	var ttv []time.Duration
	for _, ev := range events {
		switch ev.Kind {
		case KindCTAOpenApp:
			snap.Acquisition.CTAOpenApp++
		case KindCTASelfHost:
			snap.Acquisition.CTASelfHost++
		case KindSignup:
			snap.Acquisition.Signups++
		case KindInviteRedeem:
			snap.Acquisition.InviteRedeems++
		case KindPlanLimitHit:
			snap.Hygiene.PlanLimitHits++
		case KindIdleWarned:
			snap.Hygiene.IdleWarned++
		case KindIdleDeleted:
			snap.Hygiene.IdleDeleted++
		}
	}
	newAccounts := snap.Acquisition.Signups + snap.Acquisition.InviteRedeems
	snap.Acquisition.SignupRate = rate(newAccounts, snap.Acquisition.CTAOpenApp)

	for _, org := range facts.Orgs {
		if org.HasProject {
			snap.Activation.OrgsWithProject++
		}
		if org.HasConnector {
			snap.Activation.OrgsWithWorker++
		}
		if !org.FirstTaskAt.IsZero() {
			snap.Activation.OrgsWithTask++
			if !org.CreatedAt.IsZero() && !org.FirstTaskAt.Before(org.CreatedAt) {
				ttv = append(ttv, org.FirstTaskAt.Sub(org.CreatedAt))
			}
		}
		if paidPlan(org.Plan) {
			snap.Conversion.PaidOrgs++
		}
	}
	nOrgs := len(facts.Orgs)
	snap.Activation.ProjectRate = rate(snap.Activation.OrgsWithProject, nOrgs)
	snap.Activation.WorkerRate = rate(snap.Activation.OrgsWithWorker, snap.Activation.OrgsWithProject)
	snap.Activation.TaskRate = rate(snap.Activation.OrgsWithTask, snap.Activation.OrgsWithWorker)
	snap.Activation.TimeToValueHours = medianHours(ttv)
	snap.Conversion.ConvertedRate = rate(snap.Conversion.PaidOrgs, snap.Activation.OrgsWithTask)

	for _, acc := range facts.Accounts {
		if acc.IsAdmin {
			continue
		}
		countReturn(acc, events, now, 7, &snap.Retention.D7Eligible, &snap.Retention.D7Returned)
		countReturn(acc, events, now, 30, &snap.Retention.D30Eligible, &snap.Retention.D30Returned)
	}
	snap.Retention.D7Rate = rate(snap.Retention.D7Returned, snap.Retention.D7Eligible)
	snap.Retention.D30Rate = rate(snap.Retention.D30Returned, snap.Retention.D30Eligible)
	return snap
}

func countReturn(acc AccountFact, events []Event, now time.Time, days int, eligible, returned *int) {
	cutoff := acc.CreatedAt.Add(time.Duration(days) * 24 * time.Hour)
	if cutoff.After(now) {
		return
	}
	*eligible++
	for _, ev := range events {
		if ev.Kind != KindLogin || ev.AccountID != acc.ID {
			continue
		}
		if !ev.OccurredAt.Before(cutoff) {
			*returned++
			return
		}
	}
}

func paidPlan(plan string) bool {
	switch plan {
	case "starter", "team":
		return true
	default:
		return false
	}
}

func rate(num, den int) *float64 {
	if den == 0 {
		return nil
	}
	v := float64(num) / float64(den)
	return &v
}

func medianHours(durs []time.Duration) *float64 {
	if len(durs) == 0 {
		return nil
	}
	sorted := slices.Clone(durs)
	slices.Sort(sorted)
	n := len(sorted)
	var h float64
	if n%2 == 1 {
		h = sorted[n/2].Hours()
	} else {
		h = (sorted[n/2-1].Hours() + sorted[n/2].Hours()) / 2
	}
	return &h
}
