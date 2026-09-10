package mailer

import "html"

// KindInvite is the outbox template name for an organization invitation.
const KindInvite = "org_invite"

// Invite is the letter that asks someone to join an organization. Locale
// follows the inviter's account language. An unknown locale ships both
// languages so a row that predates the column is readable.
func Invite(orgName, link, locale string) (subject, text, htmlBody string) {
	safe := html.EscapeString(link)
	safeOrg := html.EscapeString(orgName)
	switch locale {
	case "pl":
		subject = "Zaproszenie do " + orgName
		text = "Zaproszenie do " + orgName + "\n\n" +
			"Otwórz ten link w ciągu 7 dni, żeby dołączyć:\n" +
			link + "\n\n" +
			"Jeśli nie spodziewałeś się tej wiadomości, zignoruj ją.\n"
		htmlBody = `<p>Zaproszenie do ` + safeOrg + `</p>
<p><a href="` + safe + `">Dołącz</a></p>
<p>Link wygasa po 7 dniach. Jeśli nie spodziewałeś się tej wiadomości, zignoruj ją.</p>`
		return subject, text, htmlBody
	case "en":
		subject = "Invitation to " + orgName
		text = "Invitation to " + orgName + "\n\n" +
			"Open this link within 7 days to join:\n" +
			link + "\n\n" +
			"If you were not expecting this, you can ignore this email.\n"
		htmlBody = `<p>Invitation to ` + safeOrg + `</p>
<p><a href="` + safe + `">Join</a></p>
<p>This link expires in 7 days. If you were not expecting this, ignore this email.</p>`
		return subject, text, htmlBody
	default:
		subject = "Invitation to " + orgName + " / Zaproszenie do " + orgName
		text = "Invitation to " + orgName + "\n\n" +
			"Open this link within 7 days to join:\n" +
			link + "\n\n" +
			"If you were not expecting this, you can ignore this email.\n\n" +
			"—\n\n" +
			"Zaproszenie do " + orgName + "\n\n" +
			"Otwórz ten link w ciągu 7 dni, żeby dołączyć:\n" +
			link + "\n\n" +
			"Jeśli nie spodziewałeś się tej wiadomości, zignoruj ją.\n"
		htmlBody = `<p>Invitation to ` + safeOrg + `</p>
<p><a href="` + safe + `">Join</a></p>
<p>This link expires in 7 days. If you were not expecting this, ignore this email.</p>
<hr>
<p>Zaproszenie do ` + safeOrg + `</p>
<p><a href="` + safe + `">Dołącz</a></p>
<p>Link wygasa po 7 dniach. Jeśli nie spodziewałeś się tej wiadomości, zignoruj ją.</p>`
		return subject, text, htmlBody
	}
}
