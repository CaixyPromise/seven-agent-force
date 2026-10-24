package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	continuationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	principalinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/eventlog"
	tasknotification "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/facade"
)

func (s *Store) Decide(ctx context.Context, request facade.HumanDecisionRequest) (facade.HumanDecisionResult, bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return facade.HumanDecisionResult{}, false, err
	}
	defer tx.Rollback(ctx)
	requestDigest := humanDecisionRequestDigest(request.Action, request.Reason, request.RevisionGuidance, request.IdempotencyKey)
	if replay, found, replayErr := replayHumanDecision(ctx, tx, request, requestDigest); replayErr != nil {
		return facade.HumanDecisionResult{}, false, replayErr
	} else if found {
		return replay, true, tx.Commit(ctx)
	}

	var taskID, turnID, attemptID, finalizationID int64
	var runStatus, taskStatus, verdict, reportDigest, reasonCode string
	var reportJSON []byte
	err = tx.QueryRow(ctx, `SELECT r.task_id,r.task_turn_id,r.attempt_id,r.finalization_run_id,r.status,t.status,r.report_verdict,r.canonical_report_digest,r.canonical_report_json,COALESCE(NULLIF(r.closure_reason_code,''),r.error_code)
FROM workforce_review_runs r JOIN workforce_tasks t ON t.id=r.task_id AND t.tenant_id=r.tenant_id
WHERE r.id=$1 AND r.tenant_id=$2 FOR UPDATE OF r,t`, request.ReviewRunID, request.TenantID).
		Scan(&taskID, &turnID, &attemptID, &finalizationID, &runStatus, &taskStatus, &verdict, &reportDigest, &reportJSON, &reasonCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return facade.HumanDecisionResult{}, false, domain.ErrNotFound
	}
	if err != nil {
		return facade.HumanDecisionResult{}, false, err
	}
	// The first lookup can race another transaction that applies the same
	// decision while this transaction waits for the authoritative Run/Task
	// lock. Recheck before stale-state validation so identical concurrent
	// requests converge to the persisted decision.
	if replay, found, replayErr := replayHumanDecision(ctx, tx, request, requestDigest); replayErr != nil {
		return facade.HumanDecisionResult{}, false, replayErr
	} else if found {
		return replay, true, tx.Commit(ctx)
	}
	if taskStatus != "MANUAL_REVIEW" || (runStatus != domain.StatusManualReview && runStatus != domain.StatusRejected) {
		return facade.HumanDecisionResult{}, false, domain.ErrHumanDecisionStale
	}
	var currentTurnID int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(current_task_turn_id,0) FROM workforce_tasks WHERE id=$1`, taskID).Scan(&currentTurnID); err != nil || currentTurnID != turnID {
		return facade.HumanDecisionResult{}, false, domain.ErrHumanDecisionStale
	}
	actorPrincipalID, err := principalinfra.EnsureUser(ctx, tx, s.idGen, request.ActorUserID)
	if err != nil {
		return facade.HumanDecisionResult{}, false, err
	}

	result := facade.HumanDecisionResult{ID: s.idGen.NextID(), TaskID: taskID, TaskTurnID: turnID, ReviewRunID: request.ReviewRunID, Action: request.Action, AIVerdict: verdict, ReportDigest: reportDigest}
	switch request.Action {
	case domain.HumanActionAcceptRisk:
		if (verdict != "REVISE" && verdict != "REJECT") || reportDigest == "" || len(reportJSON) == 0 {
			return facade.HumanDecisionResult{}, false, domain.ErrHumanActionNotAllowed
		}
		if err = s.acceptRisk(ctx, tx, request, taskID, turnID, attemptID, finalizationID); err != nil {
			return facade.HumanDecisionResult{}, false, err
		}
		result.QualityOutcome = domain.QualityHumanAccepted
	case domain.HumanActionAuthorizeRevision:
		if s.revisionTurnCreator == nil {
			return facade.HumanDecisionResult{}, false, domain.ErrHumanActionNotAllowed
		}
		if reportDigest == "" && strings.TrimSpace(request.RevisionGuidance) == "" {
			return facade.HumanDecisionResult{}, false, domain.ErrHumanGuidanceRequired
		}
		created, createErr := s.revisionTurnCreator.CreateReviewRevisionTurn(ctx, tx, continuationfacade.ReviewRevisionTurnRequest{
			TenantID: request.TenantID, TaskID: taskID, SourceTaskTurnID: turnID, SourceAttemptID: attemptID,
			SourceReviewRunID: request.ReviewRunID, RevisionKind: continuationfacade.RevisionHumanAuthorized,
			ReportVerdict: verdict, ReportDigest: reportDigest, CanonicalReport: reportJSON,
			SafeReasonCode: reasonCode, RevisionGuidance: request.RevisionGuidance, TriggerUserID: request.ActorUserID,
		})
		if createErr != nil {
			if errors.Is(createErr, continuationfacade.ErrReviewRevisionCheckpointUnavailable) {
				return facade.HumanDecisionResult{}, false, continuationfacade.ErrReviewRevisionCheckpointUnavailable
			}
			if errors.Is(createErr, continuationfacade.ErrReviewRevisionIdentityConflict) {
				return facade.HumanDecisionResult{}, false, domain.ErrHumanDecisionStale
			}
			return facade.HumanDecisionResult{}, false, createErr
		}
		result.ResultingTaskTurnID = created.TaskTurnID
		result.RevisionGuidance = request.RevisionGuidance
		if _, err = tx.Exec(ctx, `UPDATE workforce_review_runs SET successor_task_turn_id=$2,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, request.ReviewRunID, created.TaskTurnID); err != nil {
			return facade.HumanDecisionResult{}, false, err
		}
		if err = eventlog.Append(ctx, tx, taskID, attemptID, "independent_review.human_revision_authorized", map[string]any{"reviewRunId": request.ReviewRunID, "taskTurnId": created.TaskTurnID, "actorPrincipalId": actorPrincipalID}); err != nil {
			return facade.HumanDecisionResult{}, false, err
		}
	case domain.HumanActionCancelTask:
		if err = s.cancelManualClosure(ctx, tx, request.ReviewRunID, taskID, turnID, attemptID); err != nil {
			return facade.HumanDecisionResult{}, false, err
		}
	default:
		return facade.HumanDecisionResult{}, false, domain.ErrHumanActionNotAllowed
	}

	methods := normalizedProofMethods(request.Proof.AuthenticationMethods)
	methodsJSON, _ := json.Marshal(methods)
	proofHash := hashText(request.Proof.ProofIdentifier)
	challengeHash := hashText(request.Proof.ChallengeIdentifier)
	_, err = tx.Exec(ctx, `INSERT INTO workforce_human_review_decisions
(id,tenant_id,task_id,task_turn_id,review_run_id,action,idempotency_key,request_digest,actor_principal_id,reason,revision_guidance,
 proof_identifier_sha256,challenge_identifier_sha256,assurance_level,authentication_methods_json,ai_verdict,canonical_report_digest,status,resulting_task_turn_id,quality_outcome,decided_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,''),$12,$13,'AAL2',$14,$15,$16,'APPLIED',NULLIF($17::BIGINT,0),$18,CURRENT_TIMESTAMP)`,
		result.ID, request.TenantID, taskID, turnID, request.ReviewRunID, request.Action, request.IdempotencyKey, requestDigest, actorPrincipalID, request.Reason,
		request.RevisionGuidance, proofHash, challengeHash, methodsJSON, verdict, reportDigest, result.ResultingTaskTurnID, result.QualityOutcome)
	if err != nil {
		return facade.HumanDecisionResult{}, false, err
	}
	if err = tx.QueryRow(ctx, `SELECT to_char(decided_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') FROM workforce_human_review_decisions WHERE id=$1`, result.ID).Scan(&result.DecidedAt); err != nil {
		return facade.HumanDecisionResult{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return facade.HumanDecisionResult{}, false, err
	}
	return result, false, nil
}

func (s *Store) acceptRisk(ctx context.Context, tx pgx.Tx, request facade.HumanDecisionRequest, taskID, turnID, attemptID, finalizationID int64) error {
	var checkpointID int64
	var finalizationStatus string
	var deterministicFailures int
	err := tx.QueryRow(ctx, `SELECT c.id,f.status
FROM workforce_continuation_checkpoints c JOIN workforce_finalization_runs f ON f.id=$4 AND f.attempt_id=$3
WHERE c.task_id=$1 AND c.task_turn_id=$2 AND c.attempt_id=$3 AND c.decision='PASS' AND c.status='AVAILABLE' AND c.format_version=2 AND c.consumed_by_turn_id IS NULL AND c.expires_at>CURRENT_TIMESTAMP
ORDER BY c.available_at DESC,c.id DESC LIMIT 1 FOR UPDATE OF c,f`, taskID, turnID, attemptID, finalizationID).Scan(&checkpointID, &finalizationStatus)
	if err != nil || checkpointID == 0 || finalizationStatus != "FROZEN" {
		return domain.ErrDeterministicAuthorityInvalid
	}
	if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM workforce_check_runs WHERE task_id=$1 AND attempt_id=$2 AND (status<>'SUCCEEDED' OR COALESCE(exit_code,-1)<>0)`, taskID, attemptID).Scan(&deterministicFailures); err != nil {
		return err
	}
	if deterministicFailures != 0 {
		return domain.ErrDeterministicAuthorityInvalid
	}
	if tag, updateErr := tx.Exec(ctx, `UPDATE workforce_artifacts SET status='AVAILABLE' WHERE task_id=$1 AND attempt_id=$2 AND status='PENDING_REVIEW'`, taskID, attemptID); updateErr != nil || tag.RowsAffected() < 1 {
		if updateErr != nil {
			return updateErr
		}
		return domain.ErrHumanDecisionStale
	}
	if err = requireOne(ctx, tx, `UPDATE workforce_task_attempts SET status='SUCCEEDED',finished_at=CURRENT_TIMESTAMP,exit_code=0 WHERE id=$1 AND status='MANUAL_REVIEW'`, attemptID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='SUCCEEDED',updated_at=CURRENT_TIMESTAMP WHERE task_id=$1 AND task_turn_id=$2 AND status='MANUAL_REVIEW'`, taskID, turnID); err != nil {
		return err
	}
	if err = requireOne(ctx, tx, `UPDATE workforce_task_turns SET status='SUCCEEDED',result_summary='Human accepted independent review risk',finished_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='MANUAL_REVIEW'`, turnID); err != nil {
		return err
	}
	if err = requireOne(ctx, tx, `UPDATE workforce_tasks SET status='SUCCEEDED',quality_outcome='HUMAN_ACCEPTED',result_summary='Human accepted independent review risk',verified_at=CURRENT_TIMESTAMP,finished_at=CURRENT_TIMESTAMP,version=version+1 WHERE id=$1 AND current_task_turn_id=$2 AND status='MANUAL_REVIEW'`, taskID, turnID); err != nil {
		return err
	}
	occurrence, err := eventlog.AppendWithSequence(ctx, tx, taskID, attemptID, "task.succeeded", map[string]any{"taskTurnId": turnID, "reviewRunId": request.ReviewRunID, "qualityOutcome": domain.QualityHumanAccepted})
	if err != nil {
		return err
	}
	if s.notificationRecorder != nil {
		_, err = s.notificationRecorder.RecordTransition(ctx, tx, tasknotification.TaskNotificationTransition{TenantID: request.TenantID, TaskID: taskID, TaskTurnID: turnID, SourceType: tasknotification.SourceIndependentReviewRun, SourceID: request.ReviewRunID, SourceOccurrence: occurrence, CurrentStatus: "SUCCEEDED", ReasonCode: "INDEPENDENT_REVIEW_HUMAN_ACCEPTED", Recoverability: tasknotification.RecoverabilityTerminal, OccurredAt: s.now().UTC()})
	}
	return err
}

func (s *Store) cancelManualClosure(ctx context.Context, tx pgx.Tx, runID, taskID, turnID, attemptID int64) error {
	if _, err := tx.Exec(ctx, `UPDATE workforce_review_runs SET status='CANCELLED',owner_epoch=owner_epoch+1,lease_token_sha256='',lease_expires_at=NULL,launch_permit_sha256='',launch_permit_expires_at=NULL,cancelled_at=CURRENT_TIMESTAMP,finished_at=COALESCE(finished_at,CURRENT_TIMESTAMP),updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND status IN ('MANUAL_REVIEW','REJECTED')`, runID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE workforce_review_run_attempts SET status='CANCELLED',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE review_run_id=$1 AND status IN ('ASSIGNED','RUNNING','VALIDATING_REPORT')`, runID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE workforce_outbox_events SET status='SUPERSEDED',last_error='TASK_CANCELLED' WHERE aggregate_type IN ('review_run','assignment') AND status='PENDING' AND (aggregate_id=$1 OR aggregate_id IN (SELECT id FROM workforce_task_assignments WHERE task_id=$2))`, runID, taskID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE workforce_task_attempts SET status='CANCELLED',finished_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='MANUAL_REVIEW'`, attemptID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE workforce_task_turns SET status='CANCELLED',finished_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='MANUAL_REVIEW'`, turnID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='CANCELLED',updated_at=CURRENT_TIMESTAMP WHERE task_id=$1 AND task_turn_id=$2 AND status='MANUAL_REVIEW'`, taskID, turnID); err != nil {
		return err
	}
	if err := requireOne(ctx, tx, `UPDATE workforce_tasks SET status='CANCELLED',cancel_requested_at=CURRENT_TIMESTAMP,cancel_reason='INDEPENDENT_REVIEW_HUMAN_CANCELLED',finished_at=CURRENT_TIMESTAMP,version=version+1 WHERE id=$1 AND current_task_turn_id=$2 AND status='MANUAL_REVIEW'`, taskID, turnID); err != nil {
		return err
	}
	return eventlog.Append(ctx, tx, taskID, attemptID, "independent_review.human_cancelled", map[string]any{"reviewRunId": runID})
}

func loadHumanDecision(ctx context.Context, tx pgx.Tx, tenantID, runID int64, key string) (facade.HumanDecisionResult, error) {
	var result facade.HumanDecisionResult
	err := tx.QueryRow(ctx, `SELECT id,task_id,task_turn_id,review_run_id,action,ai_verdict,canonical_report_digest,quality_outcome,COALESCE(resulting_task_turn_id,0),COALESCE(revision_guidance,''),to_char(decided_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') FROM workforce_human_review_decisions WHERE tenant_id=$1 AND review_run_id=$2 AND idempotency_key=$3`, tenantID, runID, key).
		Scan(&result.ID, &result.TaskID, &result.TaskTurnID, &result.ReviewRunID, &result.Action, &result.AIVerdict, &result.ReportDigest, &result.QualityOutcome, &result.ResultingTaskTurnID, &result.RevisionGuidance, &result.DecidedAt)
	return result, err
}

func replayHumanDecision(ctx context.Context, tx pgx.Tx, request facade.HumanDecisionRequest, requestDigest string) (facade.HumanDecisionResult, bool, error) {
	replay, err := loadHumanDecision(ctx, tx, request.TenantID, request.ReviewRunID, request.IdempotencyKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return facade.HumanDecisionResult{}, false, nil
	}
	if err != nil {
		return facade.HumanDecisionResult{}, false, err
	}
	var stored string
	if err = tx.QueryRow(ctx, `SELECT request_digest FROM workforce_human_review_decisions WHERE id=$1`, replay.ID).Scan(&stored); err != nil {
		return facade.HumanDecisionResult{}, false, err
	}
	if stored != requestDigest {
		return facade.HumanDecisionResult{}, false, domain.ErrHumanIdempotency
	}
	return replay, true, nil
}

func humanDecisionRequestDigest(action, reason, guidance, idempotencyKey string) string {
	return hashText(action + "\x00" + reason + "\x00" + guidance + "\x00" + idempotencyKey)
}

func hashText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func normalizedProofMethods(values []string) []string {
	allowed := map[string]bool{"WEBAUTHN_PASSKEY_ASSERTION": true, "PASSKEY": true, "TIME_BASED_ONE_TIME_PASSWORD": true, "TOTP": true, "EMAIL_ONE_TIME_PASSWORD": true, "EMAIL_OTP": true}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToUpper(strings.TrimSpace(value))
		if allowed[value] {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
