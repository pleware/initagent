package agent

import (
	"path"
	"testing"
)

func TestManagedPathEntriesWithoutHomeNeverReturnsRelativePaths(t *testing.T) {
	if paths := managedPathEntries("windows", "", ""); len(paths) != 0 {
		t.Fatalf("Windows without an absolute base should add no PATH entries: %#v", paths)
	}
	if paths := managedPathEntries("windows", "", ".relative-dir"); len(paths) != 0 {
		t.Fatalf("Windows with a relative base should add no PATH entries: %#v", paths)
	}
	for _, p := range managedPathEntries("linux", "", "") {
		if !path.IsAbs(p) {
			t.Fatalf("managed PATH entry must be absolute: %q", p)
		}
	}
}
