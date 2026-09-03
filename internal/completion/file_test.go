package completion

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSentinelPath(t *testing.T) {
	got, err := SentinelPath("runs", "run-abc")
	if err != nil {
		t.Fatalf("SentinelPath: %v", err)
	}
	want := filepath.Join("runs", "run-abc.done")
	if got != want {
		t.Fatalf("SentinelPath = %q, want %q", got, want)
	}
}

func TestSentinelPathRejects(t *testing.T) {
	cases := []struct {
		name  string
		dir   string
		runID string
	}{
		{"empty dir", "", "run-1"},
		{"empty run", "/runs", ""},
		{"slash in run", "/runs", "run/../escape"},
		{"backslash in run", "/runs", `run\..\escape`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := SentinelPath(tc.dir, tc.runID); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestFileResolver_Name(t *testing.T) {
	f := &FileResolver{}
	if name := f.Name(); name != "file" {
		t.Fatalf("Name() = %q, want %q", name, "file")
	}
}

func TestFileResolver_Supports(t *testing.T) {
	f := &FileResolver{}
	if !f.Supports(LaunchSendKeys) {
		t.Error("expected support for LaunchSendKeys")
	}
	if !f.Supports(LaunchSupervised) {
		t.Error("expected support for LaunchSupervised")
	}
}

func writeDone(t *testing.T, dir, runID, body string) string {
	t.Helper()
	path, err := SentinelPath(dir, runID)
	if err != nil {
		t.Fatalf("SentinelPath: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	return path
}

func watchDone(t *testing.T, dir, runID string) Outcome {
	t.Helper()
	f := &FileResolver{PollInterval: 20 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := f.Watch(ctx, RunContext{RunID: runID, SentinelDir: dir})
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	select {
	case outcome := <-ch:
		return outcome
	case <-ctx.Done():
		t.Fatal("timeout waiting for outcome")
		return Outcome{}
	}
}

func TestFileResolver_PlainSentinel(t *testing.T) {
	tmp := t.TempDir()
	writeDone(t, tmp, "run-plain", "42\n")

	outcome := watchDone(t, tmp, "run-plain")
	if !outcome.Done {
		t.Fatal("expected Done=true")
	}
	if outcome.ExitCode != 42 {
		t.Fatalf("ExitCode = %d, want 42", outcome.ExitCode)
	}
	if outcome.Reason != "file" {
		t.Fatalf("Reason = %q, want %q", outcome.Reason, "file")
	}
	if outcome.Trust != TrustHigh {
		t.Fatalf("Trust = %q, want %q", outcome.Trust, TrustHigh)
	}
}

func TestFileResolver_JSONSentinel(t *testing.T) {
	tmp := t.TempDir()
	writeDone(t, tmp, "run-json", `{"exit_code": 7, "timestamp": 1234567890}`)

	outcome := watchDone(t, tmp, "run-json")
	if !outcome.Done {
		t.Fatal("expected Done=true")
	}
	if outcome.ExitCode != 7 {
		t.Fatalf("ExitCode = %d, want 7", outcome.ExitCode)
	}
	if outcome.Trust != TrustHigh {
		t.Fatalf("Trust = %q, want %q", outcome.Trust, TrustHigh)
	}
}

func TestFileResolver_MalformedSentinel(t *testing.T) {
	tmp := t.TempDir()
	writeDone(t, tmp, "run-bad", "not-a-number\n")

	outcome := watchDone(t, tmp, "run-bad")
	if !outcome.Done {
		t.Fatal("expected Done=true")
	}
	if outcome.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want 1 (malformed)", outcome.ExitCode)
	}
	if outcome.Trust != TrustHigh {
		t.Fatalf("Trust = %q, want %q", outcome.Trust, TrustHigh)
	}
}

func TestFileResolver_AppearsAfterWatch(t *testing.T) {
	tmp := t.TempDir()
	runID := "run-late"

	f := &FileResolver{PollInterval: 20 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := f.Watch(ctx, RunContext{RunID: runID, SentinelDir: tmp})
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	go func() {
		time.Sleep(40 * time.Millisecond)
		writeDone(t, tmp, runID, "0\n")
	}()

	select {
	case outcome := <-ch:
		if outcome.ExitCode != 0 {
			t.Fatalf("ExitCode = %d, want 0", outcome.ExitCode)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for late sentinel")
	}
}

func TestFileResolver_TwoRunsDoNotShareAFile(t *testing.T) {
	tmp := t.TempDir()
	writeDone(t, tmp, "run-a", "1\n")
	writeDone(t, tmp, "run-b", "2\n")

	a := watchDone(t, tmp, "run-a")
	b := watchDone(t, tmp, "run-b")
	if a.ExitCode != 1 {
		t.Fatalf("run-a ExitCode = %d, want 1", a.ExitCode)
	}
	if b.ExitCode != 2 {
		t.Fatalf("run-b ExitCode = %d, want 2", b.ExitCode)
	}
}

func TestFileResolver_IgnoresBareDone(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, ".done"), []byte("99\n"), 0o644); err != nil {
		t.Fatalf("write leftover .done: %v", err)
	}

	f := &FileResolver{PollInterval: 20 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	ch, err := f.Watch(ctx, RunContext{RunID: "run-x", SentinelDir: tmp})
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	select {
	case outcome := <-ch:
		t.Fatalf("watched the leftover .done file: %+v", outcome)
	case <-ctx.Done():
	}
}

func TestFileResolver_NoSentinelTimeout(t *testing.T) {
	tmp := t.TempDir()

	f := &FileResolver{PollInterval: 20 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	ch, err := f.Watch(ctx, RunContext{RunID: "run-none", SentinelDir: tmp})
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	select {
	case outcome := <-ch:
		t.Fatalf("unexpected outcome: %+v", outcome)
	case <-ctx.Done():
	}
}

func TestFileResolver_MissingSentinelDir(t *testing.T) {
	f := &FileResolver{}
	_, err := f.Watch(context.Background(), RunContext{RunID: "run-1"})
	if err == nil {
		t.Fatal("expected error for missing SentinelDir")
	}
}

func TestFileResolver_MissingRunID(t *testing.T) {
	f := &FileResolver{}
	_, err := f.Watch(context.Background(), RunContext{SentinelDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected error for missing RunID")
	}
}
