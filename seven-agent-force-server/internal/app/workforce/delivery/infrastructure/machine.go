package infrastructure

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
)

func (s *Store) Claim(ctx context.Context, a application.MachineAuthority, attemptID int64) (application.DeliveryClaim, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return application.DeliveryClaim{}, err
	}
	defer tx.Rollback(ctx)
	var c application.DeliveryClaim
	var worker int64
	var status, instance, boot, wrapped string
	var actionJSON []byte
	err = tx.QueryRow(ctx, `SELECT a.id,a.request_id,r.task_id,r.task_turn_id,r.action_type,r.action_json,r.processor_release_id,r.processor_digest,a.attempt_number,a.generation,a.status,a.worker_id,a.worker_instance_id,a.host_boot_id,r.checkpoint_id,c.checkpoint_identity_sha256,c.repository_bundle_object_key,c.repository_bundle_content_sha256,c.repository_bundle_object_sha256,c.repository_bundle_size_bytes,c.wrapped_data_key,r.result_commit,r.result_tree,r.selection_digest,r.expected_base_commit,r.delivery_commit,to_char(r.commit_timestamp AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),r.base_branch,r.destination_branch,a.reconciliation_only FROM workforce_delivery_attempts a JOIN workforce_delivery_requests r ON r.id=a.request_id JOIN workforce_continuation_checkpoints c ON c.id=r.checkpoint_id WHERE a.id=$1 AND c.format_version=2 FOR UPDATE OF a,r`, attemptID).Scan(&c.AttemptID, &c.RequestID, &c.TaskID, &c.TaskTurnID, &c.ActionType, &actionJSON, &c.ProcessorReleaseID, &c.ProcessorDigest, &c.AttemptNumber, &c.Generation, &status, &worker, &instance, &boot, &c.CheckpointID, &c.CheckpointIdentity, &c.RepositoryBundleObjectKey, &c.RepositoryBundleContentSHA256, &c.RepositoryBundleObjectSHA256, &c.RepositoryBundleSizeBytes, &wrapped, &c.ResultCommit, &c.ResultTree, &c.SelectionDigest, &c.ExpectedBaseCommit, &c.DeliveryCommit, &c.CommitTimestamp, &c.BaseBranch, &c.DestinationBranch, &c.ReconciliationOnly)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, application.ErrNotFound
	}
	if err != nil {
		return c, err
	}
	if err = tx.QueryRow(ctx, `SELECT archive_object_key FROM workforce_result_processor_releases WHERE id=$1 AND archive_sha256=$2`, c.ProcessorReleaseID, c.ProcessorDigest).Scan(&c.ProcessorArchiveObjectKey); err != nil {
		return c, domain.ErrIdentityConflict
	}
	c.HarnessImageDigest, err = deliveryHarnessDigestTx(ctx, tx, c.RequestID)
	if err != nil || strings.TrimSpace(c.HarnessImageDigest) == "" {
		return c, domain.ErrIdentityConflict
	}
	if json.Unmarshal(actionJSON, &c.Action) != nil || c.Action.Type != c.ActionType {
		return c, domain.ErrIdentityConflict
	}
	if status != "PENDING" && status != "RETRY_WAIT" {
		return c, domain.ErrIdentityConflict
	}
	if worker != a.WorkerID || instance != a.InstanceID || boot != a.HostBootID || c.Generation != a.Generation {
		return c, domain.ErrIdentityConflict
	}
	var current string
	if err = tx.QueryRow(ctx, `SELECT current_instance_id FROM workforce_workers WHERE id=$1 AND decommissioned_at IS NULL`, worker).Scan(&current); err != nil || current != instance {
		return c, domain.ErrIdentityConflict
	}
	var projectID, tenant int64
	var frozenTargetDigest string
	if err = tx.QueryRow(ctx, `SELECT project_id,tenant_id,target_config_digest FROM workforce_delivery_requests WHERE id=$1`, c.RequestID).Scan(&projectID, &tenant, &frozenTargetDigest); err != nil {
		return c, err
	}
	plan, err := s.loadDeliveryRepositoryPlan(ctx, tenant, c.TaskID, c.TaskTurnID, c.RequestID)
	if err != nil {
		return c, err
	}
	if plan.ProjectID != projectID || plan.TargetConfigDigest != frozenTargetDigest {
		return c, domain.ErrIdentityConflict
	}
	c.Target, err = plan.target(tenant)
	if err != nil {
		return c, err
	}
	lease := randomToken()
	expires := time.Now().UTC().Add(5 * time.Minute)
	phase, requestState := "PREPARING", "RUNNING"
	if c.ReconciliationOnly {
		phase, requestState = "RECONCILING", "CONFIRMING"
	}
	_, err = tx.Exec(ctx, `UPDATE workforce_delivery_attempts SET status='RUNNING',phase=$2,lease_sha256=$3,lease_expires_at=$4,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, attemptID, phase, secretHash(lease), expires)
	if err != nil {
		return c, err
	}
	_, err = tx.Exec(ctx, `UPDATE workforce_delivery_requests SET status=$2,version=version+1,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, c.RequestID, requestState)
	if err != nil {
		return c, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status=$2,updated_at=CURRENT_TIMESTAMP WHERE delivery_request_id=$1 AND operation_kind='PUSH_BRANCH'`, c.RequestID, requestState); err != nil {
		return c, err
	}
	if err = s.projectDeliveryRequest(ctx, tx, c.RequestID); err != nil {
		return c, err
	}
	if err = tx.Commit(ctx); err != nil {
		return c, err
	}
	c.LeaseToken = lease
	c.LeaseExpiresAt = expires.Format(time.RFC3339Nano)
	c.WrappedDataKey = wrapped
	return c, nil
}

// RecordReconciliation accepts only the observation from a read-only
// reconciliation attempt. No push authorization is issued on this path, so an
// uncertain remote side effect cannot be repeated blindly.
func (s *Store) RecordReconciliation(ctx context.Context, a application.MachineAuthority, attemptID int64, lease string, result application.DeliveryResult) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var requestID, worker int64
	var expectedBase, delivery, stored, instance, boot string
	var generation int64
	var reconciliationOnly bool
	err = tx.QueryRow(ctx, `SELECT a.request_id,r.expected_base_commit,r.delivery_commit,a.lease_sha256,a.worker_id,a.worker_instance_id,a.host_boot_id,a.generation,a.reconciliation_only FROM workforce_delivery_attempts a JOIN workforce_delivery_requests r ON r.id=a.request_id WHERE a.id=$1 AND a.status='RUNNING' FOR UPDATE OF a,r`, attemptID).Scan(&requestID, &expectedBase, &delivery, &stored, &worker, &instance, &boot, &generation, &reconciliationOnly)
	if err != nil {
		return err
	}
	if !reconciliationOnly || result.PushStarted || stored != secretHash(lease) || worker != a.WorkerID || instance != a.InstanceID || boot != a.HostBootID || generation != a.Generation {
		return domain.ErrIdentityConflict
	}
	decision := domain.ReconcileAfter(expectedBase, delivery, result.RemoteAfter, result.Readable)
	decision, attemptStatus, retryable := reconciliationTransition(decision, 0)
	nextRetry := time.Time{}
	if retryable {
		epochAttemptNumber, countErr := deliveryRetryEpochAttemptCount(ctx, tx, requestID)
		if countErr != nil {
			return countErr
		}
		decision, attemptStatus, retryable = reconciliationTransition(decision, epochAttemptNumber)
		if retryable {
			nextRetry = retryAt(epochAttemptNumber)
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_attempts SET status=$2::text,phase='VERIFIED',remote_after=$3,reason_code=$4,next_retry_at=CASE WHEN $2='RETRY_WAIT' THEN $5::timestamptz ELSE NULL END,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, attemptID, attemptStatus, result.RemoteAfter, decision.ReasonCode, nextRetry); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_requests SET status=$2::text,reason_code=$3,finished_at=CASE WHEN $2 IN ('SUCCEEDED','FAILED','MANUAL_REVIEW') THEN CURRENT_TIMESTAMP ELSE NULL END,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, requestID, decision.Status, decision.ReasonCode); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status=$2::text,reason_code=$3,next_retry_at=CASE WHEN $2='RETRY_WAIT' THEN $4::timestamptz ELSE NULL END,finished_at=CASE WHEN $2 IN ('SUCCEEDED','FAILED','MANUAL_REVIEW') THEN CURRENT_TIMESTAMP ELSE NULL END,updated_at=CURRENT_TIMESTAMP WHERE delivery_request_id=$1 AND operation_kind='PUSH_BRANCH'`, requestID, decision.Status, decision.ReasonCode, nextRetry); err != nil {
		return err
	}
	if decision.Status == "SUCCEEDED" {
		if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status='READY',reason_code='',next_retry_at=NULL,updated_at=CURRENT_TIMESTAMP WHERE delivery_request_id=$1 AND operation_kind='CREATE_PULL_REQUEST' AND status='WAITING_DEPENDENCY'`, requestID); err != nil {
			return err
		}
	}
	if err = s.recordDeliveryAttention(ctx, tx, requestID, decision.Status, decision.ReasonCode); err != nil {
		return err
	}
	if decision.Status == "SUCCEEDED" {
		if err = s.recordDeliveryCompleted(ctx, tx, requestID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// reconciliationTransition keeps the immutable readback Attempt in the same
// state consumed by ReconcileRetries. A separate RECONCILED terminal state
// loses the retry timestamp and strands the parent Request indefinitely.
func reconciliationTransition(decision domain.RemoteDecision, epochAttemptNumber int) (domain.RemoteDecision, string, bool) {
	if decision.Status != "RETRY_WAIT" {
		return decision, decision.Status, false
	}
	if epochAttemptNumber >= 3 {
		decision.Status = "FAILED"
		decision.ReasonCode = "DELIVERY_RETRY_EXHAUSTED"
		return decision, "FAILED", false
	}
	return decision, "RETRY_WAIT", true
}

func (s *Store) PreparePush(ctx context.Context, a application.MachineAuthority, attemptID int64, lease string, commit string) (application.PrepareResult, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return application.PrepareResult{}, err
	}
	defer tx.Rollback(ctx)
	var requestID, worker, tenantID, taskID, taskTurnID, projectID int64
	var expected, status, requestStatus, stored, instance, boot, targetDigest, currentTarget string
	var generation int64
	var leaseExpiry time.Time
	err = tx.QueryRow(ctx, `SELECT a.request_id,r.tenant_id,r.task_id,r.task_turn_id,r.project_id,r.delivery_commit,a.status,r.status,a.lease_sha256,a.lease_expires_at,a.worker_id,a.worker_instance_id,a.host_boot_id,a.generation,r.target_config_digest FROM workforce_delivery_attempts a JOIN workforce_delivery_requests r ON r.id=a.request_id WHERE a.id=$1 FOR UPDATE OF a,r`, attemptID).Scan(&requestID, &tenantID, &taskID, &taskTurnID, &projectID, &expected, &status, &requestStatus, &stored, &leaseExpiry, &worker, &instance, &boot, &generation, &targetDigest)
	if err != nil {
		return application.PrepareResult{}, err
	}
	plan, err := s.loadDeliveryRepositoryPlan(ctx, tenantID, taskID, taskTurnID, requestID)
	if err != nil {
		return application.PrepareResult{}, err
	}
	currentTarget = plan.TargetConfigDigest
	if commit != expected || status != "RUNNING" || requestStatus != "RUNNING" || stored != secretHash(lease) || !leaseExpiry.After(time.Now()) || worker != a.WorkerID || instance != a.InstanceID || boot != a.HostBootID || generation != a.Generation || projectID != plan.ProjectID || targetDigest != currentTarget {
		return application.PrepareResult{}, domain.ErrIdentityConflict
	}
	token := randomToken()
	expires := time.Now().UTC().Add(30 * time.Second)
	_, err = tx.Exec(ctx, `UPDATE workforce_delivery_attempts SET phase='PREPARED',push_authorization_sha256=$2,push_authorization_expires_at=$3,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, attemptID, secretHash(token), expires)
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE workforce_delivery_requests SET status='CONFIRMING',updated_at=CURRENT_TIMESTAMP WHERE id=$1`, requestID)
	}
	if err != nil {
		return application.PrepareResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return application.PrepareResult{}, err
	}
	return application.PrepareResult{PushAuthorization: token, ExpiresAt: expires.Format(time.RFC3339Nano)}, nil
}

func (s *Store) RecordResult(ctx context.Context, a application.MachineAuthority, attemptID int64, lease string, authorization string, result application.DeliveryResult) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var requestID, worker int64
	var expectedBase, delivery, status, requestStatus, leaseHash, authHash, instance, boot string
	var generation int64
	var authExpiry time.Time
	err = tx.QueryRow(ctx, `SELECT a.request_id,r.expected_base_commit,r.delivery_commit,a.status,r.status,a.lease_sha256,a.push_authorization_sha256,a.push_authorization_expires_at,a.worker_id,a.worker_instance_id,a.host_boot_id,a.generation FROM workforce_delivery_attempts a JOIN workforce_delivery_requests r ON r.id=a.request_id WHERE a.id=$1 FOR UPDATE OF a,r`, attemptID).Scan(&requestID, &expectedBase, &delivery, &status, &requestStatus, &leaseHash, &authHash, &authExpiry, &worker, &instance, &boot, &generation)
	if err != nil {
		return err
	}
	// push_authorization_expires_at gates the start of the side effect in
	// PreparePush. Once Git has started, its final readback may legitimately
	// arrive after that short deadline and must still be persisted for safe
	// arbitration rather than discarded.
	if status != "RUNNING" {
		return errors.New("DELIVERY_ATTEMPT_NOT_RUNNING")
	}
	if leaseHash != secretHash(lease) {
		return errors.New("DELIVERY_LEASE_INVALID")
	}
	if authHash != secretHash(authorization) {
		return errors.New("DELIVERY_PUSH_AUTHORIZATION_INVALID")
	}
	if worker != a.WorkerID || instance != a.InstanceID || boot != a.HostBootID {
		return errors.New("DELIVERY_OWNER_STALE")
	}
	if generation != a.Generation {
		return errors.New("DELIVERY_GENERATION_STALE")
	}
	_ = authExpiry
	decision := domain.ReconcileAfter(expectedBase, delivery, result.RemoteAfter, result.Readable)
	if requestStatus == "CANCELLING" {
		switch {
		case result.Readable && result.RemoteAfter == delivery:
			decision = domain.RemoteDecision{Status: "SUCCEEDED", ReasonCode: "CANCEL_AFTER_EFFECT"}
		case result.Readable && (result.RemoteAfter == "" || result.RemoteAfter == expectedBase):
			decision = domain.RemoteDecision{Status: "CANCELLED", ReasonCode: "USER_CANCELLED"}
		default:
			decision = domain.RemoteDecision{Status: "OUTCOME_UNKNOWN", ReasonCode: "CANCEL_REMOTE_REF_UNAVAILABLE"}
		}
	}
	epochAttemptNumber, err := deliveryRetryEpochAttemptCount(ctx, tx, requestID)
	if err != nil {
		return err
	}
	attemptStatus := decision.Status
	if attemptStatus == "RETRY_WAIT" && epochAttemptNumber >= 3 {
		attemptStatus = "FAILED"
		decision.Status = "FAILED"
		decision.ReasonCode = "DELIVERY_RETRY_EXHAUSTED"
	}
	nextRetry := retryAt(epochAttemptNumber)
	_, err = tx.Exec(ctx, `UPDATE workforce_delivery_attempts SET status=$2::text,phase='VERIFIED',remote_before=$3,remote_after=$4,push_started_at=CASE WHEN $5 THEN COALESCE(push_started_at,CURRENT_TIMESTAMP) ELSE push_started_at END,verified_at=CASE WHEN $2='SUCCEEDED' THEN CURRENT_TIMESTAMP ELSE verified_at END,reason_code=$6,next_retry_at=CASE WHEN $2='RETRY_WAIT' THEN $7::timestamptz ELSE NULL END,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, attemptID, attemptStatus, result.RemoteBefore, result.RemoteAfter, result.PushStarted, decision.ReasonCode, nextRetry)
	if err != nil {
		return err
	}
	finalRequestStatus := decision.Status
	_, err = tx.Exec(ctx, `UPDATE workforce_delivery_requests SET status=$2::text,reason_code=$3,version=version+1,finished_at=CASE WHEN $2 IN ('SUCCEEDED','FAILED','MANUAL_REVIEW','CANCELLED') THEN CURRENT_TIMESTAMP ELSE finished_at END,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, requestID, finalRequestStatus, decision.ReasonCode)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status=$2::text,reason_code=$3,finished_at=CASE WHEN $2 IN ('SUCCEEDED','FAILED','MANUAL_REVIEW','CANCELLED') THEN CURRENT_TIMESTAMP ELSE finished_at END,updated_at=CURRENT_TIMESTAMP WHERE delivery_request_id=$1 AND operation_kind='PUSH_BRANCH'`, requestID, finalRequestStatus, decision.ReasonCode); err != nil {
		return err
	}
	if decision.Status == "SUCCEEDED" {
		if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status='READY',updated_at=CURRENT_TIMESTAMP WHERE delivery_request_id=$1 AND operation_kind='CREATE_PULL_REQUEST' AND status='WAITING_DEPENDENCY'`, requestID); err != nil {
			return err
		}
	}
	if err = s.projectDeliveryRequest(ctx, tx, requestID); err != nil {
		return err
	}
	if decision.Status == "SUCCEEDED" {
		if err = s.recordDeliveryCompleted(ctx, tx, requestID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) RecordFailure(ctx context.Context, a application.MachineAuthority, attemptID int64, lease string, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 128 {
		return domain.ErrIdentityConflict
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var requestID, worker int64
	var status, stored, instance, boot string
	var generation int64
	err = tx.QueryRow(ctx, `SELECT request_id,status,lease_sha256,worker_id,worker_instance_id,host_boot_id,generation FROM workforce_delivery_attempts WHERE id=$1 FOR UPDATE`, attemptID).Scan(&requestID, &status, &stored, &worker, &instance, &boot, &generation)
	if err != nil {
		return err
	}
	if status != "RUNNING" || stored != secretHash(lease) || worker != a.WorkerID || instance != a.InstanceID || boot != a.HostBootID || generation != a.Generation {
		return domain.ErrIdentityConflict
	}
	epochAttemptNumber, err := deliveryRetryEpochAttemptCount(ctx, tx, requestID)
	if err != nil {
		return err
	}
	retryable := deliveryFailureRetryable(reason) && epochAttemptNumber < 3
	attemptStatus, requestStatus := "FAILED", "FAILED"
	if reason == "CUSTOM_DELIVERY_REQUIRES_VERIFIER" {
		attemptStatus, requestStatus = "MANUAL_REVIEW", "MANUAL_REVIEW"
	}
	if retryable {
		attemptStatus, requestStatus = "RETRY_WAIT", "RETRY_WAIT"
	}
	_, err = tx.Exec(ctx, `UPDATE workforce_delivery_attempts SET status=$2::text,reason_code=$3,next_retry_at=CASE WHEN $2='RETRY_WAIT' THEN $4::timestamptz ELSE NULL END,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, attemptID, attemptStatus, reason, retryAt(epochAttemptNumber))
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE workforce_delivery_requests SET status=$2::text,reason_code=$3,version=version+1,finished_at=CASE WHEN $2='FAILED' THEN CURRENT_TIMESTAMP ELSE NULL END,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, requestID, requestStatus, reason)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status=$2::text,reason_code=$3,finished_at=CASE WHEN $2 IN ('FAILED','MANUAL_REVIEW') THEN CURRENT_TIMESTAMP ELSE NULL END,updated_at=CURRENT_TIMESTAMP WHERE delivery_request_id=$1 AND operation_kind='PUSH_BRANCH'`, requestID, requestStatus, reason)
	}
	if err != nil {
		return err
	}
	if err = s.projectDeliveryRequest(ctx, tx, requestID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ResolveCredentialEnvelope(ctx context.Context, a application.MachineAuthority, attemptID int64, lease string) (application.EncryptedCredentialEnvelope, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return application.EncryptedCredentialEnvelope{}, err
	}
	defer tx.Rollback(ctx)

	var tenantID, taskID, taskTurnID, projectID, worker, requestID int64
	var version, authorizationDigest, stored, instance, boot, status, requestStatus, targetDigest string
	var generation int64
	var expires time.Time
	err = tx.QueryRow(ctx, `SELECT r.tenant_id,r.task_id,r.task_turn_id,r.project_id,r.credential_version,r.authorization_digest,a.lease_sha256,a.lease_expires_at,a.worker_id,a.worker_instance_id,a.host_boot_id,a.generation,a.status,r.status,r.target_config_digest,r.id
		FROM workforce_delivery_attempts a
		JOIN workforce_delivery_requests r ON r.id=a.request_id
		WHERE a.id=$1 FOR SHARE OF a,r`, attemptID).Scan(&tenantID, &taskID, &taskTurnID, &projectID, &version, &authorizationDigest, &stored, &expires, &worker, &instance, &boot, &generation, &status, &requestStatus, &targetDigest, &requestID)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.EncryptedCredentialEnvelope{}, application.ErrNotFound
	}
	if err != nil {
		return application.EncryptedCredentialEnvelope{}, err
	}
	plan, err := s.loadDeliveryRepositoryPlan(ctx, tenantID, taskID, taskTurnID, requestID)
	if err != nil {
		return application.EncryptedCredentialEnvelope{}, err
	}
	if status != "RUNNING" || (requestStatus != "RUNNING" && requestStatus != "CONFIRMING") || stored != secretHash(lease) || !expires.After(time.Now()) || worker != a.WorkerID || instance != a.InstanceID || boot != a.HostBootID || generation != a.Generation || projectID != plan.ProjectID || targetDigest != plan.TargetConfigDigest || version != plan.credentialVersion() || authorizationDigest != plan.PushAuthorizationDigest {
		return application.EncryptedCredentialEnvelope{}, domain.ErrIdentityConflict
	}
	if err = tx.Commit(ctx); err != nil {
		return application.EncryptedCredentialEnvelope{}, err
	}
	return plan.Credential, nil
}

func retryAt(attemptNumber int) time.Time {
	delays := []time.Duration{time.Second, 10 * time.Second, 60 * time.Second}
	if attemptNumber < 1 || attemptNumber > len(delays) {
		return time.Now().UTC().Add(time.Minute)
	}
	return time.Now().UTC().Add(delays[attemptNumber-1])
}

func deliveryFailureRetryable(reason string) bool {
	switch reason {
	case "DELIVERY_CHECKPOINT_SOURCE_UNAVAILABLE", "DELIVERY_REMOTE_UNREACHABLE", "DELIVERY_WORKER_TRANSIENT", "DELIVERY_GIT_TRANSIENT":
		return true
	default:
		return false
	}
}

func randomToken() string {
	body := make([]byte, 32)
	_, _ = rand.Read(body)
	return hex.EncodeToString(body)
}
func secretHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
