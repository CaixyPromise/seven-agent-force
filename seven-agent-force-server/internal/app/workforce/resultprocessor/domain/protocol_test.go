package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateRequiresProtectedPythonEntrypoint(t *testing.T) {
	manifest := DefaultManifest("demo")
	valid := Validate(manifest, []SourceFile{{Path: EntrypointFile, Content: "def process(context):\n    return {'decision': 'PASS', 'reasonCode': 'OK', 'message': 'ok'}\n"}})
	if !valid.Valid {
		t.Fatalf("valid processor rejected: %#v", valid)
	}
	invalid := Validate(manifest, []SourceFile{{Path: EntrypointFile, Content: "def process(value):\n    return {}\n"}})
	if invalid.Valid || !strings.Contains(strings.Join(invalid.Errors, ";"), "process(context)") {
		t.Fatalf("invalid signature accepted: %#v", invalid)
	}
}

func TestValidateRejectsDeliveryDriftAndReservedPath(t *testing.T) {
	manifest := DefaultManifest("demo")
	manifest.Capabilities = []string{"PROCESSOR_DELIVER"}
	result := Validate(manifest, []SourceFile{
		{Path: EntrypointFile, Content: "def process(context):\n    return {}\n"},
		{Path: "seven_agent/force/shadow.py", Content: "x = 1\n"},
	})
	if result.Valid || !strings.Contains(strings.Join(result.Errors, ";"), "PROCESSOR_DELIVER") || !strings.Contains(strings.Join(result.Errors, ";"), "platform-reserved") {
		t.Fatalf("delivery/reserved drift was accepted: %#v", result)
	}
}

func TestValidateRejectsRuntimeSDKModuleShadow(t *testing.T) {
	for _, sourcePath := range []string{"seven_agent.py", "seven_agent/__init__.py", "seven_agent/force/shadow.py", "seven_agent.force.py", "seven_workforce_runtime.py", "seven_workforce_runtime/shadow.py"} {
		t.Run(sourcePath, func(t *testing.T) {
			result := Validate(DefaultManifest("demo"), []SourceFile{
				{Path: EntrypointFile, Content: "def process(context):\n    return {}\n"},
				{Path: sourcePath, Content: "x = 1\n"},
			})
			if result.Valid || !strings.Contains(strings.Join(result.Errors, ";"), "platform-reserved") {
				t.Fatalf("SDK module shadow accepted: %#v", result)
			}
		})
	}
}

func TestResultContractDecisionsAndOutputBoundary(t *testing.T) {
	contract := ResultContract{Decision: "REVISE", ReasonCode: "CHECK_FAILED", Message: "fix it", Findings: []Finding{{Code: "MISSING", Message: "missing file"}}}
	body, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateContract(body); err != nil {
		t.Fatalf("valid contract rejected: %v", err)
	}
	contract.Outputs = []Output{{Name: "later", Kind: "PATH", Path: "safe.txt", Storage: "ON_DEMAND"}}
	body, _ = json.Marshal(contract)
	if _, err := ValidateContract(body); err != nil {
		t.Fatalf("on-demand output was rejected after Continuation Closure: %v", err)
	}
	contract.Outputs[0].Storage = "LATER"
	body, _ = json.Marshal(contract)
	if _, err := ValidateContract(body); err == nil {
		t.Fatal("unknown output storage was accepted")
	}
}

func TestResultContractActionIsDeclarativeAndBounded(t *testing.T) {
	t.Parallel()
	valid := []byte(`{"decision":"PASS","reasonCode":"OK","message":"done","actions":[{"type":"GIT_PUSH","when":"AFTER_APPROVAL","targetId":"1001","baseBranch":"main","branch":"seven/task"}]}`)
	if _, err := ValidateContract(valid); err != nil {
		t.Fatalf("valid declarative Git action rejected: %v", err)
	}
	for name, body := range map[string][]byte{
		"unknown type":              []byte(`{"decision":"PASS","reasonCode":"OK","message":"done","actions":[{"type":"SHELL"}]}`),
		"invalid timing":            []byte(`{"decision":"PASS","reasonCode":"OK","message":"done","actions":[{"type":"GIT_PUSH","when":"NOW"}]}`),
		"missing target":            []byte(`{"decision":"PASS","reasonCode":"OK","message":"done","actions":[{"type":"GIT_PUSH","baseBranch":"main","branch":"seven/task"}]}`),
		"raw url":                   []byte(`{"decision":"PASS","reasonCode":"OK","message":"done","actions":[{"type":"GIT_PUSH","targetId":"1001","baseBranch":"main","branch":"seven/task","payload":{"url":"https://example.invalid"}}]}`),
		"multiple delivery actions": []byte(`{"decision":"PASS","reasonCode":"OK","message":"done","actions":[{"type":"GIT_PUSH","targetId":"1001","baseBranch":"main","branch":"seven/task"},{"type":"GIT_PUSH","targetId":"1001","baseBranch":"main","branch":"seven/task-2"}]}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ValidateContract(body); err == nil {
				t.Fatal("invalid action was accepted")
			}
		})
	}
}

func TestResultContractArchiveAndContinuationSelection(t *testing.T) {
	t.Parallel()
	valid := []byte(`{"decision":"PASS","reasonCode":"OK","message":"done","outputs":[{"name":"release","kind":"ARCHIVE","selection":{"includes":["dist/**","README.md"],"excludes":["dist/**/*.map"]},"archiveFormat":"ZIP","storage":"ON_DEMAND"}],"continuation":{"includes":["generated/schema.json"]}}`)
	contract, err := ValidateContract(valid)
	if err != nil || contract.Outputs[0].ArchiveFormat != "ZIP" {
		t.Fatalf("valid archive contract rejected: contract=%+v err=%v", contract, err)
	}
	for name, body := range map[string][]byte{
		"missing format": []byte(`{"decision":"PASS","reasonCode":"OK","message":"done","outputs":[{"name":"release","kind":"ARCHIVE","selection":{"includes":["dist/**"]},"storage":"ON_DEMAND"}]}`),
		"traversal":      []byte(`{"decision":"PASS","reasonCode":"OK","message":"done","outputs":[{"name":"release","kind":"ARCHIVE","selection":{"includes":["../secret"]},"archiveFormat":"ZIP","storage":"ON_DEMAND"}]}`),
		"git protected":  []byte(`{"decision":"PASS","reasonCode":"OK","message":"done","continuation":{"includes":[".git/config"]}}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ValidateContract(body); err == nil {
				t.Fatal("invalid selection accepted")
			}
		})
	}
}

func TestCanonicalArchiveRoundTrip(t *testing.T) {
	manifest := DefaultManifest("demo")
	files := []SourceFile{{Path: EntrypointFile, Content: "def process(context):\n    return {'decision':'PASS','reasonCode':'OK','message':'ok'}\n"}}
	archive, digest, err := CanonicalArchive(manifest, files)
	if err != nil || digest == "" {
		t.Fatalf("canonical archive: digest=%q err=%v", digest, err)
	}
	decoded, decodedFiles, err := ImportArchive(archive)
	if err != nil || decoded.Name != manifest.Name || len(decodedFiles) != 1 {
		t.Fatalf("round trip: manifest=%+v files=%d err=%v", decoded, len(decodedFiles), err)
	}
}

func TestParametersSchemaRejectsUnknownAndValidatesFrozenTaskInput(t *testing.T) {
	schema := json.RawMessage(`{
		"type":"object",
		"properties":{
			"branch":{"type":"string","minLength":1,"maxLength":32},
			"retries":{"type":"integer","minimum":0,"maximum":5},
			"mode":{"enum":["quick","full"]}
		},
		"required":["branch"],
		"additionalProperties":false
	}`)
	if err := ValidateParametersSchema(schema); err != nil {
		t.Fatalf("valid restricted schema rejected: %v", err)
	}
	if err := ValidateParameters(schema, json.RawMessage(`{"branch":"main","retries":2,"mode":"quick"}`)); err != nil {
		t.Fatalf("valid frozen input rejected: %v", err)
	}
	for name, body := range map[string]json.RawMessage{
		"missing":    json.RawMessage(`{"retries":2}`),
		"unknown":    json.RawMessage(`{"branch":"main","extra":true}`),
		"wrong-type": json.RawMessage(`{"branch":"main","retries":2.5}`),
		"enum":       json.RawMessage(`{"branch":"main","mode":"unsafe"}`),
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateParameters(schema, body); err == nil {
				t.Fatalf("invalid input %s was accepted", name)
			}
		})
	}
	if err := ValidateParametersSchema(json.RawMessage(`{"type":"object","oneOf":[]}`)); err == nil {
		t.Fatal("unsupported JSON Schema keyword was accepted")
	}
}
