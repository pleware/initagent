package auth

import (
	"encoding/hex"
	"fmt"
	"time"
)

// ResetTTL is how long a password-reset secret may be redeemed. Short enough
// that a leaked mailbox is not a standing credential, long enough that a
// person can open the mail on a phone and type a new password.
const ResetTTL = time.Hour

// ResetTokenBytes is the entropy behind the secret in the mail. It is not an
// identifier: the row is `rst-`, and only a SHA-256 of this value is stored.
const ResetTokenBytes = 32

// NewResetToken mints the one-time secret that goes in the reset link.
func NewResetToken() (string, error) {
	b, err := randomBytes(ResetTokenBytes)
	if err != nil {
		return "", fmt.Errorf("mint reset token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
