package completion

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSentinelResolver_Name(t *testing.T) {
	s := &SentinelResolver{}
	if got := s.Name(); got != "sentinel" {
		t.Fatalf("Name() = %q, want sentinel", got)
	}
}

func TestSentinelResolver_Supports(t *testing.T) {
	s := &SentinelResolver{}
	if !s.Supports(LaunchSendKeys) {
		t.Error("expected support for LaunchSendKeys")
	}
	if s.Supports(LaunchSupervised) {
		t.Error("did not expect support for LaunchSupervised")
	}
}

func TestMintNonce(t *testing.T) {
	n, err := MintNonce()
	if err != nil {
		t.Fatal(err)
	}
	if !ValidNonce(n) {
		t.Fatalf("minted nonce %q is invalid", n)
	}
	other, err := MintNonce()
	if err != nil {
		t.Fatal(err)
	}
	if n == other {
		t.Fatal("expected distinct nonces")
	}
}

func TestValidNonce(t *testing.T) {
	if ValidNonce("short") || ValidNonce("") || ValidNonce("GHIJKLMNOPQRSTUV") {
		t.Fatal("accepted an invalid nonce")
	}
	if !ValidNonce("0123456789abcdef") {
		t.Fatal("rejected a 16-char hex nonce")
	}
}

func TestMatch_LastMatchingMarker(t *testing.T) {
	nonce := "0123456789abcdef0123456789abcdef"
	other := "fedcba9876543210fedcba9876543210"
	output := Marker(other, 1) + "\nwork\n" + Marker(nonce, 3) + "\n" + Marker(nonce, 0)
	code, ok := Match(output, nonce)
	if !ok || code != 0 {
		t.Fatalf("Match = %d %v, want 0 true", code, ok)
	}
}

func TestMatch_WrongNonce(t *testing.T) {
	output := Marker("0123456789abcdef0123456789abcdef", 0)
	if _, ok := Match(output, "fedcba9876543210fedcba9876543210"); ok {
		t.Fatal("matched a different nonce")
	}
}

func TestWrapUnixContainsMarkerFormat(t *testing.T) {
	nonce := "0123456789abcdef0123456789abcdef"
	got := WrapUnix(`coder-cli "$@"`, nonce)
	if !strings.Contains(got, "<<<initagent "+nonce+" exit=%d>>>") {
		t.Fatalf("wrap = %q", got)
	}
	if !strings.Contains(got, "$?") {
		t.Fatalf("wrap missing $?: %q", got)
	}
}

func TestWrapPowerShellContainsNonce(t *testing.T) {
	nonce := "0123456789abcdef0123456789abcdef"
	got := WrapPowerShell("coder-cli @args", nonce)
	if !strings.Contains(got, "<<<initagent "+nonce+" exit=$LASTEXITCODE>>>") {
		t.Fatalf("wrap = %q", got)
	}
}

func TestWrapCommand_NonEmpty(t *testing.T) {
	got := WrapCommand("true", "0123456789abcdef0123456789abcdef")
	if got == "" {
		t.Fatal("empty wrap")
	}
}

func TestSentinelResolver_WatchOutput(t *testing.T) {
	nonce := "0123456789abcdef0123456789abcdef"
	s := &SentinelResolver{}
	ch, err := s.Watch(t.Context(), RunContext{
		RunID:      "run-1",
		LaunchMode: LaunchSendKeys,
		Nonce:      nonce,
		Output:     "log\n" + Marker(nonce, 7) + "\n",
	})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	outcome := <-ch
	if !outcome.Done || outcome.ExitCode != 7 || outcome.Reason != "sentinel" || outcome.Trust != TrustMedium {
		t.Fatalf("outcome = %+v", outcome)
	}
	if _, ok := <-ch; ok {
		t.Fatal("channel should be closed")
	}
}

func TestSentinelResolver_WatchRejects(t *testing.T) {
	s := &SentinelResolver{}
	if _, err := s.Watch(t.Context(), RunContext{LaunchMode: LaunchSendKeys}); err == nil {
		t.Fatal("expected error for missing nonce")
	}
	nonce := "0123456789abcdef0123456789abcdef"
	if _, err := s.Watch(t.Context(), RunContext{Nonce: nonce, LaunchMode: LaunchSendKeys}); err == nil {
		t.Fatal("expected error for missing output")
	}
	if _, err := s.Watch(t.Context(), RunContext{
		Nonce: nonce, Output: "no marker", LaunchMode: LaunchSendKeys,
	}); err == nil {
		t.Fatal("expected error when marker is absent")
	}
}

func TestSentinelResolver_WatchFile(t *testing.T) {
	nonce := "0123456789abcdef0123456789abcdef"
	dir := t.TempDir()
	path := filepath.Join(dir, "pane.txt")
	s := &SentinelResolver{PollInterval: 20 * time.Millisecond}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	ch, err := s.Watch(ctx, RunContext{
		RunID:      "run-1",
		LaunchMode: LaunchSendKeys,
		Nonce:      nonce,
		OutputPath: path,
	})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	go func() {
		time.Sleep(40 * time.Millisecond)
		_ = os.WriteFile(path, []byte(Marker(nonce, 4)), 0o644)
	}()

	select {
	case outcome := <-ch:
		if !outcome.Done || outcome.ExitCode != 4 {
			t.Fatalf("outcome = %+v", outcome)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for sentinel file")
	}
}

func TestSentinelResolver_WatchFileCancel(t *testing.T) {
	nonce := "0123456789abcdef0123456789abcdef"
	s := &SentinelResolver{PollInterval: 20 * time.Millisecond}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	ch, err := s.Watch(ctx, RunContext{
		Nonce:      nonce,
		OutputPath: filepath.Join(t.TempDir(), "missing.txt"),
	})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	select {
	case outcome, ok := <-ch:
		if ok {
			t.Fatalf("unexpected outcome: %+v", outcome)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("channel not closed after cancel")
	}
}

func TestSentinelResolver_WatchFileImmediate(t *testing.T) {
	nonce := "0123456789abcdef0123456789abcdef"
	path := filepath.Join(t.TempDir(), "pane.txt")
	if err := os.WriteFile(path, []byte(Marker(nonce, 2)), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &SentinelResolver{PollInterval: 20 * time.Millisecond}
	ch, err := s.Watch(t.Context(), RunContext{
		Nonce:      nonce,
		OutputPath: path,
	})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	outcome := <-ch
	if !outcome.Done || outcome.ExitCode != 2 {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestResolve_SendKeysSentinel(t *testing.T) {
	nonce := "0123456789abcdef0123456789abcdef"
	outcome, err := Default.Resolve(t.Context(), RunContext{
		RunID:      "run-1",
		LaunchMode: LaunchSendKeys,
		Nonce:      nonce,
		Output:     Marker(nonce, 0),
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if outcome.Reason != "sentinel" || outcome.ExitCode != 0 || outcome.Trust != TrustMedium {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestDefaultRegistry_HasSentinel(t *testing.T) {
	if Default.Get("sentinel") == nil {
		t.Fatal("expected sentinel resolver in default registry")
	}
}

func TestWrapUnixDoneWritesPath(t *testing.T) {
	nonce := "0123456789abcdef0123456789abcdef"
	got := WrapUnixDone("true", nonce, "/tmp/runs/run-1.done")
	if !strings.Contains(got, "/tmp/runs/run-1.done") {
		t.Fatalf("wrap missing done path: %q", got)
	}
	if !strings.Contains(got, "_ia_code") {
		t.Fatalf("wrap missing captured exit: %q", got)
	}
}

func TestWrapUnixDoneEmptyPathMatchesWrapUnix(t *testing.T) {
	nonce := "0123456789abcdef0123456789abcdef"
	if WrapUnixDone("true", nonce, "") != WrapUnix("true", nonce) {
		t.Fatal("empty done path should match WrapUnix")
	}
}

func TestWrapPowerShellDoneWritesPath(t *testing.T) {
	nonce := "0123456789abcdef0123456789abcdef"
	got := WrapPowerShellDone("coder-cli", nonce, `C:\runs\run-1.done`)
	if !strings.Contains(got, `C:\runs\run-1.done`) {
		t.Fatalf("wrap missing done path: %q", got)
	}
}

func TestResolve_DoneBodyUsesFile(t *testing.T) {
	outcome, err := Default.Resolve(t.Context(), RunContext{
		RunID:      "run-1",
		LaunchMode: LaunchSendKeys,
		DoneBody:   "0\n",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if outcome.Reason != "file" || outcome.ExitCode != 0 || outcome.Trust != TrustHigh {
		t.Fatalf("outcome = %+v", outcome)
	}
}
