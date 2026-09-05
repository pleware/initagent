package auth

import (
	"fmt"
	"strings"
)

// Locales the hub stores on an account. The cockpit and the marketing site
// share this allowlist: a value that is not here is a bad request, not a
// silent fallback, so a form cannot invent a language we have no mail or
// translations for.
const (
	LocaleEN = "en"
	LocalePL = "pl"
)

// NormalizeLocale maps a submitted BCP 47 tag onto the stored vocabulary.
// Empty means English: older clients and first-run forms that omit the field
// still mint a usable account. A region tag ("pl-PL") collapses to the
// language. Anything else is ErrLocale.
func NormalizeLocale(raw string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return LocaleEN, nil
	}
	base := trimmed
	if i := strings.IndexAny(trimmed, "-_"); i >= 0 {
		base = trimmed[:i]
	}
	switch base {
	case LocaleEN, LocalePL:
		return base, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrLocale, trimmed)
	}
}
