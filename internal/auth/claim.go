package auth

import (
	"crypto/subtle"
	"encoding/hex"
	"fmt"

	"github.com/pleware/initagent/internal/offering"
)

// ClaimTokenBytes is the entropy behind a bootstrap token. It is a secret
// with a lifetime of one hub start, so there is no reason to be frugal.
const ClaimTokenBytes = 32

// NewClaimToken mints the one-time secret that proves the operator during
// first-run.
//
// Why this exists at all: upstream let whoever posted first set the password
// on an empty hub. On a laptop that is convenience; on a public name it is a
// land grab, and the loser is the real operator. The token turns "first
// stranger to find the URL" into "someone who can read the hub's own log or
// data directory", which is the operator by definition (26).
//
// The token is not persisted across a restart on purpose. An unclaimed hub
// mints a fresh one every time it starts, which is also the entire recovery
// story when the operator loses it: restart and read the new one. Nothing to
// reset, no support path to build.
func NewClaimToken() (string, error) {
	b, err := randomBytes(ClaimTokenBytes)
	if err != nil {
		return "", fmt.Errorf("mint claim token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Claimed reports whether the hub already has an owner.
//
// An account row is the modern answer. Upstream's anonymous password setting
// counts too: a hub set up before accounts existed would otherwise read as
// unclaimed after an upgrade, and a stranger could claim a hub that already
// belongs to somebody (26 records this as the legacy path).
func Claimed(accounts int, legacyPasswordHash string) bool {
	return accounts > 0 || legacyPasswordHash != ""
}

// State is what the hub knows about itself when a claim arrives.
type State struct {
	Offering offering.Kind
	Claimed  bool
	// ExpectedToken is the token minted at start. Empty means the hub has no
	// token to compare against, which can never authorise a claim.
	ExpectedToken string
}

// ClaimRequest is the untrusted first-run submission. Confirm-password is
// deliberately absent: it is a typo guard on the form, and two fields
// carrying one value on the wire would only add a second way to fail (26).
type ClaimRequest struct {
	Email    string
	Password string
	Token    string
}

// Credentials is what the hub stores once a claim is accepted.
type Credentials struct {
	Email        string
	PasswordHash string
}

// Claim decides whether a first-run submission may take ownership, and
// returns the values to persist.
//
// The order of the checks is part of the decision:
//
//   - Claimed first, so a claimed hub answers the same way whatever token is
//     submitted. Checking the token first would turn this endpoint into an
//     oracle for guessing it.
//   - The token before the email and password, so an unauthenticated caller
//     cannot make us run argon2id — a deliberately expensive function — once
//     per request.
func Claim(st State, req ClaimRequest) (Credentials, error) {
	if st.Claimed {
		return Credentials{}, ErrAlreadyClaimed
	}
	if st.ExpectedToken == "" || subtle.ConstantTimeCompare([]byte(st.ExpectedToken), []byte(req.Token)) != 1 {
		return Credentials{}, ErrClaimToken
	}
	email, err := NormalizeEmail(req.Email)
	if err != nil {
		return Credentials{}, err
	}
	if err := CheckPassword(st.Offering, email, req.Password); err != nil {
		return Credentials{}, err
	}
	hash, err := HashPassword(req.Password)
	if err != nil {
		return Credentials{}, err
	}
	return Credentials{Email: email, PasswordHash: hash}, nil
}
