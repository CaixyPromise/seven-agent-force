package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	routingfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionrouting/facade"
	openapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/application"
	taskfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/infrastructure"
)

func TestRuntimeSelectionValidationPreservesOpenConflictAndWebClassification(t *testing.T) {
	for _, cause := range []error{routingfacade.ErrRuntimeSelectionRequired, routingfacade.ErrRuntimeSelectionUnavailable, routingfacade.ErrRuntimeSelectionIncompatible} {
		err := fmt.Errorf("submission: %w", invalidRuntimeSelection(cause))
		if !IsRequestValidationError(err) || !errors.Is(err, taskfacade.ErrInvalidTaskSubmission) || !errors.Is(err, cause) {
			t.Fatalf("routing cause or Web classification lost: %v", err)
		}
		status, code, _ := openapp.StableError(err)
		wantStatus := 409
		if cause == routingfacade.ErrRuntimeSelectionRequired {
			wantStatus = 400
		}
		if status != wantStatus || code != cause.Error() {
			t.Fatalf("Task-to-Open mapping = %d %s, want %d %s", status, code, wantStatus, cause)
		}
	}
}

func TestOpenProjectTaskRejectsMissingAuthenticatedIdentityAndCorrelation(t *testing.T) {
	valid := taskfacade.OpenProjectTaskCommand{TenantID: 10, ApplicationID: 20, CreatorUserID: 30, ProjectID: 40, ExternalCorrelationID: "issue-1"}
	cases := []struct {
		name string
		edit func(*taskfacade.OpenProjectTaskCommand)
	}{
		{"tenant", func(c *taskfacade.OpenProjectTaskCommand) { c.TenantID = 0 }},
		{"application", func(c *taskfacade.OpenProjectTaskCommand) { c.ApplicationID = 0 }},
		{"creator", func(c *taskfacade.OpenProjectTaskCommand) { c.CreatorUserID = 0 }},
		{"empty correlation", func(c *taskfacade.OpenProjectTaskCommand) { c.ExternalCorrelationID = "  " }},
		{"long correlation", func(c *taskfacade.OpenProjectTaskCommand) { c.ExternalCorrelationID = strings.Repeat("a", 129) }},
		{"invalid utf8", func(c *taskfacade.OpenProjectTaskCommand) { c.ExternalCorrelationID = string([]byte{0xff}) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			command := valid
			tc.edit(&command)
			// Rejection must happen before consulting a repository or compiler.
			_, replayed, err := (*Service)(nil).CreateOpenProjectTask(context.Background(), command)
			if !IsRequestValidationError(err) || !errors.Is(fmt.Errorf("submission: %w", err), taskfacade.ErrInvalidTaskSubmission) || replayed {
				t.Fatalf("replayed=%v error=%v", replayed, err)
			}
		})
	}
}

func TestTaskSubmissionClassificationDoesNotMatchIncidentalErrorText(t *testing.T) {
	for _, err := range []error{errors.New("TASK_APPLICATION_IDENTITY_REQUIRED"), errors.New(taskfacade.ErrInvalidTaskSubmission.Error())} {
		if IsRequestValidationError(err) || errors.Is(err, taskfacade.ErrInvalidTaskSubmission) {
			t.Fatalf("incidental error classified as client input: %v", err)
		}
	}
}

func TestProjectTaskHashPreservesWebAndBindsOpenOwnership(t *testing.T) {
	request := taskfacade.ProjectTaskRequest{Instruction: "fix issue", ModelSelection: taskfacade.ModelSelection{CredentialID: 90, Model: "primary"}, RuntimeReleaseID: 91}
	command := submitTaskCommand{TenantID: 10, CreatorUserID: 30, PrincipalType: "USER", Channel: "WEB"}
	legacy, err := json.Marshal(struct {
		CreatorID int64                         `json:"creatorId,string"`
		ProjectID int64                         `json:"projectId,string"`
		Request   taskfacade.ProjectTaskRequest `json:"request"`
	}{30, 40, request})
	if err != nil {
		t.Fatal(err)
	}
	webHash, err := projectTaskRequestHash(40, request, command)
	if err != nil || webHash != infrastructure.RequestHashText(string(legacy)) {
		t.Fatalf("Web idempotency changed: hash=%s error=%v", webHash, err)
	}
	command.PrincipalType, command.Channel, command.IntegrationApplicationID, command.ExternalCorrelationID = "APPLICATION", "OPEN_REST", 20, "issue-1"
	openHash, err := projectTaskRequestHash(40, request, command)
	if err != nil || openHash == webHash {
		t.Fatalf("Open identity not bound: hash=%s error=%v", openHash, err)
	}
	for _, tc := range []struct {
		name string
		edit func(*submitTaskCommand)
	}{
		{"application", func(c *submitTaskCommand) { c.IntegrationApplicationID++ }},
		{"creator", func(c *submitTaskCommand) { c.CreatorUserID++ }},
		{"correlation", func(c *submitTaskCommand) { c.ExternalCorrelationID = "issue-2" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := command
			tc.edit(&other)
			hash, err := projectTaskRequestHash(40, request, other)
			if err != nil || hash == openHash {
				t.Fatalf("changed identity shares hash: %s %v", hash, err)
			}
		})
	}
	if replayHash, err := projectTaskRequestHash(40, request, command); err != nil || replayHash != openHash {
		t.Fatalf("same request is not stable: %s %v", replayHash, err)
	}
}
