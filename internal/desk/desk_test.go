package desk

import (
	"slices"
	"testing"
)

// TestExportedIdentity locks the strings that travel: a role name appears in
// an operator's environment and in a failure the desk says out loud, and a
// shape name is the dialect a provider entry claims. Renaming one silently
// would leave a working configuration unreadable.
func TestExportedIdentity(t *testing.T) {
	t.Parallel()
	cases := []struct {
		got  string
		want string
	}{
		{string(RoleChat), "desk.chat"},
		{string(RoleSTT), "desk.stt"},
		{string(RoleTTS), "desk.tts"},
		{string(ShapeOpenAI), "openai"},
		{string(ShapeAnthropic), "anthropic"},
		{string(LineFromPerson), "person"},
		{string(LineFromStaff), "staff"},
		{string(AudioPCM16), "pcm16"},
		{string(AudioWAV), "wav"},
		{string(AudioMP3), "mp3"},
		{string(SilenceUnbound), "unbound"},
		{string(SilenceNoKey), "no-key"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("identity string = %q, want %q", tc.got, tc.want)
		}
	}
}

// TestRolesOrderIsFixed keeps reported configuration stable. Silences are
// listed in this order, so a map would make the same box print differently on
// two runs.
func TestRolesOrderIsFixed(t *testing.T) {
	t.Parallel()
	if want := []Role{RoleChat, RoleSTT, RoleTTS}; !slices.Equal(Roles, want) {
		t.Fatalf("Roles = %v, want %v", Roles, want)
	}
}

// TestEveryRoleHasAVariable stops a fourth role arriving with no way to bind
// it, which would look like a desk that ignores its configuration.
func TestEveryRoleHasAVariable(t *testing.T) {
	t.Parallel()
	for _, role := range Roles {
		if roleEnv[role] == "" {
			t.Errorf("role %q binds to no environment variable", role)
		}
	}
	if len(roleEnv) != len(Roles) {
		t.Errorf("roleEnv has %d entries for %d roles", len(roleEnv), len(Roles))
	}
}

// TestEveryShapeHasABaseURL keeps a new dialect from being accepted by
// LoadConfig while having nowhere to call.
func TestEveryShapeHasABaseURL(t *testing.T) {
	t.Parallel()
	for _, shape := range []Shape{ShapeOpenAI, ShapeAnthropic} {
		if shapeBaseURL[shape] == "" {
			t.Errorf("shape %q has no default base URL", shape)
		}
	}
}
