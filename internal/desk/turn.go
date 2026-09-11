package desk

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"
)

// UtteranceID is what the person said once — one press-and-release, or one
// textarea submit. The glass mints it once per utterance and reuses it on
// every retry; it is never regenerated per attempt, which is the only thing
// that makes a retry recognisable at all.
type UtteranceID string

// TurnID is one segment of one utterance being answered by one person.
type TurnID string

// turnDomain separates this hash from every other id derived in the product,
// so the same utterance cannot collide with a key minted elsewhere.
const turnDomain = "desk.turn:v1"

// turnIDChars is how much of the digest an id carries. 128 bits is far past
// what a desk needs and short enough to read in a fact.
const turnIDChars = 32

// TurnClaimRetention is how long a settled claim is remembered.
//
// It must outlive the longest path that can re-deliver the same utterance,
// which here is the glass reconnecting and replaying its outbox — seconds
// normally, minutes after a closed lid. Past this the same utterance is
// treated as new work, which is stated rather than discovered.
const TurnClaimRetention = 15 * time.Minute

// ErrTurnText marks the same turn arriving with different text. It wraps
// ErrRequest because it is a client defect: replaying the first answer to a
// second question would be worse than refusing.
var ErrTurnText = fmt.Errorf("%w: turn text changed", ErrRequest)

// ErrTurnUnclaimed marks settling a turn nobody claimed.
var ErrTurnUnclaimed = fmt.Errorf("%w: turn was never claimed", ErrRequest)

// NewTurnID derives the key for one segment.
//
// A direct utterance with two segments becomes two turns rather than one turn
// with two names, because a half-delivered utterance must be retryable
// without double-booking the segment that already landed. The key is a pure
// function of the intention: same utterance and same segment is always the
// same turn, a different segment is always a different turn, and there is no
// clock and no random source in it.
func NewTurnID(utterance UtteranceID, index int) TurnID {
	h := sha256.New()
	h.Write([]byte(turnDomain))
	// Length-prefixed fields, so ("ab", 1) and ("a", 11) cannot hash alike.
	writeField(h, string(utterance))
	writeField(h, strconv.Itoa(index))
	return TurnID(hex.EncodeToString(h.Sum(nil))[:turnIDChars])
}

func writeField(w io.Writer, s string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(s)))
	w.Write(length[:])
	w.Write([]byte(s))
}

// ClaimOutcome is what a claim on a turn answers.
type ClaimOutcome string

const (
	// ClaimGranted means this attempt owns the turn: run it.
	ClaimGranted ClaimOutcome = "granted"

	// ClaimInFlight means an earlier attempt is still running. The duplicate
	// is refused rather than let through: a stalled attempt whose fate is
	// unknown is exactly when duplicating costs most.
	ClaimInFlight ClaimOutcome = "in-flight"

	// ClaimReplay means the turn already ran. Answer by replaying its facts,
	// never by running it a second time.
	ClaimReplay ClaimOutcome = "replay"
)

// TurnClaims is the gate in front of a turn.
//
// It answers whether an attempt may run, and nothing else — the facts to
// replay live in the seam's own log, because a claim store that also kept
// transcripts would be two things with one lock.
//
// It lives in memory on the connector. A connector restart forgets claims,
// which is honest: the floor survives a shell restart because the connector
// holds it, and a claim outlives only the utterance it belongs to.
type TurnClaims struct {
	mu     sync.Mutex
	claims map[TurnID]turnClaim
}

type turnClaim struct {
	text    [sha256.Size]byte
	claimed time.Time
	settled bool
}

// NewTurnClaims returns an empty gate.
func NewTurnClaims() *TurnClaims {
	return &TurnClaims{claims: map[TurnID]turnClaim{}}
}

// Claim records the intention to run a turn and says whether this attempt owns
// it.
//
// The record is written before the turn acts, so a crash between the model
// call and its answer leaves evidence that something must be resolved later
// rather than a silently repeated effect. The insert and the check are one
// operation under one lock; a read followed by a write would be a race, and
// duplicates arrive in bursts exactly when a dependency is degraded.
func (c *TurnClaims) Claim(turn TurnID, text string, now time.Time) (ClaimOutcome, error) {
	if turn == "" {
		return "", fmt.Errorf("%w: a claim needs a turn", ErrRequest)
	}
	sum := sha256.Sum256([]byte(text))

	c.mu.Lock()
	defer c.mu.Unlock()

	existing, held := c.claims[turn]
	if !held {
		c.claims[turn] = turnClaim{text: sum, claimed: now}
		return ClaimGranted, nil
	}
	if existing.text != sum {
		return "", fmt.Errorf("%w: turn %s", ErrTurnText, turn)
	}
	if existing.settled {
		return ClaimReplay, nil
	}
	return ClaimInFlight, nil
}

// Settle marks a claimed turn finished, whichever way it finished.
//
// Success and failure settle the same way: both are answers the desk can
// replay. The third outcome — a turn whose fate is unknown — is deliberately
// not settled here, so it stays in flight until something resolves it rather
// than becoming a replayable answer nobody produced.
func (c *TurnClaims) Settle(turn TurnID, now time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	existing, held := c.claims[turn]
	if !held {
		return fmt.Errorf("%w: turn %s", ErrTurnUnclaimed, turn)
	}
	if existing.settled {
		return nil
	}
	existing.settled = true
	existing.claimed = now
	c.claims[turn] = existing
	return nil
}

// Release drops a claim so the turn may be attempted again.
//
// This is for a refusal that never reached the provider — a bad request, an
// unbound role — where nothing happened and nothing needs replaying. A turn
// that failed mid-reply is settled, not released: words may already be on the
// scene.
func (c *TurnClaims) Release(turn TurnID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if existing, held := c.claims[turn]; held && !existing.settled {
		delete(c.claims, turn)
	}
}

// Prune forgets settled claims older than TurnClaimRetention and reports how
// many it dropped. In-flight claims are kept whatever their age: forgetting
// one would turn a duplicate into a second run.
func (c *TurnClaims) Prune(now time.Time) int {
	cutoff := now.Add(-TurnClaimRetention)

	c.mu.Lock()
	defer c.mu.Unlock()

	dropped := 0
	for turn, claim := range c.claims {
		if claim.settled && claim.claimed.Before(cutoff) {
			delete(c.claims, turn)
			dropped++
		}
	}
	return dropped
}

// Len reports how many claims are held, for a status line and for tests.
func (c *TurnClaims) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.claims)
}
