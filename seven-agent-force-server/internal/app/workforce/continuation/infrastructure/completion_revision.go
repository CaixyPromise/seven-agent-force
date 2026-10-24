package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"

	completion "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/checkpoint"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/eventlog"
	"github.com/jackc/pgx/v5"
)

var _ facade.CompletionRevisionTurnCreator = (*Store)(nil)

// CreateCompletionRevisionTurnTx is called by the CompletionCheck application
// after its machine safe-point authority and additive revision policy. It never
// fabricates an evaluation or interaction. All writes remain in caller's tx.
func (s *Store) CreateCompletionRevisionTurnTx(ctx context.Context, tx pgx.Tx, r facade.CompletionRevisionTurnRequest) (facade.CompletionRevisionTurnResult, error) {
	fail := func() (facade.CompletionRevisionTurnResult, error) {
		return facade.CompletionRevisionTurnResult{}, completion.ErrConflict
	}
	if s == nil || s.idGen == nil || tx == nil {
		return facade.CompletionRevisionTurnResult{}, completion.ErrRevisionUnavailable
	}
	if r.TenantID <= 0 || r.TaskID <= 0 || r.SourceTaskTurnID <= 0 || r.SourceAttemptID <= 0 || r.CheckpointID <= 0 || r.RevisionID <= 0 || r.CreatorUserID <= 0 {
		return fail()
	}
	var input completion.Input
	var refs []completion.Reference
	if json.Unmarshal(r.InputJSON, &input) != nil || json.Unmarshal(r.ReferencesJSON, &refs) != nil {
		return fail()
	}
	frozen, err := completion.Freeze(&input)
	if err != nil || frozen.Digest != r.InputDigest {
		return fail()
	}
	digest, err := completion.FrozenDigest(input, refs)
	if err != nil || digest != r.CheckDigest {
		return fail()
	}
	var current int64
	var status string
	var cancelled bool
	if err = tx.QueryRow(ctx, `SELECT current_task_turn_id,status,cancel_requested_at IS NOT NULL FROM workforce_tasks WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, r.TaskID, r.TenantID).Scan(&current, &status, &cancelled); err != nil || cancelled {
		return fail()
	}
	key := "completion-revision:" + strconv.FormatInt(r.RevisionID, 10)
	body, _ := json.Marshal(r)
	sum := sha256.Sum256(body)
	requestDigest := hex.EncodeToString(sum[:])
	var replay, source, cp int64
	var stored string
	err = tx.QueryRow(ctx, `SELECT id,source_turn_id,source_checkpoint_id,request_digest FROM workforce_task_turns WHERE task_id=$1 AND tenant_id=$2 AND idempotency_key=$3`, r.TaskID, r.TenantID, key).Scan(&replay, &source, &cp, &stored)
	if err == nil {
		if current != replay || source != r.SourceTaskTurnID || cp != r.CheckpointID || stored != requestDigest {
			return fail()
		}
		return facade.CompletionRevisionTurnResult{TaskTurnID: replay}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return facade.CompletionRevisionTurnResult{}, err
	}
	if current != r.SourceTaskTurnID || (status != "VALIDATING" && status != "REVIEWING") {
		return fail()
	}
	var number int
	var instruction, snapshot string
	var baseline int64
	if err = tx.QueryRow(ctx, `SELECT turn_number,instruction,execution_snapshot_digest,COALESCE(completion_check_baseline_result_id,0) FROM workforce_task_turns WHERE id=$1 AND task_id=$2 AND tenant_id=$3 FOR UPDATE`, current, r.TaskID, r.TenantID).Scan(&number, &instruction, &snapshot, &baseline); err != nil || snapshot != r.ExecutionSnapshotDigest || baseline != r.OriginalBaselineResultID {
		return fail()
	}
	var oldInput int64
	var sourceCommit, sourceIdentity string
	if err = tx.QueryRow(ctx, `SELECT id,source_commit,source_identity_digest FROM workforce_task_input_snapshots WHERE task_id=$1 AND task_turn_id=$2 AND tenant_id=$3 FOR SHARE`, r.TaskID, current, r.TenantID).Scan(&oldInput, &sourceCommit, &sourceIdentity); err != nil || sourceCommit != r.SourceCommit || sourceIdentity != r.SourceIdentityDigest {
		return fail()
	}
	var decision, session, identity, bundle, overlay, sessionKey, manifest string
	var cycle, generation int64
	var cycleNumber int
	// Runtime stop is recorded by the trusted Worker finish path: PASS remains
	// RUNNING until Outcome; WAIT_USER checkpoint publication closes its runtime.
	if err = tx.QueryRow(ctx, `SELECT c.decision,e.session_digest,c.checkpoint_identity_sha256,c.repository_bundle_object_key,c.workspace_overlay_object_key,c.session_object_key,c.identity_manifest_object_key,e.id,a.generation,e.cycle_number
 FROM workforce_continuation_checkpoints c JOIN workforce_result_evaluation_cycles e ON e.id=c.evaluation_cycle_id AND e.task_id=c.task_id AND e.task_turn_id=c.task_turn_id AND e.attempt_id=c.attempt_id
 JOIN workforce_task_attempts a ON a.id=c.attempt_id AND a.task_id=c.task_id AND a.task_turn_id=c.task_turn_id
 WHERE c.id=$1 AND c.task_id=$2 AND c.task_turn_id=$3 AND c.attempt_id=$4 AND c.purpose='RESULT' AND c.interaction_id IS NULL AND c.status='AVAILABLE' AND c.format_version=2 AND c.available_at IS NOT NULL AND c.consumed_by_turn_id IS NULL AND c.consumed_at IS NULL AND c.expires_at>clock_timestamp()
 AND c.task_execution_snapshot_digest=$5 AND e.status='PERSISTED' AND e.persisted_at IS NOT NULL AND e.completed_at IS NOT NULL AND e.decision=c.decision AND c.decision IN ('PASS','WAIT_USER')
 AND ((c.decision='PASS' AND a.status='RUNNING') OR (c.decision='WAIT_USER' AND a.status='WAITING_USER'))
 FOR UPDATE OF c,e,a`, r.CheckpointID, r.TaskID, current, r.SourceAttemptID, snapshot).Scan(&decision, &session, &identity, &bundle, &overlay, &sessionKey, &manifest, &cycle, &generation, &cycleNumber); err != nil {
		return fail()
	}
	// Keep latest-cycle validation in this transaction under the existing Task
	// and source-row locks, without a fourth physical table in the SELECT.
	var newerCycle bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workforce_result_evaluation_cycles WHERE attempt_id=$1 AND cycle_number>$2)`, r.SourceAttemptID, cycleNumber).Scan(&newerCycle); err != nil || newerCycle {
		return fail()
	}
	if (decision == "PASS" && status != "VALIDATING") || (decision == "WAIT_USER" && status != "REVIEWING") {
		return fail()
	}
	ci := checkpoint.Input{TaskID: r.TaskID, TaskTurnID: current, AttemptID: r.SourceAttemptID, Generation: generation, EvaluationCycleID: cycle, Decision: decision, SessionDigest: session}
	expected := checkpoint.Identity(ci)
	b, o, se, ma := checkpoint.ObjectKeysV2(ci, expected)
	if identity != expected || bundle != b || overlay != o || sessionKey != se || manifest != ma {
		return fail()
	}
	var runtimeOK bool
	if err = tx.QueryRow(ctx, `SELECT r.worker_id=a.worker_id AND r.worker_instance_id=a.worker_instance_id AND r.host_boot_id=a.host_boot_id AND r.generation=a.generation AND r.owner_epoch=a.owner_epoch AND r.execution_lease_sha256=a.lease_token AND a.lease_expires_at>clock_timestamp() AND r.execution_lease_expires_at>clock_timestamp() AND r.fenced_at IS NULL AND r.lost_at IS NULL AND r.terminating_at IS NULL AND (($2='PASS' AND r.status='RUNNING') OR ($2='WAIT_USER' AND r.status='SUCCEEDED')) FROM workforce_attempt_runtimes r JOIN workforce_task_attempts a ON a.id=r.attempt_id WHERE r.attempt_id=$1 FOR UPDATE OF r`, r.SourceAttemptID, decision).Scan(&runtimeOK); err != nil || !runtimeOK {
		return fail()
	}
	// The frozen Project repository snapshot and original input are copied;
	// mutable Project changes cannot redirect an existing continuation.
	var planID int64
	var frozenRepositoryValid bool
	if err = tx.QueryRow(ctx, `SELECT id,project_id IS NULL OR (project_version_id>0 AND repository_config_digest ~ '^[0-9a-f]{64}$' AND repository_display_url<>'' AND (delivery_mode='NONE' OR (octet_length(repository_credential_ciphertext)>0 AND octet_length(repository_credential_edek)>0 AND repository_credential_wrap_key_ref<>'' AND repository_credential_version>0))) FROM workforce_task_git_plans WHERE task_id=$1 AND task_turn_id=$2 AND tenant_id=$3 FOR SHARE`, r.TaskID, current, r.TenantID).Scan(&planID, &frozenRepositoryValid); err != nil || !frozenRepositoryValid {
		return fail()
	}
	turnID, nodeID, inputID := s.idGen.NextID(), s.idGen.NextID(), s.idGen.NextID()
	execOne := func(sql string, args ...any) error {
		tag, e := tx.Exec(ctx, sql, args...)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return completion.ErrConflict
		}
		return nil
	}
	// Retire the current ACTIVE row before inserting its successor: the real
	// schema enforces one ACTIVE turn per Task. Caller rollback restores it.
	if err = execOne(`UPDATE workforce_task_turns SET status='SUPERSEDED',finished_at=COALESCE(finished_at,CURRENT_TIMESTAMP) WHERE id=$1 AND task_id=$2 AND tenant_id=$3 AND status IN ('ACTIVE','REVIEWING')`, current, r.TaskID, r.TenantID); err != nil {
		return facade.CompletionRevisionTurnResult{}, err
	}
	if err = execOne(`INSERT INTO workforce_task_turns(id,task_id,tenant_id,turn_number,instruction,source_turn_id,source_checkpoint_id,execution_snapshot_digest,continuation_lineage_id,created_by,processor_release_id,processor_digest,environment_config_digest,environment_snapshot_json,secret_version_identities,parameters_json,status,idempotency_key,request_digest,completion_check_json,completion_check_digest,completion_check_input_digest,completion_check_references_json)
 SELECT $1,task_id,tenant_id,turn_number+1,instruction,id,$2,execution_snapshot_digest,continuation_lineage_id,$3,processor_release_id,processor_digest,environment_config_digest,environment_snapshot_json,secret_version_identities,parameters_json,'ACTIVE',$4,$5,$6,$7,$8,$9 FROM workforce_task_turns WHERE id=$10 AND tenant_id=$11 AND task_id=$12`, turnID, r.CheckpointID, r.CreatorUserID, key, requestDigest, r.InputJSON, r.CheckDigest, r.InputDigest, r.ReferencesJSON, current, r.TenantID, r.TaskID); err != nil {
		return facade.CompletionRevisionTurnResult{}, err
	}
	if err = execOne(continuationInputSnapshotCopySQL, inputID, turnID, oldInput); err != nil {
		return facade.CompletionRevisionTurnResult{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_task_input_references(task_input_snapshot_id,reference_asset_id,reference_content_sha256,display_order) SELECT $1,reference_asset_id,reference_content_sha256,display_order FROM workforce_task_input_references WHERE task_input_snapshot_id=$2`, inputID, oldInput); err != nil {
		return facade.CompletionRevisionTurnResult{}, err
	}
	if err = execOne(continuationGitPlanCopySQL, s.idGen.NextID(), turnID, planID); err != nil {
		return facade.CompletionRevisionTurnResult{}, err
	}
	if err = execOne(`UPDATE workforce_continuation_checkpoints SET consumed_by_turn_id=$2,consumed_at=CURRENT_TIMESTAMP WHERE id=$1 AND purpose='RESULT' AND consumed_by_turn_id IS NULL`, r.CheckpointID, turnID); err != nil {
		return facade.CompletionRevisionTurnResult{}, err
	}
	// Supersede only the stopped source occurrence, without declaring a trusted
	// result or changing its baseline/evaluation history or generating Finalizer.
	for _, sql := range []string{
		`UPDATE workforce_execution_nodes SET status='SUPERSEDED',updated_at=CURRENT_TIMESTAMP WHERE task_id=$1 AND task_turn_id=$2`,
	} {
		if _, err = tx.Exec(ctx, sql, r.TaskID, current); err != nil {
			return facade.CompletionRevisionTurnResult{}, err
		}
	}
	if err = execOne(`UPDATE workforce_task_attempts SET status='SUPERSEDED',finished_at=COALESCE(finished_at,CURRENT_TIMESTAMP) WHERE id=$1 AND task_id=$2 AND task_turn_id=$3`, r.SourceAttemptID, r.TaskID, current); err != nil {
		return facade.CompletionRevisionTurnResult{}, err
	}
	if err = execOne(`UPDATE workforce_attempt_runtimes SET status='SUCCEEDED',terminal_reason_code='COMPLETION_CHECK_REVISED',updated_at=CURRENT_TIMESTAMP WHERE attempt_id=$1`, r.SourceAttemptID); err != nil {
		return facade.CompletionRevisionTurnResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_task_assignments SET status='COMPLETED',completed_at=COALESCE(completed_at,CURRENT_TIMESTAMP),release_reason_code='COMPLETION_CHECK_REVISED' WHERE id=(SELECT assignment_id FROM workforce_task_attempts WHERE id=$1)`, r.SourceAttemptID); err != nil {
		return facade.CompletionRevisionTurnResult{}, err
	}
	if err = execOne(`INSERT INTO workforce_execution_nodes(id,task_id,task_turn_id,node_key,status,generation,execution_requirement_digest,wait_reason_code,max_attempts,retry_count) VALUES($1,$2,$3,'main','QUEUED',1,$4,'NO_ONLINE_WORKER',3,0)`, nodeID, r.TaskID, turnID, snapshot); err != nil {
		return facade.CompletionRevisionTurnResult{}, err
	}
	if err = execOne(`UPDATE workforce_tasks SET current_task_turn_id=$2,current_turn_number=$3,status='QUEUED',result_summary='',latest_attempt_id=NULL,started_at=NULL,finished_at=NULL,verified_at=NULL,queued_at=CURRENT_TIMESTAMP,version=version+1 WHERE id=$1 AND current_task_turn_id=$4 AND cancel_requested_at IS NULL`, r.TaskID, turnID, number+1, current); err != nil {
		return facade.CompletionRevisionTurnResult{}, err
	}
	if err = eventlog.Append(ctx, tx, r.TaskID, r.SourceAttemptID, "task.turn.created", map[string]any{"taskTurnId": turnID, "sourceTurnId": current, "sourceCheckpointId": r.CheckpointID, "revisionId": r.RevisionID}); err != nil {
		return facade.CompletionRevisionTurnResult{}, err
	}
	return facade.CompletionRevisionTurnResult{TaskTurnID: turnID}, nil
}
