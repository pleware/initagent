package config

import (
	"bytes"
	"testing"
)

func TestYAMLIsPresent(t *testing.T) {
	t.Parallel()
	if len(YAML) == 0 {
		t.Fatal("empty catalog.yaml")
	}
	if !bytes.Contains(YAML, []byte("plansOrder")) {
		t.Fatal("catalog.yaml missing plansOrder")
	}
	if !bytes.Contains(YAML, []byte("\n    free:\n")) {
		t.Fatal("catalog.yaml missing free slug key")
	}
	if bytes.Contains(YAML, []byte("\n      label:")) {
		t.Fatal("catalog.yaml must not carry labels")
	}
}
