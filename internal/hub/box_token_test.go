package hub

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/id"
	"github.com/pleware/initagent/internal/offering"
)

// mintBoxToken posts a mint request as the fixture's session and returns
// the secret alongside the row, failing the test when the response is not
// a 201.
func mintBoxToken(t *testing.T, f *adminFixture, boxID string) (string, BoxToken) {
	t.Helper()
	resp := f.do(t, http.MethodPost, "/api/boxes/"+boxID+"/tokens", map[string]string{"name": "primary"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/boxes/%s/tokens: %d, want 201", boxID, resp.StatusCode)
	}
	var out struct {
		Token string   `json:"token"`
		Row   BoxToken `json:"row"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Token, out.Row
}

// The operator mints a box's sync credential through the wire, the secret
// comes back exactly once beside its row, and it resolves to the box.
// The resolution stamps last_used_at.
func TestBoxTokenMintAndAuthenticate(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	box, err := f.srv.store.CreateBox("box-tok", "Tokenized", "", "")
	if err != nil {
		t.Fatal(err)
	}

	secret, row := mintBoxToken(t, f, box.ID)
	if secret == "" {
		t.Fatal("mint returned an empty secret")
	}
	if !id.Is(id.Token, row.Id) {
		t.Errorf("minted id %q is not a token identifier", row.Id)
	}
	if row.BoxId != box.ID {
		t.Errorf("row box = %q, want %q", row.BoxId, box.ID)
	}
	if row.Name != "primary" {
		t.Errorf("row name = %q, want primary", row.Name)
	}
	if row.CreatedAt == 0 {
		t.Errorf("row createdAt not set: %+v", row)
	}

	got, ok, err := f.srv.store.BoxTokenAuth(secret)
	if err != nil || !ok {
		t.Fatalf("BoxTokenAuth = (%q, %v, %v), want the box", got, ok, err)
	}
	if got != box.ID {
		t.Errorf("resolved box = %q, want %q", got, box.ID)
	}

	listed, err := f.srv.store.ListBoxTokens(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Id != row.Id {
		t.Fatalf("listed = %+v, want the minted token", listed)
	}
	if listed[0].LastUsedAt == 0 {
		t.Error("auth did not stamp last_used_at")
	}
}

// One active token per box: a second mint revokes the first inside the
// same transaction, so the old secret is a hard miss the moment the new
// one exists, and the list carries only the live row.
func TestBoxTokenSecondMintRevokesFirst(t *testing.T) {
	s := testStore(t)
	box, err := s.CreateBox("box-rot", "Rotated", "", "")
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := s.CreateBoxToken(box.ID, "first")
	if err != nil {
		t.Fatal(err)
	}
	second, row, err := s.CreateBoxToken(box.ID, "second")
	if err != nil {
		t.Fatal(err)
	}
	if got, ok, err := s.BoxTokenAuth(first); err != nil || ok || got != "" {
		t.Errorf("old secret after rotation = (%q, %v, %v), want not found", got, ok, err)
	}
	if got, ok, err := s.BoxTokenAuth(second); err != nil || !ok || got != box.ID {
		t.Errorf("new secret = (%q, %v, %v), want the box", got, ok, err)
	}
	listed, err := s.ListBoxTokens(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Id != row.Id {
		t.Errorf("listed after rotation = %+v, want only the new token", listed)
	}
}

// A presented secret that is unknown is a hard miss, never a flagged
// credential.
func TestBoxTokenAuthUnknownSecret(t *testing.T) {
	s := testStore(t)
	box, err := s.CreateBox("box-auth", "Auth", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateBoxToken(box.ID, "x"); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.BoxTokenAuth("iagt_bogus")
	if err != nil || ok || got != "" {
		t.Errorf("unknown secret = (%q, %v, %v), want not found", got, ok, err)
	}
}

// RevokeBoxToken is scoped to the box and to live rows: a token id that
// belongs to another box is not revoked, and a second revoke finds
// nothing to do.
func TestBoxTokenRevokeScoping(t *testing.T) {
	s := testStore(t)
	boxA, err := s.CreateBox("box-rev-a", "A", "", "")
	if err != nil {
		t.Fatal(err)
	}
	boxB, err := s.CreateBox("box-rev-b", "B", "", "")
	if err != nil {
		t.Fatal(err)
	}
	secret, row, err := s.CreateBoxToken(boxA.ID, "a")
	if err != nil {
		t.Fatal(err)
	}

	// Box B cannot revoke box A's token id.
	revoked, err := s.RevokeBoxToken(row.Id, boxB.ID)
	if err != nil || revoked {
		t.Fatalf("cross-box revoke = (%v, %v), want (false, nil)", revoked, err)
	}
	if got, ok, err := s.BoxTokenAuth(secret); err != nil || !ok || got != boxA.ID {
		t.Fatalf("secret after a cross-box revoke = (%q, %v, %v), want the box", got, ok, err)
	}

	revoked, err = s.RevokeBoxToken(row.Id, boxA.ID)
	if err != nil || !revoked {
		t.Fatalf("own-box revoke = (%v, %v), want (true, nil)", revoked, err)
	}
	if got, ok, err := s.BoxTokenAuth(secret); err != nil || ok || got != "" {
		t.Errorf("secret after revoke = (%q, %v, %v), want not found", got, ok, err)
	}
	revoked, err = s.RevokeBoxToken(row.Id, boxA.ID)
	if err != nil || revoked {
		t.Errorf("second revoke = (%v, %v), want (false, nil)", revoked, err)
	}
}

// A box with no live token lists as an empty slice, not null.
func TestBoxTokenListEmpty(t *testing.T) {
	s := testStore(t)
	box, err := s.CreateBox("box-empty", "Empty", "", "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.ListBoxTokens(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("ListBoxTokens for a box with no tokens = %v, want a non-nil empty slice", got)
	}
}

// The wire lifecycle: the operator lists the live token, revokes it, sees
// the list empty, and the second delete answers 404 rather than claiming
// a second success.
func TestBoxTokenListAndRevoke(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	box, err := f.srv.store.CreateBox("box-list", "Listed", "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, row := mintBoxToken(t, f, box.ID)

	var listed []BoxToken
	resp := f.do(t, http.MethodGet, "/api/boxes/"+box.ID+"/tokens", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET tokens: %d, want 200", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Id != row.Id {
		t.Fatalf("listed = %+v, want the minted token", listed)
	}

	resp = f.do(t, http.MethodDelete, "/api/boxes/"+box.ID+"/tokens/"+row.Id, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE token: %d, want 200", resp.StatusCode)
	}
	listed = listed[:0]
	resp = f.do(t, http.MethodGet, "/api/boxes/"+box.ID+"/tokens", nil)
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Errorf("listed after revoke = %+v, want empty", listed)
	}
	resp = f.do(t, http.MethodDelete, "/api/boxes/"+box.ID+"/tokens/"+row.Id, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("deleting a revoked token: %d, want 404", resp.StatusCode)
	}
}

// A token never reaches these routes: requireSession admits a browser
// session and refuses every bearer, so a token cannot mint or revoke a
// token. A customer session is authenticated but not the platform
// operator, so the gate refuses it with 403.
func TestBoxTokenSurfaceRefusals(t *testing.T) {
	f := hostedCustomer(t)
	box, err := f.srv.store.CreateBox("box-gate-tok", "Gated", "", "")
	if err != nil {
		t.Fatal(err)
	}

	wide := f.mintToken(t, authz.Grant{Scopes: authz.GrantableScopes()})
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/boxes/" + box.ID + "/tokens"},
		{http.MethodPost, "/api/boxes/" + box.ID + "/tokens"},
		{http.MethodDelete, "/api/boxes/" + box.ID + "/tokens/token-whatever"},
		{http.MethodPatch, "/api/boxes/" + box.ID + "/tokens/token-whatever"},
	} {
		resp := f.withToken(t, wide, c.method, c.path)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s with a token: %d, want 401", c.method, c.path, resp.StatusCode)
		}
	}

	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/boxes/" + box.ID + "/tokens"},
		{http.MethodPost, "/api/boxes/" + box.ID + "/tokens"},
		{http.MethodDelete, "/api/boxes/" + box.ID + "/tokens/token-whatever"},
		{http.MethodPatch, "/api/boxes/" + box.ID + "/tokens/token-whatever"},
	} {
		resp := f.do(t, c.method, c.path, nil)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s as a customer: %d, want 403", c.method, c.path, resp.StatusCode)
		}
	}
}

// Every box token route 404s a missing box rather than writing or leaking
// anything, and a token id that is not a live token of the box is a 404
// on the delete.
func TestBoxTokenMissingBoxAndForeignToken(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	box, err := f.srv.store.CreateBox("box-miss-tok", "Missing", "", "")
	if err != nil {
		t.Fatal(err)
	}
	missing := "box-00000000-0000-0000-0000-000000000000"
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/api/boxes/" + missing + "/tokens"},
		{http.MethodGet, "/api/boxes/" + missing + "/tokens"},
		{http.MethodDelete, "/api/boxes/" + missing + "/tokens/token-whatever"},
		{http.MethodPatch, "/api/boxes/" + missing + "/tokens/token-whatever"},
	} {
		resp := f.do(t, c.method, c.path, nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s on a missing box: %d, want 404", c.method, c.path, resp.StatusCode)
		}
	}

	_, row := mintBoxToken(t, f, box.ID)
	resp := f.do(t, http.MethodDelete, "/api/boxes/"+missing+"/tokens/"+row.Id, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("deleting a live token through another box's path: %d, want 404", resp.StatusCode)
	}
	resp = f.do(t, http.MethodDelete, "/api/boxes/"+box.ID+"/tokens/token-whatever", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("deleting an unknown token id: %d, want 404", resp.StatusCode)
	}
}

// A box token carries the name the operator gave it, and the name can be
// relabelled without touching the secret.
func TestBoxTokenNameAndRename(t *testing.T) {
	s := testStore(t)
	box, err := s.CreateBox("box-named", "Named", "", "")
	if err != nil {
		t.Fatal(err)
	}
	secret, row, err := s.CreateBoxToken(box.ID, "primary")
	if err != nil {
		t.Fatal(err)
	}
	if row.Name != "primary" {
		t.Errorf("minted name = %q, want primary", row.Name)
	}

	renamed, err := s.RenameBoxToken(row.Id, box.ID, "backup")
	if err != nil || !renamed {
		t.Fatalf("RenameBoxToken = (%v, %v), want (true, nil)", renamed, err)
	}
	// The secret still authenticates after a rename.
	if got, ok, err := s.BoxTokenAuth(secret); err != nil || !ok || got != box.ID {
		t.Errorf("secret after rename = (%q, %v, %v), want the box", got, ok, err)
	}
	listed, err := s.ListBoxTokens(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Name != "backup" {
		t.Errorf("listed after rename = %+v, want one token named backup", listed)
	}

	// A rename scoped to another box finds nothing.
	other, err := s.CreateBox("box-named-other", "Other", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.RenameBoxToken(row.Id, other.ID, "stolen"); err != nil || got {
		t.Errorf("cross-box rename = (%v, %v), want (false, nil)", got, err)
	}
}
