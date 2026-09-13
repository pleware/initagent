package funnel

import "testing"

func TestKnown(t *testing.T) {
	t.Parallel()
	if !Known(KindSignup) {
		t.Fatal("signup must be known")
	}
	if Known("initagent.hub.account.unknown") || Known("") {
		t.Fatal("unknown kinds must be rejected")
	}
}

func TestKnownKindsStayQualified(t *testing.T) {
	t.Parallel()
	want := []string{
		KindCTAOpenApp,
		KindCTASelfHost,
		KindSignup,
		KindInviteRedeem,
		KindLogin,
		KindProjectCreated,
		KindConnectorEnrolled,
		KindTaskFinished,
		KindPlanLimitHit,
		KindIdleWarned,
		KindIdleDeleted,
	}
	if len(known) != len(want) {
		t.Fatalf("known = %d kinds, want %d", len(known), len(want))
	}
	for _, k := range want {
		if !Known(k) {
			t.Errorf("missing %q", k)
		}
	}
}
