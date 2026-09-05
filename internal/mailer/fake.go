package mailer

import "context"

// Fake records Send calls. Set Err to fail the next call.
type Fake struct {
	Err  error
	Sent []Message
}

// Send implements Sender.
func (f *Fake) Send(_ context.Context, msg Message) (string, error) {
	if f.Err != nil {
		return "", f.Err
	}
	f.Sent = append(f.Sent, msg)
	return "fake-" + msg.ID, nil
}
