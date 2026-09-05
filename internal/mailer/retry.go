package mailer

import "time"

// MaxAttempts is how many failed sends a row may take before it is dead.
const MaxAttempts = 8

// StaleAfter is how long a row may sit in `sending` before the drain
// assumes the process died and reclaims it.
const StaleAfter = 5 * time.Minute

// RetainFor is how long an outbox row may live before the hub deletes it,
// regardless of status (draft 26).
const RetainFor = 30 * 24 * time.Hour

// Backoff is how long to wait after n failed sends (n >= 1) before the
// next attempt. Caps at one hour.
func Backoff(failedAttempts int) time.Duration {
	n := max(failedAttempts, 1)
	d := 15 * time.Second
	for range n - 1 {
		d = min(d*2, time.Hour)
	}
	return d
}

// Dead reports whether another failure should stop retrying.
func Dead(failedAttempts int) bool {
	return failedAttempts >= MaxAttempts
}
