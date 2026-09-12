package funnel

import (
	"testing"
	"time"
)

func TestSnapshotFromEmpty(t *testing.T) {
	t.Parallel()
	snap := SnapshotFrom(Facts{}, nil, time.Now())
	if snap.Acquisition.SignupRate != nil || snap.Activation.ProjectRate != nil {
		t.Fatal("empty denominators must omit rates")
	}
	if snap.Conversion.TimeToPayAvailable || snap.Conversion.MRRAvailable {
		t.Fatal("Stripe fields stay unavailable until checkout exists")
	}
	if snap.Activation.TimeToValueHours != nil {
		t.Fatal("no finished tasks means no time-to-value")
	}
}

func TestSnapshotFromRatesAndMedianTTV(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	facts := Facts{
		Accounts: []AccountFact{
			{ID: "account-old", CreatedAt: now.Add(-20 * day)},
			{ID: "account-new", CreatedAt: now.Add(-2 * day)},
			{ID: "account-ops", CreatedAt: now.Add(-40 * day), IsAdmin: true},
		},
		Orgs: []OrgFact{
			{
				ID: "org-1", CreatedAt: now.Add(-10 * day), Plan: "free",
				HasProject: true, HasDevice: true, FirstTaskAt: now.Add(-10*day + 6*time.Hour),
			},
			{
				ID: "org-2", CreatedAt: now.Add(-8 * day), Plan: "starter",
				HasProject: true, HasDevice: true, FirstTaskAt: now.Add(-8*day + 18*time.Hour),
			},
			{
				ID: "org-3", CreatedAt: now.Add(-3 * day), Plan: "free",
				HasProject: true,
			},
			{ID: "org-4", CreatedAt: now.Add(-1 * day), Plan: "team"},
		},
	}
	events := []Event{
		{Kind: KindCTAOpenApp},
		{Kind: KindCTAOpenApp},
		{Kind: KindCTASelfHost},
		{Kind: KindSignup},
		{Kind: KindInviteRedeem},
		{Kind: KindPlanLimitHit},
		{Kind: KindIdleWarned},
		{Kind: KindIdleDeleted},
		{Kind: KindLogin, AccountID: "account-old", OccurredAt: now.Add(-10 * day)},
		{Kind: KindLogin, AccountID: "account-ops", OccurredAt: now},
	}
	snap := SnapshotFrom(facts, events, now)

	if snap.Acquisition.CTAOpenApp != 2 || snap.Acquisition.CTASelfHost != 1 {
		t.Fatalf("cta = %+v", snap.Acquisition)
	}
	if snap.Acquisition.Signups != 1 || snap.Acquisition.InviteRedeems != 1 {
		t.Fatalf("accounts = %+v", snap.Acquisition)
	}
	if snap.Acquisition.SignupRate == nil || *snap.Acquisition.SignupRate != 1 {
		t.Fatalf("signup rate = %v, want 1", ptr(snap.Acquisition.SignupRate))
	}
	if snap.Activation.OrgsWithProject != 3 || snap.Activation.OrgsWithWorker != 2 || snap.Activation.OrgsWithTask != 2 {
		t.Fatalf("activation counts = %+v", snap.Activation)
	}
	if snap.Activation.TimeToValueHours == nil || *snap.Activation.TimeToValueHours != 12 {
		t.Fatalf("median TTV hours = %v, want 12", ptr(snap.Activation.TimeToValueHours))
	}
	if snap.Hygiene.PlanLimitHits != 1 || snap.Hygiene.IdleWarned != 1 || snap.Hygiene.IdleDeleted != 1 {
		t.Fatalf("hygiene = %+v", snap.Hygiene)
	}
	if snap.Retention.D7Eligible != 1 || snap.Retention.D7Returned != 1 {
		t.Fatalf("D7 = %+v (admin and too-new accounts must drop out)", snap.Retention)
	}
	if snap.Retention.D30Eligible != 0 {
		t.Fatalf("D30 eligible = %d, want 0 — oldest customer is 20 days", snap.Retention.D30Eligible)
	}
	if snap.Conversion.PaidOrgs != 2 {
		t.Fatalf("paid orgs = %d, want starter+team", snap.Conversion.PaidOrgs)
	}
	if snap.Conversion.ConvertedRate == nil || *snap.Conversion.ConvertedRate != 1 {
		t.Fatalf("converted rate = %v, want 1 (2 paid / 2 with a task)", ptr(snap.Conversion.ConvertedRate))
	}
}

func TestMedianHoursEvenCount(t *testing.T) {
	t.Parallel()
	got := medianHours([]time.Duration{2 * time.Hour, 10 * time.Hour, 4 * time.Hour, 8 * time.Hour})
	if got == nil || *got != 6 {
		t.Fatalf("even median = %v, want 6", ptr(got))
	}
}

func TestD30Return(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	acc := AccountFact{ID: "account-1", CreatedAt: now.Add(-40 * 24 * time.Hour)}
	events := []Event{{
		Kind: KindLogin, AccountID: "account-1",
		OccurredAt: now.Add(-5 * 24 * time.Hour),
	}}
	snap := SnapshotFrom(Facts{Accounts: []AccountFact{acc}}, events, now)
	if snap.Retention.D30Eligible != 1 || snap.Retention.D30Returned != 1 {
		t.Fatalf("D30 = %+v, want eligible+returned", snap.Retention)
	}
}

func TestLoginBeforeCutoffIsNotAReturn(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	created := now.Add(-20 * 24 * time.Hour)
	snap := SnapshotFrom(Facts{Accounts: []AccountFact{{ID: "account-1", CreatedAt: created}}}, []Event{{
		Kind: KindLogin, AccountID: "account-1", OccurredAt: created.Add(2 * 24 * time.Hour),
	}}, now)
	if snap.Retention.D7Eligible != 1 || snap.Retention.D7Returned != 0 {
		t.Fatalf("early login must not count as D7: %+v", snap.Retention)
	}
}

func ptr(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}
