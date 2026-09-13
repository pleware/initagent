package gdesk

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/pleware/initagent/internal/brand"
)

// fileConfig is the on-disk shape of a desk. It is draft 41's provider
// entry plus the two local things that entry does not cover: who may join
// this box, and which role uses which provider.
//
// Secrets sit in their own map, named by secret_kind, never on the provider
// row — the row is what you paste into a bug report.
type fileConfig struct {
	Seam      *fileSeam         `yaml:"seam"`
	Roles     *fileRoles        `yaml:"roles"`
	Vision    *fileVision       `yaml:"vision"`
	Providers []fileProvider    `yaml:"providers"`
	Secrets   map[string]string `yaml:"secrets"`
}

type fileVision struct {
	Camera  *int   `yaml:"camera"`
	Sensor  string `yaml:"sensor"`
	Command string `yaml:"command"`
}

type fileSeam struct {
	Addr  string `yaml:"addr"`
	Token string `yaml:"token"`
}

type fileRoles struct {
	Chat string `yaml:"chat"`
	STT  string `yaml:"stt"`
	TTS  string `yaml:"tts"`
}

type fileProvider struct {
	ID         string `yaml:"id"`
	Shape      string `yaml:"shape"`
	BaseURL    string `yaml:"base_url"`
	APIPath    string `yaml:"api_path"`
	SecretKind string `yaml:"secret_kind"`
}

// LoadConfigFromEnv reads the optional desk YAML and then the process
// environment. A variable that is set wins over the file, so a one-off
// override does not require editing the file, and tests that set the
// process stay isolated from whatever lives in the operator's home.
func LoadConfigFromEnv() (Config, error) {
	env, err := mergeDeskEnv(os.Environ())
	if err != nil {
		return Config{}, err
	}
	return LoadConfig(env)
}

// mergeDeskEnv turns the process snapshot and the desk YAML into one map.
func mergeDeskEnv(environ []string) (map[string]string, error) {
	env := AliasLegacyEnv(environMap(environ))
	path, err := deskConfigPath(env)
	if err != nil {
		return nil, err
	}
	if path == "" {
		return env, nil
	}
	fromFile, err := loadDeskFile(path)
	if err != nil {
		return nil, err
	}
	return overlayEnv(fromFile, env), nil
}

// AliasLegacyEnv copies every `INITAGENT_GDESK_…` variable onto its
// `INITAGENT_GDESK_…` name, which is what the desk reads since it became the
// `gdesk` context (workspace draft 05).
//
// The new name wins where both are set, and the old one is left in place
// rather than deleted — a caller that prints its own environment should see
// what it exported. Aliasing by prefix rather than by a list of names keeps
// the provider keys working, since those are `PROVIDER_<ID>_<FIELD>` and no
// list would stay complete.
func AliasLegacyEnv(env map[string]string) map[string]string {
	out := maps.Clone(env)
	if out == nil {
		out = map[string]string{}
	}
	for key, value := range env {
		legacy, ok := strings.CutPrefix(key, brand.LegacyGdeskEnvPrefix)
		if !ok {
			continue
		}
		current := brand.GdeskEnvPrefix + legacy
		if _, taken := out[current]; taken {
			continue
		}
		out[current] = value
	}
	return out
}

func environMap(environ []string) map[string]string {
	env := make(map[string]string, len(environ))
	for _, entry := range environ {
		if key, value, ok := strings.Cut(entry, "="); ok {
			env[key] = value
		}
	}
	return env
}

// overlayEnv copies file values, then lets the process win on every key it
// actually set — including an empty string, which is how a test clears a
// token the file would have opened.
func overlayEnv(file, process map[string]string) map[string]string {
	out := maps.Clone(file)
	if out == nil {
		out = map[string]string{}
	}
	for key, value := range process {
		out[key] = value
	}
	return out
}

// deskConfigPath is the YAML to read, or empty if none should be.
//
// The home lookup accepts the pre-`gdesk` filename as well, and in that order:
// the file holds a seam token that exists nowhere else, so an upgrade that
// silently stopped reading it would close the seam with nothing to point at.
func deskConfigPath(env map[string]string) (string, error) {
	if explicit := strings.TrimSpace(env[brand.EnvGdeskConfig]); explicit != "" {
		if _, err := os.Stat(explicit); err != nil {
			return "", fmt.Errorf("%w: %s is %q, and that file cannot be read: %v",
				ErrConfig, brand.EnvGdeskConfig, explicit, err)
		}
		return explicit, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", nil
	}
	return firstExistingFile(filepath.Join(home, brand.ConfigDir),
		brand.GdeskConfigFile, brand.LegacyDeskConfigFile), nil
}

// firstExistingFile returns the first of names that exists in dir, or empty.
// Order is the caller's preference, and the only reason this is a function of
// its own is that the preference is the part worth a test.
func firstExistingFile(dir string, names ...string) string {
	for _, name := range names {
		candidate := filepath.Join(dir, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func loadDeskFile(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot read %s: %v", ErrConfig, path, err)
	}
	env, err := deskFileToEnv(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrConfig, path, err)
	}
	return env, nil
}

func deskFileToEnv(raw []byte) (map[string]string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("file is empty")
	}
	var file fileConfig
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&file); err != nil {
		return nil, err
	}

	env := map[string]string{}
	if file.Seam != nil {
		put(env, brand.EnvGdeskSeamAddr, file.Seam.Addr)
		put(env, brand.EnvGdeskSeamToken, file.Seam.Token)
	}
	if file.Roles != nil {
		put(env, brand.EnvGdeskChat, file.Roles.Chat)
		put(env, brand.EnvGdeskSTT, file.Roles.STT)
		put(env, brand.EnvGdeskTTS, file.Roles.TTS)
	}
	if file.Vision != nil {
		camera := 0
		if file.Vision.Camera != nil {
			camera = *file.Vision.Camera
		}
		env[brand.EnvGdeskVisionCamera] = strconv.Itoa(camera)
		put(env, brand.EnvGdeskVisionSensor, file.Vision.Sensor)
		put(env, brand.EnvGdeskVisionCommand, file.Vision.Command)
	}
	for i, provider := range file.Providers {
		if strings.TrimSpace(provider.ID) == "" {
			return nil, fmt.Errorf("providers[%d] has no id", i)
		}
		segment := envSegment(provider.ID)
		prefix := brand.EnvGdeskProviderPrefix + segment
		put(env, prefix+"_SHAPE", provider.Shape)
		put(env, prefix+"_BASE_URL", provider.BaseURL)
		put(env, prefix+"_API_PATH", provider.APIPath)
		put(env, prefix+"_SECRET_KIND", provider.SecretKind)
	}
	for kind, value := range file.Secrets {
		kind = strings.TrimSpace(kind)
		if kind == "" {
			return nil, fmt.Errorf("a secret is named by an empty kind")
		}
		put(env, brand.EnvAPIKey(kind), value)
	}
	return env, nil
}

func put(env map[string]string, key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	env[key] = strings.TrimSpace(value)
}
