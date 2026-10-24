package infrastructure

import (
	"context"
	"errors"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	"github.com/jackc/pgx/v5"
)

func (s *Store) ReadEvaluation(ctx context.Context, identity facade.MachineIdentity, attemptID int64, lease string, pending facade.EvaluationReplayIdentity) (facade.EvaluationReplay, error) {
	if err := validRuntimeIdentity(identity, attemptID, lease); err != nil {
		return facade.EvaluationReplay{}, err
	}
	if pending.CycleID <= 0 || pending.CycleNumber < 1 || len(pending.SessionDigest) > 128 {
		return facade.EvaluationReplay{}, ErrConflict
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return facade.EvaluationReplay{}, err
	}
	defer tx.Rollback(ctx)
	result, err := s.readEvaluationTx(ctx, tx, identity, attemptID, lease, pending)
	if err != nil {
		return facade.EvaluationReplay{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return facade.EvaluationReplay{}, err
	}
	return result, nil
}

func (s *Store) readEvaluationTx(ctx context.Context, tx pgx.Tx, identity facade.MachineIdentity, attemptID int64, lease string, pending facade.EvaluationReplayIdentity) (facade.EvaluationReplay, error) {
	// This locks Task/Attempt/Runtime and checks worker, instance, host boot,
	// generation, owner epoch, lease hash/expiry and live Runtime before lookup.
	scope, err := s.loadEvaluationScope(ctx, tx, identity, attemptID, lease)
	if err != nil {
		return facade.EvaluationReplay{}, err
	}
	if scope.AttemptStatus != "RUNNING" || (scope.TaskStatus != "RUNNING" && scope.TaskStatus != "VALIDATING" && scope.TaskStatus != "REVIEWING" && scope.TaskStatus != "MANUAL_REVIEW") || pending.CycleNumber > scope.MaxCycles {
		return facade.EvaluationReplay{}, ErrConflict
	}
	var currentTurn int64
	var cancelled bool
	var currentSession string
	if err = tx.QueryRow(ctx, `SELECT COALESCE(t.current_task_turn_id,0),t.cancel_requested_at IS NOT NULL,tr.session_digest FROM workforce_tasks t JOIN workforce_task_turns tr ON tr.id=t.current_task_turn_id WHERE t.id=$1`, scope.TaskID).Scan(&currentTurn, &cancelled, &currentSession); err != nil {
		return facade.EvaluationReplay{}, err
	}
	if currentTurn != scope.TurnID || cancelled || (currentSession != "" && currentSession != pending.SessionDigest) {
		return facade.EvaluationReplay{}, ErrConflict
	}
	var cycleID int64
	var session, decision, reason, message string
	err = tx.QueryRow(ctx, `SELECT id,session_digest,decision,reason_code,message FROM workforce_result_evaluation_cycles WHERE attempt_id=$1 AND task_id=$2 AND task_turn_id=$3 AND cycle_number=$4 AND status='PERSISTED' FOR UPDATE`, attemptID, scope.TaskID, scope.TurnID, pending.CycleNumber).Scan(&cycleID, &session, &decision, &reason, &message)
	if errors.Is(err, pgx.ErrNoRows) {
		if scope.TaskStatus != "RUNNING" && scope.TaskStatus != "VALIDATING" {
			return facade.EvaluationReplay{}, ErrConflict
		}
		return facade.EvaluationReplay{}, nil
	}
	if err != nil {
		return facade.EvaluationReplay{}, err
	}
	if cycleID != pending.CycleID || session != pending.SessionDigest {
		return facade.EvaluationReplay{}, ErrConflict
	}
	directive, err := s.directiveForPersistedCycle(ctx, tx, cycleID, decision, reason, message, scope.MaxCycles-pending.CycleNumber)
	if err != nil {
		return facade.EvaluationReplay{}, err
	}
	return facade.EvaluationReplay{Found: true, Directive: directive}, nil
}
