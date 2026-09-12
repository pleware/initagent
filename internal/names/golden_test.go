package names

import (
	"bytes"
	"os"
	"testing"
)

// Where the source and the two artifacts sit, relative to this package.
const (
	sourcePath = "../../names/names.yaml"
	goPath     = "../id/registry_gen.go"
	jsonPath   = "../../names/names.json"
)

// TestCommittedArtifactsAreCurrent is the mechanism that makes a stale
// generated file impossible to merge.
//
// Both files are committed rather than built at compile time, because a
// consumer in another repository fetches names.json by path and `internal/id`
// has to compile from a clean clone with no generator run. Committed generated
// files rot; this test is the price of not having them rot.
func TestCommittedArtifactsAreCurrent(t *testing.T) {
	t.Parallel()
	registry, err := Load(sourcePath)
	if err != nil {
		t.Fatalf("%s does not load: %v", sourcePath, err)
	}

	goWant, err := EmitGo(registry)
	if err != nil {
		t.Fatalf("EmitGo: %v", err)
	}
	compare(t, goPath, goWant)

	jsonWant, err := EmitJSON(registry)
	if err != nil {
		t.Fatalf("EmitJSON: %v", err)
	}
	compare(t, jsonPath, jsonWant)
}

func compare(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("%s is missing; regenerate with: %s", path, RegenerateCommand)
		return
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s is stale: it does not match what %s produces from %s.\n"+
			"Regenerate with: %s\n"+
			"committed %d bytes, generated %d bytes",
			path, "internal/names", sourcePath, RegenerateCommand, len(got), len(want))
	}
}

// TestTheRealRegistryHoldsTheFivePlanes is a claim about the data rather than
// the mechanism, so it belongs beside the source and not in the emitter tests:
// five is an ontology decision, and no derivation can defend it.
func TestTheRealRegistryHoldsTheFivePlanes(t *testing.T) {
	t.Parallel()
	registry, err := Load(sourcePath)
	if err != nil {
		t.Fatalf("%s does not load: %v", sourcePath, err)
	}
	want := []string{"hub", "project", "gdesk", "fleet", "worker"}
	got := registry.ContextNames()
	if len(got) != len(want) {
		t.Fatalf("contexts = %v, want the five planes %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("context %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestTheRealRegistryMatchesTheSeamEnvelope holds names.yaml to the version the
// connector and the glass actually negotiate on. It is spelled as a literal
// because importing internal/gdeskseam here would make the generator depend on
// the seam, and the seam is above the vocabulary rather than beside it.
func TestTheRealRegistryMatchesTheSeamEnvelope(t *testing.T) {
	t.Parallel()
	registry, err := Load(sourcePath)
	if err != nil {
		t.Fatalf("%s does not load: %v", sourcePath, err)
	}
	const seamVersion = 2 // internal/gdeskseam.Version, glass SEAM_VERSION
	if registry.Seam.EnvelopeVersion != seamVersion {
		t.Errorf("names.yaml declares envelope version %d, the seam speaks %d",
			registry.Seam.EnvelopeVersion, seamVersion)
	}
}
