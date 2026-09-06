package agent

import (
	"path/filepath"
	"testing"

	"github.com/pleware/initagent/internal/completion"
	"github.com/pleware/initagent/internal/protocol"
)

func TestProcessCommandExitZero(t *testing.T) {
	a := &Agent{}
	res := a.processCommand(protocol.ProcessStart{Command: "exit 0", TimeoutSec: 10})
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d pid = %d", res.ExitCode, res.Pid)
	}
	if res.Pid == 0 {
		t.Fatal("expected a pid")
	}
}

func TestProcessCommandExitSeven(t *testing.T) {
	a := &Agent{}
	res := a.processCommand(protocol.ProcessStart{Command: "exit 7", TimeoutSec: 10})
	if res.ExitCode != 7 {
		t.Fatalf("exit = %d, want 7", res.ExitCode)
	}
}

func TestProcessCommandMissingCwd(t *testing.T) {
	a := &Agent{}
	res := a.processCommand(protocol.ProcessStart{
		Command:    "exit 0",
		Cwd:        filepath.Join(t.TempDir(), "missing"),
		TimeoutSec: 5,
	})
	if res.ExitCode != -1 {
		t.Fatalf("exit = %d, want -1 for missing cwd", res.ExitCode)
	}
}

func TestRunSendKeysRequiresTmux(t *testing.T) {
	if tmuxAvailable() {
		t.Skip("tmux is present; this case covers the missing-tmux error")
	}
	a := &Agent{}
	_, err := a.runSendKeys(protocol.RunSendKeys{
		Command: "true",
		Nonce:   "0123456789abcdef0123456789abcdef",
		RunID:   "sess-1",
	})
	if err == nil {
		t.Fatal("expected error without tmux")
	}
}

func TestRunSendKeysRejectsEmptyCommand(t *testing.T) {
	a := &Agent{}
	_, err := a.runSendKeys(protocol.RunSendKeys{
		Command: "  ",
		Nonce:   "0123456789abcdef0123456789abcdef",
		RunID:   "sess-1",
	})
	if err == nil {
		t.Fatal("expected empty command error")
	}
}

func TestRunSendKeysRejectsBadNonce(t *testing.T) {
	a := &Agent{}
	_, err := a.runSendKeys(protocol.RunSendKeys{Command: "true", Nonce: "nope", RunID: "sess-1"})
	if err == nil {
		t.Fatal("expected invalid nonce error")
	}
}

func TestRunSendKeysRecoversDoneFile(t *testing.T) {
	dir := t.TempDir()
	runID := "tsk-recover"
	if err := completion.WriteDone(dir, runID, 4); err != nil {
		t.Fatal(err)
	}
	a := &Agent{runsDir: dir}
	res, err := a.runSendKeys(protocol.RunSendKeys{
		Command: "true",
		Nonce:   "0123456789abcdef0123456789abcdef",
		RunID:   runID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 4 {
		t.Fatalf("exit = %d, want 4", res.ExitCode)
	}
	if res.DoneFile != "4\n" {
		t.Fatalf("DoneFile = %q", res.DoneFile)
	}
}

func TestProcessCommandWritesDoneFile(t *testing.T) {
	dir := t.TempDir()
	runID := "tsk-write"
	a := &Agent{runsDir: dir}
	res := a.processCommand(protocol.ProcessStart{Command: "exit 3", TimeoutSec: 10, RunID: runID})
	if res.ExitCode != 3 {
		t.Fatalf("exit = %d, want 3", res.ExitCode)
	}
	got, ok := completion.ReadDone(dir, runID)
	if !ok || got.ExitCode != 3 {
		t.Fatalf("ReadDone = %+v %v", got, ok)
	}
}

func TestProcessCommandRecoversDoneFile(t *testing.T) {
	dir := t.TempDir()
	runID := "tsk-again"
	if err := completion.WriteDone(dir, runID, 8); err != nil {
		t.Fatal(err)
	}
	a := &Agent{runsDir: dir}
	res := a.processCommand(protocol.ProcessStart{Command: "exit 0", TimeoutSec: 10, RunID: runID})
	if res.ExitCode != 8 {
		t.Fatalf("exit = %d, want recovered 8", res.ExitCode)
	}
	if res.Pid != 0 {
		t.Fatalf("pid = %d, recover should not start a child", res.Pid)
	}
}
