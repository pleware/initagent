package agent

import (
	"context"
	"encoding/json"
	"fmt"
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

func (a *Agent) runSendKeys(req protocol.RunSendKeys) (protocol.RunSendKeysResult, error) {
	if !completion.ValidNonce(req.Nonce) {
		return protocol.RunSendKeysResult{}, fmt.Errorf("invalid sentinel nonce")
	}
	if strings.TrimSpace(req.Command) == "" {
		return protocol.RunSendKeysResult{}, fmt.Errorf("empty command")
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
				return protocol.RunSendKeysResult{ExitCode: code, Output: text}, nil
			}
		}
	}
}
