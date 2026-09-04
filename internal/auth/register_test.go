package auth

import (
	"errors"
	"testing"

	"github.com/pleware/initagent/internal/offering"
)

func TestSignupOpen(t *testing.T) {
	tests := []struct {
		name    string
		kind    offering.Kind
		claimed bool
		want    bool
	}{
		{"hosted claimed", offering.Hosted, true, true},
		{"hosted unclaimed", offering.Hosted, false, false},
		{"self-host claimed", offering.Selfhost, true, false},
		{"self-host unclaimed", offering.Selfhost, false, false},
		{"unknown offering claimed", offering.Kind("something-new"), true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SignupOpen(tc.kind, tc.claimed); got != tc.want {
				t.Errorf("SignupOpen(%q, %v) = %v, want %v", tc.kind, tc.claimed, got, tc.want)
			}
		})
	}
}

func TestRegisterAcceptsHostedCustomer(t *testing.T) {
	st := State{Offering: offering.Hosted, Claimed: true}
	got, err := Register(st, RegisterRequest{
		Email:    "  Ada@Example.COM ",
		Password: "correct-horse-battery",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got.Email != "ada@example.com" {
		t.Errorf("stored email is %q, want the normalised form", got.Email)
	}
	if got.PasswordHash == "" || got.PasswordHash == "correct-horse-battery" {
		t.Fatalf("stored hash is %q, want an argon2id digest", got.PasswordHash)
	}
	if !VerifyPassword(got.PasswordHash, "correct-horse-battery") {
		t.Error("the returned hash does not verify the submitted password")
	}
	if got.OrgName != DefaultOrgName {
		t.Errorf("org name is %q, want %q — boarding names the company", got.OrgName, DefaultOrgName)
	}
}

func TestRegisterRejects(t *testing.T) {
	hosted := State{Offering: offering.Hosted, Claimed: true}
	tests := []struct {
		name    string
		state   State
		req     RegisterRequest
		wantErr error
	}{
		{
			name:    "self-host claimed",
			state:   State{Offering: offering.Selfhost, Claimed: true},
			req:     RegisterRequest{Email: "ada@example.com", Password: "correct-horse-battery"},
			wantErr: ErrNotHosted,
		},
		{
			name:    "self-host unclaimed",
			state:   State{Offering: offering.Selfhost},
			req:     RegisterRequest{Email: "ada@example.com", Password: "correct-horse-battery"},
			wantErr: ErrNotHosted,
		},
		{
			name:    "hosted unclaimed",
			state:   State{Offering: offering.Hosted},
			req:     RegisterRequest{Email: "ada@example.com", Password: "correct-horse-battery"},
			wantErr: ErrNotClaimed,
		},
		{
			name:    "invalid email",
			state:   hosted,
			req:     RegisterRequest{Email: "root@localhost", Password: "correct-horse-battery"},
			wantErr: ErrEmailInvalid,
		},
		{
			name:    "password below the hosted floor",
			state:   hosted,
			req:     RegisterRequest{Email: "ada@example.com", Password: "hunter22"},
			wantErr: ErrPasswordWeak,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Register(tc.state, tc.req)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Register error = %v, want %v", err, tc.wantErr)
			}
			if got != (Credentials{}) {
				t.Errorf("rejected register returned %+v, want the zero value", got)
			}
		})
	}
}

// A closed door must not run argon2id. Self-host with a weak password still
// reports the offering, not the payload.
func TestRegisterChecksOfferingBeforeHashing(t *testing.T) {
	_, err := Register(State{Offering: offering.Selfhost, Claimed: true}, RegisterRequest{
		Email: "not-an-email", Password: "x",
	})
	if !errors.Is(err, ErrNotHosted) {
		t.Fatalf("Register error = %v, want ErrNotHosted to win over the payload checks", err)
	}
}

func TestRegisterWithoutEntropy(t *testing.T) {
	failRandom(t)
	st := State{Offering: offering.Hosted, Claimed: true}
	req := RegisterRequest{Email: "ada@example.com", Password: "correct-horse-battery"}
	if _, err := Register(st, req); err == nil {
		t.Fatal("Register succeeded with no entropy source, so the stored hash had a fixed salt")
	}
}
