package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	binaryPath string
	buildOnce  sync.Once
)

// buildBinary builds fakecoder binary once for all tests.
// go run doesn't preserve exit codes properly (always returns 1 for non-zero exits).
func buildBinary(t *testing.T) string {
	buildOnce.Do(func() {
		tmp := t.TempDir()
		bin := filepath.Join(tmp, "fakecoder")
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		cmd := exec.Command("go", "build", "-o", bin, ".")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build failed: %v\n%s", err, out)
		}
		binaryPath = bin
	})
	return binaryPath
}

func TestExitCode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		code int
	}{
		{"zero", 0},
		{"success", 0},
		{"failure", 1},
		{"custom", 42},
	}

	binary := buildBinary(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cmd := exec.Command(binary, "--exit", fmt.Sprintf("%d", tc.code))
			err := cmd.Run()
			if tc.code == 0 {
				if err != nil {
					t.Fatalf("expected success, got: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected non-zero exit, got success")
				}
				exitErr, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatalf("expected ExitError, got: %T", err)
				}
				if exitErr.ExitCode() != tc.code {
					t.Fatalf("exit code = %d, want %d", exitErr.ExitCode(), tc.code)
				}
			}
		})
	}
}

func TestAfterDelay(t *testing.T) {
	t.Parallel()
	binary := buildBinary(t)
	start := time.Now()
	cmd := exec.Command(binary, "--after", "0.3")
	if err := cmd.Run(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 250*time.Millisecond {
		t.Fatalf("elapsed %v < 250ms (delay didn't wait)", elapsed)
	}
	// go run overhead can be significant, allow up to 5s
	if elapsed > 5*time.Second {
		t.Fatalf("elapsed %v > 5s (hung or too slow)", elapsed)
	}
}

func TestWriteSentinelPlain(t *testing.T) {
	t.Parallel()
	binary := buildBinary(t)
	tmp := t.TempDir()
	sentinel := filepath.Join(tmp, "done.txt")

	cmd := exec.Command(binary, "--exit", "3", "--write-sentinel", sentinel)
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected exit 3, got success")
	}
	exitErr := err.(*exec.ExitError)
	if exitErr.ExitCode() != 3 {
		t.Fatalf("exit code = %d, want 3", exitErr.ExitCode())
	}

	content, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}
	got := strings.TrimSpace(string(content))
	if got != "3" {
		t.Fatalf("sentinel content = %q, want %q", got, "3")
	}
}

func TestWriteSentinelJSON(t *testing.T) {
	t.Parallel()
	binary := buildBinary(t)
	tmp := t.TempDir()
	sentinel := filepath.Join(tmp, "done.json")

	cmd := exec.Command(binary, "--exit", "5", "--write-sentinel", sentinel, "--sentinel-format", "json")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected exit 5, got success")
	}

	content, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}
	got := string(content)
	if !strings.Contains(got, `"exit_code": 5`) {
		t.Fatalf("sentinel missing exit_code=5, got: %s", got)
	}
	if !strings.Contains(got, `"timestamp":`) {
		t.Fatalf("sentinel missing timestamp, got: %s", got)
	}
}

func TestWriteSentinelCreatesParentDir(t *testing.T) {
	t.Parallel()
	binary := buildBinary(t)
	tmp := t.TempDir()
	sentinel := filepath.Join(tmp, "subdir", "nested", "done.txt")

	cmd := exec.Command(binary, "--write-sentinel", sentinel)
	if err := cmd.Run(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("sentinel not created: %v", err)
	}
}

func TestHangAndKill(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("signal handling not reliable on Windows in this test harness")
	}
	t.Parallel()
	binary := buildBinary(t)
	cmd := exec.Command(binary, "--hang")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Give it time to start and hang
	time.Sleep(500 * time.Millisecond)

	// Send SIGTERM
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("signal failed: %v", err)
	}

	// Wait for graceful exit
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		// Should exit 0 after catching signal
		if err != nil {
			t.Fatalf("expected clean exit after signal, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Fatal("process didn't exit after signal within 5s")
	}
}

func TestInvalidSentinelFormat(t *testing.T) {
	t.Parallel()
	binary := buildBinary(t)
	tmp := t.TempDir()
	sentinel := filepath.Join(tmp, "done.txt")

	cmd := exec.Command(binary, "--write-sentinel", sentinel, "--sentinel-format", "xml")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected failure for invalid format, got success")
	}
	exitErr := err.(*exec.ExitError)
	// Should exit 1 (sentinel write failure)
	if exitErr.ExitCode() != 1 {
		t.Fatalf("exit code = %d, want 1", exitErr.ExitCode())
	}
}

// Direct unit tests for internal functions (to get coverage).

func TestWriteSentinelFile_Plain(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")

	err := writeSentinelFile(path, 7, "plain")
	if err != nil {
		t.Fatalf("writeSentinelFile failed: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	got := strings.TrimSpace(string(content))
	if got != "7" {
		t.Fatalf("got %q, want %q", got, "7")
	}
}

func TestWriteSentinelFile_JSON(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.json")

	err := writeSentinelFile(path, 9, "json")
	if err != nil {
		t.Fatalf("writeSentinelFile failed: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	got := string(content)
	if !strings.Contains(got, `"exit_code": 9`) {
		t.Errorf("missing exit_code=9, got: %s", got)
	}
	if !strings.Contains(got, `"timestamp":`) {
		t.Errorf("missing timestamp, got: %s", got)
	}
}

func TestWriteSentinelFile_InvalidFormat(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")

	err := writeSentinelFile(path, 0, "yaml")
	if err == nil {
		t.Fatal("expected error for invalid format, got nil")
	}
	if !strings.Contains(err.Error(), "unknown sentinel format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_ExitCode(t *testing.T) {
	// Save original flags
	origExit := *exitCode
	origAfter := *afterSec
	origHang := *hang
	origSentinel := *writeSentinel
	defer func() {
		*exitCode = origExit
		*afterSec = origAfter
		*hang = origHang
		*writeSentinel = origSentinel
	}()

	*exitCode = 13
	*afterSec = 0
	*hang = false
	*writeSentinel = ""

	code := run()
	if code != 13 {
		t.Fatalf("run() = %d, want 13", code)
	}
}

func TestRun_WithSentinel(t *testing.T) {
	origExit := *exitCode
	origAfter := *afterSec
	origHang := *hang
	origSentinel := *writeSentinel
	origFormat := *sentinelFormat
	defer func() {
		*exitCode = origExit
		*afterSec = origAfter
		*hang = origHang
		*writeSentinel = origSentinel
		*sentinelFormat = origFormat
	}()

	tmp := t.TempDir()
	sentinelPath := filepath.Join(tmp, "run-sentinel.txt")

	*exitCode = 11
	*afterSec = 0
	*hang = false
	*writeSentinel = sentinelPath
	*sentinelFormat = "plain"

	code := run()
	if code != 11 {
		t.Fatalf("run() = %d, want 11", code)
	}

	content, err := os.ReadFile(sentinelPath)
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}
	got := strings.TrimSpace(string(content))
	if got != "11" {
		t.Fatalf("sentinel = %q, want %q", got, "11")
	}
}

func TestRun_WithDelay(t *testing.T) {
	origExit := *exitCode
	origAfter := *afterSec
	origHang := *hang
	origSentinel := *writeSentinel
	defer func() {
		*exitCode = origExit
		*afterSec = origAfter
		*hang = origHang
		*writeSentinel = origSentinel
	}()

	*exitCode = 0
	*afterSec = 0.1
	*hang = false
	*writeSentinel = ""

	start := time.Now()
	code := run()
	elapsed := time.Since(start)

	if code != 0 {
		t.Fatalf("run() = %d, want 0", code)
	}
	if elapsed < 90*time.Millisecond {
		t.Fatalf("elapsed %v < 90ms (delay didn't wait)", elapsed)
	}
}

func TestRun_SentinelWriteFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hard to trigger write failure on Windows without admin")
	}
	origExit := *exitCode
	origAfter := *afterSec
	origHang := *hang
	origSentinel := *writeSentinel
	origFormat := *sentinelFormat
	defer func() {
		*exitCode = origExit
		*afterSec = origAfter
		*hang = origHang
		*writeSentinel = origSentinel
		*sentinelFormat = origFormat
	}()

	// Use /dev/null on Unix (cannot create file there)
	*exitCode = 0
	*afterSec = 0
	*hang = false
	*writeSentinel = "/dev/null/subdir/file.txt"
	*sentinelFormat = "plain"

	code := run()
	if code != 1 {
		t.Fatalf("run() = %d, want 1 (sentinel write failure)", code)
	}
}

func TestRun_SentinelInvalidFormat(t *testing.T) {
	origExit := *exitCode
	origAfter := *afterSec
	origHang := *hang
	origSentinel := *writeSentinel
	origFormat := *sentinelFormat
	defer func() {
		*exitCode = origExit
		*afterSec = origAfter
		*hang = origHang
		*writeSentinel = origSentinel
		*sentinelFormat = origFormat
	}()

	tmp := t.TempDir()
	*exitCode = 0
	*afterSec = 0
	*hang = false
	*writeSentinel = filepath.Join(tmp, "test.txt")
	*sentinelFormat = "xml"

	code := run()
	if code != 1 {
		t.Fatalf("run() = %d, want 1 (invalid format)", code)
	}
}
