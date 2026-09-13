package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pleware/initagent/internal/completion"
	"github.com/pleware/initagent/internal/protocol"
	"github.com/pleware/initagent/internal/scheduler"
)

const defaultExecTimeout = 60 * time.Second

// TaskView is the JSON shape for a task after enqueue or a run.
type TaskView struct {
	ID               string `json:"id"`
	ProjectID        string `json:"projectId"`
	State            string `json:"state"`
	Command          string `json:"command"`
	Launch           string `json:"launch,omitempty"`
	AssignedWorkerID string `json:"assignedWorkerId,omitempty"`
	ExitCode         int    `json:"exitCode"`
	Reason           string `json:"reason,omitempty"`
	Stdout           string `json:"stdout,omitempty"`
	Stderr           string `json:"stderr,omitempty"`
}

func viewTask(t scheduler.Task, stdout, stderr string) TaskView {
	return TaskView{
		ID:               t.ID,
		ProjectID:        t.ProjectID,
		State:            string(t.State),
		Command:          t.Command,
		Launch:           t.LaunchMode,
		AssignedWorkerID: t.AssignedWorkerID,
		ExitCode:         t.ExitCode,
		Reason:           t.Reason,
		Stdout:           stdout,
		Stderr:           stderr,
	}
}

type finishFunc func(to scheduler.TaskState, exit int, reason, stdout, stderr string) (TaskView, error)

// RunQueued claims the oldest queued task for workerID, runs it over the
// agent socket using the task's launch mode, and Finish-es the row from a
// completion-registry Outcome.
func (g *Gateway) RunQueued(ctx context.Context, projectID, workerID string) (TaskView, error) {
	if g.connForProject(projectID, workerID) == nil {
		return TaskView{}, ErrConnectorOffline
	}
	if g.draining(workerID) {
		return TaskView{}, ErrConnectorDraining
	}
	claimed, _, err := g.Claim(ctx, projectID, workerID)
	if err != nil {
		return TaskView{}, err
	}
	return g.runClaimed(ctx, claimed)
}

func (g *Gateway) runClaimed(ctx context.Context, claimed *scheduler.Task) (TaskView, error) {
	finish := func(to scheduler.TaskState, exit int, reason, stdout, stderr string) (TaskView, error) {
		err := g.store.Finish(context.WithoutCancel(ctx), claimed.ID, to, exit, reason)
		if err != nil {
			return TaskView{}, err
		}
		got, err := g.store.Task(context.WithoutCancel(ctx), claimed.ID)
		if err != nil {
			return TaskView{}, err
		}
		return viewTask(got, stdout, stderr), nil
	}

	if claimed.Command == "" {
		if err := g.store.UpdateState(ctx, claimed.ID, scheduler.TaskRunning, "empty command"); err != nil {
			return TaskView{}, err
		}
		return finish(scheduler.TaskFailed, 1, "empty command", "", "")
	}

	launch, err := scheduler.NormalizeLaunch(claimed.LaunchMode)
	if err != nil {
		if err := g.store.UpdateState(ctx, claimed.ID, scheduler.TaskRunning, err.Error()); err != nil {
			return TaskView{}, err
		}
		return finish(scheduler.TaskFailed, 1, err.Error(), "", "")
	}

	if err := g.store.UpdateState(ctx, claimed.ID, scheduler.TaskRunning, launch); err != nil {
		return TaskView{}, err
	}

	switch launch {
	case scheduler.LaunchProcess:
		return g.runProcess(ctx, claimed, finish)
	case scheduler.LaunchSendKeys:
		return g.runSendKeys(ctx, claimed, finish)
	default:
		return g.runExec(ctx, claimed, finish)
	}
}

func (g *Gateway) runExec(ctx context.Context, claimed *scheduler.Task, finish finishFunc) (TaskView, error) {
	res, err := g.execOn(ctx, claimed.AssignedWorkerID, claimed.Command)
	if err != nil {
		return finish(scheduler.TaskFailed, 1, err.Error(), "", "")
	}
	outcome, err := g.resolveExec(ctx, claimed, res)
	if err != nil {
		return finish(scheduler.TaskFailed, 1, err.Error(), "", "")
	}
	return finish(terminalState(outcome.ExitCode), outcome.ExitCode, outcome.Reason, res.Stdout, res.Stderr)
}

func (g *Gateway) runProcess(ctx context.Context, claimed *scheduler.Task, finish finishFunc) (TaskView, error) {
	res, err := g.processOn(ctx, claimed)
	if err != nil {
		return finish(scheduler.TaskFailed, 1, err.Error(), "", "")
	}
	outcome, err := g.resolveProcess(ctx, claimed, res)
	if err != nil {
		return finish(scheduler.TaskFailed, 1, err.Error(), "", "")
	}
	return finish(terminalState(outcome.ExitCode), outcome.ExitCode, outcome.Reason, "", "")
}

func (g *Gateway) runSendKeys(ctx context.Context, claimed *scheduler.Task, finish finishFunc) (TaskView, error) {
	nonce, err := completion.MintNonce()
	if err != nil {
		return finish(scheduler.TaskFailed, 1, err.Error(), "", "")
	}
	res, err := g.sendKeysOn(ctx, claimed, nonce)
	if err != nil {
		return finish(scheduler.TaskFailed, 1, err.Error(), "", "")
	}
	outcome, err := g.resolveSentinel(ctx, claimed, nonce, res)
	if err != nil {
		return finish(scheduler.TaskFailed, 1, err.Error(), "", "")
	}
	return finish(terminalState(outcome.ExitCode), outcome.ExitCode, outcome.Reason, res.Output, "")
}

func terminalState(exit int) scheduler.TaskState {
	if exit != 0 {
		return scheduler.TaskFailed
	}
	return scheduler.TaskDone
}

func (g *Gateway) resolveExec(ctx context.Context, task *scheduler.Task, res protocol.ExecResult) (completion.Outcome, error) {
	return completion.Default.Resolve(ctx, completion.RunContext{
		RunID:      task.ID,
		WorkerID:   task.AssignedWorkerID,
		LaunchMode: completion.LaunchSupervised,
		Exec:       &completion.ExecResult{ExitCode: res.ExitCode},
	})
}

func (g *Gateway) resolveProcess(ctx context.Context, task *scheduler.Task, res protocol.ProcessResult) (completion.Outcome, error) {
	exit := res.ExitCode
	return completion.Default.Resolve(ctx, completion.RunContext{
		RunID:       task.ID,
		WorkerID:    task.AssignedWorkerID,
		LaunchMode:  completion.LaunchSupervised,
		ProcessID:   res.Pid,
		ProcessExit: &exit,
	})
}

func (g *Gateway) resolveSentinel(ctx context.Context, task *scheduler.Task, nonce string, res protocol.RunSendKeysResult) (completion.Outcome, error) {
	run := completion.RunContext{
		RunID:      task.ID,
		WorkerID:   task.AssignedWorkerID,
		LaunchMode: completion.LaunchSendKeys,
		Nonce:      nonce,
		Output:     res.Output,
	}
	if res.DoneFile != "" {
		// The worker wrote `.done`. Finish through the file resolver so a
		// later reconnect can report the same high-trust outcome.
		run.DoneBody = res.DoneFile
		run.Output = ""
	}
	return completion.Default.Resolve(ctx, run)
}

func rpcTimeout(ctx context.Context) time.Duration {
	timeout := defaultExecTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if remain := time.Until(deadline); remain > 0 && remain < timeout {
			timeout = remain
		}
	}
	return timeout
}

func (g *Gateway) execOn(ctx context.Context, workerID, command string) (protocol.ExecResult, error) {
	ac := g.connFor(workerID)
	if ac == nil {
		return protocol.ExecResult{}, ErrConnectorOffline
	}
	timeout := rpcTimeout(ctx)
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	reply, err := ac.call(callCtx, protocol.TypeExec, protocol.Exec{
		Command:    command,
		TimeoutSec: int(timeout.Seconds()),
	})
	if err != nil {
		return protocol.ExecResult{}, err
	}
	var res protocol.ExecResult
	if err := json.Unmarshal(reply.Data, &res); err != nil {
		return protocol.ExecResult{}, fmt.Errorf("exec result: %w", err)
	}
	return res, nil
}

func (g *Gateway) processOn(ctx context.Context, task *scheduler.Task) (protocol.ProcessResult, error) {
	ac := g.connFor(task.AssignedWorkerID)
	if ac == nil {
		return protocol.ProcessResult{}, ErrConnectorOffline
	}
	timeout := rpcTimeout(ctx)
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	reply, err := ac.call(callCtx, protocol.TypeProcessStart, protocol.ProcessStart{
		Command:    task.Command,
		TimeoutSec: int(timeout.Seconds()),
		RunID:      task.ID,
	})
	if err != nil {
		return protocol.ProcessResult{}, err
	}
	var res protocol.ProcessResult
	if err := json.Unmarshal(reply.Data, &res); err != nil {
		return protocol.ProcessResult{}, fmt.Errorf("process result: %w", err)
	}
	return res, nil
}

func (g *Gateway) sendKeysOn(ctx context.Context, task *scheduler.Task, nonce string) (protocol.RunSendKeysResult, error) {
	ac := g.connFor(task.AssignedWorkerID)
	if ac == nil {
		return protocol.RunSendKeysResult{}, ErrConnectorOffline
	}
	timeout := rpcTimeout(ctx)
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	reply, err := ac.call(callCtx, protocol.TypeRunSendKeys, protocol.RunSendKeys{
		Session:    task.ID,
		Command:    task.Command,
		Nonce:      nonce,
		TimeoutSec: int(timeout.Seconds()),
		RunID:      task.ID,
	})
	if err != nil {
		return protocol.RunSendKeysResult{}, err
	}
	var res protocol.RunSendKeysResult
	if err := json.Unmarshal(reply.Data, &res); err != nil {
		return protocol.RunSendKeysResult{}, fmt.Errorf("send_keys result: %w", err)
	}
	return res, nil
}
