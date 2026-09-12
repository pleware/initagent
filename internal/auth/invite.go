package auth

import (
	"encoding/hex"
	"fmt"
	"time"
)

// InviteTTL is how long an unused invitation may be redeemed.
//
// A password-reset secret lasts an hour because the same person is sitting
// at the inbox. An invite sits in someone else's mail, may wait over a
// weekend, and may be forwarded by the founder. Seven days is long enough
// for that and short enough that a leaked mailbox is not a standing grant.
const InviteTTL = 7 * 24 * time.Hour

// InviteTokenBytes is the entropy behind the secret in the invite link.
// The row is `org_invite-`; only a SHA-256 of this value is stored.
const InviteTokenBytes = 32

// NewInviteToken mints the one-time secret that goes in the invite link.
func NewInviteToken() (string, error) {
	b, err := randomBytes(InviteTokenBytes)
	if err != nil {
		return "", fmt.Errorf("mint invite token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
