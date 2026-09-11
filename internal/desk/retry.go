package desk

import "time"

const (
	// MaxTurnAttempts bounds retries inside one claimed turn. A turn is a
	// person waiting, so the ceiling is low: past this the honest answer is a
	// failure she can say out loud.
	MaxTurnAttempts = 3

	// firstBackoff and maxBackoff are conversation scale, not outbox scale.
	// This is deliberately not mailer.Backoff — mail retries over hours
	// because nobody is watching the inbox, and a desk that waited fifteen
	// seconds mid-sentence has already lost the turn.
	firstBackoff = 200 * time.Millisecond
	maxBackoff   = 2 * time.Second
)

// Backoff is how long to wait before attempt failedAttempts+1.
func Backoff(failedAttempts int) time.Duration {
	wait := firstBackoff
	for range max(failedAttempts, 1) - 1 {
		wait = min(wait*2, maxBackoff)
	}
	return wait
}

// RetryChat reports whether a failed chat call may be tried again.
//
// The retry happens under the same turn claim, never as a second turn: two
// turns for one utterance is Ania answering twice, and no amount of provider
// trouble makes that the right repair.
func RetryChat(failedAttempts int) bool {
	return failedAttempts < MaxTurnAttempts
}

// RetrySTT reports whether a failed transcription may be tried again, and how
// long to wait first.
//
// The bound is the utterance's own deadline rather than a count: a transcript
// that lands after the person has spoken again is garbage, and expiry is a
// fact about the world instead of a policy we chose.
func RetrySTT(failedAttempts int, now, deadline time.Time) (time.Duration, bool) {
	wait := Backoff(failedAttempts)
	if !now.Add(wait).Before(deadline) {
		return 0, false
	}
	return wait, true
}

// RetryTTS reports whether a failed speech call may be tried again.
//
// Only before the first byte has reached the speaker. Past that, re-speaking
// from the start says half a sentence twice; the repair is an abandoned line
// plus a fact on the seam, which the scene can show without lying about what
// was said.
func RetryTTS(failedAttempts, bytesSpoken int) bool {
	if bytesSpoken > 0 {
		return false
	}
	return failedAttempts < MaxTurnAttempts
}
