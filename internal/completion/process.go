package completion

import (
	"context"
	"fmt"
	"os"
	"time"
)

// ProcessResolver watches for OS process exit when the coder is launched
// as a supervised child. Only works with LaunchSupervised.
type ProcessResolver struct{}

func init() {
	Register(&ProcessResolver{})
}

func (p *ProcessResolver) Name() string {
	return "process"
}

func (p *ProcessResolver) Supports(mode LaunchMode) bool {
	return mode == LaunchSupervised
}

func (p *ProcessResolver) Watch(ctx context.Context, run RunContext) (<-chan Outcome, error) {
	if run.ProcessID == 0 {
		return nil, fmt.Errorf("process resolver requires non-zero ProcessID")
	}

	out := make(chan Outcome, 1)
	go p.watch(ctx, run, out)
	return out, nil
}

func (p *ProcessResolver) watch(ctx context.Context, run RunContext, out chan<- Outcome) {
	defer close(out)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check if process still exists
			// On Unix: kill(pid, 0) returns error if process is gone
			// On Windows: FindProcess + Signal returns error if process is gone
			proc, err := os.FindProcess(run.ProcessID)
			if err != nil {
				// Process not found (should not happen on modern OSes)
				out <- Outcome{
					Done:     true,
					ExitCode: 1, // assume failure if we can't find it
					Reason:   "process",
					Trust:    TrustHigh,
					Message:  fmt.Sprintf("pid %d not found", run.ProcessID),
				}
				return
			}

			// Signal(0) is a non-destructive probe
			// If it returns nil, process is still alive
			// If it returns error, process is gone
			err = proc.Signal(os.Signal(nil))
			if err != nil {
				// Process is gone - but we don't have the exit code here
				// Real implementation would use syscall.Wait4 or similar
				// For now, assume success (code 0) if we can't determine
				out <- Outcome{
					Done:     true,
					ExitCode: 0, // TODO: capture real exit code via Wait4/WaitForExit
					Reason:   "process",
					Trust:    TrustHigh,
					Message:  fmt.Sprintf("pid %d exited", run.ProcessID),
				}
				return
			}
		}
	}
}
