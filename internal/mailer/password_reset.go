package mailer

import "html"

// KindPasswordReset is the outbox template name for a forgotten password.
const KindPasswordReset = "password_reset"

// PasswordReset is the letter for a forgotten password. The body follows
// the language stored on the account (en or pl). An unknown locale still
// ships both languages so a row that predates the column is readable.
func PasswordReset(link, locale string) (subject, text, htmlBody string) {
	safe := html.EscapeString(link)
	switch locale {
	case "pl":
		subject = "Zresetuj hasło"
		text = "Zresetuj hasło\n\n" +
			"Otwórz ten link w ciągu godziny:\n" +
			link + "\n\n" +
			"Jeśli tego nie prosiłeś, zignoruj tę wiadomość.\n"
		htmlBody = `<p>Zresetuj hasło</p>
<p><a href="` + safe + `">Zresetuj hasło</a></p>
<p>Link wygasa po godzinie. Jeśli tego nie prosiłeś, zignoruj tę wiadomość.</p>`
		return subject, text, htmlBody
	case "en":
		subject = "Reset your password"
		text = "Reset your password\n\n" +
			"Open this link within one hour:\n" +
			link + "\n\n" +
			"If you did not ask for this, you can ignore this email.\n"
		htmlBody = `<p>Reset your password</p>
<p><a href="` + safe + `">Reset password</a></p>
<p>This link expires in one hour. If you did not ask for this, ignore this email.</p>`
		return subject, text, htmlBody
	default:
		subject = "Reset your password / Zresetuj hasło"
		text = "Reset your password\n\n" +
			"Open this link within one hour:\n" +
			link + "\n\n" +
			"If you did not ask for this, you can ignore this email.\n\n" +
			"—\n\n" +
			"Zresetuj hasło\n\n" +
			"Otwórz ten link w ciągu godziny:\n" +
			link + "\n\n" +
			"Jeśli tego nie prosiłeś, zignoruj tę wiadomość.\n"
		htmlBody = `<p>Reset your password</p>
<p><a href="` + safe + `">Reset password</a></p>
<p>This link expires in one hour. If you did not ask for this, ignore this email.</p>
<hr>
<p>Zresetuj hasło</p>
<p><a href="` + safe + `">Zresetuj hasło</a></p>
<p>Link wygasa po godzinie. Jeśli tego nie prosiłeś, zignoruj tę wiadomość.</p>`
		return subject, text, htmlBody
	}
}
