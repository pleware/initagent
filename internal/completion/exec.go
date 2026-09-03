package completion

import (
	"context"
	"fmt"
)

// ExecResult carries the completion payload of a supervised one-shot exec.
// The gateway fills it from the agent's TypeResult reply; the exec resolver
// surfaces it as an Outcome instead of polling a pid or a file.
type ExecResult struct {
	ExitCode int
}

// ExecResolver reports completion for a supervised exec whose exit code
// arrives as a protocol callback (TypeExec -> TypeResult). It is the
// Milestone 0 resolver: the only mechanism the current protocol can drive
// without a supervised-launch request (04).
type ExecResolver struct{}

func init() {
	Register(&ExecResolver{})
}

// Name returns the resolver's registry key.
func (e *ExecResolver) Name() string {
	return "exec"
}

// Supports reports that the exec resolver works only for a supervised run:
// the exit code comes back over the protocol, so there is nothing to watch
// under send_keys.
func (e *ExecResolver) Supports(mode LaunchMode) bool {
	return mode == LaunchSupervised
}

// Watch emits the exec result as an Outcome immediately. There is no
// goroutine: the completion is already known when Watch is called, which is
// what distinguishes the protocol callback from polling a pid or a file.
func (e *ExecResolver) Watch(_ context.Context, run RunContext) (<-chan Outcome, error) {
	if run.Exec == nil {
		return nil, fmt.Errorf("exec resolver requires an exec result")
	}
	out := make(chan Outcome, 1)
	out <- Outcome{
		Done:     true,
		ExitCode: run.Exec.ExitCode,
		Reason:   "exec",
		Trust:    TrustHigh,
		Message:  fmt.Sprintf("exec exit code %d", run.Exec.ExitCode),
	}
	close(out)
	return out, nil
}
