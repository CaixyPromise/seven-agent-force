package application

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	wcp "github.com/CaixyPromise/seven-agent/force/internal/worker/checkpoint"
)

type CreateNextTurnRequest struct {
	TaskID                int64  `json:"-"`
	TenantID              int64  `json:"-"`
	CreatorUserID         int64  `json:"-"`
	SourceCheckpointID    int64  `json:"sourceCheckpointId,string"`
	ExpectedCurrentTurnID int64  `json:"expectedCurrentTurnId,string"`
	Instruction           string `json:"instruction"`
	IdempotencyKey        string `json:"idempotencyKey"`
}

type ApproveRequest struct {
	CheckpointID int64  `json:"-"`
	TenantID     int64  `json:"-"`
	Actor        int64  `json:"-"`
	Reason       string `json:"reason"`
}

type CreateMaterializationRequest struct {
	TaskID           int64  `json:"-"`
	TenantID         int64  `json:"-"`
	Actor            int64  `json:"-"`
	ActorServiceCode string `json:"-"`
	TaskTurnID       int64  `json:"taskTurnId,string"`
	Kind             string `json:"kind"`
	OutputID         int64  `json:"outputId,string,omitempty"`
	IdempotencyKey   string `json:"idempotencyKey"`
}

type Repository interface {
	CreateNextTurn(context.Context, CreateNextTurnRequest) (domain.Turn, bool, error)
	Approve(context.Context, ApproveRequest) (domain.Checkpoint, error)
	ListTurns(context.Context, int64, int64) ([]domain.Turn, error)
	GetTurn(context.Context, int64, int64) (domain.Turn, error)
	GetCheckpoint(context.Context, int64, int64) (domain.Checkpoint, error)
	CreateMaterialization(context.Context, CreateMaterializationRequest) (domain.MaterializationView, bool, error)
	ListMaterializations(context.Context, int64, int64) ([]domain.MaterializationView, error)
	GetMaterialization(context.Context, int64, int64) (domain.MaterializationView, error)
	RetryMaterialization(context.Context, int64, int64, int64, string) error
	ClaimMaterialization(context.Context, MachineAuthority, int64) (domain.MaterializationClaim, error)
	FreezeMaterialization(context.Context, MachineAuthority, int64, string, domain.MaterializationFreeze) error
	FailMaterialization(context.Context, MachineAuthority, int64, string, string) error
	AuthorizeCheckpointKey(context.Context, MachineAuthority, string) error
	ResolveRestoreKeyEnvelope(context.Context, MachineAuthority, int64) (KeyEnvelope, string, error)
}

type KeyEnvelope struct {
	Ciphertext string `json:"ciphertext"`
	EDEK       string `json:"edek"`
	KeyID      string `json:"keyId"`
}

type KeyWrapper interface {
	Wrap(context.Context, []byte) (KeyEnvelope, error)
	Unwrap(context.Context, KeyEnvelope) ([]byte, error)
}

type MachineAuthority struct {
	WorkerID   int64
	InstanceID string
	HostBootID string
	AttemptID  int64
	LeaseToken string
	Generation int64
	OwnerEpoch int64
}

type CheckpointKeyMaterialization struct {
	Algorithm      string      `json:"algorithm"`
	PlainKeyBase64 string      `json:"plainKeyBase64"`
	Envelope       KeyEnvelope `json:"envelope"`
	AADIdentity    string      `json:"aadIdentity"`
	// Machine restore response only; never included in public checkpoint views.
	InteractionRestoreInput *wcp.RestoreInput `json:"interactionRestoreInput,omitempty"`
}

type Service struct {
	repository Repository
	keys       KeyWrapper
}

func New(repository Repository) *Service { return &Service{repository: repository} }

func (s *Service) SetKeyWrapper(keys KeyWrapper) { s.keys = keys }

func (s *Service) IssueCheckpointKey(ctx context.Context, authority MachineAuthority, checkpointIdentity string) (CheckpointKeyMaterialization, error) {
	checkpointIdentity = strings.TrimSpace(checkpointIdentity)
	if s.keys == nil || len(checkpointIdentity) != 64 {
		return CheckpointKeyMaterialization{}, domain.ErrContinuationIdentity
	}
	if err := s.repository.AuthorizeCheckpointKey(ctx, authority, checkpointIdentity); err != nil {
		return CheckpointKeyMaterialization{}, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return CheckpointKeyMaterialization{}, err
	}
	envelope, err := s.keys.Wrap(ctx, key)
	if err != nil {
		return CheckpointKeyMaterialization{}, err
	}
	return CheckpointKeyMaterialization{Algorithm: "AES-256-GCM-CHUNKED", PlainKeyBase64: base64.StdEncoding.EncodeToString(key), Envelope: envelope, AADIdentity: checkpointIdentity}, nil
}

func (s *Service) ResolveRestoreKey(ctx context.Context, authority MachineAuthority, restoreRunID int64) (CheckpointKeyMaterialization, error) {
	if s.keys == nil || restoreRunID <= 0 {
		return CheckpointKeyMaterialization{}, domain.ErrContinuationIdentity
	}
	envelope, identity, err := s.repository.ResolveRestoreKeyEnvelope(ctx, authority, restoreRunID)
	if err != nil {
		return CheckpointKeyMaterialization{}, err
	}
	key, err := s.keys.Unwrap(ctx, envelope)
	if err != nil || len(key) != 32 {
		return CheckpointKeyMaterialization{}, domain.ErrContinuationIdentity
	}
	defer clear(key)
	var input *wcp.RestoreInput
	if resolver, ok := s.repository.(InteractionRestoreMaterializer); ok {
		input, err = resolver.MaterializeInteractionRestoreInput(ctx, authority, restoreRunID)
		if err != nil {
			return CheckpointKeyMaterialization{}, err
		}
		if input != nil && (input.Manifest.Validate() != nil || input.Manifest.CheckpointIdentity != identity) {
			return CheckpointKeyMaterialization{}, domain.ErrInteractionSuspend
		}
	}
	return CheckpointKeyMaterialization{Algorithm: "AES-256-GCM-CHUNKED", PlainKeyBase64: base64.StdEncoding.EncodeToString(key), AADIdentity: identity, InteractionRestoreInput: input}, nil
}

// The concrete Store implements this extension for the existing machine restore
// key endpoint. RESULT returns nil; suspend must return authenticated frozen input
// or an error, never silently fall back to a result restore.
type InteractionRestoreMaterializer interface {
	MaterializeInteractionRestoreInput(context.Context, MachineAuthority, int64) (*wcp.RestoreInput, error)
}

func (s *Service) CreateNextTurn(ctx context.Context, request CreateNextTurnRequest) (domain.Turn, bool, error) {
	request.Instruction = strings.TrimSpace(request.Instruction)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.TaskID == 0 || request.TenantID == 0 || request.CreatorUserID <= 0 || request.SourceCheckpointID == 0 || request.ExpectedCurrentTurnID == 0 || request.IdempotencyKey == "" || len(request.IdempotencyKey) > 128 {
		return domain.Turn{}, false, domain.ErrContinuationInstruction
	}
	if err := domain.ValidateInstruction(request.Instruction); err != nil {
		return domain.Turn{}, false, err
	}
	return s.repository.CreateNextTurn(ctx, request)
}

func (s *Service) ApproveManualReviewContinuation(ctx context.Context, request ApproveRequest) (domain.Checkpoint, error) {
	request.Reason = strings.TrimSpace(request.Reason)
	if request.CheckpointID == 0 || request.TenantID == 0 || request.Actor <= 0 || request.Reason == "" || len(request.Reason) > 2000 {
		return domain.Checkpoint{}, domain.ErrContinuationInstruction
	}
	return s.repository.Approve(ctx, request)
}

func (s *Service) ListTurns(ctx context.Context, tenantID int64, taskID int64) ([]domain.Turn, error) {
	return s.repository.ListTurns(ctx, tenantID, taskID)
}

func (s *Service) GetTurn(ctx context.Context, tenantID int64, turnID int64) (domain.Turn, error) {
	return s.repository.GetTurn(ctx, tenantID, turnID)
}

func (s *Service) GetCheckpoint(ctx context.Context, tenantID int64, checkpointID int64) (domain.Checkpoint, error) {
	return s.repository.GetCheckpoint(ctx, tenantID, checkpointID)
}

func (s *Service) CreateMaterialization(ctx context.Context, request CreateMaterializationRequest) (domain.MaterializationView, bool, error) {
	request.ActorServiceCode = strings.TrimSpace(request.ActorServiceCode)
	request.Kind = strings.TrimSpace(request.Kind)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.TaskID == 0 || request.TaskTurnID == 0 || request.TenantID == 0 || (request.Actor <= 0) == (request.ActorServiceCode == "") || request.IdempotencyKey == "" || len(request.IdempotencyKey) > 128 {
		return domain.MaterializationView{}, false, domain.ErrMaterializationInvalid
	}
	if request.Kind == domain.MaterializationBundle {
		request.Kind = domain.MaterializationGitBundle
	}
	if request.Kind != domain.MaterializationDeclaredOutput && request.Kind != domain.MaterializationZIP && request.Kind != domain.MaterializationTarGZ && request.Kind != domain.MaterializationPatch && request.Kind != domain.MaterializationGitBundle {
		return domain.MaterializationView{}, false, domain.ErrMaterializationInvalid
	}
	needsOutput := request.Kind == domain.MaterializationDeclaredOutput || request.Kind == domain.MaterializationZIP || request.Kind == domain.MaterializationTarGZ
	if needsOutput != (request.OutputID != 0) {
		return domain.MaterializationView{}, false, domain.ErrMaterializationInvalid
	}
	return s.repository.CreateMaterialization(ctx, request)
}

func (s *Service) GetMaterialization(ctx context.Context, tenantID int64, runID int64) (domain.MaterializationView, error) {
	return s.repository.GetMaterialization(ctx, tenantID, runID)
}

func (s *Service) ListMaterializations(ctx context.Context, tenantID int64, taskID int64) ([]domain.MaterializationView, error) {
	return s.repository.ListMaterializations(ctx, tenantID, taskID)
}

func (s *Service) RetryMaterialization(ctx context.Context, tenantID int64, actor int64, runID int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if tenantID == 0 || actor <= 0 || runID == 0 || reason == "" || len(reason) > 2000 {
		return domain.ErrMaterializationInvalid
	}
	return s.repository.RetryMaterialization(ctx, tenantID, actor, runID, reason)
}

func (s *Service) ClaimMaterialization(ctx context.Context, authority MachineAuthority, runID int64) (domain.MaterializationClaim, error) {
	claim, err := s.repository.ClaimMaterialization(ctx, authority, runID)
	if err != nil {
		return claim, err
	}
	if s.keys == nil {
		return domain.MaterializationClaim{}, domain.ErrContinuationIdentity
	}
	envelope := KeyEnvelope{}
	if err := json.Unmarshal([]byte(claim.Restore.WrappedDataKey), &envelope); err != nil {
		return domain.MaterializationClaim{}, domain.ErrContinuationIdentity
	}
	key, err := s.keys.Unwrap(ctx, envelope)
	if err != nil || len(key) != 32 {
		return domain.MaterializationClaim{}, domain.ErrContinuationIdentity
	}
	claim.Key = base64.StdEncoding.EncodeToString(key)
	claim.Restore.WrappedDataKey = ""
	return claim, nil
}

func (s *Service) FreezeMaterialization(ctx context.Context, authority MachineAuthority, runID int64, lease string, result domain.MaterializationFreeze) error {
	return s.repository.FreezeMaterialization(ctx, authority, runID, strings.TrimSpace(lease), result)
}

func (s *Service) FailMaterialization(ctx context.Context, authority MachineAuthority, runID int64, lease string, reason string) error {
	return s.repository.FailMaterialization(ctx, authority, runID, strings.TrimSpace(lease), strings.TrimSpace(reason))
}
