package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/eventlog"
	tasknotification "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/facade"
)

func (s *Store) ClaimReviewOperation(ctx context.Context) (application.ReviewWork, bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return application.ReviewWork{}, false, err
	}
	defer tx.Rollback(ctx)
	var work application.ReviewWork
	var attemptCount int
	err = tx.QueryRow(ctx, `SELECT o.id,o.tenant_id,r.id,r.task_id,r.task_turn_id,r.project_id,r.destination_branch,r.base_branch,r.delivery_commit,r.expected_base_commit,o.attempt_count,o.authorization_version,o.authorization_digest,r.target_config_digest
		FROM workforce_delivery_operations o
		JOIN workforce_delivery_requests r ON r.id=o.delivery_request_id
		WHERE o.operation_kind='CREATE_PULL_REQUEST' AND o.status IN ('READY','RETRY_WAIT') AND (o.next_retry_at IS NULL OR o.next_retry_at<=CURRENT_TIMESTAMP)
		ORDER BY o.created_at FOR UPDATE OF o SKIP LOCKED LIMIT 1`).Scan(&work.OperationID, &work.TenantID, &work.RequestID, &work.TaskID, &work.TaskTurnID, &work.ProjectID, &work.HeadBranch, &work.BaseBranch, &work.HeadCommit, &work.FrozenBaseCommit, &attemptCount, &work.CredentialVersion, &work.AuthorizationDigest, &work.ConnectionConfigDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ReviewWork{}, false, nil
	}
	if err != nil {
		return application.ReviewWork{}, false, err
	}
	plan, planErr := s.loadDeliveryRepositoryPlan(ctx, work.TenantID, work.TaskID, work.TaskTurnID, work.RequestID)
	if planErr != nil {
		_, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status='MANUAL_REVIEW',reason_code='REVIEW_ACCESS_NOT_VERIFIED',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, work.OperationID)
		if err == nil {
			err = s.recordReviewAttention(ctx, tx, work, "REVIEW_ACCESS_NOT_VERIFIED")
		}
		if err == nil {
			err = tx.Commit(ctx)
		}
		return application.ReviewWork{}, false, err
	}
	if applyFrozenReviewPlan(&work, plan) != nil {
		_, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status='MANUAL_REVIEW',reason_code='REVIEW_AUTHORIZATION_STALE',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, work.OperationID)
		if err == nil {
			err = s.recordReviewAttention(ctx, tx, work, "REVIEW_AUTHORIZATION_STALE")
		}
		if err == nil {
			err = tx.Commit(ctx)
		}
		return application.ReviewWork{}, false, err
	}
	if attemptCount >= 3 {
		_, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status='MANUAL_REVIEW',reason_code='REVIEW_RETRY_EXHAUSTED',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, work.OperationID)
		if err == nil {
			err = s.recordReviewAttention(ctx, tx, work, "REVIEW_RETRY_EXHAUSTED")
		}
		if err == nil {
			err = tx.Commit(ctx)
		}
		return application.ReviewWork{}, false, err
	}
	marker := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%d\x00%s", work.TenantID, work.RequestID, work.HeadCommit)))
	work.MarkerDigest = hex.EncodeToString(marker[:])
	err = tx.QueryRow(ctx, `SELECT id FROM workforce_pull_request_publications WHERE delivery_operation_id=$1`, work.OperationID).Scan(&work.PublicationID)
	if errors.Is(err, pgx.ErrNoRows) {
		work.PublicationID = s.idGen.NextID()
		_, err = tx.Exec(ctx, `INSERT INTO workforce_pull_request_publications (id,tenant_id,task_id,task_turn_id,delivery_request_id,delivery_operation_id,project_id,provider_type,status,head_branch,base_branch,head_commit,frozen_base_commit,marker_digest) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'PENDING',$9,$10,$11,$12,$13)`, work.PublicationID, work.TenantID, work.TaskID, work.TaskTurnID, work.RequestID, work.OperationID, work.ProjectID, work.ProviderType, work.HeadBranch, work.BaseBranch, work.HeadCommit, work.FrozenBaseCommit, work.MarkerDigest)
	}
	if err != nil {
		return application.ReviewWork{}, false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status='RUNNING',attempt_count=attempt_count+1,reason_code='',next_retry_at=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, work.OperationID); err != nil {
		return application.ReviewWork{}, false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_pull_request_publications SET status='RUNNING',updated_at=CURRENT_TIMESTAMP WHERE id=$1`, work.PublicationID); err != nil {
		return application.ReviewWork{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return application.ReviewWork{}, false, err
	}
	return work, true, nil
}

func applyFrozenReviewPlan(work *application.ReviewWork, plan frozenRepositoryPlan) error {
	if work == nil || work.ProjectID != plan.ProjectID || work.ConnectionConfigDigest != plan.TargetConfigDigest || work.CredentialVersion != plan.credentialVersion() || work.AuthorizationDigest != plan.ReviewAuthorizationDigest {
		return domain.ErrIdentityConflict
	}
	target, err := plan.target(work.TenantID)
	if err != nil {
		return err
	}
	work.ProviderType = target.ProviderType
	work.Transport = target.Transport
	work.Host = target.Host
	work.Port = target.Port
	work.RepositoryPath = target.RepositoryPath
	work.WebRepositoryURL = target.WebRepositoryURL
	work.EgressPolicy = target.Egress
	work.TLSPolicy = target.TLSPolicy
	work.CredentialEnvelope = plan.Credential
	return nil
}

func (s *Store) ResolveReviewCredential(ctx context.Context, work application.ReviewWork) (application.DeliveryCredential, error) {
	if work.CredentialEnvelope.Version <= 0 || work.CredentialVersion != fmt.Sprint(work.CredentialEnvelope.Version) {
		return application.DeliveryCredential{}, domain.ErrIdentityConflict
	}
	return s.resolveFrozenCredential(ctx, work.CredentialEnvelope)
}

func (s *Store) CompleteReviewOperation(ctx context.Context, work application.ReviewWork, publication application.ReviewPublication) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(ctx, `UPDATE workforce_pull_request_publications SET provider_review_id=$2,provider_number=$3,status='SUCCEEDED',observed_base_commit=$4,web_url=$5,reason_code='',updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND delivery_operation_id=$6 AND status='RUNNING'`, work.PublicationID, publication.ProviderReviewID, publication.ProviderNumber, publication.ObservedBase, publication.WebURL, work.OperationID)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return application.ErrReviewPublicationConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status='SUCCEEDED',reason_code='',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='RUNNING'`, work.OperationID); err != nil {
		return err
	}
	if err = s.projectDeliveryRequest(ctx, tx, work.RequestID); err != nil {
		return err
	}
	if err = s.recordDeliveryCompleted(ctx, tx, work.RequestID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) FailReviewOperation(ctx context.Context, work application.ReviewWork, reason string, retryable bool) error {
	status := "MANUAL_REVIEW"
	next := time.Time{}
	if retryable {
		status = "RETRY_WAIT"
		next = time.Now().UTC().Add(10 * time.Second)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status=$2,reason_code=$3,next_retry_at=CASE WHEN $2='RETRY_WAIT' THEN $4::timestamptz ELSE NULL END,finished_at=CASE WHEN $2='MANUAL_REVIEW' THEN CURRENT_TIMESTAMP ELSE NULL END,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='RUNNING'`, work.OperationID, status, reason, next); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_pull_request_publications SET status=$2,reason_code=$3,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, work.PublicationID, status, reason); err != nil {
		return err
	}
	if status == "MANUAL_REVIEW" {
		normalizedReason := "REVIEW_RETRY_EXHAUSTED"
		if reason == "REVIEW_ACCESS_NOT_VERIFIED" || reason == "REVIEW_AUTHORIZATION_STALE" {
			normalizedReason = reason
		}
		if err = s.recordReviewAttention(ctx, tx, work, normalizedReason); err != nil {
			return err
		}
	}
	if err = s.projectDeliveryRequest(ctx, tx, work.RequestID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) recordReviewAttention(ctx context.Context, tx pgx.Tx, work application.ReviewWork, reason string) error {
	if s.notificationRecorder == nil {
		return nil
	}
	occurrence, err := eventlog.AppendWithSequence(ctx, tx, work.TaskID, 0, "review.attention_required", map[string]any{"reviewOperationId": work.OperationID, "reasonCode": reason})
	if err != nil {
		return err
	}
	_, err = s.notificationRecorder.RecordTransition(ctx, tx, tasknotification.TaskNotificationTransition{TenantID: work.TenantID, TaskID: work.TaskID, TaskTurnID: work.TaskTurnID, SourceType: tasknotification.SourceReviewOperation, SourceID: work.OperationID, SourceOccurrence: occurrence, PreviousStatus: "RUNNING", CurrentStatus: "MANUAL_REVIEW", ReasonCode: reason, Recoverability: tasknotification.RecoverabilityUserRecoverable, OccurredAt: time.Now().UTC()})
	return err
}

func (s *Store) LoadReviewReconciliation(ctx context.Context, tenantID, operationID int64) (application.ReviewWork, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return application.ReviewWork{}, err
	}
	defer tx.Rollback(ctx)
	var work application.ReviewWork
	err = tx.QueryRow(ctx, `SELECT o.id,p.id,o.tenant_id,r.id,r.task_id,r.task_turn_id,r.project_id,r.destination_branch,r.base_branch,r.delivery_commit,r.expected_base_commit,p.marker_digest,o.authorization_version,o.authorization_digest,r.target_config_digest
		FROM workforce_delivery_operations o
		JOIN workforce_delivery_requests r ON r.id=o.delivery_request_id AND r.tenant_id=o.tenant_id
		JOIN workforce_pull_request_publications p ON p.delivery_operation_id=o.id AND p.tenant_id=o.tenant_id
		WHERE o.id=$1 AND o.tenant_id=$2 AND o.operation_kind='CREATE_PULL_REQUEST' AND o.status='MANUAL_REVIEW'`, operationID, tenantID).Scan(
		&work.OperationID, &work.PublicationID, &work.TenantID, &work.RequestID, &work.TaskID, &work.TaskTurnID, &work.ProjectID,
		&work.HeadBranch, &work.BaseBranch, &work.HeadCommit, &work.FrozenBaseCommit, &work.MarkerDigest, &work.CredentialVersion, &work.AuthorizationDigest, &work.ConnectionConfigDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ReviewWork{}, application.ErrReconciliationNotAllowed
	}
	if err != nil {
		return application.ReviewWork{}, err
	}
	plan, err := s.loadDeliveryRepositoryPlan(ctx, work.TenantID, work.TaskID, work.TaskTurnID, work.RequestID)
	if err != nil || applyFrozenReviewPlan(&work, plan) != nil {
		return application.ReviewWork{}, application.ErrReconciliationNotAllowed
	}
	if err = tx.Commit(ctx); err != nil {
		return application.ReviewWork{}, err
	}
	return work, nil
}

func (s *Store) CompleteReviewReconciliation(ctx context.Context, work application.ReviewWork, publication application.ReviewPublication, actorID int64) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(ctx, `UPDATE workforce_pull_request_publications SET provider_review_id=$2,provider_number=$3,status='SUCCEEDED',observed_base_commit=$4,web_url=$5,reason_code='',updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND delivery_operation_id=$6 AND status='MANUAL_REVIEW'`, work.PublicationID, publication.ProviderReviewID, publication.ProviderNumber, publication.ObservedBase, publication.WebURL, work.OperationID)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return application.ErrReviewPublicationConflict
	}
	command, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status='SUCCEEDED',reason_code='',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND tenant_id=$2 AND status='MANUAL_REVIEW'`, work.OperationID, work.TenantID)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return application.ErrReviewPublicationConflict
	}
	_, err = eventlog.AppendWithSequence(ctx, tx, work.TaskID, 0, "review.reconciled", map[string]any{"reviewOperationId": work.OperationID, "actorUserId": actorID, "result": "SUCCEEDED", "mode": "READBACK_ONLY"})
	if err != nil {
		return err
	}
	if err = s.projectDeliveryRequest(ctx, tx, work.RequestID); err != nil {
		return err
	}
	if err = s.recordDeliveryCompleted(ctx, tx, work.RequestID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RecordReviewReconciliation(ctx context.Context, work application.ReviewWork, actorID int64, reason string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = eventlog.AppendWithSequence(ctx, tx, work.TaskID, 0, "review.reconcile_inconclusive", map[string]any{"reviewOperationId": work.OperationID, "actorUserId": actorID, "reasonCode": reason, "mode": "READBACK_ONLY"})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
