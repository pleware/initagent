package hub

import (
	"context"
	"log"
	"time"

	"github.com/pleware/initagent/internal/brand"
	"github.com/pleware/initagent/internal/mailer"
)

// drainMailOnce claims at most one due row and hands it to the sender.
// A nil sender means hosted mail is not configured yet; rows stay pending.
func drainMailOnce(ctx context.Context, s *Store, sender mailer.Sender, now time.Time) error {
	if sender == nil {
		return nil
	}
	m, err := s.ClaimDueMail(now)
	if err != nil {
		return err
	}
	if m == nil {
		return nil
	}
	providerID, sendErr := sender.Send(ctx, mailer.Message{
		ID:      m.ID,
		Kind:    m.Kind,
		To:      m.To,
		Subject: m.Subject,
		Text:    m.Text,
		HTML:    m.HTML,
	})
	if sendErr != nil {
		if err := s.MarkMailFailed(m.ID, sendErr, now); err != nil {
			return err
		}
		return sendErr
	}
	return s.MarkMailSent(m.ID, providerID, now)
}

func (s *Server) runMailOutbox(ctx context.Context) {
	if s.mail == nil {
		log.Printf("mail outbox: hosted sender unset; queued mail stays pending until %s is set", brand.EnvResendAPIKey)
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	purge := time.NewTicker(time.Hour)
	defer purge.Stop()
	purgeOnce := func() {
		n, err := s.store.PurgeMailOutbox(time.Now())
		if err != nil {
			log.Printf("mail outbox purge: %v", err)
			return
		}
		if n > 0 {
			log.Printf("mail outbox: purged %d rows older than %s", n, mailer.RetainFor)
		}
	}
	purgeOnce()
	for {
		if err := drainMailOnce(ctx, s.store, s.mail, time.Now()); err != nil {
			log.Printf("mail outbox: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-s.mailWake:
		case <-tick.C:
		case <-purge.C:
			purgeOnce()
		}
	}
}

func (s *Server) EnqueueMail(kind, to, subject, text, html string) (*Mail, error) {
	m, err := s.store.EnqueueMail(kind, to, subject, text, html)
	if err != nil {
		return nil, err
	}
	s.kickMail()
	return m, nil
}

func (s *Server) kickMail() {
	if s.mailWake == nil {
		return
	}
	select {
	case s.mailWake <- struct{}{}:
	default:
	}
}
