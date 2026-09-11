package desk

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestNewTurnIDIsAPureFunctionOfTheIntention(t *testing.T) {
	const utterance = UtteranceID("utt-7f3a")

	first := NewTurnID(DefaultConversation, utterance, 0)
	if got := NewTurnID(DefaultConversation, utterance, 0); got != first {
		t.Errorf("the same utterance and segment gave %q then %q", first, got)
	}
	// Property 4: no clock and no random source. A second call after time has
	// visibly passed must still land on the same key.
	time.Sleep(2 * time.Millisecond)
	if got := NewTurnID(DefaultConversation, utterance, 0); got != first {
		t.Errorf("the key moved with the clock: %q then %q", first, got)
	}
	if got := NewTurnID(DefaultConversation, utterance, 1); got == first {
		t.Error("a different segment must be a different turn")
	}
	if got := NewTurnID(DefaultConversation, "utt-7f3b", 0); got == first {
		t.Error("a different utterance must be a different turn")
	}
	// Two devices may number their own utterances from one. The same words
	// from another person are another turn, or hers would be dropped as a
	// re-delivery of his.
	if got := NewTurnID("phone", utterance, 0); got == first {
		t.Error("a different conversation must be a different turn")
	}
	if len(first) != turnIDChars {
		t.Errorf("id %q is %d chars, want %d", first, len(first), turnIDChars)
	}
	for _, c := range first {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Fatalf("id %q is not hex", first)
		}
	}
}

func TestNewTurnIDSeparatesItsFields(t *testing.T) {
	// Without length-prefixed fields ("ab", 1) and ("a", 11) would hash the
	// same bytes, and two utterances would share one claim.
	if NewTurnID(DefaultConversation, "ab", 1) == NewTurnID(DefaultConversation, "a", 11) {
		t.Error("fields run together in the digest")
	}
	if NewTurnID(DefaultConversation, "", 0) == NewTurnID(DefaultConversation, "0", 0) {
		t.Error("an empty utterance collides with a named one")
	}
	if NewTurnID("a", "b", 0) == NewTurnID("ab", "", 0) {
		t.Error("the conversation runs into the utterance in the digest")
	}
}

func TestTurnClaimsGrantsATurnOnce(t *testing.T) {
	claims := NewTurnClaims()
	turn := NewTurnID(DefaultConversation, "utt-1", 0)
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)

	outcome, err := claims.Claim(turn, "sprawdź to", now)
	if err != nil || outcome != ClaimGranted {
		t.Fatalf("first claim = %q, %v; want granted", outcome, err)
	}

	// The retry arriving while the first attempt is still running is refused.
	// A stalled attempt whose fate is unknown is when duplicating costs most.
	outcome, err = claims.Claim(turn, "sprawdź to", now.Add(time.Second))
	if err != nil || outcome != ClaimInFlight {
		t.Fatalf("duplicate while running = %q, %v; want in-flight", outcome, err)
	}

	if err := claims.Settle(turn, now.Add(2*time.Second)); err != nil {
		t.Fatalf("Settle: %v", err)
	}

	// Property 5: a settled turn replays rather than running again.
	outcome, err = claims.Claim(turn, "sprawdź to", now.Add(3*time.Second))
	if err != nil || outcome != ClaimReplay {
		t.Fatalf("claim after settling = %q, %v; want replay", outcome, err)
	}
}

func TestTurnClaimsRefusesTheSameTurnWithDifferentText(t *testing.T) {
	claims := NewTurnClaims()
	turn := NewTurnID(DefaultConversation, "utt-2", 0)
	now := time.Now()

	if _, err := claims.Claim(turn, "sprawdź to", now); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	outcome, err := claims.Claim(turn, "usuń to", now)
	if !errors.Is(err, ErrTurnText) {
		t.Fatalf("want ErrTurnText, got %q, %v", outcome, err)
	}
	if !errors.Is(err, ErrRequest) {
		t.Error("a reused key with a different payload is a client defect")
	}
	if Retryable(err) {
		t.Error("retrying will send the same wrong pair again")
	}
	if outcome != "" {
		t.Errorf("outcome = %q, want none alongside an error", outcome)
	}
}

func TestTurnClaimsRefusesAnEmptyTurn(t *testing.T) {
	claims := NewTurnClaims()

	if _, err := claims.Claim("", "sprawdź to", time.Now()); !errors.Is(err, ErrRequest) {
		t.Fatalf("want ErrRequest, got %v", err)
	}
	if claims.Len() != 0 {
		t.Error("a refused claim should leave nothing behind")
	}
}

func TestTurnClaimsSettleNeedsAClaim(t *testing.T) {
	claims := NewTurnClaims()
	turn := NewTurnID(DefaultConversation, "utt-3", 0)
	now := time.Now()

	if err := claims.Settle(turn, now); !errors.Is(err, ErrTurnUnclaimed) {
		t.Fatalf("want ErrTurnUnclaimed, got %v", err)
	}

	if _, err := claims.Claim(turn, "zrób to", now); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if err := claims.Settle(turn, now); err != nil {
		t.Fatalf("Settle: %v", err)
	}
	// Settling twice is what a retried reply loop does; it is not a defect.
	if err := claims.Settle(turn, now.Add(time.Second)); err != nil {
		t.Fatalf("second Settle: %v", err)
	}
}

func TestTurnClaimsReleaseFreesATurnThatNeverRan(t *testing.T) {
	claims := NewTurnClaims()
	turn := NewTurnID(DefaultConversation, "utt-4", 0)
	now := time.Now()

	if _, err := claims.Claim(turn, "zrób to", now); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	claims.Release(turn)
	if claims.Len() != 0 {
		t.Fatal("Release should drop an unsettled claim")
	}

	outcome, err := claims.Claim(turn, "zrób to", now)
	if err != nil || outcome != ClaimGranted {
		t.Fatalf("claim after release = %q, %v; want granted", outcome, err)
	}

	// A settled turn is an answer the desk can replay. Releasing it would
	// invite a second run of something that already spoke.
	if err := claims.Settle(turn, now); err != nil {
		t.Fatalf("Settle: %v", err)
	}
	claims.Release(turn)
	outcome, err = claims.Claim(turn, "zrób to", now)
	if err != nil || outcome != ClaimReplay {
		t.Fatalf("claim after releasing a settled turn = %q, %v; want replay", outcome, err)
	}

	claims.Release(NewTurnID(DefaultConversation, "utt-never", 0))
}

func TestTurnClaimsPruneKeepsWhatIsStillRunning(t *testing.T) {
	claims := NewTurnClaims()
	now := time.Date(2026, 9, 11, 13, 0, 0, 0, time.UTC)

	stale := NewTurnID(DefaultConversation, "utt-stale", 0)
	fresh := NewTurnID(DefaultConversation, "utt-fresh", 0)
	running := NewTurnID(DefaultConversation, "utt-running", 0)

	for turn, at := range map[TurnID]time.Time{
		stale:   now.Add(-2 * TurnClaimRetention),
		fresh:   now.Add(-time.Minute),
		running: now.Add(-2 * TurnClaimRetention),
	} {
		if _, err := claims.Claim(turn, "zrób to", at); err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if turn == running {
			continue
		}
		if err := claims.Settle(turn, at); err != nil {
			t.Fatalf("Settle: %v", err)
		}
	}

	if dropped := claims.Prune(now); dropped != 1 {
		t.Errorf("dropped %d claims, want 1", dropped)
	}
	if claims.Len() != 2 {
		t.Errorf("held %d claims, want 2", claims.Len())
	}
	// Forgetting an in-flight claim would turn a duplicate into a second run,
	// whatever its age.
	outcome, err := claims.Claim(running, "zrób to", now)
	if err != nil || outcome != ClaimInFlight {
		t.Errorf("the running turn = %q, %v; want in-flight", outcome, err)
	}
	outcome, err = claims.Claim(stale, "zrób to", now)
	if err != nil || outcome != ClaimGranted {
		t.Errorf("the forgotten turn = %q, %v; want granted", outcome, err)
	}
}

func TestTurnClaimsGrantATurnToExactlyOneAttempt(t *testing.T) {
	claims := NewTurnClaims()
	turn := NewTurnID(DefaultConversation, "utt-race", 0)
	now := time.Now()

	const attempts = 32
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		granted int
	)
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outcome, err := claims.Claim(turn, "sprawdź to", now)
			if err != nil {
				t.Errorf("Claim: %v", err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if outcome == ClaimGranted {
				granted++
			}
		}()
	}
	wg.Wait()

	// A read followed by a write would let two attempts through here, which is
	// the failure a claim exists to prevent.
	if granted != 1 {
		t.Errorf("%d attempts were granted the turn, want 1", granted)
	}
}
