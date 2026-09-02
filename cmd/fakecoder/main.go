// fakecoder is a minimal test harness that exits with a chosen code after a
// chosen delay, optionally writes a sentinel file, or hangs until killed.
//
// It stands in for real coding CLIs in completion, scheduler, lease, and
// fence tests (Drafts 11, 12, 40) without requiring API keys or network.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

var (
	exitCode       = flag.Int("exit", 0, "exit code to return")
	afterSec       = flag.Float64("after", 0, "seconds to wait before exiting")
	hang           = flag.Bool("hang", false, "hang forever until killed (ignores --after and --exit)")
	writeSentinel  = flag.String("write-sentinel", "", "write a sentinel file with exit code before exiting")
	sentinelFormat = flag.String("sentinel-format", "plain", "sentinel format: 'plain' (exit code only) or 'json'")
)

func main() {
	flag.Parse()
	os.Exit(run())
}

// run executes the fakecoder logic and returns the exit code.
func run() int {
	if *hang {
		fmt.Fprintln(os.Stderr, "fakecoder: hanging forever (send SIGTERM to stop)")
		// Wait for signal
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		fmt.Fprintln(os.Stderr, "fakecoder: caught signal, exiting")
		return 0
	}

	if *afterSec > 0 {
		fmt.Fprintf(os.Stderr, "fakecoder: waiting %.2f seconds...\n", *afterSec)
		time.Sleep(time.Duration(*afterSec * float64(time.Second)))
	}

	if *writeSentinel != "" {
		if err := writeSentinelFile(*writeSentinel, *exitCode, *sentinelFormat); err != nil {
			fmt.Fprintf(os.Stderr, "fakecoder: failed to write sentinel: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "fakecoder: wrote sentinel to %s\n", *writeSentinel)
	}

	fmt.Fprintf(os.Stderr, "fakecoder: exiting with code %d\n", *exitCode)
	return *exitCode
}

func writeSentinelFile(path string, code int, format string) error {
	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("mkdir: %w", err)
		}
	}

	var content string
	switch format {
	case "plain":
		content = fmt.Sprintf("%d\n", code)
	case "json":
		content = fmt.Sprintf(`{"exit_code": %d, "timestamp": %d}`+"\n", code, time.Now().Unix())
	default:
		return fmt.Errorf("unknown sentinel format: %q (use 'plain' or 'json')", format)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}
