package main

import (
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
)

func TestPendingToolResultRequiresOriginalNativeIdentity(t *testing.T) {
	pending := &pendingToolCall{request: agentprotocol.ToolRequestPayload{
		RoundKey:         "round-1",
		LocalToolCallKey: "call-1",
		NativeToolCallID: "item_tool_call_original",
	}}
	result := agentprotocol.ToolResultPayload{
		RoundKey:         "round-1",
		LocalToolCallKey: "call-1",
		NativeToolCallID: "item_tool_call_replaced",
	}
	if pendingToolAcceptsResult(pending, result) {
		t.Fatal("result replaced the native tool call identity")
	}
	result.NativeToolCallID = pending.request.NativeToolCallID
	if !pendingToolAcceptsResult(pending, result) {
		t.Fatal("result with the original composite identity was rejected")
	}
	pending.operationID = 91
	result.OperationID = 92
	if pendingToolAcceptsResult(pending, result) {
		t.Fatal("result replaced the bound Operation identity")
	}
}
