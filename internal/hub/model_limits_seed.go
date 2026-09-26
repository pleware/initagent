package hub

import "fmt"

// factoryLimit is one default a fresh installation resolves before any admin
// has chosen: the purpose and the runaway ceiling it ships with.
type factoryLimit struct {
	purpose        string
	maxTokens      int
	timeoutSeconds int
}

// factoryLimits is the factory's default set of generation limits: the three
// generative slots get a runaway ceiling out of the box. persona and narrator
// are the short spoken answers (word budget ~120 words, so 512 tokens is a
// ceiling, not a normal limiter); worker is the coder and may answer long.
// The non-generative slots (embedding, stt, vad, tts) carry no limits and are
// not listed here.
func factoryLimits() []factoryLimit {
	return []factoryLimit{
		{purpose: "persona", maxTokens: 512},
		{purpose: "worker", maxTokens: 4096},
		{purpose: "narrator", maxTokens: 512},
	}
}

// seedLimitsMarker records that this installation has had its factory limits
// applied. Its presence is what makes the seed a one-time act.
const seedLimitsMarker = "model_limits_seeded"

// EnsureSeedLimits resolves this installation's generative slots to the
// factory's limits, once — the same shape EnsureSeedAssignments follows. The
// marker is what makes "once" stick: a limit the admin set, cleared or zeroed
// is their decision, not drift, so the seed must not bring it back on the
// next restart. The seed therefore fills only purposes that carry no limit
// row at all, and writes the marker whether or not it filled anything.
//
// The inserts, the fleet bump and the marker commit together, and the bump
// happens only when something was actually inserted.
func (s *Store) EnsureSeedLimits() error {
	applied, err := s.Setting(seedLimitsMarker)
	if err != nil {
		return fmt.Errorf("reading %s: %w", seedLimitsMarker, err)
	}
	if applied != "" {
		return nil
	}

	var pending []ModelLimits
	for _, want := range factoryLimits() {
		purpose, err := ParsePurpose(want.purpose)
		if err != nil {
			return err
		}
		existing, err := s.LimitFor(purpose)
		if err != nil {
			return err
		}
		if existing != nil {
			continue // the admin's choice, not the factory's business
		}
		pending = append(pending, ModelLimits{Purpose: purpose, MaxTokens: want.maxTokens, TimeoutSeconds: want.timeoutSeconds})
	}

	if len(pending) == 0 {
		// Nothing to add, but the installation still counts as seeded.
		return s.SetSetting(seedLimitsMarker, "1")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, l := range pending {
		if _, err := tx.Exec(`INSERT INTO model_limits (purpose, max_tokens, timeout_seconds)
			VALUES (?, ?, ?) ON CONFLICT(purpose) DO NOTHING`, l.Purpose, l.MaxTokens, l.TimeoutSeconds); err != nil {
			return err
		}
	}
	if err := bumpAllBoxes(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, seedLimitsMarker, "1"); err != nil {
		return err
	}
	return tx.Commit()
}
