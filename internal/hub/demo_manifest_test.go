package hub

import (
	"encoding/json"
	"testing"
)

// Temporary demo for the owner: prints the exact `models` section the box
// sync endpoint returns, before and after assigning the factory pins.
func TestDemoBoxManifestForOwner(t *testing.T) {
	s := testStore(t) // OpenStore seeds the 11 factory pins
	box := testBox(t, s, "demo-box")

	// Before any assignment — the resolved roster is empty.
	got, err := s.BuildBoxManifest(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.MarshalIndent(got["models"], "", "  ")
	t.Logf("models (brak assignmentów):\n%s", b)

	// Assign the six purposes to their factory pins.
	for _, a := range []struct{ purpose, id string }{
		{"persona", "qwen3.5-4b-q4_k_m"},
		{"worker", "qwen2.5-coder-7b-q4_k_m"},
		{"embedding", "bge-m3"},
		{"stt", "faster-whisper-medium"},
		{"vad", "silero-vad"},
		{"tts", "pl_PL-gosia-medium"},
	} {
		if _, err := s.SetAssignment(a.purpose, a.id); err != nil {
			t.Fatalf("SetAssignment(%s, %s): %v", a.purpose, a.id, err)
		}
	}

	got, err = s.BuildBoxManifest(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.MarshalIndent(got["models"], "", "  ")
	t.Logf("models (po przypisaniu 6 purpose):\n%s", b)
}
