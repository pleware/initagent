package hub

import (
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/pleware/initagent/internal/auth"
	"github.com/pleware/initagent/internal/brand"
)

// bootstrapClaim holds the one-time token that proves the operator while the
// hub has no owner (draft 26). The token lives in memory for one process
// lifetime and is mirrored into the data directory for an operator who reads
// files rather than logs.
//
// It is not stored in the database on purpose. An unclaimed hub mints a fresh
// token on every start, which is also the whole recovery story: lose it,
// restart, read the new one. There is nothing to reset and no support path.
type bootstrapClaim struct {
	mu    sync.Mutex
	token string
	path  string
}

// newBootstrapClaim mints a token for an unclaimed hub and publishes it. A
// claimed hub gets an empty claim and any leftover token file removed, so a
// token from an earlier unclaimed run cannot linger on disk.
func newBootstrapClaim(dataDir string, claimed bool) (*bootstrapClaim, error) {
	c := &bootstrapClaim{path: filepath.Join(dataDir, brand.ClaimTokenFile)}
	if claimed {
		os.Remove(c.path)
		return c, nil
	}
	token, err := auth.NewClaimToken()
	if err != nil {
		return nil, err
	}
	c.token = token
	// Writing the token is best effort: an operator who can read the log has
	// everything they need, so a read-only data directory should not stop the
	// hub from starting.
	if err := os.WriteFile(c.path, []byte(token+"\n"), 0o600); err != nil {
		log.Printf("first-run: could not write %s: %v", c.path, err)
	}
	log.Printf("first-run: this hub has no owner yet. Claim it with this one-time token: %s", token)
	log.Printf("first-run: the same token is in %s and is removed once the hub is claimed", c.path)
	return c, nil
}

// expected returns the token a claim must match. Empty means this hub cannot
// be claimed right now, which auth.Claim treats as a refusal.
func (c *bootstrapClaim) expected() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.token
}

// consume retires the token after a successful claim.
func (c *bootstrapClaim) consume() {
	c.mu.Lock()
	c.token = ""
	c.mu.Unlock()
	if err := os.Remove(c.path); err != nil && !os.IsNotExist(err) {
		log.Printf("first-run: could not remove %s: %v", c.path, err)
	}
}

// claimed reports whether this hub already has an owner: an account row, or
// upstream's anonymous password setting on a hub that was set up before
// accounts existed.
func (s *Server) claimed() (bool, error) {
	accounts, err := s.store.CountAccounts()
	if err != nil {
		return false, err
	}
	legacy, err := s.store.Setting(legacyPasswordSetting)
	if err != nil {
		return false, err
	}
	return auth.Claimed(accounts, legacy), nil
}

// legacyPasswordSetting is upstream's single anonymous admin password. It is
// read for login and for the claimed check, and never written again: new
// hubs mint an account instead (`08`).
const legacyPasswordSetting = "password_hash"
