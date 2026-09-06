package completion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pleware/initagent/internal/brand"
)

// FileResolver watches for a per-run done file written by the wrapper.
// Works with both LaunchSendKeys and LaunchSupervised.
type FileResolver struct {
	// PollInterval is how often to check for the sentinel file.
	// Defaults to 1 second if zero.
	PollInterval time.Duration
}

func init() {
	Register(&FileResolver{})
}

func (f *FileResolver) Name() string {
	return "file"
}

func (f *FileResolver) Supports(mode LaunchMode) bool {
	return true
}

// SentinelPath is the one place that names the done file for a run.
// The file is <SentinelDir>/<RunID>.done, matching Draft 12's
// `.initagent/runs/<run-…>.done`. Two runs on one worker therefore
// cannot share a path. RunID must be a single path element so a hostile
// value cannot walk out of SentinelDir.
func SentinelPath(dir, runID string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("file resolver requires SentinelDir")
	}
	if runID == "" {
		return "", fmt.Errorf("file resolver requires RunID")
	}
	if strings.ContainsAny(runID, `/\`) {
		return "", fmt.Errorf("file resolver: RunID must be a single path element")
	}
	return filepath.Join(dir, runID+".done"), nil
}

func (f *FileResolver) Watch(ctx context.Context, run RunContext) (<-chan Outcome, error) {
	if strings.TrimSpace(run.DoneBody) != "" {
		out := make(chan Outcome, 1)
		out <- parseDone([]byte(run.DoneBody), "worker done file")
		close(out)
		return out, nil
	}

	path, err := SentinelPath(run.SentinelDir, run.RunID)
	if err != nil {
		return nil, err
	}

	out := make(chan Outcome, 1)
	go f.watch(ctx, path, out)
	return out, nil
}

// RunsDir is `.initagent/runs` under home, matching Draft 12.
func RunsDir(home string) string {
	return filepath.Join(home, brand.ConfigDir, "runs")
}

// WriteDone writes a plain-integer done file for runID. The wrapper and the
// supervised process path both call this so a reconnect can recover the exit.
func WriteDone(dir, runID string, exit int) error {
	path, err := SentinelPath(dir, runID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, fmt.Appendf(nil, "%d\n", exit), 0o600)
}

// ReadDone reports whether a done file already exists for runID.
func ReadDone(dir, runID string) (Outcome, bool) {
	path, err := SentinelPath(dir, runID)
	if err != nil {
		return Outcome{}, false
	}
	return checkSentinel(path)
}

func (f *FileResolver) watch(ctx context.Context, path string, out chan<- Outcome) {
	defer close(out)

	// Check once before waiting. Tests that write the file first then
	// call Watch must not depend on a ticker tick — that branch made
	// owned coverage move between runs of the same commit.
	if outcome, found := checkSentinel(path); found {
		out <- outcome
		return
	}

	interval := f.PollInterval
	if interval == 0 {
		interval = 1 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if outcome, found := checkSentinel(path); found {
				out <- outcome
				return
			}
		}
	}
}

func checkSentinel(path string) (Outcome, bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Outcome{}, false
	}
	if len(bytes.TrimSpace(content)) == 0 {
		return Outcome{}, false
	}
	return parseDone(content, path), true
}

func parseDone(content []byte, source string) Outcome {
	base := Outcome{
		Done:    true,
		Reason:  "file",
		Trust:   TrustHigh,
		Message: fmt.Sprintf("sentinel file at %s", source),
	}

	text := strings.TrimSpace(string(content))
	if code, err := strconv.Atoi(text); err == nil {
		base.ExitCode = code
		return base
	}

	var jsonData struct {
		ExitCode  int   `json:"exit_code"`
		Timestamp int64 `json:"timestamp"`
	}
	if err := json.Unmarshal(content, &jsonData); err == nil {
		base.ExitCode = jsonData.ExitCode
		return base
	}

	base.ExitCode = 1
	base.Message = fmt.Sprintf("malformed sentinel at %s", source)
	return base
}
