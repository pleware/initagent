package mailer

import (
	"strings"
	"testing"
)

func TestPasswordResetEnglish(t *testing.T) {
	t.Parallel()
	const link = "https://app.example/reset?token=abc"
	subject, text, htmlBody := PasswordReset(link, "en")
	if subject != "Reset your password" {
		t.Fatalf("subject = %q", subject)
	}
	for _, body := range []string{text, htmlBody} {
		if !strings.Contains(body, link) {
			t.Fatalf("missing link in body: %s", body)
		}
		if !strings.Contains(body, "Reset") {
			t.Fatalf("missing English copy: %s", body)
		}
		if strings.Contains(body, "Zresetuj") {
			t.Fatalf("English mail still has Polish: %s", body)
		}
	}
	if !strings.Contains(htmlBody, `href="`+link+`"`) {
		t.Fatalf("html href: %s", htmlBody)
	}
}

func TestPasswordResetPolish(t *testing.T) {
	t.Parallel()
	const link = "https://app.example/reset?token=abc"
	subject, text, htmlBody := PasswordReset(link, "pl")
	if subject != "Zresetuj hasło" {
		t.Fatalf("subject = %q", subject)
	}
	for _, body := range []string{text, htmlBody} {
		if !strings.Contains(body, "Zresetuj") {
			t.Fatalf("missing Polish copy: %s", body)
		}
		if strings.Contains(body, "Reset your password") {
			t.Fatalf("Polish mail still has English: %s", body)
		}
	}
}

func TestPasswordResetUnknownLocaleIsBilingual(t *testing.T) {
	t.Parallel()
	const link = "https://app.example/reset?token=abc"
	subject, text, htmlBody := PasswordReset(link, "")
	if !strings.Contains(subject, "Reset your password") || !strings.Contains(subject, "Zresetuj") {
		t.Fatalf("subject = %q", subject)
	}
	for _, body := range []string{text, htmlBody} {
		if !strings.Contains(body, "Reset") || !strings.Contains(body, "Zresetuj") {
			t.Fatalf("mail is not bilingual: %s", body)
		}
	}
}

func TestPasswordResetEscapesHTML(t *testing.T) {
	t.Parallel()
	_, _, htmlBody := PasswordReset(`https://x/"onclick="alert(1)`, "en")
	if strings.Contains(htmlBody, `onclick="alert`) {
		t.Fatal("href was not escaped")
	}
}
