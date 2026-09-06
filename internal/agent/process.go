package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"time"

	"github.com/pleware/initagent/internal/completion"
	"github.com/pleware/initagent/internal/protocol"
)

func (a *Agent) handleProcessStart(m protocol.Msg) {
	var req protocol.ProcessStart
	if err := json.Unmarshal(m.Data, &req); err != nil {
		a.reply(m.Id, nil, err)
		return
	}
	a.execStarted()
	defer a.execDone()
	a.reply(m.Id, a.processCommand(req), nil)
}

func (a *Agent) processCommand(req protocol.ProcessStart) protocol.ProcessResult {
	if outcome, ok := a.recoverDone(req.RunID); ok {
		return protocol.ProcessResult{ExitCode: outcome.ExitCode}
	}

	timeout := time.Duration(req.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	timeout = min(timeout, 10*time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := shellCommandContext(ctx, req.Command, false)
	if req.Cwd != "" {
		cmd.Dir = req.Cwd
	}
	if err := cmd.Start(); err != nil {
		return protocol.ProcessResult{ExitCode: -1}
	}
	res := protocol.ProcessResult{Pid: cmd.Process.Pid}
	err := cmd.Wait()
	if ctx.Err() == context.DeadlineExceeded {
		res.ExitCode = -1
		return res
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
	} else if err != nil {
		res.ExitCode = -1
	}
	if dir, dirErr := a.resolvedRunsDir(); dirErr == nil && req.RunID != "" {
		_ = completion.WriteDone(dir, req.RunID, res.ExitCode)
	}
	return res
}
