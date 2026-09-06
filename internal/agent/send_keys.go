package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/pleware/initagent/internal/completion"
	"github.com/pleware/initagent/internal/protocol"
)

func (a *Agent) handleRunSendKeys(m protocol.Msg) {
	var req protocol.RunSendKeys
	if err := json.Unmarshal(m.Data, &req); err != nil {
		a.reply(m.Id, nil, err)
		return
	}
	a.execStarted()
	defer a.execDone()
	res, err := a.runSendKeys(req)
	a.reply(m.Id, res, err)
}

func (a *Agent) resolvedRunsDir() (string, error) {
	if a.runsDir != "" {
		return a.runsDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return completion.RunsDir(home), nil
}

func (a *Agent) recoverDone(runID string) (completion.Outcome, bool) {
	if runID == "" {
		return completion.Outcome{}, false
	}
	dir, err := a.resolvedRunsDir()
	if err != nil {
		return completion.Outcome{}, false
	}
	return completion.ReadDone(dir, runID)
}

func doneFileBody(exit int) string {
	return fmt.Sprintf("%d\n", exit)
}

func (a *Agent) runSendKeys(req protocol.RunSendKeys) (protocol.RunSendKeysResult, error) {
	if !completion.ValidNonce(req.Nonce) {
		return protocol.RunSendKeysResult{}, fmt.Errorf("invalid sentinel nonce")
	}
	if strings.TrimSpace(req.Command) == "" {
		return protocol.RunSendKeysResult{}, fmt.Errorf("empty command")
	}
	if outcome, ok := a.recoverDone(req.RunID); ok {
		return protocol.RunSendKeysResult{ExitCode: outcome.ExitCode, DoneFile: doneFileBody(outcome.ExitCode)}, nil
	}
	if !tmuxAvailable() {
		return protocol.RunSendKeysResult{}, fmt.Errorf("tmux is required for send_keys")
	}
	name := req.Session
	if name == "" {
		name = req.RunID
	}
	if !sessionNameRe.MatchString(name) {
		return protocol.RunSendKeysResult{}, fmt.Errorf("invalid session name %q", name)
	}
	if err := exec.Command("tmux", "has-session", "-t", name).Run(); err != nil {
		if err := a.createSession(protocol.SessionCreate{Name: name, Cwd: req.Cwd}); err != nil {
			return protocol.RunSendKeysResult{}, err
		}
	}
	wrapped := completion.WrapUnix(req.Command, req.Nonce)
	donePath := ""
	if dir, err := a.resolvedRunsDir(); err == nil && req.RunID != "" {
		if path, err := completion.SentinelPath(dir, req.RunID); err == nil {
			donePath = path
			wrapped = completion.WrapUnixDone(req.Command, req.Nonce, donePath)
		}
	}
	if out, err := exec.Command("tmux", "send-keys", "-t", name, wrapped, "Enter").CombinedOutput(); err != nil {
		return protocol.RunSendKeysResult{}, fmt.Errorf("tmux send-keys: %s", strings.TrimSpace(string(out)))
	}

	timeout := time.Duration(req.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	timeout = min(timeout, 10*time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return protocol.RunSendKeysResult{}, fmt.Errorf("sentinel timeout")
		case <-ticker.C:
			out, err := exec.Command("tmux", "capture-pane", "-p", "-J", "-t", name).Output()
			if err != nil {
				continue
			}
			text := string(out)
			if code, ok := completion.Match(text, req.Nonce); ok {
				res := protocol.RunSendKeysResult{ExitCode: code, Output: text}
				if donePath != "" {
					if body, err := os.ReadFile(donePath); err == nil {
						res.DoneFile = string(body)
					}
				}
				return res, nil
			}
		}
	}
}
