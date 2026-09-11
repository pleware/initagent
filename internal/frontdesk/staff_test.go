package frontdesk

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/desk"
)

// TestTheShippedPairCanBeToldApart is the roster rule reaching the data we
// ship: two people, no form claimed twice, and a display name for each.
func TestTheShippedPairCanBeToldApart(t *testing.T) {
	t.Parallel()

	staff := DefaultStaff()
	if len(staff) != desk.MaxStaff {
		t.Fatalf("DefaultStaff has %d people, want %d", len(staff), desk.MaxStaff)
	}
	if _, err := desk.NewRoster(rosterOf(staff)...); err != nil {
		t.Fatalf("NewRoster: %v", err)
	}
	for _, person := range staff {
		if !strings.HasPrefix(string(person.ID), "psn-") {
			t.Errorf("%s is not a persona id", person.ID)
		}
		if namesOf(staff)[person.ID] != person.Display {
			t.Errorf("%s prints as %q, want %q", person.ID, namesOf(staff)[person.ID], person.Display)
		}
	}
}

// TestSomebodyWithNoDisplayNameIsShownAsTheirID keeps the glass printable when
// the hub starts serving the roster and sends a row we did not write.
func TestSomebodyWithNoDisplayNameIsShownAsTheirID(t *testing.T) {
	t.Parallel()

	names := namesOf([]Person{{ID: "psn-nameless", Names: []string{"nameless"}}})
	if names["psn-nameless"] != "psn-nameless" {
		t.Fatalf("name = %q, want the id", names["psn-nameless"])
	}
}

// TestABriefIsFetchedForEmployedStaffOnly is the placeholder standing in for
// the hub's baseline: a brief for somebody we employ, and a refusal — not an
// empty prompt — for anybody else.
func TestABriefIsFetchedForEmployedStaffOnly(t *testing.T) {
	t.Parallel()

	people := personasOf([]Person{
		{ID: "psn-ania", Display: "Ania", Brief: "You are Ania.", MaxWords: 40},
		{ID: "psn-adam", Display: "Adam", Brief: "You are Adam."},
	})

	persona, err := people.Persona(context.Background(), "psn-ania")
	if err != nil {
		t.Fatalf("Persona: %v", err)
	}
	if persona.Brief != "You are Ania." || persona.MaxWords != 40 {
		t.Fatalf("persona = %+v", persona)
	}

	// An unset word budget is the shipped default rather than no budget: zero
	// would let one answer run for minutes out loud.
	persona, err = people.Persona(context.Background(), "psn-adam")
	if err != nil {
		t.Fatalf("Persona: %v", err)
	}
	if persona.MaxWords != defaultMaxWords {
		t.Fatalf("MaxWords = %d, want %d", persona.MaxWords, defaultMaxWords)
	}

	if _, err := people.Persona(context.Background(), "psn-stranger"); !errors.Is(err, desk.ErrConfig) {
		t.Fatalf("stranger error = %v, want ErrConfig", err)
	}
}
