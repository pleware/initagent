package authz

import (
	"slices"
	"testing"

	"github.com/pleware/initagent/internal/names"
)

// The permission plane's right-hand side is data now: `names/names.yaml` lists
// the verbs each entity grants, and `verb:context.entity` is derived from them.
//
// authz.go is deliberately not generated. Two things live in it and only one is
// vocabulary: which capabilities exist, and which role floor carries each one
// inside an organization. The second is policy — it has no row in the registry
// and no business having one — so generating the constants would split one file
// into a generated half and a hand-written half that has to agree with it,
// which is the drift this pass exists to remove rather than relocate. The
// exported constant names are also public API, and none of them derives from
// its value (`create:fleet.device` is EnrollDevice, not CreateDevice).
//
// So the constants stay written and this test makes them derived in the sense
// that matters: the registry decides which capabilities may exist, and a
// constant on either side of that with no partner fails here.
//
// This runs alongside entity_test.go rather than replacing it. That test
// resolves each capability's entity through id.Lookup — the generated registry
// as `internal/id` sees it. This one checks the source file both are generated
// from. A disagreement between the two would mean a stale registry_gen.go,
// which `internal/names`' goldens test names directly.

func registryCapabilities(t *testing.T) []string {
	t.Helper()
	registry, err := names.Load("../../names/names.yaml")
	if err != nil {
		t.Fatalf("load the naming registry: %v", err)
	}
	return registry.Capabilities()
}

func TestEveryDeclaredCapabilityIsInTheRegistry(t *testing.T) {
	t.Parallel()
	granted := registryCapabilities(t)
	for name, value := range declaredCapabilities(t) {
		if !slices.Contains(granted, value) {
			t.Errorf("%s (%q) is declared here but names/names.yaml grants no such verb on that entity; "+
				"add the verb to the entity's capabilities: list", name, value)
		}
	}
}

func TestEveryRegistryCapabilityIsDeclared(t *testing.T) {
	t.Parallel()
	declared := map[string]bool{}
	for _, value := range declaredCapabilities(t) {
		declared[value] = true
	}
	// The other direction is the one that caught nothing before this test
	// existed: a verb added to the registry with no constant here is a
	// permission the vocabulary promises and no handler can ask for.
	for _, capability := range registryCapabilities(t) {
		if !declared[capability] {
			t.Errorf("names/names.yaml grants %q but no Capability constant spells it", capability)
		}
	}
}
