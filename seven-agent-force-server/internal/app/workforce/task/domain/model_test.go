package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTaskCreatorIdentityUsesLosslessUserID(t *testing.T) {
	body, err := json.Marshal(Task{ID: 2610240832, CreatedBy: 2065424359060983808})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"createdBy":"2065424359060983808"`) {
		t.Fatalf("creator id must be encoded losslessly, got %s", body)
	}
}

func TestAttemptUnmarshalFrameworkSafeIntegers(t *testing.T) {
	var attempt Attempt
	if err := json.Unmarshal([]byte(`{"id":"1001","generation":"3","ownerEpoch":"2"}`), &attempt); err != nil {
		t.Fatal(err)
	}
	if attempt.Generation != 3 || attempt.OwnerEpoch != 2 {
		t.Fatalf("unexpected safe integers: %#v", attempt)
	}
}

func TestEventUnmarshalFrameworkSafeIdentities(t *testing.T) {
	var event Event
	body := []byte(`{"id":"9007199254740993","taskId":"9007199254740994","attemptId":"9007199254740995","sequence":"9007199254740996","type":"task.validating","payload":{},"createdAt":"2026-08-30T00:00:00.000Z"}`)
	if err := json.Unmarshal(body, &event); err != nil {
		t.Fatal(err)
	}
	if event.ID != 9007199254740993 || event.TaskID != 9007199254740994 || event.AttemptID != 9007199254740995 || event.Sequence != 9007199254740996 {
		t.Fatalf("event identities were not decoded losslessly: %+v", event)
	}
}

func TestAttemptUnmarshalAllowsEmptyTerminalDirectiveValue(t *testing.T) {
	var attempt Attempt
	if err := json.Unmarshal([]byte(`{"id":"0","generation":"0","ownerEpoch":"0"}`), &attempt); err != nil {
		t.Fatalf("unmarshal empty terminal Attempt: %v", err)
	}
	if attempt.ID != 0 || attempt.Generation != 0 || attempt.OwnerEpoch != 0 {
		t.Fatalf("unexpected empty Attempt: %+v", attempt)
	}
}

func TestAttemptUnmarshalRejectsZeroGenerationForExecutableAttempt(t *testing.T) {
	var attempt Attempt
	if err := json.Unmarshal([]byte(`{"id":"1001","generation":"0","ownerEpoch":"1"}`), &attempt); err == nil {
		t.Fatal("expected executable Attempt with zero generation to be rejected")
	}
}

func TestContinuationRestoreUnmarshalFrameworkSafeIntegers(t *testing.T) {
	var restore ContinuationRestore
	if err := json.Unmarshal([]byte(`{"checkpointId":"1002","workspaceSizeBytes":"536870912","sessionSizeBytes":"67108864","manifestSizeBytes":"4096"}`), &restore); err != nil {
		t.Fatal(err)
	}
	if restore.WorkspaceSizeBytes != 536870912 || restore.SessionSizeBytes != 67108864 || restore.ManifestSizeBytes != 4096 {
		t.Fatalf("unexpected continuation sizes: %#v", restore)
	}
}

func TestContinuationRestoreUnmarshalRejectsInvalidSize(t *testing.T) {
	var restore ContinuationRestore
	if err := json.Unmarshal([]byte(`{"workspaceSizeBytes":"not-a-number","sessionSizeBytes":1,"manifestSizeBytes":1}`), &restore); err == nil {
		t.Fatal("expected invalid continuation size to be rejected")
	}
}

func TestTaskInputContractUnmarshalFrameworkSafeIntegers(t *testing.T) {
	var input TaskInputContract
	body := []byte(`{"workspaceSourceRevisionId":"1003","repositoryBundleSizeBytes":"4096","references":[{"id":"1004","displayName":"requirements.pdf","sizeBytes":"8192"}]}`)
	if err := json.Unmarshal(body, &input); err != nil {
		t.Fatal(err)
	}
	if input.RepositoryBundleSizeBytes != 4096 || len(input.References) != 1 || input.References[0].SizeBytes != 8192 {
		t.Fatalf("unexpected task input sizes: %#v", input)
	}
}

func TestTaskInputContractUnmarshalRejectsInvalidReferenceSize(t *testing.T) {
	var input TaskInputContract
	if err := json.Unmarshal([]byte(`{"repositoryBundleSizeBytes":"1","references":[{"id":"1004","sizeBytes":"invalid"}]}`), &input); err == nil {
		t.Fatal("expected invalid reference size to be rejected")
	}
}
