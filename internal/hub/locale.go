package hub

import (
	"strings"
	"unicode"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

// localeDisplayName returns a locale's language name in its own language with
// the first letter capitalised — "pl" → "Polski", "en" → "English",
// "de" → "Deutsch". The underscore variant ("pl_PL") is accepted and
// normalised to the BCP 47 dash form. An empty or unparsable locale falls back
// to the code itself, so a bad value never turns into an empty label.
//
// The region is dropped: the display name is the base language, so "pl" and
// "pl_PL" both answer "Polski" (the narrator's language picker shows languages,
// not country variants).
func localeDisplayName(locale string) string {
	if locale == "" {
		return ""
	}
	tag, err := language.Parse(strings.ReplaceAll(locale, "_", "-"))
	if err != nil {
		return locale
	}
	name := display.Self.Name(tag)
	if name == "" {
		return locale
	}
	r := []rune(name)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
