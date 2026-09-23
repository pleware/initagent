// Package voices holds the text-to-speech catalog: every voice this fleet
// serves, with its language and — since 2026-09-23 — the engine that speaks it.
//
// It began as the Piper voices alone and one non-Piper engine joined it without
// pretending to be a Piper voice. A Piper entry carries no Engine and its
// Language and Quality are derivable from its name (`xx_YY-name-quality`); an
// entry such as voxcpm2 carries Engine and claims no name shape at all. The two
// are told apart with IsPiper, and callers that parse a name must ask first.
//
// The source of truth is the embedded voices.json, which consumers in other
// repositories (pware) fetch by path — this package only reads it.
package voices

import (
	"cmp"
	_ "embed"
	"encoding/json"
	"slices"
)

//go:embed voices.json
var embeddedJSON []byte

// Voice is one Piper voice in the catalog. Name is the full Piper voice name
// (e.g. "en_US-lessac-medium"), Language the xx_YY prefix before the first
// "-", and Quality the suffix after the last "-" ("low", "medium", "high").
type Voice struct {
	Name     string `json:"name"`
	Language string `json:"language"`
	Quality  string `json:"quality"`
	// Engine names the engine that speaks this voice. Empty means Piper, which
	// is what every historical entry is — hence omitempty: the 43 Piper voices
	// carry no engine key and their shape is exactly what it was. A non-empty
	// value means the entry is that engine's voice, name and quality are that
	// engine's own vocabulary, and nothing about a Piper name shape applies.
	Engine string `json:"engine,omitempty"`
}

// IsPiper reports whether this is a Piper voice — the historical shape, where
// Engine is empty. Anything that parses a voice name as
// `xx_YY-name-quality` must ask this first: a non-Piper entry has no such
// shape to parse.
func (v Voice) IsPiper() bool { return v.Engine == "" }

// languageOrder is the display order of the catalog's languages: Polish
// first (the product's home market), then US English (the largest group),
// then GB English. Deliberately not lexicographic.
var languageOrder = []string{"pl_PL", "en_US", "en_GB"}

var languageRank = func() map[string]int {
	rank := make(map[string]int, len(languageOrder))
	for i, lang := range languageOrder {
		rank[lang] = i
	}
	return rank
}()

var catalog = mustLoadCatalog(embeddedJSON)

// mustLoadCatalog parses catalog JSON. The embedded file is committed and
// locked by tests, so a parse failure is a programming error, not runtime I/O.
func mustLoadCatalog(data []byte) []Voice {
	voices, err := parseCatalog(data)
	if err != nil {
		panic("voices: parse embedded catalog: " + err.Error())
	}
	return voices
}

func parseCatalog(data []byte) ([]Voice, error) {
	var voices []Voice
	if err := json.Unmarshal(data, &voices); err != nil {
		return nil, err
	}
	return voices, nil
}

// List returns every voice in the catalog, sorted in display order: pl_PL
// first, then en_US, then en_GB, by Name within each language.
func List() []Voice {
	return slices.SortedFunc(slices.Values(catalog), func(a, b Voice) int {
		if ra, rb := languageRank[a.Language], languageRank[b.Language]; ra != rb {
			return cmp.Compare(ra, rb)
		}
		return cmp.Compare(a.Name, b.Name)
	})
}

// Languages returns the catalog's languages in display order — pl_PL, then
// en_US, then en_GB. The order is a product decision, not lexicographic, so
// callers must not sort it.
func Languages() []string {
	return slices.Clone(languageOrder)
}
