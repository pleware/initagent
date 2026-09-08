//go:build windows

package agent

import (
	"os/exec"
	"syscall"

	"github.com/charmbracelet/x/conpty"
	"golang.org/x/sys/windows"
)

// stillActive is the Win32 STILL_ACTIVE value GetExitCodeProcess returns
// while the child is running.
const stillActive = 259

type windowsTerminal struct {
	pty    *conpty.ConPty
	handle windows.Handle
}

func startTerminal(cmd *exec.Cmd, cols, rows uint16) (terminalBackend, error) {
	p, err := conpty.New(int(cols), int(rows), 0)
	if err != nil {
		return nil, err
	}
	_, rawHandle, err := p.Spawn(cmd.Path, cmd.Args, &syscall.ProcAttr{Dir: cmd.Dir, Env: cmd.Env})
	if err != nil {
		_ = p.Close()
		return nil, err
	}
	t := &windowsTerminal{pty: p, handle: windows.Handle(rawHandle)}
	t.watchChild()
	return t, nil
}

// watchChild waits for the ConPTY child to die, then closes the console so
// Read unblocks. Windows has no SIGCHLD; WaitForSingleObject on a duplicated
// handle is the equivalent of waitpid. The duplicate keeps KillWait free to
// close the original handle.
func (t *windowsTerminal) watchChild() {
	var dup windows.Handle
	err := windows.DuplicateHandle(
		windows.CurrentProcess(),
		t.handle,
		windows.CurrentProcess(),
		&dup,
		windows.SYNCHRONIZE,
		false,
		0,
	)
	if err != nil {
		return
	}
	go func() {
		defer windows.CloseHandle(dup)
		_, _ = windows.WaitForSingleObject(dup, windows.INFINITE)
		_ = t.pty.Close()
	}()
}

func (t *windowsTerminal) Read(p []byte) (int, error)  { return t.pty.Read(p) }
func (t *windowsTerminal) Write(p []byte) (int, error) { return t.pty.Write(p) }
func (t *windowsTerminal) Close() error                { return t.pty.Close() }
func (t *windowsTerminal) Resize(cols, rows uint16) error {
	return t.pty.Resize(int(cols), int(rows))
}
func (t *windowsTerminal) KillWait() int {
	if t.handle == 0 {
		return -1
	}
	code := waitProcessExit(t.handle, 1500)
	_ = windows.CloseHandle(t.handle)
	t.handle = 0
	return code
}

// waitProcessExit reads the real ConPTY child exit code. After the user types
// `exit` in PowerShell the process is already dead — do not TerminateProcess,
// that would invent code 1. Only kill shells that outlive the console.
func waitProcessExit(handle windows.Handle, lingerMs uint32) int {
	_, _ = windows.WaitForSingleObject(handle, lingerMs)
	if code, ok := processExitCode(handle); ok {
		return code
	}
	_ = windows.TerminateProcess(handle, 1)
	_, _ = windows.WaitForSingleObject(handle, windows.INFINITE)
	if code, ok := processExitCode(handle); ok {
		return code
	}
	return 1
}

func processExitCode(handle windows.Handle) (int, bool) {
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return -1, false
	}
	if code == stillActive {
		return -1, false
	}
	return int(code), true
}
