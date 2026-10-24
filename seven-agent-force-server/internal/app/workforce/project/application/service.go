package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	completioncheck "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/domain"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
	sharedtransport "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/transport"
)

type UpsertRequest struct {
	ExpectedVersionID          int64                      `json:"expectedVersionId,string,omitempty"`
	ProjectKey                 string                     `json:"projectKey"`
	Name                       string                     `json:"name"`
	Description                string                     `json:"description,omitempty"`
	DepartmentID               int64                      `json:"departmentId,string,omitempty"`
	Kind                       string                     `json:"kind"`
	Repository                 *RepositorySelection       `json:"repository,omitempty"`
	TechStack                  []string                   `json:"techStack,omitempty"`
	AgentGuidance              string                     `json:"agentGuidance,omitempty"`
	CompletionCheck            *completioncheck.Input     `json:"completionCheck,omitempty"`
	ReviewPrompt               string                     `json:"reviewPrompt,omitempty"`
	HookReleaseIDs             sharedtransport.Int64Slice `json:"hookReleaseIds,omitempty"`
	Delivery                   DeliveryPolicy             `json:"delivery"`
	Notification               NotificationPolicy         `json:"notification"`
	ResolvedProcessorReleaseID int64                      `json:"-"`
	DeliveryApproveAuthorized  bool                       `json:"-"`
	ResolvedRepository         *ResolvedRepository        `json:"-"`
	IdempotencyKey             string                     `json:"-"`
	CreateRequestDigest        string                     `json:"-"`
}

type RepositorySelection struct {
	CloneURL       string `json:"cloneUrl,omitempty"`
	DefaultRefName string `json:"defaultRefName"`
}

type ResolvedRepository struct {
	ProviderType         string
	Scheme               string
	Host                 string
	Port                 int
	Path                 string
	DisplayURL           string
	WebURL               string
	DefaultRefName       string
	CredentialCiphertext string
	CredentialEDEK       string
	CredentialWrapKeyRef string
	CredentialVersion    int64
	ConfigDigest         string
	Capabilities         domain.RepositoryCapabilities
	LastCheckStatus      string
	LastCheckReasonCode  string
	SourceState          string
	SourceRevisionID     int64
}

type DeliveryPolicy struct {
	Mode          string `json:"mode"`
	Authorization string `json:"authorization,omitempty"`
	BranchPattern string `json:"branchPattern,omitempty"`
}

type NotificationPolicy struct {
	Enabled      bool     `json:"enabled"`
	Events       []string `json:"events,omitempty"`
	GroupRouteID int64    `json:"groupRouteId,string,omitempty"`
}

type ListRequest struct {
	Query  string
	Status string
	Cursor int64
	Limit  int
}

type ProjectPage struct {
	Items      []domain.Project
	NextCursor int64
}

type SourcePreparer interface {
	PrepareProjectSource(context.Context, int64, int64, int64, int64) error
}

type SourcePreparerFunc func(context.Context, int64, int64, int64, int64) error

func (f SourcePreparerFunc) PrepareProjectSource(ctx context.Context, tenantID, actorID, projectID, configVersion int64) error {
	return f(ctx, tenantID, actorID, projectID, configVersion)
}

type RepositoryVerification struct {
	ProviderType   string
	DefaultRefName string
	WebURL         string
	Capabilities   domain.RepositoryCapabilities
}

type RepositoryVerifier interface {
	Verify(context.Context, domain.ParsedRepositoryURL, string) (RepositoryVerification, error)
}

type CredentialEnvelope struct {
	Ciphertext string
	EDEK       string
	WrapKeyRef string
}

type CredentialCipher interface {
	EncryptRepositoryCredential(context.Context, domain.RepositoryCredential) (CredentialEnvelope, error)
	DecryptRepositoryCredential(context.Context, string, string, string) (domain.RepositoryCredential, error)
}

type Repository interface {
	Create(context.Context, int64, int64, UpsertRequest) (domain.Project, error)
	Update(context.Context, int64, int64, int64, UpsertRequest) (domain.Project, error)
	Get(context.Context, int64, int64) (domain.Project, error)
	List(context.Context, int64, ListRequest) (ProjectPage, error)
	Archive(context.Context, int64, int64, int64, int64) (domain.Project, error)
	LatestAvailableSource(context.Context, int64, int64, int64) (int64, error)
	ResolveProcessorRelease(context.Context, int64, int64) (int64, error)
	ListPreparingSources(context.Context, int) ([]PreparingSource, error)
	MarkSourcePreparationFailed(context.Context, int64, int64, int64) error
}

type PreparingSource struct{ TenantID, ActorID, ProjectID, VersionID int64 }

type ReferenceValidator interface {
	ValidateProjectReferences(context.Context, int64, UpsertRequest) error
}

type Service struct {
	repository Repository
	references ReferenceValidator
	preparer   SourcePreparer
	verifier   RepositoryVerifier
	cipher     CredentialCipher
}

func New(repository Repository) *Service                               { return &Service{repository: repository} }
func (s *Service) BindReferenceValidator(validator ReferenceValidator) { s.references = validator }
func (s *Service) BindSourcePreparer(preparer SourcePreparer)          { s.preparer = preparer }
func (s *Service) BindRepositoryVerifier(verifier RepositoryVerifier)  { s.verifier = verifier }
func (s *Service) BindCredentialCipher(cipher CredentialCipher)        { s.cipher = cipher }

func (s *Service) Create(ctx context.Context, tenantID, actorID int64, request UpsertRequest) (domain.Project, error) {
	normalized, err := normalize(request, false)
	if err != nil {
		return domain.Project{}, err
	}
	if err = s.resolveRepository(ctx, &normalized, nil); err != nil {
		return domain.Project{}, err
	}
	if s.references != nil {
		if err = s.references.ValidateProjectReferences(ctx, tenantID, normalized); err != nil {
			return domain.Project{}, err
		}
	}
	normalized.ResolvedProcessorReleaseID, err = s.repository.ResolveProcessorRelease(ctx, tenantID, 0)
	if err != nil {
		return domain.Project{}, err
	}
	normalized.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if normalized.IdempotencyKey == "" || len(normalized.IdempotencyKey) > 200 {
		return domain.Project{}, domain.ErrInvalid
	}
	normalized.CreateRequestDigest = createRequestDigest(normalized)
	created, err := s.repository.Create(ctx, tenantID, actorID, normalized)
	if err == nil && created.Kind == projectfacade.KindCodebase && s.preparer != nil {
		if prepareErr := s.preparer.PrepareProjectSource(context.WithoutCancel(ctx), tenantID, actorID, created.ID, created.VersionID); prepareErr != nil {
			_ = s.repository.MarkSourcePreparationFailed(context.WithoutCancel(ctx), tenantID, created.ID, created.VersionID)
			return s.repository.Get(ctx, tenantID, created.ID)
		}
	}
	return created, err
}

func (s *Service) Update(ctx context.Context, tenantID, actorID, projectID int64, request UpsertRequest) (domain.Project, error) {
	normalized, err := normalize(request, true)
	if err != nil {
		return domain.Project{}, err
	}
	current, err := s.repository.Get(ctx, tenantID, projectID)
	if err != nil {
		return domain.Project{}, err
	}
	if current.Status != projectfacade.StatusActive {
		return domain.Project{}, domain.ErrArchived
	}
	if err = s.resolveRepository(ctx, &normalized, &current); err != nil {
		return domain.Project{}, err
	}
	if s.references != nil {
		if err = s.references.ValidateProjectReferences(ctx, tenantID, normalized); err != nil {
			return domain.Project{}, err
		}
	}
	// A metadata or business-rule edit must not silently adopt a newly changed
	// platform default. Keep the immutable rules already bound to this Project;
	// defaults are resolved only when the Project first enables that rule.
	normalized.ResolvedProcessorReleaseID, err = s.repository.ResolveProcessorRelease(ctx, tenantID, current.ResultProcessorReleaseID)
	if err != nil {
		return domain.Project{}, err
	}
	updated, err := s.repository.Update(ctx, tenantID, actorID, projectID, normalized)
	if err == nil && updated.Kind == projectfacade.KindCodebase && updated.RepositorySourceState == domain.RepositorySourcePreparing && s.preparer != nil {
		if prepareErr := s.preparer.PrepareProjectSource(context.WithoutCancel(ctx), tenantID, actorID, updated.ID, updated.VersionID); prepareErr != nil {
			_ = s.repository.MarkSourcePreparationFailed(context.WithoutCancel(ctx), tenantID, updated.ID, updated.VersionID)
			return s.repository.Get(ctx, tenantID, updated.ID)
		}
	}
	return updated, err
}

// ReconcileSourcePreparations treats PREPARING as a durable work signal and
// closes the process-crash window between saving a Project and enqueuing its
// source ingestion run.
func (s *Service) ReconcileSourcePreparations(ctx context.Context, limit int) error {
	if s.preparer == nil {
		return nil
	}
	items, err := s.repository.ListPreparingSources(ctx, limit)
	if err != nil {
		return err
	}
	for _, item := range items {
		if err = s.preparer.PrepareProjectSource(ctx, item.TenantID, item.ActorID, item.ProjectID, item.VersionID); err != nil {
			_ = s.repository.MarkSourcePreparationFailed(ctx, item.TenantID, item.ProjectID, item.VersionID)
		}
	}
	return nil
}

func (s *Service) RetryRepository(ctx context.Context, tenantID, actorID, projectID int64) (domain.Project, error) {
	item, err := s.repository.Get(ctx, tenantID, projectID)
	if err != nil || item.Kind != projectfacade.KindCodebase || item.Status != projectfacade.StatusActive {
		return domain.Project{}, err
	}
	if s.preparer == nil {
		return domain.Project{}, domain.ErrSourceUnavailable
	}
	if err = s.preparer.PrepareProjectSource(ctx, tenantID, actorID, item.ID, item.VersionID); err != nil {
		return domain.Project{}, err
	}
	return s.repository.Get(ctx, tenantID, projectID)
}

func (s *Service) resolveRepository(ctx context.Context, request *UpsertRequest, current *domain.Project) error {
	if request.Kind != projectfacade.KindCodebase {
		request.ResolvedRepository = nil
		return nil
	}
	if request.Repository == nil {
		return domain.ErrInvalid
	}
	if request.Repository.CloneURL == "" {
		if current == nil || current.RepositoryDisplayURL == "" {
			return domain.ErrRepositoryURLInvalid
		}
		defaultRef := strings.TrimSpace(request.Repository.DefaultRefName)
		if defaultRef == "" {
			defaultRef = current.DefaultRefName
		}
		request.ResolvedRepository = &ResolvedRepository{
			ProviderType: current.RepositoryProviderType, Scheme: current.RepositoryScheme, Host: current.RepositoryHost, Port: current.RepositoryPort,
			Path: current.RepositoryPath, DisplayURL: current.RepositoryDisplayURL, WebURL: current.RepositoryWebURL, DefaultRefName: defaultRef,
			CredentialCiphertext: current.RepositoryCredentialCiphertext, CredentialEDEK: current.RepositoryCredentialEDEK, CredentialWrapKeyRef: current.RepositoryCredentialWrapKeyRef,
			CredentialVersion: current.RepositoryCredentialVersion, ConfigDigest: current.RepositoryConfigDigest, Capabilities: current.RepositoryCapabilities,
			LastCheckStatus: current.RepositoryLastCheckStatus, LastCheckReasonCode: current.RepositoryLastCheckReasonCode,
			SourceState: current.RepositorySourceState, SourceRevisionID: current.RepositorySourceRevisionID,
		}
		if defaultRef != current.DefaultRefName {
			if s.verifier == nil || s.cipher == nil {
				return domain.ErrRepositoryPreparationFailed
			}
			credential := domain.RepositoryCredential{}
			if current.RepositoryCredentialVersion > 0 {
				var decryptErr error
				credential, decryptErr = s.cipher.DecryptRepositoryCredential(ctx, current.RepositoryCredentialCiphertext, current.RepositoryCredentialEDEK, current.RepositoryCredentialWrapKeyRef)
				if decryptErr != nil {
					return domain.ErrRepositoryPreparationFailed
				}
			}
			verified, verifyErr := s.verifier.Verify(ctx, domain.ParsedRepositoryURL{Scheme: current.RepositoryScheme, Host: current.RepositoryHost, Port: current.RepositoryPort, RepositoryPath: current.RepositoryPath, DisplayURL: current.RepositoryDisplayURL, ProviderType: current.RepositoryProviderType, Credential: credential, ConfigDigest: current.RepositoryConfigDigest}, defaultRef)
			credential.Username, credential.Token = "", ""
			if verifyErr != nil {
				return verifyErr
			}
			request.ResolvedRepository.DefaultRefName = verified.DefaultRefName
			request.ResolvedRepository.ProviderType = verified.ProviderType
			request.ResolvedRepository.WebURL = verified.WebURL
			request.ResolvedRepository.Capabilities = verified.Capabilities
			request.ResolvedRepository.LastCheckStatus = "SUCCEEDED"
			request.ResolvedRepository.LastCheckReasonCode = ""
			request.ResolvedRepository.SourceState = domain.RepositorySourcePreparing
			request.ResolvedRepository.SourceRevisionID = 0
		}
		return validateDeliveryRepository(request.Delivery, request.ResolvedRepository)
	}
	parsed, err := domain.ParseRepositoryURL(request.Repository.CloneURL)
	request.Repository.CloneURL = ""
	if err != nil {
		return err
	}
	if request.Delivery.Mode != "NONE" && (parsed.Credential.Username == "" || parsed.Credential.Token == "") {
		return domain.ErrRepositoryCredentialRequired
	}
	if s.verifier == nil || s.cipher == nil {
		return domain.ErrRepositoryPreparationFailed
	}
	verified, err := s.verifier.Verify(ctx, parsed, request.Repository.DefaultRefName)
	if err != nil {
		return err
	}
	if request.Delivery.Mode != "NONE" && !verified.Capabilities.Push {
		return domain.ErrRepositoryPushUnavailable
	}
	if request.Delivery.Mode == "NEW_BRANCH_AND_DRAFT_REVIEW" && !verified.Capabilities.DraftReview {
		return domain.ErrRepositoryReviewUnavailable
	}
	envelope := CredentialEnvelope{}
	credentialVersion := int64(0)
	if parsed.Credential.Username != "" {
		envelope, err = s.cipher.EncryptRepositoryCredential(ctx, parsed.Credential)
		if err != nil {
			return domain.ErrRepositoryPreparationFailed
		}
		credentialVersion = 1
		if current != nil {
			credentialVersion = current.RepositoryCredentialVersion + 1
		}
	}
	request.Repository.DefaultRefName = verified.DefaultRefName
	request.ResolvedRepository = &ResolvedRepository{
		ProviderType: verified.ProviderType, Scheme: parsed.Scheme, Host: parsed.Host, Port: parsed.Port, Path: parsed.RepositoryPath,
		DisplayURL: parsed.DisplayURL, WebURL: verified.WebURL, DefaultRefName: verified.DefaultRefName,
		CredentialCiphertext: envelope.Ciphertext, CredentialEDEK: envelope.EDEK, CredentialWrapKeyRef: envelope.WrapKeyRef,
		CredentialVersion: credentialVersion, ConfigDigest: parsed.ConfigDigest, Capabilities: verified.Capabilities, LastCheckStatus: "SUCCEEDED",
		SourceState: domain.RepositorySourcePreparing,
	}
	return validateDeliveryRepository(request.Delivery, request.ResolvedRepository)
}

func validateDeliveryRepository(delivery DeliveryPolicy, repository *ResolvedRepository) error {
	if delivery.Mode == "NONE" {
		return nil
	}
	if repository == nil || repository.CredentialVersion < 1 || repository.CredentialCiphertext == "" {
		return domain.ErrRepositoryCredentialRequired
	}
	if !repository.Capabilities.Push {
		return domain.ErrRepositoryPushUnavailable
	}
	if delivery.Mode == "NEW_BRANCH_AND_DRAFT_REVIEW" && !repository.Capabilities.DraftReview {
		return domain.ErrRepositoryReviewUnavailable
	}
	return nil
}

func createRequestDigest(request UpsertRequest) string {
	type repositoryDigest struct {
		ConfigDigest, DefaultRefName string
	}
	value := struct {
		ProjectKey, Name, Description, Kind, AgentGuidance string
		DepartmentID                                       int64
		Repository                                         repositoryDigest
		TechStack                                          []string
		CompletionCheck                                    *completioncheck.Input
		ReviewPrompt                                       string
		HookReleaseIDs                                     []int64
		Delivery                                           DeliveryPolicy
		Notification                                       NotificationPolicy
	}{request.ProjectKey, request.Name, request.Description, request.Kind, request.AgentGuidance, request.DepartmentID,
		repositoryDigest{}, request.TechStack, request.CompletionCheck, request.ReviewPrompt, request.HookReleaseIDs, request.Delivery, request.Notification}
	if request.ResolvedRepository != nil {
		value.Repository = repositoryDigest{request.ResolvedRepository.ConfigDigest, request.ResolvedRepository.DefaultRefName}
	}
	encoded, _ := json.Marshal(value)
	return digest(encoded)
}

func (s *Service) Get(ctx context.Context, tenantID, projectID int64) (domain.Project, error) {
	return s.repository.Get(ctx, tenantID, projectID)
}

func (s *Service) List(ctx context.Context, tenantID int64, request ListRequest) (ProjectPage, error) {
	request.Query = strings.ToLower(strings.TrimSpace(request.Query))
	request.Status = strings.ToUpper(strings.TrimSpace(request.Status))
	if tenantID < 1 || request.Cursor < 0 || utf8.RuneCountInString(request.Query) > 200 {
		return ProjectPage{}, domain.ErrInvalid
	}
	if request.Status != "" && request.Status != "ACTIVE" && request.Status != "ARCHIVED" {
		return ProjectPage{}, domain.ErrInvalid
	}
	if request.Limit < 1 || request.Limit > 50 {
		request.Limit = 20
	}
	return s.repository.List(ctx, tenantID, request)
}

func (s *Service) Archive(ctx context.Context, tenantID, actorID, projectID, expectedVersionID int64) (domain.Project, error) {
	if expectedVersionID < 1 {
		return domain.Project{}, domain.ErrInvalid
	}
	return s.repository.Archive(ctx, tenantID, actorID, projectID, expectedVersionID)
}

func (s *Service) CompileExecutionRequirements(ctx context.Context, tenantID, projectID int64) (projectfacade.CompiledExecutionRequirements, error) {
	project, err := s.repository.Get(ctx, tenantID, projectID)
	if err != nil {
		return projectfacade.CompiledExecutionRequirements{}, err
	}
	if project.Status != projectfacade.StatusActive {
		return projectfacade.CompiledExecutionRequirements{}, domain.ErrArchived
	}
	return projectfacade.CompiledExecutionRequirements{ProjectVersionID: project.VersionID, Requirements: compileRequirements(project, projectfacade.FrozenGitPlan{DeliveryMode: project.DeliveryMode})}, nil
}

func (s *Service) CompileForTask(ctx context.Context, tenantID, projectID int64, additions projectfacade.TaskAdditions) (projectfacade.FrozenTaskSpec, error) {
	project, err := s.repository.Get(ctx, tenantID, projectID)
	if err != nil {
		return projectfacade.FrozenTaskSpec{}, err
	}
	if project.Status != projectfacade.StatusActive {
		return projectfacade.FrozenTaskSpec{}, domain.ErrArchived
	}
	additions.Instruction = strings.TrimSpace(additions.Instruction)
	additions.Name = strings.TrimSpace(additions.Name)
	if additions.Instruction == "" || utf8.RuneCountInString(additions.Instruction) > 20000 {
		return projectfacade.FrozenTaskSpec{}, domain.ErrInvalid
	}
	if additions.DeliveryPreference == "" {
		additions.DeliveryPreference = projectfacade.DeliveryProjectDefault
	}
	if additions.DeliveryPreference != projectfacade.DeliveryProjectDefault && additions.DeliveryPreference != projectfacade.DeliveryAskAfterGates && additions.DeliveryPreference != projectfacade.DeliverySaveOnly {
		return projectfacade.FrozenTaskSpec{}, domain.ErrInvalid
	}
	completion, err := mergeCompletionCheck(project.CompletionCheck, additions.AdditionalCompletionCheck)
	if err != nil {
		return projectfacade.FrozenTaskSpec{}, err
	}
	sourceRevisionID := int64(0)
	if project.Kind == projectfacade.KindCodebase {
		if project.RepositorySourceState != domain.RepositorySourceReady {
			return projectfacade.FrozenTaskSpec{}, domain.ErrSourceUnavailable
		}
		sourceRevisionID, err = s.repository.LatestAvailableSource(ctx, tenantID, project.ID, project.VersionID)
		if err != nil {
			return projectfacade.FrozenTaskSpec{}, err
		}
	}
	processorReleaseID, err := s.repository.ResolveProcessorRelease(ctx, tenantID, project.ResultProcessorReleaseID)
	if err != nil {
		return projectfacade.FrozenTaskSpec{}, err
	}
	gitPlan := projectfacade.FrozenGitPlan{ProjectID: project.ID, ProjectVersionID: project.VersionID, RepositoryConfigDigest: project.RepositoryConfigDigest, RepositoryCredentialVersion: project.RepositoryCredentialVersion, RepositoryDisplayURL: project.RepositoryDisplayURL, RepositoryProviderType: project.RepositoryProviderType, RepositoryScheme: project.RepositoryScheme, RepositoryHost: project.RepositoryHost, RepositoryPort: project.RepositoryPort, RepositoryPath: project.RepositoryPath, RepositoryWebURL: project.RepositoryWebURL, RepositoryCredentialCiphertext: project.RepositoryCredentialCiphertext, RepositoryCredentialEDEK: project.RepositoryCredentialEDEK, RepositoryCredentialWrapKeyRef: project.RepositoryCredentialWrapKeyRef, SourceRevisionID: sourceRevisionID, DefaultRefName: project.DefaultRefName, ReviewBaseBranch: project.DefaultRefName, ResultBranchPattern: project.ResultBranchPattern, DeliveryMode: project.DeliveryMode, DeliveryAuthorization: project.DeliveryAuthorization, DeliveryAuthorizedByPrincipalID: project.DeliveryAuthorizedByPrincipalID}
	if additions.DeliveryPreference == projectfacade.DeliveryAskAfterGates && gitPlan.DeliveryMode != "NONE" {
		gitPlan.DeliveryAuthorization = "ASK_AFTER_GATES"
		gitPlan.DeliveryAuthorizedByPrincipalID = 0
	}
	if additions.DeliveryPreference == projectfacade.DeliverySaveOnly {
		gitPlan.DeliveryMode, gitPlan.DeliveryAuthorization = "NONE", ""
		gitPlan.DeliveryAuthorizedByPrincipalID = 0
	}
	requirements := compileRequirements(project, gitPlan)
	requirementsJSON, _ := json.Marshal(requirements)
	requirementsDigest := digest(requirementsJSON)
	return projectfacade.FrozenTaskSpec{
		ProjectID: project.ID, ProjectVersionID: project.VersionID,
		ExecutionRequirements: requirements, ExecutionRequirementsDigest: requirementsDigest, ProjectName: project.Name, ProjectKind: project.Kind,
		Instruction: additions.Instruction, Name: additions.Name, Tags: append([]string(nil), additions.Tags...), ReferenceAssetIDs: append(sharedtransport.Int64Slice(nil), additions.ReferenceAssetIDs...),
		WorkspaceSourceRevisionID: sourceRevisionID, CompletionCheck: completion, ResultProcessorReleaseID: processorReleaseID,
		ResultProcessorParameters: defaultObject(project.ResultProcessorParameters), ReviewEnabled: additions.ReviewEnabled, ReviewPrompt: project.ReviewPrompt, GitPlan: gitPlan,
		Notification: projectfacade.NotificationPolicy{Enabled: project.NotificationEnabled, Events: append([]string(nil), project.NotificationEvents...), GroupRouteID: project.NotificationGroupRouteID},
	}, nil
}

func normalize(request UpsertRequest, update bool) (UpsertRequest, error) {
	request.ProjectKey = strings.ToLower(strings.TrimSpace(request.ProjectKey))
	request.Name = strings.TrimSpace(request.Name)
	request.Description = strings.TrimSpace(request.Description)
	request.Kind = strings.ToUpper(strings.TrimSpace(request.Kind))
	request.AgentGuidance = strings.TrimSpace(request.AgentGuidance)
	request.ReviewPrompt = strings.TrimSpace(request.ReviewPrompt)
	if len(request.HookReleaseIDs) > 64 {
		return request, domain.ErrInvalid
	}
	seenHookReleases := make(map[int64]struct{}, len(request.HookReleaseIDs))
	for _, releaseID := range request.HookReleaseIDs {
		if releaseID < 1 {
			return request, domain.ErrInvalid
		}
		if _, exists := seenHookReleases[releaseID]; exists {
			return request, domain.ErrInvalid
		}
		seenHookReleases[releaseID] = struct{}{}
	}
	request.Delivery.Mode = strings.ToUpper(strings.TrimSpace(request.Delivery.Mode))
	request.Delivery.Authorization = strings.ToUpper(strings.TrimSpace(request.Delivery.Authorization))
	request.Delivery.BranchPattern = strings.TrimSpace(request.Delivery.BranchPattern)
	if request.Kind == "" {
		request.Kind = projectfacade.KindCodebase
	}
	if request.Delivery.Mode == "" {
		request.Delivery.Mode = "NONE"
	}
	if request.ProjectKey == "" || request.Name == "" || request.DepartmentID < 0 || len(request.ProjectKey) > 100 || len(request.Name) > 200 || len(request.Description) > 4000 || len(request.AgentGuidance) > 32000 || len(request.ReviewPrompt) > 16*1024 {
		return request, domain.ErrInvalid
	}
	if update && request.ExpectedVersionID < 1 {
		return request, domain.ErrInvalid
	}
	if request.Kind != projectfacade.KindCodebase && request.Kind != projectfacade.KindGeneral {
		return request, domain.ErrInvalid
	}
	var err error
	request.TechStack, err = domain.NormalizeStringList(request.TechStack)
	if err != nil {
		return request, err
	}
	request.Notification.Events, err = domain.NormalizeStringList(request.Notification.Events)
	if err != nil {
		return request, err
	}
	if request.Kind == projectfacade.KindCodebase {
		if request.Repository == nil {
			return request, domain.ErrInvalid
		}
		request.Repository.CloneURL = strings.TrimSpace(request.Repository.CloneURL)
		request.Repository.DefaultRefName = strings.TrimSpace(request.Repository.DefaultRefName)
	} else {
		if request.Repository != nil || request.Delivery.Mode != "NONE" {
			return request, domain.ErrInvalid
		}
	}
	if request.Delivery.Mode == "NONE" {
		request.Delivery.Authorization, request.Delivery.BranchPattern = "NONE", ""
	} else if request.Kind != projectfacade.KindCodebase || request.Delivery.Authorization != "AUTO_AFTER_GATES" && request.Delivery.Authorization != "ASK_AFTER_GATES" || strings.Count(request.Delivery.BranchPattern, "{task-id}") != 1 {
		return request, domain.ErrInvalid
	}
	if request.Delivery.Authorization == "AUTO_AFTER_GATES" && !request.DeliveryApproveAuthorized {
		return request, domain.ErrDeliveryApprovalRequired
	}
	if request.Notification.GroupRouteID < 0 {
		return request, domain.ErrInvalid
	}
	if !request.Notification.Enabled {
		request.Notification.Events, request.Notification.GroupRouteID = nil, 0
	}
	if request.CompletionCheck != nil {
		normalized, freezeErr := completioncheck.Normalize(*request.CompletionCheck)
		if freezeErr != nil {
			return request, domain.ErrInvalid
		}
		request.CompletionCheck = &normalized
	}
	return request, nil
}

func mergeCompletionCheck(baseJSON json.RawMessage, addition *completioncheck.Input) (*completioncheck.Input, error) {
	var base *completioncheck.Input
	if len(baseJSON) > 0 && string(baseJSON) != "null" {
		if err := json.Unmarshal(baseJSON, &base); err != nil {
			return nil, domain.ErrInvalid
		}
	}
	if base == nil && addition == nil {
		return nil, nil
	}
	if base == nil {
		normalized, err := completioncheck.Normalize(*addition)
		if err != nil {
			return nil, domain.ErrInvalid
		}
		return &normalized, nil
	}
	baseNormalized, err := completioncheck.Normalize(*base)
	if err != nil {
		return nil, domain.ErrInvalid
	}
	if addition == nil {
		return &baseNormalized, nil
	}
	extra, err := completioncheck.Normalize(*addition)
	if err != nil || extra.Mode == "NONE" && baseNormalized.Mode != "NONE" {
		return nil, domain.ErrInvalid
	}
	if baseNormalized.Mode == "NONE" {
		return &extra, nil
	}
	if extra.Mode == "NONE" {
		return nil, domain.ErrInvalid
	}
	toShell := func(input completioncheck.Input) string {
		if input.Mode == "COMMAND" {
			return input.Command
		}
		return input.Script
	}
	merged := completioncheck.Input{Mode: "SCRIPT", Script: "set -eu\n(\n" + toShell(baseNormalized) + "\n)\n(\n" + toShell(extra) + "\n)", Instructions: joinText(baseNormalized.Instructions, extra.Instructions)}
	merged.ReferenceAssetIDs = append(append([]int64(nil), baseNormalized.ReferenceAssetIDs...), extra.ReferenceAssetIDs...)
	sort.Slice(merged.ReferenceAssetIDs, func(i, j int) bool { return merged.ReferenceAssetIDs[i] < merged.ReferenceAssetIDs[j] })
	dedup := merged.ReferenceAssetIDs[:0]
	for _, id := range merged.ReferenceAssetIDs {
		if len(dedup) == 0 || dedup[len(dedup)-1] != id {
			dedup = append(dedup, id)
		}
	}
	merged.ReferenceAssetIDs = dedup
	normalized, err := completioncheck.Normalize(merged)
	if err != nil {
		return nil, domain.ErrInvalid
	}
	return &normalized, nil
}

func compileRequirements(project domain.Project, plan projectfacade.FrozenGitPlan) projectfacade.ExecutionRequirements {
	adapter := []string{"interaction.request", "interaction.respond", "user.message", "execution.suspend", "execution.resume", "tool.request", "tool.result"}
	tools := []string{"run_command_with_network_access"}
	if plan.DeliveryMode != "NONE" {
		tools = append(tools, "request_git_delivery")
	}
	return projectfacade.ExecutionRequirements{ExecutionKind: "AGENT_ATTEMPT", AdapterCapabilities: adapter, AllowedPlatformTools: tools, ResourceClass: "STANDARD", RequiresNativeResume: true}
}

func digest(value []byte) string { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }
func defaultObject(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	return append(json.RawMessage(nil), value...)
}
func joinText(values ...string) string {
	var result []string
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return strings.Join(result, "\n\n")
}
func decimalOrEmpty(value int64) string {
	if value < 1 {
		return ""
	}
	return fmt.Sprint(value)
}

func ErrorCode(err error) string {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return "PROJECT_NOT_FOUND"
	case errors.Is(err, domain.ErrArchived):
		return "PROJECT_ARCHIVED"
	case errors.Is(err, domain.ErrConflict):
		return "PROJECT_VERSION_CONFLICT"
	case errors.Is(err, domain.ErrInvalid):
		return "PROJECT_CONFIGURATION_INVALID"
	case errors.Is(err, domain.ErrDeliveryApprovalRequired):
		return "PROJECT_DELIVERY_APPROVAL_PERMISSION_REQUIRED"
	case errors.Is(err, domain.ErrRepositoryURLInvalid):
		return "PROJECT_REPOSITORY_URL_INVALID"
	case errors.Is(err, domain.ErrRepositoryCredentialRequired):
		return "PROJECT_REPOSITORY_CREDENTIAL_REQUIRED"
	case errors.Is(err, domain.ErrRepositoryAuthentication):
		return "PROJECT_REPOSITORY_AUTHENTICATION_FAILED"
	case errors.Is(err, domain.ErrRepositoryNotFound):
		return "PROJECT_REPOSITORY_NOT_FOUND"
	case errors.Is(err, domain.ErrRepositoryRefNotFound):
		return "PROJECT_REPOSITORY_REF_NOT_FOUND"
	case errors.Is(err, domain.ErrRepositoryPushUnavailable):
		return "PROJECT_REPOSITORY_PUSH_UNAVAILABLE"
	case errors.Is(err, domain.ErrRepositoryReviewUnavailable):
		return "PROJECT_REPOSITORY_REVIEW_UNAVAILABLE"
	case errors.Is(err, domain.ErrRepositoryPreparationFailed):
		return "PROJECT_REPOSITORY_PREPARATION_FAILED"
	case errors.Is(err, domain.ErrSourceUnavailable):
		return "PROJECT_REPOSITORY_NOT_READY"
	default:
		return ""
	}
}

var _ projectfacade.TaskCompiler = (*Service)(nil)
var _ projectfacade.RequirementsCompiler = (*Service)(nil)
