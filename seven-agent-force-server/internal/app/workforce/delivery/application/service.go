package application

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
	deliveryfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/facade"
)

var (
	ErrNotFound                       = errors.New("DELIVERY_NOT_FOUND")
	ErrReconciliationNotAllowed       = errors.New("DELIVERY_RECONCILIATION_NOT_ALLOWED")
	ErrReconciliationInconclusive     = errors.New("DELIVERY_RECONCILIATION_INCONCLUSIVE")
	ErrReconciliationEvidenceMismatch = errors.New("DELIVERY_RECONCILIATION_EVIDENCE_MISMATCH")
)

type CreateRequest struct {
	TaskID           int64
	TenantID         int64
	Actor            int64
	ActorServiceCode string
	IdempotencyKey   string
	Action           domain.Action `json:"action"`
}
type Approval struct {
	TenantID  int64
	RequestID int64
	Actor     int64
	Reason    string
}
type Decision struct {
	TenantID, TaskID, RequestID, Actor, ExpectedVersion int64
	Action, Reason, IdempotencyKey                      string
}
type Restart struct {
	TenantID, TaskID, Actor      int64
	ResultCommit, IdempotencyKey string
}
type Reauthorization struct {
	ProjectVersionID int64
	TenantID         int64
	RequestID        int64
	OperationID      int64
	Actor            int64
	Reason           string
}

type RequestView struct {
	ID                int64       `json:"id,string"`
	Version           int64       `json:"version,string"`
	TaskID            int64       `json:"taskId,string"`
	TaskTurnID        int64       `json:"taskTurnId,string"`
	CheckpointID      int64       `json:"checkpointId,string"`
	ActionType        string      `json:"actionType"`
	ProjectID         int64       `json:"projectId,string"`
	BaseBranch        string      `json:"baseBranch"`
	DestinationBranch string      `json:"destinationBranch"`
	ResultCommit      string      `json:"resultCommit"`
	ResultTree        string      `json:"resultTree"`
	SelectionDigest   string      `json:"selectionDigest"`
	DeliveryCommit    string      `json:"deliveryCommit"`
	ApprovalMode      string      `json:"approvalMode"`
	ApprovedBy        int64       `json:"approvedBy,string,omitempty"`
	ApprovedAt        string      `json:"approvedAt,omitempty"`
	Status            string      `json:"status"`
	ReasonCode        string      `json:"reasonCode,omitempty"`
	CreatedAt         string      `json:"createdAt"`
	Review            *ReviewView `json:"review,omitempty"`
}

type ReviewView struct {
	OperationID    int64  `json:"operationId,string,omitempty"`
	Status         string `json:"status"`
	ProviderType   string `json:"providerType"`
	ProviderNumber string `json:"providerNumber,omitempty"`
	WebURL         string `json:"webUrl,omitempty"`
	ReasonCode     string `json:"reasonCode,omitempty"`
}

type AttemptView struct {
	ID               int64  `json:"id,string"`
	RequestID        int64  `json:"requestId,string"`
	AttemptNumber    int    `json:"attemptNumber"`
	WorkerID         int64  `json:"workerId,string"`
	WorkerInstanceID string `json:"workerInstanceId"`
	HostBootID       string `json:"hostBootId"`
	Phase            string `json:"phase"`
	Status           string `json:"status"`
	RemoteBefore     string `json:"remoteBefore,omitempty"`
	RemoteAfter      string `json:"remoteAfter,omitempty"`
	ReasonCode       string `json:"reasonCode,omitempty"`
	StartedAt        string `json:"startedAt,omitempty"`
	VerifiedAt       string `json:"verifiedAt,omitempty"`
	FinishedAt       string `json:"finishedAt,omitempty"`
}

type Repository interface {
	CreateRequest(context.Context, CreateRequest) (RequestView, bool, error)
	ListRequests(context.Context, int64, int64) ([]RequestView, error)
	GetRequest(context.Context, int64, int64) (RequestView, error)
	GetAttempt(context.Context, int64, int64) (AttemptView, error)
	Approve(context.Context, Approval) (RequestView, error)
	Decide(context.Context, Decision) (RequestView, bool, error)
	Restart(context.Context, Restart) (RequestView, bool, error)
	Cancel(context.Context, int64, int64, int64) (RequestView, error)
	Retry(context.Context, int64, int64, int64) (RequestView, error)
	ReauthorizeRequest(context.Context, Reauthorization) (RequestView, error)
	ReauthorizeReview(context.Context, Reauthorization) (RequestView, error)
	PublishIntent(context.Context, deliveryfacade.Machine, deliveryfacade.PublishIntentRequest) (deliveryfacade.IntentReceipt, deliveryfacade.ImmediateToolResult, error)
	AckIntent(context.Context, deliveryfacade.Machine, deliveryfacade.AckIntentRequest) (deliveryfacade.IntentReceipt, error)
	CloseIntent(context.Context, deliveryfacade.Machine, deliveryfacade.CloseIntentRequest) (deliveryfacade.IntentReceipt, error)
}

type Service struct {
	repository       Repository
	reviewReconciler *ReviewCoordinator
}

// DeliveryCredential is an ephemeral, target-scoped value. It is returned only
// by the Machine API and must never be logged or persisted by Workforce.
type DeliveryCredential struct {
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	PrivateKey string `json:"privateKey,omitempty"`
	KnownHosts string `json:"knownHosts,omitempty"`
	ExpiresAt  string `json:"expiresAt"`
}

// EncryptedCredentialEnvelope is the credential snapshot frozen with the
// Task Git plan. Delivery may decrypt it for one machine-authorized operation,
// but it never looks up a mutable Project or Repository Connection.
type EncryptedCredentialEnvelope struct {
	Ciphertext string
	EDEK       string
	WrapKeyRef string
	Version    int64
}

type FrozenCredentialResolver interface {
	ResolveFrozenDeliveryCredential(context.Context, EncryptedCredentialEnvelope) (DeliveryCredential, error)
}

type MachineAuthority struct {
	WorkerID   int64
	InstanceID string
	HostBootID string
	Generation int64
}
type DeliveryClaim struct {
	AttemptID                     int64         `json:"attemptId,string"`
	RequestID                     int64         `json:"requestId,string"`
	TaskID                        int64         `json:"taskId,string"`
	TaskTurnID                    int64         `json:"taskTurnId,string"`
	ActionType                    string        `json:"actionType"`
	Action                        domain.Action `json:"action"`
	ProcessorReleaseID            int64         `json:"processorReleaseId,string"`
	ProcessorDigest               string        `json:"processorDigest"`
	ProcessorArchiveObjectKey     string        `json:"processorArchiveObjectKey"`
	HarnessImageDigest            string        `json:"harnessImageDigest"`
	AttemptNumber                 int           `json:"attemptNumber"`
	Generation                    int64         `json:"generation,string"`
	LeaseToken                    string        `json:"leaseToken"`
	LeaseExpiresAt                string        `json:"leaseExpiresAt"`
	PlainCheckpointKeyBase64      string        `json:"plainCheckpointKeyBase64"`
	CheckpointID                  int64         `json:"checkpointId,string"`
	CheckpointIdentity            string        `json:"checkpointIdentity"`
	RepositoryBundleObjectKey     string        `json:"repositoryBundleObjectKey"`
	RepositoryBundleContentSHA256 string        `json:"repositoryBundleContentSha256"`
	RepositoryBundleObjectSHA256  string        `json:"repositoryBundleObjectSha256"`
	RepositoryBundleSizeBytes     int64         `json:"repositoryBundleSizeBytes"`
	ResultCommit                  string        `json:"resultCommit"`
	ResultTree                    string        `json:"resultTree"`
	SelectionDigest               string        `json:"selectionDigest"`
	ExpectedBaseCommit            string        `json:"expectedBaseCommit"`
	DeliveryCommit                string        `json:"deliveryCommit"`
	CommitTimestamp               string        `json:"commitTimestamp"`
	BaseBranch                    string        `json:"baseBranch"`
	DestinationBranch             string        `json:"destinationBranch"`
	Target                        domain.Target `json:"target"`
	WrappedDataKey                string        `json:"-"`
	ReconciliationOnly            bool          `json:"reconciliationOnly,omitempty"`
}
type PrepareResult struct {
	PushAuthorization string `json:"pushAuthorization"`
	ExpiresAt         string `json:"expiresAt"`
}
type DeliveryResult struct {
	RemoteBefore string `json:"remoteBefore"`
	RemoteAfter  string `json:"remoteAfter"`
	Readable     bool   `json:"readable"`
	PushStarted  bool   `json:"pushStarted"`
}
type MachineRepository interface {
	Claim(context.Context, MachineAuthority, int64) (DeliveryClaim, error)
	PreparePush(context.Context, MachineAuthority, int64, string, string) (PrepareResult, error)
	RecordResult(context.Context, MachineAuthority, int64, string, string, DeliveryResult) error
	RecordFailure(context.Context, MachineAuthority, int64, string, string) error
	RecordReconciliation(context.Context, MachineAuthority, int64, string, DeliveryResult) error
	ResolveCredentialEnvelope(context.Context, MachineAuthority, int64, string) (EncryptedCredentialEnvelope, error)
}
type CheckpointKeyResolver interface {
	UnwrapCheckpointKey(context.Context, string) ([]byte, error)
}
type MachineService struct {
	repository  MachineRepository
	keys        CheckpointKeyResolver
	credentials FrozenCredentialResolver
}

func NewMachineService(repository MachineRepository, keys CheckpointKeyResolver) *MachineService {
	resolver := checkpointEnvelopeCredentialResolver{keys: keys}
	if binder, ok := repository.(interface {
		SetFrozenCredentialResolver(FrozenCredentialResolver)
	}); ok {
		binder.SetFrozenCredentialResolver(resolver)
	}
	return &MachineService{repository: repository, keys: keys, credentials: resolver}
}

func (s *MachineService) Claim(ctx context.Context, a MachineAuthority, id int64) (DeliveryClaim, error) {
	claim, err := s.repository.Claim(ctx, a, id)
	if err != nil {
		return claim, err
	}
	key, err := s.keys.UnwrapCheckpointKey(ctx, claim.WrappedDataKey)
	if err != nil {
		return DeliveryClaim{}, err
	}
	claim.PlainCheckpointKeyBase64 = base64.StdEncoding.EncodeToString(key)
	for index := range key {
		key[index] = 0
	}
	return claim, nil
}
func (s *MachineService) PreparePush(ctx context.Context, a MachineAuthority, id int64, lease string, commit string) (PrepareResult, error) {
	return s.repository.PreparePush(ctx, a, id, lease, commit)
}
func (s *MachineService) MaterializeCredential(ctx context.Context, a MachineAuthority, id int64, lease string) (DeliveryCredential, error) {
	if s.credentials == nil {
		return DeliveryCredential{}, errors.New("DELIVERY_CREDENTIAL_RESOLVER_UNAVAILABLE")
	}
	envelope, err := s.repository.ResolveCredentialEnvelope(ctx, a, id, lease)
	if err != nil {
		return DeliveryCredential{}, err
	}
	return s.credentials.ResolveFrozenDeliveryCredential(ctx, envelope)
}
func (s *MachineService) RecordResult(ctx context.Context, a MachineAuthority, id int64, lease string, authorization string, result DeliveryResult) error {
	return s.repository.RecordResult(ctx, a, id, lease, authorization, result)
}
func (s *MachineService) RecordFailure(ctx context.Context, a MachineAuthority, id int64, lease string, reason string) error {
	return s.repository.RecordFailure(ctx, a, id, lease, reason)
}
func (s *MachineService) RecordReconciliation(ctx context.Context, a MachineAuthority, id int64, lease string, result DeliveryResult) error {
	return s.repository.RecordReconciliation(ctx, a, id, lease, result)
}

func New(repository Repository, _ bool) *Service {
	return &Service{repository: repository}
}
func (s *Service) SetReviewReconciler(reconciler *ReviewCoordinator) { s.reviewReconciler = reconciler }

func (s *Service) PublishIntent(ctx context.Context, machine deliveryfacade.Machine, request deliveryfacade.PublishIntentRequest) (deliveryfacade.IntentReceipt, deliveryfacade.ImmediateToolResult, error) {
	return s.repository.PublishIntent(ctx, machine, request)
}
func (s *Service) AckIntent(ctx context.Context, machine deliveryfacade.Machine, request deliveryfacade.AckIntentRequest) (deliveryfacade.IntentReceipt, error) {
	return s.repository.AckIntent(ctx, machine, request)
}
func (s *Service) CloseIntent(ctx context.Context, machine deliveryfacade.Machine, request deliveryfacade.CloseIntentRequest) (deliveryfacade.IntentReceipt, error) {
	return s.repository.CloseIntent(ctx, machine, request)
}

var _ deliveryfacade.MachineToolService = (*Service)(nil)

func (s *Service) CreateRequest(ctx context.Context, request CreateRequest) (RequestView, bool, error) {
	if strings.TrimSpace(request.IdempotencyKey) == "" || len(request.IdempotencyKey) > 128 {
		return RequestView{}, false, domain.ErrIdentityConflict
	}
	return s.repository.CreateRequest(ctx, request)
}
func (s *Service) ListRequests(ctx context.Context, tenant int64, task int64) ([]RequestView, error) {
	return s.repository.ListRequests(ctx, tenant, task)
}
func (s *Service) GetRequest(ctx context.Context, tenant int64, id int64) (RequestView, error) {
	return s.repository.GetRequest(ctx, tenant, id)
}
func (s *Service) GetAttempt(ctx context.Context, tenant int64, id int64) (AttemptView, error) {
	return s.repository.GetAttempt(ctx, tenant, id)
}
func (s *Service) Approve(ctx context.Context, a Approval) (RequestView, error) {
	if strings.TrimSpace(a.Reason) == "" || len(a.Reason) > 2048 {
		return RequestView{}, domain.ErrApprovalRequired
	}
	return s.repository.Approve(ctx, a)
}
func (s *Service) Decide(ctx context.Context, d Decision) (RequestView, bool, error) {
	d.Action = strings.ToUpper(strings.TrimSpace(d.Action))
	d.Reason = strings.TrimSpace(d.Reason)
	d.IdempotencyKey = strings.TrimSpace(d.IdempotencyKey)
	if d.TenantID <= 0 || d.TaskID <= 0 || d.RequestID <= 0 || d.Actor <= 0 || d.ExpectedVersion <= 0 || (d.Action != "DELIVER" && d.Action != "SKIP_THIS_RESULT") || len([]byte(d.Reason)) > 2048 || d.IdempotencyKey == "" || len([]byte(d.IdempotencyKey)) > 128 {
		return RequestView{}, false, domain.ErrDecisionNotAllowed
	}
	return s.repository.Decide(ctx, d)
}
func (s *Service) Restart(ctx context.Context, r Restart) (RequestView, bool, error) {
	r.ResultCommit = strings.ToLower(strings.TrimSpace(r.ResultCommit))
	r.IdempotencyKey = strings.TrimSpace(r.IdempotencyKey)
	if r.TenantID <= 0 || r.TaskID <= 0 || r.Actor <= 0 || len(r.ResultCommit) != 40 || !isLowerHex(r.ResultCommit) || r.IdempotencyKey == "" || len([]byte(r.IdempotencyKey)) > 128 {
		return RequestView{}, false, domain.ErrDecisionNotAllowed
	}
	return s.repository.Restart(ctx, r)
}

func isLowerHex(value string) bool {
	for _, ch := range value {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}
func (s *Service) Cancel(ctx context.Context, tenant int64, actor int64, id int64) (RequestView, error) {
	return s.repository.Cancel(ctx, tenant, actor, id)
}
func (s *Service) Retry(ctx context.Context, tenant int64, actor int64, id int64) (RequestView, error) {
	return s.repository.Retry(ctx, tenant, actor, id)
}
func (s *Service) Reconcile(ctx context.Context, tenant int64, actor int64, id int64) (RequestView, error) {
	reconciler, ok := s.repository.(interface {
		Reconcile(context.Context, int64, int64, int64) (RequestView, error)
	})
	if !ok {
		return RequestView{}, ErrReconciliationNotAllowed
	}
	return reconciler.Reconcile(ctx, tenant, actor, id)
}
func (s *Service) ReconcileReview(ctx context.Context, tenant, actor, operationID int64) (ReviewReconciliationResult, error) {
	if s.reviewReconciler == nil {
		return ReviewReconciliationResult{}, ErrReconciliationNotAllowed
	}
	return s.reviewReconciler.ReconcileReview(ctx, tenant, actor, operationID)
}
func (s *Service) ReauthorizeRequest(ctx context.Context, request Reauthorization) (RequestView, error) {
	request.Reason = strings.TrimSpace(request.Reason)
	if request.TenantID <= 0 || request.RequestID <= 0 || request.Actor <= 0 || request.ProjectVersionID < 0 || request.Reason == "" || len([]byte(request.Reason)) > 2048 {
		return RequestView{}, domain.ErrApprovalRequired
	}
	return s.repository.ReauthorizeRequest(ctx, request)
}
func (s *Service) ReauthorizeReview(ctx context.Context, request Reauthorization) (RequestView, error) {
	if strings.TrimSpace(request.Reason) == "" || len(request.Reason) > 2048 {
		return RequestView{}, domain.ErrApprovalRequired
	}
	return s.repository.ReauthorizeReview(ctx, request)
}

type checkpointEnvelopeCredentialResolver struct{ keys CheckpointKeyResolver }

func (r checkpointEnvelopeCredentialResolver) ResolveFrozenDeliveryCredential(ctx context.Context, envelope EncryptedCredentialEnvelope) (DeliveryCredential, error) {
	if r.keys == nil || envelope.Version <= 0 || strings.TrimSpace(envelope.Ciphertext) == "" || strings.TrimSpace(envelope.EDEK) == "" || strings.TrimSpace(envelope.WrapKeyRef) == "" {
		return DeliveryCredential{}, errors.New("DELIVERY_CREDENTIAL_ENVELOPE_INVALID")
	}
	wrapper, err := json.Marshal(map[string]string{"ciphertext": envelope.Ciphertext, "edek": envelope.EDEK, "keyId": envelope.WrapKeyRef})
	if err != nil {
		return DeliveryCredential{}, err
	}
	plain, err := r.keys.UnwrapCheckpointKey(ctx, string(wrapper))
	if err != nil {
		return DeliveryCredential{}, err
	}
	defer func() {
		for index := range plain {
			plain[index] = 0
		}
	}()
	if len(plain) == 0 || len(plain) > 256<<10 {
		return DeliveryCredential{}, errors.New("DELIVERY_CREDENTIAL_INVALID")
	}
	var frozen struct {
		Username string
		Token    string
	}
	decoder := json.NewDecoder(bytes.NewReader(plain))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&frozen); err != nil || decoder.Decode(&struct{}{}) == nil {
		return DeliveryCredential{}, errors.New("DELIVERY_CREDENTIAL_INVALID")
	}
	frozen.Username = strings.TrimSpace(frozen.Username)
	frozen.Token = strings.TrimSpace(frozen.Token)
	if frozen.Username == "" || frozen.Token == "" {
		return DeliveryCredential{}, errors.New("DELIVERY_CREDENTIAL_INVALID")
	}
	return DeliveryCredential{Username: frozen.Username, Password: frozen.Token, ExpiresAt: time.Now().UTC().Add(2 * time.Minute).Format(time.RFC3339Nano)}, nil
}
