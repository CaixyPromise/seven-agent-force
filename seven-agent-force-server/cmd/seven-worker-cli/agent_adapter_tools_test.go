package main

import (
	"reflect"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
)

func TestFrozenAdapterToolDefinitionsRequireCallerAllowlist(t *testing.T) {
	definitions, err := frozenAdapterToolDefinitions(attemptSpec{})
	if err != nil || len(definitions) != 0 {
		t.Fatalf("empty caller allowlist opened tools: %#v, %v", definitions, err)
	}
	definitions, err = frozenAdapterToolDefinitions(attemptSpec{
		AllowedAdapterTools: []string{agentprotocol.GitDeliveryToolName},
	})
	if err != nil || len(definitions) != 1 || !reflect.DeepEqual(definitions[0], agentprotocol.GitDeliveryToolDefinition()) {
		t.Fatalf("Git delivery allowlist = %#v, %v", definitions, err)
	}
}
