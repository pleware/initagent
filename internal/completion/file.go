package completion

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
	path, err := SentinelPath(run.SentinelDir, run.RunID)
	if err != nil {
		return nil, err
	}

	out := make(chan Outcome, 1)
	go f.watch(ctx, path, out)
	return out, nil
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

	base := Outcome{
		Done:   true,
		Reason: "file",
		Trust:  TrustHigh,
	}

	var jsonData struct {
		ExitCode  int   `json:"exit_code"`
		Timestamp int64 `json:"timestamp"`
	}
	if err := json.Unmarshal(content, &jsonData); err == nil {
		base.ExitCode = jsonData.ExitCode
		base.Message = fmt.Sprintf("sentinel file at %s", path)
		return base, true
	}

	text := strings.TrimSpace(string(content))
	code, err := strconv.Atoi(text)
	if err != nil {
		base.ExitCode = 1
		base.Message = fmt.Sprintf("malformed sentinel at %s", path)
		return base, true
	}

	base.ExitCode = code
	base.Message = fmt.Sprintf("sentinel file at %s", path)
	return base, true
}
