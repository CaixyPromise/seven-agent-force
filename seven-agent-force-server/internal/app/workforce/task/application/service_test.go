package application

import (
	"context"
	"testing"

	notificationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
	tasknotificationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/facade"
	workspaceinputdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/domain"
)

type unexpectedNotificationValidator struct{}

func (unexpectedNotificationValidator) ResolveReadinessForOrganization(context.Context, int64, string, int64) (*notificationfacade.Readiness, error) {
	panic("disabled notification selection must not resolve external readiness")
}

func (unexpectedNotificationValidator) ListAvailableGroupRoutes(context.Context, int64, int64, string) ([]notificationfacade.GroupRouteOption, error) {
	panic("disabled notification selection must not enumerate external routes")
}

func (unexpectedNotificationValidator) IsWorkPreferenceEnabled(context.Context, int64, int64, string, string) (bool, error) {
	panic("disabled notification selection must not inspect channel preferences")
}

func TestCreateRequestHashBindsCreatorIdentity(t *testing.T) {
	request := compiledTaskRequest{Name: "same", Instruction: "same"}
	first, err := createRequestHash(101, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := createRequestHash(102, request)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("different Task creators must not share a request digest")
	}
}

func TestCreateRequestHashBindsReviewToggle(t *testing.T) {
	base := compiledTaskRequest{Name: "same", Instruction: "same"}
	firstRequest := base
	firstRequest.ReviewEnabled = true
	secondRequest := base
	secondRequest.ReviewEnabled = false
	first, err := createRequestHash(101, firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	second, err := createRequestHash(101, secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("normal Task request digest must retain the independent review toggle")
	}
}

func TestCreateRequestHashBindsSelectedRuntimeRelease(t *testing.T) {
	firstRequest := compiledTaskRequest{Name: "same", Instruction: "same", RuntimeReleaseID: 9007199254740993}
	secondRequest := firstRequest
	secondRequest.RuntimeReleaseID = 9007199254740995
	first, err := createRequestHash(101, firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	second, err := createRequestHash(101, secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("different execution image releases must not share a request digest")
	}
}

func TestCreateRequestHashBindsExecutionLimit(t *testing.T) {
	unlimited := compiledTaskRequest{Name: "same", Instruction: "same"}
	limited := unlimited
	limited.ExecutionTimeoutSeconds = 7200
	first, err := createRequestHash(101, unlimited)
	if err != nil {
		t.Fatal(err)
	}
	second, err := createRequestHash(101, limited)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("Task execution limit must be included in the idempotency digest")
	}
}

func TestInvalidNotificationSelectionKeepsStableTaskErrorCode(t *testing.T) {
	_, err := normalizeTaskNotificationSelection(&taskNotificationSelection{
		Events: []string{"NOT_A_TASK_NOTIFICATION_FACT"},
	})
	if err == nil {
		t.Fatal("invalid Task notification event was accepted")
	}
	if got := ErrorCode(err); got != "TASK_NOTIFICATION_FACT_INVALID" {
		t.Fatalf("ErrorCode=%q, want TASK_NOTIFICATION_FACT_INVALID", got)
	}
}

func TestDisabledNotificationSelectionSkipsChannelReadiness(t *testing.T) {
	service := &Service{notifications: unexpectedNotificationValidator{}}
	command := tasknotificationfacade.DefaultSubscriptionCommand()
	command.Enabled = false
	if err := service.validateTaskNotificationSelection(context.Background(), 101, 202, command); err != nil {
		t.Fatalf("disabled notification selection should be accepted without readiness checks: %v", err)
	}
}

func TestProjectGitPlanRequestClearsDeliveryIdentityForSaveOnlyTask(t *testing.T) {
	request := projectGitPlanRequest(projectfacade.FrozenGitPlan{
		ProjectID:                       101,
		SourceRevisionID:                202,
		ReviewBaseBranch:                "main",
		ResultBranchPattern:             "codex/task-{task-id}",
		DeliveryMode:                    workspaceinputdomain.DeliveryNone,
		DeliveryAuthorization:           workspaceinputdomain.DeliveryAuthorizationAuto,
		DeliveryAuthorizedByPrincipalID: 303,
	})
	if request.DeliveryMode != workspaceinputdomain.DeliveryNone {
		t.Fatalf("DeliveryMode=%q, want NONE", request.DeliveryMode)
	}
	if request.ProjectID != 0 || request.ReviewBaseSourceRevisionID != 0 || request.ReviewBaseBranch != "" || request.ResultBranch != "" || request.DeliveryAuthorization != "" || request.DeliveryAuthorizedByPrincipalID != 0 || request.FullProjectImport {
		t.Fatalf("SAVE_ONLY task leaked delivery identity: %+v", request)
	}
}
