package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// These tests exercise the wiring this package owns — argument handling,
// signal delivery, and the process exit status. Behaviour itself is covered
// by internal/fakecoder.

// binaryPath is the fakecoder binary built once for the whole package. go run
// does not preserve exit codes (it always reports 1 for a non-zero child
// exit), and the binary must outlive the parallel tests that spawn it.
var binaryPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fakecoder-bin")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mkdtemp: %v\n", err)
		os.Exit(2)
	}

	binaryPath = filepath.Join(dir, "fakecoder")
	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", binaryPath, ".").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(2)
	}

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// requireExitCode asserts the command failed with a specific process status.
func requireExitCode(t *testing.T, err error, want int) {
	t.Helper()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *exec.ExitError, got: %v", err)
	}
	if exitErr.ExitCode() != want {
		t.Fatalf("exit code = %d, want %d", exitErr.ExitCode(), want)
	}
}

func TestExitCodeReachesTheProcessStatus(t *testing.T) {
	t.Parallel()
	for _, code := range []int{0, 1, 42} {
		t.Run("exit-"+strconv.Itoa(code), func(t *testing.T) {
			t.Parallel()
			err := exec.Command(binaryPath, "--exit", strconv.Itoa(code)).Run()

			if code == 0 {
				if err != nil {
					t.Fatalf("expected success, got: %v", err)
				}
				return
			}
			requireExitCode(t, err, code)
		})
	}
}

func TestSentinelIsWrittenByTheProcess(t *testing.T) {
	t.Parallel()

	sentinel := filepath.Join(t.TempDir(), "run", ".done")

	err := exec.Command(binaryPath, "--exit", "3", "--write-sentinel", sentinel).Run()
	requireExitCode(t, err, 3)

	content, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}
	if got := strings.TrimSpace(string(content)); got != "3" {
		t.Fatalf("sentinel content = %q, want %q", got, "3")
	}
}

func TestUnknownFlagExitsTwo(t *testing.T) {
	t.Parallel()

	requireExitCode(t, exec.Command(binaryPath, "--not-a-flag").Run(), 2)
}

func TestHangExitsCleanlyOnSignal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Process.Signal(os.Interrupt) is unsupported on Windows")
	}
	t.Parallel()

	cmd := exec.Command(binaryPath, "--hang")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Give the child time to install its signal handler and block.
	time.Sleep(500 * time.Millisecond)
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("signal failed: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected clean exit after signal, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("process did not exit within 5s of the signal")
	}
}
