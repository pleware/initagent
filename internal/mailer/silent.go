package mailer

import "context"

// Silent delivers nothing and reports success. Self-host with no SMTP
// uses this so the outbox does not pile up, and so later SMTP can replace
// it without a second queue.
type Silent struct{}

// Send implements Sender.
func (Silent) Send(context.Context, Message) (string, error) {
	return "", nil
}
