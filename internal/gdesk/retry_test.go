package gdesk

import (
	"testing"
	"time"
)

func TestBackoffGrowsAndStops(t *testing.T) {
	t.Parallel()
	cases := []struct {
		failed int
		want   time.Duration
	}{
		{0, 200 * time.Millisecond},
		{1, 200 * time.Millisecond},
		{2, 400 * time.Millisecond},
		{3, 800 * time.Millisecond},
		{4, 1600 * time.Millisecond},
		{5, maxBackoff},
		{50, maxBackoff},
	}
	for _, tc := range cases {
		if got := Backoff(tc.failed); got != tc.want {
			t.Errorf("Backoff(%d) = %v, want %v", tc.failed, got, tc.want)
		}
	}
}

// TestBackoffStaysInsideATurn is the reason this is not mailer.Backoff: the
// first wait there is fifteen seconds, which is a lost conversation here.
func TestBackoffStaysInsideATurn(t *testing.T) {
	t.Parallel()
	if got := Backoff(MaxTurnAttempts); got > time.Second {
		t.Errorf("last backoff = %v, want under a second", got)
	}
}

func TestRetryChatStopsAtTheClaim(t *testing.T) {
	t.Parallel()
	cases := []struct {
		failed int
		want   bool
	}{{0, true}, {1, true}, {2, true}, {3, false}, {9, false}}
	for _, tc := range cases {
		if got := RetryChat(tc.failed); got != tc.want {
			t.Errorf("RetryChat(%d) = %v, want %v", tc.failed, got, tc.want)
		}
	}
}

func TestRetrySTTIsBoundedByTheUtterance(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 11, 9, 0, 0, 0, time.UTC)

	wait, ok := RetrySTT(0, now, now.Add(5*time.Second))
	if !ok || wait != 200*time.Millisecond {
		t.Errorf("early retry = (%v, %v), want (200ms, true)", wait, ok)
	}

	// The wait itself would land after the person spoke again, so the
	// transcript would be garbage even if it succeeded.
	if wait, ok := RetrySTT(0, now, now.Add(100*time.Millisecond)); ok {
		t.Errorf("retry past the deadline = (%v, true), want false", wait)
	}
	if _, ok := RetrySTT(0, now, now); ok {
		t.Error("retry at the deadline is allowed")
	}

	// No fixed count: attempt twenty is fine if the deadline is far away.
	if _, ok := RetrySTT(20, now, now.Add(time.Minute)); !ok {
		t.Error("a late attempt inside the deadline was refused")
	}
}

func TestRetryTTSStopsOnceSheHasStartedSpeaking(t *testing.T) {
	t.Parallel()
	if !RetryTTS(0, 0) {
		t.Error("a first failure before any audio was not retried")
	}
	if RetryTTS(0, 1) {
		t.Error("retried after a byte reached the speaker")
	}
	if RetryTTS(MaxTurnAttempts, 0) {
		t.Error("retried past the attempt ceiling")
	}
}
