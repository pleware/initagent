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

// FileResolver watches for a .done file written by the coder CLI.
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
	// Works with all launch modes
	return true
}

func (f *FileResolver) Watch(ctx context.Context, run RunContext) (<-chan Outcome, error) {
	if run.SentinelDir == "" {
		return nil, fmt.Errorf("file resolver requires SentinelDir")
	}

	out := make(chan Outcome, 1)
	go f.watch(ctx, run, out)
	return out, nil
}

func (f *FileResolver) watch(ctx context.Context, run RunContext, out chan<- Outcome) {
	defer close(out)

	interval := f.PollInterval
	if interval == 0 {
		interval = 1 * time.Second
	}

	sentinelPath := filepath.Join(run.SentinelDir, ".done")
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if outcome, found := f.checkSentinel(sentinelPath); found {
				out <- outcome
				return
			}
		}
	}
}

// checkSentinel reads and parses the sentinel file if it exists.
func (f *FileResolver) checkSentinel(path string) (Outcome, bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Outcome{}, false
		}
		// File exists but can't read - treat as incomplete
		return Outcome{}, false
	}

	// Try JSON first (matches fakecoder --sentinel-format=json)
	var jsonData struct {
		ExitCode  int   `json:"exit_code"`
		Timestamp int64 `json:"timestamp"`
	}
	if err := json.Unmarshal(content, &jsonData); err == nil {
		return Outcome{
			Done:     true,
			ExitCode: jsonData.ExitCode,
			Reason:   "file",
			Trust:    TrustMedium,
			Message:  fmt.Sprintf("sentinel file at %s", path),
		}, true
	}

	// Fall back to plain text (single line with exit code)
	text := strings.TrimSpace(string(content))
	code, err := strconv.Atoi(text)
	if err != nil {
		// Malformed sentinel - treat as failure
		return Outcome{
			Done:     true,
			ExitCode: 1,
			Reason:   "file",
			Trust:    TrustMedium,
			Message:  fmt.Sprintf("malformed sentinel at %s", path),
		}, true
	}

	return Outcome{
		Done:     true,
		ExitCode: code,
		Reason:   "file",
		Trust:    TrustMedium,
		Message:  fmt.Sprintf("sentinel file at %s", path),
	}, true
}
