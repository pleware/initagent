package hub

import (
	"net/http"
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

// localeCodes is the staff language picker's catalog — the languages the box
// offers today, "pl" first. Names come from localeDisplayName (CLDR), so the
// list stays a code list; a name is never hand-written and never drifts.
var localeCodes = []string{
	"pl", "en", "de", "fr", "es", "it", "uk", "cs", "sk",
	"ru", "nl", "sv", "no", "da", "fi", "hu", "ro", "bg", "el", "tr", "pt",
}

// localeEntry is one language in the picker catalog.
type localeEntry struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// listLocales returns the staff language picker's catalog, "pl" first, each
// code with its display name in its own language.
func listLocales() []localeEntry {
	out := make([]localeEntry, 0, len(localeCodes))
	for _, code := range localeCodes {
		out = append(out, localeEntry{Code: code, Name: localeDisplayName(code)})
	}
	return out
}

// handleListLocales serves the language picker catalog. Like the voice and
// skill catalogs, what an installation offers is pre-auth.
func (s *Server) handleListLocales(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, listLocales())
}
