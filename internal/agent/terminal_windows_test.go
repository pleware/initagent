//go:build windows

package agent

import (
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWaitProcessExitCatchesCmdExitCode(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("cmd", "/c", "exit", "7")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = cmd.Process.Kill()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(handle) })
	got := waitProcessExit(handle, 3000)
	_ = cmd.Wait()
	if got != 7 {
		t.Fatalf("exit code = %d, want 7", got)
	}
}
