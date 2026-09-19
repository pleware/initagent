package hub

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/offering"
)

func TestParsePurpose(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		err  bool
	}{
		{"persona", "persona", "persona", false},
		{"worker", "worker", "worker", false},
		{"embedding", "embedding", "embedding", false},
		{"stt", "stt", "stt", false},
		{"trimmed and folded", "  Worker  ", "worker", false},
		{"unknown", "chat", "", true},
		{"empty", "", "", true},
		{"whitespace only", "   ", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePurpose(tt.in)
			if tt.err {
				if err == nil {
					t.Fatalf("ParsePurpose(%q) = %q, want an error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePurpose(%q) error = %v, want nil", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParsePurpose(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseQuant(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		err  bool
	}{
		// canonical passthrough: floats, legacy, K-quants, I-quants.
		{"float f32", "F32", "F32", false},
		{"float f16", "F16", "F16", false},
		{"float bf16", "BF16", "BF16", false},
		{"legacy q8_0", "Q8_0", "Q8_0", false},
		{"legacy q8_1", "Q8_1", "Q8_1", false},
		{"k-quant q6_k", "Q6_K", "Q6_K", false},
		{"k-quant q3_k_l", "Q3_K_L", "Q3_K_L", false},
		{"i-quant iq2_xxs", "IQ2_XXS", "IQ2_XXS", false},
		{"i-quant iq4_nl", "IQ4_NL", "IQ4_NL", false},
		// case-insensitive normalization to uppercase canonical.
		{"normalize lowercase", "q4_k_m", "Q4_K_M", false},
		{"normalize mixed", "Iq3_S", "IQ3_S", false},
		{"normalize with whitespace", "  q5_0  ", "Q5_0", false},
		// empty is allowed: non-GGUF pins (embedding, stt) carry no quant.
		{"empty", "", "", false},
		{"whitespace only", "   ", "", false},
		// outside the dictionary: refused, never defaulted.
		{"unknown", "bogus", "", true},
		{"near-miss k-quant", "Q4_K", "", true},
		{"near-miss i-quant", "IQ2_X", "", true},
		{"lowercase unknown", "q4_k_m_x", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseQuant(tt.in)
			if tt.err {
				if err == nil {
					t.Fatalf("ParseQuant(%q) = %q, want an error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseQuant(%q) error = %v, want nil", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParseQuant(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCreateModelRejectsUnknownQuant(t *testing.T) {
	s := testStore(t)
	before, err := s.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateModel("bad-quant", "SomeOrg", "s", "bogus", "", "MIT", "persona"); err == nil {
		t.Fatal("CreateModel accepted a non-canonical quant")
	}
	after, err := s.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Errorf("ListModels after refused create has %d rows, want %d", len(after), len(before))
	}
}

func TestUpdateModelRejectsUnknownQuant(t *testing.T) {
	s := testStore(t)
	created, err := s.CreateModel("quant-guard", "SomeOrg", "s", "Q4_0", "", "MIT", "persona")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateModel(created.ID, "SomeOrg", "s2", "bogus", "", "MIT", "persona"); err == nil {
		t.Fatal("UpdateModel accepted a non-canonical quant")
	}
	got, err := s.GetModel(created.ID)
	if err != nil || got == nil {
		t.Fatalf("GetModel after refused update = (%v, %v), want the untouched pin", got, err)
	}
	if got.Quant != "Q4_0" || got.Source != "s" {
		t.Errorf("refused update left %+v, want the original row", got)
	}
}

func TestModelCRUDRoundTrip(t *testing.T) {
	s := testStore(t)
	created, err := s.CreateModel("test-model-7b-q4_0", "SomeOrg", "SomeOrg/test-model-7B-GGUF@rev", "q4_0", "", "Apache-2.0", "persona")
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "test-model-7b-q4_0" || created.Org != "SomeOrg" || created.Quant != "Q4_0" ||
		created.Purpose != "persona" || created.Digest != "" {
		t.Errorf("created = %+v, want id, org, canonical q4_0 quant, persona purpose and empty digest", created)
	}

	got, err := s.GetModel(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("GetModel returned nil for a created pin")
	}
	if got.Org != "SomeOrg" || got.Source != "SomeOrg/test-model-7B-GGUF@rev" || got.Quant != "Q4_0" || got.Licence != "Apache-2.0" {
		t.Errorf("read-back = %+v, want the created fields", got)
	}

	updated, err := s.UpdateModel(created.ID, "RenamedOrg", "other-source", "q5_k_m", "blake3-of-the-artifact", "MIT", "Worker")
	if err != nil {
		t.Fatal(err)
	}
	if updated == nil {
		t.Fatal("UpdateModel returned nil for an existing pin")
	}
	if updated.ID != created.ID || updated.Org != "RenamedOrg" || updated.Source != "other-source" || updated.Quant != "Q5_K_M" ||
		updated.Digest != "blake3-of-the-artifact" || updated.Licence != "MIT" || updated.Purpose != "worker" {
		t.Errorf("updated = %+v, want the replaced fields with the canonical worker purpose", updated)
	}

	list, err := s.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range list {
		if m.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("ListModels = %+v, want the updated pin", list)
	}
}

func TestGetModelMissing(t *testing.T) {
	s := testStore(t)
	got, err := s.GetModel("no-such-model")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("GetModel = %+v, want nil", got)
	}
}

func TestUpdateModelMissing(t *testing.T) {
	s := testStore(t)
	got, err := s.UpdateModel("no-such-model", "o", "s", "", "", "MIT", "persona")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("UpdateModel = %+v, want nil", got)
	}
}

func TestCreateModelDuplicateID(t *testing.T) {
	s := testStore(t)
	if _, err := s.CreateModel("dup-model", "o", "s", "", "", "MIT", "persona"); err != nil {
		t.Fatal(err)
	}
	_, err := s.CreateModel("dup-model", "o2", "second", "", "", "MIT", "worker")
	if !errors.Is(err, ErrModelIDTaken) {
		t.Fatalf("err = %v, want ErrModelIDTaken", err)
	}
}

func TestCreateModelRejectsUnknownPurpose(t *testing.T) {
	s := testStore(t)
	before, err := s.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateModel("bad-purpose", "o", "s", "", "", "MIT", "chat"); err == nil {
		t.Fatal("CreateModel accepted an unknown purpose")
	}
	after, err := s.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Errorf("ListModels after refused create has %d rows, want %d", len(after), len(before))
	}
}

func TestDeleteModel(t *testing.T) {
	s := testStore(t)
	created, err := s.CreateModel("doomed-model", "o", "s", "", "", "MIT", "stt")
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := s.DeleteModel(created.ID)
	if err != nil || !deleted {
		t.Fatalf("DeleteModel = (%v, %v), want (true, nil)", deleted, err)
	}
	got, err := s.GetModel(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("GetModel after delete = %+v, want nil", got)
	}
	// Deleting a missing pin is not an error.
	deleted, err = s.DeleteModel(created.ID)
	if err != nil || deleted {
		t.Fatalf("second DeleteModel = (%v, %v), want (false, nil)", deleted, err)
	}
}

func TestDeleteModelInUse(t *testing.T) {
	s := testStore(t)
	// SetAssignment and SetBoxModelOverride both refuse an empty digest, so
	// the pin carries one.
	created, err := s.CreateModel("pinned-model", "o", "s", "", "pinned-digest", "MIT", "persona")
	if err != nil {
		t.Fatal(err)
	}
	box, err := s.CreateBox("pin-box", "Pin", "", "")
	if err != nil {
		t.Fatal(err)
	}

	// Referenced by an assignment: refused.
	if _, err := s.SetAssignment("persona", created.ID); err != nil {
		t.Fatal(err)
	}
	deleted, err := s.DeleteModel(created.ID)
	if !errors.Is(err, ErrModelInUse) || deleted {
		t.Fatalf("DeleteModel on an assigned pin = (%v, %v), want (ErrModelInUse, false)", deleted, err)
	}

	// Referenced by an override instead: refused the same way.
	if _, err := s.ClearAssignment("persona"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetBoxModelOverride(box.ID, "persona", created.ID); err != nil {
		t.Fatal(err)
	}
	deleted, err = s.DeleteModel(created.ID)
	if !errors.Is(err, ErrModelInUse) || deleted {
		t.Fatalf("DeleteModel on an overridden pin = (%v, %v), want (ErrModelInUse, false)", deleted, err)
	}

	// Unreferenced: deletes.
	if _, err := s.ClearBoxModelOverride(box.ID, "persona"); err != nil {
		t.Fatal(err)
	}
	deleted, err = s.DeleteModel(created.ID)
	if err != nil || !deleted {
		t.Fatalf("DeleteModel on a free pin = (%v, %v), want (true, nil)", deleted, err)
	}
}

func TestEnsureSeedModelsSeedsFactoryPins(t *testing.T) {
	s := testStore(t) // openStore already ran the seed
	list, err := s.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 4 {
		t.Fatalf("ListModels = %+v, want the four factory pins", list)
	}
	byID := map[string]Model{}
	for _, m := range list {
		byID[m.ID] = m
	}
	tests := []struct {
		id           string
		org          string
		sourcePrefix string
		quant        string
		licence      string
		purpose      string
	}{
		{"qwen3.5-4b-q4_k_m", "unsloth", "unsloth/Qwen3.5-4B-GGUF@", "Q4_K_M", "Apache-2.0", "persona"},
		{"qwen2.5-coder-7b-q4_k_m", "Qwen", "Qwen/Qwen2.5-Coder-7B-Instruct-GGUF@", "Q4_K_M", "Apache-2.0", "worker"},
		{"bge-m3", "BAAI", "BAAI/bge-m3@", "", "MIT", "embedding"},
		{"whisper-large-v3", "openai", "openai/whisper-large-v3@", "", "MIT", "stt"},
	}
	for _, tt := range tests {
		m, ok := byID[tt.id]
		if !ok {
			t.Errorf("seed missing pin %s", tt.id)
			continue
		}
		if m.Org != tt.org || m.Quant != tt.quant || m.Licence != tt.licence || m.Purpose != tt.purpose {
			t.Errorf("%s = org %q quant %q licence %q purpose %q, want %q/%q/%q/%q",
				tt.id, m.Org, m.Quant, m.Licence, m.Purpose, tt.org, tt.quant, tt.licence, tt.purpose)
		}
		if !strings.HasPrefix(m.Source, tt.sourcePrefix) || !strings.Contains(m.Source, "@") {
			t.Errorf("%s source = %q, want a pinned repo@rev", tt.id, m.Source)
		}
		if m.Digest != "" {
			t.Errorf("%s digest = %q, want empty (no fake digest)", tt.id, m.Digest)
		}
	}
}

func TestEnsureSeedModelsKeepsAdminEdits(t *testing.T) {
	s := testStore(t)
	// The admin verifies the persona artifact and fills its digest.
	if _, err := s.UpdateModel("qwen3.5-4b-q4_k_m", "custom-org", "custom-source", "q4_k_m", "admin-computed-digest", "Custom", "persona"); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureSeedModels(); err != nil {
		t.Fatal(err)
	}
	m, err := s.GetModel("qwen3.5-4b-q4_k_m")
	if err != nil || m == nil {
		t.Fatalf("GetModel after reseed = (%v, %v), want the admin's row", m, err)
	}
	if m.Source != "custom-source" || m.Digest != "admin-computed-digest" || m.Licence != "Custom" {
		t.Errorf("reseed overwrote the admin's edit: %+v", m)
	}
	list, err := s.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 4 {
		t.Errorf("ListModels after reseed has %d rows, want 4 (nothing inserted)", len(list))
	}
}

func TestEnsureSeedModelsBumpsBoxesOnce(t *testing.T) {
	s := testStore(t)
	// Simulate a live store that predates the seed: the factory rows are
	// gone and boxes exist.
	if _, err := s.db.Exec(`DELETE FROM models`); err != nil {
		t.Fatal(err)
	}
	box, err := s.CreateBox("seed-box", "Seed", "", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := s.EnsureSeedModels(); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetBox(box.ID)
	if err != nil || got == nil {
		t.Fatalf("GetBox = (%v, %v), want the box", got, err)
	}
	if got.ConfigVersion != 2 {
		t.Errorf("config_version = %d, want 2 (one seed bump)", got.ConfigVersion)
	}

	// A second run inserts nothing and bumps nothing.
	if err := s.EnsureSeedModels(); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetBox(box.ID)
	if err != nil || got == nil {
		t.Fatalf("GetBox after second run = (%v, %v)", got, err)
	}
	if got.ConfigVersion != 2 {
		t.Errorf("config_version after second run = %d, want 2 (no re-bump)", got.ConfigVersion)
	}
	list, err := s.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 4 {
		t.Errorf("ListModels after reseed has %d rows, want 4", len(list))
	}
}

func TestOpenStoreSeedsModelsOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seed-models.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 4 {
		t.Errorf("fresh open has %d pins, want the four seeds", len(list))
	}
	box, err := s.CreateBox("reopen-box", "Reopen", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	again, err := OpenStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { again.Close() })
	// The pins already existed, so the reopen inserted nothing and bumped
	// nothing: the box keeps config_version 1.
	got, err := again.GetBox(box.ID)
	if err != nil || got == nil {
		t.Fatalf("GetBox after reopen = (%v, %v)", got, err)
	}
	if got.ConfigVersion != 1 {
		t.Errorf("config_version after reopen = %d, want 1 (seed must not re-bump)", got.ConfigVersion)
	}
	list, err = again.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 4 {
		t.Errorf("reopen has %d pins, want 4", len(list))
	}
}

func TestListModelsPublic(t *testing.T) {
	srv := newHub(t, t.TempDir(), offering.Selfhost)
	ts := httptest.NewServer(srv.mux)
	t.Cleanup(ts.Close)

	resp := requestJSON(t, ts, &http.Client{}, http.MethodGet, "/api/models", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/models: %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{`"Id"`, `"Org"`, `"Source"`, `"Quant"`, `"Licence"`} {
		if strings.Contains(string(body), banned) {
			t.Errorf("public payload leaks a Go field name: %s", banned)
		}
	}
	var got []Model
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("public catalog has %d pins, want the four seeds", len(got))
	}
	for _, m := range got {
		if m.ID == "" || m.Org == "" || m.Source == "" || m.Licence == "" || m.Purpose == "" {
			t.Errorf("public pin with empty fields: %+v", m)
		}
	}
}

func TestAdminModelEndpoints(t *testing.T) {
	f := claimedHub(t, offering.Hosted)

	// Create.
	resp := f.do(t, http.MethodPost, "/api/admin/models", map[string]string{
		"id": "admin-model", "org": "AdminOrg", "source": "Org/repo@rev", "quant": "q4_0",
		"digest": "", "licence": "MIT", "purpose": "worker",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d, want 201", resp.StatusCode)
	}
	var created Model
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ID != "admin-model" || created.Org != "AdminOrg" || created.Quant != "Q4_0" || created.Purpose != "worker" {
		t.Errorf("created = %+v, want the submitted pin with canonical q4_0 quant", created)
	}

	// Duplicate id.
	resp = f.do(t, http.MethodPost, "/api/admin/models", map[string]string{"id": "admin-model", "purpose": "worker"})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate create: %d, want 409", resp.StatusCode)
	}

	// Bad purpose.
	resp = f.do(t, http.MethodPost, "/api/admin/models", map[string]string{"id": "bad-purpose", "purpose": "chat"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad purpose: %d, want 400", resp.StatusCode)
	}

	// Bad quant: outside the canonical GGUF dictionary.
	resp = f.do(t, http.MethodPost, "/api/admin/models", map[string]string{"id": "bad-quant", "quant": "bogus", "purpose": "worker"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad quant: %d, want 400", resp.StatusCode)
	}

	// Update.
	resp = f.do(t, http.MethodPatch, "/api/admin/models/admin-model", map[string]string{
		"org": "RenamedOrg", "source": "new-source", "quant": "", "digest": "verified-digest", "licence": "MIT", "purpose": "stt",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: %d, want 200", resp.StatusCode)
	}
	var updated Model
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.Org != "RenamedOrg" || updated.Source != "new-source" || updated.Digest != "verified-digest" || updated.Purpose != "stt" {
		t.Errorf("updated = %+v, want the replaced fields", updated)
	}

	// Update missing.
	resp = f.do(t, http.MethodPatch, "/api/admin/models/no-such-model", map[string]string{"purpose": "worker"})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("update missing: %d, want 404", resp.StatusCode)
	}

	// List includes the new pin (four seeds + one).
	resp = f.do(t, http.MethodGet, "/api/admin/models", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: %d, want 200", resp.StatusCode)
	}
	var list []Model
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 5 {
		t.Errorf("list has %d pins, want 5 (seeds + admin-model)", len(list))
	}

	// Delete.
	resp = f.do(t, http.MethodDelete, "/api/admin/models/admin-model", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete: %d, want 200", resp.StatusCode)
	}

	// Delete missing.
	resp = f.do(t, http.MethodDelete, "/api/admin/models/admin-model", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("delete missing: %d, want 404", resp.StatusCode)
	}
}

func TestAdminModelRoutesRefuseNonAdmin(t *testing.T) {
	f := hostedCustomer(t)
	cases := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, "/api/admin/models", nil},
		{http.MethodPost, "/api/admin/models", map[string]string{"id": "x", "purpose": "worker"}},
		{http.MethodPatch, "/api/admin/models/qwen3.5-4b-q4_k_m", map[string]string{"purpose": "worker"}},
		{http.MethodDelete, "/api/admin/models/qwen3.5-4b-q4_k_m", nil},
	}
	for _, c := range cases {
		resp := f.do(t, c.method, c.path, c.body)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s: %d, want 403", c.method, c.path, resp.StatusCode)
		}
	}
}

func TestAdminModelRoutesRequireAuth(t *testing.T) {
	srv := newHub(t, t.TempDir(), offering.Selfhost)
	ts := httptest.NewServer(srv.mux)
	t.Cleanup(ts.Close)

	resp := requestJSON(t, ts, &http.Client{}, http.MethodGet, "/api/admin/models", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("GET /api/admin/models bare: %d, want 401", resp.StatusCode)
	}
}
