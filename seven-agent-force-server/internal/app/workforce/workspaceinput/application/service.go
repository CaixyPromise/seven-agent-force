package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	filefacade "github.com/CaixyPromise/seven-agent/force/internal/app/file/facade"
	gitdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/gittransport"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/domain"
)

var ErrNotFound = errors.New("WORKSPACE_INPUT_NOT_FOUND")

type ProjectRepositoryReader interface {
	GetProjectRepository(context.Context, int64, int64, int64) (gitdomain.RepositoryTarget, error)
}
type ArtifactStore interface {
	Put(context.Context, string, string, io.Reader, int64) error
	Open(context.Context, string) (io.ReadCloser, error)
}

type SourceRevision struct {
	ID                            int64  `json:"id,string"`
	SourceID                      int64  `json:"sourceId,string"`
	TenantID                      int64  `json:"tenantId,string"`
	Kind                          string `json:"kind"`
	ProjectID                     int64  `json:"projectId,string,omitempty"`
	ProjectVersionID              int64  `json:"projectVersionId,string,omitempty"`
	RepositoryConfigDigest        string `json:"repositoryConfigDigest,omitempty"`
	RefType                       string `json:"refType,omitempty"`
	RefName                       string `json:"refName,omitempty"`
	ResolvedCommit                string `json:"resolvedCommit,omitempty"`
	ResolvedTree                  string `json:"resolvedTree,omitempty"`
	RepositoryBundleContentSHA256 string `json:"repositoryBundleContentSha256,omitempty"`
	RepositoryBundleObjectSHA256  string `json:"repositoryBundleObjectSha256,omitempty"`
	RepositoryBundleObjectKey     string `json:"-"`
	RepositoryBundleSizeBytes     int64  `json:"repositoryBundleSizeBytes,omitempty"`
	SourceIdentityDigest          string `json:"sourceIdentityDigest,omitempty"`
	Status                        string `json:"status"`
	ReasonCode                    string `json:"reasonCode,omitempty"`
}
type IngestionRun struct {
	ID               int64  `json:"id,string"`
	TenantID         int64  `json:"tenantId,string"`
	SourceRevisionID int64  `json:"workspaceSourceRevisionId,string"`
	Kind             string `json:"kind"`
	Status           string `json:"status"`
	ReasonCode       string `json:"reasonCode,omitempty"`
	AttemptNumber    int    `json:"attemptNumber"`
	MaxAttempts      int    `json:"maxAttempts"`
}

type MachineAuthority struct {
	WorkerID   int64
	InstanceID string
	HostBootID string
	Generation int64
}

type SourceCredential struct {
	Username, Password, PrivateKey, KnownHosts, ExpiresAt string
}
type EncryptedRepositoryCredential struct {
	Ciphertext string
	EDEK       string
	WrapKeyRef string
	Version    int64
}

type SourceIngestionClaim struct {
	RunID              int64                      `json:"runId,string"`
	SourceRevisionID   int64                      `json:"workspaceSourceRevisionId,string"`
	Kind               string                     `json:"kind"`
	AttemptNumber      int                        `json:"attemptNumber"`
	Generation         int64                      `json:"generation,string"`
	LeaseToken         string                     `json:"leaseToken"`
	LeaseExpiresAt     string                     `json:"leaseExpiresAt"`
	Repository         gitdomain.RepositoryTarget `json:"repository,omitempty"`
	RequestedRefType   string                     `json:"requestedRefType,omitempty"`
	RequestedRefName   string                     `json:"requestedRefName,omitempty"`
	FrozenCommit       string                     `json:"frozenCommit,omitempty"`
	InputObjectKey     string                     `json:"inputObjectKey,omitempty"`
	InputContentSHA256 string                     `json:"inputContentSha256,omitempty"`
	InputObjectSHA256  string                     `json:"inputObjectSha256,omitempty"`
	InputSizeBytes     int64                      `json:"inputSizeBytes,omitempty"`
	BundleObjectKey    string                     `json:"bundleObjectKey"`
}

type SourceIngestionResult struct {
	ResolvedCommit                string `json:"resolvedCommit"`
	ResolvedTree                  string `json:"resolvedTree"`
	RepositoryBundleContentSHA256 string `json:"repositoryBundleContentSha256"`
	RepositoryBundleObjectSHA256  string `json:"repositoryBundleObjectSha256"`
	RepositoryBundleSizeBytes     int64  `json:"repositoryBundleSizeBytes"`
}

type CredentialResolver interface {
	ResolveSourceCredential(context.Context, EncryptedRepositoryCredential) (SourceCredential, error)
}
type ReferenceAsset struct {
	ID            int64  `json:"id,string"`
	TenantID      int64  `json:"tenantId,string"`
	DisplayName   string `json:"displayName"`
	ContentType   string `json:"contentType"`
	ContentSHA256 string `json:"contentSha256"`
	SizeBytes     int64  `json:"sizeBytes"`
	Status        string `json:"status"`
	ObjectKey     string `json:"-"`
	ObjectSHA256  string `json:"-"`
}
type FrozenTaskInput struct {
	Snapshot   domain.TaskInputSnapshot
	Source     SourceRevision
	References []ReferenceAsset
}
type TaskInputView struct {
	TaskID               int64            `json:"taskId,string"`
	TaskTurnID           int64            `json:"taskTurnId,string"`
	SourceKind           string           `json:"sourceKind"`
	WorkspaceSource      SourceRevision   `json:"workspaceSource"`
	References           []ReferenceAsset `json:"references"`
	InputSnapshotDigest  string           `json:"inputSnapshotDigest"`
	RepositoryBundleSize int64            `json:"repositoryBundleSizeBytes"`
}
type GitPlanView struct {
	SourceProjectID            int64  `json:"sourceProjectId,string,omitempty"`
	ProjectID                  int64  `json:"projectId,string,omitempty"`
	ReviewBaseSourceRevisionID int64  `json:"reviewBaseSourceRevisionId,string,omitempty"`
	SourceRefType              string `json:"sourceRefType,omitempty"`
	SourceRefName              string `json:"sourceRefName,omitempty"`
	SourceCommit               string `json:"sourceCommit"`
	ReviewBaseBranch           string `json:"reviewBaseBranch,omitempty"`
	ReviewBaseCommit           string `json:"reviewBaseCommit,omitempty"`
	ResultBranch               string `json:"resultBranch,omitempty"`
	DeliveryMode               string `json:"deliveryMode"`
	DeliveryAuthorization      string `json:"deliveryAuthorization"`
	FullProjectImport          bool   `json:"fullProjectImport"`
	PreviousDeliveryCommit     string `json:"previousDeliveryCommit,omitempty"`
}
type GitSourceRequest struct {
	ProjectID        int64  `json:"projectId,string"`
	ProjectVersionID int64  `json:"projectVersionId,string"`
	RefType          string `json:"refType"`
	RefName          string `json:"refName"`
	IdempotencyKey   string `json:"idempotencyKey"`
}
type UploadSourceRequest struct {
	// FileID is encoded as a JSON string at the HTTP boundary. Framework IDs
	// exceed JavaScript's safe integer range, so accepting a JSON number would
	// allow a browser to silently bind the wrong upload credential.
	FileID         int64  `json:"fileId,string"`
	SourceKind     string `json:"sourceKind"`
	IdempotencyKey string `json:"idempotencyKey"`
}
type ReferenceRequest struct {
	FileID         int64  `json:"fileId,string"`
	DisplayName    string `json:"displayName"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type Repository interface {
	CreateGitSource(context.Context, int64, int64, GitSourceRequest, gitdomain.RepositoryTarget) (SourceRevision, IngestionRun, bool, error)
	RetryGitSource(context.Context, int64, int64) error
	CreateUploadedSource(context.Context, int64, int64, UploadSourceRequest, filefacade.WorkforceInputClaim, string, string, string) (SourceRevision, IngestionRun, bool, error)
	CreateReference(context.Context, int64, int64, ReferenceRequest, filefacade.WorkforceInputClaim, string, string) (ReferenceAsset, bool, error)
	GetRevision(context.Context, int64, int64) (SourceRevision, error)
	GetRun(context.Context, int64, int64) (IngestionRun, error)
	GetReference(context.Context, int64, int64) (ReferenceAsset, error)
	GetTaskInput(context.Context, int64, int64) (TaskInputView, error)
	GetGitPlan(context.Context, int64, int64) (GitPlanView, error)
	ResolveTaskInput(context.Context, int64, int64, []int64) (domain.SourceRevisionView, []domain.ReferenceAssetView, error)
	ClaimIngestion(context.Context, MachineAuthority, int64) (SourceIngestionClaim, error)
	FreezeSourceCommit(context.Context, MachineAuthority, int64, string, string) error
	ResolveCredentialScope(context.Context, MachineAuthority, int64, string) (EncryptedRepositoryCredential, error)
	CompleteIngestion(context.Context, MachineAuthority, int64, string, SourceIngestionResult) (SourceRevision, error)
	FailIngestion(context.Context, MachineAuthority, int64, string, string) error
}
type Service struct {
	repository  Repository
	projects    ProjectRepositoryReader
	files       filefacade.WorkforceInputAssetFacade
	artifacts   ArtifactStore
	credentials CredentialResolver
}

func New(repository Repository, projects ProjectRepositoryReader, artifacts ArtifactStore) *Service {
	return &Service{repository: repository, projects: projects, artifacts: artifacts}
}
func (s *Service) BindFiles(files filefacade.WorkforceInputAssetFacade) { s.files = files }
func (s *Service) BindCredentials(credentials CredentialResolver)       { s.credentials = credentials }

func (s *Service) CreateGitSource(ctx context.Context, tenant int64, actor int64, request GitSourceRequest) (SourceRevision, IngestionRun, bool, error) {
	request.RefType = strings.ToUpper(strings.TrimSpace(request.RefType))
	request.RefName = strings.TrimSpace(request.RefName)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.RefType != "BRANCH" && request.RefType != "TAG" && request.RefType != "COMMIT" || request.RefName == "" || request.IdempotencyKey == "" {
		return SourceRevision{}, IngestionRun{}, false, domain.ErrTaskInputInvalid
	}
	if request.ProjectID < 1 || request.ProjectVersionID < 1 {
		return SourceRevision{}, IngestionRun{}, false, domain.ErrTaskInputInvalid
	}
	connection, err := s.projects.GetProjectRepository(ctx, tenant, request.ProjectID, request.ProjectVersionID)
	if err != nil {
		return SourceRevision{}, IngestionRun{}, false, err
	}
	if err = gitdomain.RequireCapability(connection, gitdomain.CapabilityReadSource); err != nil {
		return SourceRevision{}, IngestionRun{}, false, err
	}
	return s.repository.CreateGitSource(ctx, tenant, actor, request, connection)
}

func (s *Service) PrepareProjectSource(ctx context.Context, tenant, actor int64, request GitSourceRequest) error {
	_, run, replay, err := s.CreateGitSource(ctx, tenant, actor, request)
	if err != nil {
		return err
	}
	if replay && (run.Status == "FAILED" || run.Status == "WAITING_WORKER") {
		return s.repository.RetryGitSource(ctx, tenant, run.ID)
	}
	return nil
}
func (s *Service) CreateUploadedSource(ctx context.Context, tenant int64, actor int64, request UploadSourceRequest) (SourceRevision, IngestionRun, bool, error) {
	if s.files == nil || s.artifacts == nil {
		return SourceRevision{}, IngestionRun{}, false, errors.New("WORKFORCE_INPUT_ASSET_UNAVAILABLE")
	}
	request.SourceKind = strings.ToUpper(strings.TrimSpace(request.SourceKind))
	if request.SourceKind != "ARCHIVE" && request.SourceKind != "DIRECTORY_ARCHIVE" {
		return SourceRevision{}, IngestionRun{}, false, domain.ErrTaskInputInvalid
	}
	claim, err := s.files.ClaimWorkforceInput(ctx, filefacade.ClaimWorkforceInputCommand{FileID: request.FileID, Kind: filefacade.WorkforceInputWorkspaceArchive, DisplayName: "workspace-source"})
	if err != nil {
		return SourceRevision{}, IngestionRun{}, false, err
	}
	key := "workspace-inputs/" + safe(strconv.FormatInt(tenant, 10)) + "/" + digestText(claim.Handle+"\x00"+claim.ContentSHA256) + "/source.archive"
	objectDigest, err := s.copyAndVerify(ctx, claim, key)
	if err != nil {
		_ = s.files.ReleaseWorkforceInput(ctx, filefacade.ReleaseWorkforceInputCommand{Handle: claim.Handle, ExpectedScopeID: claim.ScopeID})
		return SourceRevision{}, IngestionRun{}, false, err
	}
	return s.repository.CreateUploadedSource(ctx, tenant, actor, request, *claim, key, claim.ContentSHA256, objectDigest)
}
func (s *Service) CreateReference(ctx context.Context, tenant int64, actor int64, request ReferenceRequest) (ReferenceAsset, bool, error) {
	if s.files == nil || s.artifacts == nil {
		return ReferenceAsset{}, false, errors.New("WORKFORCE_INPUT_ASSET_UNAVAILABLE")
	}
	claim, err := s.files.ClaimWorkforceInput(ctx, filefacade.ClaimWorkforceInputCommand{FileID: request.FileID, Kind: filefacade.WorkforceInputReference, DisplayName: request.DisplayName})
	if err != nil {
		return ReferenceAsset{}, false, err
	}
	key := "workspace-inputs/" + safe(strconv.FormatInt(tenant, 10)) + "/references/" + digestText(claim.Handle+"\x00"+claim.ContentSHA256) + "/" + safe(claim.FileName)
	objectDigest, err := s.copyAndVerify(ctx, claim, key)
	if err != nil {
		return ReferenceAsset{}, false, err
	}
	item, replayed, err := s.repository.CreateReference(ctx, tenant, actor, request, *claim, key, objectDigest)
	if replayed {
		// The immutable tenant asset already exists. The newly-issued private file
		// handle is redundant and must not accumulate as an unreachable reference.
		_ = s.files.ReleaseWorkforceInput(ctx, filefacade.ReleaseWorkforceInputCommand{Handle: claim.Handle, ExpectedScopeID: claim.ScopeID})
	}
	return item, replayed, err
}
func (s *Service) GetRevision(ctx context.Context, tenant int64, id int64) (SourceRevision, error) {
	return s.repository.GetRevision(ctx, tenant, id)
}
func (s *Service) GetRun(ctx context.Context, tenant int64, id int64) (IngestionRun, error) {
	return s.repository.GetRun(ctx, tenant, id)
}
func (s *Service) GetReference(ctx context.Context, tenant int64, id int64) (ReferenceAsset, error) {
	return s.repository.GetReference(ctx, tenant, id)
}
func (s *Service) GetTaskInput(ctx context.Context, tenant int64, taskID int64) (TaskInputView, error) {
	return s.repository.GetTaskInput(ctx, tenant, taskID)
}
func (s *Service) GetGitPlan(ctx context.Context, tenant int64, taskID int64) (GitPlanView, error) {
	return s.repository.GetGitPlan(ctx, tenant, taskID)
}
func (s *Service) ResolveTaskInput(ctx context.Context, tenant int64, source int64, refs []int64) (domain.SourceRevisionView, []domain.ReferenceAssetView, error) {
	return s.repository.ResolveTaskInput(ctx, tenant, source, refs)
}
func (s *Service) FreezeTaskInput(ctx context.Context, tenant int64, mode string, sourceID int64, referenceIDs []int64, plan domain.GitPlan) (FrozenTaskInput, error) {
	mode = strings.ToUpper(strings.TrimSpace(mode))
	var source SourceRevision
	var err error
	if mode == domain.SourceKindSourceRevision {
		source, err = s.repository.GetRevision(ctx, tenant, sourceID)
		if err != nil {
			return FrozenTaskInput{}, err
		}
	} else if mode != domain.SourceKindEmptyWorkspace {
		return FrozenTaskInput{}, domain.ErrTaskInputInvalid
	}
	referenceViews := make([]domain.ReferenceAssetView, 0, len(referenceIDs))
	references := make([]ReferenceAsset, 0, len(referenceIDs))
	for _, id := range referenceIDs {
		item, itemErr := s.repository.GetReference(ctx, tenant, id)
		if itemErr != nil {
			return FrozenTaskInput{}, itemErr
		}
		references = append(references, item)
		referenceViews = append(referenceViews, domain.ReferenceAssetView{ID: item.ID, TenantID: item.TenantID, Status: item.Status, ContentSHA256: item.ContentSHA256, SizeBytes: item.SizeBytes})
	}
	if plan.DeliveryMode == "" {
		plan.DeliveryMode = domain.DeliveryNone
	}
	if mode == domain.SourceKindEmptyWorkspace && plan.DeliveryMode != domain.DeliveryNone {
		return FrozenTaskInput{}, errors.New("TASK_SOURCE_REQUIRED_FOR_GIT_DELIVERY")
	}
	if plan.DeliveryMode != domain.DeliveryNone {
		target, targetErr := s.projects.GetProjectRepository(ctx, tenant, plan.ProjectID, plan.ProjectVersionID)
		if targetErr != nil {
			return FrozenTaskInput{}, targetErr
		}
		if targetErr = gitdomain.RequireCapability(target, gitdomain.CapabilityPushBranch); targetErr != nil {
			return FrozenTaskInput{}, targetErr
		}
		plan.TargetConfigDigest = target.ConfigDigest
		if plan.RepositoryConfigDigest != target.ConfigDigest {
			return FrozenTaskInput{}, domain.ErrGitPlanInvalid
		}
		plan.PushAuthorizationDigest = frozenRepositoryAuthorizationDigest(target.ConfigDigest, gitdomain.CapabilityPushBranch, target.Push.CredentialRef, target.Push.CredentialVersion)
		if plan.DeliveryMode == domain.DeliveryNewBranchAndDraftReview {
			if targetErr = gitdomain.RequireCapability(target, gitdomain.CapabilityCreateReview); targetErr != nil {
				return FrozenTaskInput{}, targetErr
			}
			plan.ReviewAuthorizationDigest = frozenRepositoryAuthorizationDigest(target.ConfigDigest, gitdomain.CapabilityCreateReview, target.Review.CredentialRef, target.Review.CredentialVersion)
		}
		if plan.ReviewBaseBranch == "" {
			return FrozenTaskInput{}, errors.New("TARGET_BASE_BRANCH_REQUIRED")
		}
		if plan.ReviewBaseSourceRevisionID != 0 {
			baseRevision, baseErr := s.repository.GetRevision(ctx, tenant, plan.ReviewBaseSourceRevisionID)
			if baseErr != nil {
				return FrozenTaskInput{}, baseErr
			}
			if baseRevision.Status != domain.SourceAvailable || baseRevision.Kind != gitdomain.SourceGitRepository || baseRevision.ProjectID != target.ID || baseRevision.RefType != gitdomain.RefBranch || baseRevision.RefName != plan.ReviewBaseBranch {
				return FrozenTaskInput{}, errors.New("TARGET_BASE_REVISION_IDENTITY_MISMATCH")
			}
			plan.ReviewBaseCommit = baseRevision.ResolvedCommit
		}
		if plan.ReviewBaseCommit == "" && source.ProjectID == target.ID && source.RefType == gitdomain.RefBranch && source.RefName == plan.ReviewBaseBranch {
			plan.ReviewBaseCommit = source.ResolvedCommit
		}
		if plan.ReviewBaseCommit == "" {
			return FrozenTaskInput{}, errors.New("TARGET_BASE_REVISION_NOT_FROZEN")
		}
		if source.ResolvedCommit != plan.ReviewBaseCommit {
			return FrozenTaskInput{}, errors.New("TASK_INPUT_SOURCE_CONFLICT")
		}
		policy := domain.RepositoryPolicy{ReviewSupported: target.ProviderType != gitdomain.ProviderGeneric}
		if targetErr = domain.ValidateGitPlan(plan, policy); targetErr != nil {
			return FrozenTaskInput{}, targetErr
		}
	} else if err = domain.ValidateGitPlan(plan, domain.RepositoryPolicy{}); err != nil {
		return FrozenTaskInput{}, err
	}
	var snapshot domain.TaskInputSnapshot
	if mode == domain.SourceKindEmptyWorkspace {
		identity := domain.CanonicalEmptyWorkspaceIdentity()
		source = SourceRevision{Kind: domain.SourceKindEmptyWorkspace, Status: domain.SourceAvailable, TenantID: tenant, ResolvedCommit: identity.Commit, ResolvedTree: identity.Tree, SourceIdentityDigest: identity.IdentityDigest}
		snapshot, err = domain.FreezeEmptyTaskInput(tenant, referenceViews, plan)
	} else {
		snapshot, err = domain.FreezeTaskInput(tenant, domain.SourceRevisionView{ID: source.ID, TenantID: source.TenantID, Status: source.Status, SourceCommit: source.ResolvedCommit, SourceTree: source.ResolvedTree, SourceIdentityDigest: source.SourceIdentityDigest}, referenceViews, plan)
	}
	if err != nil {
		return FrozenTaskInput{}, err
	}
	return FrozenTaskInput{Snapshot: snapshot, Source: source, References: references}, nil
}

func frozenRepositoryAuthorizationDigest(connectionDigest, capability, credentialRef, credentialVersion string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{strings.TrimSpace(connectionDigest), strings.TrimSpace(capability), strings.TrimSpace(credentialRef), strings.TrimSpace(credentialVersion)}, "\x00")))
	return hex.EncodeToString(sum[:])
}
func (s *Service) ClaimIngestion(ctx context.Context, authority MachineAuthority, id int64) (SourceIngestionClaim, error) {
	return s.repository.ClaimIngestion(ctx, authority, id)
}
func (s *Service) FreezeSourceCommit(ctx context.Context, authority MachineAuthority, id int64, lease string, commit string) error {
	return s.repository.FreezeSourceCommit(ctx, authority, id, lease, commit)
}
func (s *Service) MaterializeCredential(ctx context.Context, authority MachineAuthority, id int64, lease string) (SourceCredential, error) {
	if s.credentials == nil {
		return SourceCredential{}, errors.New("SOURCE_CREDENTIAL_RESOLVER_UNAVAILABLE")
	}
	envelope, err := s.repository.ResolveCredentialScope(ctx, authority, id, lease)
	if err != nil {
		return SourceCredential{}, err
	}
	return s.credentials.ResolveSourceCredential(ctx, envelope)
}
func (s *Service) CompleteIngestion(ctx context.Context, authority MachineAuthority, id int64, lease string, result SourceIngestionResult) (SourceRevision, error) {
	return s.repository.CompleteIngestion(ctx, authority, id, lease, result)
}
func (s *Service) FailIngestion(ctx context.Context, authority MachineAuthority, id int64, lease string, reason string) error {
	return s.repository.FailIngestion(ctx, authority, id, lease, reason)
}
func (s *Service) copyAndVerify(ctx context.Context, claim *filefacade.WorkforceInputClaim, key string) (string, error) {
	opened, err := s.files.OpenWorkforceInput(ctx, filefacade.OpenWorkforceInputCommand{Handle: claim.Handle, ExpectedScopeID: claim.ScopeID})
	if err != nil {
		return "", err
	}
	defer opened.Reader.Close()
	if opened.Size != claim.Size || !strings.EqualFold(opened.ContentSHA256, claim.ContentSHA256) {
		return "", errors.New("WORKFORCE_INPUT_IDENTITY_MISMATCH")
	}
	if err = s.artifacts.Put(ctx, key, opened.ContentType, opened.Reader, opened.Size); err != nil {
		return "", err
	}
	reader, err := s.artifacts.Open(ctx, key)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(reader, opened.Size+1))
	if err != nil || size != opened.Size {
		return "", errors.New("WORKFORCE_INPUT_READBACK_FAILED")
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(digest, opened.ContentSHA256) {
		return "", fmt.Errorf("WORKFORCE_INPUT_DIGEST_MISMATCH")
	}
	return digest, nil
}
func digestText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func safe(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "input"
	}
	return b.String()
}
