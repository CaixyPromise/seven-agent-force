package infrastructure

import (
	"context"
	"errors"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Store) LoadInteractionResumeTx(ctx context.Context, tx pgx.Tx, r application.InteractionResumeRequest) (domain.InteractionSuspend, error) {
	var v domain.InteractionSuspend
	if tx == nil {
		return v, domain.ErrInteractionSuspend
	}
	// Lock order matches cancellation and normal continuation. Never lock the
	// checkpoint first and subsequently try to acquire the Task lock.
	err := tx.QueryRow(ctx, `SELECT id,COALESCE(current_task_turn_id,0),COALESCE(latest_attempt_id,0),status,cancel_requested_at IS NOT NULL FROM workforce_tasks WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, r.TaskID, r.TenantID).
		Scan(&v.TaskID, &v.CurrentTurnID, &v.CurrentAttemptID, &v.TaskStatus, &v.Cancelled)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, domain.ErrInteractionSuspend
	}
	if err != nil {
		return v, err
	}
	err = tx.QueryRow(ctx, `SELECT c.task_turn_id,c.attempt_id,c.interaction_id,c.purpose,c.interaction_phase,c.format_version,c.decision,c.status,c.expires_at,COALESCE(c.consumed_by_turn_id,0),tr.status,tr.execution_snapshot_digest,c.task_execution_snapshot_digest,c.session_lineage_digest,tr.instruction
FROM workforce_continuation_checkpoints c JOIN workforce_task_turns tr ON tr.id=c.task_turn_id AND tr.task_id=c.task_id
WHERE c.id=$1 AND c.task_id=$2 AND tr.tenant_id=$3 FOR UPDATE OF c,tr`, r.CheckpointID, r.TaskID, r.TenantID).
		Scan(&v.TurnID, &v.AttemptID, &v.InteractionID, &v.Purpose, &v.Phase, &v.FormatVersion, &v.Source.CheckpointDecision, &v.Source.CheckpointStatus, &v.Source.CheckpointExpires, &v.Source.ConsumedByTurnID, &v.TurnStatus, &v.SnapshotDigest, &v.CheckpointSnapshotDigest, &v.SessionDigest, &v.Instruction)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return v, domain.ErrInteractionSuspend
		}
		return v, err
	}
	err = tx.QueryRow(ctx, `SELECT a.status,rt.status,(a.lease_expires_at<=CURRENT_TIMESTAMP AND rt.execution_lease_expires_at<=CURRENT_TIMESTAMP AND rt.fenced_at IS NOT NULL AND rt.owner_epoch=a.owner_epoch) FROM workforce_task_attempts a JOIN workforce_attempt_runtimes rt ON rt.attempt_id=a.id WHERE a.id=$1 AND a.task_id=$2 AND a.task_turn_id=$3 FOR UPDATE OF a,rt`, v.AttemptID, r.TaskID, v.TurnID).Scan(&v.AttemptStatus, &v.RuntimeStatus, &v.AuthoritySealed)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, domain.ErrInteractionSuspend
	}
	return v, err
}

func (s *Store) PersistInteractionResumeTx(ctx context.Context, tx pgx.Tx, r application.InteractionResumeRequest, next application.CreateNextTurnRequest) (application.InteractionResumeResult, error) {
	// Defense in depth for direct port misuse; app owns the domain transition.
	v, err := s.LoadInteractionResumeTx(ctx, tx, r)
	if err != nil {
		return application.InteractionResumeResult{}, err
	}
	if next.TaskID != r.TaskID || next.TenantID != r.TenantID || next.CreatorUserID != r.CreatorUserID || next.Instruction != v.Instruction || next.ExpectedCurrentTurnID != r.SourceTaskTurnID || next.SourceCheckpointID != r.CheckpointID {
		return application.InteractionResumeResult{}, domain.ErrInteractionSuspend
	}
	if err = v.ValidateResume(r.TaskID, r.SourceTaskTurnID, r.SourceAttemptID, r.InteractionID, time.Now().UTC()); err != nil {
		return application.InteractionResumeResult{}, err
	}
	turn, replay, err := s.createNextTurnTx(ctx, tx, next, true)
	if err != nil {
		return application.InteractionResumeResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_task_turns SET source_interaction_response_id=$2,interaction_response_json=$3 WHERE id=$1`, turn.ID, r.AnswerID, r.InteractionResponse); err != nil {
		return application.InteractionResumeResult{}, err
	}
	// No result/approval/artifact or delivery state is written. Existing taskwide
	// AUTOMATIC review-turn count stays intact: this turn has no revision label.
	for _, statement := range []string{
		`UPDATE workforce_task_turns SET status='SUPERSEDED',finished_at=COALESCE(finished_at,CURRENT_TIMESTAMP) WHERE task_id=$1 AND id=$2`,
		`UPDATE workforce_execution_nodes SET status='SUPERSEDED',updated_at=CURRENT_TIMESTAMP WHERE task_id=$1 AND task_turn_id=$2`,
		`UPDATE workforce_task_attempts SET status='SUPERSEDED',finished_at=COALESCE(finished_at,CURRENT_TIMESTAMP) WHERE task_id=$1 AND task_turn_id=$2`,
	} {
		if _, err = tx.Exec(ctx, statement, r.TaskID, r.SourceTaskTurnID); err != nil {
			return application.InteractionResumeResult{}, err
		}
	}
	tag, err := tx.Exec(ctx, `UPDATE workforce_continuation_checkpoints SET interaction_phase='RESUMED' WHERE id=$1 AND purpose='INTERACTION_SUSPEND' AND interaction_phase='RESUMED' AND consumed_by_turn_id=$2`, r.CheckpointID, turn.ID)
	if err != nil {
		return application.InteractionResumeResult{}, err
	}
	if tag.RowsAffected() != 1 {
		return application.InteractionResumeResult{}, domain.ErrInteractionSuspend
	}
	return application.InteractionResumeResult{TaskTurnID: turn.ID, SourceCheckpointID: r.CheckpointID, Replayed: replay}, nil
}

// FindInteractionResumeTx is a private cross-module projection; it returns no
// session bytes, object keys, wrapped keys or result authority. CAPTURING is an
// error, not an absent checkpoint, so an answer cannot race into same-Attempt
// delivery while the native session is being stopped.
func (s *Store) FindInteractionResumeTx(ctx context.Context, tx pgx.Tx, r facade.InteractionResumeLookup) (*facade.InteractionResumeTarget, error) {
	if tx == nil || r.TenantID <= 0 || r.TaskID <= 0 || r.AttemptID <= 0 || r.InteractionID <= 0 {
		return nil, domain.ErrInteractionSuspend
	}
	var taskID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM workforce_tasks WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, r.TaskID, r.TenantID).Scan(&taskID); err != nil {
		return nil, err
	}
	var target facade.InteractionResumeTarget
	var phase string
	err := tx.QueryRow(ctx, `SELECT c.task_turn_id,c.id,c.interaction_phase FROM workforce_continuation_checkpoints c WHERE c.task_id=$1 AND c.attempt_id=$2 AND c.interaction_id=$3 AND c.purpose='INTERACTION_SUSPEND' FOR UPDATE`, r.TaskID, r.AttemptID, r.InteractionID).Scan(&target.SourceTaskTurnID, &target.CheckpointID, &phase)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if phase == "CAPTURING" {
		return nil, domain.ErrInteractionSuspendInProgress
	}
	if phase != "SUSPENDED" {
		return nil, domain.ErrInteractionSuspend
	}
	return &target, nil
}
