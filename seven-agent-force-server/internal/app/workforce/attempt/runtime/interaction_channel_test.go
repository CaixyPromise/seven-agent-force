package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestExecutionChannelAllowsToolResultControl(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	channel := &ExecutionChannel{session: newExecutionSession(ctx, 1, 2, 1, 1, nil), container: "attempt"}
	err := channel.Send(context.Background(), "round-1", 1, "tool.result", json.RawMessage(`{"roundKey":"round-1"}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("tool.result was rejected before reaching the execution session: %v", err)
	}
}

func TestExecutionChannelRejectsUnknownControl(t *testing.T) {
	channel := &ExecutionChannel{session: &ExecutionSession{}, container: "attempt"}
	err := channel.Send(context.Background(), "round-1", 1, "platform.approve", json.RawMessage(`{}`))
	if err == nil || err.Error() != "INTERACTION_CONTROL_UNSUPPORTED" {
		t.Fatalf("unexpected error: %v", err)
	}
}
