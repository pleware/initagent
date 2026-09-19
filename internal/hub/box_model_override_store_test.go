package hub

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pleware/initagent/internal/offering"
)

// testBox mints a box whose overrides a test can write.
func testBox(t *testing.T, s *Store, slug string) *Box {
	t.Helper()
	box, err := s.CreateBox(slug, "Box "+slug, "", "")
	if err != nil {
		t.Fatal(err)
	}
	return box
}

func TestSetBoxModelOverrideRoundTrip(t *testing.T) {
	s := testStore(t)
	box := testBox(t, s, "override-box")
	m := verifiedModel(t, s, "override-worker", "worker-digest", "worker")

	o, err := s.SetBoxModelOverride(box.ID, " Worker ", m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if o.BoxID != box.ID || o.Purpose != "worker" || o.ModelID != m.ID {
		t.Errorf("override = %+v, want the box, canonical purpose and pinned id", o)
	}

	list, err := s.ListBoxModelOverrides(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ModelID != m.ID {
		t.Errorf("ListBoxModelOverrides = %+v, want the single row", list)
	}

	// Re-pinning the same box and purpose replaces the row.
	second := verifiedModel(t, s, "override-worker-2", "other-digest", "worker")
	if _, err := s.SetBoxModelOverride(box.ID, "worker", second.ID); err != nil {
		t.Fatal(err)
	}
	list, err = s.ListBoxModelOverrides(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ModelID != second.ID {
		t.Errorf("ListBoxModelOverrides after re-pin = %+v, want one row on the second model", list)
	}

	// Overrides are per-box: another box stays clean.
	other := testBox(t, s, "other-box")
	list, err = s.ListBoxModelOverrides(other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("unrelated box has %d overrides, want 0", len(list))
	}
}

func TestSetBoxModelOverrideCrossValidation(t *testing.T) {
	s := testStore(t)
	box := testBox(t, s, "validation-box")
	worker := verifiedModel(t, s, "validation-worker", "worker-digest", "worker")
	unverified := verifiedModel(t, s, "validation-unverified", "", "persona")

	tests := []struct {
		name    string
		boxID   string
		purpose string
		modelID string
		wantErr error
	}{
		{"unknown box", "no-such-box", "worker", worker.ID, ErrUnknownBox},
		{"unknown model", box.ID, "worker", "no-such-model", ErrUnknownModel},
		{"purpose mismatch", box.ID, "persona", worker.ID, ErrModelPurposeMismatch},
		{"empty digest", box.ID, "persona", unverified.ID, ErrModelUnverified},
		{"unknown purpose", box.ID, "chat", worker.ID, nil},
		{"empty purpose", box.ID, "", worker.ID, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.SetBoxModelOverride(tt.boxID, tt.purpose, tt.modelID)
			if err == nil {
				t.Fatalf("SetBoxModelOverride(%q, %q, %q) succeeded, want an error", tt.boxID, tt.purpose, tt.modelID)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}

	list, err := s.ListBoxModelOverrides(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("ListBoxModelOverrides after refused writes = %+v, want empty", list)
	}
}

func TestSetBoxModelOverrideRefusesSeededUnverifiedModels(t *testing.T) {
	s := testStore(t)
	box := testBox(t, s, "seed-refusal-box")
	_, err := s.SetBoxModelOverride(box.ID, "persona", "qwen3.5-4b-q4_k_m")
	if !errors.Is(err, ErrModelUnverified) {
		t.Fatalf("err = %v, want ErrModelUnverified", err)
	}
}

func TestListBoxModelOverridesEmpty(t *testing.T) {
	s := testStore(t)
	box := testBox(t, s, "empty-overrides-box")
	list, err := s.ListBoxModelOverrides(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if list == nil || len(list) != 0 {
		t.Fatalf("ListBoxModelOverrides on a clean box = %#v, want an empty non-nil slice", list)
	}
}

func TestListBoxModelOverridesOrderedByPurpose(t *testing.T) {
	s := testStore(t)
	box := testBox(t, s, "ordered-overrides-box")
	for _, p := range []struct{ purpose, id string }{
		{"worker", "ordered-ov-worker"},
		{"stt", "ordered-ov-stt"},
		{"persona", "ordered-ov-persona"},
	} {
		verifiedModel(t, s, p.id, p.id+"-digest", p.purpose)
		if _, err := s.SetBoxModelOverride(box.ID, p.purpose, p.id); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.ListBoxModelOverrides(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"persona", "stt", "worker"}
	if len(list) != len(want) {
		t.Fatalf("ListBoxModelOverrides has %d rows, want %d", len(list), len(want))
	}
	for i, o := range list {
		if o.Purpose != want[i] {
			t.Errorf("row %d purpose = %q, want %q", i, o.Purpose, want[i])
		}
	}
}

func TestClearBoxModelOverride(t *testing.T) {
	s := testStore(t)
	box := testBox(t, s, "clear-override-box")
	m := verifiedModel(t, s, "clear-override-stt", "stt-digest", "stt")
	if _, err := s.SetBoxModelOverride(box.ID, "stt", m.ID); err != nil {
		t.Fatal(err)
	}

	cleared, err := s.ClearBoxModelOverride(box.ID, "stt")
	if err != nil || !cleared {
		t.Fatalf("ClearBoxModelOverride = (%v, %v), want (true, nil)", cleared, err)
	}
	list, err := s.ListBoxModelOverrides(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("ListBoxModelOverrides after clear = %+v, want empty", list)
	}

	// Clearing an override that does not exist is (false, nil).
	cleared, err = s.ClearBoxModelOverride(box.ID, "stt")
	if err != nil || cleared {
		t.Fatalf("second ClearBoxModelOverride = (%v, %v), want (false, nil)", cleared, err)
	}
}

func TestResolvedModelsOverrideWinsOverAssignment(t *testing.T) {
	s := testStore(t)
	box := testBox(t, s, "resolution-box")
	canonical := verifiedModel(t, s, "canonical-worker", "canonical-digest", "worker")
	override := verifiedModel(t, s, "override-worker", "override-digest", "worker")
	if _, err := s.SetAssignment("worker", canonical.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetBoxModelOverride(box.ID, "worker", override.ID); err != nil {
		t.Fatal(err)
	}

	roster, err := s.ResolvedModels(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(roster) != 1 {
		t.Fatalf("roster = %+v, want the one resolved purpose", roster)
	}
	if roster["worker"].ID != override.ID {
		t.Errorf("roster[worker] = %q, want the override %q", roster["worker"].ID, override.ID)
	}

	// A box without an override falls back to the canonical assignment.
	clean := testBox(t, s, "clean-resolution-box")
	roster, err = s.ResolvedModels(clean.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(roster) != 1 || roster["worker"].ID != canonical.ID {
		t.Errorf("roster on the clean box = %+v, want the assignment %q", roster, canonical.ID)
	}
}

func TestResolvedModelsOmitsUnresolvedPurposes(t *testing.T) {
	s := testStore(t)
	box := testBox(t, s, "omission-box")
	persona := verifiedModel(t, s, "omission-persona", "persona-digest", "persona")
	if _, err := s.SetBoxModelOverride(box.ID, "persona", persona.ID); err != nil {
		t.Fatal(err)
	}

	roster, err := s.ResolvedModels(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(roster) != 1 {
		t.Fatalf("roster = %+v, want only persona", roster)
	}
	for _, p := range []string{"worker", "embedding", "stt"} {
		if _, ok := roster[p]; ok {
			t.Errorf("roster contains unresolved purpose %q: %+v", p, roster)
		}
	}

	// A box with nothing at all resolves to an empty roster, not an error.
	empty := testBox(t, s, "empty-resolution-box")
	roster, err = s.ResolvedModels(empty.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(roster) != 0 {
		t.Errorf("roster on a box with no pins = %+v, want empty", roster)
	}
}

func TestResolvedModelsRejectsDanglingReference(t *testing.T) {
	s := testStore(t)
	box := testBox(t, s, "dangling-box")
	if _, err := s.db.Exec(`INSERT INTO box_model_overrides (box_id, purpose, model_id)
		VALUES (?, 'worker', 'ghost-model')`, box.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolvedModels(box.ID); err == nil {
		t.Fatal("ResolvedModels accepted a reference to a model outside the registry")
	}
}

func TestDeleteBoxClearsModelOverrides(t *testing.T) {
	s := testStore(t)
	box := testBox(t, s, "cascade-box")
	m := verifiedModel(t, s, "cascade-worker", "cascade-digest", "worker")
	if _, err := s.SetBoxModelOverride(box.ID, "worker", m.ID); err != nil {
		t.Fatal(err)
	}

	deleted, err := s.DeleteBox(box.ID)
	if err != nil || !deleted {
		t.Fatalf("DeleteBox = (%v, %v), want (true, nil)", deleted, err)
	}
	list, err := s.ListBoxModelOverrides(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("ListBoxModelOverrides after DeleteBox = %+v, want no orphans", list)
	}
	// With the override cascade-deleted, the model is no longer in use.
	deletedModel, err := s.DeleteModel(m.ID)
	if err != nil || !deletedModel {
		t.Fatalf("DeleteModel after DeleteBox = (%v, %v), want (true, nil)", deletedModel, err)
	}
}

func TestModelPurposeOrderMatchesSet(t *testing.T) {
	if len(modelPurposeOrder) != len(modelPurposes) {
		t.Fatalf("modelPurposeOrder has %d entries, modelPurposes has %d", len(modelPurposeOrder), len(modelPurposes))
	}
	for _, p := range modelPurposeOrder {
		if !modelPurposes[p] {
			t.Errorf("modelPurposeOrder names %q, which modelPurposes does not carry", p)
		}
	}
}

func TestBoxModelEndpoints(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	box := testBox(t, f.srv.store, "endpoint-box")
	canonical := verifiedModel(t, f.srv.store, "endpoint-canonical", "canonical-digest", "worker")
	override := verifiedModel(t, f.srv.store, "endpoint-override", "override-digest", "worker")
	if _, err := f.srv.store.SetAssignment("worker", canonical.ID); err != nil {
		t.Fatal(err)
	}

	// Roster with no override serves the canonical assignment.
	resp := f.do(t, http.MethodGet, "/api/boxes/"+box.ID+"/models", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET: %d, want 200", resp.StatusCode)
	}
	var roster map[string]Model
	if err := json.NewDecoder(resp.Body).Decode(&roster); err != nil {
		t.Fatal(err)
	}
	if roster["worker"].ID != canonical.ID {
		t.Errorf("roster = %+v, want the canonical worker", roster)
	}

	// Set an override: the roster flips to it.
	resp = f.do(t, http.MethodPut, "/api/boxes/"+box.ID+"/models", map[string]string{
		"purpose": "worker", "modelId": override.ID,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT: %d, want 200", resp.StatusCode)
	}
	resp = f.do(t, http.MethodGet, "/api/boxes/"+box.ID+"/models", nil)
	if err := json.NewDecoder(resp.Body).Decode(&roster); err != nil {
		t.Fatal(err)
	}
	if roster["worker"].ID != override.ID {
		t.Errorf("roster after PUT = %+v, want the override", roster)
	}

	// Refusals: unknown model 404, mismatch 400, bad purpose 400.
	resp = f.do(t, http.MethodPut, "/api/boxes/"+box.ID+"/models", map[string]string{
		"purpose": "worker", "modelId": "no-such-model",
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("PUT unknown model: %d, want 404", resp.StatusCode)
	}
	resp = f.do(t, http.MethodPut, "/api/boxes/"+box.ID+"/models", map[string]string{
		"purpose": "persona", "modelId": override.ID,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("PUT purpose mismatch: %d, want 400", resp.StatusCode)
	}
	resp = f.do(t, http.MethodPut, "/api/boxes/"+box.ID+"/models", map[string]string{
		"purpose": "chat", "modelId": override.ID,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("PUT bad purpose: %d, want 400", resp.StatusCode)
	}

	// Unknown box answers 404 on every verb.
	resp = f.do(t, http.MethodGet, "/api/boxes/no-such-box/models", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET unknown box: %d, want 404", resp.StatusCode)
	}
	resp = f.do(t, http.MethodPut, "/api/boxes/no-such-box/models", map[string]string{
		"purpose": "worker", "modelId": override.ID,
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("PUT unknown box: %d, want 404", resp.StatusCode)
	}

	// Clear: 200 and back to canonical; clearing again is a 404.
	resp = f.do(t, http.MethodDelete, "/api/boxes/"+box.ID+"/models/worker", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE: %d, want 200", resp.StatusCode)
	}
	resp = f.do(t, http.MethodGet, "/api/boxes/"+box.ID+"/models", nil)
	if err := json.NewDecoder(resp.Body).Decode(&roster); err != nil {
		t.Fatal(err)
	}
	if roster["worker"].ID != canonical.ID {
		t.Errorf("roster after DELETE = %+v, want back to canonical", roster)
	}
	resp = f.do(t, http.MethodDelete, "/api/boxes/"+box.ID+"/models/worker", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("DELETE again: %d, want 404", resp.StatusCode)
	}
}

func TestBoxModelRoutesRefuseNonAdmin(t *testing.T) {
	f := hostedCustomer(t)
	box := testBox(t, f.srv.store, "forbidden-box")
	cases := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, "/api/boxes/" + box.ID + "/models", nil},
		{http.MethodPut, "/api/boxes/" + box.ID + "/models", map[string]string{"purpose": "worker", "modelId": "x"}},
		{http.MethodDelete, "/api/boxes/" + box.ID + "/models/worker", nil},
	}
	for _, c := range cases {
		resp := f.do(t, c.method, c.path, c.body)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s: %d, want 403", c.method, c.path, resp.StatusCode)
		}
	}
}

func TestBoxModelRoutesRequireAuth(t *testing.T) {
	srv := newHub(t, t.TempDir(), offering.Selfhost)
	ts := httptest.NewServer(srv.mux)
	t.Cleanup(ts.Close)

	resp := requestJSON(t, ts, &http.Client{}, http.MethodGet, "/api/boxes/box-1/models", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("GET /api/boxes/box-1/models bare: %d, want 401", resp.StatusCode)
	}
}
