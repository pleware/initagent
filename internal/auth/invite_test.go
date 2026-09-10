package auth

import (
	"testing"
	"time"
)

func TestNewInviteToken(t *testing.T) {
	a, err := NewInviteToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewInviteToken()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two tokens were identical")
	}
	if len(a) != InviteTokenBytes*2 {
		t.Fatalf("token length %d, want %d hex chars", len(a), InviteTokenBytes*2)
	}
}

func TestNewInviteTokenEntropy(t *testing.T) {
	failRandom(t)
	if _, err := NewInviteToken(); err == nil {
		t.Fatal("want an error when the random source fails")
	}
}

func TestInviteTTL(t *testing.T) {
	if InviteTTL != 7*24*time.Hour {
		t.Fatalf("InviteTTL = %s, want 7 days", InviteTTL)
	}
	if InviteTTL <= ResetTTL {
		t.Fatal("an invite must outlive a password reset")
	}
}
