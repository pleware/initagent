package gdesk

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
)

// Limits are the desk's bounds, spoken rather than implied
// (GDESK-CONVERSATION §5). An unbounded desk promises everything and delivers
// a queue nobody can see.
const (
	// MaxStaff is the fixed pair (`53`). A roster turns addressing into
	// disambiguation against N names, which is a different problem.
	MaxStaff = 2

	// MaxSegmentsPerUtterance bounds how many people one breath may address.
	MaxSegmentsPerUtterance = 4

	// MaxBackgroundPerStaff is where new work becomes queued instead.
	MaxBackgroundPerStaff = 6

	// MaxQueuedPerStaff is where she refuses out loud.
	MaxQueuedPerStaff = 12

	// MaxUtteranceChars bounds one utterance.
	MaxUtteranceChars = 2_000

	// MaxOpenCommitments bounds the whole desk.
	MaxOpenCommitments = 24
)

// StaffID names one staff member.
//
// A distinct type is the brand: it stops an account- or a persona- being
// passed where a staff member belongs.
//
// The format is settled: `staff-<uuidv7>`, registered as initagent.hub.staff in
// internal/id. The home is the hub rather than this package's context because
// the being has outgrown the desk — greeting at a glass is one of his posts,
// not who he is — and a context is a vocabulary domain, not a storage
// location. The Go names here keep the word "staff", which survived the
// decision; only the qualified name and the prefix changed.
type StaffID string

// Staff is one addressable person at the desk.
//
// The forms are data rather than grammar. Polish declines names, and a
// generator that guesses wrong routes work to the wrong person silently;
// these arrive from the hub with the rest of who she is, next to the Big Five
// baseline and the voice.
type Staff struct {
	ID StaffID

	// Names are plain forms ("ania", "anka", "adam"). They address only where
	// a segment starts, because "powiedziałem Ani" mentions her.
	Names []string

	// Vocatives are the forms Polish uses for nothing but addressing
	// ("aniu", "adamie"), so they address wherever they appear. That
	// grammatical fact settles the case lexical rules otherwise cannot.
	Vocatives []string
}

// Roster is who is at the desk.
type Roster struct {
	staff     []Staff
	names     map[string]StaffID
	vocatives map[string]StaffID
}

// NewRoster folds every form once, so resolution does no normalising and two
// staff cannot share a form.
func NewRoster(staff ...Staff) (Roster, error) {
	if len(staff) == 0 {
		return Roster{}, fmt.Errorf("%w: a desk with nobody at it", ErrConfig)
	}
	if len(staff) > MaxStaff {
		return Roster{}, fmt.Errorf("%w: %d staff, and the pair is fixed at %d", ErrConfig, len(staff), MaxStaff)
	}
	r := Roster{
		staff:     make([]Staff, 0, len(staff)),
		names:     map[string]StaffID{},
		vocatives: map[string]StaffID{},
	}
	for _, member := range staff {
		if strings.TrimSpace(string(member.ID)) == "" {
			return Roster{}, fmt.Errorf("%w: a staff member with no id", ErrConfig)
		}
		if len(member.Names) == 0 {
			return Roster{}, fmt.Errorf("%w: staff %q has no name to be called by", ErrConfig, member.ID)
		}
		folded := Staff{ID: member.ID}
		for _, form := range member.Names {
			word, err := r.claim(form, member.ID, r.names)
			if err != nil {
				return Roster{}, err
			}
			folded.Names = append(folded.Names, word)
		}
		for _, form := range member.Vocatives {
			word, err := r.claim(form, member.ID, r.vocatives)
			if err != nil {
				return Roster{}, err
			}
			folded.Vocatives = append(folded.Vocatives, word)
		}
		r.staff = append(r.staff, folded)
	}
	return r, nil
}

// claim records one folded form against one staff member.
//
// One form, one person. Two people answering to the same word is an utterance
// nobody can route, and discovering that at resolution time means discovering
// it mid-sentence.
func (r Roster) claim(form string, id StaffID, into map[string]StaffID) (string, error) {
	word := fold(form)
	if word == "" {
		return "", fmt.Errorf("%w: staff %q has an empty form", ErrConfig, id)
	}
	for _, index := range []map[string]StaffID{r.names, r.vocatives} {
		if owner, taken := index[word]; taken && owner != id {
			return "", fmt.Errorf("%w: %q would address both %s and %s", ErrConfig, word, owner, id)
		}
	}
	into[word] = id
	return word, nil
}

// Staff lists the roster in the order it was built.
func (r Roster) Staff() []Staff {
	return slices.Clone(r.staff)
}

// Has reports whether an id is on this roster.
func (r Roster) Has(id StaffID) bool {
	return slices.ContainsFunc(r.staff, func(s Staff) bool { return s.ID == id })
}

// lookup resolves one folded word to whoever answers to it, in either case.
func (r Roster) lookup(word string) (StaffID, bool) {
	if id, ok := r.names[word]; ok {
		return id, true
	}
	id, ok := r.vocatives[word]
	return id, ok
}

// FloorReason says why the current holder has the floor.
type FloorReason string

const (
	// FloorDefault is how a box opens the day. The floor is never empty: an
	// empty one makes the first unnamed utterance a guess or a refusal, and
	// both are worse than a documented default.
	FloorDefault FloorReason = "default"

	// FloorNamed means the last utterance addressed them.
	FloorNamed FloorReason = "named"

	// FloorSummoned means someone called them over and asked nothing.
	FloorSummoned FloorReason = "summoned"
)

// Floor is who hears the next sentence. It is connector state rather than a
// line in a prompt, and it is a fact so the glass can show it — a hidden
// addressee is the voice equivalent of losing keyboard focus.
type Floor struct {
	Holder StaffID
	Since  time.Time
	Reason FloorReason
}

// NewFloor opens the day with a configured holder.
func NewFloor(holder StaffID, now time.Time) Floor {
	return Floor{Holder: holder, Since: now, Reason: FloorDefault}
}

// AddressingKind is which of the four outcomes resolution reached.
type AddressingKind string

const (
	// AddressedDirect named one or more people; Segments says who and what.
	AddressedDirect AddressingKind = "direct"

	// AddressedCarried named nobody, so it reaches the floor holder.
	AddressedCarried AddressingKind = "carried"

	// AddressedSummon moves the floor and carries no work.
	AddressedSummon AddressingKind = "summon"

	// AddressedUnclear is a question, not a guess: nothing is minted, no
	// commitment is accepted, and no model is called with an invented
	// addressee. Work booked to the wrong person is discovered late, by the
	// person who did not do it.
	AddressedUnclear AddressingKind = "unclear"
)

// Segment is the part of an utterance addressed to one person.
type Segment struct {
	Staff StaffID
	Index int
	Text  string
}

// Addressing is the outcome of resolution. Which fields carry meaning depends
// on Kind, which is the closest Go gets to the seam's tagged union:
//
//	direct   Segments, one per addressee, in order
//	carried  Segments, exactly one, held by the floor
//	summon   Staff
//	unclear  Candidates, possibly empty when nobody was named
type Addressing struct {
	Kind       AddressingKind
	Segments   []Segment
	Staff      StaffID
	Candidates []StaffID
}

// summonVerbs ask for a person and nothing else. Folded, so "zawołaj" is
// matched as "zawolaj" — speech to text drops diacritics often enough that
// requiring them would make the feature depend on the microphone.
var summonVerbs = map[string]bool{
	"zawolaj":     true,
	"zawolajcie":  true,
	"przywolaj":   true,
	"wolaj":       true,
	"popros":      true,
	"poprosze":    true,
	"przelacz":    true,
	"przelaczcie": true,
}

// connectives join two addresses in one breath, so a name after one starts a
// segment: "Adam sprawdź to, a Ania zrób tamto".
var connectives = map[string]bool{
	"a":         true,
	"i":         true,
	"oraz":      true,
	"natomiast": true,
	"potem":     true,
}

// deferrals hand the work away from the pair without naming anyone. They are
// unclear rather than carried: "niech to zrobi ktoś inny" is not an
// instruction to the person holding the floor.
var deferrals = []string{
	"ktos inny",
	"kto inny",
	"ktokolwiek",
	"komus innemu",
	"kogos innego",
}

// ResolveAddressing decides who was spoken to.
//
// It is pure and total over the text, the floor and the roster, which is the
// point: let a model decide the addressee and routing stops being
// deterministic, stops being testable without a model, and starts changing
// when a provider ships a new snapshot. A model may still *propose* for an
// unclear outcome — what comes back is validated against this same closed
// vocabulary and routed by this same function.
func ResolveAddressing(text string, floor Floor, roster Roster) Addressing {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		// Nothing was said, so nobody was addressed. The loop refuses an
		// empty utterance before this; the resolver still answers.
		return Addressing{Kind: AddressedUnclear}
	}

	words := tokenize(trimmed)

	// A summon is read first, because "zawołaj Adama i Anię" would otherwise
	// look like a list of addresses and route the calling to one of them.
	if summoned, ok := findSummon(words, roster); ok {
		return summoned
	}

	addresses, mentions := findAddresses(words, roster)
	if len(addresses) > 0 {
		return direct(trimmed, words, addresses)
	}
	if folded := fold(trimmed); slices.ContainsFunc(deferrals, func(marker string) bool {
		return strings.Contains(folded, marker)
	}) {
		return Addressing{Kind: AddressedUnclear}
	}
	if len(mentions) > 0 {
		// Hearing your own name is not being addressed. Mention detection is
		// lexical and will produce false positives, so they cost a question
		// rather than a route.
		return Addressing{Kind: AddressedUnclear, Candidates: mentions}
	}
	return Addressing{Kind: AddressedCarried, Segments: []Segment{{
		Staff: floor.Holder,
		Index: 0,
		Text:  trimmed,
	}}}
}

// address is one place in the utterance where a person was addressed.
type address struct {
	staff StaffID
	token int
}

// findAddresses separates addresses from mere mentions.
func findAddresses(words []token, roster Roster) ([]address, []StaffID) {
	var addresses []address
	var mentions []StaffID
	for i, word := range words {
		if id, ok := roster.vocatives[word.word]; ok {
			addresses = append(addresses, address{staff: id, token: i})
			continue
		}
		id, ok := roster.names[word.word]
		if !ok {
			continue
		}
		// A connective continues a list of addresses; it cannot open one.
		// "Adam zrób to a Ania zrób tamto" has two addressees, while
		// "powiedziałem Ani i Adamowi" has none.
		starts := i == 0 || word.afterBreak ||
			(connectives[words[i-1].word] && len(addresses) > 0)
		if starts {
			addresses = append(addresses, address{staff: id, token: i})
			continue
		}
		if !slices.Contains(mentions, id) {
			mentions = append(mentions, id)
		}
	}
	return addresses, mentions
}

// direct builds the segments for an utterance that named someone.
func direct(text string, words []token, addresses []address) Addressing {
	var segments []Segment
	for i, at := range addresses {
		from := words[at.token].end
		// "Adam ty sprawdź to" — the pronoun belongs to the address, not to
		// what was asked.
		if next := at.token + 1; next < len(words) && words[next].word == "ty" {
			from = words[next].end
		}
		to := len(text)
		if i+1 < len(addresses) {
			to = words[addresses[i+1].token].start
		}
		said := trimSegment(text[from:to])
		if said == "" {
			continue
		}
		segments = append(segments, Segment{Staff: at.staff, Index: len(segments), Text: said})
	}

	switch {
	case len(segments) == 0 && len(addresses) == 1:
		// A name at the end still addresses: "sprawdź to, Aniu" asks before it
		// says who. Only when one person was named — with two, the words
		// before the first name belong to whoever held the floor, and that is
		// a guess.
		if said := trimSegment(text[:words[addresses[0].token].start]); said != "" {
			return Addressing{Kind: AddressedDirect, Segments: []Segment{{
				Staff: addresses[0].staff,
				Index: 0,
				Text:  said,
			}}}
		}
		// Her name and nothing else is her attention, which is what a summon
		// is.
		return Addressing{Kind: AddressedSummon, Staff: addresses[0].staff}
	case len(segments) == 0:
		// Two names and no request leaves the floor ambiguous, and the floor
		// holds one person.
		return Addressing{Kind: AddressedUnclear, Candidates: addressed(addresses)}
	case len(segments) > MaxSegmentsPerUtterance:
		return Addressing{Kind: AddressedUnclear, Candidates: addressed(addresses)}
	}
	return Addressing{Kind: AddressedDirect, Segments: segments}
}

// findSummon reads "zawołaj Adama" — a verb that asks for a person and
// nothing else.
//
// Only when the utterance opens with the verb. "Ania zawołaj Adama" is Ania
// being asked to do something, and that is a request, not an addressing
// change.
func findSummon(words []token, roster Roster) (Addressing, bool) {
	if len(words) == 0 || !summonVerbs[words[0].word] {
		return Addressing{}, false
	}
	var named []StaffID
	for _, word := range words[1:] {
		id, ok := roster.lookup(word.word)
		if ok && !slices.Contains(named, id) {
			named = append(named, id)
		}
	}
	if len(named) == 1 {
		return Addressing{Kind: AddressedSummon, Staff: named[0]}, true
	}
	// Summoning nobody, or both at once: the floor holds one person, so this
	// is a question rather than a route.
	return Addressing{Kind: AddressedUnclear, Candidates: named}, true
}

// trimSegment drops the punctuation that separated a name from what was asked.
func trimSegment(s string) string {
	return strings.Trim(s, " \t\r\n,;:.!?-—")
}

// addressed lists the people an utterance named, once each, in order.
func addressed(addresses []address) []StaffID {
	var ids []StaffID
	for _, at := range addresses {
		if !slices.Contains(ids, at.staff) {
			ids = append(ids, at.staff)
		}
	}
	return ids
}

// Move returns the floor after an outcome, and whether the glass has
// something new to show.
//
// After an utterance addressed to both, the last segment holds the floor:
// "Adam ty sprawdź to, Ania ty zrób tamto" leaves Ania holding it. That is
// arbitrary, which is exactly why it is written down and tested rather than
// left to whichever branch runs last.
func (f Floor) Move(a Addressing, now time.Time) (Floor, bool) {
	next := f
	switch a.Kind {
	case AddressedDirect:
		if len(a.Segments) == 0 {
			return f, false
		}
		next = Floor{Holder: a.Segments[len(a.Segments)-1].Staff, Since: now, Reason: FloorNamed}
	case AddressedSummon:
		next = Floor{Holder: a.Staff, Since: now, Reason: FloorSummoned}
	default:
		// carried and unclear leave the addressee alone.
		return f, false
	}
	if next.Holder == f.Holder && next.Reason == f.Reason {
		return f, false
	}
	return next, true
}

// token is one word of an utterance, folded for matching and kept positioned
// so a segment can be sliced out of the original text.
type token struct {
	word       string
	start, end int
	afterBreak bool
}

// tokenize splits an utterance into words, recording where punctuation broke
// the sentence. Speech to text punctuates unevenly, so a break is a hint that
// a segment may start rather than a requirement that one does.
func tokenize(text string) []token {
	var words []token
	broke := false
	start := -1
	flush := func(end int) {
		if start < 0 {
			return
		}
		words = append(words, token{
			word:       fold(text[start:end]),
			start:      start,
			end:        end,
			afterBreak: broke,
		})
		start, broke = -1, false
	}
	for i, r := range text {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if start < 0 {
				start = i
			}
		default:
			flush(i)
			if strings.ContainsRune(",;:.!?\n", r) {
				broke = true
			}
		}
	}
	flush(len(text))
	return words
}

// fold lowercases and drops Polish diacritics, so one written form matches
// what a transcriber returns on a bad day.
func fold(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case 'ą', 'Ą':
			return 'a'
		case 'ć', 'Ć':
			return 'c'
		case 'ę', 'Ę':
			return 'e'
		case 'ł', 'Ł':
			return 'l'
		case 'ń', 'Ń':
			return 'n'
		case 'ó', 'Ó':
			return 'o'
		case 'ś', 'Ś':
			return 's'
		case 'ź', 'Ź', 'ż', 'Ż':
			return 'z'
		}
		return unicode.ToLower(r)
	}, strings.TrimSpace(s))
}
