package desk

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	ania = StaffID("ania")
	adam = StaffID("adam")
)

func testRoster(t *testing.T) Roster {
	t.Helper()
	roster, err := NewRoster(
		Staff{
			ID:        ania,
			Names:     []string{"Ania", "Ani", "Anię", "Anka"},
			Vocatives: []string{"Aniu"},
		},
		Staff{
			ID:        adam,
			Names:     []string{"Adam", "Adama", "Adamowi"},
			Vocatives: []string{"Adamie"},
		},
	)
	if err != nil {
		t.Fatalf("NewRoster: %v", err)
	}
	return roster
}

func testFloor() Floor {
	return NewFloor(adam, time.Date(2026, 9, 11, 9, 0, 0, 0, time.UTC))
}

func TestNewRosterRefusesADeskNobodyCanRoute(t *testing.T) {
	cases := []struct {
		name  string
		staff []Staff
		want  string
	}{
		{
			name: "nobody",
			want: "nobody at it",
		},
		{
			name: "a third staff member",
			staff: []Staff{
				{ID: ania, Names: []string{"Ania"}},
				{ID: adam, Names: []string{"Adam"}},
				{ID: "ewa", Names: []string{"Ewa"}},
			},
			want: "pair is fixed",
		},
		{
			name:  "no id",
			staff: []Staff{{ID: "  ", Names: []string{"Ania"}}},
			want:  "no id",
		},
		{
			name:  "no name",
			staff: []Staff{{ID: ania}},
			want:  "no name to be called by",
		},
		{
			name:  "an empty form",
			staff: []Staff{{ID: ania, Names: []string{"Ania", " "}}},
			want:  "empty form",
		},
		{
			name:  "an empty vocative",
			staff: []Staff{{ID: ania, Names: []string{"Ania"}, Vocatives: []string{""}}},
			want:  "empty form",
		},
		{
			name: "one form, two people",
			staff: []Staff{
				{ID: ania, Names: []string{"Ania", "Ada"}},
				{ID: adam, Names: []string{"Adam", "Ada"}},
			},
			want: "would address both",
		},
		{
			name: "a name that is someone else's vocative",
			staff: []Staff{
				{ID: ania, Names: []string{"Ania"}, Vocatives: []string{"Adamie"}},
				{ID: adam, Names: []string{"Adamie"}},
			},
			want: "would address both",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewRoster(tc.staff...)
			if err == nil {
				t.Fatal("want an error, got a roster")
			}
			if !errors.Is(err, ErrConfig) {
				t.Errorf("want ErrConfig, got %v", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestNewRosterFoldsEveryForm(t *testing.T) {
	roster := testRoster(t)

	staff := roster.Staff()
	if len(staff) != 2 {
		t.Fatalf("want 2 staff, got %d", len(staff))
	}
	if got := staff[0].Names; !slices.Equal(got, []string{"ania", "ani", "anie", "anka"}) {
		t.Errorf("names not folded: %v", got)
	}
	if got := staff[1].Vocatives; !slices.Equal(got, []string{"adamie"}) {
		t.Errorf("vocatives not folded: %v", got)
	}
	if !roster.Has(ania) || !roster.Has(adam) {
		t.Error("the pair should be on the roster")
	}
	if roster.Has("ewa") {
		t.Error("someone who is not at the desk should not be on the roster")
	}
}

// TestResolveAddressingRoutes is the table from DESK-CONVERSATION §3 plus the
// shapes speech adds to it.
func TestResolveAddressingRoutes(t *testing.T) {
	cases := []struct {
		name       string
		text       string
		kind       AddressingKind
		segments   []Segment
		staff      StaffID
		candidates []StaffID
	}{
		{
			name:     "a named request",
			text:     "Ania sprawdź mi to",
			kind:     AddressedDirect,
			segments: []Segment{{Staff: ania, Index: 0, Text: "sprawdź mi to"}},
		},
		{
			name:     "an unnamed request reaches the floor",
			text:     "a teraz to samo dla marca",
			kind:     AddressedCarried,
			segments: []Segment{{Staff: adam, Index: 0, Text: "a teraz to samo dla marca"}},
		},
		{
			name:  "a summon",
			text:  "zawołaj Adama",
			kind:  AddressedSummon,
			staff: adam,
		},
		{
			name: "both, in one breath",
			text: "Adam ty sprawdź to, Ania ty zrób tamto",
			kind: AddressedDirect,
			segments: []Segment{
				{Staff: adam, Index: 0, Text: "sprawdź to"},
				{Staff: ania, Index: 1, Text: "zrób tamto"},
			},
		},
		{
			name:       "a mention is not an address",
			text:       "powiedziałem Ani, że to pilne",
			kind:       AddressedUnclear,
			candidates: []StaffID{ania},
		},
		{
			name: "handing it to somebody else",
			text: "niech to zrobi ktoś inny",
			kind: AddressedUnclear,
		},
		{
			name: "nothing said",
			text: "   \n ",
			kind: AddressedUnclear,
		},
		{
			name:     "a vocative opens",
			text:     "Aniu, sprawdź to jeszcze raz",
			kind:     AddressedDirect,
			segments: []Segment{{Staff: ania, Index: 0, Text: "sprawdź to jeszcze raz"}},
		},
		{
			name:     "a vocative closes",
			text:     "sprawdź to jeszcze raz, Aniu",
			kind:     AddressedDirect,
			segments: []Segment{{Staff: ania, Index: 0, Text: "sprawdź to jeszcze raz"}},
		},
		{
			name:  "her name and nothing else",
			text:  "Ania?",
			kind:  AddressedSummon,
			staff: ania,
		},
		{
			name:       "two names and no request",
			text:       "Adam, Ania",
			kind:       AddressedUnclear,
			candidates: []StaffID{adam, ania},
		},
		{
			name:     "a mention inside a segment is content",
			text:     "Ania sprawdź co mówił Adam",
			kind:     AddressedDirect,
			segments: []Segment{{Staff: ania, Index: 0, Text: "sprawdź co mówił Adam"}},
		},
		{
			name: "a connective starts the second address",
			text: "Adam zrób to a Ania zrób tamto",
			kind: AddressedDirect,
			segments: []Segment{
				{Staff: adam, Index: 0, Text: "zrób to a"},
				{Staff: ania, Index: 1, Text: "zrób tamto"},
			},
		},
		{
			name:     "diacritics dropped by the microphone",
			text:     "anie sprawdz to",
			kind:     AddressedDirect,
			segments: []Segment{{Staff: ania, Index: 0, Text: "sprawdz to"}},
		},
		{
			name:       "summoning both",
			text:       "zawołaj Adama i Anię",
			kind:       AddressedUnclear,
			candidates: []StaffID{adam, ania},
		},
		{
			name: "summoning nobody in particular",
			text: "zawołaj kogoś",
			kind: AddressedUnclear,
		},
		{
			name:       "too many people in one breath",
			text:       "Ania zrób to, Adam zrób tamto, Ania jeszcze to, Adam i to, Aniu koniec",
			kind:       AddressedUnclear,
			candidates: []StaffID{ania, adam},
		},
	}

	roster := testRoster(t)
	floor := testFloor()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveAddressing(tc.text, floor, roster)

			if got.Kind != tc.kind {
				t.Fatalf("kind = %q, want %q (%+v)", got.Kind, tc.kind, got)
			}
			if !slices.Equal(got.Segments, tc.segments) {
				t.Errorf("segments = %+v, want %+v", got.Segments, tc.segments)
			}
			if got.Staff != tc.staff {
				t.Errorf("staff = %q, want %q", got.Staff, tc.staff)
			}
			if !slices.Equal(got.Candidates, tc.candidates) {
				t.Errorf("candidates = %v, want %v", got.Candidates, tc.candidates)
			}
		})
	}
}

// TestResolveAddressingIsTotal is property 1: every outcome is one of the four,
// and unclear never carries a route.
func TestResolveAddressingIsTotal(t *testing.T) {
	roster := testRoster(t)
	floor := testFloor()

	texts := []string{
		"", " ", ",", "?!", "Ania", "ania ania ania", "zawołaj", "zawołaj Adama Adama",
		"Adam", "Adamie", "adamowi to zleciłem", "ktokolwiek", "sprawdź", "1 2 3",
		"Ania, Adam, Ania, Adam", "—", "Ania:", "ty", "Ania ty", "Aniu Adamie",
	}

	for _, text := range texts {
		got := ResolveAddressing(text, floor, roster)
		switch got.Kind {
		case AddressedDirect:
			if len(got.Segments) == 0 {
				t.Errorf("%q: direct with no segments", text)
			}
		case AddressedCarried:
			if len(got.Segments) != 1 || got.Segments[0].Staff != floor.Holder {
				t.Errorf("%q: carried must be one segment held by the floor, got %+v", text, got.Segments)
			}
		case AddressedSummon:
			if !roster.Has(got.Staff) {
				t.Errorf("%q: summoned %q, who is not at the desk", text, got.Staff)
			}
		case AddressedUnclear:
			// An unclear outcome asks a question. Carrying a route would make
			// it a guess wearing a question's name.
			if len(got.Segments) != 0 || got.Staff != "" {
				t.Errorf("%q: unclear carries a route: %+v", text, got)
			}
		default:
			t.Errorf("%q: unknown kind %q", text, got.Kind)
		}
	}
}

// TestFloorMoves is property 2 and the tail of property 3.
func TestFloorMoves(t *testing.T) {
	roster := testRoster(t)
	now := time.Date(2026, 9, 11, 10, 30, 0, 0, time.UTC)

	cases := []struct {
		name    string
		text    string
		holder  StaffID
		reason  FloorReason
		changed bool
	}{
		{
			name:    "a named request moves it",
			text:    "Ania sprawdź to",
			holder:  ania,
			reason:  FloorNamed,
			changed: true,
		},
		{
			name:   "an unnamed request leaves it",
			text:   "a teraz to samo dla marca",
			holder: adam,
			reason: FloorDefault,
		},
		{
			name:    "a summon moves it",
			text:    "zawołaj Anię",
			holder:  ania,
			reason:  FloorSummoned,
			changed: true,
		},
		{
			name:    "the last segment holds it",
			text:    "Adam ty sprawdź to, Ania ty zrób tamto",
			holder:  ania,
			reason:  FloorNamed,
			changed: true,
		},
		{
			name:   "an unclear utterance leaves it",
			text:   "powiedziałem Ani, że to pilne",
			holder: adam,
			reason: FloorDefault,
		},
		{
			name:    "naming the holder still changes the reason",
			text:    "Adam sprawdź to",
			holder:  adam,
			reason:  FloorNamed,
			changed: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			floor := testFloor()
			next, changed := floor.Move(ResolveAddressing(tc.text, floor, roster), now)

			if next.Holder != tc.holder {
				t.Errorf("holder = %q, want %q", next.Holder, tc.holder)
			}
			if next.Reason != tc.reason {
				t.Errorf("reason = %q, want %q", next.Reason, tc.reason)
			}
			if changed != tc.changed {
				t.Errorf("changed = %v, want %v", changed, tc.changed)
			}
			if !changed && !next.Since.Equal(floor.Since) {
				t.Error("an unchanged floor should keep its timestamp")
			}
			if changed && !next.Since.Equal(now) {
				t.Errorf("since = %v, want %v", next.Since, now)
			}
		})
	}
}

func TestFloorMoveIgnoresADirectOutcomeWithNoSegments(t *testing.T) {
	floor := testFloor()

	// Not reachable through ResolveAddressing; the guard is here because a
	// floor that indexed into an empty slice would panic mid-sentence.
	next, changed := floor.Move(Addressing{Kind: AddressedDirect}, time.Now())
	if changed || next != floor {
		t.Errorf("want the floor left alone, got %+v changed=%v", next, changed)
	}
}

func TestFloorMoveIsIdempotentForTheSameReason(t *testing.T) {
	roster := testRoster(t)
	now := time.Date(2026, 9, 11, 11, 0, 0, 0, time.UTC)

	first, moved := testFloor().Move(ResolveAddressing("Ania sprawdź to", testFloor(), roster), now)
	if !moved {
		t.Fatal("the first naming should move the floor")
	}
	later := now.Add(time.Minute)
	second, moved := first.Move(ResolveAddressing("Ania a teraz to", first, roster), later)
	if moved {
		t.Error("naming the holder again is not news for the glass")
	}
	if !second.Since.Equal(now) {
		t.Errorf("since = %v, want the unchanged %v", second.Since, now)
	}
}

func TestTokenizeKeepsPositionsAndBreaks(t *testing.T) {
	words := tokenize("Adam, ty zrób to")

	want := []token{
		{word: "adam", start: 0, end: 4},
		{word: "ty", start: 6, end: 8, afterBreak: true},
		{word: "zrob", start: 9, end: 14},
		{word: "to", start: 15, end: 17},
	}
	if !slices.Equal(words, want) {
		t.Errorf("tokenize = %+v, want %+v", words, want)
	}
}

func TestFoldDropsDiacriticsAndCase(t *testing.T) {
	cases := map[string]string{
		"Zawołaj":              "zawolaj",
		"ĄĆĘŁŃÓŚŹŻ":            "acelnoszz",
		"Anię":                 "anie",
		"  Adamowi  ":          "adamowi",
		"żółty ŚLEDŹ":          "zolty sledz",
		"already folded 123 !": "already folded 123 !",
	}

	for in, want := range cases {
		if got := fold(in); got != want {
			t.Errorf("fold(%q) = %q, want %q", in, got, want)
		}
	}
}
