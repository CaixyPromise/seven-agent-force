package main

import (
	"context"
	"errors"
	runtime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
	task "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	"testing"
)

type resolvedSuspendChannel struct {
	aborted bool
	failure error
	t       *testing.T
}

func (c *resolvedSuspendChannel) State(context.Context) (runtime.InteractionExecutionState, error) {
	return runtime.InteractionExecutionState{State: "PAUSING_PREPARE", RoundKey: "round:1", SessionGeneration: 1, SuspendControlKey: "suspend:1:1"}, nil
}
func (c *resolvedSuspendChannel) DecideSuspension(_ context.Context, _ runtime.InteractionExecutionState, commit bool) error {
	if commit {
		c.t.Fatal("already receipted answer stopped native")
	}
	c.aborted = true
	return c.failure
}
func (c *resolvedSuspendChannel) AwaitSuspension(context.Context, runtime.InteractionExecutionState) (runtime.InteractionCapture, error) {
	c.t.Fatal("capture after abort")
	return runtime.InteractionCapture{}, nil
}
func (c *resolvedSuspendChannel) WithStoppedQuiescentWorkspace(context.Context, int64, func() error) error {
	c.t.Fatal("freeze after abort")
	return nil
}

func TestAlreadyReceiptedAnswerAbortsWithoutDrainingPump(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "aborted", true: "abort-error"}[failed], func(t *testing.T) {
			channel := &resolvedSuspendChannel{t: t}
			if failed {
				channel.failure = errors.New("abort unavailable")
			}
			// A zero hub would fail Drain; nil control would panic on a request.
			w := &worker{interactions: &interactionTransportHub{}}
			err := w.suspendInteractionRuntime(context.Background(), task.WorkItem{}, task.Attempt{}, runtime.Workspace{}, channel)
			want := runtime.ErrInteractionSuspendAborted
			if failed {
				want = channel.failure
			}
			if !errors.Is(err, want) || !channel.aborted {
				t.Fatalf("err=%v aborted=%v", err, channel.aborted)
			}
		})
	}
}
