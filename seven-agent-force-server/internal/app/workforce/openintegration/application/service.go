package application

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	routingfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionrouting/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/domain"
	openinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/infrastructure"
	projectdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/domain"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
	providerfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/facade"
	resultfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	taskfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
	secretvalue "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
)

type WebhookURLValidator interface {
	Validate(context.Context, string) error
}

// ProjectTaskSubmission writes authenticated Application ownership with the
// existing Project Task graph in one transaction.
type ProjectTaskSubmission interface {
	CreateOpenProjectTask(context.Context, taskfacade.OpenProjectTaskCommand) (taskdomain.Task, bool, error)
}

type Service struct {
	store        *openinfra.Store
	tasks        ProjectTaskSubmission
	outputs      resultfacade.ResultOutputAccess
	secrets      secretvalue.Service
	urlValidator WebhookURLValidator
	models       providerfacade.TaskModelCatalog
	projects     projectfacade.RequirementsCompiler
	runtimes     routingfacade.RuntimeCatalog
}

func NewService(store *openinfra.Store, tasks ProjectTaskSubmission, outputs resultfacade.ResultOutputAccess, secrets secretvalue.Service, urlValidator WebhookURLValidator, _ string) *Service {
	service := &Service{store: store, tasks: tasks, outputs: outputs, secrets: secrets, urlValidator: urlValidator}
	return service
}

type CreateApplicationRequest struct {
	Name       string `json:"name"`
	WebhookURL string `json:"webhookUrl"`
}

type CreatedApplication struct {
	Application              domain.Application `json:"application"`
	APIKey                   string             `json:"apiKey"`
	APICredentialID          int64              `json:"apiCredentialId,string"`
	APICredentialVersion     int                `json:"apiCredentialVersion"`
	SigningSecret            string             `json:"signingSecret,omitempty"`
	SigningCredentialID      *int64             `json:"signingCredentialId,string,omitempty"`
	SigningCredentialVersion *int               `json:"signingCredentialVersion,omitempty"`
}

type APIKeyRotation struct {
	ApplicationID        int64     `json:"applicationId,string"`
	CredentialID         int64     `json:"credentialId,string"`
	CredentialVersion    int       `json:"credentialVersion"`
	APIKey               string    `json:"apiKey"`
	CredentialHint       string    `json:"credentialHint"`
	PreviousCredentialID int64     `json:"previousCredentialId,string"`
	PreviousValidUntil   time.Time `json:"previousValidUntil"`
	CreatedAt            time.Time `json:"createdAt"`
}

type SigningSecretRotation struct {
	ApplicationID            int64     `json:"applicationId,string"`
	SigningCredentialID      int64     `json:"signingCredentialId,string"`
	SigningCredentialVersion int       `json:"signingCredentialVersion"`
	SigningSecret            string    `json:"signingSecret"`
	SigningSecretHint        string    `json:"signingSecretHint"`
	PreviousCredentialID     int64     `json:"previousCredentialId,string"`
	PreviousValidUntil       time.Time `json:"previousValidUntil"`
	CreatedAt                time.Time `json:"createdAt"`
}

const (
	apiKeyRotationGrace        = 15 * time.Minute
	signingSecretRotationGrace = 24 * time.Hour
)

func (s *Service) CreateApplication(ctx context.Context, tenantID int64, creatorUserID int64, request CreateApplicationRequest) (CreatedApplication, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.WebhookURL = strings.TrimSpace(request.WebhookURL)
	if creatorUserID <= 0 || utf8.RuneCountInString(request.Name) < 1 || utf8.RuneCountInString(request.Name) > 120 || utf8.RuneCountInString(request.WebhookURL) > 2048 {
		return CreatedApplication{}, domain.ErrInvalidRequest
	}
	if request.WebhookURL != "" {
		if s.urlValidator == nil || s.secrets == nil {
			return CreatedApplication{}, errors.New("open integration webhook security services are unavailable")
		}
		if err := s.urlValidator.Validate(ctx, request.WebhookURL); err != nil {
			return CreatedApplication{}, domain.ErrInvalidRequest
		}
	}
	credentialID, apiKey, credentialDigest, credentialHint, err := domain.GenerateAPIKey()
	if err != nil {
		return CreatedApplication{}, err
	}
	var signingSecret string
	var signing *openinfra.CreateSigningCredential
	if request.WebhookURL != "" {
		var signingHint string
		signingSecret, signingHint, err = domain.GenerateSigningSecret()
		if err != nil {
			return CreatedApplication{}, err
		}
		encrypted, encryptErr := s.secrets.EncryptString(ctx, signingSecret)
		if encryptErr != nil {
			return CreatedApplication{}, encryptErr
		}
		signing = &openinfra.CreateSigningCredential{Hint: signingHint, Secret: encrypted}
	}
	application, err := s.store.CreateApplication(ctx, openinfra.CreateApplicationParams{
		TenantID: tenantID, Name: request.Name, WebhookURL: request.WebhookURL,
		CredentialID: credentialID, CredentialDigest: credentialDigest, CredentialHint: credentialHint,
		SigningCredential: signing, CreatedByUserID: creatorUserID,
	})
	if err != nil {
		return CreatedApplication{}, err
	}
	created := CreatedApplication{
		Application: application, APIKey: apiKey,
		APICredentialID: application.APICredential.ID, APICredentialVersion: application.APICredential.Version,
	}
	if application.SigningCredential != nil {
		credentialID, version := application.SigningCredential.ID, application.SigningCredential.Version
		created.SigningSecret = signingSecret
		created.SigningCredentialID = &credentialID
		created.SigningCredentialVersion = &version
	}
	return created, nil
}

func (s *Service) GetOpenCapabilities(_ context.Context, identity domain.Identity) domain.OpenCapabilitiesView {
	return domain.OpenCapabilitiesView{
		Schema:        "seven.workforce.open-capabilities/v2",
		ApplicationID: identity.ApplicationID,
		TaskSubmission: domain.OpenTaskSubmissionCapability{
			SupportsSelectionDiscovery: s.models != nil && s.projects != nil && s.runtimes != nil, RequiresProjectID: true, RequiresModelSelection: true, RequiresRuntimeReleaseID: true, SupportsReferenceAssets: true,
			SupportsAdditionalCompletionCheck: true, SupportsReviewToggle: true,
			SupportsDeliveryPreference: true, SupportsOutputs: true,
		},
	}
}

func (s *Service) RotateAPIKey(ctx context.Context, tenantID, applicationID, actorUserID int64) (APIKeyRotation, error) {
	if tenantID <= 0 || applicationID <= 0 || actorUserID <= 0 || s.store == nil {
		return APIKeyRotation{}, domain.ErrInvalidRequest
	}
	locator, apiKey, digest, hint, err := domain.GenerateAPIKey()
	if err != nil {
		return APIKeyRotation{}, err
	}
	record, err := s.store.RotateAPIKey(ctx, openinfra.RotateAPIKeyParams{
		TenantID: tenantID, ApplicationID: applicationID, ActorUserID: actorUserID,
		CredentialLocator: locator, CredentialDigest: digest, CredentialHint: hint,
		GracePeriod: apiKeyRotationGrace,
	})
	if err != nil {
		return APIKeyRotation{}, err
	}
	return APIKeyRotation{
		ApplicationID: record.ApplicationID, CredentialID: record.CredentialID,
		CredentialVersion: record.Version, APIKey: apiKey, CredentialHint: record.Hint,
		PreviousCredentialID: record.PreviousCredentialID,
		PreviousValidUntil:   record.PreviousValidUntil, CreatedAt: record.CreatedAt,
	}, nil
}

func (s *Service) RotateSigningSecret(ctx context.Context, tenantID, applicationID, actorUserID int64) (SigningSecretRotation, error) {
	if tenantID <= 0 || applicationID <= 0 || actorUserID <= 0 || s.store == nil {
		return SigningSecretRotation{}, domain.ErrInvalidRequest
	}
	if err := s.store.EnsureWebhookConfigured(ctx, tenantID, applicationID); err != nil {
		return SigningSecretRotation{}, err
	}
	if s.secrets == nil {
		return SigningSecretRotation{}, errors.New("open integration secret service is unavailable")
	}
	signingSecret, hint, err := domain.GenerateSigningSecret()
	if err != nil {
		return SigningSecretRotation{}, err
	}
	encrypted, err := s.secrets.EncryptString(ctx, signingSecret)
	if err != nil {
		return SigningSecretRotation{}, err
	}
	record, err := s.store.RotateSigningSecret(ctx, openinfra.RotateSigningSecretParams{
		TenantID: tenantID, ApplicationID: applicationID, ActorUserID: actorUserID,
		SigningSecretHint: hint, GracePeriod: signingSecretRotationGrace,
	}, encrypted)
	if err != nil {
		return SigningSecretRotation{}, err
	}
	return SigningSecretRotation{
		ApplicationID: record.ApplicationID, SigningCredentialID: record.CredentialID,
		SigningCredentialVersion: record.Version, SigningSecret: signingSecret,
		SigningSecretHint: record.Hint, PreviousCredentialID: record.PreviousCredentialID,
		PreviousValidUntil: record.PreviousValidUntil, CreatedAt: record.CreatedAt,
	}, nil
}

func (s *Service) ListApplications(ctx context.Context, tenantID int64) ([]domain.Application, error) {
	return s.store.ListApplications(ctx, tenantID, 50)
}

func (s *Service) SetApplicationStatus(ctx context.Context, tenantID, applicationID int64, status string) error {
	if status != domain.ApplicationEnabled && status != domain.ApplicationDisabled {
		return domain.ErrInvalidRequest
	}
	return s.store.SetApplicationStatus(ctx, tenantID, applicationID, status)
}

func (s *Service) Authenticate(ctx context.Context, authorization string) (domain.Identity, error) {
	token := openinfra.BearerToken(authorization)
	if token == "" {
		return domain.Identity{}, openinfra.ErrUnauthorized
	}
	return s.store.Authenticate(ctx, token)
}

func (s *Service) SubmitOpenTask(ctx context.Context, identity domain.Identity, clientIdempotencyKey string, request domain.CreateOpenTaskRequest) (domain.OpenTaskView, error) {
	clientIdempotencyKey = strings.TrimSpace(clientIdempotencyKey)
	if clientIdempotencyKey == "" || utf8.RuneCountInString(clientIdempotencyKey) > 128 {
		return domain.OpenTaskView{}, ErrIdempotencyKeyRequired
	}
	normalized, err := request.NormalizeAndValidate()
	if err != nil {
		return domain.OpenTaskView{}, err
	}
	if s.tasks == nil {
		return domain.OpenTaskView{}, errors.New("project task submission is unavailable")
	}
	internalKey := domain.InternalIdempotencyKey(identity.ApplicationID, clientIdempotencyKey)
	task, replayed, err := s.tasks.CreateOpenProjectTask(ctx, taskfacade.OpenProjectTaskCommand{
		TenantID: identity.TenantID, ApplicationID: identity.ApplicationID, CreatorUserID: identity.CreatedByUserID, ProjectID: normalized.ProjectID,
		IdempotencyKey: internalKey, ExternalCorrelationID: normalized.ExternalCorrelationID, Request: normalized.ProjectTaskRequest(),
	})
	if err != nil {
		return domain.OpenTaskView{}, err
	}
	status, _, _ := domain.PublicStatus(string(task.Status))
	return domain.OpenTaskView{
		Schema: "seven.workforce.open-task/v2", TaskID: task.ID, TaskTurnID: task.TaskTurnID, ProjectID: normalized.ProjectID,
		Status: status, Replayed: &replayed,
		ExternalCorrelationID: normalized.ExternalCorrelationID,
		Links:                 domain.TaskLinks{Self: "/api/open/workforce/v1/tasks/" + strconv.FormatInt(task.ID, 10), Operator: "/workforce/tasks/" + strconv.FormatInt(task.ID, 10)},
	}, nil
}

func (s *Service) GetOpenTask(ctx context.Context, identity domain.Identity, taskID int64) (domain.OpenTaskView, error) {
	view, err := s.store.GetOpenTask(ctx, identity, taskID)
	if err != nil {
		return domain.OpenTaskView{}, err
	}
	view.Links.Operator = "/workforce/tasks/" + strconv.FormatInt(view.TaskID, 10)
	return view, nil
}

func (s *Service) ListOpenTaskOutputs(ctx context.Context, identity domain.Identity, taskID int64) (domain.OpenTaskOutputsView, error) {
	if err := s.ensureOutputTaskOwned(ctx, identity, taskID); err != nil {
		return domain.OpenTaskOutputsView{}, err
	}
	if s.outputs == nil {
		return domain.OpenTaskOutputsView{}, errors.New("result output access is unavailable")
	}
	items, err := s.outputs.ListAvailableOutputs(ctx, identity.TenantID, taskID)
	if err != nil {
		return domain.OpenTaskOutputsView{}, err
	}
	view := domain.OpenTaskOutputsView{
		Schema:  "seven.workforce.open-task-outputs/v1",
		TaskID:  taskID,
		Outputs: make([]domain.OpenTaskOutput, 0, len(items)),
	}
	for _, item := range items {
		if item.ID <= 0 || item.TaskID != taskID {
			return domain.OpenTaskOutputsView{}, resultfacade.ErrOutputNotFound
		}
		view.Outputs = append(view.Outputs, openTaskOutput(item, taskID))
	}
	return view, nil
}

func (s *Service) OpenTaskOutput(ctx context.Context, identity domain.Identity, taskID, outputID int64) (resultfacade.OutputDownload, error) {
	if err := s.ensureOutputTaskOwned(ctx, identity, taskID); err != nil {
		return resultfacade.OutputDownload{}, err
	}
	if s.outputs == nil {
		return resultfacade.OutputDownload{}, errors.New("result output access is unavailable")
	}
	download, err := s.outputs.OpenAvailableOutput(ctx, identity.TenantID, taskID, outputID)
	if err != nil {
		return resultfacade.OutputDownload{}, err
	}
	if download.Content == nil || download.Output.ID != outputID || download.Output.TaskID != taskID {
		if download.Content != nil {
			_ = download.Content.Close()
		}
		return resultfacade.OutputDownload{}, resultfacade.ErrOutputNotFound
	}
	return download, nil
}

func (s *Service) ensureOutputTaskOwned(ctx context.Context, identity domain.Identity, taskID int64) error {
	if s.store == nil {
		return errors.New("open integration store is unavailable")
	}
	_, err := s.store.GetOpenTask(ctx, identity, taskID)
	if errors.Is(err, openinfra.ErrNotFound) {
		return resultfacade.ErrOutputNotFound
	}
	return err
}

func openTaskOutput(item resultfacade.AvailableOutput, taskID int64) domain.OpenTaskOutput {
	taskPath := "/api/open/workforce/v1/tasks/" + strconv.FormatInt(taskID, 10)
	return domain.OpenTaskOutput{
		OutputID: item.ID, Name: item.Name, OutputKind: item.OutputKind, Status: item.Status,
		MediaType: item.MediaType, SHA256: item.SHA256, SizeBytes: item.SizeBytes,
		Links: domain.OutputLinks{Download: taskPath + "/outputs/" + strconv.FormatInt(item.ID, 10) + "/download"},
	}
}

func (s *Service) ListDeliveries(ctx context.Context, tenantID, applicationID int64, limit int) ([]domain.Delivery, error) {
	return s.store.ListDeliveries(ctx, tenantID, applicationID, limit)
}

var ErrIdempotencyKeyRequired = errors.New("idempotency key is required")

func StableError(err error) (status int, code, message string) {
	switch {
	case errors.Is(err, ErrIdempotencyKeyRequired):
		return 400, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key is required and must be at most 128 characters."
	case errors.Is(err, openinfra.ErrUnauthorized):
		return 401, "UNAUTHORIZED", "The API key is invalid."
	case errors.Is(err, openinfra.ErrApplicationDisabled):
		return 401, "APPLICATION_DISABLED", "The integration application is disabled."
	case errors.Is(err, projectdomain.ErrNotFound):
		return 404, "PROJECT_NOT_FOUND", "The project was not found."
	case errors.Is(err, projectdomain.ErrArchived):
		return 409, "PROJECT_ARCHIVED", "The project no longer accepts new tasks."
	case errors.Is(err, projectdomain.ErrSourceUnavailable):
		return 409, "PROJECT_SOURCE_NOT_READY", "The project source is not ready."
	case errors.Is(err, projectdomain.ErrInvalid):
		return 409, "PROJECT_CONFIGURATION_INVALID", "The project configuration must be updated before creating a task."
	case errors.Is(err, openinfra.ErrNotFound):
		return 404, "TASK_NOT_FOUND", "The task was not found."
	case errors.Is(err, resultfacade.ErrOutputNotFound):
		return 404, "OUTPUT_NOT_FOUND", "The output was not found."
	case errors.Is(err, resultfacade.ErrOutputUnavailable):
		return 503, "OUTPUT_UNAVAILABLE", "The output is temporarily unavailable."
	case errors.Is(err, taskfacade.ErrIdempotencyConflict), errors.Is(err, openinfra.ErrIdempotencyConflict):
		return 409, "IDEMPOTENCY_CONFLICT", "The idempotency key was already used with a different request."
	case errors.Is(err, ErrSelectionUnavailable):
		return 503, "SELECTION_UNAVAILABLE", "Task selection queries are temporarily unavailable."
	case errors.Is(err, routingfacade.ErrRuntimeSelectionRequired):
		return 400, "TASK_RUNTIME_SELECTION_REQUIRED", "A RuntimeRelease selection is required."
	case errors.Is(err, routingfacade.ErrRuntimeSelectionUnavailable):
		return 409, "TASK_RUNTIME_SELECTION_UNAVAILABLE", "The selected model credential or RuntimeRelease is unavailable."
	case errors.Is(err, routingfacade.ErrRuntimeSelectionIncompatible):
		return 409, "TASK_RUNTIME_SELECTION_INCOMPATIBLE", "The selected RuntimeRelease is incompatible with the task."
	case errors.Is(err, taskfacade.ErrInvalidTaskSubmission), errors.Is(err, providerfacade.ErrInvalidTaskModelSelection), errors.Is(err, domain.ErrInvalidRequest):
		return 400, "INVALID_REQUEST", "The request is invalid."
	default:
		return 500, "INTERNAL_ERROR", "The request could not be completed."
	}
}
