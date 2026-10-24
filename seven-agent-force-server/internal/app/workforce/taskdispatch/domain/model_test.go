package domain

import (
	"encoding/json"
	"testing"

	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
)

func TestExecutionContractCarriesFrozenAdapterTools(t *testing.T) {
	original := ExecutionContract{WorkItem: taskdomain.WorkItem{
		TaskID:              1,
		AllowedAdapterTools: []string{"request_git_delivery", "run_command_with_network_access"},
	}}
	body, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(body) || !containsJSONKey(body, "allowedAdapterTools") {
		t.Fatalf("machine contract dropped frozen tools: %s", body)
	}
	var decoded ExecutionContract
	if err = json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.WorkItem.AllowedAdapterTools) != 2 || decoded.WorkItem.AllowedAdapterTools[0] != "request_git_delivery" {
		t.Fatalf("frozen tools changed: %#v", decoded.WorkItem.AllowedAdapterTools)
	}
}

func containsJSONKey(body []byte, key string) bool {
	var value map[string]any
	if json.Unmarshal(body, &value) != nil {
		return false
	}
	workItem, _ := value["workItem"].(map[string]any)
	_, ok := workItem[key]
	return ok
}

func TestExecutionContractUnmarshalFrameworkSafeIntegers(t *testing.T) {
	var contract ExecutionContract
	body := []byte(`{"directive":"START_NEW_RUNTIME","ownerEpoch":"2","attempt":{"id":"1001","generation":"3","ownerEpoch":"2"}}`)
	if err := json.Unmarshal(body, &contract); err != nil {
		t.Fatal(err)
	}
	if contract.OwnerEpoch != 2 || contract.Attempt.Generation != 3 || contract.Attempt.OwnerEpoch != 2 {
		t.Fatalf("unexpected safe integers: %#v", contract)
	}
}

func TestExecutionContractUnmarshalAllowsTerminalDirectiveWithoutRuntimeOwner(t *testing.T) {
	var contract ExecutionContract
	body := []byte(`{"directive":"ACK_TERMINAL","assignmentId":"1002","executionNodeId":"1003","executionRequirementDigest":"abc","attempt":{"id":"0","generation":"0","ownerEpoch":"0"},"workItem":{},"attemptLeaseToken":"","attemptLeaseExpiresAt":"","ownerEpoch":"0"}`)
	if err := json.Unmarshal(body, &contract); err != nil {
		t.Fatalf("unmarshal terminal contract: %v", err)
	}
	if contract.Directive != DirectiveAckTerminal || contract.OwnerEpoch != 0 {
		t.Fatalf("unexpected terminal contract: %+v", contract)
	}
}

func TestExecutionContractUnmarshalRejectsRuntimeDirectiveWithoutOwner(t *testing.T) {
	var contract ExecutionContract
	body := []byte(`{"directive":"START_NEW_RUNTIME","attempt":{"id":"0","generation":"0","ownerEpoch":"0"},"workItem":{},"ownerEpoch":"0"}`)
	if err := json.Unmarshal(body, &contract); err == nil {
		t.Fatal("expected runtime directive without owner epoch to be rejected")
	}
}

func TestRunPermitUnmarshalFrameworkSafeInteger(t *testing.T) {
	var permit RunPermit
	if err := json.Unmarshal([]byte(`{"runPermitId":"1004","ownerEpoch":"4"}`), &permit); err != nil {
		t.Fatal(err)
	}
	if permit.OwnerEpoch != 4 {
		t.Fatalf("unexpected owner epoch: %d", permit.OwnerEpoch)
	}
}
