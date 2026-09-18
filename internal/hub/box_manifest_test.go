package hub

import (
	"reflect"
	"testing"
)

// The manifest carries the box row, only its bound organizations, the staff
// roster keyed per bound org, and the box's narrator — with the version
// matching the row's config_version.
func TestBuildBoxManifestShape(t *testing.T) {
	s := testStore(t)
	orgA, err := s.CreateOrg("Alpha org")
	if err != nil {
		t.Fatal(err)
	}
	orgB, err := s.CreateOrg("Beta org")
	if err != nil {
		t.Fatal(err)
	}
	box, err := s.CreateBox("box-manifest", "Manifest box", "host-00000000-0000-0000-0000-000000000000", "company")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetBoxOrgs(box.ID, []string{orgA.Id, orgB.Id}); err != nil {
		t.Fatal(err)
	}

	got, err := s.BuildBoxManifest(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := s.GetBox(box.ID)
	if err != nil || fresh == nil {
		t.Fatalf("GetBox = (%v, %v), want the row", fresh, err)
	}
	if got["version"] != fresh.ConfigVersion {
		t.Errorf("manifest version = %v, want the row's %d", got["version"], fresh.ConfigVersion)
	}

	boxObj, ok := got["box"].(map[string]any)
	if !ok {
		t.Fatalf("manifest box = %T, want an object", got["box"])
	}
	wantBox := map[string]any{
		"id": box.ID, "slug": box.Slug, "name": box.Name,
		"edition": box.Edition, "hostId": box.HostID,
	}
	if !reflect.DeepEqual(boxObj, wantBox) {
		t.Errorf("manifest box = %v, want %v", boxObj, wantBox)
	}

	orgs, ok := got["orgs"].([]map[string]any)
	if !ok {
		t.Fatalf("manifest orgs = %T, want a list", got["orgs"])
	}
	if len(orgs) != 2 {
		t.Fatalf("manifest orgs = %v, want the two bound orgs", orgs)
	}
	orgByName := map[string]string{}
	for _, o := range orgs {
		orgByName[o["name"].(string)] = o["id"].(string)
	}
	if orgByName["Alpha org"] != orgA.Id || orgByName["Beta org"] != orgB.Id {
		t.Errorf("manifest orgs = %v, want Alpha and Beta by id", orgByName)
	}

	staff, ok := got["staff"].(map[string]any)
	if !ok {
		t.Fatalf("manifest staff = %T, want an org-keyed map", got["staff"])
	}
	if len(staff) != 2 {
		t.Fatalf("manifest staff keys = %v, want one per bound org", staff)
	}
	for _, org := range []*Org{orgA, orgB} {
		roster, ok := staff[org.Id].([]Staff)
		if !ok {
			t.Fatalf("staff for %s = %T, want a list", org.Id, staff[org.Id])
		}
		slugs := []string{}
		for _, st := range roster {
			slugs = append(slugs, st.Slug)
		}
		if want := []string{"staff-female-00", "staff-male-00"}; !reflect.DeepEqual(slugs, want) {
			t.Errorf("staff slugs for %s = %v, want the seeded roster %v", org.Id, slugs, want)
		}
	}

	narrator, ok := got["narrator"].(Staff)
	if !ok {
		t.Fatalf("manifest narrator = %T, want the box's seeded narrator", got["narrator"])
	}
	if narrator.Slug != "st_b_dt" || narrator.BoxID != box.ID {
		t.Errorf("narrator = %+v, want the box's st_b_dt row", narrator)
	}
}

// Only the organizations bound to the box appear: an org elsewhere on the
// installation contributes neither its row nor its staff roster.
func TestBuildBoxManifestBoundOrgsOnly(t *testing.T) {
	s := testStore(t)
	orgA, err := s.CreateOrg("Bound org")
	if err != nil {
		t.Fatal(err)
	}
	orgC, err := s.CreateOrg("Unbound org")
	if err != nil {
		t.Fatal(err)
	}
	box, err := s.CreateBox("box-bounded", "Bounded", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetBoxOrgs(box.ID, []string{orgA.Id}); err != nil {
		t.Fatal(err)
	}

	got, err := s.BuildBoxManifest(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	orgs := got["orgs"].([]map[string]any)
	if len(orgs) != 1 || orgs[0]["id"] != orgA.Id {
		t.Errorf("manifest orgs = %v, want only the bound org", orgs)
	}
	staff := got["staff"].(map[string]any)
	if _, present := staff[orgC.Id]; present {
		t.Errorf("manifest staff carries the unbound org %s", orgC.Id)
	}
	if _, present := staff[orgA.Id]; !present {
		t.Errorf("manifest staff is missing the bound org %s", orgA.Id)
	}
}

// A box whose narrator row is gone (not seeded yet) carries a null narrator,
// never an empty object.
func TestBuildBoxManifestNarratorNullWhenUnseeded(t *testing.T) {
	s := testStore(t)
	box, err := s.CreateBox("box-mute", "Mute", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM staff WHERE scope = 'box' AND box_id = ?`, box.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.BuildBoxManifest(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got["narrator"] != nil {
		t.Errorf("manifest narrator = %v, want null for an unseeded box", got["narrator"])
	}
}

func TestBuildBoxManifestMissingBox(t *testing.T) {
	s := testStore(t)
	if got, err := s.BuildBoxManifest("box-00000000-0000-0000-0000-000000000000"); err == nil || got != nil {
		t.Errorf("BuildBoxManifest on a missing box = (%v, %v), want an error", got, err)
	}
}
