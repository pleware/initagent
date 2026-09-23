package hub

import "testing"

// TestTheSeedNamesTheEngineOfEveryNonLlamaPin pins the one field that decides
// which program opens a pin's bytes. VoxCPM2 is the first GGUF the roster must
// NOT hand to `llama-server` — it is served by audio.cpp — and the rest of the
// factory pins name no engine at all, which means llama.cpp. A seed that set
// this on the wrong pin would send a VoxCPM2 file to llama-server, or a chat
// model to audiocpp_server, and both fail at the box rather than here.
func TestTheSeedNamesTheEngineOfEveryNonLlamaPin(t *testing.T) {
	s := testStore(t)
	byID := map[string]Model{}
	all, err := s.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range all {
		byID[m.ID] = m
	}
	vox, ok := byID["voxcpm2-q8_0"]
	if !ok {
		t.Fatal("the seed lost the voxcpm2 pin")
	}
	if vox.Engine != "audio.cpp" {
		t.Errorf("voxcpm2-q8_0 engine = %q, want audio.cpp", vox.Engine)
	}
	// The mouth's other engine is stated too: a Piper voice is opened by
	// Piper's container, and saying so is what lets a box skip the pin by name
	// instead of by an empty quantization.
	for _, id := range []string{"pl_PL-gosia-medium", "pl_PL-bass-high"} {
		if m, ok := byID[id]; ok && m.Engine != "piper" {
			t.Errorf("%s engine = %q, want piper", id, m.Engine)
		}
	}
	// llama.cpp pins keep the default: empty, so no historical pin changed
	// shape when the field arrived.
	for _, id := range []string{"qwen3.5-4b-q4_k_m", "bge-m3"} {
		if m, ok := byID[id]; ok && m.Engine != "" {
			t.Errorf("%s engine = %q, want empty (llama.cpp is the default)", id, m.Engine)
		}
	}
}