package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	continuationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	principalinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/eventlog"
)

// CreateReviewRevisionTurn creates a normal continuation turn while
// participating in the caller's ReviewRun transaction. This preserves one
// atomic authority boundary from canonical verdict to successor Turn.
func (s *Store) CreateReviewRevisionTurn(ctx context.Context, tx pgx.Tx, request continuationfacade.ReviewRevisionTurnRequest) (continuationfacade.ReviewRevisionTurnResult, error) {
	if s == nil || s.idGen == nil || tx == nil || request.TenantID < 1 || request.TaskID < 1 || request.SourceTaskTurnID < 1 || request.SourceAttemptID < 1 || request.SourceReviewRunID < 1 ||
		(request.RevisionKind != continuationfacade.RevisionAutomatic && request.RevisionKind != continuationfacade.RevisionHumanAuthorized) ||
		(request.RevisionKind != continuationfacade.RevisionHumanAuthorized && strings.TrimSpace(request.RevisionGuidance) != "") ||
		(request.ReportDigest != "" && !isHexDigest(request.ReportDigest)) ||
		(request.RevisionKind == continuationfacade.RevisionHumanAuthorized && request.ReportDigest == "" && (!safeRevisionGuidance(request.RevisionGuidance) || !safeReviewReasonCode(request.SafeReasonCode))) {
		return continuationfacade.ReviewRevisionTurnResult{}, continuationfacade.ErrReviewRevisionIdentityConflict
	}

	var replay continuationfacade.ReviewRevisionTurnResult
	var replayKind, replayDigest string
	err := tx.QueryRow(ctx, `SELECT id,turn_number,instruction,COALESCE(source_checkpoint_id,0),review_revision_kind,COALESCE(review_report_digest,'')
FROM workforce_task_turns WHERE source_review_run_id=$1`, request.SourceReviewRunID).
		Scan(&replay.TaskTurnID, &replay.TurnNumber, &replay.Instruction, &replay.SourceCheckpointID, &replayKind, &replayDigest)
	if err == nil {
		if replayKind != request.RevisionKind || replayDigest != request.ReportDigest {
			return continuationfacade.ReviewRevisionTurnResult{}, continuationfacade.ErrReviewRevisionIdentityConflict
		}
		replay.Replayed = true
		return replay, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return continuationfacade.ReviewRevisionTurnResult{}, err
	}

	var tenantID, currentTurnID, creatorID, lineageID, processorID int64
	var turnNumber int
	var taskStatus, turnStatus, originalInstruction, snapshotDigest, processorDigest, environmentDigest string
	var environment, secrets, parameters []byte
	err = tx.QueryRow(ctx, `SELECT t.tenant_id,COALESCE(t.current_task_turn_id,0),t.current_turn_number,t.status,t.created_by,
	tr.status,COALESCE((SELECT base.instruction FROM workforce_task_turns base WHERE base.task_id=t.id AND base.turn_number=1),tr.instruction),tr.execution_snapshot_digest,tr.continuation_lineage_id,tr.processor_release_id,tr.processor_digest,
 tr.environment_config_digest,tr.environment_snapshot_json,tr.secret_version_identities,tr.parameters_json
FROM workforce_tasks t JOIN workforce_task_turns tr ON tr.id=t.current_task_turn_id
WHERE t.id=$1 FOR UPDATE OF t,tr`, request.TaskID).Scan(
		&tenantID, &currentTurnID, &turnNumber, &taskStatus, &creatorID, &turnStatus, &originalInstruction,
		&snapshotDigest, &lineageID, &processorID, &processorDigest, &environmentDigest, &environment, &secrets, &parameters)
	if errors.Is(err, pgx.ErrNoRows) || tenantID != request.TenantID {
		return continuationfacade.ReviewRevisionTurnResult{}, continuationfacade.ErrReviewRevisionIdentityConflict
	}
	if err != nil {
		return continuationfacade.ReviewRevisionTurnResult{}, err
	}
	if currentTurnID != request.SourceTaskTurnID || (taskStatus != "REVIEWING" && taskStatus != "MANUAL_REVIEW") || (turnStatus != "REVIEWING" && turnStatus != "MANUAL_REVIEW") {
		return continuationfacade.ReviewRevisionTurnResult{}, continuationfacade.ErrReviewRevisionIdentityConflict
	}

	var checkpointID, checkpointTurnID, checkpointAttemptID, consumedBy int64
	var checkpointStatus, checkpointDecision string
	var checkpointExpires time.Time
	err = tx.QueryRow(ctx, `SELECT id,task_turn_id,attempt_id,status,decision,expires_at,COALESCE(consumed_by_turn_id,0)
FROM workforce_continuation_checkpoints
WHERE task_id=$1 AND task_turn_id=$2 AND attempt_id=$3 AND decision='PASS' AND status='AVAILABLE' AND format_version=2
ORDER BY available_at DESC,id DESC LIMIT 1 FOR UPDATE`, request.TaskID, request.SourceTaskTurnID, request.SourceAttemptID).
		Scan(&checkpointID, &checkpointTurnID, &checkpointAttemptID, &checkpointStatus, &checkpointDecision, &checkpointExpires, &consumedBy)
	if errors.Is(err, pgx.ErrNoRows) || checkpointStatus != "AVAILABLE" || checkpointDecision != "PASS" || checkpointTurnID != request.SourceTaskTurnID || checkpointAttemptID != request.SourceAttemptID || consumedBy != 0 || !checkpointExpires.After(time.Now().UTC()) {
		return continuationfacade.ReviewRevisionTurnResult{}, continuationfacade.ErrReviewRevisionCheckpointUnavailable
	}
	if err != nil {
		return continuationfacade.ReviewRevisionTurnResult{}, err
	}

	triggerPrincipalID, err := principalinfra.EnsureService(ctx, tx, s.idGen, "independent-review-revision")
	if request.RevisionKind == continuationfacade.RevisionHumanAuthorized {
		if request.TriggerUserID < 1 {
			return continuationfacade.ReviewRevisionTurnResult{}, continuationfacade.ErrReviewRevisionIdentityConflict
		}
		triggerPrincipalID, err = principalinfra.EnsureUser(ctx, tx, s.idGen, request.TriggerUserID)
	}
	if err != nil {
		return continuationfacade.ReviewRevisionTurnResult{}, err
	}

	instruction := correctionInstruction(originalInstruction, request)
	turnID, nodeID, inputID := s.idGen.NextID(), s.idGen.NextID(), s.idGen.NextID()
	nextNumber := turnNumber + 1
	requestBody, _ := json.Marshal(struct {
		RunID  int64  `json:"reviewRunId,string"`
		Kind   string `json:"kind"`
		Digest string `json:"digest"`
	}{request.SourceReviewRunID, request.RevisionKind, request.ReportDigest})
	requestSum := sha256.Sum256(requestBody)
	requestDigest := hex.EncodeToString(requestSum[:])
	idempotencyKey := fmt.Sprintf("independent-review:%d:%s", request.SourceReviewRunID, strings.ToLower(request.RevisionKind))

	if _, err = tx.Exec(ctx, `UPDATE workforce_task_turns SET status='SUPERSEDED',finished_at=COALESCE(finished_at,CURRENT_TIMESTAMP) WHERE id=$1 AND status IN ('REVIEWING','MANUAL_REVIEW')`, request.SourceTaskTurnID); err != nil {
		return continuationfacade.ReviewRevisionTurnResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_task_attempts SET status='SUPERSEDED',finished_at=COALESCE(finished_at,CURRENT_TIMESTAMP) WHERE id=$1 AND status IN ('REVIEWING','MANUAL_REVIEW')`, request.SourceAttemptID); err != nil {
		return continuationfacade.ReviewRevisionTurnResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='SUPERSEDED',updated_at=CURRENT_TIMESTAMP WHERE task_id=$1 AND task_turn_id=$2 AND status IN ('REVIEWING','MANUAL_REVIEW')`, request.TaskID, request.SourceTaskTurnID); err != nil {
		return continuationfacade.ReviewRevisionTurnResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_artifacts SET status='SUPERSEDED' WHERE task_id=$1 AND attempt_id=$2 AND status='PENDING_REVIEW'`, request.TaskID, request.SourceAttemptID); err != nil {
		return continuationfacade.ReviewRevisionTurnResult{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO workforce_task_turns
(id,task_id,tenant_id,turn_number,instruction,source_turn_id,source_checkpoint_id,execution_snapshot_digest,continuation_lineage_id,created_by,
 processor_release_id,processor_digest,environment_config_digest,environment_snapshot_json,secret_version_identities,parameters_json,status,idempotency_key,request_digest,
 review_revision_kind,source_review_run_id,review_report_digest,revision_trigger_principal_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,'ACTIVE',$17,$18,$19,$20,NULLIF($21,''),$22)`,
		turnID, request.TaskID, request.TenantID, nextNumber, instruction, request.SourceTaskTurnID, checkpointID, snapshotDigest, lineageID, creatorID,
		processorID, processorDigest, environmentDigest, environment, secrets, parameters, idempotencyKey, requestDigest,
		request.RevisionKind, request.SourceReviewRunID, request.ReportDigest, triggerPrincipalID)
	if err != nil {
		return continuationfacade.ReviewRevisionTurnResult{}, err
	}
	var oldInputID int64
	if _, err = tx.Exec(ctx, `UPDATE workforce_task_turns target SET completion_check_json=source.completion_check_json,completion_check_digest=source.completion_check_digest,completion_check_references_json=source.completion_check_references_json,completion_check_input_digest=source.completion_check_input_digest,completion_check_baseline_result_id=source.completion_check_baseline_result_id FROM workforce_task_turns source WHERE target.id=$1 AND source.id=$2 AND target.tenant_id=source.tenant_id AND target.task_id=source.task_id`, turnID, request.SourceTaskTurnID); err != nil {
		return continuationfacade.ReviewRevisionTurnResult{}, err
	}
	if err = tx.QueryRow(ctx, `SELECT id FROM workforce_task_input_snapshots WHERE task_id=$1 AND task_turn_id=$2`, request.TaskID, request.SourceTaskTurnID).Scan(&oldInputID); err != nil {
		return continuationfacade.ReviewRevisionTurnResult{}, fmt.Errorf("TASK_INPUT_SNAPSHOT_MISSING: %w", err)
	}
	if _, err = tx.Exec(ctx, continuationInputSnapshotCopySQL, inputID, turnID, oldInputID); err != nil {
		return continuationfacade.ReviewRevisionTurnResult{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_task_input_references (task_input_snapshot_id,reference_asset_id,reference_content_sha256,display_order) SELECT $1,reference_asset_id,reference_content_sha256,display_order FROM workforce_task_input_references WHERE task_input_snapshot_id=$2`, inputID, oldInputID); err != nil {
		return continuationfacade.ReviewRevisionTurnResult{}, err
	}
	var sourcePlanID int64
	if err = tx.QueryRow(ctx, `SELECT id FROM workforce_task_git_plans WHERE task_id=$1 AND task_turn_id=$2`, request.TaskID, request.SourceTaskTurnID).Scan(&sourcePlanID); err != nil {
		return continuationfacade.ReviewRevisionTurnResult{}, err
	}
	if _, err = tx.Exec(ctx, continuationGitPlanCopySQL, s.idGen.NextID(), turnID, sourcePlanID); err != nil {
		return continuationfacade.ReviewRevisionTurnResult{}, err
	}
	if tag, updateErr := tx.Exec(ctx, `UPDATE workforce_continuation_checkpoints SET consumed_by_turn_id=$2,consumed_at=CURRENT_TIMESTAMP WHERE id=$1 AND consumed_by_turn_id IS NULL`, checkpointID, turnID); updateErr != nil || tag.RowsAffected() != 1 {
		if updateErr != nil {
			return continuationfacade.ReviewRevisionTurnResult{}, updateErr
		}
		return continuationfacade.ReviewRevisionTurnResult{}, continuationfacade.ErrReviewRevisionIdentityConflict
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_execution_nodes (id,task_id,task_turn_id,node_key,status,generation,execution_requirement_digest,wait_reason_code,max_attempts,retry_count) VALUES ($1,$2,$3,'main','QUEUED',1,$4,'NO_ONLINE_WORKER',3,0)`, nodeID, request.TaskID, turnID, snapshotDigest); err != nil {
		return continuationfacade.ReviewRevisionTurnResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET current_task_turn_id=$2,current_turn_number=$3,task_instruction=$4,status='QUEUED',result_summary='',latest_attempt_id=NULL,started_at=NULL,finished_at=NULL,verified_at=NULL,queued_at=CURRENT_TIMESTAMP,version=version+1 WHERE id=$1`, request.TaskID, turnID, nextNumber, instruction); err != nil {
		return continuationfacade.ReviewRevisionTurnResult{}, err
	}
	if err = eventlog.Append(ctx, tx, request.TaskID, request.SourceAttemptID, "independent_review.revision_turn_created", map[string]any{"reviewRunId": request.SourceReviewRunID, "taskTurnId": turnID, "turnNumber": nextNumber, "sourceTaskTurnId": request.SourceTaskTurnID, "revisionKind": request.RevisionKind, "reportDigest": request.ReportDigest}); err != nil {
		return continuationfacade.ReviewRevisionTurnResult{}, err
	}
	return continuationfacade.ReviewRevisionTurnResult{TaskTurnID: turnID, SourceCheckpointID: checkpointID, TurnNumber: nextNumber, Instruction: instruction}, nil
}

func correctionInstruction(original string, request continuationfacade.ReviewRevisionTurnRequest) string {
	original = stripManagedCorrection(original)
	var directive string
	if request.ReportDigest != "" {
		directive = fmt.Sprintf("Use the immutable independent-review report mounted in this attempt's read-only input at %s (SHA-256 %s). Address its actionable findings, preserve unrelated behavior, and rerun deterministic validation.", continuationfacade.ReviewReportPathPlaceholder, request.ReportDigest)
		if guidance := strings.TrimSpace(request.RevisionGuidance); guidance != "" {
			directive += "\n\nAuthorized revision guidance:\n" + guidance
		}
	} else {
		directive = fmt.Sprintf("Independent review requires a human-authorized correction. Reason code: %s.\n\nAuthorized revision guidance:\n%s\n\nAddress the approved correction and rerun deterministic validation.", request.SafeReasonCode, strings.TrimSpace(request.RevisionGuidance))
	}
	return strings.TrimSpace(original) + "\n\n[System-managed independent review correction]\n" + directive
}

func stripManagedCorrection(instruction string) string {
	return continuationfacade.PublicInstruction(instruction)
}

func safeRevisionGuidance(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 4000 {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func safeReviewReasonCode(value string) bool {
	switch value {
	case "INDEPENDENT_REVIEW_REVISION_LIMIT_EXHAUSTED",
		"INDEPENDENT_REVIEW_NO_MATERIAL_PROGRESS",
		"INDEPENDENT_REVIEW_CHECKPOINT_UNAVAILABLE",
		"INDEPENDENT_REVIEW_REVISION_IDENTITY_CONFLICT",
		"INDEPENDENT_REVIEW_REJECTED",
		"INDEPENDENT_REVIEW_REPORT_INVALID",
		"INDEPENDENT_REVIEW_RETRY_EXHAUSTED":
		return true
	default:
		return false
	}
}

func isHexDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
