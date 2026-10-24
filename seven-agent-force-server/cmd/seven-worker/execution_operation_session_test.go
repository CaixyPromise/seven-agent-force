package main

import (
	"errors"
	"testing"

	attemptruntime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
)

func TestExecutionOperationSessionReadyTreatsLifecycleEdgesAsIdle(t *testing.T) {
	for _, state := range []string{"STARTING", "PAUSING_PREPARE", "FINISHED"} {
		ready, err := executionOperationSessionReady(attemptruntime.InteractionExecutionState{
			State: state, RoundKey: "round:1", SessionGeneration: 1,
		}, nil)
		if err != nil || ready {
			t.Fatalf("state %s: ready=%v err=%v", state, ready, err)
		}
	}
}

func TestExecutionOperationSessionReadyRejectsInvalidIdentity(t *testing.T) {
	ready, err := executionOperationSessionReady(attemptruntime.InteractionExecutionState{State: "RUNNING"}, nil)
	if ready || !errors.Is(err, errInteractionBinding) {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
}

func TestExecutionOperationSessionReadyPropagatesStateFailure(t *testing.T) {
	want := errors.New("state unavailable")
	ready, err := executionOperationSessionReady(attemptruntime.InteractionExecutionState{}, want)
	if ready || !errors.Is(err, want) {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
}
