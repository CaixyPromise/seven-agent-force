package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	interactionfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	wcp "github.com/CaixyPromise/seven-agent/force/internal/worker/checkpoint"
	"strconv"
	"strings"
	"time"
)

// Configure before serving requests. This port owns original question/answer
// relationships; continuation never reads another module's private tables.
func (s *Store) WithInteractionRestoreQuestions(reader interactionfacade.RestoredQuestionReader) *Store {
	s.interactionQuestions = reader
	return s
}

// This extends the existing /continuation-restores/:restoreRunId/key response.
// The initial purpose lookup grants nothing; suspend rechecks current Task lock,
// cancellation, worker session and target execution lease before returning data.
func (s *Store) MaterializeInteractionRestoreInput(ctx context.Context, a application.MachineAuthority, runID int64) (*wcp.RestoreInput, error) {
	var purpose string
	if err := s.db.QueryRow(ctx, `SELECT c.purpose FROM workforce_continuation_restore_runs r JOIN workforce_continuation_checkpoints c ON c.id=r.checkpoint_id WHERE r.id=$1 AND r.attempt_id=$2`, runID, a.AttemptID).Scan(&purpose); err != nil {
		return nil, domain.ErrInteractionSuspend
	}
	if purpose == "RESULT" {
		return nil, nil
	}
	if purpose != domain.PurposeInteractionSuspend {
		return nil, domain.ErrInteractionSuspend
	}
	input, err := s.ResolveInteractionRestoreInput(ctx, a, runID)
	if err != nil {
		return nil, err
	}
	return &input, nil
}

var _ application.InteractionRestoreMaterializer = (*Store)(nil)

// ResolveInteractionRestoreManifest is machine-only and current-target fenced.
// It exposes identity/counters, never session bytes or authority from old leases.
func (s *Store) ResolveInteractionRestoreManifest(ctx context.Context, a application.MachineAuthority, restoreRunID int64) (wcp.Manifest, error) {
	v, err := s.ResolveInteractionRestoreInput(ctx, a, restoreRunID)
	return v.Manifest, err
}

func (s *Store) ResolveInteractionRestoreInput(ctx context.Context, a application.MachineAuthority, restoreRunID int64) (wcp.RestoreInput, error) {
	if restoreRunID <= 0 || a.AttemptID <= 0 || a.WorkerID <= 0 || a.InstanceID == "" || a.HostBootID == "" || a.LeaseToken == "" || a.Generation <= 0 || a.OwnerEpoch <= 0 {
		return wcp.RestoreInput{}, domain.ErrInteractionSuspend
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return wcp.RestoreInput{}, err
	}
	defer tx.Rollback(ctx)
	var taskID, tenantID int64
	err = tx.QueryRow(ctx, `SELECT t.id,t.tenant_id FROM workforce_tasks t JOIN workforce_task_attempts a ON a.task_id=t.id AND a.task_turn_id=t.current_task_turn_id WHERE a.id=$1 AND t.latest_attempt_id=a.id AND t.cancel_requested_at IS NULL AND t.status IN ('PREPARING','RUNNING') FOR UPDATE OF t`, a.AttemptID).Scan(&taskID, &tenantID)
	if err != nil {
		return wcp.RestoreInput{}, domain.ErrInteractionSuspend
	}
	var body, response, budgetBody []byte
	var hash string
	var expires time.Time
	var targetTurnID, responseID, operationID, sourceOperationID, checkpointID int64
	var restoreStatus, restoredDigest string
	var attemptStatus string
	err = tx.QueryRow(ctx, `SELECT a.lease_token,a.lease_expires_at,a.task_turn_id,a.status FROM workforce_task_attempts a JOIN workforce_workers w ON w.id=a.worker_id AND w.current_instance_id=a.worker_instance_id JOIN workforce_worker_sessions ws ON ws.instance_id=w.current_instance_id AND ws.host_boot_id=a.host_boot_id AND ws.ended_at IS NULL WHERE a.id=$1 AND a.task_id=$2 AND a.worker_id=$3 AND a.worker_instance_id=$4 AND a.host_boot_id=$5 AND a.generation=$6 AND a.owner_epoch=$7 AND a.status IN ('PREPARING','RUNNING') FOR UPDATE OF a FOR SHARE OF w,ws`, a.AttemptID, taskID, a.WorkerID, a.InstanceID, a.HostBootID, a.Generation, a.OwnerEpoch).Scan(&hash, &expires, &targetTurnID, &attemptStatus)
	sum := sha256.Sum256([]byte(a.LeaseToken))
	if err != nil || hash != hex.EncodeToString(sum[:]) || !expires.After(time.Now().UTC()) {
		return wcp.RestoreInput{}, domain.ErrInteractionSuspend
	}
	err = tx.QueryRow(ctx, `SELECT c.interaction_manifest_json,tr.interaction_response_json,r.interaction_execution_budget_json,COALESCE(tr.source_interaction_response_id,0),COALESCE(c.operation_id,0),COALESCE(tr.source_execution_operation_id,0),c.id,r.status,COALESCE(r.restored_identity_digest,'') FROM workforce_continuation_restore_runs r
JOIN workforce_continuation_checkpoints c ON c.id=r.checkpoint_id AND c.consumed_by_turn_id=r.target_turn_id
JOIN workforce_task_turns tr ON tr.id=r.target_turn_id AND tr.source_checkpoint_id=c.id AND tr.task_id=r.task_id
WHERE r.id=$1 AND r.task_id=$2 AND r.attempt_id=$3 AND r.target_turn_id=$4
AND (r.status IN ('PENDING','RUNNING') OR (r.status='RESTORED' AND $5='PREPARING')) AND c.purpose='INTERACTION_SUSPEND' AND c.interaction_phase='RESUMED' AND c.status='AVAILABLE' AND c.expires_at>CURRENT_TIMESTAMP
FOR UPDATE OF r FOR SHARE OF c,tr`, restoreRunID, taskID, a.AttemptID, targetTurnID, attemptStatus).Scan(&body, &response, &budgetBody, &responseID, &operationID, &sourceOperationID, &checkpointID, &restoreStatus, &restoredDigest)
	if err != nil {
		return wcp.RestoreInput{}, domain.ErrInteractionSuspend
	}
	m, err := wcp.DecodeManifest(body)
	if err != nil {
		return wcp.RestoreInput{}, domain.ErrInteractionSuspend
	}
	if err = m.Validate(); err != nil {
		return wcp.RestoreInput{}, err
	}
	if restoreStatus == "RESTORED" {
		// Matches the existing Worker restore receipt contract. A readback after
		// response loss is not permission to reset/extract state or renew budget.
		digest := sha256.Sum256([]byte(strings.Join([]string{strconv.FormatInt(checkpointID, 10), m.SnapshotDigest, m.ResultCommit, m.ResultCommit, m.ResultTree, m.SessionDigest}, "\x00")))
		if restoredDigest != hex.EncodeToString(digest[:]) || len(budgetBody) == 0 {
			return wcp.RestoreInput{}, domain.ErrInteractionSuspend
		}
	}
	var questionBody []byte
	if m.OperationID > 0 {
		if m.InteractionID != 0 || operationID != m.OperationID || sourceOperationID != m.OperationID || responseID != 0 || len(response) != 0 {
			return wcp.RestoreInput{}, domain.ErrInteractionSuspend
		}
	} else {
		if s.interactionQuestions == nil || m.InteractionID <= 0 || operationID != 0 || sourceOperationID != 0 || len(response) == 0 || len(response) > 16*1024 || !json.Valid(response) {
			return wcp.RestoreInput{}, domain.ErrInteractionSuspend
		}
		question, questionErr := s.interactionQuestions.RestoredQuestionTx(ctx, tx, tenantID, taskID, targetTurnID, responseID)
		if questionErr != nil {
			return wcp.RestoreInput{}, questionErr
		}
		questionBody, err = application.ProjectInteractionRestoreQuestion(question, response, responseID)
		if err != nil {
			return wcp.RestoreInput{}, err
		}
	}
	var budget wcp.ExecutionBudget
	if len(budgetBody) == 0 {
		budget, err = application.ColdInteractionBudget(m.ExecutionBudget, a, time.Now().UTC())
		if err != nil {
			return wcp.RestoreInput{}, err
		}
		budgetBody, err = json.Marshal(budget)
		if err != nil {
			return wcp.RestoreInput{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_continuation_restore_runs SET interaction_execution_budget_json=$2::jsonb WHERE id=$1`, restoreRunID, budgetBody); err != nil {
			return wcp.RestoreInput{}, err
		}
	} else if json.Unmarshal(budgetBody, &budget) != nil || budget.Validate() != nil {
		return wcp.RestoreInput{}, domain.ErrInteractionSuspend
	}
	if budget.TaskID != taskID || budget.AttemptID != a.AttemptID || budget.Generation != a.Generation || budget.OwnerEpoch != a.OwnerEpoch || budget.ActiveLimitMillis != m.ExecutionBudget.ActiveLimitMillis || budget.ActiveElapsedMillis != m.ExecutionBudget.ActiveElapsedMillis || budget.InteractionWaitingMillis != m.ExecutionBudget.InteractionWaitingMillis || (budget.ActiveLimitMillis > 0 && budget.AbsoluteDeadlineUnixMillis <= time.Now().UnixMilli()) {
		return wcp.RestoreInput{}, domain.ErrInteractionSuspend
	}
	return wcp.RestoreInput{Manifest: m, InteractionResponse: response, QuestionRequest: questionBody, ExecutionBudget: budget}, tx.Commit(ctx)
}
