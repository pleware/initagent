package mailer

import (
	"strings"
	"testing"
)

func TestIdleWarningEnglish(t *testing.T) {
	t.Parallel()
	const link = "https://app.example/code/project-1"
	subject, text, htmlBody := IdleWarning("Storefront", link, 14, "en")
	if subject != "Project Storefront will be deleted" {
		t.Fatalf("subject = %q", subject)
	}
	for _, body := range []string{text, htmlBody} {
		if !strings.Contains(body, link) {
			t.Fatalf("missing link in body: %s", body)
		}
		if !strings.Contains(body, "Storefront") {
			t.Fatalf("missing project name: %s", body)
		}
		if !strings.Contains(body, "14") {
			t.Fatalf("missing days left: %s", body)
		}
		if strings.Contains(body, "zostanie usunięty") || strings.Contains(body, "bezczynny") {
			t.Fatalf("English mail still has Polish: %s", body)
		}
	}
	if !strings.Contains(htmlBody, `href="`+link+`"`) {
		t.Fatalf("html href: %s", htmlBody)
	}
}

func TestIdleWarningPolish(t *testing.T) {
	t.Parallel()
	const link = "https://app.example/code/project-1"
	subject, text, htmlBody := IdleWarning("Storefront", link, 14, "pl")
	if subject != "Projekt Storefront zostanie usunięty" {
		t.Fatalf("subject = %q", subject)
	}
	for _, body := range []string{text, htmlBody} {
		if !strings.Contains(body, "bezczynny") {
			t.Fatalf("missing Polish copy: %s", body)
		}
		if strings.Contains(body, "will be deleted") {
			t.Fatalf("Polish mail still has English: %s", body)
		}
	}
}

func TestIdleWarningUnknownLocaleIsBilingual(t *testing.T) {
	t.Parallel()
	const link = "https://app.example/code/project-1"
	subject, text, htmlBody := IdleWarning("Storefront", link, 14, "")
	if !strings.Contains(subject, "will be deleted") || !strings.Contains(subject, "zostanie usunięty") {
		t.Fatalf("subject = %q", subject)
	}
	for _, body := range []string{text, htmlBody} {
		if !strings.Contains(body, "has been idle") || !strings.Contains(body, "bezczynny") {
			t.Fatalf("mail is not bilingual: %s", body)
		}
	}
}

func TestIdleWarningEscapesHTML(t *testing.T) {
	t.Parallel()
	_, _, htmlBody := IdleWarning(`Ops<"x">`, `https://x/"onclick="alert(1)`, 14, "en")
	if strings.Contains(htmlBody, `onclick="alert`) {
		t.Fatal("href was not escaped")
	}
	if strings.Contains(htmlBody, `<"x">`) {
		t.Fatal("project name was not escaped")
	}
}

func TestIdleWarningDaysLeftAtLeastOne(t *testing.T) {
	t.Parallel()
	_, text, _ := IdleWarning("Storefront", "https://app.example/code/project-1", 0, "en")
	if !strings.Contains(text, "1 days") && !strings.Contains(text, "1 day") {
		if !strings.Contains(text, "1") {
			t.Fatalf("zero daysLeft must still show at least one: %s", text)
		}
	}
}
