package mailer

import (
	"strings"
	"testing"
)

func TestInviteEnglish(t *testing.T) {
	t.Parallel()
	const link = "https://app.example/invite?token=abc"
	subject, text, htmlBody := Invite("Example Ops", link, "en")
	if subject != "Invitation to Example Ops" {
		t.Fatalf("subject = %q", subject)
	}
	for _, body := range []string{text, htmlBody} {
		if !strings.Contains(body, link) {
			t.Fatalf("missing link in body: %s", body)
		}
		if !strings.Contains(body, "Example Ops") {
			t.Fatalf("missing org name: %s", body)
		}
		if strings.Contains(body, "Zaproszenie") {
			t.Fatalf("English mail still has Polish: %s", body)
		}
	}
	if !strings.Contains(htmlBody, `href="`+link+`"`) {
		t.Fatalf("html href: %s", htmlBody)
	}
}

func TestInvitePolish(t *testing.T) {
	t.Parallel()
	const link = "https://app.example/invite?token=abc"
	subject, text, htmlBody := Invite("Example Ops", link, "pl")
	if subject != "Zaproszenie do Example Ops" {
		t.Fatalf("subject = %q", subject)
	}
	for _, body := range []string{text, htmlBody} {
		if !strings.Contains(body, "Zaproszenie") {
			t.Fatalf("missing Polish copy: %s", body)
		}
		if strings.Contains(body, "Invitation to") {
			t.Fatalf("Polish mail still has English: %s", body)
		}
	}
}

func TestInviteUnknownLocaleIsBilingual(t *testing.T) {
	t.Parallel()
	const link = "https://app.example/invite?token=abc"
	subject, text, htmlBody := Invite("Example Ops", link, "")
	if !strings.Contains(subject, "Invitation to") || !strings.Contains(subject, "Zaproszenie") {
		t.Fatalf("subject = %q", subject)
	}
	for _, body := range []string{text, htmlBody} {
		if !strings.Contains(body, "Invitation") || !strings.Contains(body, "Zaproszenie") {
			t.Fatalf("mail is not bilingual: %s", body)
		}
	}
}

func TestInviteEscapesHTML(t *testing.T) {
	t.Parallel()
	_, _, htmlBody := Invite(`Ops<"x">`, `https://x/"onclick="alert(1)`, "en")
	if strings.Contains(htmlBody, `onclick="alert`) {
		t.Fatal("href was not escaped")
	}
	if strings.Contains(htmlBody, `<"x">`) {
		t.Fatal("org name was not escaped")
	}
}
