package completion

import (
	"context"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestProcessResolver_Name(t *testing.T) {
	p := &ProcessResolver{}
	if name := p.Name(); name != "process" {
		t.Fatalf("Name() = %q, want %q", name, "process")
	}
}

func TestProcessResolver_Supports(t *testing.T) {
	p := &ProcessResolver{}
	if !p.Supports(LaunchSupervised) {
		t.Error("expected support for LaunchSupervised")
	}
	if p.Supports(LaunchSendKeys) {
		t.Error("expected NO support for LaunchSendKeys")
	}
}

func TestProcessResolver_MissingProcessID(t *testing.T) {
	p := &ProcessResolver{}
	ctx := context.Background()

	run := RunContext{
		RunID:      "test-1",
		LaunchMode: LaunchSupervised,
		ProcessID:  0, // missing
	}

	_, err := p.Watch(ctx, run)
	if err == nil {
		t.Fatal("expected error for missing ProcessID")
	}
}

func TestProcessResolver_WatchProcessExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process exit detection timing is flaky on Windows in CI")
	}

	// Start a short-lived process (sleep 0.5s)
	cmd := exec.Command("sleep", "0.5")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start process: %v", err)
	}

	p := &ProcessResolver{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	run := RunContext{
		RunID:      "test-2",
		LaunchMode: LaunchSupervised,
		ProcessID:  cmd.Process.Pid,
	}

	ch, err := p.Watch(ctx, run)
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	select {
	case outcome := <-ch:
		if !outcome.Done {
			t.Fatal("expected Done=true")
		}
		if outcome.Reason != "process" {
			t.Fatalf("Reason = %q, want %q", outcome.Reason, "process")
		}
		if outcome.Trust != TrustHigh {
			t.Fatalf("Trust = %q, want %q", outcome.Trust, TrustHigh)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for process exit")
	}

	// Clean up
	cmd.Wait()
}

func TestProcessResolver_ContextCancellation(t *testing.T) {
	// Start a long-lived process
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start sleep command: %v", err)
	}
	defer cmd.Process.Kill()

	p := &ProcessResolver{}
	ctx, cancel := context.WithCancel(context.Background())

	run := RunContext{
		RunID:      "test-3",
		LaunchMode: LaunchSupervised,
		ProcessID:  cmd.Process.Pid,
	}

	ch, err := p.Watch(ctx, run)
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Cancel context immediately
	cancel()

	// Channel should close without outcome
	select {
	case outcome, ok := <-ch:
		if ok {
			t.Fatalf("unexpected outcome after cancel: %+v", outcome)
		}
		// Channel closed as expected
	case <-time.After(2 * time.Second):
		t.Fatal("channel not closed after context cancel")
	}
}

func TestGet_PackageLevel(t *testing.T) {
	// Test package-level Get function (for coverage)
	resolver := Get("file")
	if resolver == nil {
		t.Error("expected 'file' resolver from package-level Get")
	}
	if resolver.Name() != "file" {
		t.Fatalf("resolver.Name() = %q, want %q", resolver.Name(), "file")
	}
}

func TestProcessResolver_ProcessNotFound(t *testing.T) {
	// Test behavior when process is already gone
	p := &ProcessResolver{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Use a PID that is unlikely to exist (very high number)
	run := RunContext{
		RunID:      "test-4",
		LaunchMode: LaunchSupervised,
		ProcessID:  999999,
	}

	ch, err := p.Watch(ctx, run)
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	select {
	case outcome := <-ch:
		if !outcome.Done {
			t.Fatal("expected Done=true for non-existent process")
		}
		if outcome.Reason != "process" {
			t.Fatalf("Reason = %q, want %q", outcome.Reason, "process")
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for outcome")
	}
}

func TestProcessResolver_ProcessAlreadyExited(t *testing.T) {
	// Start and immediately wait for process to exit
	cmd := exec.Command("echo", "test")
	if err := cmd.Run(); err != nil {
		t.Skipf("cannot run echo: %v", err)
	}

	p := &ProcessResolver{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Process should already be gone
	run := RunContext{
		RunID:      "test-5",
		LaunchMode: LaunchSupervised,
		ProcessID:  cmd.Process.Pid,
	}

	ch, err := p.Watch(ctx, run)
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	select {
	case outcome := <-ch:
		if !outcome.Done {
			t.Fatal("expected Done=true for already-exited process")
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for outcome")
	}
}
