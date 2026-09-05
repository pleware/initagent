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
		AssignedWorkerID: t.AssignedWorkerID,
		ExitCode:         t.ExitCode,
		Reason:           t.Reason,
		Stdout:           stdout,
		Stderr:           stderr,
	}
}

// RunQueued claims the oldest queued task for workerID, runs its command
// over TypeExec on the live agent socket, and Finish-es the row. Completion
// flows through the completion registry: the exec reply becomes a resolver
// Outcome whose exit code and reason drive the terminal state.
func (g *Gateway) RunQueued(ctx context.Context, projectID, workerID string) (TaskView, error) {
	if g.connForProject(projectID, workerID) == nil {
		return TaskView{}, ErrDeviceOffline
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

	if err := g.store.UpdateState(ctx, claimed.ID, scheduler.TaskRunning, "exec"); err != nil {
		return TaskView{}, err
	}

	res, err := g.execOn(ctx, claimed.AssignedWorkerID, claimed.Command)
	if err != nil {
		return finish(scheduler.TaskFailed, 1, err.Error(), "", "")
	}

	outcome, err := g.resolveExec(ctx, claimed, res)
	if err != nil {
		return finish(scheduler.TaskFailed, 1, err.Error(), "", "")
	}

	to := scheduler.TaskDone
	if outcome.ExitCode != 0 {
		to = scheduler.TaskFailed
	}
	return finish(to, outcome.ExitCode, outcome.Reason, res.Stdout, res.Stderr)
}

// resolveExec turns the TypeExec reply into a completion Outcome through the
// registry. The run is supervised (the agent owns the child process), so the
// exec resolver is the one that fires; the registry drops resolvers that
// cannot work here (process has no pid, file has no sentinel dir).
func (g *Gateway) resolveExec(ctx context.Context, task *scheduler.Task, res protocol.ExecResult) (completion.Outcome, error) {
	return completion.Default.Resolve(ctx, completion.RunContext{
		RunID:      task.ID,
		WorkerID:   task.AssignedWorkerID,
		LaunchMode: completion.LaunchSupervised,
		Exec:       &completion.ExecResult{ExitCode: res.ExitCode},
	})
}

func (g *Gateway) execOn(ctx context.Context, workerID, command string) (protocol.ExecResult, error) {
	ac := g.connFor(workerID)
	if ac == nil {
		return protocol.ExecResult{}, ErrDeviceOffline
	}
	timeout := defaultExecTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if remain := time.Until(deadline); remain > 0 && remain < timeout {
			timeout = remain
		}
	}
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
