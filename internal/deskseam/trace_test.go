package deskseam

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
}
