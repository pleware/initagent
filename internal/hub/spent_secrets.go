package hub

import (
	"context"
	"log"
	"time"

	"github.com/pleware/initagent/internal/auth"
)

func (s *Server) runSpentSecretPurge(ctx context.Context) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	purgeOnce := func() {
		now := time.Now()
		n, err := s.store.PurgePasswordResets(now)
		if err != nil {
			log.Printf("password reset purge: %v", err)
		} else if n > 0 {
			log.Printf("password resets: purged %d spent or expired rows older than %s", n, auth.SpentRetainFor)
		}
		n, err = s.store.PurgeEnrollTokens(now)
		if err != nil {
			log.Printf("enroll token purge: %v", err)
		} else if n > 0 {
			log.Printf("enroll tokens: purged %d spent or expired rows older than %s", n, auth.SpentRetainFor)
		}
	}
	purgeOnce()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			purgeOnce()
		}
	}
}
