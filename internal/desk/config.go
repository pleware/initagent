package desk

import (
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/pleware/initagent/internal/brand"
)

// ErrConfig marks a desk configuration the connector refuses to start with.
//
// Strict here, lenient on the wire: an unknown key in this file stops the
// process, while an unknown event kind on the desk seam is counted and let
// past. A seam has a producer-first rollout to protect; configuration has one
// writer, one box and one version, and a typo that silently disables the
// mouth is the classic failure of reading it leniently.
var ErrConfig = errors.New("desk: configuration")

// Shape is the wire dialect a provider speaks. It is the dialect, not the
// brand: one openai implementation reaches OpenAI, a LiteLLM proxy, Groq,
// vLLM, a local whisper.cpp server and Piper behind a thin HTTP shim (`41`).
type Shape string

const (
	ShapeOpenAI    Shape = "openai"
	ShapeAnthropic Shape = "anthropic"
)

// shapeBaseURL is where a provider of this dialect lives when the entry does
// not say. A local endpoint always says.
var shapeBaseURL = map[Shape]string{
	ShapeOpenAI:    "https://api.openai.com/v1",
	ShapeAnthropic: "https://api.anthropic.com/v1",
}

// Provider is the non-secret half of a provider entry, in the shape draft 41
// settled for the file this will one day be read from.
//
// Key is the one field that is not configuration. It arrives from the
// environment today and from a `sec-` row later, which is a line of
// migration rather than a rewrite, and it means everything else here can be
// pasted into a bug report.
type Provider struct {
	ID         string
	Shape      Shape
	BaseURL    string
	APIPath    string
	SecretKind string
	Key        string
}

// Binding is one role answered by one model at one provider.
type Binding struct {
	Role     Role
	Provider Provider
	Model    string
}

// SilenceReason says why a role cannot answer.
type SilenceReason string

const (
	// SilenceUnbound means no variable named a provider for this role.
	SilenceUnbound SilenceReason = "unbound"

	// SilenceNoKey means the provider entry names a secret whose value is
	// not in the environment yet. Precedent is EnvResendAPIKey: hosted mail
	// with no key leaves the outbox pending rather than failing to start,
	// because a key is operations catching up, not a malformed file.
	SilenceNoKey SilenceReason = "no-key"
)

// Silence is one thing this desk cannot do, and why. A missing voice starts
// the desk mute and says so out loud; it does not crash the connector, which
// still answers device reads and MCP without a voice.
type Silence struct {
	Role     Role
	Provider string
	Reason   SilenceReason
}

// Config is the desk's half of what this box was told. Personality, voice
// identity and the fleet stay hub rows; this is the inventory that renders
// them.
type Config struct {
	bindings map[Role]Binding
	silences []Silence
}

// Binding returns the binding for a role, and whether the role can answer.
func (c Config) Binding(role Role) (Binding, bool) {
	b, ok := c.bindings[role]
	return b, ok
}

// Silences lists every role that cannot answer, in Roles order.
func (c Config) Silences() []Silence {
	return slices.Clone(c.silences)
}

// Mute reports whether the desk can say nothing out loud.
func (c Config) Mute() bool {
	_, ok := c.bindings[RoleTTS]
	return !ok
}

// Deaf reports whether the desk cannot hear speech. Typing still reaches it,
// which is why this is not the same question as Mute.
func (c Config) Deaf() bool {
	_, ok := c.bindings[RoleSTT]
	return !ok
}

// providerField is one settable field of a provider entry. Longest suffix
// first, because a provider id may itself end in one of these words.
var providerFields = []struct {
	suffix string
	set    func(*Provider, string)
}{
	{"_SECRET_KIND", func(p *Provider, v string) { p.SecretKind = v }},
	{"_BASE_URL", func(p *Provider, v string) { p.BaseURL = v }},
	{"_API_PATH", func(p *Provider, v string) { p.APIPath = v }},
	{"_SHAPE", func(p *Provider, v string) { p.Shape = Shape(v) }},
}

// roleEnv maps each role to the variable that binds it.
var roleEnv = map[Role]string{
	RoleChat: brand.EnvDeskChat,
	RoleSTT:  brand.EnvDeskSTT,
	RoleTTS:  brand.EnvDeskTTS,
}

// LoadConfigFromEnv reads the process environment. It is the only edge in
// this package; everything it decides is decided by LoadConfig.
func LoadConfigFromEnv() (Config, error) {
	environ := os.Environ()
	env := make(map[string]string, len(environ))
	for _, entry := range environ {
		if key, value, ok := strings.Cut(entry, "="); ok {
			env[key] = value
		}
	}
	return LoadConfig(env)
}

// LoadConfig turns an environment snapshot into a desk configuration.
//
// Malformed refuses to start. A role nobody bound, or a bound role whose key
// has not arrived, is a Silence instead — the desk comes up able to do less
// and able to say what.
func LoadConfig(env map[string]string) (Config, error) {
	providers, err := parseProviders(env)
	if err != nil {
		return Config{}, err
	}
	if err := rejectUnknownKeys(env); err != nil {
		return Config{}, err
	}

	cfg := Config{bindings: make(map[Role]Binding, len(Roles))}
	for _, role := range Roles {
		spec := strings.TrimSpace(env[roleEnv[role]])
		if spec == "" {
			cfg.silences = append(cfg.silences, Silence{Role: role, Reason: SilenceUnbound})
			continue
		}
		binding, err := bindRole(role, spec, providers, env)
		if err != nil {
			return Config{}, err
		}
		if binding.Provider.SecretKind != "" && binding.Provider.Key == "" {
			cfg.silences = append(cfg.silences, Silence{
				Role:     role,
				Provider: binding.Provider.ID,
				Reason:   SilenceNoKey,
			})
			continue
		}
		cfg.bindings[role] = binding
	}
	return cfg, nil
}

// bindRole resolves one `provider/model` value.
func bindRole(role Role, spec string, providers map[string]Provider, env map[string]string) (Binding, error) {
	name, model, ok := strings.Cut(spec, "/")
	// Cut, not Split: a model id may itself contain a slash, which is how
	// Groq, OpenRouter and vLLM name published weights.
	if !ok {
		return Binding{}, fmt.Errorf("%w: %s must be provider/model, got %q", ErrConfig, roleEnv[role], spec)
	}
	name = strings.TrimSpace(name)
	model = strings.TrimSpace(model)
	if name == "" || model == "" {
		return Binding{}, fmt.Errorf("%w: %s must be provider/model, got %q", ErrConfig, roleEnv[role], spec)
	}
	provider, ok := providers[name]
	if !ok {
		return Binding{}, fmt.Errorf("%w: %s names provider %q, which no %s%s_SHAPE declares",
			ErrConfig, roleEnv[role], name, brand.EnvDeskProviderPrefix, envSegment(name))
	}
	if provider.SecretKind != "" {
		provider.Key = strings.TrimSpace(env[brand.EnvAPIKey(provider.SecretKind)])
	}
	return Binding{Role: role, Provider: provider, Model: model}, nil
}

// parseProviders reads every provider entry in the snapshot. A declared but
// unbound provider is fine: that is how a swap is prepared.
func parseProviders(env map[string]string) (map[string]Provider, error) {
	raw := map[string]*Provider{}
	for _, key := range slices.Sorted(maps.Keys(env)) {
		rest, ok := strings.CutPrefix(key, brand.EnvDeskProviderPrefix)
		if !ok {
			continue
		}
		for _, field := range providerFields {
			segment, matched := strings.CutSuffix(rest, field.suffix)
			if !matched {
				continue
			}
			id, err := providerID(segment, key)
			if err != nil {
				return nil, err
			}
			entry, seen := raw[id]
			if !seen {
				entry = &Provider{ID: id}
				raw[id] = entry
			}
			field.set(entry, strings.TrimSpace(env[key]))
			break
		}
	}

	providers := make(map[string]Provider, len(raw))
	for id, entry := range raw {
		if err := completeProvider(entry); err != nil {
			return nil, err
		}
		providers[id] = *entry
	}
	return providers, nil
}

// completeProvider fills defaults and rejects an entry we could not call.
func completeProvider(p *Provider) error {
	if _, ok := shapeBaseURL[p.Shape]; !ok {
		if p.Shape == "" {
			return fmt.Errorf("%w: provider %q needs %s%s_SHAPE",
				ErrConfig, p.ID, brand.EnvDeskProviderPrefix, envSegment(p.ID))
		}
		return fmt.Errorf("%w: provider %q has shape %q, want one of %s",
			ErrConfig, p.ID, p.Shape, strings.Join(shapeNames(), " or "))
	}
	if p.BaseURL == "" {
		p.BaseURL = shapeBaseURL[p.Shape]
	}
	p.BaseURL = strings.TrimRight(p.BaseURL, "/")
	parsed, err := url.Parse(p.BaseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%w: provider %q base URL %q is not an http(s) address", ErrConfig, p.ID, p.BaseURL)
	}
	if p.APIPath != "" && !strings.HasPrefix(p.APIPath, "/") {
		return fmt.Errorf("%w: provider %q API path %q must start with /", ErrConfig, p.ID, p.APIPath)
	}
	return nil
}

// rejectUnknownKeys stops the process over a variable we do not read. A
// mistyped role name is otherwise a desk that is quietly deaf.
func rejectUnknownKeys(env map[string]string) error {
	known := map[string]bool{}
	for _, name := range roleEnv {
		known[name] = true
	}
	var unknown []string
	for key := range env {
		if !strings.HasPrefix(key, brand.EnvPrefix+"DESK_") || known[key] {
			continue
		}
		if rest, ok := strings.CutPrefix(key, brand.EnvDeskProviderPrefix); ok && knownProviderField(rest) {
			continue
		}
		unknown = append(unknown, key)
	}
	if len(unknown) == 0 {
		return nil
	}
	slices.Sort(unknown)
	return fmt.Errorf("%w: no desk setting is named %s", ErrConfig, strings.Join(unknown, ", "))
}

// knownProviderField reports whether the tail of a provider key ends in a
// field we set.
func knownProviderField(rest string) bool {
	for _, field := range providerFields {
		if segment, ok := strings.CutSuffix(rest, field.suffix); ok && segment != "" {
			return true
		}
	}
	return false
}

// providerID turns the env segment of a key into a provider id. The mapping
// is total in both directions because an id may not contain an underscore.
func providerID(segment, key string) (string, error) {
	if segment == "" {
		return "", fmt.Errorf("%w: %s names no provider", ErrConfig, key)
	}
	for i, r := range segment {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_' && i > 0 && i < len(segment)-1:
		default:
			return "", fmt.Errorf("%w: %s is not a provider id in A-Z, 0-9 and _", ErrConfig, key)
		}
	}
	return strings.ToLower(strings.ReplaceAll(segment, "_", "-")), nil
}

// envSegment is providerID reversed, for an error message that names the
// variable an operator has to write.
func envSegment(id string) string {
	return strings.ToUpper(strings.ReplaceAll(id, "-", "_"))
}

// shapeNames lists the dialects we speak, in a stable order.
func shapeNames() []string {
	names := make([]string, 0, len(shapeBaseURL))
	for shape := range shapeBaseURL {
		names = append(names, string(shape))
	}
	slices.Sort(names)
	return names
}
