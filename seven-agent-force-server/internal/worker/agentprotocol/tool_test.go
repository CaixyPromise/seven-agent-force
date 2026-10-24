package agentprotocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestNetworkCommandToolRequestRoundTrip(t *testing.T) {
	arguments := json.RawMessage(`{"purpose":"install locked dependencies","command":"pnpm install --frozen-lockfile","cwd":".","targets":[{"host":"registry.npmjs.org","protocol":"https","port":443}]}`)
	digest, err := DigestToolArguments(arguments)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(ToolRequestPayload{ToolName: NetworkCommandToolName, LocalToolCallKey: "call-1", NativeToolCallID: "item_tool_call_01HXYZ", RoundKey: "round-1", Arguments: arguments, ArgumentsDigest: digest})
	frame, _ := json.Marshal(Frame{Protocol: Version, Method: "tool.request", Key: "msg-1", Payload: payload})
	frame = append(frame, '\n')
	if _, err = Decode(frame, "up"); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, err = Decode(frame, "down"); err != ErrUnsupported {
		t.Fatalf("wrong direction = %v", err)
	}
}

func TestGitDeliveryToolRequestNormalizesSummaryAndRejectsAuthorityFields(t *testing.T) {
	arguments := json.RawMessage(`{"summary":"  Pagination fixed.  "}`)
	digest, err := DigestToolArgumentsFor(GitDeliveryToolName, arguments)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(ToolRequestPayload{ToolName: GitDeliveryToolName, LocalToolCallKey: "git-call-1", NativeToolCallID: "codex-git-call-1", RoundKey: "round-1", Arguments: arguments, ArgumentsDigest: digest})
	if err := ValidateToolPayload("tool.request", payload); err != nil {
		t.Fatalf("valid Git delivery request rejected: %v", err)
	}
	for _, body := range []string{
		`{"summary":""}`,
		`{"summary":"   "}`,
		`{"summary":"ready","remote":"https://example.com/repo.git"}`,
		`{"summary":"` + strings.Repeat("界", 683) + `"}`,
	} {
		if _, err := DigestToolArgumentsFor(GitDeliveryToolName, json.RawMessage(body)); err != ErrInvalid {
			t.Fatalf("unsafe Git delivery arguments accepted: %s (%v)", body, err)
		}
	}
}

func TestToolDefinitionsRequireExplicitKnownAllowlist(t *testing.T) {
	definitions, err := ToolDefinitions([]string{GitDeliveryToolName})
	if err != nil || len(definitions) != 1 || definitions[0].Name != GitDeliveryToolName {
		t.Fatalf("Git delivery allowlist = %#v, %v", definitions, err)
	}
	if definitions, err = ToolDefinitions(nil); err != nil || len(definitions) != 0 {
		t.Fatalf("empty allowlist opened tools: %#v, %v", definitions, err)
	}
	for _, names := range [][]string{{"unknown_tool"}, {GitDeliveryToolName, GitDeliveryToolName}} {
		if _, err := ToolDefinitions(names); err == nil {
			t.Fatalf("invalid allowlist accepted: %#v", names)
		}
	}
}

func TestToolCatalogRequiresSortedNamesAndMatchingDigest(t *testing.T) {
	names := []string{GitDeliveryToolName, NetworkCommandToolName}
	digest, err := DigestToolCatalogNames(names)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(ToolCatalogPayload{
		RoundKey:      "round-1",
		ToolNames:     names,
		CatalogDigest: digest,
	})
	if err := ValidateToolPayload("tool.catalog", payload); err != nil {
		t.Fatalf("valid catalog rejected: %v", err)
	}

	for _, invalid := range []ToolCatalogPayload{
		{RoundKey: "round-1", ToolNames: []string{NetworkCommandToolName, GitDeliveryToolName}, CatalogDigest: digest},
		{RoundKey: "round-1", ToolNames: names, CatalogDigest: strings.Repeat("0", 64)},
		{RoundKey: "round-1", ToolNames: []string{"unknown_tool"}, CatalogDigest: strings.Repeat("0", 64)},
	} {
		body, _ := json.Marshal(invalid)
		if err := ValidateToolPayload("tool.catalog", body); err != ErrInvalid {
			t.Fatalf("invalid catalog accepted: %s (%v)", body, err)
		}
	}
}

func TestNetworkCommandToolRequestRejectsUnsafeTargetsAndDigestMismatch(t *testing.T) {
	for _, body := range []string{
		`{"purpose":"x","command":"curl x","cwd":"..","targets":[{"host":"registry.npmjs.org","protocol":"https","port":443}]}`,
		`{"purpose":"x","command":"curl x","cwd":".","targets":[{"host":"127.0.0.1","protocol":"https","port":443}]}`,
		`{"purpose":"x","command":"curl x","cwd":".","targets":[{"host":"*.example.com","protocol":"https","port":443}]}`,
	} {
		if _, err := DigestToolArguments(json.RawMessage(body)); err != ErrInvalid {
			t.Fatalf("unsafe input accepted: %s (%v)", body, err)
		}
	}
	arguments := json.RawMessage(`{"purpose":"x","command":"curl x","cwd":".","targets":[{"host":"example.com","protocol":"https","port":443}]}`)
	payload, _ := json.Marshal(ToolRequestPayload{ToolName: NetworkCommandToolName, LocalToolCallKey: "call", NativeToolCallID: "item_tool_call_01HXYZ", RoundKey: "round", Arguments: arguments, ArgumentsDigest: hex.EncodeToString(sha256.New().Sum(nil))})
	frame, _ := json.Marshal(Frame{Protocol: Version, Method: "tool.request", Key: "msg", Payload: payload})
	if _, err := Decode(append(frame, '\n'), "up"); err != ErrInvalid {
		t.Fatalf("digest mismatch = %v", err)
	}
}

func TestToolResultRequiresSafeShape(t *testing.T) {
	zero := 0
	digest := sha256.Sum256([]byte("result"))
	payload, _ := json.Marshal(ToolResultPayload{RoundKey: "round-1", LocalToolCallKey: "call-1", NativeToolCallID: "item_tool_call_01HXYZ", OperationID: 42, Status: "SUCCEEDED", ExitCode: &zero, Summary: "done", Output: "ok", ResultDigest: hex.EncodeToString(digest[:])})
	frame, _ := json.Marshal(Frame{Protocol: Version, Method: "tool.result", Key: "result-1", ReplyTo: "request-1", Payload: payload})
	if _, err := Decode(append(frame, '\n'), "down"); err != nil {
		t.Fatal(err)
	}
	var exact map[string]json.RawMessage
	if json.Unmarshal(payload, &exact) != nil || len(exact) != 10 || exact["reasonCode"] == nil || exact["exitCode"] == nil {
		t.Fatalf("tool result wire shape is not cross-runtime stable: %s", payload)
	}
}

func TestGitDeliveryScheduledResultHasNoExecutionClaim(t *testing.T) {
	payload, _ := json.Marshal(ToolResultPayload{
		RoundKey: "round-1", LocalToolCallKey: "git-call-1",
		NativeToolCallID: "codex-git-call-1", OperationID: 42,
		Status: "SCHEDULED", Summary: "Git delivery intent saved.",
		ResultDigest: strings.Repeat("a", 64),
	})
	if err := ValidateToolPayload("tool.result", payload); err != nil {
		t.Fatalf("scheduled result rejected: %v", err)
	}
	var invalid ToolResultPayload
	_ = json.Unmarshal(payload, &invalid)
	exitCode := 0
	invalid.ExitCode = &exitCode
	broken, _ := json.Marshal(invalid)
	if err := ValidateToolPayload("tool.result", broken); err != ErrInvalid {
		t.Fatalf("scheduled result claimed command execution: %v", err)
	}
}

func TestToolPayloadRequiresStrictNativeToolCallIdentity(t *testing.T) {
	arguments := json.RawMessage(`{"purpose":"x","command":"curl x","cwd":".","targets":[{"host":"example.com","protocol":"https","port":443}]}`)
	digest, err := DigestToolArguments(arguments)
	if err != nil {
		t.Fatal(err)
	}
	for _, nativeID := range []string{"", " leading-space", "tool/call", string(make([]byte, MaxNativeToolCallIDBytes+1))} {
		payload, _ := json.Marshal(ToolRequestPayload{ToolName: NetworkCommandToolName, LocalToolCallKey: "call-1", NativeToolCallID: nativeID, RoundKey: "round-1", Arguments: arguments, ArgumentsDigest: digest})
		if err := ValidateToolPayload("tool.request", payload); err != ErrInvalid {
			t.Fatalf("nativeToolCallId %q accepted: %v", nativeID, err)
		}
	}
}
