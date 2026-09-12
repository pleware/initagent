package gdeskseam

import (
	"strings"
	"testing"
	"time"
)

func TestTraceForgetsTheOldestPastTheCeiling(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 11, 20, 0, 0, 0, time.UTC)
	trace := NewTrace(TraceConfig{Max: 3, Now: func() time.Time { return at }})
	for _, text := range []string{"a", "b", "c", "d"} {
		trace.Record("info", text)
	}
	dump := trace.Dump()
	if dump.V != Version {
		t.Fatalf("v = %d, want %d", dump.V, Version)
	}
	if len(dump.Lines) != 3 {
		t.Fatalf("kept %d, want 3", len(dump.Lines))
	}
	if dump.Lines[0].Text != "b" || dump.Lines[2].Text != "d" {
		t.Fatalf("kept %+v", dump.Lines)
	}
	if dump.Lines[2].Seq != 4 {
		t.Fatalf("seq = %d, want 4", dump.Lines[2].Seq)
	}
	if !strings.HasPrefix(dump.Lines[0].At, "2026-09-11T20:00:00") {
		t.Fatalf("at = %q", dump.Lines[0].At)
	}
}

func TestTraceDumpHandsOutACopy(t *testing.T) {
	t.Parallel()
	trace := NewTrace(TraceConfig{})
	trace.Record("info", "kept")
	dump := trace.Dump()
	dump.Lines[0].Text = "edited"
	again := trace.Dump()
	if again.Lines[0].Text != "kept" {
		t.Fatal("a reader edited the ring")
	}
}

func TestANilTraceIsSilent(t *testing.T) {
	t.Parallel()
	var trace *Trace
	trace.Record("info", "ignored")
	if got := trace.Dump(); len(got.Lines) != 0 || got.V != Version {
		t.Fatalf("dump = %+v", got)
	}
}

func TestNewTraceDefaultsTheCeiling(t *testing.T) {
	t.Parallel()
	trace := NewTrace(TraceConfig{})
	if trace.max != MaxTraceLines {
		t.Fatalf("max = %d, want %d", trace.max, MaxTraceLines)
	}
	if trace.age != MaxTraceAge {
		t.Fatalf("age = %s, want %s", trace.age, MaxTraceAge)
	}
}

func TestTraceForgetsWhatIsOlderThanTheWindow(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 11, 20, 0, 0, 0, time.UTC)
	trace := NewTrace(TraceConfig{MaxAge: 10 * time.Minute, Now: func() time.Time { return at }})

	trace.Record("info", "stary")
	at = at.Add(11 * time.Minute)
	trace.Record("info", "swiezy")

	dump := trace.Dump()
	if len(dump.Lines) != 1 || dump.Lines[0].Text != "swiezy" {
		t.Fatalf("kept %+v", dump.Lines)
	}
	// The count is the ring's own and does not restart: a gap in seq is how a
	// reader sees that something was dropped rather than never recorded.
	if dump.Lines[0].Seq != 2 {
		t.Fatalf("seq = %d, want 2", dump.Lines[0].Seq)
	}
}

func TestTraceForgetsOnReadSoAQuietDeskDoesNotShowAnHour(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 11, 20, 0, 0, 0, time.UTC)
	trace := NewTrace(TraceConfig{MaxAge: 10 * time.Minute, Now: func() time.Time { return at }})
	trace.Record("info", "jedyna linia")

	at = at.Add(30 * time.Minute)
	// Nothing was recorded in between, which is exactly the case a bound
	// applied only on write would miss.
	if lines := trace.Dump().Lines; len(lines) != 0 {
		t.Fatalf("kept %+v after the window passed", lines)
	}
}

func TestTraceKeepsEverythingWhenTheWindowIsWaived(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 11, 20, 0, 0, 0, time.UTC)
	trace := NewTrace(TraceConfig{MaxAge: -1, Now: func() time.Time { return at }})
	trace.Record("info", "pierwsza")
	at = at.Add(3 * time.Hour)
	trace.Record("info", "druga")
	if lines := trace.Dump().Lines; len(lines) != 2 {
		t.Fatalf("kept %+v, want both", lines)
	}
}

func TestATraceWhoseClockWentBackwardsKeepsWhatItHas(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 11, 20, 0, 0, 0, time.UTC)
	trace := NewTrace(TraceConfig{MaxAge: 10 * time.Minute, Now: func() time.Time { return at }})
	trace.Record("info", "przed skokiem")

	at = at.Add(-2 * time.Hour)
	// An emptied pane would be the worse of the two wrong answers: the lines
	// are real, and a clock correction is not a reason to hide them.
	if lines := trace.Dump().Lines; len(lines) != 1 {
		t.Fatalf("kept %+v", lines)
	}
}
