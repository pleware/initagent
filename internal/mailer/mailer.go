// Package mailer is the hub's outbound-mail seam (draft 26).
//
// Hosted sends through Resend. Self-host with no key is silent. The hub
// outbox owns retry; this package only delivers one message or returns an
// error. Callers never import the Resend SDK.
package mailer

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrNotConfigured means hosted mail was queued but no Resend key is set.
// The outbox leaves the row pending until an operator fills the env and
// restarts; it does not burn retry attempts.
var ErrNotConfigured = errors.New("mailer: not configured")

// Message is one outbound letter. ID is the outbox row (`eml-`).
type Message struct {
	ID      string
	Kind    string
	To      string
	Subject string
	Text    string
	HTML    string
}

// Sender delivers one message. Implementations must be safe for concurrent
// drain loops.
type Sender interface {
	Send(ctx context.Context, msg Message) (providerID string, err error)
}

// New picks the sender for this process. A nil sender means the drain should
// not claim rows (hosted, no API key yet). A key without From is a start
// error so we do not enqueue mail we cannot address.
func New(hosted bool, apiKey, from string) (Sender, error) {
	apiKey = strings.TrimSpace(apiKey)
	from = strings.TrimSpace(from)
	if apiKey != "" {
		if from == "" {
			return nil, fmt.Errorf("mailer: API key is set but From is empty")
		}
		return newResend(apiKey, from), nil
	}
	if hosted {
		return nil, nil
	}
	return Silent{}, nil
}
