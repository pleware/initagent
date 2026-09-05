package hub

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/pleware/initagent/internal/id"
	"github.com/pleware/initagent/internal/mailer"
	"github.com/pleware/initagent/internal/store"
)

func TestEnqueueAndSendMail(t *testing.T) {
	s := testStore(t)
	row, err := s.EnqueueMail("password_reset", "a@b.c", "Reset", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	if !id.Is(id.Mail, row.ID) {
		t.Fatalf("id = %q", row.ID)
	}
	got, err := s.ClaimDueMail(time.Now())
	if err != nil || got == nil || got.ID != row.ID || got.Status != mailSending {
		t.Fatalf("claim = %+v, %v", got, err)
	}
	if err := s.MarkMailSent(got.ID, "re_1", time.Now()); err != nil {
		t.Fatal(err)
	}
	saved, err := s.MailByID(got.ID)
	if err != nil || saved == nil || saved.Status != mailSent || saved.ProviderID != "re_1" {
		t.Fatalf("sent = %+v, %v", saved, err)
	}
	if second, err := s.ClaimDueMail(time.Now()); err != nil || second != nil {
		t.Fatalf("empty queue: %v %v", second, err)
	}
}

func TestMailRetryThenDead(t *testing.T) {
	s := testStore(t)
	row, err := s.EnqueueMail("password_reset", "a@b.c", "Reset", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	claimed, err := s.ClaimDueMail(now)
	if err != nil || claimed == nil {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	boom := errors.New("resend 429")
	if err := s.MarkMailFailed(row.ID, boom, now); err != nil {
		t.Fatal(err)
	}
	saved, err := s.MailByID(row.ID)
	if err != nil || saved.Status != mailPending || saved.Attempts != 1 {
		t.Fatalf("after fail = %+v, %v", saved, err)
	}
	if saved.AvailableAt <= now.Unix() {
		t.Fatal("backoff should push available_at into the future")
	}
	if again, err := s.ClaimDueMail(now); err != nil || again != nil {
		t.Fatalf("claimed before backoff elapsed: %v %v", again, err)
	}
	later := now.Add(mailer.Backoff(1) + time.Second)
	if _, err := s.ClaimDueMail(later); err != nil {
		t.Fatal(err)
	}
	for i := 2; i <= mailer.MaxAttempts; i++ {
		if err := s.MarkMailFailed(row.ID, boom, later); err != nil {
			t.Fatal(err)
		}
	}
	dead, err := s.MailByID(row.ID)
	if err != nil || dead.Status != mailDead {
		t.Fatalf("dead = %+v, %v", dead, err)
	}
	if dead.Attempts != mailer.MaxAttempts {
		t.Fatalf("attempts = %d", dead.Attempts)
	}
}

func TestClaimStaleSending(t *testing.T) {
	s := testStore(t)
	row, err := s.EnqueueMail("password_reset", "a@b.c", "Reset", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if _, err := s.ClaimDueMail(now); err != nil {
		t.Fatal(err)
	}
	stale := now.Add(-mailer.StaleAfter - time.Second).Unix()
	if _, err := s.db.Exec(`UPDATE mail_outbox SET claimed_at = ? WHERE id = ?`, stale, row.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.ClaimDueMail(now)
	if err != nil || got == nil || got.ID != row.ID {
		t.Fatalf("stale reclaim = %+v, %v", got, err)
	}
}

func TestEnqueueMailRejectsEmpty(t *testing.T) {
	s := testStore(t)
	if _, err := s.EnqueueMail("", "a@b.c", "S", "t", ""); err == nil {
		t.Fatal("empty kind")
	}
	if _, err := s.EnqueueMail("k", "", "S", "t", ""); err == nil {
		t.Fatal("empty to")
	}
	if _, err := s.EnqueueMail("k", "a@b.c", "", "t", ""); err == nil {
		t.Fatal("empty subject")
	}
	if _, err := s.EnqueueMail("k", "a@b.c", "S", "", ""); err == nil {
		t.Fatal("empty body")
	}
}

func TestOpenStoreCreatesMailOutbox(t *testing.T) {
	s := testStore(t)
	ok, err := s.hasTable("mail_outbox")
	if err != nil || !ok {
		t.Fatalf("mail_outbox missing: ok=%v err=%v", ok, err)
	}
	ok, err = s.hasTable("password_resets")
	if err != nil || !ok {
		t.Fatalf("password_resets missing: ok=%v err=%v", ok, err)
	}
}

func TestDrainOnceSends(t *testing.T) {
	s := testStore(t)
	row, err := s.EnqueueMail("password_reset", "a@b.c", "Reset", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	fake := &mailer.Fake{}
	if err := drainMailOnce(t.Context(), s, fake, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(fake.Sent) != 1 || fake.Sent[0].ID != row.ID {
		t.Fatalf("sent %+v", fake.Sent)
	}
	saved, _ := s.MailByID(row.ID)
	if saved.Status != mailSent {
		t.Fatalf("status %s", saved.Status)
	}
}

func TestPurgeMailOutboxAfterRetainFor(t *testing.T) {
	s := testStore(t)
	keep, err := s.EnqueueMail("password_reset", "keep@b.c", "Keep", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	dropPending, err := s.EnqueueMail("password_reset", "drop@b.c", "Drop", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	dropSent, err := s.EnqueueMail("password_reset", "sent@b.c", "Sent", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := s.MarkMailSent(dropSent.ID, "re_old", now); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-mailer.RetainFor).Unix()
	for _, id := range []string{dropPending.ID, dropSent.ID} {
		if _, err := s.db.Exec(`UPDATE mail_outbox SET created_at = ? WHERE id = ?`, old, id); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.PurgeMailOutbox(now)
	if err != nil || n != 2 {
		t.Fatalf("purged %d, %v", n, err)
	}
	if gone, err := s.MailByID(dropPending.ID); err != nil || gone != nil {
		t.Fatalf("old pending still there: %+v %v", gone, err)
	}
	if gone, err := s.MailByID(dropSent.ID); err != nil || gone != nil {
		t.Fatalf("old sent still there: %+v %v", gone, err)
	}
	if saved, err := s.MailByID(keep.ID); err != nil || saved == nil {
		t.Fatalf("fresh row dropped: %v %v", saved, err)
	}
}

func TestDrainOnceNilSenderLeavesPending(t *testing.T) {
	s := testStore(t)
	row, err := s.EnqueueMail("password_reset", "a@b.c", "Reset", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := drainMailOnce(t.Context(), s, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	saved, _ := s.MailByID(row.ID)
	if saved.Status != mailPending {
		t.Fatalf("status %s", saved.Status)
	}
}

func (s *Store) hasTable(name string) (bool, error) {
	var n int
	var err error
	switch s.db.Dialect() {
	case store.Postgres:
		err = s.db.QueryRow(`SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = ?`, name).Scan(&n)
	default:
		err = s.db.QueryRow(`SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&n)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
