package infrastructure

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	artifactdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/artifact/domain"
	reviewrun "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/eventlog"
	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	dispatchdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
	tasknotification "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/facade"
)

var (
	ErrNotFound = errors.New("post-process work not found")
	ErrConflict = errors.New("post-process identity conflict")
)

type Store struct {
	db                   *pgxpool.Pool
	idGen                IDGenerator
	now                  func() time.Time
	notificationRecorder tasknotification.TransactionalRecorder
	reviewGate           reviewrun.FinalizationGate
}

type IDGenerator interface{ NextID() int64 }

func New(db *pgxpool.Pool, idGen IDGenerator) *Store {
	return &Store{db: db, idGen: idGen, now: time.Now}
}

func (s *Store) SetTaskNotificationRecorder(recorder tasknotification.TransactionalRecorder) {
	if s != nil {
		s.notificationRecorder = recorder
	}
}

// SetReviewRunGate binds the only ReviewRun contract Artifact needs. The
// post-process Store never opens or mutates ReviewRun's private persistence.
func (s *Store) SetReviewRunGate(gate reviewrun.FinalizationGate) {
	if s != nil {
		s.reviewGate = gate
	}
}

// ReconcileTimeouts converts abandoned post-process leases into the same
// bounded retry state used by an explicit Worker failure. This is deliberately
// independent from Agent Attempt retry: neither branch creates an Assignment
// or increments the Agent generation.
func (s *Store) ReconcileTimeouts(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 256 {
		limit = 32
	}
	rows, err := s.db.Query(ctx, `
SELECT 'FINALIZATION',id FROM workforce_finalization_runs
 WHERE status='RUNNING' AND lease_expires_at<CURRENT_TIMESTAMP
UNION ALL
SELECT 'UPLOAD',id FROM workforce_artifact_upload_attempts
 WHERE status IN ('UPLOADING','VERIFYING') AND lease_expires_at<CURRENT_TIMESTAMP
ORDER BY 1,2 LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	type expired struct {
		kind string
		id   int64
	}
	items := make([]expired, 0, limit)
	for rows.Next() {
		var item expired
		if err = rows.Scan(&item.kind, &item.id); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, item)
	}
	rows.Close()
	processed := 0
	for _, item := range items {
		var changed bool
		if item.kind == "FINALIZATION" {
			changed, err = s.timeoutFinalization(ctx, item.id)
		} else {
			changed, err = s.timeoutUpload(ctx, item.id)
		}
		if err != nil {
			return processed, err
		}
		if changed {
			processed++
		}
	}
	return processed, nil
}

func (s *Store) timeoutFinalization(ctx context.Context, runID int64) (bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var taskID, attemptID, worker int64
	var status string
	var generation int64
	var runNumber, maxRuns int
	var expires time.Time
	if err = tx.QueryRow(ctx, `SELECT task_id,attempt_id,worker_id,generation,status,run_number,max_runs,lease_expires_at FROM workforce_finalization_runs WHERE id=$1 FOR UPDATE`, runID).Scan(&taskID, &attemptID, &worker, &generation, &status, &runNumber, &maxRuns, &expires); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if status != artifactdomain.FinalizationRunning || !expires.Before(s.now()) {
		return false, nil
	}
	if runNumber >= maxRuns {
		if _, err = tx.Exec(ctx, `UPDATE workforce_finalization_runs SET status='FAILED',error_code='FINALIZATION_TIMEOUT',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, runID); err != nil {
			return false, err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='MANUAL_REVIEW',result_summary='FINALIZATION_RETRY_EXHAUSTED',version=version+1 WHERE id=$1 AND status='PACKAGING'`, taskID); err != nil {
			return false, err
		}
	} else {
		next := s.now().UTC().Add([]time.Duration{time.Second, 10 * time.Second, 60 * time.Second}[minInt(max(runNumber-1, 0), 2)])
		if _, err = tx.Exec(ctx, `UPDATE workforce_finalization_runs SET status='RETRY_WAIT',run_number=run_number+1,next_retry_at=$2,error_code='FINALIZATION_TIMEOUT',lease_sha256=NULL,lease_expires_at=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, runID, next); err != nil {
			return false, err
		}
		messageID := s.idGen.NextID()
		message := artifactdomain.WorkMessage{SchemaVersion: 1, Kind: artifactdomain.FinalizationMessageKind, MessageID: messageID, WorkerID: worker, ResourceID: runID, Generation: generation}
		body, _ := json.Marshal(message)
		if _, err = tx.Exec(ctx, `INSERT INTO workforce_outbox_events (id,aggregate_type,aggregate_id,event_type,payload_json,status,exchange_name,routing_key,message_expiration_ms,next_retry_at) VALUES ($1,'finalization_run',$2,'finalization.timeout_retry',$3,'PENDING','seven.workforce.postprocess',$4,600000,$5)`, messageID, runID, body, fmt.Sprintf("postprocess.%d", worker), next); err != nil {
			return false, err
		}
	}
	var occurrence int64
	if occurrence, err = eventlog.AppendWithSequence(ctx, tx, taskID, attemptID, "finalization.timed_out", map[string]any{"finalizationRunId": runID, "runNumber": runNumber}); err != nil {
		return false, err
	}
	if runNumber >= maxRuns && s.notificationRecorder != nil {
		var tenantID, turnID int64
		if err = tx.QueryRow(ctx, `SELECT tenant_id,COALESCE(current_task_turn_id,0) FROM workforce_tasks WHERE id=$1`, taskID).Scan(&tenantID, &turnID); err != nil {
			return false, err
		}
		if _, err = s.notificationRecorder.RecordTransition(ctx, tx, tasknotification.TaskNotificationTransition{TenantID: tenantID, TaskID: taskID, TaskTurnID: turnID, SourceType: tasknotification.SourceFinalizationRun, SourceID: runID, SourceOccurrence: occurrence, PreviousStatus: "RUNNING", CurrentStatus: "MANUAL_REVIEW", ReasonCode: "FINALIZATION_RETRY_EXHAUSTED", Recoverability: tasknotification.RecoverabilityUserRecoverable, OccurredAt: s.now().UTC()}); err != nil {
			return false, err
		}
	}
	return true, tx.Commit(ctx)
}

func (s *Store) timeoutUpload(ctx context.Context, uploadID int64) (bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var taskID, attemptID, worker, materializationRunID int64
	var status string
	var generation int64
	var attemptNumber, maxAttempts int
	var expires time.Time
	if err = tx.QueryRow(ctx, `SELECT u.task_id,u.attempt_id,u.worker_id,u.status,u.attempt_number,u.max_attempts,u.lease_expires_at,COALESCE(f.generation,m.generation),COALESCE(u.materialization_run_id,0) FROM workforce_artifact_upload_attempts u LEFT JOIN workforce_finalization_runs f ON f.id=u.finalization_run_id LEFT JOIN workforce_result_materialization_runs m ON m.id=u.materialization_run_id WHERE u.id=$1 FOR UPDATE OF u`, uploadID).Scan(&taskID, &attemptID, &worker, &status, &attemptNumber, &maxAttempts, &expires, &generation, &materializationRunID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if (status != artifactdomain.UploadUploading && status != artifactdomain.UploadVerifying) || !expires.Before(s.now()) {
		return false, nil
	}
	if attemptNumber >= maxAttempts {
		if _, err = tx.Exec(ctx, `UPDATE workforce_artifact_upload_attempts SET status='FAILED',error_code='ARTIFACT_UPLOAD_TIMEOUT',updated_at=CURRENT_TIMESTAMP WHERE id=$1`, uploadID); err != nil {
			return false, err
		}
		if materializationRunID == 0 {
			if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='MANUAL_REVIEW',result_summary='ARTIFACT_UPLOAD_RETRY_EXHAUSTED',version=version+1 WHERE id=$1 AND status='UPLOADING'`, taskID); err != nil {
				return false, err
			}
		} else if _, err = tx.Exec(ctx, `UPDATE workforce_result_materialization_runs SET status='FAILED',reason_code='ARTIFACT_UPLOAD_RETRY_EXHAUSTED',updated_at=CURRENT_TIMESTAMP WHERE id=$1`, materializationRunID); err != nil {
			return false, err
		}
	} else {
		backoffs := []time.Duration{time.Second, 5 * time.Second, 30 * time.Second, 120 * time.Second, 300 * time.Second}
		next := s.now().UTC().Add(backoffs[minInt(max(attemptNumber-1, 0), len(backoffs)-1)])
		if _, err = tx.Exec(ctx, `UPDATE workforce_artifact_upload_attempts SET status='RETRY_WAIT',attempt_number=attempt_number+1,next_retry_at=$2,error_code='ARTIFACT_UPLOAD_TIMEOUT',lease_sha256=NULL,lease_expires_at=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, uploadID, next); err != nil {
			return false, err
		}
		messageID := s.idGen.NextID()
		message := artifactdomain.WorkMessage{SchemaVersion: 1, Kind: artifactdomain.UploadMessageKind, MessageID: messageID, WorkerID: worker, ResourceID: uploadID, Generation: generation}
		body, _ := json.Marshal(message)
		if _, err = tx.Exec(ctx, `INSERT INTO workforce_outbox_events (id,aggregate_type,aggregate_id,event_type,payload_json,status,exchange_name,routing_key,message_expiration_ms,next_retry_at) VALUES ($1,'artifact_upload',$2,'artifact.upload.timeout_retry',$3,'PENDING','seven.workforce.postprocess',$4,600000,$5)`, messageID, uploadID, body, fmt.Sprintf("postprocess.%d", worker), next); err != nil {
			return false, err
		}
	}
	var occurrence int64
	if occurrence, err = eventlog.AppendWithSequence(ctx, tx, taskID, attemptID, "artifact.upload.timed_out", map[string]any{"uploadAttemptId": uploadID, "attemptNumber": attemptNumber}); err != nil {
		return false, err
	}
	if attemptNumber >= maxAttempts && materializationRunID == 0 && s.notificationRecorder != nil {
		var tenantID, turnID int64
		if err = tx.QueryRow(ctx, `SELECT tenant_id,COALESCE(current_task_turn_id,0) FROM workforce_tasks WHERE id=$1`, taskID).Scan(&tenantID, &turnID); err != nil {
			return false, err
		}
		if _, err = s.notificationRecorder.RecordTransition(ctx, tx, tasknotification.TaskNotificationTransition{TenantID: tenantID, TaskID: taskID, TaskTurnID: turnID, SourceType: tasknotification.SourceArtifactUpload, SourceID: uploadID, SourceOccurrence: occurrence, PreviousStatus: status, CurrentStatus: "MANUAL_REVIEW", ReasonCode: "ARTIFACT_UPLOAD_RETRY_EXHAUSTED", Recoverability: tasknotification.RecoverabilityUserRecoverable, OccurredAt: s.now().UTC()}); err != nil {
			return false, err
		}
	}
	return true, tx.Commit(ctx)
}

func (s *Store) ListFinalizationRuns(ctx context.Context, taskID int64) ([]artifactdomain.FinalizationView, error) {
	rows, err := s.db.Query(ctx, `SELECT id,task_id,attempt_id,execution_node_id,generation,status,run_number,max_runs,worker_id,worker_instance_id,host_boot_id,COALESCE(manifest_digest,''),COALESCE(spool_identity,''),COALESCE(to_char(next_retry_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(error_code,''),to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),COALESCE(to_char(started_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(to_char(frozen_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(to_char(finished_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),'') FROM workforce_finalization_runs WHERE task_id=$1 ORDER BY created_at DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]artifactdomain.FinalizationView, 0)
	for rows.Next() {
		var item artifactdomain.FinalizationView
		if err = rows.Scan(&item.ID, &item.TaskID, &item.AttemptID, &item.ExecutionNodeID, &item.Generation, &item.Status, &item.RunNumber, &item.MaxRuns, &item.WorkerID, &item.WorkerInstanceID, &item.HostBootID, &item.ManifestDigest, &item.SpoolIdentity, &item.NextRetryAt, &item.ErrorCode, &item.CreatedAt, &item.StartedAt, &item.FrozenAt, &item.FinishedAt); err != nil {
			return nil, err
		}
		item.ArtifactUploadAttempts, err = s.listUploads(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetFinalizationRun(ctx context.Context, runID int64) (artifactdomain.FinalizationView, error) {
	var item artifactdomain.FinalizationView
	err := s.db.QueryRow(ctx, `SELECT id,task_id,attempt_id,execution_node_id,generation,status,run_number,max_runs,worker_id,worker_instance_id,host_boot_id,COALESCE(manifest_digest,''),COALESCE(spool_identity,''),COALESCE(to_char(next_retry_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(error_code,''),to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),COALESCE(to_char(started_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(to_char(frozen_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(to_char(finished_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),'') FROM workforce_finalization_runs WHERE id=$1`, runID).Scan(&item.ID, &item.TaskID, &item.AttemptID, &item.ExecutionNodeID, &item.Generation, &item.Status, &item.RunNumber, &item.MaxRuns, &item.WorkerID, &item.WorkerInstanceID, &item.HostBootID, &item.ManifestDigest, &item.SpoolIdentity, &item.NextRetryAt, &item.ErrorCode, &item.CreatedAt, &item.StartedAt, &item.FrozenAt, &item.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrNotFound
	}
	if err != nil {
		return item, err
	}
	item.ArtifactUploadAttempts, err = s.listUploads(ctx, item.ID)
	return item, err
}

func (s *Store) listUploads(ctx context.Context, runID int64) ([]artifactdomain.UploadView, error) {
	rows, err := s.db.Query(ctx, `SELECT id,COALESCE(finalization_run_id,0),COALESCE(materialization_run_id,0),task_id,attempt_id,artifact_type,artifact_name,object_key,sha256,size_bytes,media_type,attempt_number,max_attempts,status,COALESCE(to_char(next_retry_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(error_code,''),COALESCE(to_char(started_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(to_char(uploaded_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(to_char(verified_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),'') FROM workforce_artifact_upload_attempts WHERE finalization_run_id=$1 ORDER BY created_at,id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]artifactdomain.UploadView, 0)
	for rows.Next() {
		var item artifactdomain.UploadView
		if err = rows.Scan(&item.ID, &item.FinalizationRunID, &item.MaterializationRunID, &item.TaskID, &item.AttemptID, &item.ArtifactType, &item.Name, &item.ObjectKey, &item.SHA256, &item.SizeBytes, &item.MediaType, &item.AttemptNumber, &item.MaxAttempts, &item.Status, &item.NextRetryAt, &item.ErrorCode, &item.StartedAt, &item.UploadedAt, &item.VerifiedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetUploadAttempt(ctx context.Context, uploadID int64) (artifactdomain.UploadView, error) {
	var item artifactdomain.UploadView
	err := s.db.QueryRow(ctx, `SELECT id,COALESCE(finalization_run_id,0),COALESCE(materialization_run_id,0),task_id,attempt_id,artifact_type,artifact_name,object_key,sha256,size_bytes,media_type,attempt_number,max_attempts,status,COALESCE(to_char(next_retry_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(error_code,''),COALESCE(to_char(started_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(to_char(uploaded_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(to_char(verified_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),'') FROM workforce_artifact_upload_attempts WHERE id=$1`, uploadID).Scan(&item.ID, &item.FinalizationRunID, &item.MaterializationRunID, &item.TaskID, &item.AttemptID, &item.ArtifactType, &item.Name, &item.ObjectKey, &item.SHA256, &item.SizeBytes, &item.MediaType, &item.AttemptNumber, &item.MaxAttempts, &item.Status, &item.NextRetryAt, &item.ErrorCode, &item.StartedAt, &item.UploadedAt, &item.VerifiedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrNotFound
	}
	return item, err
}

func (s *Store) RetryFinalization(ctx context.Context, runID int64, actor int64, reason string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var taskID, attemptID, worker int64
	var status, errorCode string
	var generation int64
	var runNumber int
	if err = tx.QueryRow(ctx, `SELECT task_id,attempt_id,worker_id,generation,status,run_number,COALESCE(error_code,'') FROM workforce_finalization_runs WHERE id=$1 FOR UPDATE`, runID).Scan(&taskID, &attemptID, &worker, &generation, &status, &runNumber, &errorCode); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if status != artifactdomain.FinalizationFailed || errorCode == "FINALIZATION_SOURCE_LOST" {
		return ErrConflict
	}
	var taskStatus string
	if err = tx.QueryRow(ctx, `SELECT status FROM workforce_tasks WHERE id=$1 FOR UPDATE`, taskID).Scan(&taskStatus); err != nil {
		return err
	}
	if taskStatus != "MANUAL_REVIEW" {
		return ErrConflict
	}
	next := s.now().UTC()
	if _, err = tx.Exec(ctx, `UPDATE workforce_finalization_runs SET status='RETRY_WAIT',run_number=run_number+1,max_runs=GREATEST(max_runs,run_number+1),next_retry_at=$2,error_code='',finished_at=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, runID, next); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='PACKAGING',result_summary='',finished_at=NULL,version=version+1 WHERE id=$1 AND status='MANUAL_REVIEW'`, taskID); err != nil {
		return err
	}
	messageID := s.idGen.NextID()
	message := artifactdomain.WorkMessage{SchemaVersion: 1, Kind: artifactdomain.FinalizationMessageKind, MessageID: messageID, WorkerID: worker, ResourceID: runID, Generation: generation}
	body, _ := json.Marshal(message)
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_outbox_events (id,aggregate_type,aggregate_id,event_type,payload_json,status,exchange_name,routing_key,message_expiration_ms,next_retry_at) VALUES ($1,'finalization_run',$2,'finalization.operator_retry',$3,'PENDING','seven.workforce.postprocess',$4,600000,$5)`, messageID, runID, body, fmt.Sprintf("postprocess.%d", worker), next); err != nil {
		return err
	}
	if err = appendPostprocessEvent(ctx, tx, taskID, attemptID, "finalization.retry_requested", map[string]any{"finalizationRunId": runID, "actor": actor, "reason": reason, "runNumber": runNumber + 1}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RetryUpload(ctx context.Context, uploadID int64, actor int64, reason string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var taskID, attemptID, worker int64
	var status string
	var generation int64
	var attemptNumber int
	if err = tx.QueryRow(ctx, `SELECT u.task_id,u.attempt_id,u.worker_id,u.status,u.attempt_number,COALESCE(f.generation,m.generation) FROM workforce_artifact_upload_attempts u LEFT JOIN workforce_finalization_runs f ON f.id=u.finalization_run_id LEFT JOIN workforce_result_materialization_runs m ON m.id=u.materialization_run_id WHERE u.id=$1 FOR UPDATE OF u`, uploadID).Scan(&taskID, &attemptID, &worker, &status, &attemptNumber, &generation); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if status != artifactdomain.UploadFailed {
		return ErrConflict
	}
	var taskStatus string
	if err = tx.QueryRow(ctx, `SELECT status FROM workforce_tasks WHERE id=$1 FOR UPDATE`, taskID).Scan(&taskStatus); err != nil {
		return err
	}
	if taskStatus != "MANUAL_REVIEW" {
		return ErrConflict
	}
	next := s.now().UTC()
	if _, err = tx.Exec(ctx, `UPDATE workforce_artifact_upload_attempts SET status='RETRY_WAIT',attempt_number=attempt_number+1,max_attempts=GREATEST(max_attempts,attempt_number+1),next_retry_at=$2,error_code='',updated_at=CURRENT_TIMESTAMP WHERE id=$1`, uploadID, next); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='UPLOADING',result_summary='',finished_at=NULL,version=version+1 WHERE id=$1 AND status='MANUAL_REVIEW'`, taskID); err != nil {
		return err
	}
	messageID := s.idGen.NextID()
	message := artifactdomain.WorkMessage{SchemaVersion: 1, Kind: artifactdomain.UploadMessageKind, MessageID: messageID, WorkerID: worker, ResourceID: uploadID, Generation: generation}
	body, _ := json.Marshal(message)
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_outbox_events (id,aggregate_type,aggregate_id,event_type,payload_json,status,exchange_name,routing_key,message_expiration_ms,next_retry_at) VALUES ($1,'artifact_upload',$2,'artifact.upload.operator_retry',$3,'PENDING','seven.workforce.postprocess',$4,600000,$5)`, messageID, uploadID, body, fmt.Sprintf("postprocess.%d", worker), next); err != nil {
		return err
	}
	if err = appendPostprocessEvent(ctx, tx, taskID, attemptID, "artifact.upload.retry_requested", map[string]any{"uploadAttemptId": uploadID, "actor": actor, "reason": reason, "attemptNumber": attemptNumber + 1}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func appendPostprocessEvent(ctx context.Context, tx pgx.Tx, taskID int64, attemptID int64, eventType string, payload map[string]any) error {
	return eventlog.Append(ctx, tx, taskID, attemptID, eventType, payload)
}

func (s *Store) ClaimFinalization(ctx context.Context, e dispatchdomain.MachineEnvelope, runID int64) (artifactdomain.FinalizationClaim, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return artifactdomain.FinalizationClaim{}, err
	}
	defer tx.Rollback(ctx)
	var claim artifactdomain.FinalizationClaim
	var workerID int64
	var status, storedInstance, storedBoot, existingLease string
	var runNumber int
	var leaseExpires *time.Time
	err = tx.QueryRow(ctx, `SELECT f.task_id,f.task_turn_id,f.attempt_id,f.execution_node_id,f.generation,f.worker_id,f.status,f.run_number,
  f.worker_instance_id,f.host_boot_id,COALESCE(f.lease_sha256,''),f.lease_expires_at
FROM workforce_finalization_runs f WHERE f.id=$1 FOR UPDATE`, runID).Scan(&claim.TaskID, &claim.TaskTurnID, &claim.AttemptID, &claim.ExecutionNodeID, &claim.Generation, &workerID, &status, &runNumber, &storedInstance, &storedBoot, &existingLease, &leaseExpires)
	if errors.Is(err, pgx.ErrNoRows) {
		return claim, ErrNotFound
	}
	if err != nil {
		return claim, err
	}
	if workerID != e.WorkerID || e.Generation != claim.Generation {
		return claim, ErrConflict
	}
	if status == artifactdomain.FinalizationFrozen || status == artifactdomain.FinalizationFailed || status == artifactdomain.FinalizationCancelled {
		return claim, ErrConflict
	}
	var current, boot string
	if err = tx.QueryRow(ctx, `SELECT w.current_instance_id,s.host_boot_id FROM workforce_workers w JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id WHERE w.id=$1`, e.WorkerID).Scan(&current, &boot); err != nil {
		return claim, err
	}
	if current != e.InstanceID || boot != e.HostBootID {
		return claim, ErrConflict
	}
	lease := randomSecret()
	expires := s.now().UTC().Add(5 * time.Minute)
	if _, err = tx.Exec(ctx, `UPDATE workforce_finalization_runs SET status='RUNNING',worker_instance_id=$2,host_boot_id=$3,lease_sha256=$4,lease_expires_at=$5,started_at=COALESCE(started_at,CURRENT_TIMESTAMP),updated_at=CURRENT_TIMESTAMP WHERE id=$1`, runID, e.InstanceID, e.HostBootID, hash(lease), expires); err != nil {
		return claim, err
	}
	claim.RunID, claim.RunNumber, claim.LeaseToken, claim.LeaseExpiresAt = runID, runNumber, lease, expires.Format(time.RFC3339Nano)
	var item taskdomain.WorkItem
	var attempt taskdomain.Attempt
	var commands, snapshot, completion, resultContract []byte
	var processorEnvironment, processorSecrets, processorParameters []byte
	var processorReleaseID int64
	var processorDigest, processorEnvironmentDigest string
	var maxEvaluationCycles int
	var assignmentID int64
	var attemptSelectionDigest string
	err = tx.QueryRow(ctx, `SELECT t.id,t.tenant_id,t.repository_url,t.repository_ref,turn.instruction,t.validation_commands_json,
  attempt.assignment_id,attempt.assignment_selection_digest,attempt.resolved_execution_snapshot_json,
  turn.processor_release_id,turn.processor_digest,turn.environment_config_digest,turn.environment_snapshot_json,turn.secret_version_identities,turn.parameters_json
FROM workforce_tasks t
JOIN workforce_task_attempts attempt ON attempt.id=$2 AND attempt.task_id=t.id
JOIN workforce_task_turns turn ON turn.id=attempt.task_turn_id AND turn.task_id=t.id
WHERE t.id=$1 AND t.status='PACKAGING'`, claim.TaskID, claim.AttemptID).Scan(
		&item.TaskID, &item.TenantID, &item.RepositoryURL, &item.RepositoryRef, &item.Instruction, &commands,
		&assignmentID, &attemptSelectionDigest, &snapshot,
		&processorReleaseID, &processorDigest, &processorEnvironmentDigest, &processorEnvironment, &processorSecrets, &processorParameters,
	)
	if err != nil {
		return claim, err
	}
	var assignmentSelectionDigest string
	var assignmentSnapshot []byte
	err = tx.QueryRow(ctx, `SELECT h.alias,assignment.agent_runtime_release_id,assignment.runtime_image_ref,assignment.runtime_image_digest,assignment.resolved_execution_snapshot_json,
  credential.name,assignment.primary_model,COALESCE((assignment.resolved_execution_snapshot_json->>'executionDeadlineSeconds')::integer,0),assignment.selection_digest
FROM workforce_task_assignments assignment
JOIN workforce_harness_releases h ON h.id=assignment.agent_runtime_release_id AND h.image_digest=assignment.runtime_image_digest
JOIN workforce_model_credentials credential ON credential.id=assignment.model_credential_id AND credential.tenant_id=$2
WHERE assignment.id=$1 AND assignment.task_id=$3`, assignmentID, item.TenantID, item.TaskID).Scan(
		&item.HarnessAlias, &item.HarnessReleaseID, &item.HarnessImage, &item.HarnessImageDigest, &assignmentSnapshot,
		&item.ModelCredentialName, &item.PrimaryModel, &item.HarnessTimeoutSeconds, &assignmentSelectionDigest,
	)
	if err != nil {
		return claim, err
	}
	if attemptSelectionDigest != assignmentSelectionDigest || !bytes.Equal(snapshot, assignmentSnapshot) {
		return claim, ErrConflict
	}
	if err = tx.QueryRow(ctx, `SELECT COALESCE((manifest_json->>'maxEvaluationCycles')::integer,3) FROM workforce_result_processor_releases WHERE id=$1 AND archive_sha256=$2`, processorReleaseID, processorDigest).Scan(&maxEvaluationCycles); err != nil {
		return claim, err
	}
	err = tx.QueryRow(ctx, `SELECT a.id,a.generation,a.workspace_path,COALESCE(a.base_commit,''),p.raw_json FROM workforce_task_attempts a JOIN workforce_completion_proposals p ON p.attempt_id=a.id WHERE a.id=$1 AND p.validation_status='ACCEPTED'`, claim.AttemptID).Scan(&attempt.ID, &attempt.Generation, &attempt.WorkspacePath, &item.ResolvedBaseCommit, &completion)
	if err != nil {
		return claim, err
	}
	if err = tx.QueryRow(ctx, `SELECT id,contract_json FROM workforce_result_evaluation_cycles WHERE attempt_id=$1 AND decision='PASS' AND status='PERSISTED'`, claim.AttemptID).Scan(&claim.EvaluationCycleID, &resultContract); err != nil {
		return claim, err
	}
	claim.ResultContract = append(json.RawMessage(nil), resultContract...)
	item.TaskTurnID = claim.TaskTurnID
	_ = json.Unmarshal(commands, &item.ValidationCommands)
	_ = json.Unmarshal(snapshot, &item.ExecutionSnapshot)
	item.ExecutionSnapshot.ResultProcessorReleaseID = processorReleaseID
	item.ExecutionSnapshot.ResultProcessorDigest = processorDigest
	item.ExecutionSnapshot.EnvironmentConfigDigest = processorEnvironmentDigest
	item.ExecutionSnapshot.ResultProcessorEnvironment = append(json.RawMessage(nil), processorEnvironment...)
	item.ExecutionSnapshot.SecretVersionIdentities = append(json.RawMessage(nil), processorSecrets...)
	item.ExecutionSnapshot.ResultProcessorParameters = append(json.RawMessage(nil), processorParameters...)
	item.ExecutionSnapshot.MaxEvaluationCycles = maxEvaluationCycles
	item.ResultProcessorParameters = append(json.RawMessage(nil), processorParameters...)
	attempt.TaskID, attempt.ExecutionNodeID = claim.TaskID, claim.ExecutionNodeID
	claim.WorkItem, _ = json.Marshal(item)
	claim.Attempt, _ = json.Marshal(attempt)
	claim.Completion = completion
	if err = tx.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(jsonb_build_object('ordinal',ordinal,'name',name,'commandDigest',command_digest,'status',status,'exitCode',COALESCE(exit_code,-1),'timeoutSeconds',timeout_seconds,'output',COALESCE(output,'')) ORDER BY ordinal),'[]'::jsonb) FROM workforce_check_runs WHERE attempt_id=$1`, claim.AttemptID).Scan(&claim.Checks); err != nil {
		return claim, err
	}
	if err = tx.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(jsonb_build_object('id',id::text,'taskId',task_id::text,'attemptId',attempt_id::text,'type',event_type,'sequence',sequence::text,'payload',payload_json,'createdAt',to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')) ORDER BY id),'[]'::jsonb) FROM workforce_task_events WHERE task_id=$1`, claim.TaskID).Scan(&claim.Events); err != nil {
		return claim, err
	}
	if err = tx.Commit(ctx); err != nil {
		return claim, err
	}
	return claim, nil
}

func (s *Store) CompleteFreeze(ctx context.Context, e dispatchdomain.MachineEnvelope, runID int64, lease string, result artifactdomain.FreezeResult) ([]artifactdomain.WorkMessage, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var taskID, attemptID, workerID int64
	var instanceID, boot, status, storedLease string
	var generation int64
	var expires time.Time
	err = tx.QueryRow(ctx, `SELECT task_id,attempt_id,worker_id,worker_instance_id,host_boot_id,generation,status,COALESCE(lease_sha256,''),lease_expires_at FROM workforce_finalization_runs WHERE id=$1 FOR UPDATE`, runID).Scan(&taskID, &attemptID, &workerID, &instanceID, &boot, &generation, &status, &storedLease, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if workerID != e.WorkerID || instanceID != e.InstanceID || boot != e.HostBootID || generation != e.Generation || storedLease != hash(lease) || !expires.After(s.now()) || status != artifactdomain.FinalizationRunning || len(result.ManifestDigest) != 64 || len(result.SpoolIdentity) != 64 || len(result.Artifacts) < 1 {
		return nil, ErrConflict
	}
	var taskStatus, attemptStatus string
	if err = tx.QueryRow(ctx, `SELECT t.status,a.status FROM workforce_tasks t JOIN workforce_task_attempts a ON a.id=$2 WHERE t.id=$1 FOR UPDATE OF t,a`, taskID, attemptID).Scan(&taskStatus, &attemptStatus); err != nil {
		return nil, err
	}
	if taskStatus != "PACKAGING" || (attemptStatus != "EXECUTION_COMPLETED" && attemptStatus != "FINALIZING") {
		return nil, ErrConflict
	}
	// A P0.9 finalization is tied to exactly one immutable PASS cycle and its
	// Turn.  The worker's FreezeResult carries only independently recomputed
	// bytes and hashes; this query binds those trusted bytes back to the
	// Server-persisted Processor declaration.
	var taskTurnID, evaluationCycleID int64
	if err = tx.QueryRow(ctx, `SELECT a.task_turn_id,c.id
		FROM workforce_task_attempts a
		JOIN workforce_result_evaluation_cycles c ON c.attempt_id=a.id
		WHERE a.id=$1 AND c.decision='PASS' AND c.status='PERSISTED'`, attemptID).Scan(&taskTurnID, &evaluationCycleID); err != nil {
		return nil, err
	}
	if taskTurnID == 0 || evaluationCycleID == 0 {
		return nil, ErrConflict
	}
	var checkpointCommit, checkpointTree string
	if err = tx.QueryRow(ctx, `SELECT result_commit,result_tree FROM workforce_continuation_checkpoints WHERE attempt_id=$1 AND evaluation_cycle_id=$2 AND decision='PASS' AND status='AVAILABLE'`, attemptID, evaluationCycleID).Scan(&checkpointCommit, &checkpointTree); err != nil {
		return nil, err
	}
	if checkpointCommit != result.ResultCommit || checkpointTree != result.ResultTree {
		return nil, ErrConflict
	}
	manifest, _ := json.Marshal(result)
	if _, err = tx.Exec(ctx, `UPDATE workforce_finalization_runs SET status='FROZEN',result_commit=$2,result_tree=$3,manifest_digest=$4,manifest_json=$5,spool_identity=$6,frozen_at=CURRENT_TIMESTAMP,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, runID, result.ResultCommit, result.ResultTree, result.ManifestDigest, manifest, result.SpoolIdentity); err != nil {
		return nil, err
	}
	messages := make([]artifactdomain.WorkMessage, 0, len(result.Artifacts))
	for _, artifact := range result.Artifacts {
		if len(artifact.SHA256) != 64 || artifact.SizeBytes < 0 || artifact.Name == "" || artifact.Path == "" {
			return nil, ErrConflict
		}
		if artifact.Type == "RESULT_OUTPUT" {
			if artifact.SourcePath == "" {
				return nil, ErrConflict
			}
			if _, err = tx.Exec(ctx, `INSERT INTO workforce_result_outputs
				(id,task_id,task_turn_id,attempt_id,evaluation_cycle_id,name,source_path,storage,sha256,size_bytes,status)
				VALUES ($1,$2,$3,$4,$5,$6,$7,'IMMEDIATE',$8,$9,'FROZEN')
				ON CONFLICT (evaluation_cycle_id,name) DO UPDATE
				SET source_path=EXCLUDED.source_path,sha256=EXCLUDED.sha256,size_bytes=EXCLUDED.size_bytes,status='FROZEN'
				WHERE workforce_result_outputs.attempt_id=EXCLUDED.attempt_id`,
				s.idGen.NextID(), taskID, taskTurnID, attemptID, evaluationCycleID, artifact.Name, artifact.SourcePath, artifact.SHA256, artifact.SizeBytes); err != nil {
				return nil, err
			}
		}
		uploadID, messageID := s.idGen.NextID(), s.idGen.NextID()
		objectKey := fmt.Sprintf("tasks/%d/attempts/%d/finalization/%d/%s/%s", taskID, attemptID, runID, artifact.SHA256, artifact.Name)
		if _, err = tx.Exec(ctx, `INSERT INTO workforce_artifact_upload_attempts (id,finalization_run_id,task_id,task_turn_id,attempt_id,artifact_type,artifact_name,object_key,sha256,size_bytes,media_type,worker_id,host_boot_id,status) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'PENDING')`, uploadID, runID, taskID, taskTurnID, attemptID, artifact.Type, artifact.Name, objectKey, artifact.SHA256, artifact.SizeBytes, artifact.MediaType, workerID, boot); err != nil {
			return nil, err
		}
		message := artifactdomain.WorkMessage{SchemaVersion: 1, Kind: artifactdomain.UploadMessageKind, MessageID: messageID, WorkerID: workerID, ResourceID: uploadID, Generation: generation}
		body, _ := json.Marshal(message)
		if _, err = tx.Exec(ctx, `INSERT INTO workforce_outbox_events (id,aggregate_type,aggregate_id,event_type,payload_json,status,exchange_name,routing_key,message_expiration_ms) VALUES ($1,'artifact_upload',$2,'artifact.upload.requested',$3,'PENDING','seven.workforce.postprocess',$4,600000)`, messageID, uploadID, body, fmt.Sprintf("postprocess.%d", workerID)); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	if tag, updateErr := tx.Exec(ctx, `UPDATE workforce_tasks SET status='UPLOADING',version=version+1 WHERE id=$1 AND status='PACKAGING'`, taskID); updateErr != nil {
		return nil, updateErr
	} else if tag.RowsAffected() != 1 {
		return nil, ErrConflict
	}
	if tag, updateErr := tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='UPLOADING',updated_at=CURRENT_TIMESTAMP WHERE id=(SELECT execution_node_id FROM workforce_finalization_runs WHERE id=$1) AND status='PACKAGING'`, runID); updateErr != nil {
		return nil, updateErr
	} else if tag.RowsAffected() != 1 {
		return nil, ErrConflict
	}
	return messages, tx.Commit(ctx)
}

func (s *Store) FailFinalization(ctx context.Context, e dispatchdomain.MachineEnvelope, runID int64, lease string, reason string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var worker int64
	var status, instance, stored string
	var run, max int
	if err = tx.QueryRow(ctx, `SELECT status,worker_id,worker_instance_id,COALESCE(lease_sha256,''),run_number,max_runs FROM workforce_finalization_runs WHERE id=$1 FOR UPDATE`, runID).Scan(&status, &worker, &instance, &stored, &run, &max); err != nil {
		return err
	}
	if status != "RUNNING" || worker != e.WorkerID || instance != e.InstanceID || stored != hash(lease) {
		return ErrConflict
	}
	if reason == "FINALIZATION_SOURCE_LOST" {
		var taskID, attemptID, nodeID, tenantID, turnID int64
		var attemptCount, maxAttempts int
		var generation int64
		if err = tx.QueryRow(ctx, `SELECT f.task_id,f.attempt_id,f.execution_node_id,t.attempt_count,n.max_attempts,n.generation,t.tenant_id,f.task_turn_id FROM workforce_finalization_runs f JOIN workforce_tasks t ON t.id=f.task_id JOIN workforce_execution_nodes n ON n.id=f.execution_node_id WHERE f.id=$1`, runID).Scan(&taskID, &attemptID, &nodeID, &attemptCount, &maxAttempts, &generation, &tenantID, &turnID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_finalization_runs SET status='FAILED',error_code=$2,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, runID, reason); err != nil {
			return err
		}
		if attemptCount < maxAttempts {
			next := s.now().UTC().Add(time.Second)
			if _, err = tx.Exec(ctx, `UPDATE workforce_task_attempts SET status='FAILED_RETRYABLE',retry_class='INFRASTRUCTURE',error_code=$2,finished_at=CURRENT_TIMESTAMP WHERE id=$1`, attemptID, reason); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='RETRY_WAIT',generation=$2,retry_count=retry_count+1,next_retry_at=$3,last_retry_reason_code=$4,wait_reason_code='ASSIGNMENT_RETRY_PENDING',updated_at=CURRENT_TIMESTAMP WHERE id=$1`, nodeID, generation+1, next, reason); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='RETRY_WAIT',result_summary=$2,version=version+1 WHERE id=$1 AND status='PACKAGING'`, taskID, reason); err != nil {
				return err
			}
		} else {
			if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='FAILED',result_summary='AGENT_RETRY_EXHAUSTED',finished_at=CURRENT_TIMESTAMP,version=version+1 WHERE id=$1`, taskID); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='FAILED',updated_at=CURRENT_TIMESTAMP WHERE id=$1`, nodeID); err != nil {
				return err
			}
			occurrence, appendErr := eventlog.AppendWithSequence(ctx, tx, taskID, attemptID, "task.failed", map[string]any{"reasonCode": reason})
			if appendErr != nil {
				return appendErr
			}
			if s.notificationRecorder != nil {
				if _, err = s.notificationRecorder.RecordTransition(ctx, tx, tasknotification.TaskNotificationTransition{TenantID: tenantID, TaskID: taskID, TaskTurnID: turnID, SourceType: tasknotification.SourceFinalizationRun, SourceID: runID, SourceOccurrence: occurrence, PreviousStatus: "PACKAGING", CurrentStatus: "FAILED", ReasonCode: reason, Recoverability: tasknotification.RecoverabilityTerminal, RetryBudgetExhausted: true, OccurredAt: s.now().UTC()}); err != nil {
					return err
				}
			}
		}
		return tx.Commit(ctx)
	}
	if run >= max {
		var taskID, turnID, tenantID, attemptID int64
		if err = tx.QueryRow(ctx, `SELECT f.task_id,f.task_turn_id,t.tenant_id,f.attempt_id FROM workforce_finalization_runs f JOIN workforce_tasks t ON t.id=f.task_id WHERE f.id=$1`, runID).Scan(&taskID, &turnID, &tenantID, &attemptID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE workforce_finalization_runs SET status='FAILED',error_code=$2,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, runID, reason)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='MANUAL_REVIEW',result_summary='FINALIZATION_RETRY_EXHAUSTED',version=version+1 WHERE id=(SELECT task_id FROM workforce_finalization_runs WHERE id=$1)`, runID)
		}
		if err == nil {
			var occurrence int64
			occurrence, err = eventlog.AppendWithSequence(ctx, tx, taskID, attemptID, "finalization.attention_required", map[string]any{"finalizationRunId": runID, "reasonCode": "FINALIZATION_RETRY_EXHAUSTED"})
			if err == nil && s.notificationRecorder != nil {
				_, err = s.notificationRecorder.RecordTransition(ctx, tx, tasknotification.TaskNotificationTransition{TenantID: tenantID, TaskID: taskID, TaskTurnID: turnID, SourceType: tasknotification.SourceFinalizationRun, SourceID: runID, SourceOccurrence: occurrence, PreviousStatus: "RUNNING", CurrentStatus: "MANUAL_REVIEW", ReasonCode: "FINALIZATION_RETRY_EXHAUSTED", Recoverability: tasknotification.RecoverabilityUserRecoverable, OccurredAt: s.now().UTC()})
			}
		}
		return tx.Commit(ctx)
	}
	next := s.now().UTC().Add([]time.Duration{time.Second, 10 * time.Second, 60 * time.Second}[minInt(run-1, 2)])
	_, err = tx.Exec(ctx, `UPDATE workforce_finalization_runs SET status='RETRY_WAIT',run_number=run_number+1,next_retry_at=$2,error_code=$3,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, runID, next, reason)
	if err == nil {
		messageID := s.idGen.NextID()
		message := artifactdomain.WorkMessage{SchemaVersion: 1, Kind: artifactdomain.FinalizationMessageKind, MessageID: messageID, WorkerID: worker, ResourceID: runID, Generation: e.Generation}
		body, _ := json.Marshal(message)
		_, err = tx.Exec(ctx, `INSERT INTO workforce_outbox_events (id,aggregate_type,aggregate_id,event_type,payload_json,status,exchange_name,routing_key,message_expiration_ms,next_retry_at) VALUES ($1,'finalization_run',$2,'finalization.retry',$3,'PENDING','seven.workforce.postprocess',$4,600000,$5)`, messageID, runID, body, fmt.Sprintf("postprocess.%d", worker), next)
	}
	return func() error {
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}()
}

func (s *Store) ClaimUpload(ctx context.Context, e dispatchdomain.MachineEnvelope, uploadID int64) (artifactdomain.UploadClaim, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return artifactdomain.UploadClaim{}, err
	}
	defer tx.Rollback(ctx)
	var c artifactdomain.UploadClaim
	var worker int64
	var boot, status string
	var manifest json.RawMessage
	err = tx.QueryRow(ctx, `SELECT u.id,COALESCE(u.finalization_run_id,0),COALESCE(u.materialization_run_id,0),u.task_id,u.task_turn_id,u.attempt_id,u.artifact_type,u.artifact_name,u.object_key,u.sha256,u.size_bytes,u.media_type,u.attempt_number,u.worker_id,u.host_boot_id,u.status,COALESCE(f.spool_identity,m.spool_identity),COALESCE(f.manifest_json,m.manifest_json)
		FROM workforce_artifact_upload_attempts u
		LEFT JOIN workforce_finalization_runs f ON f.id=u.finalization_run_id
		LEFT JOIN workforce_result_materialization_runs m ON m.id=u.materialization_run_id
		WHERE u.id=$1 FOR UPDATE OF u`, uploadID).Scan(&c.ID, &c.FinalizationRunID, &c.MaterializationRunID, &c.TaskID, &c.TaskTurnID, &c.AttemptID, &c.ArtifactType, &c.Name, &c.ObjectKey, &c.SHA256, &c.SizeBytes, &c.MediaType, &c.AttemptNumber, &worker, &boot, &status, &c.SpoolIdentity, &manifest)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, err
	}
	if worker != e.WorkerID || boot != e.HostBootID || (status != "PENDING" && status != "RETRY_WAIT") {
		return c, ErrConflict
	}
	var taskStatus string
	if err = tx.QueryRow(ctx, `SELECT status FROM workforce_tasks WHERE id=$1 FOR UPDATE`, c.TaskID).Scan(&taskStatus); err != nil {
		return c, err
	}
	if c.MaterializationRunID == 0 && taskStatus != "UPLOADING" {
		return c, ErrConflict
	}
	if c.MaterializationRunID != 0 && (taskStatus == "CANCELLED" || taskStatus == "CANCELLING") {
		return c, ErrConflict
	}
	var current, currentBoot string
	if err = tx.QueryRow(ctx, `SELECT w.current_instance_id,s.host_boot_id FROM workforce_workers w JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id WHERE w.id=$1`, e.WorkerID).Scan(&current, &currentBoot); err != nil {
		return c, err
	}
	if current != e.InstanceID || currentBoot != e.HostBootID {
		return c, ErrConflict
	}
	if c.MaterializationRunID == 0 {
		c.SpoolKind = "FINALIZATION"
		var frozen artifactdomain.FreezeResult
		if json.Unmarshal(manifest, &frozen) != nil {
			return c, ErrConflict
		}
		for _, a := range frozen.Artifacts {
			if a.Type == c.ArtifactType && a.SHA256 == c.SHA256 {
				c.SpoolRelativePath = a.Path
			}
		}
	} else {
		c.SpoolKind = "MATERIALIZATION"
		var frozen struct{ Path, SHA256 string }
		if json.Unmarshal(manifest, &frozen) != nil || frozen.SHA256 != c.SHA256 {
			return c, ErrConflict
		}
		c.SpoolRelativePath = frozen.Path
	}
	if c.SpoolRelativePath == "" {
		return c, ErrConflict
	}
	lease := randomSecret()
	expires := s.now().UTC().Add(2 * time.Minute)
	c.LeaseToken = lease
	c.LeaseExpiresAt = expires.Format(time.RFC3339Nano)
	_, err = tx.Exec(ctx, `UPDATE workforce_artifact_upload_attempts SET status='UPLOADING',lease_sha256=$2,lease_expires_at=$3,started_at=COALESCE(started_at,CURRENT_TIMESTAMP),updated_at=CURRENT_TIMESTAMP WHERE id=$1`, uploadID, hash(lease), expires)
	if err != nil {
		return c, err
	}
	return c, tx.Commit(ctx)
}

func (s *Store) FailUpload(ctx context.Context, e dispatchdomain.MachineEnvelope, uploadID int64, lease, reason string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var worker, runID, materializationRunID int64
	var status, boot, stored string
	var attemptNumber, maxAttempts int
	err = tx.QueryRow(ctx, `SELECT status,worker_id,host_boot_id,COALESCE(lease_sha256,''),attempt_number,max_attempts,COALESCE(finalization_run_id,0),COALESCE(materialization_run_id,0) FROM workforce_artifact_upload_attempts WHERE id=$1 FOR UPDATE`, uploadID).Scan(&status, &worker, &boot, &stored, &attemptNumber, &maxAttempts, &runID, &materializationRunID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if worker != e.WorkerID || boot != e.HostBootID || stored != hash(lease) || (status != artifactdomain.UploadUploading && status != artifactdomain.UploadVerifying) {
		return ErrConflict
	}
	if attemptNumber >= maxAttempts {
		var taskID, turnID, tenantID, attemptID int64
		if err = tx.QueryRow(ctx, `SELECT u.task_id,u.task_turn_id,t.tenant_id,COALESCE(u.attempt_id,0) FROM workforce_artifact_upload_attempts u JOIN workforce_tasks t ON t.id=u.task_id WHERE u.id=$1`, uploadID).Scan(&taskID, &turnID, &tenantID, &attemptID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_artifact_upload_attempts SET status='FAILED',error_code=$2,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, uploadID, reason); err != nil {
			return err
		}
		if materializationRunID == 0 {
			if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='MANUAL_REVIEW',result_summary='ARTIFACT_UPLOAD_RETRY_EXHAUSTED',version=version+1 WHERE id=(SELECT task_id FROM workforce_artifact_upload_attempts WHERE id=$1) AND status='UPLOADING'`, uploadID); err != nil {
				return err
			}
		} else {
			if _, err = tx.Exec(ctx, `UPDATE workforce_result_materialization_runs SET status='FAILED',reason_code='ARTIFACT_UPLOAD_RETRY_EXHAUSTED',updated_at=CURRENT_TIMESTAMP WHERE id=$1`, materializationRunID); err != nil {
				return err
			}
		}
		var occurrence int64
		occurrence, err = eventlog.AppendWithSequence(ctx, tx, taskID, attemptID, "artifact.upload.attention_required", map[string]any{"artifactUploadId": uploadID, "reasonCode": "ARTIFACT_UPLOAD_RETRY_EXHAUSTED"})
		if err != nil {
			return err
		}
		if s.notificationRecorder != nil {
			if _, err = s.notificationRecorder.RecordTransition(ctx, tx, tasknotification.TaskNotificationTransition{TenantID: tenantID, TaskID: taskID, TaskTurnID: turnID, SourceType: tasknotification.SourceArtifactUpload, SourceID: uploadID, SourceOccurrence: occurrence, PreviousStatus: status, CurrentStatus: "MANUAL_REVIEW", ReasonCode: "ARTIFACT_UPLOAD_RETRY_EXHAUSTED", Recoverability: tasknotification.RecoverabilityUserRecoverable, OccurredAt: s.now().UTC()}); err != nil {
				return err
			}
		}
		return tx.Commit(ctx)
	}
	backoffs := []time.Duration{time.Second, 5 * time.Second, 30 * time.Second, 120 * time.Second, 300 * time.Second}
	next := s.now().UTC().Add(backoffs[minInt(attemptNumber-1, len(backoffs)-1)])
	if _, err = tx.Exec(ctx, `UPDATE workforce_artifact_upload_attempts SET status='RETRY_WAIT',attempt_number=attempt_number+1,next_retry_at=$2,error_code=$3,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, uploadID, next, reason); err != nil {
		return err
	}
	messageID := s.idGen.NextID()
	message := artifactdomain.WorkMessage{SchemaVersion: 1, Kind: artifactdomain.UploadMessageKind, MessageID: messageID, WorkerID: worker, ResourceID: uploadID, Generation: e.Generation}
	body, _ := json.Marshal(message)
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_outbox_events (id,aggregate_type,aggregate_id,event_type,payload_json,status,exchange_name,routing_key,message_expiration_ms,next_retry_at) VALUES ($1,'artifact_upload',$2,'artifact.upload.retry',$3,'PENDING','seven.workforce.postprocess',$4,600000,$5)`, messageID, uploadID, body, fmt.Sprintf("postprocess.%d", worker), next); err != nil {
		return err
	}
	_ = runID
	return tx.Commit(ctx)
}

func (s *Store) VerifyUpload(ctx context.Context, e dispatchdomain.MachineEnvelope, uploadID int64, lease string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var occurredAt time.Time
	if err = tx.QueryRow(ctx, `SELECT CURRENT_TIMESTAMP`).Scan(&occurredAt); err != nil {
		return err
	}
	var taskID, attemptID, runID, materializationRunID, taskTurnID, worker int64
	var typ, name, key, digest, media, boot, status, stored string
	var size int64
	var expires time.Time
	err = tx.QueryRow(ctx, `SELECT task_id,task_turn_id,attempt_id,COALESCE(finalization_run_id,0),COALESCE(materialization_run_id,0),artifact_type,artifact_name,object_key,sha256,size_bytes,media_type,worker_id,host_boot_id,status,COALESCE(lease_sha256,''),lease_expires_at FROM workforce_artifact_upload_attempts WHERE id=$1 FOR UPDATE`, uploadID).Scan(&taskID, &taskTurnID, &attemptID, &runID, &materializationRunID, &typ, &name, &key, &digest, &size, &media, &worker, &boot, &status, &stored, &expires)
	if err != nil {
		return err
	}
	if worker != e.WorkerID || boot != e.HostBootID || status != "UPLOADING" || stored != hash(lease) || !expires.After(s.now()) {
		return ErrConflict
	}
	var tenantID int64
	var taskStatus string
	if err = tx.QueryRow(ctx, `SELECT tenant_id,status FROM workforce_tasks WHERE id=$1 FOR UPDATE`, taskID).Scan(&tenantID, &taskStatus); err != nil {
		return err
	}
	if materializationRunID == 0 && taskStatus != "UPLOADING" {
		return ErrConflict
	}
	if materializationRunID != 0 && (taskStatus == "CANCELLED" || taskStatus == "CANCELLING") {
		return ErrConflict
	}
	_, err = tx.Exec(ctx, `UPDATE workforce_artifact_upload_attempts SET status='VERIFIED',uploaded_at=CURRENT_TIMESTAMP,verified_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, uploadID)
	if err != nil {
		return err
	}
	metadata, _ := json.Marshal(map[string]any{"finalizationRunId": runID, "materializationRunId": materializationRunID, "verifiedAfterUpload": true})
	var artifactID int64
	err = tx.QueryRow(ctx, `INSERT INTO workforce_artifacts (id,task_id,attempt_id,artifact_type,name,object_key,sha256,size_bytes,media_type,status,metadata_json)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'AVAILABLE',$10)
		ON CONFLICT (object_key) DO UPDATE SET status=CASE WHEN workforce_artifacts.status='PENDING_REVIEW' THEN 'PENDING_REVIEW' ELSE 'AVAILABLE' END
		WHERE workforce_artifacts.sha256=EXCLUDED.sha256
		RETURNING id`, s.idGen.NextID(), taskID, attemptID, typ, name, key, digest, size, media, metadata).Scan(&artifactID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if typ == "RESULT_OUTPUT" {
		tag, updateErr := tx.Exec(ctx, `UPDATE workforce_result_outputs
			SET artifact_id=$1,status='AVAILABLE'
			WHERE task_id=$2 AND attempt_id=$3 AND name=$4 AND sha256=$5
			AND storage='IMMEDIATE' AND status IN ('FROZEN','AVAILABLE')`, artifactID, taskID, attemptID, name, digest)
		if updateErr != nil {
			return updateErr
		}
		if tag.RowsAffected() != 1 {
			return ErrConflict
		}
	}
	if materializationRunID != 0 {
		if _, err = tx.Exec(ctx, `UPDATE workforce_result_materialization_runs SET status='AVAILABLE',artifact_id=$2,artifact_sha256=$3,artifact_size_bytes=$4,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND task_turn_id=$5 AND status='FROZEN'`, materializationRunID, artifactID, digest, size, taskTurnID); err != nil {
			return err
		}
		if strings.HasPrefix(typ, "RESULT_DECLARED_OUTPUT") {
			if _, err = tx.Exec(ctx, `UPDATE workforce_result_outputs SET artifact_id=$2,sha256=$3,size_bytes=$4,status='AVAILABLE' WHERE id=(SELECT output_id FROM workforce_result_materialization_runs WHERE id=$1) AND task_turn_id=$5 AND storage IN ('IMMEDIATE','ON_DEMAND')`, materializationRunID, artifactID, digest, size, taskTurnID); err != nil {
				return err
			}
		}
		return tx.Commit(ctx)
	}
	var pending int
	if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM workforce_artifact_upload_attempts WHERE finalization_run_id=$1 AND status<>'VERIFIED'`, runID).Scan(&pending); err != nil {
		return err
	}
	if pending == 0 {
		var node, taskTurnID int64
		var commit, tree string
		if err = tx.QueryRow(ctx, `SELECT result_commit,result_tree,execution_node_id,task_turn_id FROM workforce_finalization_runs WHERE id=$1`, runID).Scan(&commit, &tree, &node, &taskTurnID); err != nil {
			return err
		}
		if s.reviewGate != nil {
			decision, gateErr := s.reviewGate.GateVerifiedFinalization(ctx, tx, reviewrun.FinalizationInput{
				TenantID: tenantID, TaskID: taskID, TaskTurnID: taskTurnID, AttemptID: attemptID,
				FinalizationRunID: runID, ResultCommit: commit, ResultTree: tree,
			})
			if gateErr != nil {
				return fmt.Errorf("independent review finalization gate: %w", gateErr)
			}
			if decision.ReviewRequired {
				return tx.Commit(ctx)
			}
		} else {
			// A review-enabled Task must never silently follow the historical
			// success branch when an embedding forgot to wire the ReviewRun
			// facade. Normal module installation always wires it; this guard
			// protects direct Store construction and future composition changes.
			var reviewRequired bool
			if gateErr := tx.QueryRow(ctx, `SELECT review_enabled FROM workforce_tasks WHERE id=$1`, taskID).Scan(&reviewRequired); gateErr != nil {
				return fmt.Errorf("load independent review gate requirement: %w", gateErr)
			}
			if reviewRequired {
				return errors.New("INDEPENDENT_REVIEW_GATE_UNAVAILABLE")
			}
		}
		if tag, updateErr := tx.Exec(ctx, `UPDATE workforce_task_attempts SET status='SUCCEEDED',result_commit=$2,result_tree=$3,finished_at=CURRENT_TIMESTAMP,exit_code=0 WHERE id=$1 AND status IN ('EXECUTION_COMPLETED','FINALIZING')`, attemptID, commit, tree); updateErr != nil {
			return updateErr
		} else if tag.RowsAffected() != 1 {
			return ErrConflict
		}
		if tag, updateErr := tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='SUCCEEDED',updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='UPLOADING'`, node); updateErr != nil {
			return updateErr
		} else if tag.RowsAffected() != 1 {
			return ErrConflict
		}
		if tag, updateErr := tx.Exec(ctx, `UPDATE workforce_task_turns SET status='SUCCEEDED',result_commit=$2,result_tree=$3,result_summary='Finalization and artifact verification passed',finished_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='ACTIVE'`, taskTurnID, commit, tree); updateErr != nil {
			return updateErr
		} else if tag.RowsAffected() != 1 {
			return ErrConflict
		}
		if tag, updateErr := tx.Exec(ctx, `UPDATE workforce_tasks SET status='SUCCEEDED',result_summary='Finalization and artifact verification passed',verified_at=CURRENT_TIMESTAMP,finished_at=CURRENT_TIMESTAMP,version=version+1 WHERE id=$1 AND current_task_turn_id=$2 AND status='UPLOADING'`, taskID, taskTurnID); updateErr != nil {
			return updateErr
		} else if tag.RowsAffected() != 1 {
			return ErrConflict
		}
		var notificationOccurrence int64
		if notificationOccurrence, err = eventlog.AppendWithSequence(ctx, tx, taskID, attemptID, "task.succeeded", map[string]any{"taskTurnId": taskTurnID, "resultCommit": commit, "resultTree": tree}); err != nil {
			return err
		}
		if err = s.recordTaskCompletedNotification(ctx, tx, tenantID, taskID, taskTurnID, runID, notificationOccurrence, occurredAt); err != nil {
			return fmt.Errorf("TASK_COMPLETED_NOTIFICATION_PERSIST_FAILED: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) recordTaskCompletedNotification(ctx context.Context, tx pgx.Tx, tenantID, taskID, taskTurnID, finalizationRunID, sourceOccurrence int64, occurredAt time.Time) error {
	if s == nil || s.notificationRecorder == nil {
		return nil
	}
	_, err := s.notificationRecorder.RecordTransition(ctx, tx, tasknotification.TaskNotificationTransition{
		TenantID: tenantID, TaskID: taskID, TaskTurnID: taskTurnID,
		SourceType: tasknotification.SourceFinalizationRun, SourceID: finalizationRunID, SourceOccurrence: sourceOccurrence,
		ReasonCode: "TASK_TURN_SUCCEEDED", Recoverability: tasknotification.RecoverabilityTerminal, OccurredAt: occurredAt.UTC(),
	})
	return err
}

func randomSecret() string { b := make([]byte, 32); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func hash(v string) string { sum := sha256.Sum256([]byte(v)); return hex.EncodeToString(sum[:]) }
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
