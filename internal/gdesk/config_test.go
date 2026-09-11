package gdesk

import (
	"errors"
	"strings"
	"testing"
)

// openaiEntry is the smallest provider entry that can actually be called.
func openaiEntry() map[string]string {
	return map[string]string{
		"INITAGENT_GDESK_PROVIDER_OPENAI_SHAPE":       "openai",
		"INITAGENT_GDESK_PROVIDER_OPENAI_SECRET_KIND": "openai",
		"INITAGENT_OPENAI_API_KEY":                    "sk-test",
	}
}

func TestLoadConfigBindsEveryRole(t *testing.T) {
	t.Parallel()
	env := openaiEntry()
	env["INITAGENT_GDESK_CHAT"] = "openai/gpt-4o-mini"
	env["INITAGENT_GDESK_STT"] = "openai/whisper-1"
	env["INITAGENT_GDESK_TTS"] = "openai/gpt-4o-mini-tts"

	cfg, err := LoadConfig(env)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got := cfg.Silences(); len(got) != 0 {
		t.Fatalf("silences = %v, want none", got)
	}
	if cfg.Mute() || cfg.Deaf() {
		t.Fatalf("mute = %v, deaf = %v, want both false", cfg.Mute(), cfg.Deaf())
	}
	chat, ok := cfg.Binding(RoleChat)
	if !ok {
		t.Fatal("chat role is not bound")
	}
	if chat.Model != "gpt-4o-mini" {
		t.Errorf("model = %q, want gpt-4o-mini", chat.Model)
	}
	if chat.Provider.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("base URL = %q, want the shape default", chat.Provider.BaseURL)
	}
	if chat.Provider.Key != "sk-test" {
		t.Errorf("key = %q, want the value the secret kind names", chat.Provider.Key)
	}
}

// TestModelIdKeepsItsSlash matters because Groq, OpenRouter and vLLM publish
// weights under a vendor path. Splitting on every slash would turn
// meta-llama/Llama-3-8b into a malformed setting.
func TestModelIdKeepsItsSlash(t *testing.T) {
	t.Parallel()
	env := openaiEntry()
	env["INITAGENT_GDESK_CHAT"] = "openai/meta-llama/Llama-3-8b"

	cfg, err := LoadConfig(env)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	chat, _ := cfg.Binding(RoleChat)
	if chat.Model != "meta-llama/Llama-3-8b" {
		t.Errorf("model = %q, want the full vendor path", chat.Model)
	}
}

// TestUnboundRolesStartTheDeskAnyway is the rule that keeps a missing voice
// from taking device reads and MCP down with it.
func TestUnboundRolesStartTheDeskAnyway(t *testing.T) {
	t.Parallel()
	env := openaiEntry()
	env["INITAGENT_GDESK_CHAT"] = "openai/gpt-4o-mini"

	cfg, err := LoadConfig(env)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.Mute() || !cfg.Deaf() {
		t.Fatalf("mute = %v, deaf = %v, want both true", cfg.Mute(), cfg.Deaf())
	}
	want := []Silence{
		{Role: RoleSTT, Reason: SilenceUnbound},
		{Role: RoleTTS, Reason: SilenceUnbound},
	}
	got := cfg.Silences()
	if len(got) != len(want) {
		t.Fatalf("silences = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("silence %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestVendorAPIKeyBindsTheRole is the laptop that already exported
// OPENAI_API_KEY and should not have to duplicate it as INITAGENT_OPENAI_API_KEY.
func TestVendorAPIKeyBindsTheRole(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		"INITAGENT_GDESK_PROVIDER_OPENAI_SHAPE":       "openai",
		"INITAGENT_GDESK_PROVIDER_OPENAI_SECRET_KIND": "openai",
		"INITAGENT_GDESK_CHAT":                        "openai/gpt-4o-mini",
		"OPENAI_API_KEY":                              "sk-from-vendor-env",
	}
	cfg, err := LoadConfig(env)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	chat, ok := cfg.Binding(RoleChat)
	if !ok {
		t.Fatal("chat is unbound with OPENAI_API_KEY set")
	}
	if chat.Provider.Key != "sk-from-vendor-env" {
		t.Errorf("key = %q, want sk-from-vendor-env", chat.Provider.Key)
	}
}

// TestMissingKeySilencesTheRole follows the mailer precedent: a key is
// operations catching up, not a malformed file.
func TestMissingKeySilencesTheRole(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		"INITAGENT_GDESK_PROVIDER_OPENAI_SHAPE":       "openai",
		"INITAGENT_GDESK_PROVIDER_OPENAI_SECRET_KIND": "openai",
		"INITAGENT_GDESK_CHAT":                        "openai/gpt-4o-mini",
	}
	cfg, err := LoadConfig(env)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, ok := cfg.Binding(RoleChat); ok {
		t.Fatal("chat is bound with no key")
	}
	want := Silence{Role: RoleChat, Provider: "openai", Reason: SilenceNoKey}
	if got := cfg.Silences(); len(got) == 0 || got[0] != want {
		t.Fatalf("silences = %v, want first %v", got, want)
	}
}

// TestOpenEndpointNeedsNoKey is the local whisper.cpp and Piper case.
func TestOpenEndpointNeedsNoKey(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		"INITAGENT_GDESK_PROVIDER_LOCAL_WHISPER_SHAPE":    "openai",
		"INITAGENT_GDESK_PROVIDER_LOCAL_WHISPER_BASE_URL": "http://127.0.0.1:8080/v1/",
		"INITAGENT_GDESK_STT":                             "local-whisper/base",
	}
	cfg, err := LoadConfig(env)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	stt, ok := cfg.Binding(RoleSTT)
	if !ok {
		t.Fatal("stt is not bound")
	}
	if cfg.Deaf() {
		t.Error("the desk is deaf with a bound transcriber")
	}
	if stt.Provider.ID != "local-whisper" {
		t.Errorf("provider id = %q, want local-whisper", stt.Provider.ID)
	}
	if stt.Provider.BaseURL != "http://127.0.0.1:8080/v1" {
		t.Errorf("base URL = %q, want the trailing slash gone", stt.Provider.BaseURL)
	}
	if stt.Provider.Key != "" {
		t.Errorf("key = %q, want empty for an open endpoint", stt.Provider.Key)
	}
}

// TestProviderIdEndingInAFieldWord checks the longest-suffix rule: the id is
// what is left after the field, not after the first word that looks like one.
func TestProviderIdEndingInAFieldWord(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		"INITAGENT_GDESK_PROVIDER_MY_SHAPE_SHAPE": "anthropic",
		"INITAGENT_GDESK_CHAT":                    "my-shape/claude-haiku",
	}
	cfg, err := LoadConfig(env)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	chat, ok := cfg.Binding(RoleChat)
	if !ok {
		t.Fatal("chat is not bound")
	}
	if chat.Provider.ID != "my-shape" {
		t.Errorf("provider id = %q, want my-shape", chat.Provider.ID)
	}
	if chat.Provider.BaseURL != "https://api.anthropic.com/v1" {
		t.Errorf("base URL = %q, want the anthropic default", chat.Provider.BaseURL)
	}
}

// TestDeclaredButUnboundProviderIsFine is how a swap is prepared.
func TestDeclaredButUnboundProviderIsFine(t *testing.T) {
	t.Parallel()
	env := openaiEntry()
	env["INITAGENT_GDESK_PROVIDER_PIPER_LOCAL_SHAPE"] = "openai"
	env["INITAGENT_GDESK_PROVIDER_PIPER_LOCAL_BASE_URL"] = "http://127.0.0.1:5000"
	env["INITAGENT_GDESK_PROVIDER_PIPER_LOCAL_API_PATH"] = "/speak"
	env["INITAGENT_GDESK_CHAT"] = "openai/gpt-4o-mini"

	cfg, err := LoadConfig(env)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, ok := cfg.Binding(RoleChat); !ok {
		t.Fatal("chat is not bound")
	}
}

func TestAPIPathOverrideIsKept(t *testing.T) {
	t.Parallel()
	env := openaiEntry()
	env["INITAGENT_GDESK_PROVIDER_OPENAI_API_PATH"] = "/chat/completions"
	env["INITAGENT_GDESK_CHAT"] = "openai/gpt-4o-mini"

	cfg, err := LoadConfig(env)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	chat, _ := cfg.Binding(RoleChat)
	if chat.Provider.APIPath != "/chat/completions" {
		t.Errorf("API path = %q, want the override", chat.Provider.APIPath)
	}
}

// TestSilencesIsACopy keeps a caller reporting configuration from editing it.
func TestSilencesIsACopy(t *testing.T) {
	t.Parallel()
	cfg, err := LoadConfig(map[string]string{})
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	got := cfg.Silences()
	if len(got) != len(Roles) {
		t.Fatalf("silences = %v, want one per role", got)
	}
	got[0].Reason = "tampered"
	if cfg.Silences()[0].Reason != SilenceUnbound {
		t.Error("Silences handed out the configuration's own slice")
	}
}

// TestMalformedRefusesToStart is the strict half of "strict on
// configuration, lenient on the wire". Each case is a typo an operator makes
// once, and every one of them would otherwise be a quietly broken desk.
func TestMalformedRefusesToStart(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		env      map[string]string
		mentions string
	}{
		{
			name:     "role value has no model",
			env:      map[string]string{"INITAGENT_GDESK_CHAT": "openai"},
			mentions: "provider/model",
		},
		{
			name:     "role value has no provider",
			env:      map[string]string{"INITAGENT_GDESK_CHAT": "/gpt-4o-mini"},
			mentions: "provider/model",
		},
		{
			name:     "role value has no model after the slash",
			env:      map[string]string{"INITAGENT_GDESK_CHAT": "openai/"},
			mentions: "provider/model",
		},
		{
			name:     "provider was never declared",
			env:      map[string]string{"INITAGENT_GDESK_CHAT": "openai-mian/gpt-4o-mini"},
			mentions: "INITAGENT_GDESK_PROVIDER_OPENAI_MIAN_SHAPE",
		},
		{
			name: "provider entry has no shape",
			env: map[string]string{
				"INITAGENT_GDESK_PROVIDER_OPENAI_BASE_URL": "https://api.openai.com/v1",
			},
			mentions: "INITAGENT_GDESK_PROVIDER_OPENAI_SHAPE",
		},
		{
			name: "shape is not a dialect we speak",
			env: map[string]string{
				"INITAGENT_GDESK_PROVIDER_OPENAI_SHAPE": "gemini",
			},
			mentions: "anthropic or openai",
		},
		{
			name: "base URL is not an address",
			env: map[string]string{
				"INITAGENT_GDESK_PROVIDER_OPENAI_SHAPE":    "openai",
				"INITAGENT_GDESK_PROVIDER_OPENAI_BASE_URL": "api.openai.com",
			},
			mentions: "http(s)",
		},
		{
			name: "API path is not a path",
			env: map[string]string{
				"INITAGENT_GDESK_PROVIDER_OPENAI_SHAPE":    "openai",
				"INITAGENT_GDESK_PROVIDER_OPENAI_API_PATH": "chat/completions",
			},
			mentions: "must start with /",
		},
		{
			name:     "a setting nobody reads",
			env:      map[string]string{"INITAGENT_GDESK_VOICE": "ania"},
			mentions: "INITAGENT_GDESK_VOICE",
		},
		{
			name:     "a provider field nobody reads",
			env:      map[string]string{"INITAGENT_GDESK_PROVIDER_OPENAI_TIMEOUT": "30s"},
			mentions: "INITAGENT_GDESK_PROVIDER_OPENAI_TIMEOUT",
		},
		{
			name:     "provider key names no provider",
			env:      map[string]string{"INITAGENT_GDESK_PROVIDER__SHAPE": "openai"},
			mentions: "names no provider",
		},
		{
			name:     "provider segment is not a provider id",
			env:      map[string]string{"INITAGENT_GDESK_PROVIDER_Open-AI_SHAPE": "openai"},
			mentions: "not a provider id",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := LoadConfig(tc.env)
			if !errors.Is(err, ErrConfig) {
				t.Fatalf("error = %v, want ErrConfig", err)
			}
			if !strings.Contains(err.Error(), tc.mentions) {
				t.Errorf("error %q does not mention %q", err, tc.mentions)
			}
		})
	}
}

// TestTheSeamIsClosedUntilSomebodyMintsAToken is the safe default: a desk
// nobody can reach beats a desk every process on the box may join.
func TestTheSeamIsClosedUntilSomebodyMintsAToken(t *testing.T) {
	t.Parallel()
	cfg, err := LoadConfig(map[string]string{})
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Seam().Open() {
		t.Error("the seam is open with no token")
	}
	if cfg.Seam().Addr != DefaultSeamAddr {
		t.Errorf("addr = %q, want the default %q", cfg.Seam().Addr, DefaultSeamAddr)
	}
}

// TestASeamTokenOpensTheDeskAtAnAddress covers the two settings together,
// because either alone is a desk that does not answer.
func TestASeamTokenOpensTheDeskAtAnAddress(t *testing.T) {
	t.Parallel()
	cfg, err := LoadConfig(map[string]string{
		"INITAGENT_GDESK_SEAM_ADDR":  "localhost:4300",
		"INITAGENT_GDESK_SEAM_TOKEN": "sec-desk-token",
	})
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.Seam().Open() {
		t.Fatal("the seam is closed with a token set")
	}
	if cfg.Seam().Addr != "localhost:4300" {
		t.Errorf("addr = %q, want the value that was set", cfg.Seam().Addr)
	}
	if cfg.Seam().Token != "sec-desk-token" {
		t.Errorf("token = %q, want the value that was set", cfg.Seam().Token)
	}
}

// TestOnlyALoopbackAddressIsAccepted is the decision, not a preference. The
// seam's own Origin check cannot tell a phone on the Wi-Fi from the glass on
// this box, so the address is where that is settled: a device off this machine
// joins through the hub as a relay (workspace docs/DESK-SCOPES.md).
func TestOnlyALoopbackAddressIsAccepted(t *testing.T) {
	t.Parallel()
	cases := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:4202", true},
		{"localhost:4202", true},
		{"[::1]:4202", true},
		{"127.0.0.1:0", true},
		{"0.0.0.0:4202", false},
		{"192.168.1.10:4202", false},
		{":4202", false},
		{"4202", false},
		{"127.0.0.1", false},
		{"127.0.0.1:", false},
	}
	for _, tc := range cases {
		t.Run(tc.addr, func(t *testing.T) {
			t.Parallel()
			cfg, err := LoadConfig(map[string]string{"INITAGENT_GDESK_SEAM_ADDR": tc.addr})
			switch {
			case tc.want && err != nil:
				t.Fatalf("LoadConfig(%q) = %v, want it accepted", tc.addr, err)
			case tc.want && cfg.Seam().Addr != tc.addr:
				t.Errorf("addr = %q, want %q", cfg.Seam().Addr, tc.addr)
			case !tc.want && !errors.Is(err, ErrConfig):
				t.Fatalf("LoadConfig(%q) = %v, want ErrConfig", tc.addr, err)
			}
		})
	}
}

// TestUnrelatedEnvironmentIsIgnored keeps the strict check scoped: a box has
// hundreds of variables and none of the others are ours to judge.
func TestUnrelatedEnvironmentIsIgnored(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		"PATH":                 "/usr/bin",
		"INITAGENT_HUB":        "https://hub.example",
		"INITAGENT_DESKTOP":    "gnome",
		"INITAGENT_OPENAI_KEY": "not-ours-either",
	}
	if _, err := LoadConfig(env); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
}

// TestLoadConfigFromEnvReadsTheProcess covers the package's only edge.
func TestLoadConfigFromEnvReadsTheProcess(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("INITAGENT_GDESK_PROVIDER_OPENAI_SHAPE", "openai")
	t.Setenv("INITAGENT_GDESK_PROVIDER_OPENAI_SECRET_KIND", "openai")
	t.Setenv("INITAGENT_OPENAI_API_KEY", "sk-from-process")
	t.Setenv("INITAGENT_GDESK_CHAT", "openai/gpt-4o-mini")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv: %v", err)
	}
	chat, ok := cfg.Binding(RoleChat)
	if !ok {
		t.Fatal("chat is not bound")
	}
	if chat.Provider.Key != "sk-from-process" {
		t.Errorf("key = %q, want the process value", chat.Provider.Key)
	}
}
