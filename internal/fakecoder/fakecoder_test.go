package fakecoder

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseArgs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		want Config
	}{
		{
			name: "defaults",
			args: nil,
			want: Config{SentinelFormat: FormatPlain},
		},
		{
			name: "exit code",
			args: []string{"--exit", "42"},
			want: Config{ExitCode: 42, SentinelFormat: FormatPlain},
		},
		{
			name: "fractional delay",
			args: []string{"--after", "0.25"},
			want: Config{After: 250 * time.Millisecond, SentinelFormat: FormatPlain},
		},
		{
			name: "hang",
			args: []string{"--hang"},
			want: Config{Hang: true, SentinelFormat: FormatPlain},
		},
		{
			name: "json sentinel",
			args: []string{"--write-sentinel", "out/.done", "--sentinel-format", "json"},
			want: Config{SentinelPath: "out/.done", SentinelFormat: FormatJSON},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseArgs(tc.args, io.Discard)
			if err != nil {
				t.Fatalf("ParseArgs(%q) returned error: %v", tc.args, err)
			}
			if got != tc.want {
				t.Fatalf("ParseArgs(%q) = %+v, want %+v", tc.args, got, tc.want)
			}
		})
	}
}

func TestParseArgsRejectsUnknownFlag(t *testing.T) {
	t.Parallel()

	var usage bytes.Buffer
	if _, err := ParseArgs([]string{"--nope"}, &usage); err == nil {
		t.Fatal("ParseArgs accepted an unknown flag")
	}
	if usage.Len() == 0 {
		t.Fatal("ParseArgs wrote no usage text for an unknown flag")
	}
}

func TestParseArgsReportsHelpRequest(t *testing.T) {
	t.Parallel()

	_, err := ParseArgs([]string{"-h"}, io.Discard)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("ParseArgs(-h) returned %v, want flag.ErrHelp", err)
	}
}

func TestSentinelContent(t *testing.T) {
	t.Parallel()

	now := time.Unix(1756857600, 0)

	cases := []struct {
		name    string
		format  string
		code    int
		want    string
		wantErr bool
	}{
		{name: "plain", format: FormatPlain, code: 7, want: "7\n"},
		{name: "json", format: FormatJSON, code: 9, want: "{\"exit_code\": 9, \"timestamp\": 1756857600}\n"},
		{name: "unknown", format: "xml", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := SentinelContent(tc.code, tc.format, now)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("SentinelContent(%q) succeeded, want error", tc.format)
				}
				if !strings.Contains(err.Error(), "unknown sentinel format") {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("SentinelContent(%q) returned error: %v", tc.format, err)
			}
			if got != tc.want {
				t.Fatalf("SentinelContent(%q) = %q, want %q", tc.format, got, tc.want)
			}
		})
	}
}

func TestWriteSentinelCreatesParentDirs(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "run", ".done")
	if err := WriteSentinel(path, 3, FormatPlain, time.Now()); err != nil {
		t.Fatalf("WriteSentinel returned error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}
	if got := strings.TrimSpace(string(content)); got != "3" {
		t.Fatalf("sentinel content = %q, want %q", got, "3")
	}
}

// blockedSentinelPath returns a path whose parent is a regular file, so
// MkdirAll fails on every platform without needing special privileges.
func blockedSentinelPath(t *testing.T) string {
	t.Helper()

	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	return filepath.Join(blocker, "run", ".done")
}

func TestWriteSentinelReportsUnwritablePath(t *testing.T) {
	t.Parallel()

	err := WriteSentinel(blockedSentinelPath(t), 0, FormatPlain, time.Now())
	if err == nil {
		t.Fatal("WriteSentinel succeeded under a regular file")
	}
	if !strings.Contains(err.Error(), "mkdir") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteSentinelReportsWriteFailure(t *testing.T) {
	t.Parallel()

	// A directory as the target path: MkdirAll is satisfied, WriteFile is not.
	dir := t.TempDir()
	err := WriteSentinel(dir, 0, FormatPlain, time.Now())
	if err == nil {
		t.Fatal("WriteSentinel succeeded writing over a directory")
	}
	if !strings.Contains(err.Error(), "write") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunExitCode(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	code := Run(context.Background(), Config{ExitCode: 13}, &stderr)
	if code != 13 {
		t.Fatalf("Run returned %d, want 13", code)
	}
	if !strings.Contains(stderr.String(), "exiting with code 13") {
		t.Fatalf("stderr missing exit report: %q", stderr.String())
	}
}

func TestRunWaitsForDelay(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	start := time.Now()
	code := Run(context.Background(), Config{After: 100 * time.Millisecond}, &stderr)
	elapsed := time.Since(start)

	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if elapsed < 90*time.Millisecond {
		t.Fatalf("Run returned after %v, expected to wait ~100ms", elapsed)
	}
}

func TestRunDelayIsCancellable(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var stderr bytes.Buffer
	start := time.Now()
	code := Run(ctx, Config{After: 10 * time.Second, ExitCode: 5}, &stderr)

	if code != 0 {
		t.Fatalf("Run returned %d, want 0 after cancellation", code)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Run waited %v after cancellation", elapsed)
	}
	if !strings.Contains(stderr.String(), "caught signal") {
		t.Fatalf("stderr missing signal report: %q", stderr.String())
	}
}

func TestRunHangsUntilContextCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	var stderr bytes.Buffer
	done := make(chan int, 1)

	go func() {
		done <- Run(ctx, Config{Hang: true, ExitCode: 7}, &stderr)
	}()

	select {
	case code := <-done:
		t.Fatalf("Run returned %d before cancellation", code)
	case <-time.After(100 * time.Millisecond):
	}

	cancel()

	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("Run returned %d after cancellation, want 0", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5s of cancellation")
	}

	if !strings.Contains(stderr.String(), "hanging forever") {
		t.Fatalf("stderr missing hang report: %q", stderr.String())
	}
}

func TestRunWritesSentinelBeforeExiting(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".done")
	var stderr bytes.Buffer

	cfg := Config{ExitCode: 11, SentinelPath: path, SentinelFormat: FormatPlain}
	if code := Run(context.Background(), cfg, &stderr); code != 11 {
		t.Fatalf("Run returned %d, want 11", code)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}
	if got := strings.TrimSpace(string(content)); got != "11" {
		t.Fatalf("sentinel content = %q, want %q", got, "11")
	}
}

func TestRunFailsOnUnwritableSentinel(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	cfg := Config{SentinelPath: blockedSentinelPath(t), SentinelFormat: FormatPlain}

	if code := Run(context.Background(), cfg, &stderr); code != 1 {
		t.Fatalf("Run returned %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "failed to write sentinel") {
		t.Fatalf("stderr missing failure report: %q", stderr.String())
	}
}

func TestRunFailsOnUnknownSentinelFormat(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	cfg := Config{SentinelPath: filepath.Join(t.TempDir(), ".done"), SentinelFormat: "xml"}

	if code := Run(context.Background(), cfg, &stderr); code != 1 {
		t.Fatalf("Run returned %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "unknown sentinel format") {
		t.Fatalf("stderr missing format error: %q", stderr.String())
	}
}
