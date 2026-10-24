package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	notificationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	completioncheck "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	routingfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionrouting/facade"
	"strings"
	"unicode/utf8"

	projectdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/domain"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
	providerdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/domain"
	providerfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/facade"
	processorfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	reviewrunfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	taskfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/infrastructure"
	tasknotificationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/facade"
	workspaceinputdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/domain"
	workspaceinputfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/facade"
)

// requestValidationError is the closed, stable set of Task submission
// failures caused by caller-controlled input.  Keeping this type separate
// from incidental error strings lets the HTTP boundary return a useful 400
// without ever treating a database, Provider, or internal dependency error as
// client-safe merely because its prose happens to resemble a validation
// message.
type requestValidationError struct {
	code  string
	cause error
}

func (e *requestValidationError) Error() string { return e.code }
func (e *requestValidationError) Unwrap() error { return e.cause }
func (e *requestValidationError) Is(target error) bool {
	return target == taskfacade.ErrInvalidTaskSubmission
}

func invalidTaskRequest(code string) error { return &requestValidationError{code: code} }

// Preserve known routing reasons for Open's safe conflict response while
// retaining the existing Task HTTP input-validation classification.
func invalidRuntimeSelection(cause error) error {
	return &requestValidationError{code: cause.Error(), cause: cause}
}

// IsRequestValidationError is intentionally exported for the Task HTTP
// adapter.  It is not a general error classifier: only errors constructed in
// this application package may satisfy it.
func IsRequestValidationError(err error) bool {
	var target *requestValidationError
	return errors.As(err, &target)
}

type Service struct {
	store         *infrastructure.Store
	invocations   InvocationCanceller
	processors    processorfacade.Resolver
	inputs        workspaceinputfacade.Resolver
	scenarios     ScenarioReadinessChecker
	notifications NotificationSelectionValidator
	reviewRuns    reviewrunfacade.TaskSummaryProvider
	reviewReports reviewrunfacade.TaskReportProvider
	projects      projectfacade.TaskCompiler
	models        providerfacade.TaskSelectionValidator
	runtimes      routingfacade.RuntimeCatalog
}

type NotificationSelectionValidator interface {
	ResolveReadinessForOrganization(context.Context, int64, string, int64) (*notificationfacade.Readiness, error)
	ListAvailableGroupRoutes(context.Context, int64, int64, string) ([]notificationfacade.GroupRouteOption, error)
}

type notificationPreferenceResolver interface {
	IsWorkPreferenceEnabled(context.Context, int64, int64, string, string) (bool, error)
}

type ScenarioReadinessChecker interface {
	ValidateScenarioForSubmission(context.Context, int64, string) error
}

type InvocationCanceller interface {
	CancelAttemptInvocations(context.Context, int64, int64, string) error
	CancelReviewRunInvocationsForTask(context.Context, int64, int64, string) error
}

func NewService(store *infrastructure.Store, processors processorfacade.Resolver) *Service {
	return &Service{store: store, processors: processors}
}

func (s *Service) BindProjectCompiler(compiler projectfacade.TaskCompiler) { s.projects = compiler }
func (s *Service) BindModelSelectionValidator(validator providerfacade.TaskSelectionValidator) {
	s.models = validator
	s.store.SetModelSelectionValidator(validator)
}
func (s *Service) BindRuntimeCatalog(catalog routingfacade.RuntimeCatalog) {
	s.runtimes = catalog
	s.store.SetRuntimeCatalog(catalog)
}

func (s *Service) BindNotificationSelectionValidator(validator NotificationSelectionValidator) {
	if s != nil {
		s.notifications = validator
	}
}

// BindReviewRunSummaryProvider keeps Task reads on a stable facade. The
// summary is presentation-safe and never includes Provider, image, lease, or
// Worker internals frozen in the execution snapshot.
func (s *Service) BindReviewRunSummaryProvider(provider reviewrunfacade.TaskSummaryProvider) {
	if s != nil {
		s.reviewRuns = provider
	}
}

// BindReviewRunReportProvider adds the task-authorized report projection
// without widening Task's dependency to ReviewRun infrastructure or machine
// control contracts.
func (s *Service) BindReviewRunReportProvider(provider reviewrunfacade.TaskReportProvider) {
	if s != nil {
		s.reviewReports = provider
	}
}

// IndependentReviewReport returns only canonical business evidence for a
// Task already authorized by the caller. Frozen execution policy and all
// Worker/Provider control-plane facts intentionally remain absent.
func (s *Service) IndependentReviewReport(ctx context.Context, task domain.Task) (reviewrunfacade.TaskReport, error) {
	if !task.ReviewEnabled {
		return reviewrunfacade.TaskReport{Required: false}, nil
	}
	if s == nil || s.reviewReports == nil {
		return reviewrunfacade.TaskReport{Required: true, Status: "PENDING_FINALIZATION"}, nil
	}
	report, err := s.reviewReports.ReportForTask(ctx, task.TenantID, task.ID)
	if errors.Is(err, reviewrunfacade.ErrNotFound) {
		return reviewrunfacade.TaskReport{Required: true, Status: "PENDING_FINALIZATION"}, nil
	}
	return report, err
}

// IndependentReviewInputReader exposes only the frozen Turn requirement to
// the ReviewRun module through Task's stable facade.
func (s *Service) IndependentReviewInputReader() taskfacade.IndependentReviewInputReader { return s }

func (s *Service) IndependentReviewInput(ctx context.Context, tenantID, taskID, turnID int64) (taskfacade.IndependentReviewInput, error) {
	if s == nil || s.store == nil {
		return taskfacade.IndependentReviewInput{}, infrastructure.ErrNotFound
	}
	return s.store.IndependentReviewInput(ctx, tenantID, taskID, turnID)
}

type compiledTaskRequest struct {
	ProjectSpec                    *projectfacade.FrozenTaskSpec
	CompletionCheck                *completioncheck.Input
	TaskInput                      taskInputSelection
	Name                           string
	Tags                           []string
	GitPlan                        gitPlanRequest
	Instruction                    string
	ResultProcessorReleaseID       int64
	ResultProcessorParameters      json.RawMessage
	ResultProcessorConfigOverrides json.RawMessage
	ReviewEnabled                  bool
	ReviewPrompt                   string
	ModelSelection                 providerdomain.RuntimeSelection
	RuntimeReleaseID               int64
	Priority                       int
	ExecutionTimeoutSeconds        int
	Notification                   *taskNotificationSelection
}

type taskInputSelection struct {
	Mode                      string
	WorkspaceSourceRevisionID int64
	ReferenceAssetIDs         []int64
}

type gitPlanRequest struct {
	ProjectID                       int64
	ProjectVersionID                int64
	RepositoryConfigDigest          string
	ReviewBaseSourceRevisionID      int64
	ReviewBaseBranch                string
	ResultBranch                    string
	DeliveryMode                    string
	DeliveryAuthorization           string
	DeliveryAuthorizedByPrincipalID int64
	FullProjectImport               bool
}

type taskNotificationSelection struct {
	Enabled  *bool
	Events   []string
	Channels []taskNotificationChannel
}

type taskNotificationChannel struct {
	Kind    string
	RouteID int64
}

type submitTaskCommand struct {
	TenantID                 int64
	CreatorUserID            int64
	PrincipalType            string
	IntegrationApplicationID int64
	Channel                  string
	IdempotencyKey           string
	RequestHash              string
	RequestHashFinal         bool
	ScenarioKey              string
	ScenarioVersion          int
	ScenarioDigest           string
	ExternalCorrelationID    string
	Request                  compiledTaskRequest
}

// WorkspaceGCEligibility exposes the Task-owned retention decision through
// the stable facade. Worker-local storage never queries PostgreSQL directly.
func (s *Service) WorkspaceGCEligibility(ctx context.Context, taskID int64) (taskfacade.WorkspaceGCEligibility, error) {
	if s == nil || s.store == nil {
		return taskfacade.WorkspaceGCEligibility{TaskID: taskID}, errors.New("WORKSPACE_RETENTION_AUTHORITY_UNAVAILABLE")
	}
	return s.store.WorkspaceGCEligibility(ctx, taskID)
}

func (s *Service) CreateProjectTask(ctx context.Context, tenantID, creatorUserID, projectID int64, idempotencyKey string, request taskfacade.ProjectTaskRequest) (domain.Task, bool, error) {
	return s.createProjectTask(ctx, projectID, request, submitTaskCommand{TenantID: tenantID, CreatorUserID: creatorUserID, PrincipalType: "USER", Channel: "WEB", IdempotencyKey: idempotencyKey})
}

// CreateOpenProjectTask persists Application ownership in the same transaction
// as the Task graph. The Store checks the existing Application guard before
// both creates and idempotent replays; no post-create binding is required.
func (s *Service) CreateOpenProjectTask(ctx context.Context, command taskfacade.OpenProjectTaskCommand) (domain.Task, bool, error) {
	if command.TenantID < 1 || command.ApplicationID < 1 || command.CreatorUserID < 1 {
		return domain.Task{}, false, invalidTaskRequest("TASK_APPLICATION_IDENTITY_REQUIRED")
	}
	correlation := strings.TrimSpace(command.ExternalCorrelationID)
	if !utf8.ValidString(correlation) || utf8.RuneCountInString(correlation) < 1 || utf8.RuneCountInString(correlation) > 128 {
		return domain.Task{}, false, invalidTaskRequest("TASK_EXTERNAL_CORRELATION_INVALID")
	}
	return s.createProjectTask(ctx, command.ProjectID, command.Request, submitTaskCommand{
		TenantID: command.TenantID, CreatorUserID: command.CreatorUserID,
		PrincipalType: "APPLICATION", IntegrationApplicationID: command.ApplicationID,
		Channel: "OPEN_REST", IdempotencyKey: command.IdempotencyKey, ExternalCorrelationID: correlation,
	})
}

func projectTaskRequestHash(projectID int64, request taskfacade.ProjectTaskRequest, command submitTaskCommand) (string, error) {
	requestIdentity, err := json.Marshal(struct {
		CreatorID             int64                         `json:"creatorId,string"`
		ProjectID             int64                         `json:"projectId,string"`
		Request               taskfacade.ProjectTaskRequest `json:"request"`
		ApplicationID         int64                         `json:"applicationId,string,omitempty"`
		ExternalCorrelationID string                        `json:"externalCorrelationId,omitempty"`
	}{command.CreatorUserID, projectID, request, command.IntegrationApplicationID, command.ExternalCorrelationID})
	if err != nil {
		return "", err
	}
	return infrastructure.RequestHashText(string(requestIdentity)), nil
}

func (s *Service) createProjectTask(ctx context.Context, projectID int64, request taskfacade.ProjectTaskRequest, command submitTaskCommand) (domain.Task, bool, error) {
	tenantID, creatorUserID := command.TenantID, command.CreatorUserID
	if s == nil || s.projects == nil || s.models == nil || s.runtimes == nil || projectID < 1 {
		return domain.Task{}, false, invalidTaskRequest("PROJECT_NOT_AVAILABLE")
	}
	idempotencyKey := strings.TrimSpace(command.IdempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 128 {
		return domain.Task{}, false, invalidTaskRequest("TASK_IDEMPOTENCY_KEY_INVALID")
	}
	requestHash, err := projectTaskRequestHash(projectID, request, command)
	if err != nil {
		return domain.Task{}, false, err
	}
	if existing, found, conflict, lookupErr := s.store.FindIdempotent(ctx, tenantID, creatorUserID, idempotencyKey, requestHash, command.IntegrationApplicationID); lookupErr != nil {
		return domain.Task{}, false, lookupErr
	} else if conflict {
		return domain.Task{}, false, taskfacade.ErrIdempotencyConflict
	} else if found {
		return existing, true, nil
	}
	spec, err := s.projects.CompileForTask(ctx, tenantID, projectID, request.Additions())
	if err != nil {
		return domain.Task{}, false, err
	}
	modelSelection, err := s.models.ValidateTaskSelection(ctx, tenantID, request.ModelSelection.CredentialID, request.ModelSelection.Model)
	if err != nil {
		return domain.Task{}, false, err
	}
	if err = s.runtimes.ValidateTaskSelection(ctx, tenantID, spec.ProjectVersionID, request.RuntimeReleaseID, modelSelection.Protocol, spec.ExecutionRequirements); err != nil {
		if errors.Is(err, routingfacade.ErrRuntimeSelectionRequired) || errors.Is(err, routingfacade.ErrRuntimeSelectionUnavailable) || errors.Is(err, routingfacade.ErrRuntimeSelectionIncompatible) {
			return domain.Task{}, false, invalidRuntimeSelection(err)
		}
		return domain.Task{}, false, err
	}
	inputMode := workspaceinputdomain.SourceKindEmptyWorkspace
	if spec.WorkspaceSourceRevisionID > 0 {
		inputMode = workspaceinputdomain.SourceKindSourceRevision
	}
	gitPlan := projectGitPlanRequest(spec.GitPlan)
	notification := &taskNotificationSelection{Enabled: &spec.Notification.Enabled, Events: append([]string(nil), spec.Notification.Events...)}
	if spec.Notification.GroupRouteID > 0 {
		notification.Channels = []taskNotificationChannel{{Kind: "GROUP_ROUTE", RouteID: spec.Notification.GroupRouteID}}
	}
	compiled := compiledTaskRequest{
		ProjectSpec:     &spec,
		CompletionCheck: spec.CompletionCheck,
		TaskInput:       taskInputSelection{Mode: inputMode, WorkspaceSourceRevisionID: spec.WorkspaceSourceRevisionID, ReferenceAssetIDs: append([]int64(nil), request.ReferenceAssetIDs...)},
		Name:            spec.Name, Tags: spec.Tags, GitPlan: gitPlan, Instruction: spec.Instruction,
		ResultProcessorReleaseID:  spec.ResultProcessorReleaseID,
		ResultProcessorParameters: spec.ResultProcessorParameters, ResultProcessorConfigOverrides: json.RawMessage(`{}`),
		ReviewEnabled: spec.ReviewEnabled, ReviewPrompt: spec.ReviewPrompt, ModelSelection: modelSelection, RuntimeReleaseID: request.RuntimeReleaseID, Priority: 50, ExecutionTimeoutSeconds: request.ExecutionTimeoutSeconds,
		Notification: notification,
	}
	command.IdempotencyKey, command.RequestHash, command.RequestHashFinal, command.Request = idempotencyKey, requestHash, true, compiled
	task, replayed, err := s.create(ctx, command)
	if errors.Is(err, infrastructure.ErrProjectConfigurationChanged) {
		return domain.Task{}, false, projectdomain.ErrConflict
	}
	if errors.Is(err, routingfacade.ErrRuntimeSelectionRequired) || errors.Is(err, routingfacade.ErrRuntimeSelectionUnavailable) || errors.Is(err, routingfacade.ErrRuntimeSelectionIncompatible) {
		return domain.Task{}, false, invalidRuntimeSelection(err)
	}
	if err != nil {
		return domain.Task{}, false, err
	}
	return task, replayed, nil
}

func projectGitPlanRequest(plan projectfacade.FrozenGitPlan) gitPlanRequest {
	// A CODEBASE project still contributes its frozen source revision when the
	// Task elects SAVE_ONLY. The workspace input owns that source identity; a
	// Delivery NONE plan must remain completely empty so it cannot accidentally
	// carry a repository target into the delivery boundary.
	if strings.EqualFold(strings.TrimSpace(plan.DeliveryMode), workspaceinputdomain.DeliveryNone) {
		return gitPlanRequest{DeliveryMode: workspaceinputdomain.DeliveryNone}
	}
	return gitPlanRequest{
		DeliveryMode: plan.DeliveryMode, DeliveryAuthorization: plan.DeliveryAuthorization,
		DeliveryAuthorizedByPrincipalID: plan.DeliveryAuthorizedByPrincipalID,
		ProjectID:                       plan.ProjectID, ProjectVersionID: plan.ProjectVersionID, RepositoryConfigDigest: plan.RepositoryConfigDigest, ReviewBaseSourceRevisionID: plan.SourceRevisionID,
		ReviewBaseBranch: plan.ReviewBaseBranch, ResultBranch: plan.ResultBranchPattern, FullProjectImport: true,
	}
}

func (s *Service) create(ctx context.Context, command submitTaskCommand) (domain.Task, bool, error) {
	tenantID := command.TenantID
	creatorUserID := command.CreatorUserID
	if creatorUserID <= 0 {
		return domain.Task{}, false, invalidTaskRequest("TASK_CREATOR_IDENTITY_REQUIRED")
	}
	idempotencyKey := strings.TrimSpace(command.IdempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 128 {
		return domain.Task{}, false, invalidTaskRequest("TASK_IDEMPOTENCY_KEY_INVALID")
	}
	request := command.Request
	completionSnapshot, completionErr := completioncheck.Freeze(request.CompletionCheck)
	if completionErr != nil {
		return domain.Task{}, false, invalidTaskRequest("COMPLETION_CHECK_INVALID")
	}
	if completionSnapshot != nil {
		request.CompletionCheck = &completionSnapshot.Input
	}
	principalType := strings.ToUpper(strings.TrimSpace(command.PrincipalType))
	if principalType == "" {
		principalType = "USER"
	}
	if principalType != "USER" && principalType != "APPLICATION" {
		return domain.Task{}, false, invalidTaskRequest("TASK_PRINCIPAL_TYPE_INVALID")
	}
	request.TaskInput.Mode = strings.ToUpper(strings.TrimSpace(request.TaskInput.Mode))
	request.GitPlan.ReviewBaseBranch = strings.TrimSpace(request.GitPlan.ReviewBaseBranch)
	request.GitPlan.ResultBranch = strings.TrimSpace(request.GitPlan.ResultBranch)
	request.GitPlan.DeliveryMode = strings.ToUpper(strings.TrimSpace(request.GitPlan.DeliveryMode))
	request.GitPlan.DeliveryAuthorization = strings.ToUpper(strings.TrimSpace(request.GitPlan.DeliveryAuthorization))
	request.Instruction = strings.TrimSpace(request.Instruction)
	request.Name = strings.TrimSpace(request.Name)
	if request.Tags == nil {
		request.Tags = []string{}
	}
	if request.Priority < 0 || request.Priority > 100 {
		return domain.Task{}, false, invalidTaskRequest("TASK_PRIORITY_INVALID")
	}
	if request.ExecutionTimeoutSeconds < 0 || request.ExecutionTimeoutSeconds > 2147483647 {
		return domain.Task{}, false, invalidTaskRequest("TASK_EXECUTION_TIMEOUT_INVALID")
	}
	notificationSubscription, err := normalizeTaskNotificationSelection(request.Notification)
	if err != nil {
		return domain.Task{}, false, err
	}
	if err := s.validateTaskNotificationSelection(ctx, tenantID, creatorUserID, notificationSubscription); err != nil {
		return domain.Task{}, false, err
	}
	if len(request.ResultProcessorParameters) == 0 {
		request.ResultProcessorParameters = json.RawMessage(`{}`)
	}
	if len(request.ResultProcessorConfigOverrides) == 0 {
		request.ResultProcessorConfigOverrides = json.RawMessage(`{}`)
	}
	var integrationApplicationID int64
	if principalType == "APPLICATION" {
		integrationApplicationID = command.IntegrationApplicationID
		if integrationApplicationID <= 0 {
			return domain.Task{}, false, invalidTaskRequest("TASK_APPLICATION_IDENTITY_REQUIRED")
		}
	}
	precomputedHash := strings.TrimSpace(command.RequestHash)
	if precomputedHash != "" && !command.RequestHashFinal {
		if completionSnapshot != nil {
			precomputedHash = infrastructure.RequestHashText(precomputedHash + "\x00completionCheck:" + completionSnapshot.Digest)
		}
		if existing, found, conflict, err := s.store.FindIdempotent(ctx, tenantID, creatorUserID, idempotencyKey, precomputedHash, integrationApplicationID); err != nil {
			return domain.Task{}, false, err
		} else if conflict {
			return domain.Task{}, false, infrastructure.ErrIdempotencyConflict
		} else if found {
			return existing, true, nil
		}
	}
	if precomputedHash != "" && command.RequestHashFinal {
		if existing, found, conflict, err := s.store.FindIdempotent(ctx, tenantID, creatorUserID, idempotencyKey, precomputedHash, integrationApplicationID); err != nil {
			return domain.Task{}, false, err
		} else if conflict {
			return domain.Task{}, false, infrastructure.ErrIdempotencyConflict
		} else if found {
			return existing, true, nil
		}
	}
	if strings.TrimSpace(command.ScenarioKey) != "" {
		if s.scenarios == nil {
			return domain.Task{}, false, errors.New("scenario readiness checker is unavailable")
		}
		if err := s.scenarios.ValidateScenarioForSubmission(ctx, tenantID, strings.TrimSpace(command.ScenarioKey)); err != nil {
			return domain.Task{}, false, err
		}
	}
	var processorParams map[string]any
	if err := json.Unmarshal(request.ResultProcessorParameters, &processorParams); err != nil || processorParams == nil {
		return domain.Task{}, false, invalidTaskRequest("TASK_RESULT_PROCESSOR_PARAMETERS_INVALID")
	}
	var processorConfigOverrides map[string]any
	if err := json.Unmarshal(request.ResultProcessorConfigOverrides, &processorConfigOverrides); err != nil || processorConfigOverrides == nil {
		return domain.Task{}, false, invalidTaskRequest("TASK_RESULT_PROCESSOR_CONFIG_OVERRIDES_INVALID")
	}
	if s.processors == nil {
		return domain.Task{}, false, fmt.Errorf("Result Processor resolution is unavailable")
	}
	processorExecution, err := s.processors.ResolveExecution(ctx, tenantID, "RELEASE", request.ResultProcessorReleaseID, request.ResultProcessorParameters, request.ResultProcessorConfigOverrides)
	if err != nil {
		return domain.Task{}, false, err
	}
	if !taskfacade.ValidInstruction(request.Instruction) {
		return domain.Task{}, false, invalidTaskRequest("TASK_INSTRUCTION_INVALID")
	}
	request.Name, request.Tags, err = domain.NormalizeTaskMetadata(request.Name, request.Instruction, request.Tags)
	if err != nil {
		return domain.Task{}, false, err
	}
	if s.inputs == nil {
		return domain.Task{}, false, fmt.Errorf("Task Input resolution is unavailable")
	}
	if request.TaskInput.Mode != workspaceinputdomain.SourceKindEmptyWorkspace && request.TaskInput.Mode != workspaceinputdomain.SourceKindSourceRevision {
		return domain.Task{}, false, invalidTaskRequest("TASK_INPUT_MODE_INVALID")
	}
	if request.TaskInput.Mode == workspaceinputdomain.SourceKindEmptyWorkspace && request.TaskInput.WorkspaceSourceRevisionID != 0 || request.TaskInput.Mode == workspaceinputdomain.SourceKindSourceRevision && request.TaskInput.WorkspaceSourceRevisionID == 0 {
		return domain.Task{}, false, invalidTaskRequest("TASK_INPUT_SOURCE_CONFLICT")
	}
	if request.GitPlan.DeliveryMode == "" {
		request.GitPlan.DeliveryMode = workspaceinputdomain.DeliveryNone
	}
	if request.GitPlan.DeliveryMode == workspaceinputdomain.DeliveryNone {
		request.GitPlan.DeliveryAuthorization = ""
	} else if request.GitPlan.DeliveryAuthorization == "" {
		request.GitPlan.DeliveryAuthorization = workspaceinputdomain.DeliveryAuthorizationAuto
	}
	if request.GitPlan.DeliveryAuthorization == workspaceinputdomain.DeliveryAuthorizationAuto && request.GitPlan.DeliveryAuthorizedByPrincipalID < 1 {
		return domain.Task{}, false, invalidTaskRequest("TASK_DELIVERY_APPROVAL_PERMISSION_REQUIRED")
	}
	if request.GitPlan.DeliveryMode != workspaceinputdomain.DeliveryNone && request.GitPlan.ResultBranch == "" {
		sum := infrastructure.RequestHashText(fmt.Sprintf("%d\x00%d\x00%s", tenantID, creatorUserID, idempotencyKey))
		request.GitPlan.ResultBranch = "seven/task-" + sum[:16]
	}
	effectiveReferences := append([]int64(nil), request.TaskInput.ReferenceAssetIDs...)
	if completionSnapshot != nil {
		for _, id := range completionSnapshot.Input.ReferenceAssetIDs {
			found := false
			for _, existing := range effectiveReferences {
				if existing == id {
					found = true
					break
				}
			}
			if !found {
				effectiveReferences = append(effectiveReferences, id)
			}
		}
	}
	frozenInput, err := s.inputs.ResolveExecutionInput(ctx, tenantID, workspaceinputfacade.TaskInputSelection{Mode: request.TaskInput.Mode, WorkspaceSourceRevisionID: request.TaskInput.WorkspaceSourceRevisionID, ReferenceAssetIDs: effectiveReferences, GitPlan: workspaceinputdomain.GitPlan{DeliveryMode: request.GitPlan.DeliveryMode, DeliveryAuthorization: request.GitPlan.DeliveryAuthorization, ProjectID: request.GitPlan.ProjectID, ProjectVersionID: request.GitPlan.ProjectVersionID, RepositoryConfigDigest: request.GitPlan.RepositoryConfigDigest, ReviewBaseSourceRevisionID: request.GitPlan.ReviewBaseSourceRevisionID, ReviewBaseBranch: request.GitPlan.ReviewBaseBranch, ResultBranch: request.GitPlan.ResultBranch, FullProjectImport: request.GitPlan.FullProjectImport}})
	if err != nil {
		return domain.Task{}, false, err
	}
	hash := precomputedHash
	if hash == "" {
		hash, err = createRequestHash(creatorUserID, request)
		if err != nil {
			return domain.Task{}, false, err
		}
	}
	return s.store.Create(ctx, infrastructure.CreateParams{
		ProjectSpec:     request.ProjectSpec,
		CompletionCheck: completionSnapshot,
		TenantID:        tenantID, IdempotencyKey: idempotencyKey, RequestHash: hash,
		SubmissionChannel:        strings.TrimSpace(command.Channel),
		PrincipalType:            principalType,
		IntegrationApplicationID: integrationApplicationID,
		ScenarioKey:              strings.TrimSpace(command.ScenarioKey), ScenarioVersion: command.ScenarioVersion,
		ScenarioDigest: strings.TrimSpace(command.ScenarioDigest), ExternalCorrelationID: strings.TrimSpace(command.ExternalCorrelationID),
		TaskInput:                    frozenInput,
		Name:                         request.Name,
		Tags:                         request.Tags,
		CreatedBy:                    creatorUserID,
		Instruction:                  request.Instruction,
		ResultProcessorSelectionMode: "RELEASE",
		ResultProcessorExecution:     processorExecution,
		ReviewEnabled:                request.ReviewEnabled,
		ReviewPrompt:                 request.ReviewPrompt,
		ModelCredentialID:            request.ModelSelection.CredentialID,
		CredentialVersion:            request.ModelSelection.CredentialVersion,
		ModelProtocol:                request.ModelSelection.Protocol,
		PrimaryModel:                 request.ModelSelection.PrimaryModel,
		SelectedRuntimeReleaseID:     request.RuntimeReleaseID,
		Priority:                     request.Priority,
		ExecutionTimeoutSeconds:      request.ExecutionTimeoutSeconds,
		NotificationSubscription:     &notificationSubscription,
	})
}

func (s *Service) ListRuntimeOptions(ctx context.Context, tenantID, projectID, modelCredentialID int64) (routingfacade.RuntimeOptions, error) {
	if s == nil || s.runtimes == nil {
		return routingfacade.RuntimeOptions{}, routingfacade.ErrRuntimeSelectionUnavailable
	}
	return s.runtimes.ListTaskOptions(ctx, tenantID, projectID, modelCredentialID)
}

func normalizeTaskNotificationSelection(selection *taskNotificationSelection) (tasknotificationfacade.SubscriptionCommand, error) {
	command := tasknotificationfacade.DefaultSubscriptionCommand()
	if selection == nil {
		return command, nil
	}
	if selection.Enabled != nil {
		command.Enabled = *selection.Enabled
	}
	if len(selection.Events) > 0 {
		command.Events = make([]tasknotificationfacade.FactType, 0, len(selection.Events))
		for _, raw := range selection.Events {
			event := tasknotificationfacade.FactType(strings.ToUpper(strings.TrimSpace(raw)))
			if !event.Valid() {
				return tasknotificationfacade.SubscriptionCommand{}, invalidTaskRequest("TASK_NOTIFICATION_FACT_INVALID")
			}
			command.Events = append(command.Events, event)
		}
	}
	if len(selection.Channels) > 0 {
		command.Channels = make([]tasknotificationfacade.ChannelSelection, 0, len(selection.Channels))
		for _, raw := range selection.Channels {
			channel := tasknotificationfacade.ChannelType(strings.ToUpper(strings.TrimSpace(raw.Kind)))
			if !channel.Valid() {
				return tasknotificationfacade.SubscriptionCommand{}, invalidTaskRequest("TASK_NOTIFICATION_FACT_INVALID")
			}
			command.Channels = append(command.Channels, tasknotificationfacade.ChannelSelection{Type: channel, GroupRouteID: raw.RouteID})
		}
	}
	return command, nil
}

func (s *Service) validateTaskNotificationSelection(ctx context.Context, tenantID, userID int64, command tasknotificationfacade.SubscriptionCommand) error {
	// A disabled subscription records the user's explicit opt-out. It must not
	// be rejected merely because one of the otherwise-default channels is
	// disabled or unavailable for that user.
	if !command.Enabled {
		return nil
	}
	needsPlatform := false
	groupIDs := make(map[int64]struct{})
	for _, channel := range command.Channels {
		if channel.Type != tasknotificationfacade.ChannelInApp {
			needsPlatform = true
		}
		if channel.Type == tasknotificationfacade.ChannelGroupRoute {
			groupIDs[channel.GroupRouteID] = struct{}{}
		}
	}
	if s == nil || s.notifications == nil {
		if needsPlatform {
			return invalidTaskRequest("TASK_NOTIFICATION_CHANNEL_NOT_READY")
		}
		return nil
	}
	if preferences, ok := s.notifications.(notificationPreferenceResolver); ok {
		preferenceChannel := map[tasknotificationfacade.ChannelType]string{tasknotificationfacade.ChannelInApp: "IN_APP", tasknotificationfacade.ChannelEmailUser: "EMAIL_USER", tasknotificationfacade.ChannelFeishuDirect: "FEISHU_DIRECT", tasknotificationfacade.ChannelWeComDirect: "WECOM_DIRECT"}
		for _, event := range command.Events {
			for _, channel := range command.Channels {
				key, personal := preferenceChannel[channel.Type]
				if !personal {
					continue
				}
				enabled, preferenceErr := preferences.IsWorkPreferenceEnabled(ctx, tenantID, userID, string(event), key)
				if preferenceErr != nil {
					return preferenceErr
				}
				if !enabled {
					return invalidTaskRequest("TASK_NOTIFICATION_CHANNEL_DISABLED_BY_PREFERENCE")
				}
			}
		}
	}
	if !needsPlatform {
		return nil
	}
	ready, err := s.notifications.ResolveReadinessForOrganization(ctx, tenantID, "workforce_task_action_required", userID)
	if err != nil {
		return err
	}
	if ready == nil {
		return invalidTaskRequest("TASK_NOTIFICATION_CHANNEL_NOT_READY")
	}
	for _, channel := range command.Channels {
		switch channel.Type {
		case tasknotificationfacade.ChannelEmailUser:
			if !ready.Email.Available || !ready.Email.RecipientReady {
				return invalidTaskRequest("NOTIFICATION_RECIPIENT_NOT_VERIFIED")
			}
		case tasknotificationfacade.ChannelFeishuDirect:
			if !ready.Feishu.DirectReady {
				return invalidTaskRequest("EXTERNAL_IDENTITY_NOT_VERIFIED")
			}
		case tasknotificationfacade.ChannelWeComDirect:
			if !ready.WeCom.DirectReady {
				return invalidTaskRequest("EXTERNAL_IDENTITY_NOT_VERIFIED")
			}
		}
	}
	if len(groupIDs) > 0 {
		available, err := s.notifications.ListAvailableGroupRoutes(ctx, tenantID, userID, "workforce_task_action_required")
		if err != nil {
			return err
		}
		allowed := make(map[int64]struct{}, len(available))
		for _, route := range available {
			allowed[route.ID] = struct{}{}
		}
		for id := range groupIDs {
			if _, ok := allowed[id]; !ok {
				return invalidTaskRequest("NOTIFICATION_GROUP_ROUTE_NOT_ENABLED")
			}
		}
	}
	return nil
}

func createRequestHash(creatorUserID int64, request compiledTaskRequest) (string, error) {
	return infrastructure.RequestHash(struct {
		CreatorUserID int64               `json:"creatorUserId,string"`
		Request       compiledTaskRequest `json:"request"`
	}{CreatorUserID: creatorUserID, Request: request})
}

type MetadataRequest struct {
	Name                    string   `json:"name"`
	Tags                    []string `json:"tags"`
	ExpectedMetadataVersion int64    `json:"expectedMetadataVersion,string"`
}

func (s *Service) UpdateMetadata(ctx context.Context, tenantID int64, taskID int64, actor int64, request MetadataRequest) (domain.Task, error) {
	name, tags, err := domain.NormalizeTaskMetadata(request.Name, "", request.Tags)
	if err != nil {
		return domain.Task{}, err
	}
	return s.store.UpdateMetadata(ctx, tenantID, taskID, actor, name, tags, request.ExpectedMetadataVersion)
}
func (s *Service) SetTaskInputResolver(resolver workspaceinputfacade.Resolver) { s.inputs = resolver }
func (s *Service) SetScenarioReadinessChecker(checker ScenarioReadinessChecker) {
	s.scenarios = checker
}

func (s *Service) Get(ctx context.Context, id int64) (domain.Task, error) {
	task, err := s.store.Get(ctx, id)
	if err != nil {
		return domain.Task{}, err
	}
	return s.decorateIndependentReview(ctx, task), nil
}

func (s *Service) List(ctx context.Context, limit int) ([]domain.Task, error) {
	tasks, err := s.store.List(ctx, limit)
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		tasks[i] = s.decorateIndependentReview(ctx, tasks[i])
	}
	return tasks, nil
}

func (s *Service) ListTenant(ctx context.Context, tenantID int64, limit int) ([]domain.Task, error) {
	tasks, err := s.store.ListTenant(ctx, tenantID, limit)
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		tasks[i] = s.decorateIndependentReview(ctx, tasks[i])
	}
	return tasks, nil
}

func (s *Service) ListProject(ctx context.Context, tenantID, projectID int64, limit int) ([]domain.Task, error) {
	tasks, err := s.store.ListProject(ctx, tenantID, projectID, limit)
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		tasks[i] = s.decorateIndependentReview(ctx, tasks[i])
	}
	return tasks, nil
}

func (s *Service) ListPage(ctx context.Context, filter infrastructure.ListFilter) (infrastructure.TaskPage, error) {
	page, err := s.store.ListPage(ctx, filter)
	if err != nil {
		return infrastructure.TaskPage{}, err
	}
	for i := range page.Items {
		page.Items[i] = s.decorateIndependentReview(ctx, page.Items[i])
	}
	return page, nil
}

func (s *Service) decorateIndependentReview(ctx context.Context, task domain.Task) domain.Task {
	if !task.ReviewEnabled {
		return task
	}
	// A frozen policy before Finalization has a well-defined business state;
	// never infer a ReviewRun from TaskStatus or a Processor manual-review.
	task.IndependentReviewStatus = "PENDING_FINALIZATION"
	if s == nil || s.reviewRuns == nil {
		return task
	}
	summary, err := s.reviewRuns.SummaryForTask(ctx, task.TenantID, task.ID)
	if err == nil && summary.Required && summary.Status != "" {
		task.IndependentReviewStatus = summary.Status
	}
	return task
}

func (s *Service) Backlog(ctx context.Context, tenantID int64) (domain.BacklogSummary, error) {
	return s.store.Backlog(ctx, tenantID)
}

func (s *Service) SetInvocationCanceller(canceller InvocationCanceller) { s.invocations = canceller }

func (s *Service) Cancel(ctx context.Context, id int64, actor int64, reason string) (infrastructure.CancelResult, error) {
	result, err := s.store.Cancel(ctx, id, actor, strings.TrimSpace(reason))
	if err == nil && s.invocations != nil && result.TenantID != 0 {
		if result.AttemptID != 0 {
			if cancelErr := s.invocations.CancelAttemptInvocations(ctx, result.TenantID, result.AttemptID, "TASK_CANCELLED"); cancelErr != nil {
				return result, cancelErr
			}
		}
		if cancelErr := s.invocations.CancelReviewRunInvocationsForTask(ctx, result.TenantID, id, "TASK_CANCELLED"); cancelErr != nil {
			return result, cancelErr
		}
	}
	return result, err
}

func (s *Service) Events(ctx context.Context, id int64, afterID int64) ([]domain.Event, error) {
	return s.store.Events(ctx, id, afterID, 200)
}

func (s *Service) Artifacts(ctx context.Context, id int64) ([]domain.Artifact, error) {
	items, err := s.store.Artifacts(ctx, id)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].DownloadURL = fmt.Sprintf("/api/workforce/tasks/%d/artifacts/%d/download", items[i].TaskID, items[i].ID)
	}
	return items, nil
}

func IsNotFound(err error) bool { return errors.Is(err, infrastructure.ErrNotFound) }

func IsConflict(err error) bool {
	return errors.Is(err, taskfacade.ErrIdempotencyConflict) || errors.Is(err, infrastructure.ErrIdempotencyConflict) || errors.Is(err, domain.ErrTaskMetadataConflict)
}

// ErrorCode exposes only stable, user-actionable task-create errors. The
// controller must not infer an API contract from incidental error prose.
func ErrorCode(err error) string {
	var validation *requestValidationError
	if errors.As(err, &validation) {
		return validation.code
	}
	if errors.Is(err, infrastructure.ErrLegacyProcessorTaskActive) {
		return "RESULT_PROCESSOR_LEGACY_TASK_ACTIVE"
	}
	// These exact codes also cover the final, transaction-scoped selection
	// check. Never expose incidental database or encryption error text.
	switch err.Error() {
	case "MODEL_CREDENTIAL_NOT_FOUND", "MODEL_CREDENTIAL_DISABLED", "TASK_MODEL_NOT_IN_CATALOG", "TASK_MODEL_SELECTION_REQUIRED":
		return err.Error()
	}
	for _, code := range []string{"TASK_CREATOR_IDENTITY_REQUIRED", "TASK_NAME_INVALID", "TASK_TAG_INVALID", "TASK_TAG_LIMIT_EXCEEDED", "TASK_INPUT_MODE_INVALID", "TASK_INPUT_SOURCE_CONFLICT", "TASK_SOURCE_REQUIRED_FOR_GIT_DELIVERY", "WORKSPACE_SOURCE_REVISION_NOT_AVAILABLE", "REFERENCE_ASSET_NOT_AVAILABLE", "TASK_INPUT_INVALID", "TASK_GIT_PLAN_INVALID", "TARGET_BASE_REVISION_NOT_FROZEN", "REPOSITORY_CAPABILITY_UNAVAILABLE", "TASK_NOTIFICATION_FACT_INVALID"} {
		if strings.HasPrefix(err.Error(), code) {
			return code
		}
	}
	return ""
}

func SafeErrorMessage(code string) string {
	return "task request is invalid"
}
