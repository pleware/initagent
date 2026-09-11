package desk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const exampleDeskFile = `
seam:
  addr: 127.0.0.1:4202
  token: sec-from-file
roles:
  chat: openai/gpt-4o-mini
providers:
  - id: openai
    shape: openai
    secret_kind: openai
secrets:
  openai: sk-from-file
`

func TestDeskFileOpensTheSameDeskAsEnv(t *testing.T) {
	t.Parallel()
	env, err := deskFileToEnv([]byte(exampleDeskFile))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(env)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.Seam().Open() || cfg.Seam().Token != "sec-from-file" {
		t.Fatalf("seam = %+v, want the token from the file", cfg.Seam())
	}
	chat, ok := cfg.Binding(RoleChat)
	if !ok || chat.Provider.Key != "sk-from-file" {
		t.Fatalf("chat = %+v, bound = %v", chat, ok)
	}
}

func TestEmptySecretsInTheFileDoNotBindAKey(t *testing.T) {
	t.Parallel()
	env, err := deskFileToEnv([]byte(`
roles:
  chat: openai/gpt-4o-mini
providers:
  - id: openai
    shape: openai
    secret_kind: openai
secrets:
  openai: ""
`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := env["INITAGENT_OPENAI_API_KEY"]; ok {
		t.Fatal("an empty secret must not appear as a key")
	}
}

func TestAnUnknownFieldInTheFileIsRefused(t *testing.T) {
	t.Parallel()
	_, err := deskFileToEnv([]byte("voice: ania\n"))
	if err == nil {
		t.Fatal("want a refusal")
	}
}

func TestAProviderWithoutAnIdIsRefused(t *testing.T) {
	t.Parallel()
	_, err := deskFileToEnv([]byte(`
providers:
  - shape: openai
`))
	if err == nil || !strings.Contains(err.Error(), "id") {
		t.Fatalf("err = %v, want it to mention id", err)
	}
}

func TestTheProcessOverridesTheFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "desk.yaml")
	if err := os.WriteFile(path, []byte(exampleDeskFile), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INITAGENT_DESK_CONFIG", path)
	t.Setenv("INITAGENT_DESK_SEAM_TOKEN", "sec-from-process")
	t.Setenv("INITAGENT_OPENAI_API_KEY", "sk-from-process")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Seam().Token != "sec-from-process" {
		t.Errorf("token = %q, want the process to win", cfg.Seam().Token)
	}
	chat, ok := cfg.Binding(RoleChat)
	if !ok || chat.Provider.Key != "sk-from-process" {
		t.Fatalf("key = %q, bound = %v, want the process to win", chat.Provider.Key, ok)
	}
}

func TestAMissingExplicitFileIsAConfigurationError(t *testing.T) {
	t.Setenv("INITAGENT_DESK_CONFIG", filepath.Join(t.TempDir(), "gone.yaml"))
	_, err := LoadConfigFromEnv()
	if err == nil || !strings.Contains(err.Error(), "INITAGENT_DESK_CONFIG") {
		t.Fatalf("err = %v, want it to name the variable", err)
	}
}

func TestTheShippedExampleIsADeskTheLoaderAccepts(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "..", "desk.example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	env, err := deskFileToEnv(raw)
	if err != nil {
		t.Fatalf("example: %v", err)
	}
	cfg, err := LoadConfig(env)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Seam().Open() {
		t.Fatal("the example must ship with the seam closed")
	}
	if _, ok := cfg.Binding(RoleChat); ok {
		t.Fatal("the example must not invent a key")
	}
}
