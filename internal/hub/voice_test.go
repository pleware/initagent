package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/pleware/initagent/internal/offering"
	"github.com/pleware/initagent/internal/voices"
)

func TestListVoicesPublic(t *testing.T) {
	srv := newHub(t, t.TempDir(), offering.Selfhost)
	ts := httptest.NewServer(srv.mux)
	t.Cleanup(ts.Close)

	resp := requestJSON(t, ts, &http.Client{}, http.MethodGet, "/api/voices", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/voices: %d, want 200", resp.StatusCode)
	}
	var got []voices.Voice
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}

	// The catalog size is locked by internal/voices (the committed
	// voices.json); this proves the endpoint serves all of it, unfiltered.
	if len(got) != 44 {
		t.Fatalf("catalog = %d voices, want 44", len(got))
	}
	for i, v := range got {
		if v.Name == "" || v.Language == "" {
			t.Errorf("voice %d incomplete: %+v", i, v)
		}
		// Quality is Piper vocabulary: a Piper entry carries all three fields,
		// a non-Piper entry carries its engine instead and no quality at all.
		if v.IsPiper() && v.Quality == "" {
			t.Errorf("piper voice %d incomplete: %+v", i, v)
		}
	}

	// Display order on the wire: pl_PL block first, then en_US, then en_GB,
	// name-sorted within each block.
	rank := map[string]int{"pl_PL": 0, "en_US": 1, "en_GB": 2}
	for i := 1; i < len(got); i++ {
		prev, cur := got[i-1], got[i]
		if rank[prev.Language] > rank[cur.Language] {
			t.Errorf("order %d->%d: %q after %q, want pl_PL, en_US, en_GB", i-1, i, cur.Language, prev.Language)
			continue
		}
		if prev.Language == cur.Language && prev.Name > cur.Name {
			t.Errorf("order %d->%d: %q after %q in %s, want name-sorted", i-1, i, cur.Name, prev.Name, cur.Language)
		}
	}

	if got[0] != (voices.Voice{Name: "pl_PL-bass-high", Language: "pl_PL", Quality: "high"}) {
		t.Errorf("first voice = %+v, want pl_PL-bass-high (pl_PL first)", got[0])
	}
	if !slices.Contains(got, voices.Voice{Name: "en_US-lessac-high", Language: "en_US", Quality: "high"}) {
		t.Errorf("catalog missing en_US-lessac-high: %+v", got)
	}
	// The catalog carries a second engine since 2026-09-23, and the endpoint
	// must serve it the same way: the id a form saves, with its engine named.
	if !slices.Contains(got, voices.Voice{Name: "voxcpm2", Language: "pl_PL", Engine: "voxcpm2"}) {
		t.Errorf("catalog missing the voxcpm2 engine voice: %+v", got)
	}
}
