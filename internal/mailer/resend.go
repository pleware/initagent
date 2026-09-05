package mailer

import (
	"context"
	"fmt"
	"strings"

	"github.com/resend/resend-go/v4"
)

type sendFunc func(ctx context.Context, params *resend.SendEmailRequest) (*resend.SendEmailResponse, error)

// Resend delivers through the official Go SDK. The API key never leaves
// this adapter; the hub outbox only sees provider ids and errors.
type Resend struct {
	from string
	send sendFunc
}

func newResend(apiKey, from string) *Resend {
	client := resend.NewClient(apiKey)
	return &Resend{from: from, send: client.Emails.SendWithContext}
}

// Send implements Sender.
func (r *Resend) Send(ctx context.Context, msg Message) (string, error) {
	to := strings.TrimSpace(msg.To)
	if to == "" {
		return "", fmt.Errorf("mailer: empty To")
	}
	if strings.TrimSpace(msg.Subject) == "" {
		return "", fmt.Errorf("mailer: empty subject")
	}
	if msg.Text == "" && msg.HTML == "" {
		return "", fmt.Errorf("mailer: empty body")
	}
	sent, err := r.send(ctx, &resend.SendEmailRequest{
		From:    r.from,
		To:      []string{to},
		Subject: msg.Subject,
		Text:    msg.Text,
		Html:    msg.HTML,
		Tags: []resend.Tag{
			{Name: "kind", Value: msg.Kind},
			{Name: "outbox", Value: msg.ID},
		},
	})
	if err != nil {
		return "", err
	}
	if sent == nil {
		return "", fmt.Errorf("mailer: empty Resend response")
	}
	return sent.Id, nil
}
