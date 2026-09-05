package auth

import (
	"testing"
	"time"
)

func TestNewResetToken(t *testing.T) {
	a, err := NewResetToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewResetToken()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two tokens were identical")
	}
	if len(a) != ResetTokenBytes*2 {
		t.Fatalf("token length %d, want %d hex chars", len(a), ResetTokenBytes*2)
	}
}

func TestNewResetTokenEntropy(t *testing.T) {
	failRandom(t)
	if _, err := NewResetToken(); err == nil {
		t.Fatal("want an error when the random source fails")
	}
}

func TestResetTTL(t *testing.T) {
	if ResetTTL != time.Hour {
		t.Fatalf("ResetTTL = %s, want 1 hour", ResetTTL)
	}
}
