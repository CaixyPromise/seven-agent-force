package infrastructure

import (
	"context"
	"errors"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	"github.com/jackc/pgx/v5"
)

func (s *Store) LoadOperationResumeTx(ctx context.Context, tx pgx.Tx, r application.OperationResumeRequest) (domain.InteractionSuspend, error) {
	var v domain.InteractionSuspend
	if tx == nil {
		return v, domain.ErrInteractionSuspend
	}
	err := tx.QueryRow(ctx, `SELECT id,COALESCE(current_task_turn_id,0),COALESCE(latest_attempt_id,0),status,cancel_requested_at IS NOT NULL FROM workforce_tasks WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, r.TaskID, r.TenantID).Scan(&v.TaskID, &v.CurrentTurnID, &v.CurrentAttemptID, &v.TaskStatus, &v.Cancelled)
	if err != nil {
		return v, domain.ErrInteractionSuspend
	}
	err = tx.QueryRow(ctx, `SELECT c.task_turn_id,c.attempt_id,COALESCE(c.interaction_id,0),COALESCE(c.operation_id,0),c.purpose,c.interaction_phase,c.format_version,c.decision,c.status,c.expires_at,COALESCE(c.consumed_by_turn_id,0),tr.status,tr.execution_snapshot_digest,c.task_execution_snapshot_digest,c.session_lineage_digest,tr.instruction
FROM workforce_continuation_checkpoints c JOIN workforce_task_turns tr ON tr.id=c.task_turn_id AND tr.task_id=c.task_id
WHERE c.id=$1 AND c.task_id=$2 AND c.operation_id=$3 AND tr.tenant_id=$4 FOR UPDATE OF c,tr`, r.CheckpointID, r.TaskID, r.OperationID, r.TenantID).
		Scan(&v.TurnID, &v.AttemptID, &v.InteractionID, &v.OperationID, &v.Purpose, &v.Phase, &v.FormatVersion, &v.Source.CheckpointDecision, &v.Source.CheckpointStatus, &v.Source.CheckpointExpires, &v.Source.ConsumedByTurnID, &v.TurnStatus, &v.SnapshotDigest, &v.CheckpointSnapshotDigest, &v.SessionDigest, &v.Instruction)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, domain.ErrInteractionSuspend
	}
	if err != nil {
		return v, err
	}
	err = tx.QueryRow(ctx, `SELECT a.status,rt.status,(a.lease_expires_at<=CURRENT_TIMESTAMP AND rt.execution_lease_expires_at<=CURRENT_TIMESTAMP AND rt.fenced_at IS NOT NULL AND rt.owner_epoch=a.owner_epoch) FROM workforce_task_attempts a JOIN workforce_attempt_runtimes rt ON rt.attempt_id=a.id WHERE a.id=$1 AND a.task_id=$2 AND a.task_turn_id=$3 FOR UPDATE OF a,rt`, v.AttemptID, r.TaskID, v.TurnID).Scan(&v.AttemptStatus, &v.RuntimeStatus, &v.AuthoritySealed)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, domain.ErrInteractionSuspend
	}
	return v, err
}

func (s *Store) PersistOperationResumeTx(ctx context.Context, tx pgx.Tx, r application.OperationResumeRequest, next application.CreateNextTurnRequest) (application.OperationResumeResult, error) {
	v, err := s.LoadOperationResumeTx(ctx, tx, r)
	if err != nil {
		return application.OperationResumeResult{}, err
	}
	if next.TaskID != r.TaskID || next.TenantID != r.TenantID || next.CreatorUserID != r.CreatorUserID || next.Instruction != v.Instruction || next.ExpectedCurrentTurnID != r.SourceTaskTurnID || next.SourceCheckpointID != r.CheckpointID {
		return application.OperationResumeResult{}, domain.ErrInteractionSuspend
	}
	if err = v.ValidateOperationResume(r.TaskID, r.SourceTaskTurnID, r.SourceAttemptID, r.OperationID, time.Now().UTC()); err != nil {
		return application.OperationResumeResult{}, err
	}
	turn, replay, err := s.createNextTurnTx(ctx, tx, next, true)
	if err != nil {
		return application.OperationResumeResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_task_turns SET source_execution_operation_id=$2 WHERE id=$1`, turn.ID, r.OperationID); err != nil {
		return application.OperationResumeResult{}, err
	}
	for _, statement := range []string{
		`UPDATE workforce_task_turns SET status='SUPERSEDED',finished_at=COALESCE(finished_at,CURRENT_TIMESTAMP) WHERE task_id=$1 AND id=$2`,
		`UPDATE workforce_execution_nodes SET status='SUPERSEDED',updated_at=CURRENT_TIMESTAMP WHERE task_id=$1 AND task_turn_id=$2`,
		`UPDATE workforce_task_attempts SET status='SUPERSEDED',finished_at=COALESCE(finished_at,CURRENT_TIMESTAMP) WHERE task_id=$1 AND task_turn_id=$2`,
	} {
		if _, err = tx.Exec(ctx, statement, r.TaskID, r.SourceTaskTurnID); err != nil {
			return application.OperationResumeResult{}, err
		}
	}
	tag, err := tx.Exec(ctx, `UPDATE workforce_continuation_checkpoints SET interaction_phase='RESUMED' WHERE id=$1 AND purpose='INTERACTION_SUSPEND' AND operation_id=$2 AND interaction_phase='RESUMED' AND consumed_by_turn_id=$3`, r.CheckpointID, r.OperationID, turn.ID)
	if err != nil || tag.RowsAffected() != 1 {
		return application.OperationResumeResult{}, domain.ErrInteractionSuspend
	}
	return application.OperationResumeResult{TaskTurnID: turn.ID, SourceCheckpointID: r.CheckpointID, Replayed: replay}, nil
}

// FindOperationResumeTx is the operation counterpart of FindInteractionResumeTx.
// It exposes only the checkpoint identity needed by executionaccess to perform
// the user decision and one successor Turn in the same transaction.
func (s *Store) FindOperationResumeTx(ctx context.Context, tx pgx.Tx, r facade.OperationResumeLookup) (*facade.OperationResumeTarget, error) {
	if tx == nil || r.TenantID <= 0 || r.TaskID <= 0 || r.AttemptID <= 0 || r.OperationID <= 0 {
		return nil, domain.ErrInteractionSuspend
	}
	var taskID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM workforce_tasks WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, r.TaskID, r.TenantID).Scan(&taskID); err != nil {
		return nil, err
	}
	var target facade.OperationResumeTarget
	var phase string
	err := tx.QueryRow(ctx, `SELECT task_turn_id,id,interaction_phase,COALESCE(consumed_by_turn_id,0) FROM workforce_continuation_checkpoints WHERE task_id=$1 AND attempt_id=$2 AND operation_id=$3 AND purpose='INTERACTION_SUSPEND' FOR UPDATE`, r.TaskID, r.AttemptID, r.OperationID).Scan(&target.SourceTaskTurnID, &target.CheckpointID, &phase, &target.ResumedTaskTurnID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if phase == "CAPTURING" {
		return nil, domain.ErrInteractionSuspendInProgress
	}
	if phase == "RESUMED" && target.ResumedTaskTurnID > 0 {
		return &target, nil
	}
	if phase != "SUSPENDED" {
		return nil, domain.ErrInteractionSuspend
	}
	return &target, nil
}
