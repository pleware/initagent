package auth

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/pleware/initagent/internal/offering"
)

func TestNewClaimToken(t *testing.T) {
	first, err := NewClaimToken()
	if err != nil {
		t.Fatalf("NewClaimToken: %v", err)
	}
	if len(first) != ClaimTokenBytes*2 {
		t.Errorf("token is %d hex characters, want %d", len(first), ClaimTokenBytes*2)
	}
	second, err := NewClaimToken()
	if err != nil {
		t.Fatalf("NewClaimToken (second): %v", err)
	}
	if first == second {
		t.Error("two tokens are identical, so a restart would reissue a known secret")
	}
}

func TestNewClaimTokenWithoutEntropy(t *testing.T) {
	failRandom(t)
	if _, err := NewClaimToken(); err == nil {
		t.Fatal("NewClaimToken succeeded with no entropy source")
	}
}

func TestClaimed(t *testing.T) {
	tests := []struct {
		name       string
		accounts   int
		legacyHash string
		want       bool
	}{
		{"fresh hub", 0, "", false},
		{"account exists", 1, "", true},
		{"several accounts", 4, "", true},
		{"upstream password with no account row", 0, "argon2id$c2FsdA$a2V5", true},
		{"both", 2, "argon2id$c2FsdA$a2V5", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Claimed(tc.accounts, tc.legacyHash); got != tc.want {
				t.Errorf("Claimed(%d, %q) = %v, want %v", tc.accounts, tc.legacyHash, got, tc.want)
			}
		})
	}
}

func TestClaimAcceptsFirstOperator(t *testing.T) {
	st := State{Offering: offering.Hosted, ExpectedToken: "a-minted-token"}
	req := ClaimRequest{
		Email:    "  Ops@Example.COM ",
		Password: "correct-horse-battery",
		Token:    "a-minted-token",
	}
	got, err := Claim(st, req)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if got.Email != "ops@example.com" {
		t.Errorf("stored email is %q, want the normalised form", got.Email)
	}
	if got.PasswordHash == "" || got.PasswordHash == req.Password {
		t.Fatalf("stored hash is %q, want an argon2id digest", got.PasswordHash)
	}
	if !VerifyPassword(got.PasswordHash, req.Password) {
		t.Error("the returned hash does not verify the submitted password")
	}
	// A claim with no organization name still has to produce one: the hub
	// creates its first org in the same breath, and an org with an empty name
	// is a blank row in every list that shows it.
	if got.OrgName != DefaultOrgName {
		t.Errorf("org name is %q, want %q", got.OrgName, DefaultOrgName)
	}
	if got.Locale != LocaleEN {
		t.Errorf("locale is %q, want %q when the form omitted it", got.Locale, LocaleEN)
	}
}

func TestClaimKeepsASubmittedOrgName(t *testing.T) {
	st := State{Offering: offering.Selfhost, ExpectedToken: "a-minted-token"}
	got, err := Claim(st, ClaimRequest{
		Email:    "ops@example.com",
		Password: "correct-horse-battery",
		Token:    "a-minted-token",
		OrgName:  "  Example Ops  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.OrgName != "Example Ops" {
		t.Errorf("org name is %q, want the trimmed submission", got.OrgName)
	}
}

func TestClaimKeepsASubmittedLocale(t *testing.T) {
	st := State{Offering: offering.Selfhost, ExpectedToken: "a-minted-token"}
	got, err := Claim(st, ClaimRequest{
		Email:    "ops@example.com",
		Password: "correct-horse-battery",
		Token:    "a-minted-token",
		Locale:   "pl-PL",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Locale != LocalePL {
		t.Errorf("locale is %q, want %q", got.Locale, LocalePL)
	}
}

func TestCheckOrgName(t *testing.T) {
	if got := CheckOrgName("   "); got != DefaultOrgName {
		t.Errorf("a whitespace-only name = %q, want %q", got, DefaultOrgName)
	}
	if got := CheckOrgName("Acme"); got != "Acme" {
		t.Errorf("CheckOrgName(%q) = %q, want it unchanged", "Acme", got)
	}
	if got := CheckOrgName(""); got != DefaultOrgName {
		t.Errorf("empty name = %q, want %q", got, DefaultOrgName)
	}
	// Truncation counts runes, so a long name in a non-Latin script cannot
	// end in half a character.
	long := strings.Repeat("ż", orgNameMax+40)
	got := CheckOrgName(long)
	if utf8.RuneCountInString(got) != orgNameMax {
		t.Errorf("truncated to %d runes, want %d", utf8.RuneCountInString(got), orgNameMax)
	}
	if !utf8.ValidString(got) {
		t.Error("truncation split a character")
	}
}

func TestClaimRejects(t *testing.T) {
	const token = "a-minted-token"
	fresh := State{Offering: offering.Hosted, ExpectedToken: token}

	tests := []struct {
		name    string
		state   State
		req     ClaimRequest
		wantErr error
	}{
		{
			name:    "claimed hub",
			state:   State{Offering: offering.Hosted, Claimed: true, ExpectedToken: token},
			req:     ClaimRequest{Email: "ops@example.com", Password: "correct-horse-battery", Token: token},
			wantErr: ErrAlreadyClaimed,
		},
		{
			name:    "unsupported locale",
			state:   fresh,
			req:     ClaimRequest{Email: "ops@example.com", Password: "correct-horse-battery", Token: token, Locale: "de"},
			wantErr: ErrLocale,
		},
		{
			name:    "wrong token",
			state:   fresh,
			req:     ClaimRequest{Email: "ops@example.com", Password: "correct-horse-battery", Token: "guessed"},
			wantErr: ErrClaimToken,
		},
		{
			name:    "no token submitted",
			state:   fresh,
			req:     ClaimRequest{Email: "ops@example.com", Password: "correct-horse-battery"},
			wantErr: ErrClaimToken,
		},
		{
			name:    "hub minted no token",
			state:   State{Offering: offering.Hosted},
			req:     ClaimRequest{Email: "ops@example.com", Password: "correct-horse-battery"},
			wantErr: ErrClaimToken,
		},
		{
			name:    "hub minted no token and none is submitted either",
			state:   State{Offering: offering.Hosted},
			req:     ClaimRequest{Email: "ops@example.com", Password: "correct-horse-battery", Token: ""},
			wantErr: ErrClaimToken,
		},
		{
			name:    "invalid email",
			state:   fresh,
			req:     ClaimRequest{Email: "root@localhost", Password: "correct-horse-battery", Token: token},
			wantErr: ErrEmailInvalid,
		},
		{
			name:    "password below the hosted floor",
			state:   fresh,
			req:     ClaimRequest{Email: "ops@example.com", Password: "hunter22", Token: token},
			wantErr: ErrPasswordWeak,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Claim(tc.state, tc.req)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Claim error = %v, want %v", err, tc.wantErr)
			}
			if got != (Credentials{}) {
				t.Errorf("rejected claim returned %+v, want the zero value", got)
			}
		})
	}
}

// A claimed hub must answer the same way whatever token arrives, or the
// endpoint becomes an oracle for guessing the token of a hub that has one.
func TestClaimChecksOwnershipBeforeToken(t *testing.T) {
	claimed := State{Offering: offering.Hosted, Claimed: true, ExpectedToken: "a-minted-token"}
	right := ClaimRequest{Email: "ops@example.com", Password: "correct-horse-battery", Token: "a-minted-token"}
	wrong := ClaimRequest{Email: "ops@example.com", Password: "correct-horse-battery", Token: "guessed"}

	_, withRight := Claim(claimed, right)
	_, withWrong := Claim(claimed, wrong)
	if !errors.Is(withRight, ErrAlreadyClaimed) || !errors.Is(withWrong, ErrAlreadyClaimed) {
		t.Fatalf("claimed hub answered %v with the right token and %v with a wrong one", withRight, withWrong)
	}
}

// An unauthenticated caller must not be able to make the hub run argon2id,
// which is expensive by design. A bad token has to lose before the password
// is looked at, so a weak password with a wrong token reports the token.
func TestClaimChecksTokenBeforeHashing(t *testing.T) {
	st := State{Offering: offering.Hosted, ExpectedToken: "a-minted-token"}
	_, err := Claim(st, ClaimRequest{Email: "not-an-email", Password: "x", Token: "guessed"})
	if !errors.Is(err, ErrClaimToken) {
		t.Fatalf("Claim error = %v, want ErrClaimToken to win over the payload checks", err)
	}
}

func TestClaimWithoutEntropy(t *testing.T) {
	failRandom(t)
	st := State{Offering: offering.Hosted, ExpectedToken: "a-minted-token"}
	req := ClaimRequest{Email: "ops@example.com", Password: "correct-horse-battery", Token: "a-minted-token"}
	if _, err := Claim(st, req); err == nil {
		t.Fatal("Claim succeeded with no entropy source, so the stored hash had a fixed salt")
	}
}
