package voices

import (
	"cmp"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestListHoldsTheFullCatalog(t *testing.T) {
	t.Parallel()
	voices := List()

	if got, want := len(voices), 43; got != want {
		t.Fatalf("List() returned %d voices, want %d", got, want)
	}

	byLanguage := map[string]int{}
	for _, v := range voices {
		byLanguage[v.Language]++
	}
	wantCounts := map[string]int{"pl_PL": 5, "en_GB": 11, "en_US": 27}
	if len(byLanguage) != len(wantCounts) {
		t.Fatalf("languages = %v, want exactly %v", byLanguage, wantCounts)
	}
	for lang, want := range wantCounts {
		if byLanguage[lang] != want {
			t.Errorf("%s has %d voices, want %d", lang, byLanguage[lang], want)
		}
	}
}

func TestListIsInDisplayOrder(t *testing.T) {
	t.Parallel()
	voices := List()

	// Voices appear as contiguous language blocks in Languages() order,
	// and each block is sorted by Name.
	blockStart := 0
	for _, lang := range Languages() {
		i := blockStart
		for ; i < len(voices) && voices[i].Language == lang; i++ {
		}
		if i == blockStart {
			t.Fatalf("no voices for language %s", lang)
		}
		block := voices[blockStart:i]
		if !slices.IsSortedFunc(block, func(a, b Voice) int {
			return cmp.Compare(a.Name, b.Name)
		}) {
			t.Errorf("%s block not sorted by Name: %v", lang, names(block))
		}
		blockStart = i
	}
	if blockStart != len(voices) {
		t.Errorf("%d voices fall outside the %v blocks: %v",
			len(voices)-blockStart, Languages(), names(voices[blockStart:]))
	}
}

func TestLanguagesReturnsDisplayOrder(t *testing.T) {
	t.Parallel()
	got := Languages()
	want := []string{"pl_PL", "en_US", "en_GB"}
	if !slices.Equal(got, want) {
		t.Errorf("Languages() = %v, want %v (display order, not lexicographic)", got, want)
	}
}

func TestEveryVoiceDerivesLanguageAndQualityFromItsName(t *testing.T) {
	t.Parallel()
	for _, v := range List() {
		dash := strings.IndexByte(v.Name, '-')
		if dash < 0 {
			t.Errorf("%s: no language prefix", v.Name)
			continue
		}
		if lang := v.Name[:dash]; lang != v.Language {
			t.Errorf("%s: language %q, want %q", v.Name, v.Language, lang)
		}
		if qual := v.Name[strings.LastIndexByte(v.Name, '-')+1:]; qual != v.Quality {
			t.Errorf("%s: quality %q, want %q", v.Name, v.Quality, qual)
		}
	}
}

// TestEmbeddedJSONIsTheCatalog is the golden gate on the committed file: the
// embedded JSON must parse and hold exactly the catalog List() serves. pware
// fetches voices.json by path, so the two cannot be allowed to drift.
func TestEmbeddedJSONIsTheCatalog(t *testing.T) {
	t.Parallel()
	var got []Voice
	if err := json.Unmarshal(embeddedJSON, &got); err != nil {
		t.Fatalf("embedded voices.json does not parse: %v", err)
	}

	want := List()
	got = sortedByName(got)
	want = sortedByName(want)
	if !slices.Equal(got, want) {
		t.Errorf("embedded voices.json does not match the catalog:\n json: %v\nList(): %v",
			names(got), names(want))
	}
}

func TestMustLoadCatalogPanicsOnCorruptJSON(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Error("mustLoadCatalog(corrupt JSON) did not panic")
		}
	}()
	mustLoadCatalog([]byte("{not json"))
}

func TestParseCatalogRejectsInvalidJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data string
	}{
		{name: "empty", data: ""},
		{name: "not an array", data: `{"name": "pl_PL-bass-high"}`},
		{name: "malformed entry", data: `[{"name": "pl_PL-bass-high"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseCatalog([]byte(tt.data)); err == nil {
				t.Errorf("parseCatalog(%q) returned no error", tt.data)
			}
		})
	}
}

func names(voices []Voice) []string {
	out := make([]string, len(voices))
	for i, v := range voices {
		out[i] = v.Name
	}
	return out
}

func sortedByName(voices []Voice) []Voice {
	return slices.SortedFunc(slices.Values(voices), func(a, b Voice) int {
		return cmp.Compare(a.Name, b.Name)
	})
}
