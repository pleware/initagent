package gdeskfront

import (
	"context"
	"fmt"

	"github.com/pleware/initagent/internal/gdesk"
)

// defaultMaxWords bounds one spoken reply. About forty seconds out loud, which
// is where an answer stops being an answer and becomes a lecture nobody can
// interrupt.
const defaultMaxWords = 120

// Person is one staff member as this box holds her: the forms she is addressed
// by, how the glass writes her name, and the brief a model reads before it
// speaks as her.
//
// A struct here rather than gdesk.Staff plus two maps, because these four
// things arrive together — from the hub, once the roster is a row — and
// splitting them at the assembly is what lets a name and a brief drift apart.
type Person struct {
	ID gdesk.StaffID

	// Display is her name as the glass prints it above a reply.
	Display string

	// Names are plain forms; Vocatives are the address-only ones. Both are
	// data because Polish declines names and a generator that guesses wrong
	// routes work to the wrong person silently (gdesk.Staff).
	Names     []string
	Vocatives []string

	// Brief is who she is, already prose. It stands in for the hub's Big Five
	// baseline plus the mood offset, which is why this package renders nothing:
	// when the hub serves it, this field is filled from there instead
	// (workspace docs/GDESK-CONVERSATION.md §10).
	Brief string

	// MaxWords bounds one reply. Zero means defaultMaxWords.
	MaxWords int
}

// DefaultStaff is the pair the product ships with.
//
// Two people, fixed at gdesk.MaxStaff, and both are ours: talking to the desk
// must not feel like talking to a service. They are personas, so the id is
// `psn-` (workspace drafts/05).
func DefaultStaff() []Person {
	return []Person{
		{
			ID:        "psn-ania",
			Display:   "Ania",
			Names:     []string{"ania", "anka"},
			Vocatives: []string{"aniu", "anko"},
			Brief: "You are Ania at the front desk. You are warm, direct and " +
				"quick, and you would rather ask one short question than guess. " +
				"Answer in the language the person used, in a few sentences, as " +
				"speech rather than a document: no headings, no bullet lists, no " +
				"code unless it was asked for.",
		},
		{
			ID:        "psn-adam",
			Display:   "Adam",
			Names:     []string{"adam"},
			Vocatives: []string{"adamie"},
			Brief: "You are Adam at the front desk. You are calm, precise and " +
				"sparing with words, and you say plainly when something will not " +
				"work. Answer in the language the person used, in a few sentences, " +
				"as speech rather than a document: no headings, no bullet lists, " +
				"no code unless it was asked for.",
		},
	}
}

// rosterOf is the addressing half of the staff.
func rosterOf(staff []Person) []gdesk.Staff {
	out := make([]gdesk.Staff, 0, len(staff))
	for _, person := range staff {
		out = append(out, gdesk.Staff{
			ID:        person.ID,
			Names:     person.Names,
			Vocatives: person.Vocatives,
		})
	}
	return out
}

// namesOf is the display half, which the seam writes into events.
func namesOf(staff []Person) map[gdesk.StaffID]string {
	out := make(map[gdesk.StaffID]string, len(staff))
	for _, person := range staff {
		name := person.Display
		if name == "" {
			name = string(person.ID)
		}
		out[person.ID] = name
	}
	return out
}

// personas answers who is speaking from what this box was told.
//
// Static on purpose: mood decay and the Big Five baseline live where the
// personality does, and inventing a second source of it here would make the
// hub's copy advisory.
type personas map[gdesk.StaffID]gdesk.Persona

func personasOf(staff []Person) personas {
	out := make(personas, len(staff))
	for _, person := range staff {
		words := person.MaxWords
		if words <= 0 {
			words = defaultMaxWords
		}
		out[person.ID] = gdesk.Persona{Brief: person.Brief, MaxWords: words}
	}
	return out
}

// Persona implements gdesk.Personas.
func (p personas) Persona(_ context.Context, staff gdesk.StaffID) (gdesk.Persona, error) {
	persona, ok := p[staff]
	if !ok {
		return gdesk.Persona{}, fmt.Errorf("%w: nobody at this desk is %q", gdesk.ErrConfig, staff)
	}
	return persona, nil
}
