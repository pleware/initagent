package completion

import (
	"context"
	"testing"
)

func TestExecResolver_Name(t *testing.T) {
	e := &ExecResolver{}
	if got := e.Name(); got != "exec" {
		t.Fatalf("Name() = %q, want %q", got, "exec")
	}
}

func TestExecResolver_Supports(t *testing.T) {
	e := &ExecResolver{}
	if !e.Supports(LaunchSupervised) {
		t.Error("expected support for LaunchSupervised")
	}
	if e.Supports(LaunchSendKeys) {
		t.Error("did not expect support for LaunchSendKeys")
	}
}

func TestExecResolver_Watch(t *testing.T) {
	e := &ExecResolver{}
	ch, err := e.Watch(t.Context(), RunContext{Exec: &ExecResult{ExitCode: 7}})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	outcome := <-ch
	if !outcome.Done {
		t.Fatal("expected Done=true")
	}
	if outcome.ExitCode != 7 {
		t.Fatalf("ExitCode = %d, want 7", outcome.ExitCode)
	}
	if outcome.Reason != "exec" {
		t.Fatalf("Reason = %q, want exec", outcome.Reason)
	}
	if outcome.Trust != TrustHigh {
		t.Fatalf("Trust = %q, want %q", outcome.Trust, TrustHigh)
	}
	if _, ok := <-ch; ok {
		t.Fatal("channel should be closed after one outcome")
	}
}

func TestExecResolver_WatchMissingExec(t *testing.T) {
	e := &ExecResolver{}
	if _, err := e.Watch(t.Context(), RunContext{RunID: "run-1"}); err == nil {
		t.Fatal("expected error for missing exec result")
	}
}

func TestDefaultRegistry_HasExec(t *testing.T) {
	if Default.Get("exec") == nil {
		t.Fatal("expected exec resolver in default registry")
	}
}

func TestResolve_SupervisedExec(t *testing.T) {
	outcome, err := Default.Resolve(t.Context(), RunContext{
		RunID:      "run-1",
		LaunchMode: LaunchSupervised,
		Exec:       &ExecResult{ExitCode: 0},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if outcome.Reason != "exec" || outcome.ExitCode != 0 || outcome.Trust != TrustHigh {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestResolve_NoResolverFires(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockResolver{name: "none"})
	if _, err := r.Resolve(t.Context(), RunContext{RunID: "run-1", LaunchMode: LaunchSupervised}); err == nil {
		t.Fatal("expected error when no resolver reports completion")
	}
}

// errorResolver supports every mode but cannot Watch, so Resolve drops it.
type errorResolver struct{ name string }

func (e *errorResolver) Name() string             { return e.name }
func (e *errorResolver) Supports(LaunchMode) bool { return true }
func (e *errorResolver) Watch(context.Context, RunContext) (<-chan Outcome, error) {
	return nil, context.DeadlineExceeded
}

// outcomeResolver immediately emits a fixed outcome.
type outcomeResolver struct{ o Outcome }

func (o *outcomeResolver) Name() string             { return "out" }
func (o *outcomeResolver) Supports(LaunchMode) bool { return true }
func (o *outcomeResolver) Watch(context.Context, RunContext) (<-chan Outcome, error) {
	ch := make(chan Outcome, 1)
	ch <- o.o
	close(ch)
	return ch, nil
}

func TestResolve_DropsErroredResolver(t *testing.T) {
	r := NewRegistry()
	r.Register(&errorResolver{name: "broken"})
	want := Outcome{Done: true, ExitCode: 3, Reason: "out", Trust: TrustHigh}
	r.Register(&outcomeResolver{o: want})

	got, err := r.Resolve(t.Context(), RunContext{RunID: "run-1", LaunchMode: LaunchSupervised})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.ExitCode != 3 || got.Reason != "out" {
		t.Fatalf("got = %+v", got)
	}
}

// modeResolver emits an outcome but only under send_keys.
type modeResolver struct{ o Outcome }

func (m *modeResolver) Name() string                  { return "mode" }
func (m *modeResolver) Supports(mode LaunchMode) bool { return mode == LaunchSendKeys }
func (m *modeResolver) Watch(context.Context, RunContext) (<-chan Outcome, error) {
	ch := make(chan Outcome, 1)
	ch <- m.o
	close(ch)
	return ch, nil
}

func TestResolve_SkipsWrongMode(t *testing.T) {
	r := NewRegistry()
	r.Register(&modeResolver{o: Outcome{Done: true, ExitCode: 9, Reason: "mode", Trust: TrustHigh}})
	if _, err := r.Resolve(t.Context(), RunContext{RunID: "run-1", LaunchMode: LaunchSupervised}); err == nil {
		t.Fatal("expected error when only a send_keys resolver is registered")
	}
}

// blockingResolver returns a channel that never emits, so Resolve can only
// observe the run's context cancellation.
type blockingResolver struct{}

func (b *blockingResolver) Name() string             { return "block" }
func (b *blockingResolver) Supports(LaunchMode) bool { return true }
func (b *blockingResolver) Watch(context.Context, RunContext) (<-chan Outcome, error) {
	return make(chan Outcome), nil
}

func TestResolve_CancelledContext(t *testing.T) {
	r := NewRegistry()
	r.Register(&blockingResolver{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Resolve(ctx, RunContext{RunID: "run-1", LaunchMode: LaunchSupervised}); err == nil {
		t.Fatal("expected error from a cancelled context")
	}
}
