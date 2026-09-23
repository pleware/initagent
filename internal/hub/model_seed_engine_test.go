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
	// Every other pin stays on the default: empty is llama.cpp, and a Piper
	// voice or a recogniser is served by its own program, never by the roster.
	for _, id := range []string{"pl_PL-gosia-medium", "faster-whisper-medium", "qwen3.5-4b-q4_k_m"} {
		if m, ok := byID[id]; ok && m.Engine != "" && m.Engine != "llama.cpp" {
			t.Errorf("%s engine = %q, want empty (llama.cpp) or an engine its own program serves", id, m.Engine)
		}
	}
}