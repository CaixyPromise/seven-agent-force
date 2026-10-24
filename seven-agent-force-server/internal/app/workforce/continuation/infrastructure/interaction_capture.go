package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Store) AuthorizeInteractionCaptureTx(ctx context.Context, tx pgx.Tx, r application.InteractionCaptureRequest) error {
	if tx == nil || r.TenantID <= 0 || r.TaskID <= 0 || r.TaskTurnID <= 0 || (r.InteractionID > 0) == (r.OperationID > 0) || r.CheckpointID <= 0 || r.WorkerID <= 0 || r.AttemptID <= 0 || r.Generation <= 0 || r.OwnerEpoch <= 0 || r.InstanceID == "" || r.HostBootID == "" || r.LeaseToken == "" {
		return domain.ErrInteractionSuspend
	}
	var id int64
	if err := tx.QueryRow(ctx, `SELECT id FROM workforce_tasks WHERE id=$1 AND tenant_id=$2 AND current_task_turn_id=$3 AND latest_attempt_id=$4 AND status='RUNNING' AND cancel_requested_at IS NULL FOR UPDATE`, r.TaskID, r.TenantID, r.TaskTurnID, r.AttemptID).Scan(&id); err != nil {
		return domain.ErrInteractionSuspend
	}
	var lease string
	var expires time.Time
	err := tx.QueryRow(ctx, `SELECT a.lease_token,a.lease_expires_at FROM workforce_task_attempts a
JOIN workforce_workers w ON w.id=a.worker_id AND w.current_instance_id=a.worker_instance_id
JOIN workforce_worker_sessions ws ON ws.instance_id=w.current_instance_id AND ws.ended_at IS NULL AND ws.host_boot_id=a.host_boot_id
WHERE a.id=$1 AND a.task_id=$2 AND a.task_turn_id=$3 AND a.worker_id=$4 AND a.worker_instance_id=$5 AND a.host_boot_id=$6 AND a.generation=$7 AND a.owner_epoch=$8 AND a.status='RUNNING'
FOR UPDATE OF a FOR SHARE OF w,ws`, r.AttemptID, r.TaskID, r.TaskTurnID, r.WorkerID, r.InstanceID, r.HostBootID, r.Generation, r.OwnerEpoch).Scan(&lease, &expires)
	if err != nil {
		return domain.ErrInteractionSuspend
	}
	sum := sha256.Sum256([]byte(r.LeaseToken))
	if lease != hex.EncodeToString(sum[:]) || !expires.After(time.Now().UTC()) {
		return domain.ErrInteractionSuspend
	}
	if err = tx.QueryRow(ctx, `SELECT id FROM workforce_continuation_checkpoints WHERE id=$1 AND attempt_id=$2 AND task_id=$3 AND task_turn_id=$4 AND COALESCE(interaction_id,0)=$5 AND COALESCE(operation_id,0)=$6 AND purpose='INTERACTION_SUSPEND' AND decision='' AND interaction_phase='CAPTURING' AND status='PENDING' AND format_version=2 AND checkpoint_identity_sha256=$7 AND consumed_by_turn_id IS NULL AND expires_at>CURRENT_TIMESTAMP FOR UPDATE`, r.CheckpointID, r.AttemptID, r.TaskID, r.TaskTurnID, r.InteractionID, r.OperationID, r.CheckpointIdentity).Scan(&id); err != nil {
		return domain.ErrInteractionSuspend
	}
	return nil
}
