package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/eventlog"
	wcp "github.com/CaixyPromise/seven-agent/force/internal/worker/checkpoint"
	"github.com/jackc/pgx/v5"
)

func (s *Store) BeginInteractionTx(ctx context.Context) (pgx.Tx, error) { return s.db.Begin(ctx) }

func (s *Store) LoadInteractionBudgetBaselineTx(ctx context.Context, tx pgx.Tx, taskID int64) (int64, *wcp.ExecutionBudget, error) {
	var attemptSnapshot, body []byte
	var turnID, attemptID int64
	err := tx.QueryRow(ctx, `SELECT CASE WHEN a.id IS NULL THEN NULL ELSE a.resolved_execution_snapshot_json END,
       tr.id,COALESCE(t.latest_attempt_id,0)
FROM workforce_tasks t
JOIN workforce_task_turns tr ON tr.id=t.current_task_turn_id AND tr.task_id=t.id
LEFT JOIN workforce_task_attempts a ON a.id=t.latest_attempt_id AND a.task_turn_id=tr.id AND a.task_id=t.id
WHERE t.id=$1
FOR UPDATE OF t`, taskID).Scan(&attemptSnapshot, &turnID, &attemptID)
	if err != nil {
		return 0, nil, err
	}
	if attemptID > 0 && len(attemptSnapshot) == 0 {
		return 0, nil, domain.ErrInteractionSuspend
	}
	limit, err := interactionActiveLimitMillis(attemptSnapshot)
	if err != nil {
		return 0, nil, err
	}
	err = tx.QueryRow(ctx, `SELECT COALESCE(r.interaction_execution_budget_json,c.interaction_manifest_json->'executionBudget') FROM workforce_task_turns tr LEFT JOIN workforce_continuation_checkpoints c ON c.id=tr.source_checkpoint_id LEFT JOIN workforce_continuation_restore_runs r ON r.target_turn_id=tr.id AND r.attempt_id=$2 WHERE tr.id=$1`, turnID, attemptID).Scan(&body)
	if err != nil {
		return 0, nil, err
	}
	if len(body) == 0 {
		return limit, nil, nil
	}
	var previous wcp.ExecutionBudget
	if json.Unmarshal(body, &previous) != nil || previous.Validate() != nil {
		return 0, nil, domain.ErrInteractionSuspend
	}
	return limit, &previous, nil
}

type interactionExecutionSnapshot struct {
	ExecutionDeadlineSeconds *int64 `json:"executionDeadlineSeconds"`
}

// interactionActiveLimitMillis resolves the business execution limit without
// consulting Project configuration.
// Once an Attempt exists its immutable resolved snapshot is authoritative;
// before that, the frozen Project execution policy is the only valid source.
func interactionActiveLimitMillis(attemptSnapshot []byte) (int64, error) {
	seconds := int64(0)
	if len(attemptSnapshot) > 0 && string(attemptSnapshot) != "null" {
		var snapshot interactionExecutionSnapshot
		if err := json.Unmarshal(attemptSnapshot, &snapshot); err != nil {
			return 0, fmt.Errorf("decode Attempt execution snapshot: %w", domain.ErrInteractionSuspend)
		}
		if snapshot.ExecutionDeadlineSeconds == nil {
			return 0, domain.ErrInteractionSuspend
		}
		seconds = *snapshot.ExecutionDeadlineSeconds
	}
	if seconds < 0 || seconds > 2147483647 {
		return 0, domain.ErrInteractionSuspend
	}
	return seconds * int64(time.Second/time.Millisecond), nil
}

// Publish retries may read the same immutable SUSPENDED record; key issuance
// retains the narrower CAPTURING-only authority path.
func (s *Store) AuthorizeInteractionPublishTx(ctx context.Context, tx pgx.Tx, r application.InteractionCaptureRequest) error {
	// A sealed source can only replay its exact published receipt. Its former
	// lease hash identifies that receipt; expired authority is never reactivated.
	var parked bool
	if err := tx.QueryRow(ctx, `SELECT status='INTERACTION_SUSPENDED' FROM workforce_tasks WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, r.TaskID, r.TenantID).Scan(&parked); err != nil {
		return domain.ErrInteractionSuspend
	}
	if parked {
		sum := sha256.Sum256([]byte(r.LeaseToken))
		var id int64
		err := tx.QueryRow(ctx, `SELECT a.id FROM workforce_tasks t JOIN workforce_task_attempts a ON a.id=t.latest_attempt_id AND a.task_turn_id=t.current_task_turn_id
JOIN workforce_attempt_runtimes rt ON rt.attempt_id=a.id
WHERE t.id=$1 AND t.tenant_id=$2 AND t.cancel_requested_at IS NULL AND a.id=$3 AND a.task_turn_id=$4 AND a.worker_id=$5 AND a.worker_instance_id=$6 AND a.host_boot_id=$7 AND a.generation=$8 AND a.owner_epoch=$9+1 AND rt.owner_epoch=a.owner_epoch
AND a.status='INTERACTION_SUSPENDED' AND rt.status='INTERACTION_SUSPENDED' AND rt.fenced_at IS NOT NULL AND a.lease_expires_at<=CURRENT_TIMESTAMP AND rt.execution_lease_expires_at<=CURRENT_TIMESTAMP AND a.lease_token=$10
FOR UPDATE OF a,rt`, r.TaskID, r.TenantID, r.AttemptID, r.TaskTurnID, r.WorkerID, r.InstanceID, r.HostBootID, r.Generation, r.OwnerEpoch, hex.EncodeToString(sum[:])).Scan(&id)
		if err != nil {
			return domain.ErrInteractionSuspend
		}
		if err = tx.QueryRow(ctx, `SELECT id FROM workforce_workers WHERE id=$1 AND current_instance_id=$2 FOR SHARE`, r.WorkerID, r.InstanceID).Scan(&id); err != nil {
			return domain.ErrInteractionSuspend
		}
		if err = tx.QueryRow(ctx, `SELECT id FROM workforce_continuation_checkpoints WHERE id=$1 AND attempt_id=$2 AND task_turn_id=$3 AND COALESCE(interaction_id,0)=$4 AND COALESCE(operation_id,0)=$5 AND checkpoint_identity_sha256=$6 AND purpose='INTERACTION_SUSPEND' AND interaction_phase='SUSPENDED' AND status='AVAILABLE' AND consumed_by_turn_id IS NULL FOR UPDATE`, r.CheckpointID, r.AttemptID, r.TaskTurnID, r.InteractionID, r.OperationID, r.CheckpointIdentity).Scan(&id); err != nil {
			return domain.ErrInteractionSuspend
		}
		return nil
	}
	if _, _, err := s.LockInteractionCaptureSourceTx(ctx, tx, application.ReserveInteractionRequest{InteractionCaptureRequest: r}); err != nil {
		return err
	}
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM workforce_continuation_checkpoints WHERE id=$1 AND task_id=$2 AND task_turn_id=$3 AND attempt_id=$4 AND COALESCE(interaction_id,0)=$5 AND COALESCE(operation_id,0)=$6 AND checkpoint_identity_sha256=$7 AND purpose='INTERACTION_SUSPEND' AND decision='' AND format_version=2 AND consumed_by_turn_id IS NULL AND expires_at>CURRENT_TIMESTAMP AND ((interaction_phase='CAPTURING' AND status='PENDING') OR (interaction_phase='SUSPENDED' AND status='AVAILABLE')) FOR UPDATE`, r.CheckpointID, r.TaskID, r.TaskTurnID, r.AttemptID, r.InteractionID, r.OperationID, r.CheckpointIdentity).Scan(&id)
	if err != nil {
		return domain.ErrInteractionSuspend
	}
	return nil
}

func (s *Store) LockInteractionCaptureSourceTx(ctx context.Context, tx pgx.Tx, r application.ReserveInteractionRequest) (string, int, error) {
	if tx == nil || r.TenantID <= 0 || r.TaskID <= 0 || r.TaskTurnID <= 0 || r.AttemptID <= 0 || r.WorkerID <= 0 || (r.InteractionID > 0) == (r.OperationID > 0) || r.Generation <= 0 || r.OwnerEpoch <= 0 || r.InstanceID == "" || r.HostBootID == "" || r.LeaseToken == "" {
		return "", 0, domain.ErrInteractionSuspend
	}
	var snapshot string
	err := tx.QueryRow(ctx, `SELECT tr.execution_snapshot_digest FROM workforce_tasks t JOIN workforce_task_turns tr ON tr.id=t.current_task_turn_id AND tr.task_id=t.id WHERE t.id=$1 AND t.tenant_id=$2 AND t.current_task_turn_id=$3 AND t.latest_attempt_id=$4 AND t.cancel_requested_at IS NULL AND t.status='RUNNING' AND tr.status='ACTIVE' FOR UPDATE OF t,tr`, r.TaskID, r.TenantID, r.TaskTurnID, r.AttemptID).Scan(&snapshot)
	if err != nil {
		return "", 0, domain.ErrInteractionSuspend
	}
	var hash string
	var expiry time.Time
	err = tx.QueryRow(ctx, `SELECT a.lease_token,a.lease_expires_at FROM workforce_task_attempts a JOIN workforce_workers w ON w.id=a.worker_id AND w.current_instance_id=a.worker_instance_id JOIN workforce_worker_sessions ws ON ws.instance_id=w.current_instance_id AND ws.host_boot_id=a.host_boot_id AND ws.ended_at IS NULL WHERE a.id=$1 AND a.task_id=$2 AND a.task_turn_id=$3 AND a.worker_id=$4 AND a.worker_instance_id=$5 AND a.host_boot_id=$6 AND a.generation=$7 AND a.owner_epoch=$8 AND a.status='RUNNING' FOR UPDATE OF a FOR SHARE OF w,ws`, r.AttemptID, r.TaskID, r.TaskTurnID, r.WorkerID, r.InstanceID, r.HostBootID, r.Generation, r.OwnerEpoch).Scan(&hash, &expiry)
	sum := sha256.Sum256([]byte(r.LeaseToken))
	if err != nil || hash != hex.EncodeToString(sum[:]) || !expiry.After(time.Now().UTC()) {
		return "", 0, domain.ErrInteractionSuspend
	}
	var budget int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM workforce_task_turns WHERE task_id=$1 AND review_revision_kind='AUTOMATIC'`, r.TaskID).Scan(&budget); err != nil {
		return "", 0, err
	}
	return snapshot, budget, nil
}

func (s *Store) ReserveInteractionTx(ctx context.Context, tx pgx.Tx, r application.ReserveInteractionRequest, v application.InteractionReservation) (application.InteractionReservation, error) {
	if tx == nil || s.idGen == nil {
		return application.InteractionReservation{}, domain.ErrInteractionSuspend
	}
	var id int64
	var identity, phase string
	err := tx.QueryRow(ctx, `SELECT id,checkpoint_identity_sha256,interaction_phase FROM workforce_continuation_checkpoints WHERE task_id=$1 AND COALESCE(interaction_id,0)=$2 AND COALESCE(operation_id,0)=$3 FOR UPDATE`, r.TaskID, r.InteractionID, r.OperationID).Scan(&id, &identity, &phase)
	if err == nil {
		if identity != v.Identity || phase != "CAPTURING" {
			return application.InteractionReservation{}, domain.ErrInteractionSuspend
		}
		r.CheckpointID = id
		r.CheckpointIdentity = identity
		if err = s.AuthorizeInteractionCaptureTx(ctx, tx, r.InteractionCaptureRequest); err != nil {
			return application.InteractionReservation{}, err
		}
		record, loadErr := s.LoadInteractionReservationTx(ctx, tx, r.InteractionCaptureRequest)
		if loadErr != nil {
			return record, loadErr
		}
		if record.CurrentRound != v.CurrentRound || record.SuspendControlKeySHA256 != v.SuspendControlKeySHA256 || record.AdapterSessionGeneration != v.AdapterSessionGeneration || (r.SessionDigest != "" && record.SessionDigest != "" && record.SessionDigest != r.SessionDigest) {
			return application.InteractionReservation{}, domain.ErrInteractionSuspend
		}
		return record, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return application.InteractionReservation{}, err
	}
	v.ID = s.idGen.NextID()
	envelope, err := json.Marshal(v.Envelope)
	if err != nil {
		return application.InteractionReservation{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO workforce_continuation_checkpoints(id,task_id,task_turn_id,attempt_id,evaluation_cycle_id,decision,checkpoint_identity_sha256,format_version,purpose,interaction_id,operation_id,interaction_phase,adapter_session_generation,automatic_revision_count,session_object_key,session_sha256,identity_manifest_object_key,manifest_sha256,task_execution_snapshot_digest,session_lineage_digest,encryption_algorithm,encryption_key_id,wrapped_data_key,status,expires_at,interaction_round,suspend_control_key_sha256)
VALUES($1,$2,$3,$4,NULL,'',$5,2,'INTERACTION_SUSPEND',NULLIF($6::BIGINT,0),NULLIF($7::BIGINT,0),'CAPTURING',$8,$9,'','','','',$10,$11,'AES-256-GCM-CHUNKED',$12,$13,'PENDING',CURRENT_TIMESTAMP+INTERVAL '30 days',$14,$15)`, v.ID, r.TaskID, r.TaskTurnID, r.AttemptID, v.Identity, r.InteractionID, r.OperationID, v.AdapterSessionGeneration, v.AutomaticRevisionCount, v.SnapshotDigest, v.SessionDigest, v.Envelope.KeyID, string(envelope), v.CurrentRound, v.SuspendControlKeySHA256)
	return v, err
}

func (s *Store) LoadInteractionReservationTx(ctx context.Context, tx pgx.Tx, r application.InteractionCaptureRequest) (application.InteractionReservation, error) {
	var v application.InteractionReservation
	var body []byte
	err := tx.QueryRow(ctx, `SELECT id,checkpoint_identity_sha256,task_execution_snapshot_digest,session_lineage_digest,adapter_session_generation,automatic_revision_count,wrapped_data_key,interaction_round,suspend_control_key_sha256 FROM workforce_continuation_checkpoints WHERE id=$1 AND task_id=$2 AND task_turn_id=$3 AND attempt_id=$4 AND COALESCE(interaction_id,0)=$5 AND COALESCE(operation_id,0)=$6 AND purpose='INTERACTION_SUSPEND' FOR UPDATE`, r.CheckpointID, r.TaskID, r.TaskTurnID, r.AttemptID, r.InteractionID, r.OperationID).Scan(&v.ID, &v.Identity, &v.SnapshotDigest, &v.SessionDigest, &v.AdapterSessionGeneration, &v.AutomaticRevisionCount, &body, &v.CurrentRound, &v.SuspendControlKeySHA256)
	if err != nil {
		return v, err
	}
	if json.Unmarshal(body, &v.Envelope) != nil {
		return v, domain.ErrInteractionSuspend
	}
	return v, nil
}

func (s *Store) PublishInteractionTx(ctx context.Context, tx pgx.Tx, r application.PublishInteractionRequest, m wcp.Manifest) error {
	if err := m.Validate(); err != nil {
		return err
	}
	body, err := json.Marshal(m)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(body)
	var phase string
	if err = tx.QueryRow(ctx, `SELECT interaction_phase FROM workforce_continuation_checkpoints WHERE id=$1 FOR UPDATE`, r.CheckpointID).Scan(&phase); err != nil {
		return err
	}
	if phase == "SUSPENDED" {
		var matches bool
		err = tx.QueryRow(ctx, `SELECT interaction_manifest_json=$2::jsonb AND repository_bundle_object_key=$3 AND repository_bundle_object_sha256=$4 AND repository_bundle_size_bytes=$5 AND workspace_overlay_object_key=$6 AND workspace_overlay_object_sha256=$7 AND workspace_overlay_size_bytes=$8 AND session_object_key=$9 AND session_object_sha256=$10 AND session_size_bytes=$11 AND identity_manifest_object_key=$12 AND manifest_object_sha256=$13 AND manifest_size_bytes=$14 FROM workforce_continuation_checkpoints WHERE id=$1`, r.CheckpointID, body, r.Bundle.ObjectKey, r.Bundle.SHA256, r.Bundle.SizeBytes, r.Overlay.ObjectKey, r.Overlay.SHA256, r.Overlay.SizeBytes, r.Session.ObjectKey, r.Session.SHA256, r.Session.SizeBytes, r.Manifest.ObjectKey, r.Manifest.SHA256, r.Manifest.SizeBytes).Scan(&matches)
		if err != nil {
			return err
		}
		if !matches {
			return domain.ErrInteractionSuspend
		}
		return nil
	}
	tag, err := tx.Exec(ctx, `UPDATE workforce_continuation_checkpoints SET interaction_phase='SUSPENDED',status='AVAILABLE',available_at=CURRENT_TIMESTAMP,
repository_bundle_object_key=$2,repository_bundle_content_sha256=$3,repository_bundle_object_sha256=$4,repository_bundle_size_bytes=$5,
workspace_overlay_object_key=$6,workspace_overlay_content_sha256=$7,workspace_overlay_object_sha256=$8,workspace_overlay_size_bytes=$9,selection_digest=$10,
session_object_key=$11,session_sha256=$12,session_content_sha256=$12,session_object_sha256=$13,session_size_bytes=$14,
identity_manifest_object_key=$15,manifest_sha256=$16,manifest_content_sha256=$16,manifest_object_sha256=$17,manifest_size_bytes=$18,
base_commit=$19,result_commit=$20,result_tree=$21,interaction_manifest_json=$22,session_lineage_digest=$23
WHERE id=$1 AND purpose='INTERACTION_SUSPEND' AND interaction_phase='CAPTURING' AND status='PENDING' AND consumed_by_turn_id IS NULL`, r.CheckpointID, r.Bundle.ObjectKey, m.RepositoryBundle.SHA256, r.Bundle.SHA256, r.Bundle.SizeBytes, r.Overlay.ObjectKey, m.WorkspaceOverlay.SHA256, r.Overlay.SHA256, r.Overlay.SizeBytes, m.SelectionDigest, r.Session.ObjectKey, m.Session.SHA256, r.Session.SHA256, r.Session.SizeBytes, r.Manifest.ObjectKey, hex.EncodeToString(sum[:]), r.Manifest.SHA256, r.Manifest.SizeBytes, m.BaseCommit, m.ResultCommit, m.ResultTree, body, m.SessionDigest)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrInteractionSuspend
	}
	// Publication ends execution authority atomically. Leases remain expired
	// evidence; their hashes can identify a publish replay but cannot run work.
	tag, err = tx.Exec(ctx, `UPDATE workforce_attempt_runtimes SET status='INTERACTION_SUSPENDED',owner_epoch=owner_epoch+1,execution_lease_expires_at=CURRENT_TIMESTAMP,launch_permit_expires_at=CURRENT_TIMESTAMP,run_permit_expires_at=CURRENT_TIMESTAMP,reattach_permit_expires_at=CURRENT_TIMESTAMP,fenced_at=CURRENT_TIMESTAMP,terminal_reason_code='INTERACTION_SUSPENDED',updated_at=CURRENT_TIMESTAMP WHERE attempt_id=$1 AND status='RUNNING' AND owner_epoch=$2`, r.AttemptID, r.OwnerEpoch)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrInteractionSuspend
	}
	tag, err = tx.Exec(ctx, `UPDATE workforce_task_attempts SET status='INTERACTION_SUSPENDED',owner_epoch=owner_epoch+1,lease_expires_at=CURRENT_TIMESTAMP,finished_at=CURRENT_TIMESTAMP WHERE id=$1 AND task_id=$2 AND task_turn_id=$3 AND status='RUNNING' AND owner_epoch=$4`, r.AttemptID, r.TaskID, r.TaskTurnID, r.OwnerEpoch)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrInteractionSuspend
	}
	for _, statement := range []string{
		`UPDATE workforce_task_turns SET status='INTERACTION_SUSPENDED' WHERE task_id=$1 AND id=$2 AND status='ACTIVE'`,
		`UPDATE workforce_execution_nodes SET status='INTERACTION_SUSPENDED',updated_at=CURRENT_TIMESTAMP WHERE task_id=$1 AND task_turn_id=$2 AND status='RUNNING'`,
		`UPDATE workforce_tasks SET status='INTERACTION_SUSPENDED',version=version+1 WHERE id=$1 AND current_task_turn_id=$2 AND status='RUNNING' AND cancel_requested_at IS NULL`,
	} {
		tag, err = tx.Exec(ctx, statement, r.TaskID, r.TaskTurnID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return domain.ErrInteractionSuspend
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_task_assignments SET status='COMPLETED',completed_at=CURRENT_TIMESTAMP,release_reason_code='INTERACTION_SUSPENDED',delivery_disposition='INTERACTION_SUSPENDED' WHERE id=(SELECT assignment_id FROM workforce_task_attempts WHERE id=$1) AND status='CLAIMED'`, r.AttemptID); err != nil {
		return err
	}
	return eventlog.Append(ctx, tx, r.TaskID, r.AttemptID, "interaction.suspended", map[string]any{"interactionId": r.InteractionID, "operationId": r.OperationID, "checkpointId": r.CheckpointID, "purpose": domain.PurposeInteractionSuspend})
}
