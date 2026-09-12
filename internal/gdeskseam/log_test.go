package gdeskseam

import (
	"strings"
	"testing"
	"time"
)

func fixedClock() func() time.Time {
	at := time.Date(2026, 9, 11, 3, 0, 0, 0, time.UTC)
	return func() time.Time { return at }
}

func newTestLog(t *testing.T, max int, notify func(Event)) *Log {
	t.Helper()
	log, err := NewLog(LogConfig{Stream: "gdesk:local", Max: max, Now: fixedClock(), Notify: notify})
	if err != nil {
		t.Fatal(err)
	}
	return log
}

func TestNewLogRefusesAStreamlessLog(t *testing.T) {
	_, err := NewLog(LogConfig{})
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "needs a stream") {
		t.Fatalf("err = %v", err)
	}
}

func TestNewLogDefaultsTheCeilingAndTheClock(t *testing.T) {
	log, err := NewLog(LogConfig{Stream: "gdesk:local"})
	if err != nil {
		t.Fatal(err)
	}
	if log.max != MaxLogEvents {
		t.Fatalf("max = %d, want %d", log.max, MaxLogEvents)
	}
	if log.now == nil {
		t.Fatal("a log without a clock cannot stamp a fact")
	}
	if log.Stream() != "gdesk:local" {
		t.Fatalf("stream = %q", log.Stream())
	}
}

func TestAppendNumbersFromOneWithoutGaps(t *testing.T) {
	log := newTestLog(t, 0, nil)
	if log.Seq() != 0 {
		t.Fatalf("seq = %d before the first fact, want 0", log.Seq())
	}
	for want := int64(1); want <= 5; want++ {
		got := log.Append(EventSurfaceAppended, surfaceAppended{ID: "surface-1"}, "")
		if got.Seq != want {
			t.Fatalf("seq = %d, want %d", got.Seq, want)
		}
	}
	if log.Seq() != 5 {
		t.Fatalf("seq = %d, want 5", log.Seq())
	}
}

func TestAppendStampsTheEnvelope(t *testing.T) {
	log := newTestLog(t, 0, nil)
	got := log.Append(EventSurfaceOpened, surfaceOpened{}, "c1")
	if got.V != Version || got.Stream != "gdesk:local" || got.Kind != EventSurfaceOpened {
		t.Fatalf("envelope = %+v", got.Envelope)
	}
	if got.InReplyTo != "c1" {
		t.Fatalf("inReplyTo = %q", got.InReplyTo)
	}
	if got.At != "2026-09-11T03:00:00Z" {
		t.Fatalf("at = %q", got.At)
	}
}

func TestAppendTellsAListenerInSequenceOrder(t *testing.T) {
	var seen []int64
	log := newTestLog(t, 0, func(event Event) { seen = append(seen, event.Seq) })
	for range 4 {
		log.Append(EventSurfaceAppended, surfaceAppended{}, "")
	}
	if len(seen) != 4 {
		t.Fatalf("heard %d facts, want 4", len(seen))
	}
	for i, seq := range seen {
		if seq != int64(i+1) {
			t.Fatalf("heard %v, want them in order", seen)
		}
	}
}

func TestAppendForgetsTheOldestPastTheCeiling(t *testing.T) {
	log := newTestLog(t, 3, nil)
	for range 5 {
		log.Append(EventSurfaceAppended, surfaceAppended{}, "")
	}
	kept := log.Since(0)
	if len(kept) != 3 {
		t.Fatalf("kept %d facts, want 3", len(kept))
	}
	if kept[0].Seq != 3 || kept[2].Seq != 5 {
		t.Fatalf("kept %d..%d, want 3..5", kept[0].Seq, kept[2].Seq)
	}
	if log.Seq() != 5 {
		t.Fatalf("seq = %d, want 5 - forgetting a fact must not rewind the count", log.Seq())
	}
}

func TestSinceAnswersFromWhereTheGlassStopped(t *testing.T) {
	log := newTestLog(t, 0, nil)
	for range 5 {
		log.Append(EventSurfaceAppended, surfaceAppended{}, "")
	}
	got := log.Since(4)
	if len(got) != 2 || got[0].Seq != 4 || got[1].Seq != 5 {
		t.Fatalf("got %+v, want 4 and 5", seqsOf(got))
	}
}

func TestSinceAnswersEverythingItStillHasRatherThanNothing(t *testing.T) {
	log := newTestLog(t, 3, nil)
	for range 6 {
		log.Append(EventSurfaceAppended, surfaceAppended{}, "")
	}
	got := log.Since(1)
	if len(got) != 3 || got[0].Seq != 4 {
		t.Fatalf("got %v, want the three it still holds", seqsOf(got))
	}
}

func TestSinceIsEmptyWhenTheGlassIsAhead(t *testing.T) {
	log := newTestLog(t, 0, nil)
	log.Append(EventSurfaceAppended, surfaceAppended{}, "")
	if got := log.Since(2); len(got) != 0 {
		t.Fatalf("got %v, want nothing", seqsOf(got))
	}
}

func TestSinceHandsOutACopy(t *testing.T) {
	log := newTestLog(t, 0, nil)
	log.Append(EventSurfaceAppended, surfaceAppended{ID: "surface-1"}, "")
	first := log.Since(0)
	first[0].Seq = 99
	again := log.Since(0)
	if again[0].Seq != 1 {
		t.Fatalf("seq = %d, want 1 - a reader edited the log", again[0].Seq)
	}
}

func seqsOf(events []Event) []int64 {
	out := make([]int64, 0, len(events))
	for _, event := range events {
		out = append(out, event.Seq)
	}
	return out
}
