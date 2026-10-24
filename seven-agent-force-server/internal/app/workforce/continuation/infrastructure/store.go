package infrastructure

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	artifactdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/artifact/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	interactionfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	principalinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/checkpoint"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/eventlog"
)

var ErrNotFound = errors.New("CONTINUATION_NOT_FOUND")

type IDGenerator interface{ NextID() int64 }

type Store struct {
	db                   *pgxpool.Pool
	idGen                IDGenerator
	interactionQuestions interactionfacade.RestoredQuestionReader
}

func New(db *pgxpool.Pool, idGen ...IDGenerator) *Store {
	store := &Store{db: db}
	if len(idGen) > 0 {
		store.idGen = idGen[0]
	}
	return store
}

// A continued Turn keeps the exact input identity from its source Turn. The
// source kind is part of that identity and is also required by the product
// input summary after the old Runtime has been removed.
const continuationInputSnapshotCopySQL = `INSERT INTO workforce_task_input_snapshots (id,tenant_id,task_id,task_turn_id,source_kind,workspace_source_revision_id,source_commit,source_tree,source_identity_digest,repository_bundle_object_key,repository_bundle_content_sha256,repository_bundle_object_sha256,repository_bundle_size_bytes,input_snapshot_digest) SELECT $1,tenant_id,task_id,$2,source_kind,workspace_source_revision_id,source_commit,source_tree,source_identity_digest,repository_bundle_object_key,repository_bundle_content_sha256,repository_bundle_object_sha256,repository_bundle_size_bytes,input_snapshot_digest FROM workforce_task_input_snapshots WHERE id=$3`

const continuationGitPlanCopySQL = `INSERT INTO workforce_task_git_plans (id,tenant_id,task_id,task_turn_id,project_id,project_version_id,repository_provider_type,repository_scheme,repository_host,repository_port,repository_path,repository_display_url,repository_web_url,repository_config_digest,repository_credential_ciphertext,repository_credential_edek,repository_credential_wrap_key_ref,repository_credential_version,review_base_source_revision_id,source_ref_type,source_ref_name,source_commit,review_base_branch,review_base_commit,result_branch,delivery_mode,delivery_authorization,delivery_authorized_by_principal_id,delivery_authorized_at,target_config_digest,push_authorization_digest,review_authorization_digest,full_project_import,previous_delivery_commit,plan_digest,status) SELECT $1,tenant_id,task_id,$2,project_id,project_version_id,repository_provider_type,repository_scheme,repository_host,repository_port,repository_path,repository_display_url,repository_web_url,repository_config_digest,repository_credential_ciphertext,repository_credential_edek,repository_credential_wrap_key_ref,repository_credential_version,review_base_source_revision_id,source_ref_type,source_ref_name,source_commit,review_base_branch,review_base_commit,result_branch,delivery_mode,delivery_authorization,delivery_authorized_by_principal_id,delivery_authorized_at,target_config_digest,push_authorization_digest,review_authorization_digest,full_project_import,previous_delivery_commit,plan_digest,'ACTIVE' FROM workforce_task_git_plans WHERE id=$3`

const continuationGitPlanCopyWithLatestDeliverySQL = `INSERT INTO workforce_task_git_plans (id,tenant_id,task_id,task_turn_id,project_id,project_version_id,repository_provider_type,repository_scheme,repository_host,repository_port,repository_path,repository_display_url,repository_web_url,repository_config_digest,repository_credential_ciphertext,repository_credential_edek,repository_credential_wrap_key_ref,repository_credential_version,review_base_source_revision_id,source_ref_type,source_ref_name,source_commit,review_base_branch,review_base_commit,result_branch,delivery_mode,delivery_authorization,delivery_authorized_by_principal_id,delivery_authorized_at,target_config_digest,push_authorization_digest,review_authorization_digest,full_project_import,previous_delivery_commit,plan_digest,status) SELECT $1,tenant_id,task_id,$2,project_id,project_version_id,repository_provider_type,repository_scheme,repository_host,repository_port,repository_path,repository_display_url,repository_web_url,repository_config_digest,repository_credential_ciphertext,repository_credential_edek,repository_credential_wrap_key_ref,repository_credential_version,review_base_source_revision_id,source_ref_type,source_ref_name,source_commit,review_base_branch,review_base_commit,result_branch,delivery_mode,delivery_authorization,delivery_authorized_by_principal_id,delivery_authorized_at,target_config_digest,push_authorization_digest,review_authorization_digest,full_project_import,COALESCE((SELECT delivery_commit FROM workforce_delivery_requests WHERE task_turn_id=$3 AND status='SUCCEEDED' ORDER BY finished_at DESC LIMIT 1),previous_delivery_commit),plan_digest,'ACTIVE' FROM workforce_task_git_plans WHERE task_id=$4 AND task_turn_id=$3`

func (s *Store) CreateNextTurn(ctx context.Context, request application.CreateNextTurnRequest) (domain.Turn, bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.Turn{}, false, err
	}
	defer tx.Rollback(ctx)
	turn, replay, err := s.CreateNextTurnTx(ctx, tx, request)
	if err != nil {
		return domain.Turn{}, false, err
	}
	return turn, replay, tx.Commit(ctx)
}

// CreateNextTurnTx participates in the caller's transaction without committing it.
// Serialize on the tenant-scoped Task before idempotency lookup so concurrent
// identical requests converge and replay cannot cross a tenant boundary.
func (s *Store) CreateNextTurnTx(ctx context.Context, tx pgx.Tx, request application.CreateNextTurnRequest) (domain.Turn, bool, error) {
	return s.createNextTurnTx(ctx, tx, request, false)
}

// interactionSuspend is reachable only through the separately validated cold
// answer transaction. Public continuation cannot opt into this purpose.
func (s *Store) createNextTurnTx(ctx context.Context, tx pgx.Tx, request application.CreateNextTurnRequest, interactionSuspend bool) (domain.Turn, bool, error) {
	request.Instruction = strings.TrimSpace(request.Instruction)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if tx == nil || request.TaskID <= 0 || request.TenantID <= 0 || request.CreatorUserID <= 0 || request.SourceCheckpointID <= 0 || request.ExpectedCurrentTurnID <= 0 || request.IdempotencyKey == "" || len(request.IdempotencyKey) > 128 {
		return domain.Turn{}, false, domain.ErrContinuationInstruction
	}
	if err := domain.ValidateInstruction(request.Instruction); err != nil {
		return domain.Turn{}, false, err
	}
	var lockedID int64
	err := tx.QueryRow(ctx, `SELECT id FROM workforce_tasks WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, request.TaskID, request.TenantID).Scan(&lockedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Turn{}, false, ErrNotFound
	}
	if err != nil {
		return domain.Turn{}, false, err
	}
	requestBody, _ := json.Marshal(struct {
		CreatorUserID int64  `json:"creatorUserId,string"`
		Checkpoint    int64  `json:"checkpoint,string"`
		Current       int64  `json:"current,string"`
		Instruction   string `json:"instruction"`
	}{request.CreatorUserID, request.SourceCheckpointID, request.ExpectedCurrentTurnID, request.Instruction})
	sum := sha256.Sum256(requestBody)
	requestDigest := hex.EncodeToString(sum[:])
	var replay domain.Turn
	err = scanTurn(tx.QueryRow(ctx, turnSelect+` WHERE tr.task_id=$1 AND tr.idempotency_key=$2`, request.TaskID, request.IdempotencyKey), &replay)
	if err == nil {
		var storedDigest string
		if scanErr := tx.QueryRow(ctx, `SELECT request_digest FROM workforce_task_turns WHERE id=$1`, replay.ID).Scan(&storedDigest); scanErr != nil {
			return domain.Turn{}, false, scanErr
		}
		if storedDigest != requestDigest {
			return domain.Turn{}, false, domain.ErrIdempotencyConflict
		}
		return replay, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Turn{}, false, err
	}

	var source domain.Source
	var tenantID, processorID, lineageID, checkpointTurnID int64
	var snapshotDigest, processorDigest, environmentDigest string
	var environment, secrets, parameters []byte
	var turnNumber, checkpointFormat int
	var checkpointExpires time.Time
	var approvedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT t.tenant_id,COALESCE(t.current_task_turn_id,0),t.current_turn_number,
		tr.status,tr.execution_snapshot_digest,tr.continuation_lineage_id,tr.processor_release_id,tr.processor_digest,tr.environment_config_digest,tr.environment_snapshot_json,tr.secret_version_identities,tr.parameters_json,
		c.task_turn_id,c.decision,c.status,c.expires_at,COALESCE(c.consumed_by_turn_id,0),c.approved_at,c.format_version
		FROM workforce_tasks t
		JOIN workforce_task_turns tr ON tr.id=t.current_task_turn_id
		JOIN workforce_continuation_checkpoints c ON c.id=$2 AND c.task_id=t.id
		WHERE t.id=$1 FOR UPDATE OF t,tr,c`, request.TaskID, request.SourceCheckpointID).
		Scan(&tenantID, &source.CurrentTurnID, &turnNumber, &source.CurrentTurnStatus, &snapshotDigest, &lineageID, &processorID, &processorDigest, &environmentDigest, &environment, &secrets, &parameters,
			&checkpointTurnID, &source.CheckpointDecision, &source.CheckpointStatus, &checkpointExpires, &source.ConsumedByTurnID, &approvedAt, &checkpointFormat)
	if errors.Is(err, pgx.ErrNoRows) || tenantID != request.TenantID {
		return domain.Turn{}, false, ErrNotFound
	}
	if err != nil {
		return domain.Turn{}, false, err
	}
	if checkpointFormat != 2 {
		return domain.Turn{}, false, domain.ErrCheckpointFormat
	}
	source.CheckpointID = request.SourceCheckpointID
	source.CheckpointExpires = checkpointExpires
	source.ApprovedAt = approvedAt
	if checkpointTurnID != source.CurrentTurnID {
		return domain.Turn{}, false, domain.ErrTaskTurnConflict
	}
	if interactionSuspend {
		if source.CurrentTurnID != request.ExpectedCurrentTurnID || source.CurrentTurnStatus != domain.InteractionSuspended || source.CheckpointDecision != "" || source.CheckpointStatus != "AVAILABLE" || source.ConsumedByTurnID != 0 || !source.CheckpointExpires.After(time.Now().UTC()) {
			return domain.Turn{}, false, domain.ErrInteractionSuspend
		}
	} else if err = source.ValidateCreate(request.ExpectedCurrentTurnID, time.Now().UTC()); err != nil {
		return domain.Turn{}, false, err
	}
	var frozenRepositoryValid bool
	if err = tx.QueryRow(ctx, `SELECT project_id IS NULL OR (project_version_id>0 AND repository_config_digest ~ '^[0-9a-f]{64}$' AND repository_display_url<>'' AND (delivery_mode='NONE' OR (octet_length(repository_credential_ciphertext)>0 AND octet_length(repository_credential_edek)>0 AND repository_credential_wrap_key_ref<>'' AND repository_credential_version>0))) FROM workforce_task_git_plans WHERE task_id=$1 AND task_turn_id=$2 AND tenant_id=$3 FOR SHARE`, request.TaskID, source.CurrentTurnID, tenantID).Scan(&frozenRepositoryValid); err != nil || !frozenRepositoryValid {
		return domain.Turn{}, false, errors.New("TASK_REPOSITORY_SNAPSHOT_INVALID")
	}

	if s.idGen == nil {
		return domain.Turn{}, false, errors.New("workforce ID generator is not configured")
	}
	turnID, nodeID := s.idGen.NextID(), s.idGen.NextID()
	nextNumber := turnNumber + 1
	if _, err = tx.Exec(ctx, `UPDATE workforce_task_turns SET status=CASE WHEN status='ACTIVE' THEN $2 ELSE status END,finished_at=COALESCE(finished_at,CURRENT_TIMESTAMP) WHERE id=$1`, source.CurrentTurnID, source.CurrentTurnStatus); err != nil {
		return domain.Turn{}, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_task_turns
		(id,task_id,tenant_id,turn_number,instruction,source_turn_id,source_checkpoint_id,execution_snapshot_digest,continuation_lineage_id,created_by,
		 processor_release_id,processor_digest,environment_config_digest,environment_snapshot_json,secret_version_identities,parameters_json,status,idempotency_key,request_digest)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,'ACTIVE',$17,$18)`,
		turnID, request.TaskID, tenantID, nextNumber, request.Instruction, source.CurrentTurnID, request.SourceCheckpointID, snapshotDigest, lineageID, request.CreatorUserID,
		processorID, processorDigest, environmentDigest, environment, secrets, parameters, request.IdempotencyKey, requestDigest); err != nil {
		return domain.Turn{}, false, err
	}
	newInputID := s.idGen.NextID()
	if _, err = tx.Exec(ctx, `UPDATE workforce_task_turns target SET completion_check_json=source.completion_check_json,completion_check_digest=source.completion_check_digest,completion_check_references_json=source.completion_check_references_json,completion_check_input_digest=source.completion_check_input_digest,completion_check_baseline_result_id=source.completion_check_baseline_result_id FROM workforce_task_turns source WHERE target.id=$1 AND source.id=$2 AND target.tenant_id=source.tenant_id AND target.task_id=source.task_id`, turnID, source.CurrentTurnID); err != nil {
		return domain.Turn{}, false, err
	}
	var oldInputID int64
	if err = tx.QueryRow(ctx, `SELECT id FROM workforce_task_input_snapshots WHERE task_id=$1 AND task_turn_id=$2`, request.TaskID, source.CurrentTurnID).Scan(&oldInputID); err != nil {
		return domain.Turn{}, false, fmt.Errorf("TASK_INPUT_SNAPSHOT_MISSING: %w", err)
	}
	if _, err = tx.Exec(ctx, continuationInputSnapshotCopySQL, newInputID, turnID, oldInputID); err != nil {
		return domain.Turn{}, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_task_input_references (task_input_snapshot_id,reference_asset_id,reference_content_sha256,display_order) SELECT $1,reference_asset_id,reference_content_sha256,display_order FROM workforce_task_input_references WHERE task_input_snapshot_id=$2`, newInputID, oldInputID); err != nil {
		return domain.Turn{}, false, err
	}
	if _, err = tx.Exec(ctx, continuationGitPlanCopyWithLatestDeliverySQL, s.idGen.NextID(), turnID, source.CurrentTurnID, request.TaskID); err != nil {
		return domain.Turn{}, false, err
	}
	consumeSQL := `UPDATE workforce_continuation_checkpoints SET consumed_by_turn_id=$2,consumed_at=CURRENT_TIMESTAMP WHERE id=$1 AND consumed_by_turn_id IS NULL`
	if interactionSuspend {
		consumeSQL = `UPDATE workforce_continuation_checkpoints SET consumed_by_turn_id=$2,consumed_at=CURRENT_TIMESTAMP,interaction_phase='RESUMED' WHERE id=$1 AND consumed_by_turn_id IS NULL AND interaction_phase='SUSPENDED' AND purpose='INTERACTION_SUSPEND'`
	}
	consumeTag, consumeErr := tx.Exec(ctx, consumeSQL, request.SourceCheckpointID, turnID)
	if consumeErr != nil {
		return domain.Turn{}, false, consumeErr
	}
	if consumeTag.RowsAffected() != 1 {
		return domain.Turn{}, false, domain.ErrTaskTurnConflict
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_execution_nodes (id,task_id,task_turn_id,node_key,status,generation,execution_requirement_digest,wait_reason_code,max_attempts,retry_count) VALUES ($1,$2,$3,'main','QUEUED',1,$4,'NO_ONLINE_WORKER',3,0)`, nodeID, request.TaskID, turnID, snapshotDigest); err != nil {
		return domain.Turn{}, false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET current_task_turn_id=$2,current_turn_number=$3,task_instruction=$4,status='QUEUED',result_summary='',latest_attempt_id=NULL,started_at=NULL,finished_at=NULL,verified_at=NULL,queued_at=CURRENT_TIMESTAMP,version=version+1 WHERE id=$1`, request.TaskID, turnID, nextNumber, request.Instruction); err != nil {
		return domain.Turn{}, false, err
	}
	if err = eventlog.Append(ctx, tx, request.TaskID, 0, "task.turn.created", map[string]any{"taskTurnId": turnID, "turnNumber": nextNumber, "sourceTurnId": source.CurrentTurnID, "sourceCheckpointId": request.SourceCheckpointID}); err != nil {
		return domain.Turn{}, false, err
	}
	var created domain.Turn
	err = scanTurn(tx.QueryRow(ctx, turnSelect+` WHERE tr.tenant_id=$1 AND tr.id=$2`, tenantID, turnID), &created)
	return created, false, err
}

func (s *Store) Approve(ctx context.Context, request application.ApproveRequest) (domain.Checkpoint, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.Checkpoint{}, err
	}
	defer tx.Rollback(ctx)
	principalID, err := principalinfra.EnsureUser(ctx, tx, s.idGen, request.Actor)
	if err != nil {
		return domain.Checkpoint{}, err
	}
	tag, err := tx.Exec(ctx, `UPDATE workforce_continuation_checkpoints c SET approved_by_principal_id=$3,approved_at=CURRENT_TIMESTAMP,approval_reason=$4
		FROM workforce_tasks t WHERE c.id=$1 AND c.task_id=t.id AND t.tenant_id=$2 AND c.decision='MANUAL_REVIEW' AND c.status='AVAILABLE' AND c.consumed_by_turn_id IS NULL`, request.CheckpointID, request.TenantID, principalID, request.Reason)
	if err != nil {
		return domain.Checkpoint{}, err
	}
	if tag.RowsAffected() != 1 {
		return domain.Checkpoint{}, ErrNotFound
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Checkpoint{}, err
	}
	return s.GetCheckpoint(ctx, request.TenantID, request.CheckpointID)
}

const turnSelect = `SELECT tr.id,tr.task_id,tr.tenant_id,tr.turn_number,tr.instruction,tr.status,COALESCE(tr.source_turn_id,0),COALESCE(tr.source_checkpoint_id,0),tr.execution_snapshot_digest,tr.continuation_lineage_id,tr.created_by,COALESCE(tr.result_commit,''),COALESCE(tr.result_tree,''),COALESCE(tr.result_summary,''),to_char(tr.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),COALESCE(to_char(tr.started_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(to_char(tr.finished_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),'') FROM workforce_task_turns tr`

func scanTurn(row pgx.Row, item *domain.Turn) error {
	return row.Scan(&item.ID, &item.TaskID, &item.TenantID, &item.TurnNumber, &item.Instruction, &item.Status, &item.SourceTurnID, &item.SourceCheckpointID, &item.ExecutionSnapshotDigest, &item.ContinuationLineageID, &item.CreatedBy, &item.ResultCommit, &item.ResultTree, &item.ResultSummary, &item.CreatedAt, &item.StartedAt, &item.FinishedAt)
}

func (s *Store) ListTurns(ctx context.Context, tenantID int64, taskID int64) ([]domain.Turn, error) {
	rows, err := s.db.Query(ctx, turnSelect+` WHERE tr.tenant_id=$1 AND tr.task_id=$2 ORDER BY tr.turn_number`, tenantID, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.Turn, 0)
	for rows.Next() {
		var item domain.Turn
		if err = rows.Scan(&item.ID, &item.TaskID, &item.TenantID, &item.TurnNumber, &item.Instruction, &item.Status, &item.SourceTurnID, &item.SourceCheckpointID, &item.ExecutionSnapshotDigest, &item.ContinuationLineageID, &item.CreatedBy, &item.ResultCommit, &item.ResultTree, &item.ResultSummary, &item.CreatedAt, &item.StartedAt, &item.FinishedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetTurn(ctx context.Context, tenantID int64, turnID int64) (domain.Turn, error) {
	var item domain.Turn
	err := scanTurn(s.db.QueryRow(ctx, turnSelect+` WHERE tr.tenant_id=$1 AND tr.id=$2`, tenantID, turnID), &item)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Turn{}, ErrNotFound
	}
	return item, err
}

func (s *Store) GetCheckpoint(ctx context.Context, tenantID int64, checkpointID int64) (domain.Checkpoint, error) {
	var item domain.Checkpoint
	err := s.db.QueryRow(ctx, `SELECT c.id,c.task_id,c.task_turn_id,c.attempt_id,c.decision,c.status,c.checkpoint_identity_sha256,c.manifest_sha256,to_char(c.expires_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),COALESCE(c.approved_by_principal_id,0),COALESCE(to_char(c.approved_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(c.approval_reason,''),COALESCE(c.consumed_by_turn_id,0),COALESCE(to_char(c.consumed_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),'') FROM workforce_continuation_checkpoints c JOIN workforce_tasks t ON t.id=c.task_id WHERE c.id=$1 AND t.tenant_id=$2`, checkpointID, tenantID).Scan(&item.ID, &item.TaskID, &item.TaskTurnID, &item.AttemptID, &item.Decision, &item.Status, &item.CheckpointIdentity, &item.ManifestSHA256, &item.ExpiresAt, &item.ApprovedBy, &item.ApprovedAt, &item.ApprovalReason, &item.ConsumedByTurnID, &item.ConsumedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Checkpoint{}, ErrNotFound
	}
	return item, err
}

func (s *Store) AuthorizeCheckpointKey(ctx context.Context, authority application.MachineAuthority, checkpointIdentity string) error {
	var worker, taskID, turnID, cycleID int64
	var instance, boot, leaseHash, decision, sessionDigest string
	var generation, owner int64
	var expires time.Time
	err := s.db.QueryRow(ctx, `SELECT a.worker_id,a.worker_instance_id,a.host_boot_id,a.lease_token,a.lease_expires_at,a.generation,a.owner_epoch,a.task_id,a.task_turn_id,
		ec.id,ec.decision,ec.session_digest
		FROM workforce_task_attempts a
		JOIN workforce_tasks t ON t.id=a.task_id AND t.current_task_turn_id=a.task_turn_id
		JOIN workforce_result_evaluation_cycles ec ON ec.attempt_id=a.id AND ec.task_turn_id=a.task_turn_id AND ec.status='PERSISTED' AND ec.decision IN ('PASS','WAIT_USER','MANUAL_REVIEW')
		WHERE a.id=$1 AND a.status IN ('RUNNING','EXECUTION_COMPLETED','WAITING_USER','MANUAL_REVIEW') ORDER BY ec.cycle_number DESC LIMIT 1`, authority.AttemptID).
		Scan(&worker, &instance, &boot, &leaseHash, &expires, &generation, &owner, &taskID, &turnID, &cycleID, &decision, &sessionDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrContinuationIdentity
	}
	if err != nil {
		return err
	}
	leaseSum := sha256.Sum256([]byte(authority.LeaseToken))
	if worker != authority.WorkerID || instance != authority.InstanceID || boot != authority.HostBootID || generation != authority.Generation || owner != authority.OwnerEpoch || leaseHash != hex.EncodeToString(leaseSum[:]) || !expires.After(time.Now()) {
		return domain.ErrContinuationIdentity
	}
	expected := checkpoint.Identity(checkpoint.Input{TaskID: taskID, TaskTurnID: turnID, AttemptID: authority.AttemptID, Generation: generation, EvaluationCycleID: cycleID, Decision: decision, SessionDigest: sessionDigest})
	if expected != checkpointIdentity {
		return domain.ErrContinuationIdentity
	}
	return nil
}

func (s *Store) ResolveRestoreKeyEnvelope(ctx context.Context, authority application.MachineAuthority, restoreRunID int64) (application.KeyEnvelope, string, error) {
	var worker, attemptID, taskID, targetTurn, currentTurn int64
	var envelopeJSON, identity, instance, boot, leaseHash string
	var generation, owner int64
	var expires time.Time
	err := s.db.QueryRow(ctx, `SELECT c.wrapped_data_key,c.checkpoint_identity_sha256,a.worker_id,a.worker_instance_id,a.host_boot_id,a.lease_token,a.lease_expires_at,a.generation,a.owner_epoch,a.id,r.task_id,r.target_turn_id
		FROM workforce_continuation_restore_runs r
		JOIN workforce_continuation_checkpoints c ON c.id=r.checkpoint_id
		JOIN workforce_task_attempts a ON a.id=r.attempt_id
		WHERE r.id=$1 AND (r.status IN ('PENDING','RUNNING') OR (r.status='RESTORED' AND c.purpose='INTERACTION_SUSPEND' AND a.status='PREPARING')) AND c.encryption_algorithm='AES-256-GCM-CHUNKED'`, restoreRunID).
		Scan(&envelopeJSON, &identity, &worker, &instance, &boot, &leaseHash, &expires, &generation, &owner, &attemptID, &taskID, &targetTurn)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.KeyEnvelope{}, "", domain.ErrContinuationIdentity
	}
	if err != nil {
		return application.KeyEnvelope{}, "", err
	}
	if err = s.db.QueryRow(ctx, `SELECT COALESCE(current_task_turn_id,0) FROM workforce_tasks WHERE id=$1`, taskID).Scan(&currentTurn); err != nil {
		return application.KeyEnvelope{}, "", err
	}
	leaseSum := sha256.Sum256([]byte(authority.LeaseToken))
	if authority.AttemptID != attemptID || worker != authority.WorkerID || instance != authority.InstanceID || boot != authority.HostBootID || generation != authority.Generation || owner != authority.OwnerEpoch || leaseHash != hex.EncodeToString(leaseSum[:]) || !expires.After(time.Now()) || targetTurn != currentTurn {
		return application.KeyEnvelope{}, "", domain.ErrContinuationIdentity
	}
	var envelope application.KeyEnvelope
	if json.Unmarshal([]byte(envelopeJSON), &envelope) != nil || envelope.Ciphertext == "" || envelope.EDEK == "" || envelope.KeyID == "" {
		return application.KeyEnvelope{}, "", domain.ErrContinuationIdentity
	}
	return envelope, identity, nil
}

const materializationSelect = `SELECT r.id,r.task_id,r.task_turn_id,r.source_checkpoint_id,COALESCE(r.output_id,0),r.kind,r.status,r.run_number,r.max_runs,COALESCE(r.artifact_id,0),r.artifact_sha256,r.artifact_size_bytes,r.reason_code,to_char(r.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),COALESCE(to_char(r.started_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(to_char(r.finished_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(to_char(r.next_retry_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),'') FROM workforce_result_materialization_runs r`

func scanMaterialization(row pgx.Row, item *domain.MaterializationView) error {
	return row.Scan(&item.ID, &item.TaskID, &item.TaskTurnID, &item.SourceCheckpointID, &item.OutputID, &item.Kind, &item.Status, &item.RunNumber, &item.MaxRuns, &item.ArtifactID, &item.ArtifactSHA256, &item.ArtifactSizeBytes, &item.ReasonCode, &item.CreatedAt, &item.StartedAt, &item.FinishedAt, &item.NextRetryAt)
}

func (s *Store) CreateMaterialization(ctx context.Context, request application.CreateMaterializationRequest) (domain.MaterializationView, bool, error) {
	body, _ := json.Marshal(struct {
		Turn   int64  `json:"turn,string"`
		Kind   string `json:"kind"`
		Output int64  `json:"output,string"`
	}{request.TaskTurnID, request.Kind, request.OutputID})
	requestSum := sha256.Sum256(body)
	requestDigest := hex.EncodeToString(requestSum[:])
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.MaterializationView{}, false, err
	}
	defer tx.Rollback(ctx)
	var replay domain.MaterializationView
	err = scanMaterialization(tx.QueryRow(ctx, materializationSelect+` WHERE r.task_id=$1 AND r.idempotency_key=$2`, request.TaskID, request.IdempotencyKey), &replay)
	if err == nil {
		var stored string
		if err = tx.QueryRow(ctx, `SELECT request_digest FROM workforce_result_materialization_runs WHERE id=$1`, replay.ID).Scan(&stored); err != nil {
			return domain.MaterializationView{}, false, err
		}
		if stored != requestDigest {
			return domain.MaterializationView{}, false, domain.ErrIdempotencyConflict
		}
		return replay, true, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.MaterializationView{}, false, err
	}
	var tenant, currentTurn, checkpointID, attemptID int64
	var turnStatus, checkpointIdentity, checkpointDecision, checkpointStatus string
	var expires time.Time
	var generation int64
	err = tx.QueryRow(ctx, `SELECT t.tenant_id,COALESCE(t.current_task_turn_id,0),tr.status,c.id,c.checkpoint_identity_sha256,c.decision,c.status,c.expires_at,c.attempt_id
		FROM workforce_tasks t
		JOIN workforce_task_turns tr ON tr.id=$3 AND tr.task_id=t.id
		JOIN workforce_continuation_checkpoints c ON c.task_id=t.id AND c.task_turn_id=tr.id
		WHERE t.id=$1 AND t.tenant_id=$2 AND c.status='AVAILABLE' AND c.format_version=2 AND c.decision IN ('PASS','WAIT_USER','MANUAL_REVIEW')
		ORDER BY c.available_at DESC LIMIT 1 FOR UPDATE OF t,tr,c`, request.TaskID, request.TenantID, request.TaskTurnID).
		Scan(&tenant, &currentTurn, &turnStatus, &checkpointID, &checkpointIdentity, &checkpointDecision, &checkpointStatus, &expires, &attemptID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.MaterializationView{}, false, ErrNotFound
	}
	if err != nil {
		return domain.MaterializationView{}, false, err
	}
	if err = tx.QueryRow(ctx, `SELECT generation FROM workforce_task_attempts WHERE id=$1`, attemptID).Scan(&generation); err != nil {
		return domain.MaterializationView{}, false, err
	}
	if tenant != request.TenantID || checkpointStatus != "AVAILABLE" || !expires.After(time.Now()) {
		return domain.MaterializationView{}, false, domain.ErrMaterializationExpired
	}
	if request.Kind == domain.MaterializationBundle {
		request.Kind = domain.MaterializationGitBundle
	}
	if request.Kind == domain.MaterializationPatch || request.Kind == domain.MaterializationGitBundle {
		if turnStatus != domain.TurnSucceeded || checkpointDecision != domain.DecisionPass {
			return domain.MaterializationView{}, false, domain.ErrMaterializationInvalid
		}
	}
	if request.Kind == domain.MaterializationDeclaredOutput || request.Kind == domain.MaterializationZIP || request.Kind == domain.MaterializationTarGZ {
		var outputTurn int64
		var outputKind, archiveFormat string
		if err = tx.QueryRow(ctx, `SELECT task_turn_id,COALESCE(output_kind,'PATH'),COALESCE(archive_format,'') FROM workforce_result_outputs WHERE id=$1 AND task_id=$2 AND storage IN ('IMMEDIATE','ON_DEMAND') AND status IN ('DECLARED','AVAILABLE')`, request.OutputID, request.TaskID).Scan(&outputTurn, &outputKind, &archiveFormat); errors.Is(err, pgx.ErrNoRows) {
			return domain.MaterializationView{}, false, domain.ErrMaterializationInvalid
		} else if err != nil {
			return domain.MaterializationView{}, false, err
		}
		if outputTurn != request.TaskTurnID || ((request.Kind == domain.MaterializationZIP || request.Kind == domain.MaterializationTarGZ) && (outputKind != "ARCHIVE" || archiveFormat != request.Kind)) {
			return domain.MaterializationView{}, false, domain.ErrMaterializationInvalid
		}
	}
	inputSum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d", checkpointIdentity, request.Kind, request.OutputID)))
	inputIdentity := hex.EncodeToString(inputSum[:])
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, fmt.Sprintf("%d:%s", request.TaskTurnID, inputIdentity)); err != nil {
		return domain.MaterializationView{}, false, err
	}
	var existing domain.MaterializationView
	err = scanMaterialization(tx.QueryRow(ctx, materializationSelect+` WHERE r.task_turn_id=$1 AND r.kind=$2 AND r.input_identity_digest=$3 AND COALESCE(r.output_id,0)=$4`, request.TaskTurnID, request.Kind, inputIdentity, request.OutputID), &existing)
	if err == nil {
		// One frozen result identity has one materialization lifecycle. A second
		// button click or a different HTTP idempotency key observes the existing
		// run instead of bypassing retry limits with parallel post-process work.
		return existing, true, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.MaterializationView{}, false, err
	}
	var workerID int64
	var instanceID, bootID string
	err = tx.QueryRow(ctx, `SELECT w.id,w.current_instance_id,s.host_boot_id FROM workforce_workers w JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id WHERE w.desired_state='ENABLED' AND w.decommissioned_at IS NULL AND s.ended_at IS NULL AND s.last_heartbeat_at>CURRENT_TIMESTAMP-INTERVAL '35 seconds' AND s.consumer_state='CONSUMING' ORDER BY s.last_heartbeat_at DESC,w.id LIMIT 1`).Scan(&workerID, &instanceID, &bootID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.MaterializationView{}, false, domain.ErrMaterializationInvalid
	}
	if err != nil {
		return domain.MaterializationView{}, false, err
	}
	runID, messageID := s.idGen.NextID(), s.idGen.NextID()
	var creatorPrincipalID int64
	if request.Actor > 0 {
		creatorPrincipalID, err = principalinfra.EnsureUser(ctx, tx, s.idGen, request.Actor)
	} else {
		creatorPrincipalID, err = principalinfra.EnsureService(ctx, tx, s.idGen, request.ActorServiceCode)
	}
	if err != nil {
		return domain.MaterializationView{}, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_result_materialization_runs
		(id,task_id,task_turn_id,source_checkpoint_id,output_id,kind,input_identity_digest,worker_id,worker_instance_id,host_boot_id,attempt_id,generation,status,idempotency_key,request_digest,created_by_principal_id)
		VALUES ($1,$2,$3,$4,NULLIF($5::BIGINT,0),$6,$7,$8,$9,$10,$11,$12,'PENDING',$13,$14,$15)`,
		runID, request.TaskID, request.TaskTurnID, checkpointID, request.OutputID, request.Kind, inputIdentity, workerID, instanceID, bootID, attemptID, generation, request.IdempotencyKey, requestDigest, creatorPrincipalID); err != nil {
		return domain.MaterializationView{}, false, err
	}
	if request.OutputID != 0 {
		if _, err = tx.Exec(ctx, `UPDATE workforce_result_outputs SET status='MATERIALIZING' WHERE id=$1 AND storage='IMMEDIATE' AND output_kind='ARCHIVE' AND status='DECLARED'`, request.OutputID); err != nil {
			return domain.MaterializationView{}, false, err
		}
	}
	message := domain.MaterializationMessage{SchemaVersion: 1, Kind: domain.MaterializationMessageKind, MessageID: messageID, WorkerID: workerID, ResourceID: runID, Generation: generation}
	messageBody, _ := json.Marshal(message)
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_outbox_events (id,aggregate_type,aggregate_id,event_type,payload_json,status,exchange_name,routing_key,message_expiration_ms) VALUES ($1,'result_materialization',$2,'result.materialization.requested',$3,'PENDING','seven.workforce.postprocess',$4,600000)`, messageID, runID, messageBody, "postprocess."+strconv.FormatInt(workerID, 10)); err != nil {
		return domain.MaterializationView{}, false, err
	}
	if err = eventlog.Append(ctx, tx, request.TaskID, attemptID, "result.materialization.requested", map[string]any{"materializationRunId": runID, "taskTurnId": request.TaskTurnID, "kind": request.Kind, "outputId": request.OutputID}); err != nil {
		return domain.MaterializationView{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.MaterializationView{}, false, err
	}
	created, err := s.GetMaterialization(ctx, request.TenantID, runID)
	return created, false, err
}

// ReconcileImmediateMaterializations turns an IMMEDIATE ARCHIVE declaration
// into the same deterministic post-process lifecycle used by an on-demand
// archive. It only runs after a format-2 Checkpoint is AVAILABLE, so the
// archive never depends on a live Agent Runtime or mutable workspace.
func (s *Store) ReconcileImmediateMaterializations(ctx context.Context, limit int) (int, error) {
	if limit <= 0 || limit > 128 {
		limit = 32
	}
	rows, err := s.db.Query(ctx, `SELECT o.id,o.task_id,o.task_turn_id,o.archive_format,t.tenant_id
		FROM workforce_result_outputs o
		JOIN workforce_tasks t ON t.id=o.task_id
		JOIN workforce_continuation_checkpoints c ON c.task_id=o.task_id AND c.task_turn_id=o.task_turn_id
		WHERE o.storage='IMMEDIATE' AND o.output_kind='ARCHIVE' AND o.status='DECLARED'
		  AND o.archive_format IN ('ZIP','TAR_GZ')
		  AND c.status='AVAILABLE' AND c.format_version=2 AND c.expires_at>CURRENT_TIMESTAMP AND c.decision IN ('PASS','WAIT_USER','MANUAL_REVIEW')
		ORDER BY o.created_at,o.id LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	type candidate struct {
		outputID int64
		taskID   int64
		turnID   int64
		format   string
		tenantID int64
	}
	candidates := make([]candidate, 0, limit)
	for rows.Next() {
		var item candidate
		if err = rows.Scan(&item.outputID, &item.taskID, &item.turnID, &item.format, &item.tenantID); err != nil {
			rows.Close()
			return 0, err
		}
		candidates = append(candidates, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	created := 0
	for _, item := range candidates {
		_, replay, createErr := s.CreateMaterialization(ctx, application.CreateMaterializationRequest{
			TaskID: item.taskID, TaskTurnID: item.turnID, TenantID: item.tenantID,
			ActorServiceCode: "result-coordinator", Kind: item.format, OutputID: item.outputID,
			IdempotencyKey: fmt.Sprintf("immediate-archive:%d:%s", item.outputID, item.format),
		})
		if createErr != nil {
			return created, createErr
		}
		if !replay {
			created++
		}
	}
	return created, nil
}

func (s *Store) GetMaterialization(ctx context.Context, tenantID int64, runID int64) (domain.MaterializationView, error) {
	var item domain.MaterializationView
	err := scanMaterialization(s.db.QueryRow(ctx, materializationSelect+` JOIN workforce_tasks t ON t.id=r.task_id WHERE r.id=$1 AND t.tenant_id=$2`, runID, tenantID), &item)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrNotFound
	}
	return item, err
}

func (s *Store) ListMaterializations(ctx context.Context, tenantID int64, taskID int64) ([]domain.MaterializationView, error) {
	rows, err := s.db.Query(ctx, materializationSelect+` JOIN workforce_tasks t ON t.id=r.task_id WHERE r.task_id=$1 AND t.tenant_id=$2 ORDER BY r.created_at DESC,r.id DESC`, taskID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.MaterializationView, 0, 8)
	for rows.Next() {
		var item domain.MaterializationView
		if err = scanMaterialization(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ClaimMaterialization(ctx context.Context, authority application.MachineAuthority, runID int64) (domain.MaterializationClaim, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.MaterializationClaim{}, err
	}
	defer tx.Rollback(ctx)
	var c domain.MaterializationClaim
	var worker int64
	var status, instance, boot, wrapped string
	var expires time.Time
	err = tx.QueryRow(ctx, `SELECT r.id,r.task_id,r.task_turn_id,r.attempt_id,r.generation,r.run_number,r.kind,COALESCE(r.output_id,0),COALESCE(o.name,''),COALESCE(o.source_path,''),COALESCE(o.output_kind,'PATH'),COALESCE(o.archive_format,''),COALESCE(o.selection_spec_json::text,'{}'),
		c.id,c.checkpoint_identity_sha256,c.task_turn_id,c.format_version,
		c.repository_bundle_object_key,c.repository_bundle_content_sha256,c.repository_bundle_object_sha256,c.repository_bundle_size_bytes,
		c.workspace_overlay_object_key,c.workspace_overlay_content_sha256,c.workspace_overlay_object_sha256,c.workspace_overlay_size_bytes,c.selection_digest,
		c.session_object_key,c.session_content_sha256,c.session_object_sha256,c.session_size_bytes,c.identity_manifest_object_key,c.manifest_content_sha256,c.manifest_object_sha256,c.manifest_size_bytes,c.task_execution_snapshot_digest,c.base_commit,c.result_commit,c.result_tree,c.session_lineage_digest,c.encryption_algorithm,c.encryption_key_id,c.wrapped_data_key,c.expires_at,
		r.status,r.worker_id,r.worker_instance_id,r.host_boot_id
		FROM workforce_result_materialization_runs r
		JOIN workforce_continuation_checkpoints c ON c.id=r.source_checkpoint_id AND c.decision IN ('PASS','WAIT_USER','MANUAL_REVIEW')
		LEFT JOIN workforce_result_outputs o ON o.id=r.output_id
		WHERE r.id=$1 AND c.format_version=2 FOR UPDATE OF r`, runID).Scan(
		&c.RunID, &c.TaskID, &c.TaskTurnID, &c.AttemptID, &c.Generation, &c.RunNumber, &c.Kind, &c.OutputID, &c.OutputName, &c.OutputSourcePath, &c.OutputKind, &c.ArchiveFormat, &c.SelectionSpec,
		&c.Restore.CheckpointID, &c.Restore.CheckpointIdentity, &c.Restore.SourceTurnID, &c.Restore.FormatVersion,
		&c.Restore.RepositoryBundleObjectKey, &c.Restore.RepositoryBundleContentSHA256, &c.Restore.RepositoryBundleObjectSHA256, &c.Restore.RepositoryBundleSizeBytes,
		&c.Restore.WorkspaceOverlayObjectKey, &c.Restore.WorkspaceOverlayContentSHA256, &c.Restore.WorkspaceOverlayObjectSHA256, &c.Restore.WorkspaceOverlaySizeBytes, &c.Restore.SelectionDigest,
		&c.Restore.SessionObjectKey, &c.Restore.SessionContentSHA256, &c.Restore.SessionObjectSHA256, &c.Restore.SessionSizeBytes, &c.Restore.ManifestObjectKey, &c.Restore.ManifestContentSHA256, &c.Restore.ManifestObjectSHA256, &c.Restore.ManifestSizeBytes, &c.Restore.TaskExecutionSnapshotDigest, &c.ExpectedBaseCommit, &c.ExpectedCommit, &c.ExpectedTree, &c.Restore.SessionLineageDigest, &c.Restore.EncryptionAlgorithm, &c.Restore.EncryptionKeyID, &wrapped, &expires,
		&status, &worker, &instance, &boot)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, err
	}
	if status != "PENDING" && status != "RETRY_WAIT" {
		return c, domain.ErrMaterializationInvalid
	}
	if !expires.After(time.Now()) {
		return c, domain.ErrMaterializationExpired
	}
	if worker != authority.WorkerID || instance != authority.InstanceID || boot != authority.HostBootID || c.Generation != authority.Generation {
		return c, domain.ErrContinuationIdentity
	}
	var current string
	if err = tx.QueryRow(ctx, `SELECT current_instance_id FROM workforce_workers WHERE id=$1 AND decommissioned_at IS NULL`, worker).Scan(&current); err != nil || current != instance {
		return c, domain.ErrContinuationIdentity
	}
	lease := randomMaterializationSecret()
	leaseExpires := time.Now().UTC().Add(5 * time.Minute)
	c.Restore.TargetTurnID = c.TaskTurnID
	c.Restore.RestoreRunID = c.RunID
	c.Restore.WrappedDataKey = wrapped
	c.LeaseToken = lease
	c.LeaseExpiresAt = leaseExpires.Format(time.RFC3339Nano)
	if _, err = tx.Exec(ctx, `UPDATE workforce_result_materialization_runs SET status='RUNNING',lease_sha256=$2,lease_expires_at=$3,started_at=COALESCE(started_at,CURRENT_TIMESTAMP),updated_at=CURRENT_TIMESTAMP WHERE id=$1`, runID, materializationHash(lease), leaseExpires); err != nil {
		return c, err
	}
	return c, tx.Commit(ctx)
}

func (s *Store) FreezeMaterialization(ctx context.Context, authority application.MachineAuthority, runID int64, lease string, result domain.MaterializationFreeze) error {
	if result.Name == "" || result.Path == "" || result.MediaType == "" || len(result.SHA256) != 64 || result.SizeBytes < 0 || len(result.SpoolIdentity) != 64 {
		return domain.ErrMaterializationInvalid
	}
	if len(result.SelectionManifest) > 0 && (!json.Valid(result.SelectionManifest) || len(result.SelectionManifestDigest) != 64 || result.SelectedFileCount < 1) {
		return domain.ErrMaterializationInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var taskID, turnID, attemptID, worker, outputID int64
	var instance, boot, status, stored, kind string
	var generation int64
	err = tx.QueryRow(ctx, `SELECT task_id,task_turn_id,attempt_id,worker_id,worker_instance_id,host_boot_id,status,COALESCE(lease_sha256,''),kind,COALESCE(output_id,0),generation FROM workforce_result_materialization_runs WHERE id=$1 FOR UPDATE`, runID).Scan(&taskID, &turnID, &attemptID, &worker, &instance, &boot, &status, &stored, &kind, &outputID, &generation)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if worker != authority.WorkerID || instance != authority.InstanceID || boot != authority.HostBootID || generation != authority.Generation || status != "RUNNING" || stored != materializationHash(lease) {
		return domain.ErrContinuationIdentity
	}
	manifest, _ := json.Marshal(result)
	if _, err = tx.Exec(ctx, `UPDATE workforce_result_materialization_runs SET status='FROZEN',artifact_sha256=$2,artifact_size_bytes=$3,artifact_name=$4,artifact_media_type=$5,spool_identity=$6,manifest_json=$7,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, runID, result.SHA256, result.SizeBytes, result.Name, result.MediaType, result.SpoolIdentity, manifest); err != nil {
		return err
	}
	if outputID != 0 && len(result.SelectionManifest) > 0 {
		if _, err = tx.Exec(ctx, `UPDATE workforce_result_outputs SET selection_manifest_json=$2,selection_manifest_digest=$3,selected_file_count=$4 WHERE id=$1 AND task_turn_id=$5`, outputID, result.SelectionManifest, result.SelectionManifestDigest, result.SelectedFileCount, turnID); err != nil {
			return err
		}
	}
	uploadID, messageID := s.idGen.NextID(), s.idGen.NextID()
	objectKey := fmt.Sprintf("tasks/%d/turns/%d/materializations/%d/%s/%s", taskID, turnID, runID, result.SHA256, result.Name)
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_artifact_upload_attempts (id,materialization_run_id,task_id,task_turn_id,attempt_id,artifact_type,artifact_name,object_key,sha256,size_bytes,media_type,worker_id,host_boot_id,status) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'PENDING')`, uploadID, runID, taskID, turnID, attemptID, "RESULT_"+kind, result.Name, objectKey, result.SHA256, result.SizeBytes, result.MediaType, worker, boot); err != nil {
		return err
	}
	message := artifactdomain.WorkMessage{SchemaVersion: 1, Kind: artifactdomain.UploadMessageKind, MessageID: messageID, WorkerID: worker, ResourceID: uploadID, Generation: generation}
	body, _ := json.Marshal(message)
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_outbox_events (id,aggregate_type,aggregate_id,event_type,payload_json,status,exchange_name,routing_key,message_expiration_ms) VALUES ($1,'artifact_upload',$2,'artifact.upload.requested',$3,'PENDING','seven.workforce.postprocess',$4,600000)`, messageID, uploadID, body, "postprocess."+strconv.FormatInt(worker, 10)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) FailMaterialization(ctx context.Context, authority application.MachineAuthority, runID int64, lease string, reason string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var worker int64
	var instance, boot, status, stored string
	var generation int64
	var run, maxRuns int
	err = tx.QueryRow(ctx, `SELECT worker_id,worker_instance_id,host_boot_id,status,COALESCE(lease_sha256,''),generation,run_number,max_runs FROM workforce_result_materialization_runs WHERE id=$1 FOR UPDATE`, runID).Scan(&worker, &instance, &boot, &status, &stored, &generation, &run, &maxRuns)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if worker != authority.WorkerID || instance != authority.InstanceID || boot != authority.HostBootID || generation != authority.Generation || status != "RUNNING" || stored != materializationHash(lease) {
		return domain.ErrContinuationIdentity
	}
	if run >= maxRuns {
		_, err = tx.Exec(ctx, `UPDATE workforce_result_materialization_runs SET status='FAILED',reason_code=$2,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, runID, reason)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	next := time.Now().UTC().Add([]time.Duration{time.Second, 10 * time.Second, 60 * time.Second}[minMaterialization(run-1, 2)])
	if _, err = tx.Exec(ctx, `UPDATE workforce_result_materialization_runs SET status='RETRY_WAIT',run_number=run_number+1,next_retry_at=$2,reason_code=$3,lease_sha256=NULL,lease_expires_at=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, runID, next, reason); err != nil {
		return err
	}
	messageID := s.idGen.NextID()
	message := domain.MaterializationMessage{SchemaVersion: 1, Kind: domain.MaterializationMessageKind, MessageID: messageID, WorkerID: worker, ResourceID: runID, Generation: generation}
	body, _ := json.Marshal(message)
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_outbox_events (id,aggregate_type,aggregate_id,event_type,payload_json,status,exchange_name,routing_key,message_expiration_ms,next_retry_at) VALUES ($1,'result_materialization',$2,'result.materialization.retry',$3,'PENDING','seven.workforce.postprocess',$4,600000,$5)`, messageID, runID, body, "postprocess."+strconv.FormatInt(worker, 10), next); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RetryMaterialization(ctx context.Context, tenantID int64, actor int64, runID int64, reason string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = principalinfra.EnsureUser(ctx, tx, s.idGen, actor); err != nil {
		return err
	}
	var worker int64
	var status string
	var generation int64
	if err = tx.QueryRow(ctx, `SELECT r.worker_id,r.status,r.generation FROM workforce_result_materialization_runs r JOIN workforce_tasks t ON t.id=r.task_id WHERE r.id=$1 AND t.tenant_id=$2 FOR UPDATE OF r`, runID, tenantID).Scan(&worker, &status, &generation); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if status != "FAILED" {
		return domain.ErrMaterializationInvalid
	}
	next := time.Now().UTC()
	if _, err = tx.Exec(ctx, `UPDATE workforce_result_materialization_runs SET status='RETRY_WAIT',run_number=run_number+1,max_runs=GREATEST(max_runs,run_number+1),reason_code='',next_retry_at=$2,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, runID, next); err != nil {
		return err
	}
	messageID := s.idGen.NextID()
	message := domain.MaterializationMessage{SchemaVersion: 1, Kind: domain.MaterializationMessageKind, MessageID: messageID, WorkerID: worker, ResourceID: runID, Generation: generation}
	body, _ := json.Marshal(message)
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_outbox_events (id,aggregate_type,aggregate_id,event_type,payload_json,status,exchange_name,routing_key,message_expiration_ms) VALUES ($1,'result_materialization',$2,'result.materialization.manual_retry',$3,'PENDING','seven.workforce.postprocess',$4,600000)`, messageID, runID, body, "postprocess."+strconv.FormatInt(worker, 10)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func randomMaterializationSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
func materializationHash(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}
func minMaterialization(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ = strings.TrimSpace
