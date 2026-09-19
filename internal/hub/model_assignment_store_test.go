package hub

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/offering"
)

// verifiedModel registers a pin whose digest is filled, so the factory can
// point at it. The purpose and digest are test-controlled.
func verifiedModel(t *testing.T, s *Store, id, digest, purpose string) *Model {
	t.Helper()
	m, err := s.CreateModel(id, "Org", "Org/"+id+"@rev", "", digest, "MIT", purpose)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSetAssignmentRoundTrip(t *testing.T) {
	s := testStore(t)
	m := verifiedModel(t, s, "verified-worker", "worker-digest", "worker")

	a, err := s.SetAssignment(" Worker ", m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if a.Purpose != "worker" || a.ModelID != m.ID {
		t.Errorf("assignment = %+v, want canonical purpose and the pinned id", a)
	}

	got, err := s.AssignmentFor("worker")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Purpose != "worker" || got.ModelID != m.ID {
		t.Errorf("AssignmentFor = %+v, want the written row", got)
	}

	list, err := s.ListAssignments()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ModelID != m.ID {
		t.Errorf("ListAssignments = %+v, want the single row", list)
	}

	// Re-pinning the same purpose replaces the row: still one row per
	// purpose.
	second := verifiedModel(t, s, "verified-worker-2", "other-digest", "worker")
	if _, err := s.SetAssignment("worker", second.ID); err != nil {
		t.Fatal(err)
	}
	list, err = s.ListAssignments()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ModelID != second.ID {
		t.Errorf("ListAssignments after re-pin = %+v, want one row on the second model", list)
	}
}

func TestSetAssignmentCrossValidation(t *testing.T) {
	s := testStore(t)
	worker := verifiedModel(t, s, "mismatch-worker", "worker-digest", "worker")
	unverified := verifiedModel(t, s, "unverified-persona", "", "persona")

	tests := []struct {
		name    string
		purpose string
		modelID string
		wantErr error
	}{
		{"unknown purpose", "chat", "some-model", nil},
		{"unknown model", "persona", "no-such-model", ErrUnknownModel},
		{"purpose mismatch", "persona", worker.ID, ErrModelPurposeMismatch},
		{"empty digest", "persona", unverified.ID, ErrModelUnverified},
		{"empty purpose", "", "some-model", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.SetAssignment(tt.purpose, tt.modelID)
			if err == nil {
				t.Fatalf("SetAssignment(%q, %q) succeeded, want an error", tt.purpose, tt.modelID)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}

	list, err := s.ListAssignments()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("ListAssignments after refused writes = %+v, want empty", list)
	}
}

func TestSetAssignmentRefusesSeededUnverifiedModels(t *testing.T) {
	s := testStore(t)
	// Every factory seed pin carries an empty digest, so a fresh hub cannot
	// assign anything until an admin verifies an artifact and fills the pin.
	_, err := s.SetAssignment("persona", "qwen3.5-4b-q4_k_m")
	if !errors.Is(err, ErrModelUnverified) {
		t.Fatalf("err = %v, want ErrModelUnverified", err)
	}
	list, err := s.ListAssignments()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("fresh hub has %d assignments, want 0", len(list))
	}
}

func TestClearAssignment(t *testing.T) {
	s := testStore(t)
	m := verifiedModel(t, s, "clearable-stt", "stt-digest", "stt")
	if _, err := s.SetAssignment("stt", m.ID); err != nil {
		t.Fatal(err)
	}

	cleared, err := s.ClearAssignment("stt")
	if err != nil || !cleared {
		t.Fatalf("ClearAssignment = (%v, %v), want (true, nil)", cleared, err)
	}
	got, err := s.AssignmentFor("stt")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("AssignmentFor after clear = %+v, want nil", got)
	}
	list, err := s.ListAssignments()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("ListAssignments after clear = %+v, want empty", list)
	}

	// Clearing a purpose with no assignment is (false, nil).
	cleared, err = s.ClearAssignment("stt")
	if err != nil || cleared {
		t.Fatalf("second ClearAssignment = (%v, %v), want (false, nil)", cleared, err)
	}
}

func TestListAssignmentsEmpty(t *testing.T) {
	s := testStore(t)
	list, err := s.ListAssignments()
	if err != nil {
		t.Fatal(err)
	}
	if list == nil || len(list) != 0 {
		t.Fatalf("ListAssignments on a fresh store = %#v, want an empty non-nil slice", list)
	}
}

func TestListAssignmentsOrderedByPurpose(t *testing.T) {
	s := testStore(t)
	for _, p := range []struct{ purpose, id string }{
		{"worker", "ordered-worker"},
		{"stt", "ordered-stt"},
		{"persona", "ordered-persona"},
	} {
		verifiedModel(t, s, p.id, p.id+"-digest", p.purpose)
		if _, err := s.SetAssignment(p.purpose, p.id); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.ListAssignments()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"persona", "stt", "worker"}
	if len(list) != len(want) {
		t.Fatalf("ListAssignments has %d rows, want %d", len(list), len(want))
	}
	for i, a := range list {
		if a.Purpose != want[i] {
			t.Errorf("row %d purpose = %q, want %q", i, a.Purpose, want[i])
		}
	}
}

func TestAdminAssignmentEndpoints(t *testing.T) {
	f := claimedHub(t, offering.Hosted)

	// A fresh hub has no assignments.
	resp := f.do(t, http.MethodGet, "/api/admin/models/assignments", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET fresh: %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "[]\n" {
		t.Errorf("fresh list body = %q, want an empty array", body)
	}

	// The seeded persona pin is unverified: 400.
	resp = f.do(t, http.MethodPut, "/api/admin/models/assignments", map[string]string{
		"purpose": "persona", "modelId": "qwen3.5-4b-q4_k_m",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("PUT unverified seed: %d, want 400", resp.StatusCode)
	}

	// A verified pin assigns: 200 with the lowercase wire shape.
	verifiedModel(t, f.srv.store, "verified-worker", "worker-digest", "worker")
	resp = f.do(t, http.MethodPut, "/api/admin/models/assignments", map[string]string{
		"purpose": " Worker ", "modelId": "verified-worker",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT: %d, want 200", resp.StatusCode)
	}
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{`"Purpose"`, `"ModelID"`, `"model_id"`} {
		if strings.Contains(string(body), banned) {
			t.Errorf("assignment payload leaks a Go or column name: %s", banned)
		}
	}
	var a ModelAssignment
	if err := json.Unmarshal(body, &a); err != nil {
		t.Fatal(err)
	}
	if a.Purpose != "worker" || a.ModelID != "verified-worker" {
		t.Errorf("assignment = %+v, want canonical worker → verified-worker", a)
	}

	// Refusals: unknown model 404, purpose mismatch 400, unknown purpose 400.
	resp = f.do(t, http.MethodPut, "/api/admin/models/assignments", map[string]string{
		"purpose": "worker", "modelId": "no-such-model",
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("PUT unknown model: %d, want 404", resp.StatusCode)
	}
	resp = f.do(t, http.MethodPut, "/api/admin/models/assignments", map[string]string{
		"purpose": "persona", "modelId": "verified-worker",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("PUT purpose mismatch: %d, want 400", resp.StatusCode)
	}
	resp = f.do(t, http.MethodPut, "/api/admin/models/assignments", map[string]string{
		"purpose": "chat", "modelId": "verified-worker",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("PUT unknown purpose: %d, want 400", resp.StatusCode)
	}

	// List shows the one row.
	resp = f.do(t, http.MethodGet, "/api/admin/models/assignments", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET: %d, want 200", resp.StatusCode)
	}
	var list []ModelAssignment
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ModelID != "verified-worker" {
		t.Errorf("list = %+v, want the one worker row", list)
	}

	// Clear: 200, then gone; clearing again is a 404.
	resp = f.do(t, http.MethodDelete, "/api/admin/models/assignments/worker", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE: %d, want 200", resp.StatusCode)
	}
	resp = f.do(t, http.MethodGet, "/api/admin/models/assignments", nil)
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "[]\n" {
		t.Errorf("list after clear = %q, want an empty array", body)
	}
	resp = f.do(t, http.MethodDelete, "/api/admin/models/assignments/worker", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("DELETE again: %d, want 404", resp.StatusCode)
	}
}

func TestAdminAssignmentRoutesRefuseNonAdmin(t *testing.T) {
	f := hostedCustomer(t)
	cases := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, "/api/admin/models/assignments", nil},
		{http.MethodPut, "/api/admin/models/assignments", map[string]string{"purpose": "worker", "modelId": "x"}},
		{http.MethodDelete, "/api/admin/models/assignments/worker", nil},
	}
	for _, c := range cases {
		resp := f.do(t, c.method, c.path, c.body)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s: %d, want 403", c.method, c.path, resp.StatusCode)
		}
	}
}

func TestAdminAssignmentRoutesRequireAuth(t *testing.T) {
	srv := newHub(t, t.TempDir(), offering.Selfhost)
	ts := httptest.NewServer(srv.mux)
	t.Cleanup(ts.Close)

	for _, c := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/admin/models/assignments"},
		{http.MethodDelete, "/api/admin/models/assignments/worker"},
	} {
		resp := requestJSON(t, ts, &http.Client{}, c.method, c.path, nil)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s bare: %d, want 401", c.method, c.path, resp.StatusCode)
		}
	}
}
