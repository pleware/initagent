package completion

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const nonceBytes = 16

// markerRe matches the wrapper line from WrapUnix / WrapPowerShell.
// The nonce is hex so the pattern cannot be confused with a shell metachar.
var markerRe = regexp.MustCompile(`<<<initagent ([a-f0-9]{16,64}) exit=(-?\d+)>>>`)

// SentinelResolver watches a stream (or a file of captured output) for the
// per-run marker the launch wrapper prints. TrustMedium: the model can also
// print the same bytes, which is why the nonce is minted per run and never
// placed in the prompt.
type SentinelResolver struct {
	PollInterval time.Duration
}

func init() {
	Register(&SentinelResolver{})
}

func (s *SentinelResolver) Name() string {
	return "sentinel"
}

func (s *SentinelResolver) Supports(mode LaunchMode) bool {
	return mode == LaunchSendKeys
}

func (s *SentinelResolver) Watch(ctx context.Context, run RunContext) (<-chan Outcome, error) {
	if !ValidNonce(run.Nonce) {
		return nil, fmt.Errorf("sentinel resolver requires a nonce")
	}
	if run.Output == "" && run.OutputPath == "" {
		return nil, fmt.Errorf("sentinel resolver requires Output or OutputPath")
	}

	out := make(chan Outcome, 1)
	if run.Output != "" {
		code, ok := Match(run.Output, run.Nonce)
		if !ok {
			return nil, fmt.Errorf("sentinel marker not found")
		}
		out <- sentinelOutcome(code)
		close(out)
		return out, nil
	}

	go s.watchFile(ctx, run, out)
	return out, nil
}

func (s *SentinelResolver) watchFile(ctx context.Context, run RunContext, out chan<- Outcome) {
	defer close(out)

	if outcome, ok := readSentinelFile(run.OutputPath, run.Nonce); ok {
		out <- outcome
		return
	}

	interval := cmp.Or(s.PollInterval, time.Second)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if outcome, ok := readSentinelFile(run.OutputPath, run.Nonce); ok {
				out <- outcome
				return
			}
		}
	}
}

func readSentinelFile(path, nonce string) (Outcome, bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Outcome{}, false
	}
	code, ok := Match(string(content), nonce)
	if !ok {
		return Outcome{}, false
	}
	return sentinelOutcome(code), true
}

func sentinelOutcome(exit int) Outcome {
	return Outcome{
		Done:     true,
		ExitCode: exit,
		Reason:   "sentinel",
		Trust:    TrustMedium,
		Message:  fmt.Sprintf("sentinel exit code %d", exit),
	}
}

// MintNonce returns a hex nonce for one run. The gateway mints it and the
// agent only interpolates it into the wrapper printf — it is not a model
// prompt.
func MintNonce() (string, error) {
	var b [nonceBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// ValidNonce reports whether s is a gateway-minted hex nonce.
func ValidNonce(s string) bool {
	if len(s) < 16 || len(s) > 64 {
		return false
	}
	for i := range len(s) {
		c := s[i]
		if c >= '0' && c <= '9' || c >= 'a' && c <= 'f' {
			continue
		}
		return false
	}
	return true
}

// Marker is the line the wrapper prints after the command.
func Marker(nonce string, exit int) string {
	return fmt.Sprintf("<<<initagent %s exit=%d>>>", nonce, exit)
}

// Match returns the exit code of the last marker whose nonce matches.
func Match(output, nonce string) (int, bool) {
	if !ValidNonce(nonce) {
		return 0, false
	}
	matches := markerRe.FindAllStringSubmatch(output, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		if matches[i][1] != nonce {
			continue
		}
		code, err := strconv.Atoi(matches[i][2])
		if err != nil {
			continue
		}
		return code, true
	}
	return 0, false
}

// WrapUnix wraps command so a POSIX pane prints Marker after it exits.
func WrapUnix(command, nonce string) string {
	return WrapUnixDone(command, nonce, "")
}

// WrapUnixDone is WrapUnix plus a write of the exit code to donePath.
// Sentinel and file are one wrapper, two signals (Draft 12). An empty
// donePath keeps the stream-only wrap.
func WrapUnixDone(command, nonce, donePath string) string {
	if donePath == "" {
		return "{ " + command + "; printf '\\n<<<initagent " + nonce + " exit=%d>>>\\n' $?; }"
	}
	quoted := shellSingleQuote(filepath.ToSlash(donePath))
	dir := shellSingleQuote(filepath.ToSlash(filepath.Dir(donePath)))
	return "{ " + command + "; _ia_code=$?; printf '\\n<<<initagent " + nonce + " exit=%d>>>\\n' \"$_ia_code\"; mkdir -p " + dir + " && printf '%d\\n' \"$_ia_code\" > " + quoted + "; }"
}

// WrapPowerShell wraps command for a PowerShell pane.
func WrapPowerShell(command, nonce string) string {
	return WrapPowerShellDone(command, nonce, "")
}

// WrapPowerShellDone is WrapPowerShell plus a write of the exit code to donePath.
func WrapPowerShellDone(command, nonce, donePath string) string {
	marker := command + `; Write-Output ("` + "`n" + "<<<initagent " + nonce + ` exit=$LASTEXITCODE>>>")`
	if donePath == "" {
		return marker
	}
	quoted := powershellSingleQuote(donePath)
	dir := powershellSingleQuote(filepath.Dir(donePath))
	return marker + `; New-Item -ItemType Directory -Force -Path ` + dir + ` | Out-Null; Set-Content -LiteralPath ` + quoted + ` -Value $LASTEXITCODE`
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, `'`, `'\''`) + "'"
}

func powershellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, `'`, `''`) + "'"
}

// WrapCommand picks the wrapper for this agent's OS. tmux send_keys always
// uses WrapUnix: the pane is a Unix shell even when the connector is mixed.
func WrapCommand(command, nonce string) string {
	if runtime.GOOS == "windows" {
		return WrapPowerShell(command, nonce)
	}
	return WrapUnix(command, nonce)
}
