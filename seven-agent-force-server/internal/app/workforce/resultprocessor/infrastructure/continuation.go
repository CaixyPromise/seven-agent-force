package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/checkpoint"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/eventlog"
	tasknotification "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/facade"
)

const continuationRetention = 30 * 24 * time.Hour

// RecordContinuationCheckpoint accepts only a byte-for-byte verified Worker
// candidate whose object keys are deterministic from the frozen execution
// identity.  For WAIT_USER and MANUAL_REVIEW it is also the terminal cleanup
// boundary: the Agent Runtime has exited, the checkpoint is durable, and only
// then can the Assignment stop reserving Fleet capacity.
func (s *Store) RecordContinuationCheckpoint(ctx context.Context, identity facade.MachineIdentity, attemptID int64, lease string, candidate facade.ContinuationCheckpointCandidate) (facade.ContinuationCheckpointView, error) {
	if err := validRuntimeIdentity(identity, attemptID, lease); err != nil {
		return facade.ContinuationCheckpointView{}, err
	}
	if err := validateCheckpointCandidate(candidate); err != nil {
		return facade.ContinuationCheckpointView{}, err
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, candidate.ExpiresAt)
	if err != nil {
		return facade.ContinuationCheckpointView{}, fmt.Errorf("CHECKPOINT_EXPIRY_INVALID")
	}
	now := time.Now().UTC()
	if !expiresAt.After(now) || expiresAt.After(now.Add(continuationRetention+time.Hour)) {
		return facade.ContinuationCheckpointView{}, fmt.Errorf("CHECKPOINT_EXPIRY_INVALID")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return facade.ContinuationCheckpointView{}, err
	}
	defer tx.Rollback(ctx)
	scope, err := s.loadEvaluationScope(ctx, tx, identity, attemptID, lease)
	if err != nil {
		return facade.ContinuationCheckpointView{}, fmt.Errorf("CHECKPOINT_SCOPE_INVALID: %w", err)
	}
	if !checkpointTaskStatusAllows(scope.TaskStatus, candidate.Decision) {
		return facade.ContinuationCheckpointView{}, fmt.Errorf("CHECKPOINT_TASK_STATE_INVALID")
	}
	var decision, sessionDigest string
	err = tx.QueryRow(ctx, `SELECT decision,session_digest
		FROM workforce_result_evaluation_cycles
		WHERE id=$1 AND attempt_id=$2 AND task_turn_id=$3 AND status='PERSISTED'
		FOR UPDATE`, candidate.EvaluationCycleID, attemptID, scope.TurnID).Scan(&decision, &sessionDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return facade.ContinuationCheckpointView{}, ErrNotFound
	}
	if err != nil {
		return facade.ContinuationCheckpointView{}, err
	}
	if decision != candidate.Decision {
		return facade.ContinuationCheckpointView{}, fmt.Errorf("CHECKPOINT_DECISION_MISMATCH")
	}
	identityInput := checkpoint.Input{
		TaskID: scope.TaskID, TaskTurnID: scope.TurnID, AttemptID: attemptID, Generation: identity.Generation,
		EvaluationCycleID: candidate.EvaluationCycleID, Decision: candidate.Decision, SessionDigest: sessionDigest,
	}
	expectedIdentity := checkpoint.Identity(identityInput)
	bundleKey, overlayKey, sessionKey, manifestKey := checkpoint.ObjectKeysV2(identityInput, expectedIdentity)
	if candidate.CheckpointIdentity != expectedIdentity || candidate.RepositoryBundleObjectKey != bundleKey || candidate.WorkspaceOverlayObjectKey != overlayKey || candidate.SessionObjectKey != sessionKey || candidate.ManifestObjectKey != manifestKey {
		return facade.ContinuationCheckpointView{}, fmt.Errorf("CHECKPOINT_IDENTITY_MISMATCH")
	}

	var existing facade.ContinuationCheckpointView
	err = tx.QueryRow(ctx, `SELECT id,task_id,task_turn_id,attempt_id,checkpoint_identity_sha256,evaluation_cycle_id,decision,manifest_sha256,status,
		to_char(expires_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		COALESCE(to_char(available_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),'')
		FROM workforce_continuation_checkpoints WHERE attempt_id=$1 AND checkpoint_identity_sha256=$2 FOR UPDATE`, attemptID, expectedIdentity).
		Scan(&existing.ID, &existing.TaskID, &existing.TaskTurnID, &existing.AttemptID, &existing.CheckpointIdentity, &existing.EvaluationCycleID, &existing.Decision, &existing.ManifestSHA256, &existing.Status, &existing.ExpiresAt, &existing.CreatedAt, &existing.AvailableAt)
	if err == nil {
		if existing.EvaluationCycleID != candidate.EvaluationCycleID || existing.Decision != candidate.Decision || existing.ManifestSHA256 != candidate.ManifestSHA256 || existing.Status != "AVAILABLE" {
			return facade.ContinuationCheckpointView{}, fmt.Errorf("CHECKPOINT_REPLAY_CONFLICT")
		}
		if err = tx.Commit(ctx); err != nil {
			return facade.ContinuationCheckpointView{}, err
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return facade.ContinuationCheckpointView{}, err
	}
	checkpointID := s.idGen.NextID()
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_continuation_checkpoints
		(id,task_id,task_turn_id,attempt_id,evaluation_cycle_id,decision,checkpoint_identity_sha256,
		 format_version,repository_bundle_object_key,repository_bundle_content_sha256,repository_bundle_object_sha256,repository_bundle_size_bytes,
		 workspace_overlay_object_key,workspace_overlay_content_sha256,workspace_overlay_object_sha256,workspace_overlay_size_bytes,selection_digest,selection_policy_digest,gitignore_digest,
		 session_object_key,session_sha256,session_content_sha256,session_object_sha256,session_size_bytes,
		 identity_manifest_object_key,manifest_sha256,manifest_content_sha256,manifest_object_sha256,manifest_size_bytes,
		 task_execution_snapshot_digest,base_commit,result_commit,result_tree,session_lineage_digest,encryption_algorithm,encryption_key_id,wrapped_data_key,status,expires_at,available_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,2,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,
		 $19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,'AVAILABLE',$37,CURRENT_TIMESTAMP)`,
		checkpointID, scope.TaskID, scope.TurnID, attemptID, candidate.EvaluationCycleID, candidate.Decision, expectedIdentity,
		candidate.RepositoryBundleObjectKey, candidate.RepositoryBundleContentSHA256, candidate.RepositoryBundleObjectSHA256, candidate.RepositoryBundleSizeBytes,
		candidate.WorkspaceOverlayObjectKey, candidate.WorkspaceOverlayContentSHA256, candidate.WorkspaceOverlayObjectSHA256, candidate.WorkspaceOverlaySizeBytes, candidate.SelectionDigest, candidate.SelectionPolicyDigest, candidate.GitignoreDigest,
		candidate.SessionObjectKey, candidate.SessionContentSHA256, candidate.SessionContentSHA256, candidate.SessionObjectSHA256, candidate.SessionSizeBytes,
		candidate.ManifestObjectKey, candidate.ManifestContentSHA256, candidate.ManifestContentSHA256, candidate.ManifestObjectSHA256, candidate.ManifestSizeBytes,
		candidate.TaskExecutionSnapshotDigest, candidate.BaseCommit, candidate.ResultCommit, candidate.ResultTree, candidate.SessionLineageDigest, candidate.EncryptionAlgorithm, candidate.EncryptionKeyID, candidate.WrappedDataKey, expiresAt.UTC()); err != nil {
		return facade.ContinuationCheckpointView{}, fmt.Errorf("CHECKPOINT_PERSIST_FAILED: %w", classifyConflict(err))
	}
	if candidate.Decision == "WAIT_USER" || candidate.Decision == "MANUAL_REVIEW" {
		if err = finishReviewRuntime(ctx, tx, scope, attemptID, candidate.Decision, candidate.EvaluationCycleID); err != nil {
			return facade.ContinuationCheckpointView{}, fmt.Errorf("CHECKPOINT_RUNTIME_FINALIZE_FAILED: %w", err)
		}
	}
	var notificationOccurrence int64
	if notificationOccurrence, err = eventlog.AppendWithSequence(ctx, tx, scope.TaskID, attemptID, "continuation.checkpoint.available", map[string]any{
		"checkpointId": checkpointID, "checkpointIdentity": expectedIdentity, "evaluationCycleId": candidate.EvaluationCycleID,
		"decision": candidate.Decision, "manifestSha256": candidate.ManifestSHA256, "expiresAt": expiresAt.UTC().Format(time.RFC3339Nano),
	}); err != nil {
		return facade.ContinuationCheckpointView{}, fmt.Errorf("CHECKPOINT_EVENT_PERSIST_FAILED: %w", err)
	}
	if candidate.Decision == "WAIT_USER" || candidate.Decision == "MANUAL_REVIEW" {
		if err = s.recordContinuationNotification(ctx, tx, scope, checkpointID, notificationOccurrence, candidate.Decision, now); err != nil {
			return facade.ContinuationCheckpointView{}, fmt.Errorf("CHECKPOINT_NOTIFICATION_PERSIST_FAILED: %w", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return facade.ContinuationCheckpointView{}, err
	}
	return facade.ContinuationCheckpointView{
		ID: checkpointID, TaskID: scope.TaskID, TaskTurnID: scope.TurnID, AttemptID: attemptID,
		CheckpointIdentity: expectedIdentity, EvaluationCycleID: candidate.EvaluationCycleID, Decision: candidate.Decision,
		ManifestSHA256: candidate.ManifestSHA256, Status: "AVAILABLE", ExpiresAt: expiresAt.UTC().Format(time.RFC3339Nano),
		CreatedAt: now.Format(time.RFC3339Nano), AvailableAt: now.Format(time.RFC3339Nano),
	}, nil
}

func (s *Store) recordContinuationNotification(ctx context.Context, tx pgx.Tx, scope evaluationScope, checkpointID, sourceOccurrence int64, decision string, occurredAt time.Time) error {
	if s == nil || s.notificationRecorder == nil {
		return nil
	}
	reasonCode := ""
	switch decision {
	case "WAIT_USER":
		reasonCode = "PROCESSOR_WAIT_USER"
	case "MANUAL_REVIEW":
		reasonCode = "PROCESSOR_MANUAL_REVIEW"
	default:
		return nil
	}
	_, err := s.notificationRecorder.RecordTransition(ctx, tx, tasknotification.TaskNotificationTransition{
		TenantID: scope.TenantID, TaskID: scope.TaskID, TaskTurnID: scope.TurnID,
		SourceType: tasknotification.SourceContinuationCheckpoint, SourceID: checkpointID, SourceOccurrence: sourceOccurrence,
		ReasonCode: reasonCode, AttentionUserID: 0, Recoverability: tasknotification.RecoverabilityUserRecoverable, OccurredAt: occurredAt.UTC(),
	})
	return err
}

func (s *Store) ListContinuationCheckpoints(ctx context.Context, tenantID int64, taskID int64) ([]facade.ContinuationCheckpointView, error) {
	rows, err := s.db.Query(ctx, `SELECT c.id,c.task_id,c.task_turn_id,c.attempt_id,c.checkpoint_identity_sha256,COALESCE(c.evaluation_cycle_id,0),c.decision,c.manifest_sha256,c.status,
		to_char(c.expires_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		to_char(c.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		COALESCE(to_char(c.available_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),'')
		FROM workforce_continuation_checkpoints c
		JOIN workforce_tasks t ON t.id=c.task_id
		WHERE t.tenant_id=$1 AND c.task_id=$2 ORDER BY c.created_at DESC`, tenantID, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]facade.ContinuationCheckpointView, 0)
	for rows.Next() {
		var item facade.ContinuationCheckpointView
		if err = rows.Scan(&item.ID, &item.TaskID, &item.TaskTurnID, &item.AttemptID, &item.CheckpointIdentity, &item.EvaluationCycleID, &item.Decision, &item.ManifestSHA256, &item.Status, &item.ExpiresAt, &item.CreatedAt, &item.AvailableAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func validateCheckpointCandidate(candidate facade.ContinuationCheckpointCandidate) error {
	if candidate.FormatVersion != 2 || len(strings.TrimSpace(candidate.CheckpointIdentity)) != 64 || len(strings.TrimSpace(candidate.RepositoryBundleContentSHA256)) != 64 || len(strings.TrimSpace(candidate.RepositoryBundleObjectSHA256)) != 64 || len(strings.TrimSpace(candidate.WorkspaceOverlayContentSHA256)) != 64 || len(strings.TrimSpace(candidate.WorkspaceOverlayObjectSHA256)) != 64 || len(strings.TrimSpace(candidate.SessionContentSHA256)) != 64 || len(strings.TrimSpace(candidate.SessionObjectSHA256)) != 64 || len(strings.TrimSpace(candidate.ManifestContentSHA256)) != 64 || len(strings.TrimSpace(candidate.ManifestObjectSHA256)) != 64 || len(strings.TrimSpace(candidate.SelectionDigest)) != 64 || len(strings.TrimSpace(candidate.SelectionPolicyDigest)) != 64 || len(strings.TrimSpace(candidate.GitignoreDigest)) != 64 ||
		candidate.EvaluationCycleID <= 0 || (candidate.Decision != "PASS" && candidate.Decision != "WAIT_USER" && candidate.Decision != "MANUAL_REVIEW") ||
		strings.TrimSpace(candidate.RepositoryBundleObjectKey) == "" || strings.TrimSpace(candidate.WorkspaceOverlayObjectKey) == "" || strings.TrimSpace(candidate.SessionObjectKey) == "" || strings.TrimSpace(candidate.ManifestObjectKey) == "" ||
		candidate.RepositoryBundleSizeBytes < 1 || candidate.RepositoryBundleSizeBytes > checkpoint.MaxArchiveBytes+(16<<10) || candidate.WorkspaceOverlaySizeBytes < 1 || candidate.WorkspaceOverlaySizeBytes > checkpoint.MaxArchiveBytes+(16<<10) || candidate.SessionSizeBytes < 1 || candidate.SessionSizeBytes > checkpoint.MaxSessionArchiveBytes+(16<<10) || candidate.ManifestSizeBytes < 1 || candidate.ManifestSizeBytes > 1<<20 || len(candidate.TaskExecutionSnapshotDigest) != 64 || strings.TrimSpace(candidate.SessionLineageDigest) == "" || candidate.EncryptionAlgorithm != "AES-256-GCM-CHUNKED" || strings.TrimSpace(candidate.EncryptionKeyID) == "" || candidate.WrappedDataKey == "" {
		return fmt.Errorf("CHECKPOINT_CANDIDATE_INVALID")
	}
	return nil
}

func checkpointTaskStatusAllows(status, decision string) bool {
	switch decision {
	case "PASS":
		return status == "VALIDATING"
	case "WAIT_USER":
		return status == "REVIEWING"
	case "MANUAL_REVIEW":
		return status == "MANUAL_REVIEW"
	default:
		return false
	}
}

func finishReviewRuntime(ctx context.Context, tx pgx.Tx, scope evaluationScope, attemptID int64, decision string, cycleID int64) error {
	attemptStatus, nodeStatus := "WAITING_USER", "WAIT_USER"
	if decision == "MANUAL_REVIEW" {
		attemptStatus, nodeStatus = "MANUAL_REVIEW", "MANUAL_REVIEW"
	}
	if tag, err := tx.Exec(ctx, `UPDATE workforce_task_attempts SET status=$2,finished_at=CURRENT_TIMESTAMP,exit_code=0 WHERE id=$1 AND status IN ('RUNNING','PREPARING')`, attemptID, attemptStatus); err != nil {
		return err
	} else if err = requireReviewRuntimeTransition(tag.RowsAffected(), "ATTEMPT"); err != nil {
		return err
	}
	if tag, err := tx.Exec(ctx, `UPDATE workforce_attempt_runtimes SET status='SUCCEEDED',terminal_reason_code=$2,updated_at=CURRENT_TIMESTAMP WHERE attempt_id=$1 AND status='RUNNING'`, attemptID, decision); err != nil {
		return err
	} else if err = requireReviewRuntimeTransition(tag.RowsAffected(), "RUNTIME"); err != nil {
		return err
	}
	if tag, err := tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status=$2,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND status IN ('RUNNING','PREPARING')`, scope.NodeID, nodeStatus); err != nil {
		return err
	} else if err = requireReviewRuntimeTransition(tag.RowsAffected(), "EXECUTION_NODE"); err != nil {
		return err
	}
	if tag, err := tx.Exec(ctx, `UPDATE workforce_task_assignments SET status='COMPLETED',completed_at=CURRENT_TIMESTAMP,release_reason_code=$2 WHERE id=(SELECT assignment_id FROM workforce_task_attempts WHERE id=$1) AND status='CLAIMED'`, attemptID, "RESULT_PROCESSOR_"+decision); err != nil {
		return err
	} else if err = requireReviewRuntimeTransition(tag.RowsAffected(), "ASSIGNMENT"); err != nil {
		return err
	}
	_ = cycleID
	return nil
}

func requireReviewRuntimeTransition(rows int64, component string) error {
	if rows != 1 {
		return fmt.Errorf("CHECKPOINT_RUNTIME_%s_STALE", component)
	}
	return nil
}
