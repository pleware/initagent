package gdesk

import (
	"sync"
	"time"
)

// ConversationID names one person's ongoing talk at the desk.
//
// It is the scope of the floor and of the transcripts (docs/GDESK-SCOPES.md).
// Neither belongs to the desk: with one shared floor, one person says "Aniu"
// and the next person's unnamed sentence reaches Ania because the desk
// remembered somebody else's addressee — no error, and nothing in a log. With
// one shared transcript per staff member, Ania answers one person using
// another person's lines, which is wrong and also a confidentiality failure.
//
// The connector attaches this, never the caller who typed. A uniqueness a
// client promises is untrusted input, and two devices minting the same id is
// how one person's sentence would silently become a re-delivery of another's.
type ConversationID string

// DefaultConversation is the desk's only conversation.
//
// The empty value is not a placeholder for a real identifier. While one person
// talks there is one conversation, and an id for it would be a name nothing
// reads. The prefix is registered — initagent.gdesk.conversation mints
// `conversation-` — so naming it is a decision about need rather than about
// vocabulary. It becomes necessary at the same moment a second person may
// talk, which is also when the connector starts attaching one.
const DefaultConversation ConversationID = ""

// conversation is what belongs to one person talking rather than to the desk:
// who she last addressed, and what each staff member has heard from her.
//
// Staff, their personas and their commitments are deliberately absent. One
// Ania serves the whole desk (docs/GDESK-SCOPES.md), so putting her here would
// give each person a private copy of a person.
type conversation struct {
	// id is carried so a fact can be recorded with whose it is, without every
	// call passing it alongside the conversation it already has.
	id ConversationID

	// ear serialises answering. One person cannot listen to two replies at
	// once — not even to Adam and Ania at the same time — so it is held for a
	// whole turn, provider call included. It is per conversation and not per
	// desk because that is the whole point: the person on the phone must not
	// wait for the person at the glass to get her sentence back.
	ear sync.Mutex

	// mu guards what a turn writes. It is taken in short sections only, so a
	// reader asking who holds the floor is never behind a model.
	mu      sync.Mutex
	floor   Floor
	threads map[StaffID][]Line
}

// newConversation opens one with the floor already held, because the floor is
// never empty (docs/GDESK-CONVERSATION.md §2).
func newConversation(id ConversationID, holder StaffID, now time.Time) *conversation {
	return &conversation{
		id:      id,
		floor:   NewFloor(holder, now),
		threads: make(map[StaffID][]Line),
	}
}

// currentFloor reports who hears her next sentence.
func (c *conversation) currentFloor() Floor {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.floor
}

// setFloor records a move. Callers hold the ear, so reading the floor and
// moving it is one step from the point of view of another turn.
func (c *conversation) setFloor(floor Floor) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.floor = floor
}

// thread copies a staff member's lines, so a request being built cannot see
// the next turn's edits.
func (c *conversation) thread(staff StaffID) []Line {
	c.mu.Lock()
	defer c.mu.Unlock()

	lines := c.threads[staff]
	if len(lines) == 0 {
		return nil
	}
	out := make([]Line, len(lines))
	copy(out, lines)
	return out
}

// remember appends to a staff member's transcript, keeping the most recent
// lines. An empty line is dropped: a turn that failed before the first word
// said nothing, and a blank line reads as her ignoring the person.
func (c *conversation) remember(staff StaffID, line Line) {
	if line.Text == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	lines := append(c.threads[staff], line)
	if len(lines) > MaxThreadLines {
		lines = lines[len(lines)-MaxThreadLines:]
	}
	c.threads[staff] = lines
}
