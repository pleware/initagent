// Package completion provides pluggable detection for when a coding run finishes.
//
// Completion detection is a registry of resolvers behind one interface.
// The scheduler depends on the interface, never on a specific resolver.
// See Draft 12 for design rationale.
package completion

import (
	"context"
)

// Trust indicates the reliability of a completion signal.
type Trust string

const (
	TrustLow    Trust = "low"    // Heuristic (timeout, activity window)
	TrustMedium Trust = "medium" // Sentinel string in a stream the model can also print
	TrustHigh   Trust = "high"   // OS exit code, a per-run done file holding that code, or a protocol callback
)

// Outcome carries the result of a completion check.
// Reason and Trust travel with the outcome so a soft signal is auditable.
type Outcome struct {
	Done     bool   // true when the run is complete
	ExitCode int    // exit code (0 = success); only meaningful when Done=true
	Reason   string // "sentinel" | "process" | "file" | "callback" | "timeout"
	Trust    Trust  // reliability of this signal
	Message  string // optional human-readable detail
}

// LaunchMode describes how the coder CLI was started.
type LaunchMode string

const (
	LaunchSendKeys   LaunchMode = "send_keys"  // keystrokes into tmux; no OS exit code
	LaunchSupervised LaunchMode = "supervised" // connector owns child process; real exit status
)

// RunContext carries the minimal state a resolver needs to watch for completion.
// Real implementation will pull from scheduler state.
type RunContext struct {
	RunID       string
	WorkerID    string
	LaunchMode  LaunchMode
	OutputPath  string // path to terminal/log output file (for sentinel/file resolvers)
	ProcessID   int    // OS pid (for process resolver); 0 if not supervised
	SentinelDir string // directory of per-run done files: <SentinelDir>/<RunID>.done
}

// Resolver detects when a coding run completes.
// A resolver must not assume a launch mode. The file resolver works under both;
// the process resolver works only under supervised.
type Resolver interface {
	// Name returns a unique identifier for this resolver (e.g., "process", "sentinel").
	Name() string

	// Supports reports whether this resolver can work with the given launch mode.
	Supports(mode LaunchMode) bool

	// Watch monitors the run and emits Outcome when completion is detected.
	// The resolver should respect ctx cancellation and close the channel when done.
	// A resolver may emit multiple outcomes (e.g., a sentinel then a process exit);
	// the scheduler prioritizes by trust level.
	Watch(ctx context.Context, run RunContext) (<-chan Outcome, error)
}
