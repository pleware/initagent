package agent

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestMaybeSelfUpdateGating(t *testing.T) {
	a := New(Config{HubURL: "http://127.0.0.1:1"}, "v0.1.0")

	// Not managed: never updates, regardless of version.
	os.Unsetenv(managedEnv)
	if a.maybeSelfUpdate(context.Background(), "v9.9.9", "ErzenXz/overseer") {
		t.Error("should not self-update when not managed")
	}

	os.Setenv(managedEnv, "1")
	defer os.Unsetenv(managedEnv)

	// Same version: no update.
	a2 := New(Config{HubURL: "http://127.0.0.1:1"}, "v0.1.0")
	if a2.maybeSelfUpdate(context.Background(), "v0.1.0", "ErzenXz/overseer") {
		t.Error("should not update to the same version")
	}

	// Dev/untagged hub version: skip to avoid update loops.
	a3 := New(Config{HubURL: "http://127.0.0.1:1"}, "0.1.0-dev")
	if a3.maybeSelfUpdate(context.Background(), "abc123-dirty", "ErzenXz/overseer") {
		t.Error("should not update to a non-tagged version")
	}

	// Empty hub version: no update.
	a4 := New(Config{HubURL: "http://127.0.0.1:1"}, "v0.1.0")
	if a4.maybeSelfUpdate(context.Background(), "", "ErzenXz/overseer") {
		t.Error("should not update when hub reports no version")
	}

	// Missing repository metadata never attempts an update.
	a5 := New(Config{HubURL: "http://127.0.0.1:1"}, "v0.1.0")
	if a5.maybeSelfUpdate(context.Background(), "v0.2.0", "") {
		t.Error("should not update without trusted repository metadata")
	}
}

func TestMaybeSelfUpdateWaitsForARunningCommand(t *testing.T) {
	os.Setenv(managedEnv, "1")
	defer os.Unsetenv(managedEnv)

	a := New(Config{HubURL: "http://127.0.0.1:1"}, "v0.1.0")
	a.execStarted()

	if a.maybeSelfUpdate(context.Background(), "v9.9.9", "ErzenXz/overseer") {
		t.Error("should not swap the binary while a command is running")
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.nextUpdateAttempt.IsZero() {
		t.Error("a busy skip should not spend the hourly retry budget")
	}
}

func TestRestartWhenIdleReleasesAnIdleDevice(t *testing.T) {
	a := New(Config{HubURL: "http://127.0.0.1:1"}, "v0.1.0")

	a.restartWhenIdle(context.Background())

	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.updateApplied {
		t.Error("an idle device should restart into the staged binary")
	}
}

func TestRestartWhenIdleWaitsForARunningCommand(t *testing.T) {
	previous := idleRestartPoll
	idleRestartPoll = time.Millisecond
	defer func() { idleRestartPoll = previous }()

	a := New(Config{HubURL: "http://127.0.0.1:1"}, "v0.1.0")
	a.execStarted()

	done := make(chan struct{})
	go func() {
		a.restartWhenIdle(context.Background())
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("restarted while a command was still running")
	case <-time.After(20 * time.Millisecond):
	}

	a.execDone()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("did not restart after the command finished")
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.updateApplied {
		t.Error("the staged binary should be picked up once the slot frees")
	}
}

func TestRestartWhenIdleGivesUpWhenTheAgentStops(t *testing.T) {
	previous := idleRestartPoll
	idleRestartPoll = time.Millisecond
	defer func() { idleRestartPoll = previous }()

	a := New(Config{HubURL: "http://127.0.0.1:1"}, "v0.1.0")
	a.execStarted()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.restartWhenIdle(ctx)

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.updateApplied {
		t.Error("a cancelled agent should not claim the swap succeeded")
	}
}
