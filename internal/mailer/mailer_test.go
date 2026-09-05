package mailer

import (
	"errors"
	"testing"
	"time"
)

func TestBackoff(t *testing.T) {
	t.Parallel()
	cases := []struct {
		n    int
		want time.Duration
	}{
		{0, 15 * time.Second},
		{1, 15 * time.Second},
		{2, 30 * time.Second},
		{3, time.Minute},
		{8, 32 * time.Minute},
		{20, time.Hour},
	}
	for _, tc := range cases {
		if got := Backoff(tc.n); got != tc.want {
			t.Errorf("Backoff(%d) = %s, want %s", tc.n, got, tc.want)
		}
	}
}

func TestDead(t *testing.T) {
	t.Parallel()
	if Dead(MaxAttempts - 1) {
		t.Fatal("last retry should still be allowed")
	}
	if !Dead(MaxAttempts) {
		t.Fatal("MaxAttempts should be dead")
	}
}

func TestRetainFor(t *testing.T) {
	t.Parallel()
	if RetainFor != 30*24*time.Hour {
		t.Fatalf("RetainFor = %s, want 30 days", RetainFor)
	}
}

func TestNew(t *testing.T) {
	t.Parallel()
	s, err := New(false, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.(Silent); !ok {
		t.Fatalf("self-host without key: %T", s)
	}
	s, err = New(true, "", "")
	if err != nil || s != nil {
		t.Fatalf("hosted without key: sender=%v err=%v", s, err)
	}
	_, err = New(true, "re_test", "")
	if err == nil {
		t.Fatal("key without From should fail")
	}
	s, err = New(true, "re_test", "Init <noreply@example.com>")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.(*Resend); !ok {
		t.Fatalf("hosted with key: %T", s)
	}
}

func TestSilentAndFake(t *testing.T) {
	t.Parallel()
	id, err := Silent{}.Send(t.Context(), Message{To: "a@b.c"})
	if err != nil || id != "" {
		t.Fatalf("Silent: %q %v", id, err)
	}
	f := &Fake{}
	got, err := f.Send(t.Context(), Message{ID: "eml-1", To: "a@b.c"})
	if err != nil || got != "fake-eml-1" || len(f.Sent) != 1 {
		t.Fatalf("Fake: %q %v sent=%d", got, err, len(f.Sent))
	}
	f.Err = errors.New("boom")
	if _, err := f.Send(t.Context(), Message{ID: "eml-2"}); err == nil {
		t.Fatal("Fake should return Err")
	}
}
