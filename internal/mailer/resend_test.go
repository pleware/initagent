package mailer

import (
	"context"
	"testing"

	"github.com/resend/resend-go/v4"
)

func TestResendSend(t *testing.T) {
	t.Parallel()
	r := &Resend{
		from: "Init <noreply@example.com>",
		send: func(_ context.Context, params *resend.SendEmailRequest) (*resend.SendEmailResponse, error) {
			if params.From != "Init <noreply@example.com>" {
				t.Fatalf("From = %q", params.From)
			}
			if len(params.To) != 1 || params.To[0] != "a@b.c" {
				t.Fatalf("To = %v", params.To)
			}
			if params.Subject != "Reset" || params.Text != "hi" {
				t.Fatalf("body %+v", params)
			}
			if len(params.Tags) != 2 || params.Tags[1].Value != "eml-1" {
				t.Fatalf("tags %+v", params.Tags)
			}
			return &resend.SendEmailResponse{Id: "re_abc"}, nil
		},
	}
	id, err := r.Send(t.Context(), Message{
		ID: "eml-1", Kind: "password_reset", To: "a@b.c", Subject: "Reset", Text: "hi",
	})
	if err != nil || id != "re_abc" {
		t.Fatalf("Send = %q %v", id, err)
	}
}

func TestResendRejectsEmpty(t *testing.T) {
	t.Parallel()
	r := &Resend{from: "a@b.c", send: func(context.Context, *resend.SendEmailRequest) (*resend.SendEmailResponse, error) {
		t.Fatal("should not call Resend")
		return nil, nil
	}}
	if _, err := r.Send(t.Context(), Message{Subject: "x", Text: "hi"}); err == nil {
		t.Fatal("empty To")
	}
	if _, err := r.Send(t.Context(), Message{To: "a@b.c", Text: "hi"}); err == nil {
		t.Fatal("empty subject")
	}
	if _, err := r.Send(t.Context(), Message{To: "a@b.c", Subject: "x"}); err == nil {
		t.Fatal("empty body")
	}
}
