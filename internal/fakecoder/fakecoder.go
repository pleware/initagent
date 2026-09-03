// Package fakecoder implements the behaviour of a stand-in coding CLI: it
// exits with a chosen code after a chosen delay, optionally writes a sentinel
// file, or hangs until its context is cancelled.
//
// It stands in for real coding CLIs in completion, scheduler, lease, and
// fence tests (Drafts 11, 12, 40) without requiring API keys or network.
// The signal handling and process exit live in cmd/fakecoder; everything
// here is a decision over explicit inputs.
package fakecoder

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Sentinel formats accepted by SentinelContent.
const (
	FormatPlain = "plain"
	FormatJSON  = "json"
)

// Config is the fully resolved behaviour of one fakecoder invocation.
type Config struct {
	ExitCode       int
	After          time.Duration
	Hang           bool
	SentinelPath   string
	SentinelFormat string
}

// ParseArgs resolves command-line arguments into a Config. It reports flag
// errors instead of exiting, so the parse is testable on its own. Usage and
// flag errors are written to usage; a help request returns flag.ErrHelp.
func ParseArgs(args []string, usage io.Writer) (Config, error) {
	fs := flag.NewFlagSet("fakecoder", flag.ContinueOnError)
	fs.SetOutput(usage)

	exitCode := fs.Int("exit", 0, "exit code to return")
	afterSec := fs.Float64("after", 0, "seconds to wait before exiting")
	hang := fs.Bool("hang", false, "hang until killed (ignores --after and --exit)")
	sentinelPath := fs.String("write-sentinel", "", "write a sentinel file with exit code before exiting")
	sentinelFormat := fs.String("sentinel-format", FormatPlain, "sentinel format: 'plain' (exit code only) or 'json'")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	return Config{
		ExitCode:       *exitCode,
		After:          time.Duration(*afterSec * float64(time.Second)),
		Hang:           *hang,
		SentinelPath:   *sentinelPath,
		SentinelFormat: *sentinelFormat,
	}, nil
}

// Run performs the configured behaviour and returns the exit code the process
// should use. Progress goes to stderr, matching a real CLI.
func Run(ctx context.Context, cfg Config, stderr io.Writer) int {
	if cfg.Hang {
		fmt.Fprintln(stderr, "fakecoder: hanging forever (send SIGTERM to stop)")
		<-ctx.Done()
		fmt.Fprintln(stderr, "fakecoder: caught signal, exiting")
		return 0
	}

	if cfg.After > 0 {
		fmt.Fprintf(stderr, "fakecoder: waiting %.2f seconds...\n", cfg.After.Seconds())
		select {
		case <-ctx.Done():
			fmt.Fprintln(stderr, "fakecoder: caught signal, exiting")
			return 0
		case <-time.After(cfg.After):
		}
	}

	if cfg.SentinelPath != "" {
		if err := WriteSentinel(cfg.SentinelPath, cfg.ExitCode, cfg.SentinelFormat, time.Now()); err != nil {
			fmt.Fprintf(stderr, "fakecoder: failed to write sentinel: %v\n", err)
			return 1
		}
		fmt.Fprintf(stderr, "fakecoder: wrote sentinel to %s\n", cfg.SentinelPath)
	}

	fmt.Fprintf(stderr, "fakecoder: exiting with code %d\n", cfg.ExitCode)
	return cfg.ExitCode
}

// SentinelContent renders the sentinel payload. It owns the set of valid
// formats; callers validate by calling it rather than keeping a second list.
func SentinelContent(code int, format string, now time.Time) (string, error) {
	switch format {
	case FormatPlain:
		return fmt.Sprintf("%d\n", code), nil
	case FormatJSON:
		return fmt.Sprintf("{\"exit_code\": %d, \"timestamp\": %d}\n", code, now.Unix()), nil
	default:
		return "", fmt.Errorf("unknown sentinel format: %q (use %q or %q)", format, FormatPlain, FormatJSON)
	}
}

// WriteSentinel renders and writes the sentinel file, creating parent
// directories so a caller may point at a fresh workspace path.
func WriteSentinel(path string, code int, format string, now time.Time) error {
	content, err := SentinelContent(code, format, now)
	if err != nil {
		return err
	}

	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir: %w", err)
		}
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}
