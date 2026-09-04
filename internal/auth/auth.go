// Package auth owns the hub's credential decisions: how an email address is
// normalised, what a password has to satisfy, how it is hashed, what
// proves the operator when a fresh hub is claimed for the first time, and
// whether a claimed hosted hub may mint a customer account (drafts 08, 09, 26).
//
// Every function here is a decision over explicit inputs. Reading a token
// file, inserting an account row and issuing a session cookie are edges and
// belong to the hub, not to this package.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"

	"github.com/pleware/initagent/internal/offering"
)

// Password floors per offering. Eight characters was a floor for a personal
// tool on a home network, which is what upstream was. The same credential now
// guards a publicly reachable control plane, so the hosted offering asks for
// more (26).
const (
	MinPasswordSelfhost = 8
	MinPasswordHosted   = 12
)

// Errors the HTTP edge maps onto status codes. They are sentinels rather than
// strings so a handler cannot drift from the decision it is reporting.
var (
	ErrEmailInvalid   = errors.New("email is not a valid address")
	ErrPasswordWeak   = errors.New("password does not meet the minimum")
	ErrAlreadyClaimed = errors.New("hub already has an owner")
	ErrClaimToken     = errors.New("bootstrap token does not match")
	ErrNotHosted      = errors.New("registration is not available on this hub")
	ErrNotClaimed     = errors.New("hub has no owner yet")
	ErrEmailTaken     = errors.New("email already registered")
)

// MinPassword is the shortest password an offering accepts. An unknown
// offering gets the stricter floor: a new offering token should not silently
// arrive with the weaker rule.
func MinPassword(kind offering.Kind) int {
	if kind == offering.Selfhost {
		return MinPasswordSelfhost
	}
	return MinPasswordHosted
}

// NormalizeEmail trims and lowercases an address and rejects anything that is
// not a bare mailbox. RFC parsing comes from net/mail; the extra rules are
// ours, because "Ops <ops@example.com>" and "root@localhost" both parse and
// neither is something we want as an account's identity.
func NormalizeEmail(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%w: empty", ErrEmailInvalid)
	}
	if utf8.RuneCountInString(trimmed) > 254 {
		return "", fmt.Errorf("%w: longer than 254 characters", ErrEmailInvalid)
	}
	addr, err := mail.ParseAddress(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrEmailInvalid, err)
	}
	// ParseAddress accepts a display name; the address it returns is then a
	// different string from the input, which is how we detect that shape.
	if addr.Address != trimmed {
		return "", fmt.Errorf("%w: give the address only, without a name", ErrEmailInvalid)
	}
	at := strings.LastIndex(addr.Address, "@")
	domain := addr.Address[at+1:]
	if !strings.Contains(domain, ".") {
		return "", fmt.Errorf("%w: domain %q has no dot", ErrEmailInvalid, domain)
	}
	return strings.ToLower(addr.Address), nil
}

// CheckPassword applies the offering's floor. It deliberately does not score
// complexity: a length floor plus "not the thing next to it on the screen" is
// what a first-run form can enforce honestly, and a composition rule pushes
// people toward one predictable substitution.
func CheckPassword(kind offering.Kind, email, password string) error {
	min := MinPassword(kind)
	if utf8.RuneCountInString(password) < min {
		return fmt.Errorf("%w: at least %d characters", ErrPasswordWeak, min)
	}
	if strings.TrimSpace(password) == "" {
		return fmt.Errorf("%w: whitespace only", ErrPasswordWeak)
	}
	if strings.EqualFold(strings.TrimSpace(email), strings.TrimSpace(password)) {
		return fmt.Errorf("%w: same as the email address", ErrPasswordWeak)
	}
	return nil
}

// argon2id parameters. These match what upstream wrote, on purpose: a hub
// that was set up before this package existed keeps a verifiable hash.
const (
	argonTime    = 3
	argonMemory  = 64 * 1024
	argonThreads = 2
	argonKeyLen  = 32
	argonSaltLen = 16
)

// HashPassword returns an argon2id hash in the inherited
// "argon2id$<salt>$<key>" encoding.
//
// Unlike the code this replaces, a failure to read the random source is
// returned rather than ignored. Ignoring it produces an all-zero salt, so
// every hub on earth would share one, and nothing about the result looks
// wrong.
func HashPassword(password string) (string, error) {
	salt, err := randomBytes(argonSaltLen)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("argon2id$%s$%s", encode(salt), encode(key)), nil
}

// VerifyPassword reports whether password matches a stored hash. A malformed
// or empty stored value is a mismatch, never an accidental pass.
func VerifyPassword(stored, password string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 3 || parts[0] != "argon2id" {
		return false
	}
	salt, err := decode(parts[1])
	if err != nil {
		return false
	}
	want, err := decode(parts[2])
	if err != nil || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// decoySalt is deliberately fixed and all-zero: nothing derived from it is
// stored or compared. Its only job is to make the work measurable.
var decoySalt = make([]byte, argonSaltLen)

// BurnVerify spends roughly the work VerifyPassword spends, and answers
// nothing.
//
// A login form that returns immediately when no account has that address,
// and slowly when one does, tells an attacker who has an account here. This
// keeps the two paths comparable. It is not a constant-time guarantee — the
// surrounding request has its own variance — and it does not hide anything
// an invite flow would reveal anyway.
func BurnVerify(password string) {
	argon2.IDKey([]byte(password), decoySalt, argonTime, argonMemory, argonThreads, argonKeyLen)
}

// randomBytes is a seam so a test can prove the error path of the callers
// above without a broken machine. Production always reads crypto/rand.
var randomBytes = func(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

func encode(b []byte) string { return base64.RawStdEncoding.EncodeToString(b) }

func decode(s string) ([]byte, error) { return base64.RawStdEncoding.DecodeString(s) }
