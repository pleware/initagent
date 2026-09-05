package hub

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pleware/initagent/internal/id"
	"github.com/pleware/initagent/internal/mailer"
	"github.com/pleware/initagent/internal/store"
)

const (
	mailPending = "pending"
	mailSending = "sending"
	mailSent    = "sent"
	mailDead    = "dead"
)

// Mail is one outbox row. The body is not a secret, but it is PII; do not
// log it.
type Mail struct {
	ID          string
	Kind        string
	To          string
	Subject     string
	Text        string
	HTML        string
	Status      string
	Attempts    int
	LastError   string
	ProviderID  string
	AvailableAt int64
	ClaimedAt   int64
	CreatedAt   int64
	SentAt      int64
}

// EnqueueMail inserts a pending row ready to send now. kind is a stable
// template name (`password_reset`), not a prefix.
func (s *Store) EnqueueMail(kind, to, subject, text, html string) (*Mail, error) {
	kind = strings.TrimSpace(kind)
	to = strings.TrimSpace(to)
	subject = strings.TrimSpace(subject)
	if kind == "" {
		return nil, fmt.Errorf("mail: empty kind")
	}
	if to == "" {
		return nil, fmt.Errorf("mail: empty To")
	}
	if subject == "" {
		return nil, fmt.Errorf("mail: empty subject")
	}
	if text == "" && html == "" {
		return nil, fmt.Errorf("mail: empty body")
	}
	rowID, err := id.New(id.Mail)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	m := &Mail{
		ID:          rowID,
		Kind:        kind,
		To:          to,
		Subject:     subject,
		Text:        text,
		HTML:        html,
		Status:      mailPending,
		AvailableAt: now,
		CreatedAt:   now,
	}
	_, err = s.db.Exec(`INSERT INTO mail_outbox (
		id, kind, to_addr, subject, text_body, html_body, status,
		attempts, last_error, provider_id, available_at, claimed_at, created_at, sent_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, 0, '', '', ?, 0, ?, 0)`,
		m.ID, m.Kind, m.To, m.Subject, m.Text, m.HTML, m.Status, m.AvailableAt, m.CreatedAt)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// ClaimDueMail takes one due row (pending, or sending that went stale) and
// marks it sending. Nil, nil means the queue is empty.
func (s *Store) ClaimDueMail(now time.Time) (*Mail, error) {
	unix := now.Unix()
	staleBefore := now.Add(-mailer.StaleAfter).Unix()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var m *Mail
	if s.db.Dialect() == store.Postgres {
		m, err = claimDueMailPostgres(tx, unix, staleBefore)
	} else {
		m, err = claimDueMailSQLite(tx, unix, staleBefore)
	}
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return m, nil
}

func claimDueMailSQLite(tx *store.Tx, now, staleBefore int64) (*Mail, error) {
	m, err := scanMail(tx.QueryRow(`UPDATE mail_outbox SET status = ?, claimed_at = ?
		WHERE id = (
			SELECT id FROM mail_outbox
			WHERE (status = ? AND available_at <= ?)
			   OR (status = ? AND claimed_at > 0 AND claimed_at <= ?)
			ORDER BY available_at ASC, id ASC
			LIMIT 1
		)
		RETURNING id, kind, to_addr, subject, text_body, html_body, status, attempts,
			last_error, provider_id, available_at, claimed_at, created_at, sent_at`,
		mailSending, now, mailPending, now, mailSending, staleBefore))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return m, err
}

func claimDueMailPostgres(tx *store.Tx, now, staleBefore int64) (*Mail, error) {
	m, err := scanMail(tx.QueryRow(`
		WITH pick AS (
			SELECT id FROM mail_outbox
			WHERE (status = ? AND available_at <= ?)
			   OR (status = ? AND claimed_at > 0 AND claimed_at <= ?)
			ORDER BY available_at ASC, id ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE mail_outbox
		SET status = ?, claimed_at = ?
		FROM pick
		WHERE mail_outbox.id = pick.id
		RETURNING mail_outbox.id, mail_outbox.kind, mail_outbox.to_addr, mail_outbox.subject,
			mail_outbox.text_body, mail_outbox.html_body, mail_outbox.status, mail_outbox.attempts,
			mail_outbox.last_error, mail_outbox.provider_id, mail_outbox.available_at,
			mail_outbox.claimed_at, mail_outbox.created_at, mail_outbox.sent_at`,
		mailPending, now, mailSending, staleBefore, mailSending, now))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return m, err
}

func scanMail(row *sql.Row) (*Mail, error) {
	var m Mail
	err := row.Scan(
		&m.ID, &m.Kind, &m.To, &m.Subject, &m.Text, &m.HTML, &m.Status, &m.Attempts,
		&m.LastError, &m.ProviderID, &m.AvailableAt, &m.ClaimedAt, &m.CreatedAt, &m.SentAt,
	)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// MarkMailSent records a provider id after a successful send.
func (s *Store) MarkMailSent(id, providerID string, now time.Time) error {
	_, err := s.db.Exec(`UPDATE mail_outbox SET status = ?, provider_id = ?, sent_at = ?, last_error = ''
		WHERE id = ?`, mailSent, providerID, now.Unix(), id)
	return err
}

// MarkMailFailed records a send error. The row goes pending with backoff,
// or dead once MaxAttempts is reached.
func (s *Store) MarkMailFailed(id string, sendErr error, now time.Time) error {
	var attempts int
	err := s.db.QueryRow(`SELECT attempts FROM mail_outbox WHERE id = ?`, id).Scan(&attempts)
	if err != nil {
		return err
	}
	attempts++
	msg := ""
	if sendErr != nil {
		msg = sendErr.Error()
	}
	if len(msg) > 512 {
		msg = msg[:512]
	}
	if mailer.Dead(attempts) {
		_, err = s.db.Exec(`UPDATE mail_outbox SET status = ?, attempts = ?, last_error = ?, available_at = ?
			WHERE id = ?`, mailDead, attempts, msg, now.Unix(), id)
		return err
	}
	next := now.Add(mailer.Backoff(attempts)).Unix()
	_, err = s.db.Exec(`UPDATE mail_outbox SET status = ?, attempts = ?, last_error = ?, available_at = ?, claimed_at = 0
		WHERE id = ?`, mailPending, attempts, msg, next, id)
	return err
}

// MailByID is for tests and operators. Missing row is nil, nil.
func (s *Store) MailByID(id string) (*Mail, error) {
	m, err := scanMail(s.db.QueryRow(`SELECT id, kind, to_addr, subject, text_body, html_body, status, attempts,
		last_error, provider_id, available_at, claimed_at, created_at, sent_at
		FROM mail_outbox WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return m, err
}

// PurgeMailOutbox deletes rows whose created_at is at least RetainFor old.
// Any status. Returns how many rows went.
func (s *Store) PurgeMailOutbox(now time.Time) (int64, error) {
	cutoff := now.Add(-mailer.RetainFor).Unix()
	res, err := s.db.Exec(`DELETE FROM mail_outbox WHERE created_at <= ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
