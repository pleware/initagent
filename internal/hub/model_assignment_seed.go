package hub

import "fmt"

// factoryAssignment is one default a fresh installation resolves before any
// admin has chosen: the purpose, and the factory pin that answers it.
type factoryAssignment struct {
	purpose string
	modelID string
}

// factoryAssignments is the factory's default set — what a box bound to a
// brand-new installation resolves without anybody opening the admin.
//
// It is deliberately short and it names pins the factory vouches for: a pin
// without a digest is not assignable, so it cannot appear here. Every purpose
// not listed stays unresolved until an admin assigns one; adding a line is the
// whole change, because EnsureSeedAssignments applies whatever this returns.
//
// persona is Qwen3.6-35B-A3B. It answers a role that reasons, codes and calls
// tools rather than the small dense pin that used to be the only option, and
// it is a MoE the box serves at 50.9 t/s on one 16 GB card with the tuning the
// CLI generates for it (`-ngl 99 -ncmoe 20 -c 32768`) — 13.6 t/s without it,
// which is why the tuning ships beside this default.
func factoryAssignments() []factoryAssignment {
	return []factoryAssignment{
		{purpose: "persona", modelID: "qwen3.6-35b-a3b-q4_k_m"},
	}
}

// seedAssignmentsMarker records that this installation has had its factory
// defaults applied. Its presence is what makes the seed a one-time act.
const seedAssignmentsMarker = "model_assignments_seeded"

// EnsureSeedAssignments resolves this installation's purposes to the factory's
// picks, once.
//
// "Once" is the whole of the semantics, and it is why this is not shaped like
// EnsureSeedModels. The model pins are factory-owned: a drifted pin is written
// back to the factory definition on every open, because an admin customizes
// through the assignment and override layers rather than by editing the pin.
// model_assignments *is* the admin's layer, so a seed that enforced it would
// take the table away from the admin — a purpose they cleared would come back
// on the next restart, and a purpose they set would be overwritten. This seed
// therefore fills only purposes that carry no assignment at all, and writes
// seedAssignmentsMarker whether or not it filled anything. From that point on
// the layer belongs to the admin, including the decision to leave a purpose
// unresolved: a cleared purpose is a decision, not drift.
//
// A factory assignment whose pin is missing, unverified or of another purpose
// is skipped rather than fatal. The pin seed runs first and restores what the
// factory vouches for, and a default that cannot be honoured must not keep the
// hub from opening.
//
// The inserts, the fleet bump and the marker commit together, and the bump
// happens only when something was actually inserted — the shape
// EnsureSeedModels uses. A purpose that already carries an assignment adds
// nothing, so it bumps nothing.
func (s *Store) EnsureSeedAssignments() error {
	applied, err := s.Setting(seedAssignmentsMarker)
	if err != nil {
		return fmt.Errorf("reading %s: %w", seedAssignmentsMarker, err)
	}
	if applied != "" {
		return nil
	}

	pending, err := s.pendingFactoryAssignments()
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		// Nothing to add, but the installation still counts as seeded: a
		// fresh store whose pins are not in place yet must not have its
		// factory defaults applied later, on top of an admin's choices.
		return s.SetSetting(seedAssignmentsMarker, "1")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, a := range pending {
		if _, err := tx.Exec(`INSERT INTO model_assignments (purpose, model_id)
			VALUES (?, ?) ON CONFLICT(purpose) DO NOTHING`, a.Purpose, a.ModelID); err != nil {
			return err
		}
	}
	if err := bumpAllBoxes(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, seedAssignmentsMarker, "1"); err != nil {
		return err
	}
	return tx.Commit()
}

// pendingFactoryAssignments reads the factory defaults this installation has
// not answered yet: a purpose an admin has already assigned is their choice
// and is left out, and so is a default whose pin the store cannot yet honour.
// The reads happen before the caller opens its transaction, so the write path
// stays write-only.
func (s *Store) pendingFactoryAssignments() ([]ModelAssignment, error) {
	var pending []ModelAssignment
	for _, want := range factoryAssignments() {
		purpose, err := ParsePurpose(want.purpose)
		if err != nil {
			return nil, err
		}
		existing, err := s.AssignmentFor(purpose)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			continue // the admin's choice, not the factory's business
		}
		m, err := s.GetModel(want.modelID)
		if err != nil {
			return nil, err
		}
		if m == nil || m.Purpose != purpose || m.Digest == "" {
			continue
		}
		pending = append(pending, ModelAssignment{Purpose: purpose, ModelID: m.ID})
	}
	return pending, nil
}
