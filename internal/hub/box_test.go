package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/offering"
)

// ParseEdition accepts the five appliance classes and the empty string,
// trimmed and case-insensitive; the empty string is lite and an unknown
// name is refused so a typo cannot mint the wrong appliance.
func TestParseEdition(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{name: "empty is lite", in: "", want: "lite", ok: true},
		{name: "lite", in: "lite", want: "lite", ok: true},
		{name: "company", in: "company", want: "company", ok: true},
		{name: "home trimmed and cased", in: " Home ", want: "home", ok: true},
		{name: "assist", in: "assist", want: "assist", ok: true},
		{name: "care cased", in: "CARE", want: "care", ok: true},
		{name: "unknown refused", in: "gaming", want: "", ok: false},
		{name: "hub refused", in: "hub", want: "", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseEdition(tt.in)
			if tt.ok && err != nil {
				t.Fatalf("ParseEdition(%q) error = %v, want nil", tt.in, err)
			}
			if !tt.ok && err == nil {
				t.Fatalf("ParseEdition(%q) = %q, want an error", tt.in, got)
			}
			if got != tt.want {
				t.Errorf("ParseEdition(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// Edition travels on the wire: create defaults to lite, carries a named
// edition, and refuses an unknown one with 400; update replaces it, and
// each write bumps config_version so a connector's sync sees the change.
func TestBoxEditionWire(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)

	resp := f.do(t, http.MethodPost, "/api/boxes", map[string]any{"slug": "box-lite", "name": "Lite"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST without an edition: %d, want 200", resp.StatusCode)
	}
	var lite Box
	if err := json.NewDecoder(resp.Body).Decode(&lite); err != nil {
		t.Fatal(err)
	}
	if lite.Edition != "lite" || lite.ConfigVersion != 1 {
		t.Errorf("default create = %+v, want edition lite and configVersion 1", lite)
	}

	resp = f.do(t, http.MethodPost, "/api/boxes", map[string]any{
		"slug": "box-company", "name": "Company", "edition": " Company ",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST with an edition: %d, want 200", resp.StatusCode)
	}
	var company Box
	if err := json.NewDecoder(resp.Body).Decode(&company); err != nil {
		t.Fatal(err)
	}
	if company.Edition != "company" || company.ConfigVersion != 1 {
		t.Errorf("named create = %+v, want edition company and configVersion 1", company)
	}

	resp = f.do(t, http.MethodPost, "/api/boxes", map[string]any{
		"slug": "box-bad", "name": "Bad", "edition": "gaming",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("POST with an unknown edition: %d, want 400", resp.StatusCode)
	}

	resp = f.do(t, http.MethodPatch, "/api/boxes/"+company.ID, map[string]any{"name": "Care", "edition": "care"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH with an edition: %d, want 200", resp.StatusCode)
	}
	var updated Box
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.Edition != "care" || updated.ConfigVersion != 2 {
		t.Errorf("updated = %+v, want edition care and configVersion 2", updated)
	}

	resp = f.do(t, http.MethodPatch, "/api/boxes/"+company.ID, map[string]any{"name": "Bad", "edition": "bogus"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("PATCH with an unknown edition: %d, want 400", resp.StatusCode)
	}
}

// Deleting a box removes the row and everything bound to it; the second
// delete answers 404.
func TestBoxDelete(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	box, err := f.srv.store.CreateBox("box-gone", "Gone", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.srv.store.SetBoxOrgs(box.ID, []string{"org-a"}); err != nil {
		t.Fatal(err)
	}

	resp := f.do(t, http.MethodDelete, "/api/boxes/"+box.ID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /api/boxes/%s: %d, want 200", box.ID, resp.StatusCode)
	}
	if got, err := f.srv.store.GetBox(box.ID); err != nil || got != nil {
		t.Fatalf("GetBox after delete = (%v, %v), want (nil, nil)", got, err)
	}
	roster, err := f.srv.store.StaffForBox(box.ID)
	if err != nil || len(roster) != 0 {
		t.Fatalf("StaffForBox after delete = (%v, %v), want empty", roster, err)
	}
	orgs, err := f.srv.store.ListBoxOrgs(box.ID)
	if err != nil || len(orgs) != 0 {
		t.Fatalf("ListBoxOrgs after delete = (%v, %v), want empty", orgs, err)
	}

	resp = f.do(t, http.MethodDelete, "/api/boxes/"+box.ID, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("second DELETE: %d, want 404", resp.StatusCode)
	}
}

// The platform operator creates, lists, reads and updates boxes over the
// wire, and every write answers with the full row.
func TestBoxCRUD(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)

	resp := f.do(t, http.MethodGet, "/api/boxes", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/boxes: %d, want 200", resp.StatusCode)
	}
	var empty []Box
	if err := json.NewDecoder(resp.Body).Decode(&empty); err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("initial boxes = %+v, want none", empty)
	}

	resp = f.do(t, http.MethodPost, "/api/boxes", map[string]any{
		"slug": "box-alpha", "name": "Alpha box", "hostId": "host-00000000-0000-0000-0000-000000000000",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/boxes: %d, want 200", resp.StatusCode)
	}
	var created Box
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Slug != "box-alpha" || created.Name != "Alpha box" ||
		created.HostID != "host-00000000-0000-0000-0000-000000000000" {
		t.Errorf("created = %+v, want a minted row for the submitted slug, name and hostId", created)
	}
	if created.CreatedAt == 0 || created.UpdatedAt == 0 {
		t.Errorf("timestamps not set: %+v", created)
	}

	// A duplicate slug is refused before any write.
	resp = f.do(t, http.MethodPost, "/api/boxes", map[string]any{"slug": "box-alpha", "name": "Twin"})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate slug: %d, want 409", resp.StatusCode)
	}

	resp = f.do(t, http.MethodGet, "/api/boxes/"+created.ID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/boxes/%s: %d, want 200", created.ID, resp.StatusCode)
	}
	var got Box
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.ID != created.ID || got.Slug != "box-alpha" {
		t.Errorf("read-back = %+v, want the created row", got)
	}

	resp = f.do(t, http.MethodPatch, "/api/boxes/"+created.ID, map[string]any{
		"name": "Alpha renamed", "hostId": "",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH /api/boxes/%s: %d, want 200", created.ID, resp.StatusCode)
	}
	var updated Box
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID || updated.Name != "Alpha renamed" || updated.HostID != "" {
		t.Errorf("updated = %+v, want the same row renamed with the host binding cleared", updated)
	}

	var boxes []Box
	resp = f.do(t, http.MethodGet, "/api/boxes", nil)
	if err := json.NewDecoder(resp.Body).Decode(&boxes); err != nil {
		t.Fatal(err)
	}
	if len(boxes) != 1 || boxes[0].ID != created.ID {
		t.Errorf("list = %+v, want the one created box", boxes)
	}
}

// The operator replaces a box's organization set through the wire, and a
// missing box is a 404 rather than a write.
func TestBoxSetOrgs(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	box, err := f.srv.store.CreateBox("box-orgs", "Orgs", "", "")
	if err != nil {
		t.Fatal(err)
	}

	resp := f.do(t, http.MethodPut, "/api/boxes/"+box.ID+"/orgs", map[string]any{
		"orgIds": []string{"org-a", "org-b"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT orgs: %d, want 200", resp.StatusCode)
	}
	got, err := f.srv.store.ListBoxOrgs(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "org-a" || got[1] != "org-b" {
		t.Errorf("bound orgs = %v, want [org-a org-b]", got)
	}

	// The GET reads back exactly the set the PUT wrote, in the same shape.
	resp = f.do(t, http.MethodGet, "/api/boxes/"+box.ID+"/orgs", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET orgs: %d, want 200", resp.StatusCode)
	}
	var bound struct {
		OrgIDs []string `json:"orgIds"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&bound); err != nil {
		t.Fatal(err)
	}
	if len(bound.OrgIDs) != 2 || bound.OrgIDs[0] != "org-a" || bound.OrgIDs[1] != "org-b" {
		t.Errorf("GET orgs = %v, want [org-a org-b]", bound.OrgIDs)
	}
	resp = f.do(t, http.MethodGet, "/api/boxes/box-00000000-0000-0000-0000-000000000000/orgs", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET orgs on a missing box: %d, want 404", resp.StatusCode)
	}

	// The second call replaces the set, not appends to it.
	resp = f.do(t, http.MethodPut, "/api/boxes/"+box.ID+"/orgs", map[string]any{
		"orgIds": []string{"org-c"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT orgs again: %d, want 200", resp.StatusCode)
	}
	got, _ = f.srv.store.ListBoxOrgs(box.ID)
	if len(got) != 1 || got[0] != "org-c" {
		t.Errorf("bound orgs after replace = %v, want [org-c]", got)
	}

	resp = f.do(t, http.MethodPut, "/api/boxes/box-00000000-0000-0000-0000-000000000000/orgs",
		map[string]any{"orgIds": []string{"org-c"}})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("PUT orgs on a missing box: %d, want 404", resp.StatusCode)
	}
}

// A box is born with its narrator already seeded (CreateBox runs
// EnsureSeedBoxNarrator), so the endpoint serves it straight away; a box
// whose narrator row is gone answers 404 with the reason until it is
// seeded again.
func TestBoxNarrator(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	box, err := f.srv.store.CreateBox("box-nar", "Narrated", "", "")
	if err != nil {
		t.Fatal(err)
	}

	resp := f.do(t, http.MethodGet, "/api/boxes/"+box.ID+"/narrator", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("narrator after create: %d, want 200", resp.StatusCode)
	}
	var narrator Staff
	if err := json.NewDecoder(resp.Body).Decode(&narrator); err != nil {
		t.Fatal(err)
	}
	if narrator.Slug != "st_b_pi" || narrator.Scope != "box" || narrator.BoxID != box.ID {
		t.Errorf("narrator = %+v, want the seeded box-scoped st_b_pi row", narrator)
	}

	// Take the narrator away: a box with no narrator answers 404 with the
	// reason, so the cockpit can tell "nothing to show" from "not allowed".
	if _, err := f.srv.store.db.Exec(`DELETE FROM staff WHERE scope = 'box' AND box_id = ?`, box.ID); err != nil {
		t.Fatal(err)
	}
	resp = f.do(t, http.MethodGet, "/api/boxes/"+box.ID+"/narrator", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("narrator of a box without one: %d, want 404", resp.StatusCode)
	}

	if err := f.srv.store.EnsureSeedBoxNarrator(box.ID); err != nil {
		t.Fatal(err)
	}
	resp = f.do(t, http.MethodGet, "/api/boxes/"+box.ID+"/narrator", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("narrator after re-seeding: %d, want 200", resp.StatusCode)
	}

	resp = f.do(t, http.MethodGet, "/api/boxes/box-00000000-0000-0000-0000-000000000000/narrator", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("narrator of a missing box: %d, want 404", resp.StatusCode)
	}
}

// Editing the narrator round-trips the nine editable fields, bumps the
// box's config_version by exactly one per edit, and the seed does not
// bump: a fresh box stays at version 1.
func TestBoxNarratorEdit(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	box, err := f.srv.store.CreateBox("box-narrator-edit", "Editable", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := f.srv.store.GetBox(box.ID); got.ConfigVersion != 1 {
		t.Fatalf("fresh box config_version = %d, want 1 (the seed must not bump)", got.ConfigVersion)
	}

	want := Character{
		Openness: 0.9, Conscientiousness: 0.8, Extraversion: 0.7,
		Agreeableness: 0.6, Neuroticism: 0.2,
	}
	resp := f.do(t, http.MethodPatch, "/api/boxes/"+box.ID+"/narrator", map[string]any{
		"name": "Lore", "locale": "en", "age": 42, "wordBudget": 1200,
		"avatarModel3d": "lore.glb", "voice": "lore-v2", "biologicalGender": "female", "brief": "warm and precise",
		"soulCore": "explain first", "bigFive": map[string]float64{
			"openness": 0.9, "conscientiousness": 0.8, "extraversion": 0.7,
			"agreeableness": 0.6, "neuroticism": 0.2,
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH narrator: %d, want 200", resp.StatusCode)
	}
	var narrator Staff
	if err := json.NewDecoder(resp.Body).Decode(&narrator); err != nil {
		t.Fatal(err)
	}
	if narrator.Slug != "st_b_pi" || narrator.Scope != "box" || narrator.BoxID != box.ID {
		t.Errorf("narrator identity = %+v, want the box-scoped st_b_pi row", narrator)
	}
	if narrator.Name != "Lore" || narrator.Locale != "en" || narrator.Age != 42 ||
		narrator.WordBudget != 1200 || narrator.AvatarModel3D != "lore.glb" ||
		narrator.Voice != "lore-v2" || narrator.BiologicalGender != "female" ||
		narrator.Brief != "warm and precise" || narrator.SoulCore != "explain first" || narrator.BigFive != want {
		t.Errorf("narrator after the edit = %+v, want the submitted nine fields", narrator)
	}
	if got, _ := f.srv.store.GetBox(box.ID); got.ConfigVersion != 2 {
		t.Errorf("config_version after the edit = %d, want 2", got.ConfigVersion)
	}

	// The GET reads the same edited row back.
	resp = f.do(t, http.MethodGet, "/api/boxes/"+box.ID+"/narrator", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET narrator after the edit: %d, want 200", resp.StatusCode)
	}
	var readBack Staff
	if err := json.NewDecoder(resp.Body).Decode(&readBack); err != nil {
		t.Fatal(err)
	}
	if readBack.ID != narrator.ID || readBack.Name != "Lore" {
		t.Errorf("narrator read back = %+v, want the edited row %+v", readBack, narrator)
	}

	// A second edit bumps again, exactly once more.
	resp = f.do(t, http.MethodPatch, "/api/boxes/"+box.ID+"/narrator", map[string]any{
		"name": "Lore Two", "biologicalGender": "female",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("second PATCH narrator: %d, want 200", resp.StatusCode)
	}
	if got, _ := f.srv.store.GetBox(box.ID); got.ConfigVersion != 3 {
		t.Errorf("config_version after the second edit = %d, want 3", got.ConfigVersion)
	}
	// The locale was omitted, so it falls back to the Polish default.
	var after Staff
	if err := json.NewDecoder(resp.Body).Decode(&after); err != nil {
		t.Fatal(err)
	}
	if after.Locale != "pl" {
		t.Errorf("narrator locale after an edit without locale = %q, want the pl default", after.Locale)
	}

	// A missing box is a 404 before anything is written.
	resp = f.do(t, http.MethodPatch, "/api/boxes/box-00000000-0000-0000-0000-000000000000/narrator",
		map[string]any{"name": "Ghost"})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("PATCH narrator of a missing box: %d, want 404", resp.StatusCode)
	}
}

// The narrator edit refuses a blank name and a negative age or word budget
// with 400, and a refused edit writes nothing.
func TestBoxNarratorEditValidation(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	box, err := f.srv.store.CreateBox("box-narrator-validation", "Strict", "", "")
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name string
		body map[string]any
	}{
		{"no name", map[string]any{"locale": "en"}},
		{"blank name", map[string]any{"name": "   "}},
		{"negative age", map[string]any{"name": "Lore", "age": -1}},
		{"negative word budget", map[string]any{"name": "Lore", "wordBudget": -1}},
		{"invalid biological gender", map[string]any{"name": "Lore", "biologicalGender": "x"}},
	} {
		resp := f.do(t, http.MethodPatch, "/api/boxes/"+box.ID+"/narrator", c.body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", c.name, resp.StatusCode)
		}
	}
	if got, _ := f.srv.store.GetBox(box.ID); got.ConfigVersion != 1 {
		t.Errorf("config_version after the refusals = %d, want 1 (nothing written)", got.ConfigVersion)
	}
}

// Box creation needs a nameable box: slug and name are required, the slug
// must fit the [a-z0-9-] shape, and a missing box answers 404 on the read
// paths.
func TestBoxValidation(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)

	for _, c := range []struct {
		name string
		body map[string]any
	}{
		{"no slug", map[string]any{"name": "Nova"}},
		{"blank slug", map[string]any{"slug": "   ", "name": "Nova"}},
		{"no name", map[string]any{"slug": "box-nova"}},
		{"blank name", map[string]any{"slug": "box-nova", "name": " "}},
		{"slug with capitals and a space", map[string]any{"slug": "AXL HQ!", "name": "Nova"}},
		{"slug with underscores and capitals", map[string]any{"slug": "AxL_hq", "name": "Nova"}},
	} {
		resp := f.do(t, http.MethodPost, "/api/boxes", c.body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", c.name, resp.StatusCode)
		}
	}

	// The same handler admits a slug in the promised shape.
	resp := f.do(t, http.MethodPost, "/api/boxes", map[string]any{"slug": "axl-hq", "name": "Nova"})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("valid slug: %d, want 200", resp.StatusCode)
	}

	resp = f.do(t, http.MethodPatch, "/api/boxes/box-00000000-0000-0000-0000-000000000000",
		map[string]any{"name": "Nope"})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("PATCH a missing box: %d, want 404", resp.StatusCode)
	}
	resp = f.do(t, http.MethodGet, "/api/boxes/box-00000000-0000-0000-0000-000000000000", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET a missing box: %d, want 404", resp.StatusCode)
	}
}

// The update path applies the same name rule as create: a missing or blank
// name is a 400, and a malformed body is refused before any store call.
func TestBoxUpdateValidation(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	box, err := f.srv.store.CreateBox("box-patch", "Patchable", "", "")
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name string
		body map[string]any
	}{
		{"no name", map[string]any{"hostId": ""}},
		{"blank name", map[string]any{"name": "   "}},
	} {
		resp := f.do(t, http.MethodPatch, "/api/boxes/"+box.ID, c.body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", c.name, resp.StatusCode)
		}
	}

	// The host binding itself may be set or cleared by the same call.
	resp := f.do(t, http.MethodPatch, "/api/boxes/"+box.ID, map[string]any{
		"name": "Bound", "hostId": "host-00000000-0000-0000-0000-000000000007",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH with a hostId: %d, want 200", resp.StatusCode)
	}
	var updated Box
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.HostID != "host-00000000-0000-0000-0000-000000000007" {
		t.Errorf("HostID after binding = %q, want the submitted host", updated.HostID)
	}
}

// A body that is not JSON is refused with 400 on every write path before
// anything reaches the store.
func TestBoxMalformedBody(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	box, err := f.srv.store.CreateBox("box-json", "JSON", "", "")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ method, path string }{
		{http.MethodPost, "/api/boxes"},
		{http.MethodPatch, "/api/boxes/" + box.ID},
		{http.MethodPut, "/api/boxes/" + box.ID + "/orgs"},
		{http.MethodPatch, "/api/boxes/" + box.ID + "/narrator"},
	}
	for _, c := range cases {
		req, err := http.NewRequest(c.method, f.ts.URL+c.path, strings.NewReader(`{"slug": `))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := f.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s %s with a malformed body: %d, want 400", c.method, c.path, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

// An installation token carrying read:fleet.box stands at the empty
// boundary, so the wire admits it on the box read routes; the same token
// is refused on the write routes, which gate on admin:fleet.box. A
// customer session fails the gate with 403 and an anonymous caller is
// turned away at the middleware with 401.
func TestBoxGateRefusals(t *testing.T) {
	// The installation token is minted by the platform admin on their own
	// hub; the customer refusals run on a separate hosted hub.
	tf := claimedHub(t, offering.Hosted)
	secret, _, err := tf.srv.store.CreateAdminToken("ci", tf.ownerId, []authz.Capability{authz.ReadBox})
	if err != nil {
		t.Fatal(err)
	}
	resp := tf.withToken(t, secret, http.MethodGet, "/api/boxes")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("list with a read installation token: %d, want 200", resp.StatusCode)
	}
	resp = tf.withToken(t, secret, http.MethodPost, "/api/boxes")
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("create with a read-only installation token: %d, want 403", resp.StatusCode)
	}
	resp = tf.withToken(t, secret, http.MethodDelete, "/api/boxes/box-00000000-0000-0000-0000-000000000000")
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("delete with a read-only installation token: %d, want 403", resp.StatusCode)
	}

	f := hostedCustomer(t)
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/boxes"},
		{http.MethodPost, "/api/boxes"},
		{http.MethodDelete, "/api/boxes/box-00000000-0000-0000-0000-000000000000"},
		{http.MethodPatch, "/api/boxes/box-00000000-0000-0000-0000-000000000000/narrator"},
	} {
		resp := f.do(t, c.method, c.path, nil)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s as a customer: %d, want 403", c.method, c.path, resp.StatusCode)
		}
		resp = requestJSON(t, f.ts, &http.Client{}, c.method, c.path, nil)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s without a session: %d, want 401", c.method, c.path, resp.StatusCode)
		}
	}
}

// A credential that reaches the handler without the box capability is
// refused at the empty boundary, whether the capability rides on an
// installation token missing the verb or on an org token whose boundary
// cannot contain the installation.
func TestBoxGateRefusesWrongCredentials(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	cases := []struct {
		name string
		cred authz.Credential
	}{
		{
			name: "installation token without the capability",
			cred: authz.Credential{
				Requester: authz.Requester{Account: "account-ops", Platform: true},
				Grant:     &authz.Grant{Installation: true, Scopes: []authz.Capability{authz.ReadOrg}},
			},
		},
		{
			name: "org token carrying the capability",
			cred: authz.Credential{
				Requester: authz.Requester{Account: "account-ops", Platform: true},
				Grant:     &authz.Grant{Org: "org-1", Scopes: []authz.Capability{authz.ReadBox}},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/boxes", nil)
			rec := httptest.NewRecorder()
			f.srv.handleListBoxes(rec, req, c.cred)
			if rec.Code != http.StatusForbidden {
				t.Errorf("list boxes: %d, want 403", rec.Code)
			}
		})
	}
}

// The id-bearing handlers share the box surface's empty-boundary gates: a
// credential that reaches them without the installation grant is refused
// before any lookup, so no box id leaks through the refusal. The org
// grant below carries both box verbs, which its boundary still cannot
// hold at the installation.
func TestBoxGateRefusesWrongCredentialsOnIdHandlers(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	box, err := f.srv.store.CreateBox("box-gate", "Gated", "", "")
	if err != nil {
		t.Fatal(err)
	}
	cred := authz.Credential{
		Requester: authz.Requester{Account: "account-ops", Platform: true},
		Grant:     &authz.Grant{Org: "org-1", Scopes: []authz.Capability{authz.ReadBox, authz.AdminBox}},
	}
	cases := []struct {
		name string
		call func(w http.ResponseWriter, r *http.Request)
	}{
		{name: "get box", call: func(w http.ResponseWriter, r *http.Request) { f.srv.handleGetBox(w, r, cred) }},
		{name: "update box", call: func(w http.ResponseWriter, r *http.Request) { f.srv.handleUpdateBox(w, r, cred) }},
		{name: "delete box", call: func(w http.ResponseWriter, r *http.Request) { f.srv.handleDeleteBox(w, r, cred) }},
		{name: "set box orgs", call: func(w http.ResponseWriter, r *http.Request) { f.srv.handleSetBoxOrgs(w, r, cred) }},
		{name: "list box orgs", call: func(w http.ResponseWriter, r *http.Request) { f.srv.handleListBoxOrgs(w, r, cred) }},
		{name: "get box narrator", call: func(w http.ResponseWriter, r *http.Request) { f.srv.handleGetBoxNarrator(w, r, cred) }},
		{name: "update box narrator", call: func(w http.ResponseWriter, r *http.Request) { f.srv.handleUpdateBoxNarrator(w, r, cred) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/boxes/"+box.ID, nil)
			req.SetPathValue("id", box.ID)
			rec := httptest.NewRecorder()
			c.call(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Errorf("handler: %d, want 403", rec.Code)
			}
		})
	}
}

// The box surface splits on its two verbs: a token carrying only
// read:fleet.box is admitted on the read handlers and refused on every
// mutation, and a token carrying only admin:fleet.box is the mirror
// image — admitted on the writes, refused on the reads. Each verb is
// checked in both directions so neither gate can collapse into the
// other.
func TestBoxGateReadAdminSplit(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	box, err := f.srv.store.CreateBox("box-split", "Split", "", "")
	if err != nil {
		t.Fatal(err)
	}
	readOnly := authz.Credential{
		Requester: authz.Requester{Account: "account-ops", Platform: true},
		Grant:     &authz.Grant{Installation: true, Scopes: []authz.Capability{authz.ReadBox}},
	}
	adminOnly := authz.Credential{
		Requester: authz.Requester{Account: "account-ops", Platform: true},
		Grant:     &authz.Grant{Installation: true, Scopes: []authz.Capability{authz.AdminBox}},
	}
	get := httptest.NewRequest(http.MethodGet, "/api/boxes/"+box.ID, nil)
	get.SetPathValue("id", box.ID)

	// A read-only token is admitted on the read handlers.
	rec := httptest.NewRecorder()
	f.srv.handleListBoxes(rec, httptest.NewRequest(http.MethodGet, "/api/boxes", nil), readOnly)
	if rec.Code != http.StatusOK {
		t.Errorf("list boxes with a read-only token: %d, want 200", rec.Code)
	}
	rec = httptest.NewRecorder()
	f.srv.handleGetBox(rec, get, readOnly)
	if rec.Code != http.StatusOK {
		t.Errorf("get box with a read-only token: %d, want 200", rec.Code)
	}

	// ...and refused on the mutating handlers.
	mutating := []struct {
		name string
		call func(w http.ResponseWriter, r *http.Request, cred authz.Credential)
	}{
		{name: "create box", call: func(w http.ResponseWriter, r *http.Request, cred authz.Credential) { f.srv.handleCreateBox(w, r, cred) }},
		{name: "update box", call: func(w http.ResponseWriter, r *http.Request, cred authz.Credential) { f.srv.handleUpdateBox(w, r, cred) }},
		{name: "delete box", call: func(w http.ResponseWriter, r *http.Request, cred authz.Credential) { f.srv.handleDeleteBox(w, r, cred) }},
		{name: "set box orgs", call: func(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
			f.srv.handleSetBoxOrgs(w, r, cred)
		}},
		{name: "update box narrator", call: func(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
			f.srv.handleUpdateBoxNarrator(w, r, cred)
		}},
	}
	for _, c := range mutating {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c.call(rec, get, readOnly)
			if rec.Code != http.StatusForbidden {
				t.Errorf("with a read-only token: %d, want 403", rec.Code)
			}
		})
	}

	// An admin-only token is admitted on the writes...
	update := httptest.NewRequest(http.MethodPatch, "/api/boxes/"+box.ID,
		strings.NewReader(`{"name":"Split renamed"}`))
	update.SetPathValue("id", box.ID)
	update.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	f.srv.handleUpdateBox(rec, update, adminOnly)
	if rec.Code != http.StatusOK {
		t.Errorf("update box with an admin-only token: %d, want 200", rec.Code)
	}

	// ...and refused on the reads.
	for _, c := range []struct {
		name string
		call func(w http.ResponseWriter, r *http.Request, cred authz.Credential)
	}{
		{name: "list boxes", call: func(w http.ResponseWriter, r *http.Request, cred authz.Credential) { f.srv.handleListBoxes(w, r, cred) }},
		{name: "get box", call: func(w http.ResponseWriter, r *http.Request, cred authz.Credential) { f.srv.handleGetBox(w, r, cred) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c.call(rec, get, adminOnly)
			if rec.Code != http.StatusForbidden {
				t.Errorf("with an admin-only token: %d, want 403", rec.Code)
			}
		})
	}
}
