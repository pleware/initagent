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

	narrator, ok := got["narrator"].(*Narrator)
	if !ok {
		t.Fatalf("manifest narrator = %T, want the box's seeded narrator", got["narrator"])
	}
	if narrator.Slug != "st_b_pi" {
		t.Errorf("narrator = %+v, want the box's st_b_pi narrator", narrator)
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
	if _, err := s.db.Exec(`DELETE FROM box_narrator WHERE box_id = ?`, box.ID); err != nil {
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

// The manifest models section is the resolved roster: per purpose the six
// pin fields, the override winning over the assignment, and a purpose with
// neither omitted. The value carries exactly id, source, quant, file, digest
// and licence — the purpose rides in the key, never inside the value.
func TestBuildBoxManifestModelsSection(t *testing.T) {
	s := testStoreNoAssignments(t)
	// Create the pin directly (not via verifiedModel) so it carries a real
	// file — the manifest must emit it, not just round-trip an empty value.
	canonical, err := s.CreateModel("manifest-worker", "Org", "Org/manifest-worker@rev", "", "manifest-worker.gguf", "canonical-digest", "MIT", "worker")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetAssignment("worker", canonical.ID); err != nil {
		t.Fatal(err)
	}

	factoryBox := testBox(t, s, "manifest-factory-box")
	got, err := s.BuildBoxManifest(factoryBox.ID)
	if err != nil {
		t.Fatal(err)
	}
	models, ok := got["models"].(map[string]map[string]any)
	if !ok {
		t.Fatalf("manifest models = %T, want a purpose-keyed map", got["models"])
	}
	if len(models) != 1 {
		t.Fatalf("manifest models = %+v, want the one factory purpose", models)
	}
	wantPin := map[string]any{
		"id":      canonical.ID,
		"source":  canonical.Source,
		"engine":  canonical.Engine,
		"quant":   canonical.Quant,
		"file":    canonical.File,
		"digest":  canonical.Digest,
		"licence": canonical.Licence,
		"files":   []ModelFile{},
	}
	if !reflect.DeepEqual(models["worker"], wantPin) {
		t.Errorf("models[worker] = %v, want exactly %v", models["worker"], wantPin)
	}
	if _, hasOrg := models["worker"]["org"]; hasOrg {
		t.Errorf("manifest pin leaks the registry org key: %v", models["worker"])
	}

	// A box with an override resolves to the override, same key shape.
	override := verifiedModel(t, s, "manifest-override", "override-digest", "worker")
	overrideBox := testBox(t, s, "manifest-override-box")
	if _, err := s.SetBoxModelOverride(overrideBox.ID, "worker", override.ID); err != nil {
		t.Fatal(err)
	}
	got, err = s.BuildBoxManifest(overrideBox.ID)
	if err != nil {
		t.Fatal(err)
	}
	models = got["models"].(map[string]map[string]any)
	if models["worker"]["id"] != override.ID {
		t.Errorf("override box models = %+v, want the override %q", models, override.ID)
	}

	// A directory-shaped pin carries its whole file list into the manifest,
	// not just the anchor: the box's puller fetches what the pin names, and
	// the ear loads the directory. A single-artifact pin answers [] — one
	// shape for the box's parser either way.
	ear, err := s.CreateModel("manifest-ear", "Systran", "Systran/faster-whisper-medium@rev",
		"", "model.bin", "ear-digest", "MIT", "stt")
	if err != nil {
		t.Fatal(err)
	}
	earFiles := []ModelFile{
		{File: "model.bin"},
		{File: "config.json"},
		{File: "tokenizer.json"},
		{File: "vocabulary.txt"},
	}
	if _, err := s.SetModelFiles(ear.ID, earFiles); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetAssignment("stt", ear.ID); err != nil {
		t.Fatal(err)
	}
	got, err = s.BuildBoxManifest(factoryBox.ID)
	if err != nil {
		t.Fatal(err)
	}
	models = got["models"].(map[string]map[string]any)
	listed, ok := models["stt"]["files"].([]ModelFile)
	if !ok {
		t.Fatalf("manifest stt files = %T, want a file list", models["stt"]["files"])
	}
	if len(listed) != len(earFiles) || listed[3].File != "vocabulary.txt" {
		t.Errorf("manifest stt files = %+v, want %+v", listed, earFiles)
	}
	// The seed writes names only, so the anchor's entry arrives empty — the
	// manifest fills it from the pin's own verified digest, or the one file the
	// hub has verified would be the one file the box pulls unverified.
	if listed[0].Digest != "ear-digest" {
		t.Errorf("manifest anchor entry = %+v, want the pin's digest on the anchor", listed[0])
	}
	if listed[1].Digest != "" {
		t.Errorf("manifest filled a digest the pin does not have: %+v", listed[1])
	}
}

// A box on a store with no pins at all carries an empty models map, not a
// null. Assignments are installation-wide, so this needs its own store: a
// box beside a factory assignment can never be pin-free.
func TestBuildBoxManifestModelsEmpty(t *testing.T) {
	s := testStoreNoAssignments(t)
	bareBox := testBox(t, s, "manifest-bare-box")
	got, err := s.BuildBoxManifest(bareBox.ID)
	if err != nil {
		t.Fatal(err)
	}
	models, ok := got["models"].(map[string]map[string]any)
	if !ok || len(models) != 0 {
		t.Errorf("bare box models = %#v, want an empty map", got["models"])
	}
}
