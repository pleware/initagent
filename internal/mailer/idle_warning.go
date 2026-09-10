package mailer

import (
	"html"
	"strconv"
)

// KindIdleWarning is the outbox template name for a free-project idle notice.
const KindIdleWarning = "idle_warning"

// IdleWarning is the letter that a free project will be deleted unless
// someone uses it. Locale follows the owner's account language. An
// unknown locale ships both languages so a row that predates the
// column is readable.
func IdleWarning(projectName, link string, daysLeft int, locale string) (subject, text, htmlBody string) {
	safeName := html.EscapeString(projectName)
	safeLink := html.EscapeString(link)
	days := strconv.Itoa(max(1, daysLeft))
	switch locale {
	case "pl":
		subject = "Projekt " + projectName + " zostanie usunięty"
		text = "Projekt " + projectName + " jest bezczynny.\n\n" +
			"Otwórz go w ciągu " + days + " dni, albo usuniemy go z darmowego planu:\n" +
			link + "\n\n" +
			"Jeśli już nie potrzebujesz tego projektu, możesz zignorować tę wiadomość.\n"
		htmlBody = `<p>Projekt ` + safeName + ` jest bezczynny.</p>
<p><a href="` + safeLink + `">Otwórz projekt</a> w ciągu ` + days + ` dni, albo usuniemy go z darmowego planu.</p>
<p>Jeśli już nie potrzebujesz tego projektu, możesz zignorować tę wiadomość.</p>`
		return subject, text, htmlBody
	case "en":
		subject = "Project " + projectName + " will be deleted"
		text = "Project " + projectName + " has been idle.\n\n" +
			"Open it within " + days + " days, or we will delete it on the free plan:\n" +
			link + "\n\n" +
			"If you no longer need this project, you can ignore this email.\n"
		htmlBody = `<p>Project ` + safeName + ` has been idle.</p>
<p><a href="` + safeLink + `">Open the project</a> within ` + days + ` days, or we will delete it on the free plan.</p>
<p>If you no longer need this project, you can ignore this email.</p>`
		return subject, text, htmlBody
	default:
		subject = "Project " + projectName + " will be deleted / Projekt " + projectName + " zostanie usunięty"
		text = "Project " + projectName + " has been idle.\n\n" +
			"Open it within " + days + " days, or we will delete it on the free plan:\n" +
			link + "\n\n" +
			"If you no longer need this project, you can ignore this email.\n\n" +
			"—\n\n" +
			"Projekt " + projectName + " jest bezczynny.\n\n" +
			"Otwórz go w ciągu " + days + " dni, albo usuniemy go z darmowego planu:\n" +
			link + "\n\n" +
			"Jeśli już nie potrzebujesz tego projektu, możesz zignorować tę wiadomość.\n"
		htmlBody = `<p>Project ` + safeName + ` has been idle.</p>
<p><a href="` + safeLink + `">Open the project</a> within ` + days + ` days, or we will delete it on the free plan.</p>
<p>If you no longer need this project, you can ignore this email.</p>
<hr>
<p>Projekt ` + safeName + ` jest bezczynny.</p>
<p><a href="` + safeLink + `">Otwórz projekt</a> w ciągu ` + days + ` dni, albo usuniemy go z darmowego planu.</p>
<p>Jeśli już nie potrzebujesz tego projektu, możesz zignorować tę wiadomość.</p>`
		return subject, text, htmlBody
	}
}
