package domain

import (
	"testing"

	taskfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
)

func TestOpenRequestCompilesOnlyProjectTaskAdditions(t *testing.T) {
	request, err := (CreateOpenTaskRequest{
		ProjectID: 701, Name: " Example ", Instruction: " Do it ", Tags: []string{" bugfix "},
		ReferenceAssetIDs: []int64{801}, ReviewEnabled: boolPointer(false), ExternalCorrelationID: " issue-1 ",
		ModelSelection: taskModelSelection(), RuntimeReleaseID: 901,
	}).NormalizeAndValidate()
	if err != nil {
		t.Fatal(err)
	}
	compiled := request.ProjectTaskRequest()
	if request.ProjectID != 701 || compiled.Name != "Example" || compiled.Instruction != "Do it" || compiled.DeliveryPreference != "PROJECT_DEFAULT" {
		t.Fatalf("unexpected project task request: project=%d request=%+v", request.ProjectID, compiled)
	}
	if len(compiled.Tags) != 1 || compiled.Tags[0] != "bugfix" || len(compiled.ReferenceAssetIDs) != 1 || compiled.ReferenceAssetIDs[0] != 801 || compiled.ReviewEnabled == nil || *compiled.ReviewEnabled {
		t.Fatalf("task additions were not preserved: %+v", compiled)
	}
}

func boolPointer(value bool) *bool { return &value }

func TestOpenRequestValidationAndApplicationScopedIdempotency(t *testing.T) {
	base := CreateOpenTaskRequest{ProjectID: 701, Name: "name", Instruction: "instruction", ExternalCorrelationID: "external-1", ModelSelection: taskModelSelection(), RuntimeReleaseID: 901}
	_, err := base.NormalizeAndValidate()
	if err != nil {
		t.Fatal(err)
	}
	bad := base
	bad.ProjectID = 0
	if _, err := bad.NormalizeAndValidate(); err == nil {
		t.Fatal("missing Project identity was accepted")
	}
	bad = base
	bad.DeliveryPreference = "AUTO_AFTER_GATES"
	if _, err := bad.NormalizeAndValidate(); err == nil {
		t.Fatal("Task was allowed to strengthen Project delivery authorization")
	}
	if InternalIdempotencyKey(1001, "same") == InternalIdempotencyKey(1002, "same") {
		t.Fatal("idempotency key is not application scoped")
	}
}

func taskModelSelection() taskfacade.ModelSelection {
	return taskfacade.ModelSelection{CredentialID: 801, Model: "gpt-seven"}
}
