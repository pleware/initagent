package completion

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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

func TestFileResolver_PlainSentinel(t *testing.T) {
	tmp := t.TempDir()
	sentinelPath := filepath.Join(tmp, ".done")

	// Write plain text sentinel
	if err := os.WriteFile(sentinelPath, []byte("42\n"), 0644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	f := &FileResolver{PollInterval: 50 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	run := RunContext{
		RunID:       "test-1",
		SentinelDir: tmp,
	}

	ch, err := f.Watch(ctx, run)
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	select {
	case outcome := <-ch:
		if !outcome.Done {
			t.Fatal("expected Done=true")
		}
		if outcome.ExitCode != 42 {
			t.Fatalf("ExitCode = %d, want 42", outcome.ExitCode)
		}
		if outcome.Reason != "file" {
			t.Fatalf("Reason = %q, want %q", outcome.Reason, "file")
		}
		if outcome.Trust != TrustMedium {
			t.Fatalf("Trust = %q, want %q", outcome.Trust, TrustMedium)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for outcome")
	}
}

func TestFileResolver_JSONSentinel(t *testing.T) {
	tmp := t.TempDir()
	sentinelPath := filepath.Join(tmp, ".done")

	// Write JSON sentinel (matches fakecoder --sentinel-format=json)
	json := `{"exit_code": 7, "timestamp": 1234567890}`
	if err := os.WriteFile(sentinelPath, []byte(json), 0644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	f := &FileResolver{PollInterval: 50 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	run := RunContext{
		RunID:       "test-2",
		SentinelDir: tmp,
	}

	ch, err := f.Watch(ctx, run)
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	select {
	case outcome := <-ch:
		if !outcome.Done {
			t.Fatal("expected Done=true")
		}
		if outcome.ExitCode != 7 {
			t.Fatalf("ExitCode = %d, want 7", outcome.ExitCode)
		}
		if outcome.Reason != "file" {
			t.Fatalf("Reason = %q, want %q", outcome.Reason, "file")
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for outcome")
	}
}

func TestFileResolver_MalformedSentinel(t *testing.T) {
	tmp := t.TempDir()
	sentinelPath := filepath.Join(tmp, ".done")

	// Write invalid content
	if err := os.WriteFile(sentinelPath, []byte("not-a-number\n"), 0644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	f := &FileResolver{PollInterval: 50 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	run := RunContext{
		RunID:       "test-3",
		SentinelDir: tmp,
	}

	ch, err := f.Watch(ctx, run)
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	select {
	case outcome := <-ch:
		if !outcome.Done {
			t.Fatal("expected Done=true")
		}
		if outcome.ExitCode != 1 {
			t.Fatalf("ExitCode = %d, want 1 (malformed)", outcome.ExitCode)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for outcome")
	}
}

func TestFileResolver_NoSentinelTimeout(t *testing.T) {
	tmp := t.TempDir()

	f := &FileResolver{PollInterval: 50 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	run := RunContext{
		RunID:       "test-4",
		SentinelDir: tmp,
	}

	ch, err := f.Watch(ctx, run)
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	select {
	case outcome := <-ch:
		t.Fatalf("unexpected outcome: %+v", outcome)
	case <-ctx.Done():
		// Expected - no sentinel appeared
	}
}

func TestFileResolver_MissingSentinelDir(t *testing.T) {
	f := &FileResolver{}
	ctx := context.Background()

	run := RunContext{
		RunID:       "test-5",
		SentinelDir: "", // missing
	}

	_, err := f.Watch(ctx, run)
	if err == nil {
		t.Fatal("expected error for missing SentinelDir")
	}
}
