package protocol

// Additive run-lifecycle messages. New behaviour lives here rather than in
// protocol.go so Overseer merges stay a one-line dispatch hook (draft 04).

const (
	// TypeProcessStart asks the agent to start a supervised child and wait
	// for it. Distinct from TypeExec: no 256KB cockpit dump is required, and
	// the gateway finishes through the process resolver.
	TypeProcessStart = "process.start"
	// TypeRunSendKeys types a wrapped command into a tmux pane and waits for
	// the sentinel marker. The nonce is minted by the gateway.
	TypeRunSendKeys = "run.send_keys"
)

// ProcessStart is a supervised coding-CLI launch.
type ProcessStart struct {
	Command    string `json:"command"`
	Cwd        string `json:"cwd,omitempty"`
	TimeoutSec int    `json:"timeoutSec,omitempty"`
	RunID      string `json:"runId,omitempty"`
}

// ProcessResult is the reply to process.start.
type ProcessResult struct {
	Pid      int `json:"pid,omitempty"`
	ExitCode int `json:"exitCode"`
}

// RunSendKeys types Command into Session (created from RunID when empty)
// after wrapping it with Nonce. The agent replies when the marker appears
// or the timeout fires.
type RunSendKeys struct {
	Session    string `json:"session,omitempty"`
	Command    string `json:"command"`
	Nonce      string `json:"nonce"`
	Cwd        string `json:"cwd,omitempty"`
	TimeoutSec int    `json:"timeoutSec,omitempty"`
	RunID      string `json:"runId,omitempty"`
}

// RunSendKeysResult is the reply to run.send_keys.
type RunSendKeysResult struct {
	ExitCode int    `json:"exitCode"`
	Output   string `json:"output,omitempty"`
}
