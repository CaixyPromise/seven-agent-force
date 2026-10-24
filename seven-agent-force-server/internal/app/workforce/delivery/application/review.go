package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

type ReviewWork struct {
	OperationID                                                        int64
	PublicationID                                                      int64
	TenantID                                                           int64
	TaskID                                                             int64
	TaskTurnID                                                         int64
	RequestID                                                          int64
	ProjectID                                                          int64
	ProviderType                                                       string
	Transport                                                          string
	Host                                                               string
	RepositoryPath, WebRepositoryURL                                   string
	Port                                                               int
	EgressPolicy                                                       json.RawMessage
	TLSPolicy                                                          json.RawMessage
	HeadBranch, BaseBranch, HeadCommit, FrozenBaseCommit, MarkerDigest string
	CredentialVersion                                                  string
	AuthorizationDigest, ConnectionConfigDigest                        string
	CredentialEnvelope                                                 EncryptedCredentialEnvelope
}

type ReviewPublication struct {
	ProviderReviewID string
	ProviderNumber   string
	WebURL           string
	ObservedBase     string
}

type ReviewRepository interface {
	ClaimReviewOperation(context.Context) (ReviewWork, bool, error)
	ResolveReviewCredential(context.Context, ReviewWork) (DeliveryCredential, error)
	CompleteReviewOperation(context.Context, ReviewWork, ReviewPublication) error
	FailReviewOperation(context.Context, ReviewWork, string, bool) error
}

type ReviewProvider interface {
	PublishDraft(context.Context, ReviewWork, DeliveryCredential) (ReviewPublication, error)
}

type ReviewReadbackProvider interface {
	ReconcileDraft(context.Context, ReviewWork, DeliveryCredential) (ReviewPublication, bool, error)
}

type ReviewReconciliationRepository interface {
	LoadReviewReconciliation(context.Context, int64, int64) (ReviewWork, error)
	CompleteReviewReconciliation(context.Context, ReviewWork, ReviewPublication, int64) error
	RecordReviewReconciliation(context.Context, ReviewWork, int64, string) error
}

type ReviewReconciliationResult struct {
	OperationID int64  `json:"operationId,string"`
	Status      string `json:"status"`
	ReasonCode  string `json:"reasonCode,omitempty"`
}

type ReviewCoordinator struct {
	repository ReviewRepository
	provider   ReviewProvider
}

func NewReviewCoordinator(repository ReviewRepository, provider ReviewProvider) *ReviewCoordinator {
	return &ReviewCoordinator{repository: repository, provider: provider}
}

func (c *ReviewCoordinator) Reconcile(ctx context.Context, limit int) (int, error) {
	if c == nil || c.repository == nil || c.provider == nil {
		return 0, nil
	}
	if limit < 1 || limit > 64 {
		limit = 16
	}
	completed := 0
	for index := 0; index < limit; index++ {
		work, ok, err := c.repository.ClaimReviewOperation(ctx)
		if err != nil {
			return completed, err
		}
		if !ok {
			return completed, nil
		}
		credential := DeliveryCredential{}
		if work.ProviderType != "LOCAL_TEST" {
			credential, err = c.repository.ResolveReviewCredential(ctx, work)
			if err != nil {
				_ = c.repository.FailReviewOperation(ctx, work, "REVIEW_CREDENTIAL_UNAVAILABLE", false)
				continue
			}
		}
		publication, err := c.provider.PublishDraft(ctx, work, credential)
		if err != nil {
			reason, retryable := classifyReviewError(err)
			_ = c.repository.FailReviewOperation(ctx, work, reason, retryable)
			continue
		}
		if err = c.repository.CompleteReviewOperation(ctx, work, publication); err != nil {
			return completed, err
		}
		completed++
	}
	return completed, nil
}

func classifyReviewError(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	reason := strings.TrimSpace(err.Error())
	if reason == "REVIEW_REMOTE_UNAVAILABLE" || reason == "REVIEW_RESPONSE_LOST" || reason == "REVIEW_RATE_LIMITED" {
		return reason, true
	}
	if len(reason) == 0 || len(reason) > 96 || !strings.HasPrefix(reason, "REVIEW_") {
		return "REVIEW_PUBLICATION_FAILED", false
	}
	return reason, false
}

var ErrReviewPublicationConflict = errors.New("REVIEW_PUBLICATION_CONFLICT")

func (c *ReviewCoordinator) ReconcileReview(ctx context.Context, tenantID, actorID, operationID int64) (ReviewReconciliationResult, error) {
	if c == nil || tenantID <= 0 || actorID <= 0 || operationID <= 0 {
		return ReviewReconciliationResult{}, ErrReconciliationNotAllowed
	}
	repository, ok := c.repository.(ReviewReconciliationRepository)
	provider, providerOK := c.provider.(ReviewReadbackProvider)
	if !ok || !providerOK {
		return ReviewReconciliationResult{}, ErrReconciliationNotAllowed
	}
	work, err := repository.LoadReviewReconciliation(ctx, tenantID, operationID)
	if err != nil {
		return ReviewReconciliationResult{}, err
	}
	credential := DeliveryCredential{}
	if work.ProviderType != "LOCAL_TEST" {
		credential, err = c.repository.ResolveReviewCredential(ctx, work)
		if err != nil {
			return ReviewReconciliationResult{}, ErrReconciliationNotAllowed
		}
	}
	publication, found, err := provider.ReconcileDraft(ctx, work, credential)
	if err != nil {
		_ = repository.RecordReviewReconciliation(ctx, work, actorID, "DELIVERY_RECONCILIATION_INCONCLUSIVE")
		return ReviewReconciliationResult{}, ErrReconciliationInconclusive
	}
	if !found {
		_ = repository.RecordReviewReconciliation(ctx, work, actorID, "DELIVERY_RECONCILIATION_INCONCLUSIVE")
		return ReviewReconciliationResult{OperationID: operationID, Status: "MANUAL_REVIEW", ReasonCode: "DELIVERY_RECONCILIATION_INCONCLUSIVE"}, ErrReconciliationInconclusive
	}
	if publication.ObservedBase != work.FrozenBaseCommit {
		_ = repository.RecordReviewReconciliation(ctx, work, actorID, "DELIVERY_RECONCILIATION_EVIDENCE_MISMATCH")
		return ReviewReconciliationResult{}, ErrReconciliationEvidenceMismatch
	}
	if err = repository.CompleteReviewReconciliation(ctx, work, publication, actorID); err != nil {
		return ReviewReconciliationResult{}, err
	}
	return ReviewReconciliationResult{OperationID: operationID, Status: "SUCCEEDED"}, nil
}
