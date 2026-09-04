package auth

import (
	"errors"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/offering"
)

// failRandom replaces the entropy source for one test so the error paths of
// HashPassword and NewClaimToken are exercised without a broken machine.
func failRandom(t *testing.T) {
	t.Helper()
	original := randomBytes
	randomBytes = func(int) ([]byte, error) { return nil, errors.New("no entropy") }
	t.Cleanup(func() { randomBytes = original })
}

func TestMinPassword(t *testing.T) {
	tests := []struct {
		name string
		kind offering.Kind
		want int
	}{
		{"selfhost keeps the personal-tool floor", offering.Selfhost, MinPasswordSelfhost},
		{"hosted guards a public control plane", offering.Hosted, MinPasswordHosted},
		{"unknown offering gets the stricter floor", offering.Kind("something-new"), MinPasswordHosted},
		{"empty offering gets the stricter floor", offering.Kind(""), MinPasswordHosted},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := MinPassword(tc.kind); got != tc.want {
				t.Errorf("MinPassword(%q) = %d, want %d", tc.kind, got, tc.want)
			}
		})
	}
}

func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain address", "ops@example.com", "ops@example.com"},
		{"trimmed", "  ops@example.com\t", "ops@example.com"},
		{"lowercased", "Ops@Example.COM", "ops@example.com"},
		{"subdomain", "a@mail.corp.example.org", "a@mail.corp.example.org"},
		{"plus tag survives", "ops+hub@example.com", "ops+hub@example.com"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeEmail(tc.in)
			if err != nil {
				t.Fatalf("NormalizeEmail(%q) errored: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("NormalizeEmail(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeEmailRejects(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
		{"no at sign", "ops.example.com"},
		{"no domain", "ops@"},
		{"no local part", "@example.com"},
		{"display name form", "Ops <ops@example.com>"},
		{"domain without a dot", "root@localhost"},
		{"two addresses", "a@example.com, b@example.com"},
		{"longer than 254 characters", strings.Repeat("a", 250) + "@example.com"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeEmail(tc.in)
			if !errors.Is(err, ErrEmailInvalid) {
				t.Fatalf("NormalizeEmail(%q) = (%q, %v), want ErrEmailInvalid", tc.in, got, err)
			}
			if got != "" {
				t.Errorf("rejected input returned %q, want empty", got)
			}
		})
	}
}

func TestCheckPassword(t *testing.T) {
	tests := []struct {
		name     string
		kind     offering.Kind
		email    string
		password string
		wantErr  bool
	}{
		{"selfhost accepts eight", offering.Selfhost, "ops@example.com", "hunter22", false},
		{"selfhost rejects seven", offering.Selfhost, "ops@example.com", "hunter2", true},
		{"hosted rejects eight", offering.Hosted, "ops@example.com", "hunter22", true},
		{"hosted accepts twelve", offering.Hosted, "ops@example.com", "correct-horse", false},
		{"whitespace only is refused at length", offering.Hosted, "ops@example.com", "            ", true},
		{"same as the email", offering.Hosted, "operator@example.com", "operator@example.com", true},
		{"same as the email, different case", offering.Hosted, "operator@example.com", "Operator@Example.com", true},
		{"multibyte counts as runes", offering.Selfhost, "ops@example.com", "ąćęłńóśźż", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckPassword(tc.kind, tc.email, tc.password)
			if tc.wantErr {
				if !errors.Is(err, ErrPasswordWeak) {
					t.Fatalf("CheckPassword(%q) = %v, want ErrPasswordWeak", tc.password, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("CheckPassword(%q) errored: %v", tc.password, err)
			}
		})
	}
}

func TestHashAndVerifyPassword(t *testing.T) {
	const password = "correct-horse-battery"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "argon2id$") {
		t.Fatalf("hash %q does not carry the inherited encoding", hash)
	}
	if !VerifyPassword(hash, password) {
		t.Error("the password it was built from did not verify")
	}
	if VerifyPassword(hash, password+"!") {
		t.Error("a different password verified")
	}

	// Two hashes of one password must differ, or the salt is not random and
	// the digest is a rainbow-table lookup.
	second, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword (second): %v", err)
	}
	if second == hash {
		t.Error("two hashes of the same password are identical, so the salt is fixed")
	}
	if !VerifyPassword(second, password) {
		t.Error("second hash did not verify")
	}
}

func TestHashPasswordWithoutEntropy(t *testing.T) {
	failRandom(t)
	if _, err := HashPassword("correct-horse-battery"); err == nil {
		t.Fatal("HashPassword succeeded with no entropy source; an all-zero salt would look fine")
	}
}

// BurnVerify answers nothing by design; what it must not do is panic or
// depend on stored state, because it runs on the path where no account was
// found.
func TestBurnVerify(t *testing.T) {
	for _, password := range []string{"", "short", "correct-horse-battery"} {
		BurnVerify(password)
	}
}

func TestVerifyPasswordRejectsMalformed(t *testing.T) {
	valid, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	parts := strings.Split(valid, "$")

	tests := []struct {
		name   string
		stored string
	}{
		{"empty", ""},
		{"too few fields", "argon2id$" + parts[1]},
		{"too many fields", valid + "$extra"},
		{"unknown algorithm", "bcrypt$" + parts[1] + "$" + parts[2]},
		{"salt is not base64", "argon2id$not base64$" + parts[2]},
		{"key is not base64", "argon2id$" + parts[1] + "$not base64"},
		{"empty key", "argon2id$" + parts[1] + "$"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if VerifyPassword(tc.stored, "correct-horse-battery") {
				t.Errorf("malformed stored value %q verified", tc.stored)
			}
		})
	}
}

// The parameters are pinned so a hub set up before this package existed keeps
// a verifiable hash. Changing them is a migration, not a tweak.
func TestVerifyPasswordAcceptsInheritedHash(t *testing.T) {
	// Produced with the same argon2id parameters upstream used.
	const password = "hunter22"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if argonTime != 3 || argonMemory != 64*1024 || argonThreads != 2 || argonKeyLen != 32 {
		t.Fatalf("argon2id parameters changed (%d, %d, %d, %d): existing hubs can no longer log in",
			argonTime, argonMemory, argonThreads, argonKeyLen)
	}
	if !VerifyPassword(hash, password) {
		t.Error("hash did not verify under the pinned parameters")
	}
}
