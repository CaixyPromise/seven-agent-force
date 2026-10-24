package agentprotocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDescriptorRejectsTaskEntryAndUnsupportedCapability(t *testing.T) {
	a := Descriptor{ID: "fake", Version: "1", Executable: "/opt/seven/adapter/entrypoint", BinarySHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64), Transport: "stdio", Capabilities: []string{"execution.start", "execution.interrupt"}}
	a.Protocol.Name = "seven.agent-adapter"
	a.Protocol.Min = 1
	a.Protocol.Max = 1
	check := func(a Descriptor) error {
		raw, _ := json.Marshal(map[string]any{"adapter": a})
		_, err := ParseDescriptor(raw)
		return err
	}
	if err := check(a); err != nil {
		t.Fatal(err)
	}
	bad := a
	bad.Executable = "/workspace/adapter.py"
	if check(bad) == nil {
		t.Fatal("Task entrypoint accepted")
	}
	bad = a
	bad.Capabilities = []string{"execution.start", "execution.interrupt", "interaction.respond"}
	if check(bad) == nil {
		t.Fatal("unimplemented capability accepted")
	}
	bad = a
	bad.Protocol.Min = 2
	if check(bad) == nil {
		t.Fatal("unsupported protocol accepted")
	}
}
