package application

import (
	"context"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/domain"
	openinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/infrastructure"
	projectdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/domain"
	resultfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	taskfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
)

type recordingSubmission struct {
	tenantID, creatorUserID, projectID int64
	applicationID                      int64
	correlation                        string
	idempotencyKey                     string
	request                            taskfacade.ProjectTaskRequest
}

func (s *recordingSubmission) CreateOpenProjectTask(_ context.Context, command taskfacade.OpenProjectTaskCommand) (taskdomain.Task, bool, error) {
	s.tenantID, s.creatorUserID, s.projectID, s.idempotencyKey, s.request = command.TenantID, command.CreatorUserID, command.ProjectID, command.IdempotencyKey, command.Request
	s.applicationID, s.correlation = command.ApplicationID, command.ExternalCorrelationID
	return taskdomain.Task{ID: 1001, TaskTurnID: 1002, Status: taskdomain.TaskQueued}, false, nil
}

func TestSubmitOpenTaskUsesProjectScopedTaskContract(t *testing.T) {
	submission := &recordingSubmission{}
	service := NewService(nil, submission, nil, nil, nil, "ignored")
	identity := domain.Identity{ApplicationID: 2001, TenantID: 3001, Name: "Jira", CreatedByUserID: 101}
	view, err := service.SubmitOpenTask(context.Background(), identity, "issue-1", domain.CreateOpenTaskRequest{
		ProjectID: 4001, Name: "Task", Instruction: "Do work", Tags: []string{"bugfix"},
		ReviewEnabled: boolPointer(false), DeliveryPreference: "SAVE_ONLY", ExternalCorrelationID: "PROJ-1",
		ModelSelection: taskfacade.ModelSelection{CredentialID: 5001, Model: "gpt-seven"}, RuntimeReleaseID: 6001,
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.Links.Operator != "/workforce/tasks/1001" {
		t.Fatalf("unsafe/missing operator link: %+v", view.Links)
	}
	if view.TaskID != 1001 || view.ProjectID != 4001 || view.Status != "QUEUED" || submission.tenantID != 3001 || submission.creatorUserID != 101 || submission.projectID != 4001 {
		t.Fatalf("unexpected Project submission/view: submission=%+v view=%+v", submission, view)
	}
	if submission.request.Instruction != "Do work" || submission.request.ReviewEnabled == nil || *submission.request.ReviewEnabled || submission.request.DeliveryPreference != "SAVE_ONLY" || len(submission.request.Tags) != 1 {
		t.Fatalf("unexpected Task additions: %+v", submission.request)
	}
	if submission.applicationID != identity.ApplicationID || submission.correlation != "PROJ-1" || submission.idempotencyKey != domain.InternalIdempotencyKey(identity.ApplicationID, "issue-1") {
		t.Fatalf("Open ownership/idempotency not in atomic command: %+v", submission)
	}
}

func boolPointer(value bool) *bool { return &value }

func TestStableOpenErrorsDoNotExposeIncidentalMessages(t *testing.T) {
	status, code, message := StableError(context.DeadlineExceeded)
	if status != 500 || code != "INTERNAL_ERROR" || message != "The request could not be completed." {
		t.Fatalf("unexpected fallback error: %d %s %s", status, code, message)
	}
}

func TestOpenBindingConflictUsesPublicIdempotencyError(t *testing.T) {
	status, code, message := StableError(openinfra.ErrIdempotencyConflict)
	if status != 409 || code != "IDEMPOTENCY_CONFLICT" || message == "" {
		t.Fatalf("unexpected binding conflict: %d %s %q", status, code, message)
	}
}

func TestStableOpenProjectErrors(t *testing.T) {
	for _, test := range []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{projectdomain.ErrNotFound, 404, "PROJECT_NOT_FOUND"},
		{projectdomain.ErrArchived, 409, "PROJECT_ARCHIVED"},
		{projectdomain.ErrSourceUnavailable, 409, "PROJECT_SOURCE_NOT_READY"},
		{projectdomain.ErrInvalid, 409, "PROJECT_CONFIGURATION_INVALID"},
	} {
		status, code, _ := StableError(test.err)
		if status != test.wantStatus || code != test.wantCode {
			t.Fatalf("StableError(%v)=(%d,%s), want (%d,%s)", test.err, status, code, test.wantStatus, test.wantCode)
		}
	}
}

func TestOpenCapabilitiesExposeProjectScopedSubmissionOnly(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil, "default")
	view := service.GetOpenCapabilities(context.Background(), domain.Identity{
		ApplicationID: 9_007_199_254_740_993, TenantID: 42, CreatedByUserID: 101, Name: "Internal Codex",
	})
	if view.Schema != "seven.workforce.open-capabilities/v2" || view.ApplicationID != 9_007_199_254_740_993 {
		t.Fatalf("unexpected capabilities: %+v", view)
	}
	capability := view.TaskSubmission
	if !capability.RequiresModelSelection || !capability.RequiresRuntimeReleaseID || !capability.RequiresProjectID || !capability.SupportsReferenceAssets || !capability.SupportsAdditionalCompletionCheck || !capability.SupportsReviewToggle || !capability.SupportsDeliveryPreference || !capability.SupportsOutputs {
		t.Fatalf("unexpected task submission capability: %+v", capability)
	}
}

func TestOpenTaskOutputUsesPublicIdentityAndLinkOnly(t *testing.T) {
	view := openTaskOutput(resultfacade.AvailableOutput{
		ID: 9_007_199_254_740_993, TaskID: 9_007_199_254_740_995,
		Name: "result", OutputKind: "PATH", Status: "AVAILABLE", MediaType: "text/markdown",
		SHA256: "abc", SizeBytes: 9_007_199_254_740_997,
	}, 9_007_199_254_740_995)
	if view.OutputID != 9_007_199_254_740_993 || view.SizeBytes != 9_007_199_254_740_997 {
		t.Fatalf("unexpected output: %+v", view)
	}
	if view.Links.Download != "/api/open/workforce/v1/tasks/9007199254740995/outputs/9007199254740993/download" {
		t.Fatalf("unexpected download link: %q", view.Links.Download)
	}
}

func TestStableOpenOutputErrors(t *testing.T) {
	for _, test := range []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{resultfacade.ErrOutputNotFound, 404, "OUTPUT_NOT_FOUND"},
		{resultfacade.ErrOutputUnavailable, 503, "OUTPUT_UNAVAILABLE"},
	} {
		status, code, _ := StableError(test.err)
		if status != test.wantStatus || code != test.wantCode {
			t.Fatalf("StableError(%v)=(%d,%s), want (%d,%s)", test.err, status, code, test.wantStatus, test.wantCode)
		}
	}
}
