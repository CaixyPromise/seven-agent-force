package infrastructure

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	routingfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionrouting/facade"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/eventlog"
	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
	tasknotification "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/facade"
)

type Store struct {
	db                   *pgxpool.Pool
	idGen                IDGenerator
	now                  func() time.Time
	notificationRecorder tasknotification.TransactionalRecorder
	routing              routingfacade.AssignmentResolver
}

func (s *Store) SetAssignmentResolver(resolver routingfacade.AssignmentResolver) {
	if s != nil {
		s.routing = resolver
	}
}

func (s *Store) SetTaskNotificationRecorder(recorder tasknotification.TransactionalRecorder) {
	if s != nil {
		s.notificationRecorder = recorder
	}
}

type IDGenerator interface{ NextID() int64 }

type finalizationWorkMessage struct {
	SchemaVersion int    `json:"schemaVersion"`
	Kind          string `json:"kind"`
	MessageID     int64  `json:"messageId,string"`
	WorkerID      int64  `json:"workerId,string"`
	ResourceID    int64  `json:"resourceId,string"`
	Generation    int64  `json:"generation,string"`
}

func New(db *pgxpool.Pool, idGen IDGenerator) *Store {
	return &Store{db: db, idGen: idGen, now: time.Now}
}

func NewWithClock(db *pgxpool.Pool, now func() time.Time, idGen IDGenerator) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{db: db, idGen: idGen, now: now}
}

func (s *Store) ReconcileTimeouts(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 256 {
		limit = 32
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	now := s.now().UTC()
	rows, err := tx.Query(ctx, `SELECT id,tenant_id,COALESCE(current_task_turn_id,0),status,COALESCE(latest_attempt_id,0) FROM workforce_tasks
WHERE (status='QUEUED' AND created_at<$2)
   OR (status='PREPARING' AND started_at<$3)
ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT $1`, limit, now.Add(-30*time.Minute), now.Add(-2*time.Minute))
	if err != nil {
		return 0, err
	}
	type candidate struct {
		task, tenant, turn, attempt int64
		status                      string
	}
	items := make([]candidate, 0, limit)
	for rows.Next() {
		var v candidate
		if err = rows.Scan(&v.task, &v.tenant, &v.turn, &v.status, &v.attempt); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, v)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	for _, item := range items {
		reason := "QUEUE_TIMEOUT"
		if item.status == "PREPARING" {
			reason = "SUPERVISOR_STARTUP_TIMEOUT"
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='TIMED_OUT',result_summary=$2,finished_at=CURRENT_TIMESTAMP,version=version+1 WHERE id=$1 AND status=$3`, item.task, reason, item.status); err != nil {
			return 0, err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='TIMED_OUT',updated_at=CURRENT_TIMESTAMP WHERE task_id=$1 AND status NOT IN ('SUCCEEDED','FAILED','CANCELLED','TIMED_OUT')`, item.task); err != nil {
			return 0, err
		}
		if item.attempt != 0 {
			_, _ = tx.Exec(ctx, `UPDATE workforce_task_attempts SET status='TIMED_OUT',timed_out_at=CURRENT_TIMESTAMP,timeout_stage=$2,finished_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='PREPARING'`, item.attempt, reason)
			_, _ = tx.Exec(ctx, `UPDATE workforce_attempt_runtimes SET status='TIMED_OUT',terminal_reason_code=$2,updated_at=CURRENT_TIMESTAMP WHERE attempt_id=$1 AND status IN ('AUTHORIZED','CONTAINER_CREATED','SUPERVISOR_READY','STARTING')`, item.attempt, reason)
		}
		_, _ = tx.Exec(ctx, `UPDATE workforce_outbox_events SET status='SUPERSEDED',last_error=$2,updated_at=CURRENT_TIMESTAMP WHERE aggregate_id IN (SELECT id FROM workforce_task_assignments WHERE task_id=$1 AND status IN ('RESERVED','PUBLISHED','CLAIMED')) AND status='PENDING'`, item.task, reason)
		_, _ = tx.Exec(ctx, `UPDATE workforce_task_assignments SET status='COMPLETED',completed_at=CURRENT_TIMESTAMP,release_reason_code=$2 WHERE task_id=$1 AND status IN ('RESERVED','PUBLISHED','CLAIMED')`, item.task, reason)
		var occurrence int64
		if occurrence, err = eventlog.AppendWithSequence(ctx, tx, item.task, item.attempt, "task.timed_out", map[string]any{"reasonCode": reason}); err != nil {
			return 0, err
		}
		if s.notificationRecorder != nil && item.turn > 0 {
			if _, err = s.notificationRecorder.RecordTransition(ctx, tx, tasknotification.TaskNotificationTransition{TenantID: item.tenant, TaskID: item.task, TaskTurnID: item.turn, SourceType: tasknotification.SourceTaskTimeout, SourceID: item.task, SourceOccurrence: occurrence, PreviousStatus: item.status, CurrentStatus: "TIMED_OUT", ReasonCode: reason, Recoverability: tasknotification.RecoverabilityTerminal, OccurredAt: now}); err != nil {
				return 0, err
			}
		}
	}
	return len(items), tx.Commit(ctx)
}

func (s *Store) DispatchOne(ctx context.Context, excludedTenants []int64) (int64, bool, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback(ctx)
	var nodeID, taskID, taskTurnID, tenantID int64
	var requirement string
	var requirementsJSON, baseSnapshotJSON json.RawMessage
	var generation int64
	tenantRows, err := tx.Query(ctx, `SELECT DISTINCT t.tenant_id
FROM workforce_tasks t
JOIN workforce_execution_nodes n ON n.task_id=t.id
WHERE t.status='QUEUED' AND n.status='QUEUED'
  AND NOT (t.tenant_id=ANY($1))
ORDER BY t.tenant_id`, excludedTenants)
	if err != nil {
		return 0, false, err
	}
	var tenantIDs []int64
	for tenantRows.Next() {
		var id int64
		if err := tenantRows.Scan(&id); err != nil {
			tenantRows.Close()
			return 0, false, err
		}
		tenantIDs = append(tenantIDs, id)
	}
	if err := tenantRows.Err(); err != nil {
		tenantRows.Close()
		return 0, false, err
	}
	tenantRows.Close()
	if len(tenantIDs) == 0 {
		return 0, false, nil
	}
	lastAssigned := make(map[int64]time.Time, len(tenantIDs))
	historyRows, err := tx.Query(ctx, `SELECT t.tenant_id,MAX(a.completed_at)
FROM workforce_tasks t
JOIN workforce_task_assignments a ON a.task_id=t.id
WHERE t.tenant_id=ANY($1) AND a.status='COMPLETED'
GROUP BY t.tenant_id`, tenantIDs)
	if err != nil {
		return 0, false, err
	}
	for historyRows.Next() {
		var id int64
		var completed time.Time
		if err := historyRows.Scan(&id, &completed); err != nil {
			historyRows.Close()
			return 0, false, err
		}
		lastAssigned[id] = completed
	}
	if err := historyRows.Err(); err != nil {
		historyRows.Close()
		return 0, false, err
	}
	historyRows.Close()
	sort.SliceStable(tenantIDs, func(i, j int) bool {
		left, leftSeen := lastAssigned[tenantIDs[i]]
		right, rightSeen := lastAssigned[tenantIDs[j]]
		if leftSeen != rightSeen {
			return !leftSeen
		}
		if !left.Equal(right) {
			return left.Before(right)
		}
		return tenantIDs[i] < tenantIDs[j]
	})
	tenantID = tenantIDs[0]
	// Preserve queue ordering while reading each frozen release by its primary key.
	skippedNodes := make([]int64, 0)
	dispatchAt := s.now().UTC()
	for {
		err = tx.QueryRow(ctx, `
SELECT n.id,n.task_id,n.task_turn_id,n.generation,n.execution_requirement_digest,t.execution_requirements_json,
       jsonb_build_object(
         'resultProcessorReleaseId',tr.processor_release_id::text,
         'resultProcessorDigest',tr.processor_digest,
         'environmentConfigDigest',tr.environment_config_digest,
         'resultProcessorEnvironment',tr.environment_snapshot_json,
         'secretVersionIdentities',tr.secret_version_identities,
         'resultProcessorParameters',tr.parameters_json,
         'executionDeadlineSeconds',COALESCE(t.execution_timeout_seconds,0),
		 'isolationProfile','seven.worker-image/v1') ||
		 jsonb_strip_nulls(jsonb_build_object('stagePurpose',NULLIF(t.execution_requirements_json->>'stagePurpose',''),'approvedPlan',t.work_item_input_json))
FROM workforce_execution_nodes n
JOIN workforce_tasks t ON t.id=n.task_id
JOIN workforce_task_turns tr ON tr.id=n.task_turn_id AND tr.task_id=t.id
WHERE n.status='QUEUED' AND t.status='QUEUED' AND t.tenant_id=$1 AND NOT (n.id=ANY($3::BIGINT[]))
ORDER BY
  LEAST(100,t.priority+FLOOR(EXTRACT(EPOCH FROM ($2::timestamptz-t.created_at))/300)::integer) DESC,
	  t.created_at,t.id
FOR UPDATE OF n,t SKIP LOCKED LIMIT 1`, tenantID, dispatchAt, skippedNodes).Scan(&nodeID, &taskID, &taskTurnID, &generation, &requirement, &requirementsJSON, &baseSnapshotJSON)
		if errors.Is(err, pgx.ErrNoRows) {
			return tenantID, false, nil
		}
		if err != nil {
			return tenantID, false, err
		}
		var policySnapshot json.RawMessage
		err = tx.QueryRow(ctx, `SELECT $2::JSONB || jsonb_build_object(
			'maxEvaluationCycles',COALESCE((pr.manifest_json->>'maxEvaluationCycles')::integer,3),
			'continuationPolicy',COALESCE(pr.manifest_json->>'continuationPolicy','CHECKPOINT_REQUIRED'),
			'deliveryPolicy',COALESCE(pr.manifest_json->>'deliveryPolicy','OPTIONAL'))
			FROM workforce_result_processor_releases pr
			WHERE pr.id=($2::JSONB->>'resultProcessorReleaseId')::BIGINT
			AND pr.archive_sha256=$2::JSONB->>'resultProcessorDigest'
			AND (pr.scope_type='SYSTEM' OR (pr.scope_type='TENANT' AND pr.tenant_id=$1))`, tenantID, baseSnapshotJSON).Scan(&policySnapshot)
		if errors.Is(err, pgx.ErrNoRows) {
			skippedNodes = append(skippedNodes, nodeID)
			continue
		}
		if err != nil {
			return tenantID, false, err
		}
		baseSnapshotJSON = policySnapshot
		break
	}
	if s.routing == nil {
		return tenantID, false, errors.New("execution assignment resolver is unavailable")
	}
	var requirements projectfacade.ExecutionRequirements
	var baseSnapshot taskdomain.ExecutionSnapshot
	if json.Unmarshal(requirementsJSON, &requirements) != nil || json.Unmarshal(baseSnapshotJSON, &baseSnapshot) != nil {
		_, _ = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET wait_reason_code=$2,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, nodeID, routingfacade.WaitProjectInvalid)
		return tenantID, false, tx.Commit(ctx)
	}
	resolved, waitReason, err := s.routing.ResolveAssignment(ctx, tx, routingfacade.NodeIdentity{TenantID: tenantID, TaskID: taskID, TaskTurnID: taskTurnID, NodeID: nodeID, Generation: generation}, requirements, baseSnapshot)
	if err != nil {
		return tenantID, false, err
	}
	if waitReason != "" {
		_, _ = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET wait_reason_code=$2,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, nodeID, waitReason)
		return tenantID, false, tx.Commit(ctx)
	}
	var selectedRuntimeReleaseID int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(selected_runtime_release_id,0) FROM workforce_tasks WHERE tenant_id=$1 AND id=$2`, tenantID, taskID).Scan(&selectedRuntimeReleaseID); err != nil {
		return tenantID, false, err
	}
	if selectedRuntimeReleaseID < 1 || resolved.RuntimeReleaseID != selectedRuntimeReleaseID || resolved.RuntimeImageDigest == "" {
		return tenantID, false, errors.New("TASK_RUNTIME_SELECTION_MISMATCH")
	}
	workerID, instanceID := resolved.WorkerID, resolved.WorkerInstanceID
	assignmentID, messageID := s.idGen.NextID(), s.idGen.NextID()
	lease := randomToken()
	leaseSum := sha256.Sum256([]byte(lease))
	expires := s.now().UTC().Add(30 * time.Second)
	message := domain.AssignmentMessage{SchemaVersion: 1, Kind: domain.AssignmentSchema, MessageID: messageID, AssignmentID: assignmentID, TaskID: taskID, ExecutionNodeID: nodeID, WorkerID: workerID, Generation: generation, ExecutionRequirementDigest: requirement, ExpiresAt: expires.Format(time.RFC3339Nano)}
	payload, _ := json.Marshal(message)
	_, err = tx.Exec(ctx, `INSERT INTO workforce_task_assignments
  (id,task_id,execution_node_id,worker_id,worker_instance_id,generation,execution_requirement_digest,
   agent_runtime_release_id,runtime_image_ref,runtime_image_digest,adapter_identity,adapter_version,adapter_protocol,adapter_capabilities_json,
   hook_configuration_digest,model_credential_id,credential_version,model_protocol,primary_model,
   provider_proxy_identity,worker_image_contract,isolation_profile,resolved_execution_snapshot_json,selection_digest,
   lease_token_sha256,lease_expires_at,status,message_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,'RESERVED',$27)`,
		assignmentID, taskID, nodeID, workerID, instanceID, generation, requirement,
		resolved.RuntimeReleaseID, resolved.RuntimeImageRef, resolved.RuntimeImageDigest, resolved.AdapterID, resolved.AdapterVersion, resolved.AdapterProtocol, mustJSON(resolved.AdapterCapabilities),
		resolved.HookConfigurationDigest, resolved.ModelCredentialID, resolved.CredentialVersion, resolved.ModelProtocol, resolved.PrimaryModel,
		resolved.Snapshot.ProviderGatewayIdentity, resolved.WorkerImageContract, resolved.IsolationProfile, resolved.SnapshotJSON, resolved.SelectionDigest,
		hex.EncodeToString(leaseSum[:]), expires, messageID)
	if err != nil {
		return tenantID, false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO workforce_outbox_events (id,aggregate_type,aggregate_id,event_type,payload_json,status,exchange_name,routing_key,message_expiration_ms) VALUES ($1,'assignment',$2,'assignment.offered',$3,'PENDING','seven.workforce.tasks',$4,30000)`, s.idGen.NextID(), assignmentID, payload, fmt.Sprintf("worker.%d", workerID))
	if err != nil {
		return tenantID, false, err
	}
	_, err = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='ASSIGNED',wait_reason_code='',updated_at=CURRENT_TIMESTAMP WHERE id=$1`, nodeID)
	if err != nil {
		return tenantID, false, err
	}
	return tenantID, true, tx.Commit(ctx)
}

func (s *Store) ReconcileExpired(ctx context.Context, limit int) (int, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT id,execution_node_id FROM workforce_task_assignments WHERE status IN ('RESERVED','PUBLISHED') AND lease_expires_at<CURRENT_TIMESTAMP ORDER BY lease_expires_at FOR UPDATE SKIP LOCKED LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	type pair struct {
		id, node int64
	}
	var items []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.id, &p.node); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, p)
	}
	rows.Close()
	for _, p := range items {
		_, _ = tx.Exec(ctx, `UPDATE workforce_task_assignments SET status='EXPIRED',completed_at=CURRENT_TIMESTAMP WHERE id=$1`, p.id)
		_, _ = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='QUEUED',wait_reason_code='ASSIGNMENT_RETRY_PENDING',updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='ASSIGNED'`, p.node)
	}
	return len(items), tx.Commit(ctx)
}

func (s *Store) Claim(ctx context.Context, e domain.MachineEnvelope, assignmentID int64, requirement string) (domain.ExecutionContract, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.ExecutionContract{}, err
	}
	defer tx.Rollback(ctx)
	var taskID, nodeID, workerID int64
	var instanceID, status, storedRequirement string
	var assignmentSelectionDigest string
	var resolvedSnapshotJSON json.RawMessage
	var generation int64
	err = tx.QueryRow(ctx, `SELECT task_id,execution_node_id,worker_id,worker_instance_id,generation,execution_requirement_digest,status,selection_digest,resolved_execution_snapshot_json FROM workforce_task_assignments WHERE id=$1 FOR UPDATE`, assignmentID).Scan(&taskID, &nodeID, &workerID, &instanceID, &generation, &storedRequirement, &status, &assignmentSelectionDigest, &resolvedSnapshotJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ExecutionContract{}, application.ErrNotFound
	}
	if err != nil {
		return domain.ExecutionContract{}, err
	}
	if workerID != e.WorkerID || generation != e.Generation || storedRequirement != requirement {
		return domain.ExecutionContract{}, application.ErrIdentityConflict
	}
	var current, boot, desiredState string
	if err = tx.QueryRow(ctx, `SELECT w.current_instance_id,s.host_boot_id,w.desired_state FROM workforce_workers w JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id WHERE w.id=$1`, workerID).Scan(&current, &boot, &desiredState); err != nil {
		return domain.ExecutionContract{}, err
	}
	if current != e.InstanceID || boot != e.HostBootID {
		return domain.ExecutionContract{}, application.ErrStaleInstance
	}
	if desiredState != "ENABLED" && (status == "RESERVED" || status == "PUBLISHED") {
		_, _ = tx.Exec(ctx, `UPDATE workforce_task_assignments SET status='RELEASED',release_reason_code='WORKER_DRAINING',released_at=CURRENT_TIMESTAMP,completed_at=CURRENT_TIMESTAMP WHERE id=$1`, assignmentID)
		_, _ = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='QUEUED',wait_reason_code='WORKER_DRAINING',updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='ASSIGNED'`, nodeID)
		contract := domain.ExecutionContract{Directive: domain.DirectiveRejectStaleAssignment, AssignmentID: assignmentID, ExecutionNodeID: nodeID, ExecutionRequirementDigest: storedRequirement}
		return contract, tx.Commit(ctx)
	}
	if instanceID != e.InstanceID {
		var runtimeStatus string
		runtimeErr := tx.QueryRow(ctx, `SELECT status FROM workforce_attempt_runtimes WHERE assignment_id=$1`, assignmentID).Scan(&runtimeStatus)
		if runtimeErr == nil && (runtimeStatus == "AUTHORIZED" || runtimeStatus == "CONTAINER_CREATED" || runtimeStatus == "SUPERVISOR_READY" || runtimeStatus == "STARTING" || runtimeStatus == "RUNNING" || runtimeStatus == "RECONCILIATION_REQUIRED") {
			return domain.ExecutionContract{Directive: domain.DirectiveReconcileLiveRuntime, AssignmentID: assignmentID, ExecutionNodeID: nodeID, ExecutionRequirementDigest: storedRequirement}, tx.Commit(ctx)
		}
		return domain.ExecutionContract{Directive: domain.DirectiveRejectStaleAssignment, AssignmentID: assignmentID, ExecutionNodeID: nodeID, ExecutionRequirementDigest: storedRequirement}, tx.Commit(ctx)
	}
	if status == "CLAIMED" {
		var runtimeStatus, runtimeInstance string
		owner := int64(1)
		runtimeErr := tx.QueryRow(ctx, `SELECT status,worker_instance_id,owner_epoch FROM workforce_attempt_runtimes WHERE assignment_id=$1`, assignmentID).Scan(&runtimeStatus, &runtimeInstance, &owner)
		directive := domain.DirectiveResumeAuthorizedRuntime
		if runtimeErr == nil {
			switch {
			case runtimeStatus == "RUNNING":
				directive = domain.DirectiveAckAlreadyRunning
			case runtimeInstance != e.InstanceID:
				directive = domain.DirectiveReconcileLiveRuntime
			}
		} else if !errors.Is(runtimeErr, pgx.ErrNoRows) {
			return domain.ExecutionContract{}, runtimeErr
		}
		if directive == domain.DirectiveAckAlreadyRunning || directive == domain.DirectiveReconcileLiveRuntime {
			return domain.ExecutionContract{Directive: directive, AssignmentID: assignmentID, ExecutionNodeID: nodeID, ExecutionRequirementDigest: storedRequirement, OwnerEpoch: owner}, tx.Commit(ctx)
		}
		lease := randomToken()
		expires := time.Now().UTC().Add(30 * time.Second)
		_, err = tx.Exec(ctx, `UPDATE workforce_task_attempts SET lease_token=$2,lease_expires_at=$3 WHERE assignment_id=$1`, assignmentID, hashToken(lease), expires)
		if err != nil {
			return domain.ExecutionContract{}, err
		}
		if runtimeErr == nil {
			_, err = tx.Exec(ctx, `UPDATE workforce_attempt_runtimes SET execution_lease_sha256=$2,execution_lease_expires_at=$3,execution_lease_renewed_at=CURRENT_TIMESTAMP WHERE assignment_id=$1`, assignmentID, hashToken(lease), expires)
			if err != nil {
				return domain.ExecutionContract{}, err
			}
		}
		contract, err := s.loadContract(ctx, tx, assignmentID, taskID, nodeID, storedRequirement, directive, lease, expires, owner)
		if err != nil {
			return domain.ExecutionContract{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return domain.ExecutionContract{}, err
		}
		return contract, nil
	}
	if status != "RESERVED" && status != "PUBLISHED" {
		if status == "COMPLETED" || status == "CANCELLED" {
			return domain.ExecutionContract{Directive: domain.DirectiveAckTerminal, AssignmentID: assignmentID, ExecutionNodeID: nodeID, ExecutionRequirementDigest: storedRequirement}, nil
		}
		return domain.ExecutionContract{Directive: domain.DirectiveRejectStaleAssignment, AssignmentID: assignmentID, ExecutionNodeID: nodeID, ExecutionRequirementDigest: storedRequirement}, nil
	}
	attemptID, lease := s.idGen.NextID(), randomToken()
	leaseExpires := time.Now().UTC().Add(30 * time.Second)
	workspacePath := fmt.Sprintf("/workspace/%d/attempts/%d", taskID, attemptID)
	_, err = tx.Exec(ctx, `WITH current_node AS (
  SELECT task_turn_id FROM workforce_execution_nodes WHERE id=$8
)
INSERT INTO workforce_task_attempts (id,task_id,task_turn_id,attempt_no,generation,worker_id,worker_instance_id,host_boot_id,assignment_id,execution_node_id,workspace_path,status,lease_token,lease_expires_at,runtime_driver,retry_of_attempt_id,assignment_selection_digest,resolved_execution_snapshot_json)
SELECT $1,$2,current_node.task_turn_id,
       COALESCE((SELECT MAX(attempt_no) FROM workforce_task_attempts WHERE task_id=$2),0)+1,
       $3,$4,$5,$6,$7,$8,$9,'PREPARING',$10,$11,'docker',
       (SELECT id FROM workforce_task_attempts
        WHERE task_id=$2 AND task_turn_id=current_node.task_turn_id
        ORDER BY attempt_no DESC LIMIT 1),$12,$13
FROM current_node`, attemptID, taskID, generation, workerID, e.InstanceID, e.HostBootID, assignmentID, nodeID, workspacePath, hashToken(lease), leaseExpires, assignmentSelectionDigest, resolvedSnapshotJSON)
	if err != nil {
		return domain.ExecutionContract{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE workforce_task_assignments SET status='CLAIMED',claimed_at=CURRENT_TIMESTAMP WHERE id=$1`, assignmentID)
	if err != nil {
		return domain.ExecutionContract{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='PREPARING',updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='ASSIGNED'`, nodeID)
	if err != nil {
		return domain.ExecutionContract{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='PREPARING',attempt_count=attempt_count+1,latest_attempt_id=$2,started_at=COALESCE(started_at,CURRENT_TIMESTAMP),version=version+1 WHERE id=$1 AND status='QUEUED'`, taskID, attemptID)
	if err != nil {
		return domain.ExecutionContract{}, err
	}
	if err = appendEvent(ctx, tx, taskID, attemptID, "attempt.preparing", map[string]any{"workerId": workerID, "instanceId": e.InstanceID, "assignmentId": assignmentID, "executionNodeId": nodeID}); err != nil {
		return domain.ExecutionContract{}, err
	}
	contract, err := s.loadContract(ctx, tx, assignmentID, taskID, nodeID, storedRequirement, domain.DirectiveStartNewRuntime, lease, leaseExpires, 1)
	if err != nil {
		return domain.ExecutionContract{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.ExecutionContract{}, err
	}
	return contract, nil
}

func (s *Store) ReleaseAssignment(ctx context.Context, e domain.MachineEnvelope, assignmentID int64, reason string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var nodeID, workerID int64
	var instanceID, status string
	var generation int64
	if err = tx.QueryRow(ctx, `SELECT execution_node_id,worker_id,worker_instance_id,status,generation FROM workforce_task_assignments WHERE id=$1 FOR UPDATE`, assignmentID).Scan(&nodeID, &workerID, &instanceID, &status, &generation); errors.Is(err, pgx.ErrNoRows) {
		return application.ErrNotFound
	} else if err != nil {
		return err
	}
	if workerID != e.WorkerID || instanceID != e.InstanceID || generation != e.Generation {
		return application.ErrIdentityConflict
	}
	if status == "RELEASED" || status == "EXPIRED" {
		return tx.Commit(ctx)
	}
	if status != "RESERVED" && status != "PUBLISHED" {
		return application.ErrNotClaimable
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_task_assignments SET status='RELEASED',released_at=CURRENT_TIMESTAMP,release_reason_code=$2,delivery_disposition='ACKED',completed_at=CURRENT_TIMESTAMP WHERE id=$1`, assignmentID, reason); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='QUEUED',wait_reason_code='ASSIGNMENT_RETRY_PENDING',updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='ASSIGNED'`, nodeID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) loadContract(ctx context.Context, q pgx.Tx, assignmentID int64, taskID int64, nodeID int64, requirement string, directive domain.DeliveryDirective, lease string, leaseExpires time.Time, ownerEpoch int64) (domain.ExecutionContract, error) {
	var item taskdomain.WorkItem
	var attempt taskdomain.Attempt
	var snapshotJSON, processorParameters []byte
	var modelCredentialID int64
	err := q.QueryRow(ctx, `SELECT t.id,t.tenant_id,t.project_version_id,'PROJECT',assignment.agent_runtime_release_id,assignment.runtime_image_ref,assignment.runtime_image_digest,assignment.resolved_execution_snapshot_json,assignment.model_credential_id,assignment.primary_model,COALESCE((assignment.resolved_execution_snapshot_json->>'executionDeadlineSeconds')::integer,0),t.execution_active_milliseconds,attempt.id,attempt.generation,attempt.workspace_path,attempt.task_turn_id
FROM workforce_tasks t
JOIN workforce_task_assignments assignment ON assignment.id=$1 AND assignment.task_id=t.id
JOIN workforce_task_attempts attempt ON attempt.task_id=t.id AND attempt.assignment_id=assignment.id
WHERE t.id=$2 AND attempt.assignment_selection_digest=assignment.selection_digest AND attempt.resolved_execution_snapshot_json=assignment.resolved_execution_snapshot_json`, assignmentID, taskID).Scan(&item.TaskID, &item.TenantID, &item.ProjectVersionID, &item.ResultProcessorSelectionMode, &item.HarnessReleaseID, &item.HarnessImage, &item.HarnessImageDigest, &snapshotJSON, &modelCredentialID, &item.PrimaryModel, &item.HarnessTimeoutSeconds, &item.ExecutionActiveMilliseconds, &attempt.ID, &attempt.Generation, &attempt.WorkspacePath, &item.TaskTurnID)
	if err != nil {
		return domain.ExecutionContract{}, err
	}
	if err = q.QueryRow(ctx, `SELECT alias FROM workforce_harness_releases WHERE id=$1 AND image_digest=$2`, item.HarnessReleaseID, item.HarnessImageDigest).Scan(&item.HarnessAlias); err != nil {
		return domain.ExecutionContract{}, err
	}
	if err = q.QueryRow(ctx, `SELECT name FROM workforce_model_credentials WHERE id=$1 AND tenant_id=$2`, modelCredentialID, item.TenantID).Scan(&item.ModelCredentialName); err != nil {
		return domain.ExecutionContract{}, err
	}
	if err = q.QueryRow(ctx, `SELECT instruction,COALESCE(parameters_json,'{}'::jsonb),execution_snapshot_digest,COALESCE(source_execution_operation_id,0) FROM workforce_task_turns WHERE id=$1 AND task_id=$2`, item.TaskTurnID, item.TaskID).Scan(&item.Instruction, &processorParameters, &item.TaskExecutionSnapshotDigest, &item.SourceExecutionOperationID); err != nil {
		return domain.ExecutionContract{}, err
	}
	var revisionKind, reportDigest string
	var sourceReviewRunID int64
	if err = q.QueryRow(ctx, `SELECT COALESCE(review_revision_kind,''),COALESCE(source_review_run_id,0),COALESCE(review_report_digest,'') FROM workforce_task_turns WHERE id=$1`, item.TaskTurnID).Scan(&revisionKind, &sourceReviewRunID, &reportDigest); err != nil {
		return domain.ExecutionContract{}, err
	}
	if sourceReviewRunID > 0 {
		correction := taskdomain.ReviewCorrection{SourceReviewRunID: sourceReviewRunID, RevisionKind: revisionKind, ReportDigest: reportDigest}
		if reportDigest != "" {
			if err = q.QueryRow(ctx, `SELECT report_verdict,report_schema_version,canonical_report_json FROM workforce_review_runs WHERE id=$1 AND tenant_id=$2 AND canonical_report_digest=$3`, sourceReviewRunID, item.TenantID, reportDigest).Scan(&correction.Verdict, &correction.ReportSchema, &correction.Report); err != nil {
				return domain.ExecutionContract{}, errors.New("INDEPENDENT_REVIEW_CORRECTION_IDENTITY_INVALID")
			}
		} else if err = q.QueryRow(ctx, `SELECT closure_reason_code FROM workforce_review_runs WHERE id=$1 AND tenant_id=$2`, sourceReviewRunID, item.TenantID).Scan(&correction.ReasonCode); err != nil {
			return domain.ExecutionContract{}, errors.New("INDEPENDENT_REVIEW_CORRECTION_IDENTITY_INVALID")
		}
		item.ReviewCorrection = &correction
	}
	attempt.TaskID = taskID
	attempt.ExecutionNodeID = nodeID
	attempt.AssignmentID = assignmentID
	item.ResultProcessorParameters = append(json.RawMessage(nil), processorParameters...)
	input := taskdomain.TaskInputContract{}
	if err = q.QueryRow(ctx, `SELECT source_kind,COALESCE(workspace_source_revision_id,0),source_commit,source_tree,source_identity_digest,repository_bundle_object_key,repository_bundle_content_sha256,repository_bundle_object_sha256,repository_bundle_size_bytes,input_snapshot_digest FROM workforce_task_input_snapshots WHERE task_id=$1 AND task_turn_id=$2`, item.TaskID, item.TaskTurnID).Scan(&input.SourceKind, &input.SourceRevisionID, &input.SourceCommit, &input.SourceTree, &input.SourceIdentityDigest, &input.RepositoryBundleObjectKey, &input.RepositoryBundleContentSHA256, &input.RepositoryBundleObjectSHA256, &input.RepositoryBundleSizeBytes, &input.SnapshotDigest); err != nil {
		return domain.ExecutionContract{}, fmt.Errorf("TASK_INPUT_SNAPSHOT_MISSING: %w", err)
	}
	rows, rowsErr := q.Query(ctx, `SELECT a.id,a.display_name,a.content_type,a.content_sha256,a.object_key,a.object_sha256,a.size_bytes FROM workforce_task_input_references r JOIN workforce_reference_assets a ON a.id=r.reference_asset_id WHERE r.task_input_snapshot_id=(SELECT id FROM workforce_task_input_snapshots WHERE task_id=$1 AND task_turn_id=$2) ORDER BY r.display_order`, item.TaskID, item.TaskTurnID)
	if rowsErr != nil {
		return domain.ExecutionContract{}, rowsErr
	}
	for rows.Next() {
		var ref taskdomain.TaskInputReference
		if scanErr := rows.Scan(&ref.ID, &ref.DisplayName, &ref.ContentType, &ref.ContentSHA256, &ref.ObjectKey, &ref.ObjectSHA256, &ref.SizeBytes); scanErr != nil {
			rows.Close()
			return domain.ExecutionContract{}, scanErr
		}
		input.References = append(input.References, ref)
	}
	rows.Close()
	if rows.Err() != nil {
		return domain.ExecutionContract{}, rows.Err()
	}
	item.TaskInput = &input
	item.ResolvedBaseCommit = input.SourceCommit
	// Network access is a platform-owned operation available to admitted tool
	// adapters. Git delivery is exposed only when this exact Turn has a frozen,
	// active delivery plan; the Agent never supplies the target or mode.
	item.AllowedAdapterTools = []string{"run_command_with_network_access"}
	var deliveryMode string
	if planErr := q.QueryRow(ctx, `SELECT delivery_mode FROM workforce_task_git_plans WHERE task_id=$1 AND task_turn_id=$2 AND tenant_id=$3 AND status='ACTIVE'`, item.TaskID, item.TaskTurnID, item.TenantID).Scan(&deliveryMode); planErr == nil {
		if deliveryMode == "NEW_BRANCH" || deliveryMode == "NEW_BRANCH_AND_DRAFT_REVIEW" {
			item.AllowedAdapterTools = append(item.AllowedAdapterTools, "request_git_delivery")
		}
	} else if !errors.Is(planErr, pgx.ErrNoRows) {
		return domain.ExecutionContract{}, planErr
	}
	if err = json.Unmarshal(snapshotJSON, &item.ExecutionSnapshot); err != nil {
		return domain.ExecutionContract{}, err
	}
	if item.ExecutionSnapshot.StagePurpose == "PLAN" {
		item.AllowedAdapterTools = []string{}
	}
	var completionJSON, completionRefs []byte
	if err = q.QueryRow(ctx, `SELECT COALESCE(completion_check_json,'null'::jsonb),COALESCE(completion_check_digest,''),COALESCE(completion_check_input_digest,''),COALESCE(completion_check_references_json,'[]'::jsonb) FROM workforce_task_turns WHERE id=$1 AND tenant_id=$2 AND task_id=$3`, item.TaskTurnID, item.TenantID, item.TaskID).Scan(&completionJSON, &item.CompletionCheckDigest, &item.CompletionCheckInputDigest, &completionRefs); err != nil {
		return domain.ExecutionContract{}, err
	}
	if err = json.Unmarshal(completionJSON, &item.CompletionCheck); err != nil {
		return domain.ExecutionContract{}, err
	}
	if err = json.Unmarshal(completionRefs, &item.CompletionCheckReferences); err != nil {
		return domain.ExecutionContract{}, err
	}
	if item.ExecutionSnapshot.ResultProcessorReleaseID == 0 || item.ExecutionSnapshot.ResultProcessorDigest == "" {
		return domain.ExecutionContract{}, errors.New("RESULT_PROCESSOR_EXECUTION_IDENTITY_MISSING")
	}
	if item.TaskExecutionSnapshotDigest == "" {
		return domain.ExecutionContract{}, errors.New("TASK_EXECUTION_SNAPSHOT_IDENTITY_MISSING")
	}
	if err = q.QueryRow(ctx, `SELECT archive_object_key FROM workforce_result_processor_releases WHERE id=$1 AND archive_sha256=$2 AND ((scope_type='SYSTEM' AND tenant_id IS NULL) OR (scope_type='TENANT' AND tenant_id=$3))`, item.ExecutionSnapshot.ResultProcessorReleaseID, item.ExecutionSnapshot.ResultProcessorDigest, item.TenantID).Scan(&item.ResultProcessorArchiveObjectKey); err != nil {
		return domain.ExecutionContract{}, fmt.Errorf("resolve Result Processor release archive: %w", err)
	}
	for i := range item.ExecutionSnapshot.PackageReleases {
		_ = q.QueryRow(ctx, `SELECT archive_object_key FROM workforce_hook_package_releases WHERE id=$1`, item.ExecutionSnapshot.PackageReleases[i].ID).Scan(&item.ExecutionSnapshot.PackageReleases[i].ArchiveObjectKey)
	}
	var sourceCheckpointID int64
	if err = q.QueryRow(ctx, `SELECT COALESCE(source_checkpoint_id,0) FROM workforce_task_turns WHERE id=$1`, item.TaskTurnID).Scan(&sourceCheckpointID); err != nil {
		return domain.ExecutionContract{}, err
	}
	if sourceCheckpointID != 0 {
		restore := taskdomain.ContinuationRestore{CheckpointID: sourceCheckpointID, TargetTurnID: item.TaskTurnID}
		if err = q.QueryRow(ctx, `SELECT c.task_turn_id,c.checkpoint_identity_sha256,c.format_version,
			c.repository_bundle_object_key,c.repository_bundle_content_sha256,c.repository_bundle_object_sha256,c.repository_bundle_size_bytes,
			c.workspace_overlay_object_key,c.workspace_overlay_content_sha256,c.workspace_overlay_object_sha256,c.workspace_overlay_size_bytes,c.selection_digest,
			c.session_object_key,COALESCE(NULLIF(c.session_content_sha256,''),c.session_sha256),COALESCE(NULLIF(c.session_object_sha256,''),c.session_sha256),c.session_size_bytes,
			c.identity_manifest_object_key,COALESCE(NULLIF(c.manifest_content_sha256,''),c.manifest_sha256),COALESCE(NULLIF(c.manifest_object_sha256,''),c.manifest_sha256),c.manifest_size_bytes,
			c.task_execution_snapshot_digest,c.base_commit,c.result_commit,c.result_tree,c.session_lineage_digest,c.encryption_algorithm,c.encryption_key_id
			FROM workforce_continuation_checkpoints c WHERE c.id=$1 AND c.status='AVAILABLE' AND c.format_version=2 AND c.consumed_by_turn_id=$2 AND c.expires_at>CURRENT_TIMESTAMP`, sourceCheckpointID, item.TaskTurnID).Scan(
			&restore.SourceTurnID, &restore.CheckpointIdentity, &restore.FormatVersion,
			&restore.RepositoryBundleObjectKey, &restore.RepositoryBundleContentSHA256, &restore.RepositoryBundleObjectSHA256, &restore.RepositoryBundleSizeBytes,
			&restore.WorkspaceOverlayObjectKey, &restore.WorkspaceOverlayContentSHA256, &restore.WorkspaceOverlayObjectSHA256, &restore.WorkspaceOverlaySizeBytes, &restore.SelectionDigest,
			&restore.SessionObjectKey, &restore.SessionContentSHA256, &restore.SessionObjectSHA256, &restore.SessionSizeBytes,
			&restore.ManifestObjectKey, &restore.ManifestContentSHA256, &restore.ManifestObjectSHA256, &restore.ManifestSizeBytes,
			&restore.TaskExecutionSnapshotDigest, &restore.ExpectedBaseCommit, &restore.ExpectedResultCommit, &restore.ExpectedResultTree, &restore.SessionLineageDigest, &restore.EncryptionAlgorithm, &restore.EncryptionKeyID); err != nil {
			return domain.ExecutionContract{}, fmt.Errorf("CONTINUATION_CHECKPOINT_MISSING: %w", err)
		}
		if restore.TaskExecutionSnapshotDigest != requirement {
			return domain.ExecutionContract{}, errors.New("CONTINUATION_IDENTITY_INCOMPATIBLE")
		}
		resumeIdentity := sha256.Sum256([]byte(fmt.Sprintf("seven.continuation.resume\x00%d\x00%d\x00%s", item.TaskTurnID, sourceCheckpointID, restore.SessionLineageDigest)))
		restore.ResumeNonce = hex.EncodeToString(resumeIdentity[:])
		resumeDigest := sha256.Sum256([]byte(restore.ResumeNonce))
		restore.ResumeNonceDigest = hex.EncodeToString(resumeDigest[:])
		if _, err = q.Exec(ctx, `UPDATE workforce_task_turns SET resume_nonce_sha256=COALESCE(NULLIF(resume_nonce_sha256,''),$2) WHERE id=$1 AND (resume_nonce_sha256='' OR resume_nonce_sha256=$2)`, item.TaskTurnID, restore.ResumeNonceDigest); err != nil {
			return domain.ExecutionContract{}, err
		}
		if err = q.QueryRow(ctx, `SELECT id FROM workforce_continuation_restore_runs WHERE target_turn_id=$1 AND attempt_id=$2 AND status IN ('PENDING','RUNNING','RESTORED')`, item.TaskTurnID, attempt.ID).Scan(&restore.RestoreRunID); errors.Is(err, pgx.ErrNoRows) {
			restore.RestoreRunID = s.idGen.NextID()
			var worker int64
			var instance, boot string
			if err = q.QueryRow(ctx, `SELECT worker_id,worker_instance_id,host_boot_id FROM workforce_task_attempts WHERE id=$1`, attempt.ID).Scan(&worker, &instance, &boot); err != nil {
				return domain.ExecutionContract{}, err
			}
			if _, err = q.Exec(ctx, `INSERT INTO workforce_continuation_restore_runs (id,task_id,source_turn_id,target_turn_id,execution_node_id,attempt_id,checkpoint_id,worker_id,worker_instance_id,host_boot_id,generation,owner_epoch,expected_identity_digest,status) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'PENDING')`, restore.RestoreRunID, item.TaskID, restore.SourceTurnID, item.TaskTurnID, nodeID, attempt.ID, sourceCheckpointID, worker, instance, boot, attempt.Generation, ownerEpoch, requirement); err != nil {
				return domain.ExecutionContract{}, err
			}
		} else if err != nil {
			return domain.ExecutionContract{}, err
		}
		item.ContinuationRestore = &restore
	}
	return domain.ExecutionContract{Directive: directive, AssignmentID: assignmentID, ExecutionNodeID: nodeID, ExecutionRequirementDigest: requirement, Attempt: attempt, WorkItem: item, AttemptLeaseToken: lease, AttemptLeaseExpiresAt: leaseExpires.UTC().Format(time.RFC3339Nano), OwnerEpoch: ownerEpoch}, nil
}

func (s *Store) CompleteContinuationRestore(ctx context.Context, e domain.MachineEnvelope, attemptID int64, lease string, restoreRunID int64, resultDigest string, baseCommit string, resultCommit string, resultTree string) error {
	if len(resultDigest) != 64 || restoreRunID == 0 {
		return application.ErrIdentityConflict
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var worker, runAttempt, currentTurn, targetTurn, checkpointID int64
	var instance, boot, storedLease, status, expectedBase, expectedResult, expectedTree string
	var taskID int64
	if err = tx.QueryRow(ctx, `SELECT t.id FROM workforce_tasks t JOIN workforce_task_attempts a ON a.task_id=t.id AND a.task_turn_id=t.current_task_turn_id WHERE a.id=$1 AND t.latest_attempt_id=a.id AND t.cancel_requested_at IS NULL AND t.status='PREPARING' FOR UPDATE OF t`, attemptID).Scan(&taskID); err != nil {
		return application.ErrIdentityConflict
	}
	var restoreStatus, restoredDigest string
	var generation, owner int64
	var expires time.Time
	err = tx.QueryRow(ctx, `SELECT a.worker_id,a.worker_instance_id,a.host_boot_id,a.lease_token,a.lease_expires_at,a.status,a.generation,a.owner_epoch,r.attempt_id,r.target_turn_id,r.checkpoint_id,COALESCE(t.current_task_turn_id,0),r.status,COALESCE(r.restored_identity_digest,'')
		FROM workforce_continuation_restore_runs r JOIN workforce_task_attempts a ON a.id=r.attempt_id AND a.task_id=r.task_id AND a.task_turn_id=r.target_turn_id AND a.worker_id=r.worker_id AND a.worker_instance_id=r.worker_instance_id AND a.host_boot_id=r.host_boot_id AND a.generation=r.generation AND a.owner_epoch=r.owner_epoch JOIN workforce_tasks t ON t.id=r.task_id WHERE r.id=$1 FOR UPDATE OF r,a,t`, restoreRunID).Scan(&worker, &instance, &boot, &storedLease, &expires, &status, &generation, &owner, &runAttempt, &targetTurn, &checkpointID, &currentTurn, &restoreStatus, &restoredDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ErrNotFound
	}
	if err != nil {
		return err
	}
	var snapshot, session, purpose string
	if err = tx.QueryRow(ctx, `SELECT base_commit,result_commit,result_tree,task_execution_snapshot_digest,session_lineage_digest,purpose FROM workforce_continuation_checkpoints WHERE id=$1 AND task_id=$2 AND consumed_by_turn_id=$3 AND status='AVAILABLE' AND expires_at>clock_timestamp() FOR SHARE`, checkpointID, taskID, targetTurn).Scan(&expectedBase, &expectedResult, &expectedTree, &snapshot, &session, &purpose); err != nil {
		return err
	}
	if runAttempt != attemptID || targetTurn != currentTurn || worker != e.WorkerID || instance != e.InstanceID || boot != e.HostBootID || generation != e.Generation || owner != e.OwnerEpoch || storedLease != hashToken(lease) || !expires.After(time.Now()) || status != "PREPARING" {
		return application.ErrIdentityConflict
	}
	var currentInstance, currentBoot string
	if err = tx.QueryRow(ctx, `SELECT s.instance_id,s.host_boot_id FROM workforce_workers w JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id AND s.worker_id=w.id WHERE w.id=$1 AND w.desired_state IN ('ENABLED','DRAINING') AND w.decommissioned_at IS NULL AND s.ended_at IS NULL AND s.session_state='REGISTERED' FOR SHARE OF w,s`, worker).Scan(&currentInstance, &currentBoot); err != nil || currentInstance != instance || currentBoot != boot {
		return application.ErrIdentityConflict
	}
	if purpose == "INTERACTION_SUSPEND" {
		expectedDigest := hashToken(strings.Join([]string{strconv.FormatInt(checkpointID, 10), snapshot, expectedResult, expectedResult, expectedTree, session}, "\x00"))
		if resultDigest != expectedDigest {
			return application.ErrIdentityConflict
		}
	}
	if expectedBase != "" && baseCommit != expectedBase || expectedResult != "" && resultCommit != expectedResult || expectedTree != "" && resultTree != expectedTree {
		return application.ErrIdentityConflict
	}
	if restoreStatus == "RESTORED" {
		if restoredDigest != resultDigest {
			return application.ErrIdentityConflict
		}
		return tx.Commit(ctx) // exact authority-bound readback, no duplicate event
	}
	tag, err := tx.Exec(ctx, `UPDATE workforce_continuation_restore_runs SET status='RESTORED',restored_identity_digest=$2,restored_at=CURRENT_TIMESTAMP,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND status IN ('PENDING','RUNNING')`, restoreRunID, resultDigest)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return application.ErrIdentityConflict
	}
	if err = appendEvent(ctx, tx, (func() int64 {
		var value int64
		_ = tx.QueryRow(ctx, `SELECT task_id FROM workforce_task_attempts WHERE id=$1`, attemptID).Scan(&value)
		return value
	})(), attemptID, "continuation.restore.completed", map[string]any{"restoreRunId": restoreRunID, "taskTurnId": targetTurn, "resultDigest": resultDigest}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) AcquireLaunchPermit(ctx context.Context, e domain.MachineEnvelope, attemptID int64, lease string) (domain.LaunchPermit, bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.LaunchPermit{}, false, err
	}
	defer tx.Rollback(ctx)
	var taskID, nodeID, assignmentID, workerID int64
	var taskStatus, instanceID, boot, storedLease, status, imageDigest, hookDigest string
	var generation, owner int64
	var leaseExpires time.Time
	if err = tx.QueryRow(ctx, `SELECT task_id FROM workforce_task_attempts WHERE id=$1`, attemptID).Scan(&taskID); errors.Is(err, pgx.ErrNoRows) {
		return domain.LaunchPermit{}, false, application.ErrNotFound
	} else if err != nil {
		return domain.LaunchPermit{}, false, err
	}
	if err = tx.QueryRow(ctx, `SELECT status FROM workforce_tasks WHERE id=$1 FOR UPDATE`, taskID).Scan(&taskStatus); err != nil {
		return domain.LaunchPermit{}, false, err
	}
	err = tx.QueryRow(ctx, `SELECT a.execution_node_id,a.assignment_id,a.worker_id,a.worker_instance_id,a.host_boot_id,a.lease_token,a.lease_expires_at,a.status,a.generation,a.owner_epoch,
COALESCE(a.resolved_execution_snapshot_json->>'harnessImageDigest',''),COALESCE(a.resolved_execution_snapshot_json->>'hookConfigurationDigest','')
FROM workforce_task_attempts a WHERE a.id=$1 FOR UPDATE OF a`, attemptID).Scan(&nodeID, &assignmentID, &workerID, &instanceID, &boot, &storedLease, &leaseExpires, &status, &generation, &owner, &imageDigest, &hookDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.LaunchPermit{}, false, application.ErrNotFound
	}
	if err != nil {
		return domain.LaunchPermit{}, false, err
	}
	if workerID != e.WorkerID || instanceID != e.InstanceID || boot != e.HostBootID || generation != e.Generation || owner != e.OwnerEpoch || storedLease != hashToken(lease) || !leaseExpires.After(time.Now()) {
		return domain.LaunchPermit{}, false, application.ErrIdentityConflict
	}
	if taskStatus != "PREPARING" || status != "PREPARING" {
		return domain.LaunchPermit{}, false, application.ErrNotClaimable
	}
	var existingID int64
	var existingHash, existingName, identity string
	var expires time.Time
	err = tx.QueryRow(ctx, `SELECT COALESCE(launch_permit_id,0),COALESCE(launch_permit_sha256,''),expected_container_name,execution_identity_digest,COALESCE(launch_permit_expires_at,CURRENT_TIMESTAMP) FROM workforce_attempt_runtimes WHERE attempt_id=$1`, attemptID).Scan(&existingID, &existingHash, &existingName, &identity, &expires)
	if err == nil && existingID != 0 && expires.After(time.Now()) {
		token := deriveLaunchSecret(lease, existingID)
		if hashToken(token) != existingHash {
			return domain.LaunchPermit{}, false, application.ErrIdentityConflict
		}
		return domain.LaunchPermit{PermitID: existingID, PermitToken: token, ExpiresAt: expires.UTC().Format(time.RFC3339Nano), ExpectedContainerName: existingName, ExecutionIdentityDigest: identity, Labels: labels(workerID, instanceID, boot, taskID, nodeID, assignmentID, attemptID, generation, owner, identity, imageDigest, hookDigest)}, true, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return domain.LaunchPermit{}, false, err
	}
	name := fmt.Sprintf("seven-attempt-%d-g%d", attemptID, generation)
	identity = identityDigest(workerID, instanceID, boot, taskID, nodeID, assignmentID, attemptID, imageDigest, hookDigest, generation, owner)
	permitID := s.idGen.NextID()
	token := deriveLaunchSecret(lease, permitID)
	sum := sha256.Sum256([]byte(token))
	expires = time.Now().UTC().Add(2 * time.Minute)
	_, err = tx.Exec(ctx, `INSERT INTO workforce_attempt_runtimes (attempt_id,execution_node_id,assignment_id,worker_id,worker_instance_id,host_boot_id,generation,owner_epoch,execution_identity_digest,expected_container_name,harness_image_digest,hook_configuration_digest,status,launch_permit_id,launch_permit_sha256,launch_permit_expires_at,execution_lease_sha256,execution_lease_expires_at,execution_lease_renewed_at,startup_deadline,supervisor_protocol_version) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'AUTHORIZED',$13,$14,$15,$16,$17,CURRENT_TIMESTAMP,$18,2)`, attemptID, nodeID, assignmentID, workerID, instanceID, boot, generation, owner, identity, name, imageDigest, hookDigest, permitID, hex.EncodeToString(sum[:]), expires, storedLease, leaseExpires, time.Now().UTC().Add(2*time.Minute))
	if err != nil {
		return domain.LaunchPermit{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.LaunchPermit{}, false, err
	}
	return domain.LaunchPermit{PermitID: permitID, PermitToken: token, ExpiresAt: expires.Format(time.RFC3339Nano), ExpectedContainerName: name, ExecutionIdentityDigest: identity, Labels: labels(workerID, instanceID, boot, taskID, nodeID, assignmentID, attemptID, generation, owner, identity, imageDigest, hookDigest)}, false, nil
}

func (s *Store) RecordContainerCreated(ctx context.Context, e domain.MachineEnvelope, attemptID int64, lease string, containerID string, imageDigest string, identity string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	authority, err := loadRuntimeAuthority(ctx, tx, attemptID)
	if err != nil || !authority.valid(e, lease) {
		if err != nil {
			return err
		}
		return application.ErrIdentityConflict
	}
	if authority.ImageDigest != imageDigest || authority.IdentityDigest != identity {
		return application.ErrIdentityConflict
	}
	if authority.Status == "CONTAINER_CREATED" && authority.ContainerID == containerID {
		return tx.Commit(ctx)
	}
	if authority.Status != "AUTHORIZED" || (authority.ContainerID != "" && authority.ContainerID != containerID) {
		return application.ErrNotClaimable
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_attempt_runtimes SET container_id=$2,status='CONTAINER_CREATED',container_created_at=COALESCE(container_created_at,CURRENT_TIMESTAMP),updated_at=CURRENT_TIMESTAMP WHERE attempt_id=$1`, attemptID, containerID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_task_attempts SET container_id=$2,runtime_image_digest=$3,image_contract_version='seven.worker-image/v1',supervisor_protocol_version=2,last_heartbeat_at=CURRENT_TIMESTAMP,last_progress_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='PREPARING'`, attemptID, containerID, imageDigest); err != nil {
		return err
	}
	if err = appendEvent(ctx, tx, authority.TaskID, attemptID, "runtime.container_created", map[string]any{"containerId": containerID, "imageDigest": imageDigest, "executionIdentityDigest": identity}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RecordProviderProxyReady(ctx context.Context, e domain.MachineEnvelope, attemptID int64, lease string, ready domain.ProviderProxyReady) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	authority, err := loadRuntimeAuthority(ctx, tx, attemptID)
	if err != nil || !authority.valid(e, lease) {
		if err != nil {
			return err
		}
		return application.ErrIdentityConflict
	}
	var modelCredentialID int64
	var expectedDigest string
	if err = tx.QueryRow(ctx, `SELECT COALESCE(a.resolved_execution_snapshot_json->>'providerProxyImageDigest',''),
COALESCE((a.resolved_execution_snapshot_json->>'modelCredentialId')::BIGINT,0)
FROM workforce_task_attempts a WHERE a.id=$1`, attemptID).Scan(&expectedDigest, &modelCredentialID); err != nil {
		return err
	}
	if modelCredentialID == 0 || expectedDigest == "" || ready.ImageDigest != expectedDigest || ready.ControlState != "READY" {
		return application.ErrIdentityConflict
	}
	var existingID, existingIdentity string
	if err = tx.QueryRow(ctx, `SELECT COALESCE(provider_proxy_container_id,''),provider_proxy_execution_identity_digest
FROM workforce_attempt_runtimes WHERE attempt_id=$1`, attemptID).Scan(&existingID, &existingIdentity); err != nil {
		return err
	}
	if existingID != "" {
		if existingID != ready.ContainerID || existingIdentity != ready.ExecutionIdentityDigest {
			return application.ErrIdentityConflict
		}
		return tx.Commit(ctx)
	}
	if authority.Status != domain.RuntimeAuthorized {
		return application.ErrNotClaimable
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_attempt_runtimes SET
provider_proxy_container_id=$2,provider_proxy_container_name=$3,provider_proxy_image_digest=$4,
provider_proxy_execution_identity_digest=$5,provider_proxy_owner_epoch=$6,provider_proxy_control_state='READY',
provider_proxy_ready_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE attempt_id=$1`,
		attemptID, ready.ContainerID, ready.ContainerName, ready.ImageDigest, ready.ExecutionIdentityDigest, e.OwnerEpoch); err != nil {
		return err
	}
	if err = appendEvent(ctx, tx, authority.TaskID, attemptID, "runtime.provider_proxy_ready", map[string]any{
		"containerId": ready.ContainerID, "containerName": ready.ContainerName, "imageDigest": ready.ImageDigest,
		"executionIdentityDigest": ready.ExecutionIdentityDigest, "ownerEpoch": e.OwnerEpoch,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RecordSupervisorReady(ctx context.Context, e domain.MachineEnvelope, attemptID int64, lease string, ready domain.SupervisorReady) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	authority, err := loadRuntimeAuthority(ctx, tx, attemptID)
	if err != nil || !authority.valid(e, lease) {
		if err != nil {
			return err
		}
		return application.ErrIdentityConflict
	}
	if authority.ContainerID != ready.ContainerID || authority.IdentityDigest != ready.ExecutionIdentity || ready.ControlProtocol != "seven.supervisor-control/v2" {
		return application.ErrIdentityConflict
	}
	if authority.Status == "SUPERVISOR_READY" || authority.Status == "STARTING" || authority.Status == "RUNNING" {
		return tx.Commit(ctx)
	}
	if authority.Status != "CONTAINER_CREATED" {
		return application.ErrNotClaimable
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_attempt_runtimes SET status='SUPERVISOR_READY',supervisor_ready_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE attempt_id=$1`, attemptID); err != nil {
		return err
	}
	if err = appendEvent(ctx, tx, authority.TaskID, attemptID, "runtime.supervisor_ready", map[string]any{"containerId": ready.ContainerID, "controlProtocol": ready.ControlProtocol}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) AcquireRunPermit(ctx context.Context, e domain.MachineEnvelope, attemptID int64, lease string) (domain.RunPermit, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.RunPermit{}, err
	}
	defer tx.Rollback(ctx)
	authority, err := loadRuntimeAuthority(ctx, tx, attemptID)
	if err != nil || !authority.valid(e, lease) {
		if err != nil {
			return domain.RunPermit{}, err
		}
		return domain.RunPermit{}, application.ErrIdentityConflict
	}
	if authority.Status != "SUPERVISOR_READY" && authority.Status != "STARTING" {
		return domain.RunPermit{}, application.ErrNotClaimable
	}
	var modelCredentialID int64
	var providerProxyDigest, recordedProxyID, recordedProxyDigest, proxyState string
	var proxyOwner *int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE((a.resolved_execution_snapshot_json->>'modelCredentialId')::BIGINT,0),
COALESCE(a.resolved_execution_snapshot_json->>'providerProxyImageDigest',''),
COALESCE(r.provider_proxy_container_id,''),COALESCE(r.provider_proxy_image_digest,''),COALESCE(r.provider_proxy_control_state,''),r.provider_proxy_owner_epoch
FROM workforce_task_attempts a
JOIN workforce_attempt_runtimes r ON r.attempt_id=a.id
WHERE a.id=$1`, attemptID).Scan(&modelCredentialID, &providerProxyDigest, &recordedProxyID, &recordedProxyDigest, &proxyState, &proxyOwner); err != nil {
		return domain.RunPermit{}, err
	}
	if modelCredentialID != 0 && (recordedProxyID == "" || recordedProxyDigest != providerProxyDigest || proxyState != "READY" || proxyOwner == nil || *proxyOwner != authority.OwnerEpoch) {
		return domain.RunPermit{}, application.ErrNotClaimable
	}
	if authority.Status == "STARTING" {
		var permitID int64
		var permitHash, nonceHash string
		var expires time.Time
		if err = tx.QueryRow(ctx, `SELECT COALESCE(run_permit_id,0),COALESCE(run_permit_sha256,''),COALESCE(run_nonce_sha256,''),run_permit_expires_at FROM workforce_attempt_runtimes WHERE attempt_id=$1`, attemptID).Scan(&permitID, &permitHash, &nonceHash, &expires); err != nil {
			return domain.RunPermit{}, err
		}
		if permitID == 0 || !expires.After(s.now()) {
			return domain.RunPermit{}, application.ErrNotClaimable
		}
		permitToken := deriveRunSecret(lease, "permit", permitID)
		runNonce := deriveRunSecret(lease, "nonce", permitID)
		if hashToken(permitToken) != permitHash || hashToken(runNonce) != nonceHash {
			return domain.RunPermit{}, application.ErrIdentityConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return domain.RunPermit{}, err
		}
		return domain.RunPermit{PermitID: permitID, PermitToken: permitToken, RunNonce: runNonce, ExpiresAt: expires.UTC().Format(time.RFC3339Nano), OwnerEpoch: authority.OwnerEpoch}, nil
	}
	permitID := s.idGen.NextID()
	permitToken := deriveRunSecret(lease, "permit", permitID)
	runNonce := deriveRunSecret(lease, "nonce", permitID)
	expires := s.now().UTC().Add(30 * time.Second)
	if _, err = tx.Exec(ctx, `UPDATE workforce_attempt_runtimes SET status='STARTING',run_permit_id=$2,run_permit_sha256=$3,run_permit_expires_at=$4,run_permit_issued_at=CURRENT_TIMESTAMP,run_nonce_sha256=$5,run_nonce_accepted_at=NULL,updated_at=CURRENT_TIMESTAMP WHERE attempt_id=$1`, attemptID, permitID, hashToken(permitToken), expires, hashToken(runNonce)); err != nil {
		return domain.RunPermit{}, err
	}
	if err = appendEvent(ctx, tx, authority.TaskID, attemptID, "runtime.run_permit_issued", map[string]any{"runPermitId": permitID, "expiresAt": expires.Format(time.RFC3339Nano), "ownerEpoch": authority.OwnerEpoch}); err != nil {
		return domain.RunPermit{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.RunPermit{}, err
	}
	return domain.RunPermit{PermitID: permitID, PermitToken: permitToken, RunNonce: runNonce, ExpiresAt: expires.Format(time.RFC3339Nano), OwnerEpoch: authority.OwnerEpoch}, nil
}

func (s *Store) RecordHarnessStarted(ctx context.Context, e domain.MachineEnvelope, attemptID int64, lease string, started domain.HarnessStarted) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	authority, err := loadRuntimeAuthority(ctx, tx, attemptID)
	if err != nil || !authority.valid(e, lease) {
		if err != nil {
			return err
		}
		return application.ErrIdentityConflict
	}
	var permitID int64
	var permitHash, nonceHash string
	var permitExpires time.Time
	if err = tx.QueryRow(ctx, `SELECT COALESCE(run_permit_id,0),COALESCE(run_permit_sha256,''),COALESCE(run_nonce_sha256,''),COALESCE(run_permit_expires_at,CURRENT_TIMESTAMP) FROM workforce_attempt_runtimes WHERE attempt_id=$1`, attemptID).Scan(&permitID, &permitHash, &nonceHash, &permitExpires); err != nil {
		return err
	}
	if permitID != started.RunPermitID || permitHash != started.RunPermitDigest || nonceHash != started.RunNonceDigest {
		return application.ErrIdentityConflict
	}
	if authority.Status == "RUNNING" {
		var storedPID int
		if err = tx.QueryRow(ctx, `SELECT harness_pid FROM workforce_attempt_runtimes WHERE attempt_id=$1`, attemptID).Scan(&storedPID); err != nil {
			return err
		}
		if storedPID != started.HarnessPID {
			return application.ErrIdentityConflict
		}
		return tx.Commit(ctx)
	}
	if authority.Status != "STARTING" || !permitExpires.After(time.Now()) {
		return application.ErrNotClaimable
	}
	startedAt, parseErr := time.Parse(time.RFC3339Nano, started.StartedAt)
	if parseErr != nil || startedAt.After(time.Now().Add(time.Minute)) {
		return application.ErrIdentityConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_attempt_runtimes SET status='RUNNING',harness_started_at=$2,harness_pid=$3,run_nonce_accepted_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE attempt_id=$1`, attemptID, startedAt, started.HarnessPID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_task_attempts SET status='RUNNING',started_at=COALESCE(started_at,$2),last_heartbeat_at=CURRENT_TIMESTAMP,last_progress_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='PREPARING'`, attemptID, startedAt); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='RUNNING',version=version+1 WHERE id=$1 AND status='PREPARING'`, authority.TaskID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='RUNNING',updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='PREPARING'`, authority.NodeID); err != nil {
		return err
	}
	if err = appendEvent(ctx, tx, authority.TaskID, attemptID, "runtime.harness_started", map[string]any{"runPermitId": permitID, "runNonceDigest": nonceHash, "harnessPid": started.HarnessPID, "startedAt": startedAt.Format(time.RFC3339Nano)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RenewExecutionLease(ctx context.Context, e domain.MachineEnvelope, attemptID int64, lease string, activeElapsedMillis *int64) (string, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	authority, err := loadRuntimeAuthority(ctx, tx, attemptID)
	if err != nil || !authority.validAllowExpired(e, lease, 5*time.Second) {
		if err != nil {
			return "", err
		}
		return "", application.ErrIdentityConflict
	}
	if authority.Status == "SUCCEEDED" || authority.Status == "FAILED" || authority.Status == "CANCELLED" {
		return "", application.ErrNotClaimable
	}
	var taskID int64
	var confirmedActiveMillis int64
	var timeoutSeconds int64
	if activeElapsedMillis != nil {
		// loadRuntimeAuthority already locks Task before Attempt/Runtime,
		// matching interaction responses and cancellation.
		err = tx.QueryRow(ctx, `SELECT id,execution_active_milliseconds,COALESCE(execution_timeout_seconds,0) FROM workforce_tasks WHERE id=$1 FOR UPDATE`, authority.TaskID).Scan(&taskID, &confirmedActiveMillis, &timeoutSeconds)
		if err != nil {
			return "", err
		}
	}
	if activeElapsedMillis != nil {
		if authority.TaskID != taskID || *activeElapsedMillis < confirmedActiveMillis || (timeoutSeconds > 0 && *activeElapsedMillis > timeoutSeconds*1000+1000) {
			return "", application.ErrIdentityConflict
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET execution_active_milliseconds=$2 WHERE id=$1`, taskID, *activeElapsedMillis); err != nil {
			return "", err
		}
	}
	expires := time.Now().UTC().Add(30 * time.Second)
	if _, err = tx.Exec(ctx, `UPDATE workforce_task_attempts SET lease_expires_at=$2,last_heartbeat_at=CURRENT_TIMESTAMP WHERE id=$1`, attemptID, expires); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_attempt_runtimes SET execution_lease_expires_at=$2,execution_lease_renewed_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE attempt_id=$1`, attemptID, expires); err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return expires.Format(time.RFC3339Nano), nil
}

func (s *Store) AcquireReattachPermit(ctx context.Context, e domain.MachineEnvelope, attemptID int64, evidence domain.RuntimeEvidence) (domain.ReattachPermit, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.ReattachPermit{}, err
	}
	defer tx.Rollback(ctx)

	var currentInstance, desiredState, currentBoot string
	if err = tx.QueryRow(ctx, `SELECT w.current_instance_id,w.desired_state,s.host_boot_id
FROM workforce_workers w
JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id
WHERE w.id=$1 AND w.decommissioned_at IS NULL
FOR UPDATE OF w,s`, e.WorkerID).Scan(&currentInstance, &desiredState, &currentBoot); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ReattachPermit{}, application.ErrNotFound
		}
		return domain.ReattachPermit{}, err
	}
	if currentInstance != e.InstanceID || currentBoot != e.HostBootID || desiredState == "DISABLED" {
		return domain.ReattachPermit{}, application.ErrStaleInstance
	}

	var expected domain.RuntimeRecoveryIdentity
	var runtimeStatus, previousStatus, taskStatus, oldInstance string
	var deadline time.Time
	err = tx.QueryRow(ctx, `SELECT r.attempt_id,r.execution_node_id,r.assignment_id,r.worker_id,r.host_boot_id,
  r.generation,r.owner_epoch,COALESCE(r.container_id,''),r.expected_container_name,r.execution_identity_digest,
  r.harness_image_digest,r.hook_configuration_digest,r.status,COALESCE(r.recovery_previous_status,''),
  COALESCE(r.reattach_deadline,CURRENT_TIMESTAMP),a.worker_instance_id,t.status,
  COALESCE(r.provider_proxy_container_id,''),COALESCE(r.provider_proxy_container_name,''),
  r.provider_proxy_image_digest,r.provider_proxy_execution_identity_digest
FROM workforce_attempt_runtimes r
JOIN workforce_task_attempts a ON a.id=r.attempt_id
JOIN workforce_tasks t ON t.id=a.task_id
WHERE r.attempt_id=$1
FOR UPDATE OF r,a,t`, attemptID).Scan(
		&expected.AttemptID, &expected.ExecutionNodeID, &expected.AssignmentID, &expected.WorkerID, &expected.HostBootID,
		&expected.Generation, &expected.OwnerEpoch, &expected.ContainerID, &expected.ContainerName, &expected.ExecutionIdentityDigest,
		&expected.HarnessImageDigest, &expected.HookConfigurationDigest, &runtimeStatus, &previousStatus, &deadline, &oldInstance, &taskStatus,
		&expected.ProviderProxyContainerID, &expected.ProviderProxyContainerName,
		&expected.ProviderProxyImageDigest, &expected.ProviderProxyIdentityDigest,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ReattachPermit{}, application.ErrNotFound
	}
	if err != nil {
		return domain.ReattachPermit{}, err
	}
	if err = tx.QueryRow(ctx, `SELECT COALESCE((resolved_execution_snapshot_json->>'modelCredentialId')::BIGINT,0)>0 FROM workforce_task_attempts WHERE id=$1`, attemptID).Scan(&expected.ProviderBacked); err != nil {
		return domain.ReattachPermit{}, err
	}
	expected.ReattachDeadline = deadline
	if expected.WorkerID != e.WorkerID || expected.HostBootID != e.HostBootID || expected.Generation != e.Generation || expected.OwnerEpoch != e.OwnerEpoch {
		return domain.ReattachPermit{}, application.ErrIdentityConflict
	}
	if taskStatus == "CANCELLING" || taskStatus == "INTERACTION_SUSPENDED" || isTerminalTaskStatus(taskStatus) {
		return domain.ReattachPermit{}, application.ErrNotClaimable
	}
	if runtimeStatus != domain.RuntimeRecovering && runtimeStatus != domain.RuntimeReattaching && runtimeStatus != domain.RuntimeReconciliationRequired {
		return domain.ReattachPermit{}, application.ErrNotClaimable
	}
	if err = evidence.ValidateFor(expected, s.now().UTC()); err != nil {
		return domain.ReattachPermit{}, application.ErrIdentityConflict
	}
	mode, startNeeded, err := domain.ResolveReattachMode(evidence.SupervisorState, evidence.HarnessPID)
	if err != nil || !validRecoveryProgress(previousStatus, evidence.SupervisorState, evidence.HarnessPID) {
		return domain.ReattachPermit{}, application.ErrIdentityConflict
	}

	newOwner := expected.OwnerEpoch + 1
	permitID := s.idGen.NextID()
	leaseToken := randomToken()
	permitToken := deriveReattachSecret(leaseToken, permitID, newOwner)
	expires := s.now().UTC().Add(30 * time.Second)
	leaseExpires := s.now().UTC().Add(domain.DefaultRecoveryPolicy().LeaseTTL)
	evidenceBody, _ := json.Marshal(evidence)
	evidenceSum := sha256.Sum256(evidenceBody)
	reconciliationID := s.idGen.NextID()

	clearRun := ""
	if startNeeded {
		clearRun = `,run_permit_id=NULL,run_permit_sha256=NULL,run_permit_expires_at=NULL,run_permit_issued_at=NULL,run_nonce_sha256=NULL,run_nonce_accepted_at=NULL,harness_pid=NULL,harness_started_at=NULL`
	}
	query := `UPDATE workforce_attempt_runtimes SET status='REATTACHING',owner_epoch=$2,pending_worker_instance_id=$3,
  pending_owner_epoch=$2,reattach_permit_id=$4,reattach_permit_sha256=$5,reattach_permit_expires_at=$6,
  execution_lease_sha256=$7,execution_lease_expires_at=$8,execution_lease_renewed_at=CURRENT_TIMESTAMP,
  authority_receipt_sha256=NULL,updated_at=CURRENT_TIMESTAMP` + clearRun + ` WHERE attempt_id=$1`
	if _, err = tx.Exec(ctx, query, attemptID, newOwner, e.InstanceID, permitID, hashToken(permitToken), expires, hashToken(leaseToken), leaseExpires); err != nil {
		return domain.ReattachPermit{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_task_attempts SET worker_instance_id=$2,host_boot_id=$3,owner_epoch=$4,
  lease_token=$5,lease_expires_at=$6,status='RECOVERING',last_heartbeat_at=CURRENT_TIMESTAMP,last_progress_at=CURRENT_TIMESTAMP
WHERE id=$1`, attemptID, e.InstanceID, e.HostBootID, newOwner, hashToken(leaseToken), leaseExpires); err != nil {
		return domain.ReattachPermit{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_task_assignments SET worker_instance_id=$2 WHERE id=$1`, expected.AssignmentID, e.InstanceID); err != nil {
		return domain.ReattachPermit{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='RECOVERING',updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND status IN ('PREPARING','RUNNING','RECOVERING')`, expected.ExecutionNodeID); err != nil {
		return domain.ReattachPermit{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_runtime_reconciliation_runs
  (id,attempt_id,execution_node_id,worker_id,old_instance_id,new_instance_id,host_boot_id,old_owner_epoch,new_owner_epoch,status,runtime_evidence_sha256,deadline)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'PERMIT_ISSUED',$10,$11)`, reconciliationID, attemptID, expected.ExecutionNodeID, e.WorkerID, oldInstance, e.InstanceID, e.HostBootID, expected.OwnerEpoch, newOwner, hex.EncodeToString(evidenceSum[:]), expires); err != nil {
		return domain.ReattachPermit{}, err
	}
	if err = appendEvent(ctx, tx, taskIDForAttempt(ctx, tx, attemptID), attemptID, "runtime.reattach_permit_issued", map[string]any{
		"reconciliationId": strconv.FormatInt(reconciliationID, 10), "oldOwnerEpoch": strconv.FormatInt(expected.OwnerEpoch, 10), "newOwnerEpoch": strconv.FormatInt(newOwner, 10), "mode": mode,
		"runtimeEvidence": evidence,
	}); err != nil {
		return domain.ReattachPermit{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.ReattachPermit{}, err
	}
	return domain.ReattachPermit{
		InteractionReplayRequested: evidence.InteractionReplayRequested && !startNeeded,
		ContainerID:                expected.ContainerID, ExecutionIdentityDigest: expected.ExecutionIdentityDigest,
		AdapterSessionGeneration: evidence.AdapterSessionGeneration, RoundKey: evidence.RoundKey,
		PermitID: permitID, PermitToken: permitToken, ExpiresAt: expires.Format(time.RFC3339Nano),
		OldOwnerEpoch: expected.OwnerEpoch, NewOwnerEpoch: newOwner, LeaseToken: leaseToken,
		LeaseExpiresAt: leaseExpires.Format(time.RFC3339Nano), Mode: mode, StartNeeded: startNeeded,
	}, nil
}

func (s *Store) CommitReattach(ctx context.Context, e domain.MachineEnvelope, attemptID int64, lease string, ack domain.ReattachAck) (domain.ReattachResult, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.ReattachResult{}, err
	}
	defer tx.Rollback(ctx)
	var currentInstance string
	if err = tx.QueryRow(ctx, `SELECT current_instance_id FROM workforce_workers WHERE id=$1 AND decommissioned_at IS NULL FOR UPDATE`, e.WorkerID).Scan(&currentInstance); err != nil {
		return domain.ReattachResult{}, err
	}
	if currentInstance != e.InstanceID {
		return domain.ReattachResult{}, application.ErrStaleInstance
	}
	var taskID, nodeID, assignmentID, workerID, permitID int64
	var instanceID, boot, status, permitHash, storedLease, requirement string
	var providerBacked bool
	var generation, owner int64
	var permitExpires, leaseExpires time.Time
	err = tx.QueryRow(ctx, `SELECT a.task_id,a.execution_node_id,a.assignment_id,a.worker_id,a.worker_instance_id,a.host_boot_id,
  a.generation,a.owner_epoch,a.lease_token,a.lease_expires_at,r.status,COALESCE(r.reattach_permit_id,0),
  COALESCE(r.reattach_permit_sha256,''),COALESCE(r.reattach_permit_expires_at,CURRENT_TIMESTAMP),n.execution_requirement_digest
FROM workforce_task_attempts a
JOIN workforce_attempt_runtimes r ON r.attempt_id=a.id
JOIN workforce_execution_nodes n ON n.id=a.execution_node_id
WHERE a.id=$1 FOR UPDATE OF a,r,n`, attemptID).Scan(&taskID, &nodeID, &assignmentID, &workerID, &instanceID, &boot,
		&generation, &owner, &storedLease, &leaseExpires, &status, &permitID, &permitHash, &permitExpires, &requirement)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ReattachResult{}, application.ErrNotFound
	}
	if err != nil {
		return domain.ReattachResult{}, err
	}
	if err = tx.QueryRow(ctx, `SELECT COALESCE((resolved_execution_snapshot_json->>'modelCredentialId')::BIGINT,0)>0 FROM workforce_task_attempts WHERE id=$1`, attemptID).Scan(&providerBacked); err != nil {
		return domain.ReattachResult{}, err
	}
	if workerID != e.WorkerID || instanceID != e.InstanceID || boot != e.HostBootID || generation != e.Generation || owner != e.OwnerEpoch ||
		storedLease != hashToken(lease) || !leaseExpires.After(s.now()) || permitID != ack.PermitID ||
		permitHash != ack.PermitDigest || ack.NewOwnerEpoch != owner || ack.OldOwnerEpoch+1 != owner {
		return domain.ReattachResult{}, application.ErrIdentityConflict
	}
	if providerBacked {
		if ack.ProviderProxy == nil || ack.ProviderProxy.PermitID != ack.PermitID || ack.ProviderProxy.PermitDigest != ack.PermitDigest ||
			ack.ProviderProxy.OldOwnerEpoch != ack.OldOwnerEpoch || ack.ProviderProxy.NewOwnerEpoch != ack.NewOwnerEpoch || ack.ProviderProxy.ControlState != "READY" {
			return domain.ReattachResult{}, application.ErrIdentityConflict
		}
	} else if ack.ProviderProxy != nil {
		return domain.ReattachResult{}, application.ErrIdentityConflict
	}
	mode, startNeeded, err := domain.ResolveReattachMode(ack.SupervisorState, ack.HarnessPID)
	if err != nil {
		return domain.ReattachResult{}, application.ErrIdentityConflict
	}
	restoredStatus := ack.SupervisorState
	if restoredStatus == domain.RuntimeStarting && ack.HarnessPID > 0 {
		restoredStatus = domain.RuntimeRunning
	}
	ackBody, _ := json.Marshal(ack)
	ackSum := sha256.Sum256(ackBody)
	replay, err := validateInteractionReattachCommit(ctx, tx, e, taskID, attemptID, ack, hex.EncodeToString(ackSum[:]), startNeeded)
	if err != nil {
		return domain.ReattachResult{}, err
	}
	directive := domain.DirectiveResumeAuthorizedRuntime
	if restoredStatus == domain.RuntimeRunning {
		directive = domain.DirectiveAckAlreadyRunning
	}
	if replay.Completed {
		if status != restoredStatus {
			return domain.ReattachResult{}, application.ErrIdentityConflict
		}
		contract, loadErr := s.loadContract(ctx, tx, assignmentID, taskID, nodeID, requirement, directive, lease, leaseExpires, owner)
		if loadErr != nil {
			return domain.ReattachResult{}, loadErr
		}
		if err = tx.Commit(ctx); err != nil {
			return domain.ReattachResult{}, err
		}
		return domain.ReattachResult{Contract: contract, RuntimeState: restoredStatus, Mode: mode, StartNeeded: startNeeded, InteractionReplayBinding: replay.Binding}, nil
	}
	if status != domain.RuntimeReattaching || !permitExpires.After(s.now()) {
		return domain.ReattachResult{}, application.ErrIdentityConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_attempt_runtimes SET status=$2,worker_instance_id=$3,pending_worker_instance_id=NULL,
  pending_owner_epoch=NULL,reattached_at=CURRENT_TIMESTAMP,authority_receipt_sha256=$4,recovery_previous_status=NULL,
  harness_pid=NULLIF($5::BIGINT,0),provider_proxy_owner_epoch=CASE WHEN $6 THEN $7 ELSE provider_proxy_owner_epoch END,
  provider_proxy_control_state=CASE WHEN $6 THEN 'READY' ELSE provider_proxy_control_state END,updated_at=CURRENT_TIMESTAMP WHERE attempt_id=$1`, attemptID, restoredStatus, e.InstanceID, hex.EncodeToString(ackSum[:]), ack.HarnessPID, providerBacked, owner); err != nil {
		return domain.ReattachResult{}, err
	}
	attemptStatus, nodeStatus := "PREPARING", "PREPARING"
	if restoredStatus == domain.RuntimeRunning {
		attemptStatus, nodeStatus = "RUNNING", "RUNNING"
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_task_attempts SET status=$2,last_heartbeat_at=CURRENT_TIMESTAMP,last_progress_at=CURRENT_TIMESTAMP WHERE id=$1`, attemptID, attemptStatus); err != nil {
		return domain.ReattachResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status=$2,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, nodeID, nodeStatus); err != nil {
		return domain.ReattachResult{}, err
	}
	proxyAckHash := ""
	if ack.ProviderProxy != nil {
		proxyBody, _ := json.Marshal(ack.ProviderProxy)
		proxySum := sha256.Sum256(proxyBody)
		proxyAckHash = hex.EncodeToString(proxySum[:])
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_runtime_reconciliation_runs SET status='COMPLETED',supervisor_ack_sha256=$2,provider_proxy_ack_sha256=NULLIF($4,''),completed_at=CURRENT_TIMESTAMP
WHERE attempt_id=$1 AND new_owner_epoch=$3 AND status='PERMIT_ISSUED'`, attemptID, hex.EncodeToString(ackSum[:]), owner, proxyAckHash); err != nil {
		return domain.ReattachResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_worker_sessions SET session_state='REGISTERED',reconciliation_deadline=NULL
WHERE instance_id=$1 AND NOT EXISTS (SELECT 1 FROM workforce_attempt_runtimes WHERE worker_id=$2 AND status IN ('RECOVERING','REATTACHING','RECONCILIATION_REQUIRED') AND attempt_id<>$3)`, e.InstanceID, e.WorkerID, attemptID); err != nil {
		return domain.ReattachResult{}, err
	}
	contract, err := s.loadContract(ctx, tx, assignmentID, taskID, nodeID, requirement, directive, lease, leaseExpires, owner)
	if err != nil {
		return domain.ReattachResult{}, err
	}
	if err = appendEvent(ctx, tx, taskID, attemptID, "runtime.reattached", map[string]any{"ownerEpoch": strconv.FormatInt(owner, 10), "runtimeStatus": restoredStatus, "mode": mode, "reconciliationId": strconv.FormatInt(replay.ReconciliationID, 10), "interactionReplayBinding": replay.Binding}); err != nil {
		return domain.ReattachResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.ReattachResult{}, err
	}
	return domain.ReattachResult{Contract: contract, RuntimeState: restoredStatus, Mode: mode, StartNeeded: startNeeded, InteractionReplayBinding: replay.Binding}, nil
}

func (s *Store) ReportRuntimeLoss(ctx context.Context, e domain.MachineEnvelope, attemptID int64, reason string, evidenceDigest string) (domain.RuntimeLossResult, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.RuntimeLossResult{}, err
	}
	defer tx.Rollback(ctx)
	var currentInstance string
	if err = tx.QueryRow(ctx, `SELECT current_instance_id FROM workforce_workers WHERE id=$1 AND decommissioned_at IS NULL FOR UPDATE`, e.WorkerID).Scan(&currentInstance); err != nil {
		return domain.RuntimeLossResult{}, err
	}
	if currentInstance != e.InstanceID {
		return domain.RuntimeLossResult{}, application.ErrStaleInstance
	}
	// Runtime loss is a fencing operation, not a generic Worker mutation. Bind
	// it to the exact Worker, generation and owner that the current instance was
	// asked to reconcile. Without this check, the current instance of Worker A
	// could consume the retry budget of an Attempt owned by Worker B merely by
	// knowing its public Attempt ID.
	var runtimeWorker int64
	var runtimeGeneration, runtimeOwner int64
	if err = tx.QueryRow(ctx, `SELECT worker_id,generation,owner_epoch FROM workforce_attempt_runtimes WHERE attempt_id=$1 FOR UPDATE`, attemptID).Scan(&runtimeWorker, &runtimeGeneration, &runtimeOwner); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.RuntimeLossResult{}, application.ErrNotFound
		}
		return domain.RuntimeLossResult{}, err
	}
	if runtimeWorker != e.WorkerID || runtimeGeneration != e.Generation || runtimeOwner != e.OwnerEpoch {
		return domain.RuntimeLossResult{}, application.ErrIdentityConflict
	}
	result, err := s.fenceRuntimeLoss(ctx, tx, attemptID, reason, evidenceDigest)
	if err != nil {
		return domain.RuntimeLossResult{}, err
	}
	if err = completeEmptyReconciliations(ctx, tx); err != nil {
		return domain.RuntimeLossResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.RuntimeLossResult{}, err
	}
	return result, nil
}

func (s *Store) ReconcileRuntimeLosses(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 256 {
		limit = 32
	}
	// A terminal result can be committed by another authoritative module (for
	// example Result Processor REJECT) after the Agent runtime has ended. Heal
	// the corresponding capacity record here so an already-terminal Attempt
	// cannot permanently occupy a Worker slot after response loss or restart.
	// This is deliberately identity-derived; no caller supplies a cleanup target.
	if _, err := s.db.Exec(ctx, `UPDATE workforce_task_assignments a
SET status='COMPLETED',completed_at=COALESCE(completed_at,CURRENT_TIMESTAMP),
    release_reason_code=COALESCE(NULLIF(release_reason_code,''),'TERMINAL_ATTEMPT_RECONCILED')
FROM workforce_task_attempts ta
WHERE ta.assignment_id=a.id
  AND a.status IN ('RESERVED','PUBLISHED','CLAIMED')
  AND ta.status IN ('SUCCEEDED','FAILED','CANCELLED','TIMED_OUT','LOST','EXECUTION_COMPLETED','INTERACTION_SUSPENDED')`); err != nil {
		return 0, err
	}
	if _, err := s.db.Exec(ctx, `UPDATE workforce_execution_nodes n SET status='QUEUED',next_retry_at=NULL,updated_at=CURRENT_TIMESTAMP
FROM workforce_tasks t WHERE t.id=n.task_id AND n.status='RETRY_WAIT' AND n.next_retry_at<=CURRENT_TIMESTAMP
  AND t.status='RETRY_WAIT' AND t.cancel_requested_at IS NULL`); err != nil {
		return 0, err
	}
	if _, err := s.db.Exec(ctx, `UPDATE workforce_tasks t SET status='QUEUED',version=version+1
WHERE t.status='RETRY_WAIT' AND EXISTS (SELECT 1 FROM workforce_execution_nodes n WHERE n.task_id=t.id AND n.status='QUEUED')`); err != nil {
		return 0, err
	}
	rows, err := s.db.Query(ctx, `SELECT attempt_id FROM workforce_attempt_runtimes
WHERE status IN ('RECOVERING','REATTACHING','RECONCILIATION_REQUIRED')
  AND reattach_deadline IS NOT NULL AND reattach_deadline+INTERVAL '15 seconds'<=CURRENT_TIMESTAMP
ORDER BY reattach_deadline LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	var attemptIDs []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		attemptIDs = append(attemptIDs, id)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	completed := 0
	for _, attemptID := range attemptIDs {
		tx, beginErr := s.db.Begin(ctx)
		if beginErr != nil {
			return completed, beginErr
		}
		_, fenceErr := s.fenceRuntimeLoss(ctx, tx, attemptID, domain.ReasonWorkerLostWithoutRuntime, "")
		if fenceErr == nil {
			fenceErr = completeEmptyReconciliations(ctx, tx)
		}
		if fenceErr == nil {
			fenceErr = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
		if errors.Is(fenceErr, application.ErrNotClaimable) {
			continue
		}
		if fenceErr != nil {
			return completed, fenceErr
		}
		completed++
	}
	return completed, nil
}

// completeEmptyReconciliations closes the Worker startup gate once every
// Runtime presented to the current instance has either been reattached or
// fenced. Runtime-loss reporting is handled by Task Dispatch while the Session
// is owned by Worker Registry, so this small cross-module write is deliberately
// kept inside the same loss transaction: otherwise a crash between fencing the
// last Runtime and reopening the Consumer can strand the Worker forever in
// RECONCILING with no remaining work to reconcile.
func completeEmptyReconciliations(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `UPDATE workforce_worker_sessions s
SET session_state='REGISTERED',reconciliation_deadline=NULL
FROM workforce_workers w
WHERE w.current_instance_id=s.instance_id
  AND s.worker_id=w.id
  AND s.ended_at IS NULL
  AND s.session_state='RECONCILING'
  AND NOT EXISTS (
    SELECT 1 FROM workforce_attempt_runtimes r
    WHERE r.worker_id=w.id
      AND r.status IN ('AUTHORIZED','CONTAINER_CREATED','SUPERVISOR_READY','STARTING','RUNNING','RECOVERING','REATTACHING','RECONCILIATION_REQUIRED')
  )`)
	return err
}

func (s *Store) fenceRuntimeLoss(ctx context.Context, tx pgx.Tx, attemptID int64, reason string, evidenceDigest string) (domain.RuntimeLossResult, error) {
	var taskID, nodeID, assignmentID int64
	var taskStatus, runtimeStatus string
	var attemptCount, maxAttempts int
	var generation, owner int64
	err := tx.QueryRow(ctx, `SELECT a.task_id,a.execution_node_id,a.assignment_id,a.owner_epoch,r.status FROM workforce_task_attempts a JOIN workforce_attempt_runtimes r ON r.attempt_id=a.id WHERE a.id=$1 FOR UPDATE OF a,r`, attemptID).Scan(&taskID, &nodeID, &assignmentID, &owner, &runtimeStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RuntimeLossResult{}, application.ErrNotFound
	}
	if err != nil {
		return domain.RuntimeLossResult{}, err
	}
	err = tx.QueryRow(ctx, `SELECT t.status,t.attempt_count,n.max_attempts,n.generation FROM workforce_tasks t JOIN workforce_execution_nodes n ON n.task_id=t.id AND n.id=$2 WHERE t.id=$1 FOR UPDATE OF t,n`, taskID, nodeID).Scan(&taskStatus, &attemptCount, &maxAttempts, &generation)
	if err != nil {
		return domain.RuntimeLossResult{}, err
	}
	if taskStatus == "INTERACTION_SUSPENDED" || isTerminalRuntimeStatus(runtimeStatus) || (isTerminalTaskStatus(taskStatus) && taskStatus != "CANCELLED") {
		return domain.RuntimeLossResult{}, application.ErrNotClaimable
	}
	if reason == "REATTACH_DEADLINE_EXPIRED" {
		reason = domain.ReasonWorkerLostWithoutRuntime
	}
	newOwner := owner + 1
	if _, err = tx.Exec(ctx, `UPDATE workforce_attempt_runtimes SET status='LOST',owner_epoch=$2,execution_lease_expires_at=CURRENT_TIMESTAMP,
  reattach_permit_expires_at=CURRENT_TIMESTAMP,lost_at=CURRENT_TIMESTAMP,fenced_at=CURRENT_TIMESTAMP,terminal_reason_code=$3,updated_at=CURRENT_TIMESTAMP
WHERE attempt_id=$1`, attemptID, newOwner, reason); err != nil {
		return domain.RuntimeLossResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_task_attempts SET status='LOST',owner_epoch=$2,lease_expires_at=CURRENT_TIMESTAMP,
  lost_at=CURRENT_TIMESTAMP,retry_class='INFRASTRUCTURE',error_code=$3,finished_at=CURRENT_TIMESTAMP WHERE id=$1`, attemptID, newOwner, reason); err != nil {
		return domain.RuntimeLossResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_task_assignments SET status='COMPLETED',completed_at=CURRENT_TIMESTAMP,release_reason_code=$2 WHERE id=$1`, assignmentID, reason); err != nil {
		return domain.RuntimeLossResult{}, err
	}
	if taskStatus == "CANCELLING" || taskStatus == "CANCELLED" {
		if _, err = tx.Exec(ctx, `UPDATE workforce_task_attempts SET status='CANCELLED',retry_class='' WHERE id=$1`, attemptID); err != nil {
			return domain.RuntimeLossResult{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_attempt_runtimes SET status='CANCELLED' WHERE attempt_id=$1`, attemptID); err != nil {
			return domain.RuntimeLossResult{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='CANCELLED',updated_at=CURRENT_TIMESTAMP WHERE id=$1`, nodeID); err != nil {
			return domain.RuntimeLossResult{}, err
		}
		// The task cancellation was already committed; only fence and close
		// residual runtime resources, without a second task event or revision.
		if taskStatus == "CANCELLED" {
			return domain.RuntimeLossResult{Disposition: "CANCELLED", AttemptCount: attemptCount, MaxAttempts: maxAttempts, ReasonCode: reason}, nil
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='CANCELLED',finished_at=CURRENT_TIMESTAMP,version=version+1 WHERE id=$1`, taskID); err != nil {
			return domain.RuntimeLossResult{}, err
		}
		if err = appendEvent(ctx, tx, taskID, attemptID, "task.cancelled", map[string]any{"reasonCode": reason, "stage": "RUNTIME_LOSS"}); err != nil {
			return domain.RuntimeLossResult{}, err
		}
		return domain.RuntimeLossResult{Disposition: "CANCELLED", AttemptCount: attemptCount, MaxAttempts: maxAttempts, ReasonCode: reason}, nil
	}

	var frozen int
	if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM workforce_finalization_runs WHERE attempt_id=$1 AND status='FROZEN'`, attemptID).Scan(&frozen); err != nil {
		return domain.RuntimeLossResult{}, err
	}
	if frozen > 0 {
		if _, err = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='FAILED',updated_at=CURRENT_TIMESTAMP WHERE id=$1`, nodeID); err != nil {
			return domain.RuntimeLossResult{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='MANUAL_REVIEW',result_summary='FINALIZATION_SOURCE_LOST',version=version+1 WHERE id=$1`, taskID); err != nil {
			return domain.RuntimeLossResult{}, err
		}
		_ = appendEvent(ctx, tx, taskID, attemptID, "runtime.loss_manual_review", map[string]any{"reasonCode": "FINALIZATION_SOURCE_LOST"})
		return domain.RuntimeLossResult{Disposition: "MANUAL_REVIEW", AttemptCount: attemptCount, MaxAttempts: maxAttempts, ReasonCode: "FINALIZATION_SOURCE_LOST"}, nil
	}

	policy := domain.DefaultRecoveryPolicy()
	if maxAttempts > 0 {
		policy.MaxAgentAttempts = maxAttempts
	}
	// A retryable infrastructure reason does not authorize executing an Agent
	// twice. Preserve authoritative start/permit history even when recovery has
	// replaced the current status or cleared a superseded runtime permit.
	execution := domain.RuntimeLossExecutionEvidence{RuntimeStatus: runtimeStatus, TaskStatus: taskStatus}
	if err = tx.QueryRow(ctx, `SELECT
 (r.harness_started_at IS NOT NULL OR COALESCE(r.harness_pid,0)>0 OR EXISTS(SELECT 1 FROM workforce_task_events e WHERE e.attempt_id=r.attempt_id AND e.event_type='runtime.harness_started')),
 r.run_nonce_accepted_at IS NOT NULL,
 (r.run_permit_id IS NOT NULL OR r.run_permit_issued_at IS NOT NULL OR COALESCE(r.run_nonce_sha256,'')<>'' OR EXISTS(SELECT 1 FROM workforce_task_events e WHERE e.attempt_id=r.attempt_id AND e.event_type='runtime.run_permit_issued')),
 COALESCE(r.recovery_previous_status,'')
 FROM workforce_attempt_runtimes r WHERE r.attempt_id=$1`, attemptID).Scan(&execution.HarnessStarted, &execution.RunNonceAccepted, &execution.RunPermitIssued, &execution.PreviousRuntimeStatus); err != nil {
		return domain.RuntimeLossResult{}, err
	}
	if blockedReason := execution.ReplacementBlockedReason(); blockedReason != "" {
		if _, err = tx.Exec(ctx, `UPDATE workforce_task_attempts SET status='FAILED',finished_at=CURRENT_TIMESTAMP,retry_class='',error_code=$2 WHERE id=$1`, attemptID, blockedReason); err != nil {
			return domain.RuntimeLossResult{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='FAILED',next_retry_at=NULL,wait_reason_code='',last_retry_reason_code=$2,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, nodeID, blockedReason); err != nil {
			return domain.RuntimeLossResult{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='FAILED',finished_at=CURRENT_TIMESTAMP,result_summary=$2,version=version+1 WHERE id=$1`, taskID, blockedReason); err != nil {
			return domain.RuntimeLossResult{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_task_turns SET status='FAILED',finished_at=CURRENT_TIMESTAMP,result_summary=$2 WHERE id=(SELECT current_task_turn_id FROM workforce_tasks WHERE id=$1) AND task_id=$1`, taskID, blockedReason); err != nil {
			return domain.RuntimeLossResult{}, err
		}
		occurrence, appendErr := eventlog.AppendWithSequence(ctx, tx, taskID, attemptID, "task.failed", map[string]any{"reasonCode": blockedReason, "lossReasonCode": reason, "evidenceDigest": evidenceDigest, "harnessStarted": execution.HarnessStarted, "runNonceAccepted": execution.RunNonceAccepted, "runPermitIssued": execution.RunPermitIssued, "runtimeStatus": runtimeStatus, "previousRuntimeStatus": execution.PreviousRuntimeStatus})
		if appendErr != nil {
			return domain.RuntimeLossResult{}, appendErr
		}
		if s.notificationRecorder != nil {
			var tenantID, turnID int64
			if err = tx.QueryRow(ctx, `SELECT tenant_id,COALESCE(current_task_turn_id,0) FROM workforce_tasks WHERE id=$1`, taskID).Scan(&tenantID, &turnID); err != nil {
				return domain.RuntimeLossResult{}, err
			}
			_, err = s.notificationRecorder.RecordTransition(ctx, tx, tasknotification.TaskNotificationTransition{TenantID: tenantID, TaskID: taskID, TaskTurnID: turnID, SourceType: tasknotification.SourceRuntimeRecovery, SourceID: attemptID, SourceOccurrence: occurrence, PreviousStatus: taskStatus, CurrentStatus: "FAILED", ReasonCode: "RUNTIME_UNRECOVERABLE", Recoverability: tasknotification.RecoverabilityTerminal, OccurredAt: s.now().UTC()})
		}
		if err != nil {
			return domain.RuntimeLossResult{}, err
		}
		return domain.RuntimeLossResult{Disposition: "FAILED", AttemptCount: attemptCount, MaxAttempts: policy.MaxAgentAttempts, ReasonCode: blockedReason}, nil
	}
	if policy.CanCreateReplacement(attemptCount, reason) {
		nextGeneration := generation + 1
		nextRetry := s.now().UTC().Add(time.Second)
		if _, err = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='RETRY_WAIT',generation=$2,retry_count=retry_count+1,
  next_retry_at=$3,last_retry_reason_code=$4,wait_reason_code='ASSIGNMENT_RETRY_PENDING',updated_at=CURRENT_TIMESTAMP WHERE id=$1`, nodeID, nextGeneration, nextRetry, reason); err != nil {
			return domain.RuntimeLossResult{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='RETRY_WAIT',result_summary=$2,version=version+1 WHERE id=$1`, taskID, reason); err != nil {
			return domain.RuntimeLossResult{}, err
		}
		if err = appendEvent(ctx, tx, taskID, attemptID, "attempt.retry_scheduled", map[string]any{"reasonCode": reason, "nextGeneration": nextGeneration, "attemptCount": attemptCount, "maxAttempts": policy.MaxAgentAttempts, "evidenceDigest": evidenceDigest}); err != nil {
			return domain.RuntimeLossResult{}, err
		}
		return domain.RuntimeLossResult{Disposition: "RETRY_SCHEDULED", AttemptCount: attemptCount, MaxAttempts: policy.MaxAgentAttempts, NextGeneration: nextGeneration, ReasonCode: reason}, nil
	}

	if _, err = tx.Exec(ctx, `UPDATE workforce_task_attempts SET status='FAILED_RETRYABLE' WHERE id=$1`, attemptID); err != nil {
		return domain.RuntimeLossResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='FAILED',updated_at=CURRENT_TIMESTAMP WHERE id=$1`, nodeID); err != nil {
		return domain.RuntimeLossResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='FAILED',finished_at=CURRENT_TIMESTAMP,result_summary='AGENT_RETRY_EXHAUSTED',version=version+1 WHERE id=$1`, taskID); err != nil {
		return domain.RuntimeLossResult{}, err
	}
	if err = appendEvent(ctx, tx, taskID, attemptID, "attempt.retry_exhausted", map[string]any{"reasonCode": reason, "attemptCount": attemptCount, "maxAttempts": policy.MaxAgentAttempts}); err != nil {
		return domain.RuntimeLossResult{}, err
	}
	var occurrence int64
	if occurrence, err = eventlog.AppendWithSequence(ctx, tx, taskID, attemptID, "task.failed", map[string]any{"reasonCode": "AGENT_RETRY_EXHAUSTED"}); err != nil {
		return domain.RuntimeLossResult{}, err
	}
	if s.notificationRecorder != nil {
		var tenantID, turnID int64
		if err = tx.QueryRow(ctx, `SELECT tenant_id,COALESCE(current_task_turn_id,0) FROM workforce_tasks WHERE id=$1`, taskID).Scan(&tenantID, &turnID); err != nil {
			return domain.RuntimeLossResult{}, err
		}
		if _, err = s.notificationRecorder.RecordTransition(ctx, tx, tasknotification.TaskNotificationTransition{TenantID: tenantID, TaskID: taskID, TaskTurnID: turnID, SourceType: tasknotification.SourceTaskAttempt, SourceID: attemptID, SourceOccurrence: occurrence, PreviousStatus: "RETRY_WAIT", CurrentStatus: "FAILED", ReasonCode: "AGENT_RETRY_EXHAUSTED", Recoverability: tasknotification.RecoverabilityTerminal, RetryBudgetExhausted: true, OccurredAt: s.now().UTC()}); err != nil {
			return domain.RuntimeLossResult{}, err
		}
	}
	return domain.RuntimeLossResult{Disposition: "RETRY_EXHAUSTED", AttemptCount: attemptCount, MaxAttempts: policy.MaxAgentAttempts, ReasonCode: "AGENT_RETRY_EXHAUSTED"}, nil
}

func validRecoveryProgress(previous, observed string, harnessPID int) bool {
	if previous == "" || previous == observed {
		return true
	}
	switch previous {
	case domain.RuntimeAuthorized, domain.RuntimeContainerCreated, domain.RuntimeSupervisorReady:
		return observed == domain.RuntimeSupervisorReady || observed == domain.RuntimeStarting || observed == domain.RuntimeRunning
	case domain.RuntimeStarting:
		return observed == domain.RuntimeStarting || observed == domain.RuntimeRunning || (observed == domain.RuntimeStarting && harnessPID > 0)
	case domain.RuntimeRunning:
		return observed == domain.RuntimeRunning
	default:
		return false
	}
}

func isTerminalRuntimeStatus(status string) bool {
	switch status {
	case domain.RuntimeLost, domain.RuntimeFenced, domain.RuntimeSucceeded, domain.RuntimeFailed, domain.RuntimeCancelled, domain.RuntimeTimedOut, "INTERACTION_SUSPENDED":
		return true
	default:
		return false
	}
}

func isTerminalTaskStatus(status string) bool {
	switch status {
	case "SUCCEEDED", "FAILED", "CANCELLED", "TIMED_OUT", "MANUAL_REVIEW":
		return true
	default:
		return false
	}
}

func taskIDForAttempt(ctx context.Context, tx pgx.Tx, attemptID int64) int64 {
	var taskID int64
	_ = tx.QueryRow(ctx, `SELECT task_id FROM workforce_task_attempts WHERE id=$1`, attemptID).Scan(&taskID)
	return taskID
}

func deriveReattachSecret(lease string, permitID int64, ownerEpoch int64) string {
	mac := hmac.New(sha256.New, []byte(lease))
	_, _ = mac.Write([]byte(fmt.Sprintf("seven.reattach-permit/v1\x00%d\x00%d", permitID, ownerEpoch)))
	return hex.EncodeToString(mac.Sum(nil))
}

type runtimeAuthority struct {
	TaskID                                           int64
	NodeID                                           string
	WorkerID                                         int64
	InstanceID                                       string
	HostBootID                                       string
	Status, ContainerID, ImageDigest, IdentityDigest string
	Generation, OwnerEpoch                           int64
	LeaseHash                                        string
	LeaseExpires                                     time.Time
}

func loadRuntimeAuthority(ctx context.Context, tx pgx.Tx, attemptID int64) (runtimeAuthority, error) {
	var a runtimeAuthority
	// Native question/receipt traffic holds Task before Attempt/Runtime. Lease
	// renewal must use that order too, including when active time is reported.
	if err := tx.QueryRow(ctx, `SELECT t.id FROM workforce_tasks t JOIN workforce_task_attempts a ON a.task_id=t.id WHERE a.id=$1 FOR UPDATE OF t`, attemptID).Scan(&a.TaskID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return a, application.ErrNotFound
		}
		return a, err
	}
	err := tx.QueryRow(ctx, `SELECT a.task_id,a.execution_node_id,a.worker_id,a.worker_instance_id,a.host_boot_id,a.generation,a.owner_epoch,a.lease_token,a.lease_expires_at,r.status,COALESCE(r.container_id,''),r.harness_image_digest,r.execution_identity_digest FROM workforce_task_attempts a JOIN workforce_attempt_runtimes r ON r.attempt_id=a.id WHERE a.id=$1 FOR UPDATE OF a,r`, attemptID).Scan(&a.TaskID, &a.NodeID, &a.WorkerID, &a.InstanceID, &a.HostBootID, &a.Generation, &a.OwnerEpoch, &a.LeaseHash, &a.LeaseExpires, &a.Status, &a.ContainerID, &a.ImageDigest, &a.IdentityDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, application.ErrNotFound
	}
	return a, err
}

func (a runtimeAuthority) valid(e domain.MachineEnvelope, lease string) bool {
	return a.validAllowExpired(e, lease, 0)
}

func (a runtimeAuthority) validAllowExpired(e domain.MachineEnvelope, lease string, grace time.Duration) bool {
	if a.Status == domain.RuntimeRecovering || a.Status == domain.RuntimeReattaching || a.Status == domain.RuntimeReconciliationRequired || isTerminalRuntimeStatus(a.Status) {
		return false
	}
	return a.WorkerID == e.WorkerID && a.InstanceID == e.InstanceID && a.HostBootID == e.HostBootID && a.Generation == e.Generation && a.OwnerEpoch == e.OwnerEpoch && a.LeaseHash == hashToken(lease) && a.LeaseExpires.Add(grace).After(time.Now())
}

func labels(worker int64, instance, boot string, task, node, assignment, attempt int64, generation, owner int64, identity, image, hook string) map[string]string {
	return map[string]string{"seven.managed": "true", "seven.worker.id": strconv.FormatInt(worker, 10), "seven.worker.instance": instance, "seven.worker.host-boot-id": boot, "seven.task.id": strconv.FormatInt(task, 10), "seven.execution.node.id": strconv.FormatInt(node, 10), "seven.assignment.id": strconv.FormatInt(assignment, 10), "seven.attempt.id": strconv.FormatInt(attempt, 10), "seven.attempt.generation": fmt.Sprint(generation), "seven.owner.epoch": fmt.Sprint(owner), "seven.execution.identity.digest": identity, "seven.harness.image.digest": image, "seven.hook.configuration.digest": hook}
}
func identityDigest(values ...any) string {
	body, _ := json.Marshal(values)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
func randomToken() string { b := make([]byte, 32); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func deriveRunSecret(lease string, purpose string, permitID int64) string {
	mac := hmac.New(sha256.New, []byte(lease))
	_, _ = mac.Write([]byte(fmt.Sprintf("seven.run-permit/v1\x00%s\x00%d", purpose, permitID)))
	return hex.EncodeToString(mac.Sum(nil))
}
func deriveLaunchSecret(lease string, permitID int64) string {
	mac := hmac.New(sha256.New, []byte(lease))
	_, _ = mac.Write([]byte(fmt.Sprintf("seven.launch-permit/v1\x00%d", permitID)))
	return hex.EncodeToString(mac.Sum(nil))
}
func hashToken(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func mustJSON(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`null`)
	}
	return encoded
}
func appendEvent(ctx context.Context, tx pgx.Tx, taskID int64, attemptID int64, event string, payload any) error {
	return eventlog.Append(ctx, tx, taskID, attemptID, event, payload)
}

func (s *Store) RecordFacts(ctx context.Context, e domain.MachineEnvelope, attemptID int64, lease string, facts []domain.Fact) error {
	return s.recordFacts(ctx, e, attemptID, lease, facts, "WORKER")
}

// The authenticated journal-only Machine route selects the source. A Fact body
// cannot choose its producer namespace or acquire authority through its payload.
func (s *Store) RecordJournalFacts(ctx context.Context, e domain.MachineEnvelope, attemptID int64, lease string, facts []domain.Fact) error {
	return s.recordFacts(ctx, e, attemptID, lease, facts, "SUPERVISOR_JOURNAL")
}

func (s *Store) recordFacts(ctx context.Context, e domain.MachineEnvelope, attemptID int64, lease string, facts []domain.Fact, source string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var taskID, nodeID, workerID int64
	var instanceID, boot, storedLease string
	var generation, ownerEpoch int64
	var leaseExpiresAt time.Time
	if err = tx.QueryRow(ctx, `SELECT task_id,execution_node_id,worker_id,worker_instance_id,host_boot_id,lease_token,generation,owner_epoch,lease_expires_at FROM workforce_task_attempts WHERE id=$1 FOR UPDATE`, attemptID).Scan(&taskID, &nodeID, &workerID, &instanceID, &boot, &storedLease, &generation, &ownerEpoch, &leaseExpiresAt); errors.Is(err, pgx.ErrNoRows) {
		return application.ErrNotFound
	} else if err != nil {
		return err
	}
	if workerID != e.WorkerID || instanceID != e.InstanceID || boot != e.HostBootID || generation != e.Generation || ownerEpoch != e.OwnerEpoch || storedLease != hashToken(lease) || !leaseExpiresAt.After(time.Now()) {
		return application.ErrIdentityConflict
	}
	if source == "SUPERVISOR_JOURNAL" {
		var valid bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workforce_attempt_runtimes r JOIN workforce_runtime_reconciliation_runs c ON c.attempt_id=r.attempt_id WHERE r.attempt_id=$1 AND r.worker_id=$2 AND r.worker_instance_id=$3 AND r.host_boot_id=$4 AND r.generation=$5 AND r.owner_epoch=$6 AND r.status='RUNNING' AND c.new_owner_epoch=r.owner_epoch AND c.new_instance_id=r.worker_instance_id AND c.status='COMPLETED' AND c.supervisor_ack_sha256=r.authority_receipt_sha256)`, attemptID, e.WorkerID, e.InstanceID, e.HostBootID, e.Generation, e.OwnerEpoch).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return application.ErrIdentityConflict
		}
	}
	for _, fact := range facts {
		if fact.FactID == "" || fact.ProducerSequence < 1 || len(fact.Payload) > 256<<10 || !json.Valid(fact.Payload) {
			return application.ErrIdentityConflict
		}
		if source == "SUPERVISOR_JOURNAL" {
			if fact.FactID != fmt.Sprintf("jrn_%d_%d", attemptID, fact.ProducerSequence) || !domain.ReplayableJournalFact(fact.EventType) {
				return application.ErrIdentityConflict
			}
		} else if strings.HasPrefix(fact.FactID, "jrn_") {
			return application.ErrIdentityConflict
		}
		sum := sha256.Sum256(fact.Payload)
		actual := hex.EncodeToString(sum[:])
		if fact.PayloadDigest != "" && fact.PayloadDigest != actual {
			return application.ErrIdentityConflict
		}
		tag, insertErr := tx.Exec(ctx, `INSERT INTO workforce_attempt_facts (attempt_id,fact_id,producer_sequence,event_type,payload_sha256,producer_source) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, attemptID, fact.FactID, fact.ProducerSequence, fact.EventType, actual, source)
		if insertErr != nil {
			return insertErr
		}
		if tag.RowsAffected() == 0 {
			var stored string
			if err := tx.QueryRow(ctx, `SELECT payload_sha256 FROM workforce_attempt_facts WHERE attempt_id=$1 AND fact_id=$2 AND producer_source=$3 AND producer_sequence=$4 AND event_type=$5`, attemptID, fact.FactID, source, fact.ProducerSequence, fact.EventType).Scan(&stored); err != nil || stored != actual {
				return application.ErrIdentityConflict
			}
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(fact.Payload, &payload); err != nil {
			return err
		}
		switch fact.EventType {
		case "workspace.ready":
			base, _ := payload["baseCommit"].(string)
			if len(base) < 7 || len(base) > 128 {
				return application.ErrIdentityConflict
			}
			_, err = tx.Exec(ctx, `UPDATE workforce_tasks SET resolved_base_commit=$2 WHERE id=$1`, taskID, base)
			if err == nil {
				_, err = tx.Exec(ctx, `UPDATE workforce_task_attempts SET base_commit=$2 WHERE id=$1`, attemptID, base)
			}
		case "runtime.created":
			// Supervisor v2 makes container creation a first-class machine API
			// transition. Rejecting the legacy fact prevents an old Worker from
			// advancing a Task to RUNNING before the Harness has started.
			return application.ErrNotClaimable
		case "task.validating":
			_, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='VALIDATING',version=version+1 WHERE id=$1 AND status='RUNNING'`, taskID)
		case "task.packaging":
			_, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='PACKAGING',version=version+1 WHERE id=$1 AND status='VALIDATING'`, taskID)
		case "task.uploading":
			_, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='UPLOADING',version=version+1 WHERE id=$1 AND status='PACKAGING'`, taskID)
		case "assignment.delivery_acked":
			_, err = tx.Exec(ctx, `UPDATE workforce_task_assignments SET delivery_disposition='ACKED',delivery_acked_at=COALESCE(delivery_acked_at,CURRENT_TIMESTAMP) WHERE execution_node_id=$1 AND status='CLAIMED'`, nodeID)
		default:
			err = nil
		}
		if err != nil {
			return err
		}
		if err = appendEvent(ctx, tx, taskID, attemptID, fact.EventType, payload); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) MarkTerminal(ctx context.Context, attemptID int64, outcome string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var node, assignment int64
	if err = tx.QueryRow(ctx, `SELECT execution_node_id,assignment_id FROM workforce_task_attempts WHERE id=$1`, attemptID).Scan(&node, &assignment); err != nil {
		return err
	}
	nodeStatus := "FAILED"
	if outcome == "SUCCEEDED" {
		nodeStatus = "SUCCEEDED"
	} else if outcome == "CANCELLED" {
		nodeStatus = "CANCELLED"
	}
	_, err = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status=$2,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, node, nodeStatus)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE workforce_task_assignments SET status='COMPLETED',completed_at=CURRENT_TIMESTAMP WHERE id=$1`, assignment)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE workforce_attempt_runtimes SET status=$2,updated_at=CURRENT_TIMESTAMP WHERE attempt_id=$1`, attemptID, nodeStatus)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// FinalizeOutcome records the end of the Agent execution. A successful Agent
// does not make the Task successful: it creates exactly one FinalizationRun
// whose post-process pipeline is the only component allowed to promote the
// Task to SUCCEEDED.
func (s *Store) FinalizeOutcome(ctx context.Context, attemptID int64, input domain.Outcome) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var taskID, nodeID, assignmentID, taskTurnID, currentTurnID, tenantID int64
	var attemptStatus, taskStatus string
	if err = tx.QueryRow(ctx, `SELECT a.task_id,a.execution_node_id,a.assignment_id,a.status,t.status,a.task_turn_id,COALESCE(t.current_task_turn_id,0),t.tenant_id FROM workforce_task_attempts a JOIN workforce_tasks t ON t.id=a.task_id WHERE a.id=$1 FOR UPDATE OF a,t`, attemptID).Scan(&taskID, &nodeID, &assignmentID, &attemptStatus, &taskStatus, &taskTurnID, &currentTurnID, &tenantID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.ErrNotFound
		}
		return err
	}
	if taskTurnID == 0 || taskTurnID != currentTurnID {
		return application.ErrIdentityConflict
	}
	if taskStatus == "SUCCEEDED" || taskStatus == "FAILED" || taskStatus == "CANCELLED" || taskStatus == "TIMED_OUT" {
		if taskStatus == input.Outcome {
			return tx.Commit(ctx)
		}
		return application.ErrIdentityConflict
	}
	if taskStatus == "CANCELLING" && input.Outcome != "CANCELLED" {
		return application.ErrIdentityConflict
	}
	nodeStatus := input.Outcome
	eventType := "task." + strings.ToLower(input.Outcome)
	payload := map[string]any{}
	notificationReason := ""
	notificationSource := tasknotification.SourceTaskAttempt
	notificationSourceID := attemptID
	actorPrincipalID := int64(0)
	switch input.Outcome {
	case "EXECUTION_COMPLETED":
		if taskStatus != "VALIDATING" || (attemptStatus != "RUNNING" && attemptStatus != "EXECUTION_COMPLETED") {
			return application.ErrIdentityConflict
		}
		var proposalCount, passCycleCount int
		var proposalID, passCycleID int64
		var passContractDigest string
		var proposalRaw []byte
		if err = tx.QueryRow(ctx, `SELECT COUNT(*),COALESCE(MIN(id),0),COALESCE(MIN(raw_json::text),'') FROM workforce_completion_proposals WHERE attempt_id=$1 AND validation_status='ACCEPTED'`, attemptID).Scan(&proposalCount, &proposalID, &proposalRaw); err != nil {
			return err
		}
		// New Tasks are accepted by an immutable Result Processor cycle, not by
		// the legacy validation_commands_json/check_runs path.  There must be
		// exactly one persisted final PASS for the exact Attempt that owns the
		// accepted proposal.  Earlier REVISE cycles remain audit evidence only.
		if err = tx.QueryRow(ctx, `SELECT COUNT(*),COALESCE(MIN(id),0),COALESCE(MIN(contract_digest),'') FROM workforce_result_evaluation_cycles WHERE attempt_id=$1 AND decision='PASS' AND status='PERSISTED'`, attemptID).Scan(&passCycleCount, &passCycleID, &passContractDigest); err != nil {
			return err
		}
		if proposalCount != 1 || passCycleCount != 1 || passCycleID == 0 || passContractDigest == "" {
			return application.ErrIdentityConflict
		}
		identityInput, _ := json.Marshal(map[string]any{
			"attemptId":             attemptID,
			"proposalId":            proposalID,
			"proposal":              json.RawMessage(proposalRaw),
			"resultEvaluationCycle": passCycleID,
			"resultContractDigest":  passContractDigest,
			"generation":            input.Generation,
		})
		identitySum := sha256.Sum256(identityInput)
		inputIdentity := hex.EncodeToString(identitySum[:])
		finalizationID := s.idGen.NextID()
		messageID := s.idGen.NextID()
		message, _ := json.Marshal(finalizationWorkMessage{SchemaVersion: 1, Kind: "seven.finalization-run/v1", MessageID: messageID, WorkerID: input.WorkerID, ResourceID: finalizationID, Generation: input.Generation})
		if _, err = tx.Exec(ctx, `INSERT INTO workforce_finalization_runs (id,task_id,task_turn_id,execution_node_id,attempt_id,generation,input_identity_digest,completion_proposal_id,status,worker_id,worker_instance_id,host_boot_id)
VALUES ($1,$2,(SELECT task_turn_id FROM workforce_task_attempts WHERE id=$4),$3,$4,$5,$6,$7,'PENDING',$8,$9,$10)
ON CONFLICT (attempt_id,input_identity_digest) DO NOTHING`, finalizationID, taskID, nodeID, attemptID, input.Generation, inputIdentity, proposalID, input.WorkerID, input.WorkerInstanceID, input.HostBootID); err != nil {
			return err
		}
		var persistedRunID int64
		if err = tx.QueryRow(ctx, `SELECT id FROM workforce_finalization_runs WHERE attempt_id=$1 AND input_identity_digest=$2`, attemptID, inputIdentity).Scan(&persistedRunID); err != nil {
			return err
		}
		if persistedRunID == finalizationID {
			if _, err = tx.Exec(ctx, `INSERT INTO workforce_outbox_events (id,aggregate_type,aggregate_id,event_type,payload_json,status,exchange_name,routing_key,message_expiration_ms)
VALUES ($1,'finalization_run',$2,'finalization.requested',$3,'PENDING','seven.workforce.postprocess',$4,600000)`, messageID, finalizationID, message, fmt.Sprintf("postprocess.%d", input.WorkerID)); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_task_attempts SET status='EXECUTION_COMPLETED',execution_completed_at=COALESCE(execution_completed_at,CURRENT_TIMESTAMP),exit_code=0 WHERE id=$1 AND status IN ('RUNNING','EXECUTION_COMPLETED')`, attemptID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='PACKAGING',version=version+1 WHERE id=$1 AND status='VALIDATING'`, taskID); err != nil {
			return err
		}
		nodeStatus = "PACKAGING"
		eventType = "task.execution_completed"
		payload = map[string]any{"finalizationRunId": persistedRunID, "inputIdentityDigest": inputIdentity}
	case "FAILED":
		nodeStatus = "FAILED"
		message := input.Message
		if len(message) > 1000 {
			message = message[:1000]
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_task_attempts SET status='FAILED',finished_at=CURRENT_TIMESTAMP,error_code=$2,error_message=$3 WHERE id=$1`, attemptID, input.ReasonCode, message); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='FAILED',finished_at=CURRENT_TIMESTAMP,result_summary=$2,version=version+1 WHERE id=$1`, taskID, input.ReasonCode); err != nil {
			return err
		}
		payload = map[string]any{"code": input.ReasonCode, "message": message}
		notificationReason = terminalExecutionReason(input.ReasonCode)
	case "CANCELLED":
		nodeStatus = "CANCELLED"
		if taskStatus != "CANCELLING" {
			return application.ErrIdentityConflict
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_task_attempts SET status='CANCELLED',finished_at=CURRENT_TIMESTAMP WHERE id=$1`, attemptID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='CANCELLED',finished_at=CURRENT_TIMESTAMP,version=version+1 WHERE id=$1`, taskID); err != nil {
			return err
		}
		var creatorUserID, cancelUserID int64
		var principalType string
		if err = tx.QueryRow(ctx, `SELECT t.created_by,COALESCE(t.cancel_requested_by_principal_id,0),COALESCE(p.user_id,0),COALESCE(p.principal_type,'SERVICE') FROM workforce_tasks t LEFT JOIN workforce_principals p ON p.id=t.cancel_requested_by_principal_id WHERE t.id=$1`, taskID).Scan(&creatorUserID, &actorPrincipalID, &cancelUserID, &principalType); err != nil {
			return err
		}
		switch {
		case cancelUserID > 0 && cancelUserID == creatorUserID:
			notificationReason = "TASK_CANCELLED_BY_CREATOR"
		case principalType == "USER":
			notificationReason = "TASK_CANCELLED_BY_OPERATOR"
		default:
			notificationReason = "TASK_CANCELLED_BY_SYSTEM"
		}
		notificationSource = tasknotification.SourceTaskCancel
		var cancelRequestID int64
		if queryErr := tx.QueryRow(ctx, `SELECT COALESCE(cancel_request_id,0) FROM workforce_tasks WHERE id=$1`, taskID).Scan(&cancelRequestID); queryErr != nil {
			return queryErr
		}
		if cancelRequestID > 0 {
			notificationSourceID = cancelRequestID
		}
	case "TIMED_OUT":
		nodeStatus = "TIMED_OUT"
		if _, err = tx.Exec(ctx, `UPDATE workforce_task_attempts SET status='TIMED_OUT',timed_out_at=CURRENT_TIMESTAMP,timeout_stage=$2,finished_at=CURRENT_TIMESTAMP,error_code=$2 WHERE id=$1`, attemptID, input.ReasonCode); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='TIMED_OUT',finished_at=CURRENT_TIMESTAMP,result_summary=$2,version=version+1 WHERE id=$1`, taskID, input.ReasonCode); err != nil {
			return err
		}
		notificationReason = "TASK_TIMED_OUT"
	default:
		return application.ErrIdentityConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status=$2,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, nodeID, nodeStatus); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_task_assignments SET status='COMPLETED',completed_at=CURRENT_TIMESTAMP WHERE id=$1`, assignmentID); err != nil {
		return err
	}
	runtimeStatus := nodeStatus
	if input.Outcome == "EXECUTION_COMPLETED" {
		runtimeStatus = "SUCCEEDED"
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_attempt_runtimes SET status=$2,terminal_reason_code=CASE WHEN $3='' THEN terminal_reason_code ELSE $3 END,updated_at=CURRENT_TIMESTAMP WHERE attempt_id=$1`, attemptID, runtimeStatus, input.ReasonCode); err != nil {
		return err
	}
	occurrence, err := eventlog.AppendWithSequence(ctx, tx, taskID, attemptID, eventType, payload)
	if err != nil {
		return err
	}
	if notificationReason != "" && s.notificationRecorder != nil {
		if _, err = s.notificationRecorder.RecordTransition(ctx, tx, tasknotification.TaskNotificationTransition{TenantID: tenantID, TaskID: taskID, TaskTurnID: taskTurnID, SourceType: notificationSource, SourceID: notificationSourceID, SourceOccurrence: occurrence, PreviousStatus: taskStatus, CurrentStatus: input.Outcome, ReasonCode: notificationReason, ActorPrincipalID: actorPrincipalID, Recoverability: tasknotification.RecoverabilityTerminal, OccurredAt: s.now().UTC()}); err != nil {
			return err
		}
	}
	_ = attemptStatus
	return tx.Commit(ctx)
}

func terminalExecutionReason(reason string) string {
	switch strings.TrimSpace(reason) {
	case "RUNTIME_UNRECOVERABLE", "QUEUE_TIMEOUT", "SUPERVISOR_STARTUP_TIMEOUT":
		return strings.TrimSpace(reason)
	default:
		return "TASK_FAILED"
	}
}

type AttemptContext struct {
	TaskID                 int64
	TaskTurnID             int64
	TenantID               int64
	ModelCredentialName    string
	PrimaryModel           string
	WorkerID               int64
	InstanceID             string
	HostBootID             string
	Lease                  string
	Generation, OwnerEpoch int64
	LeaseExpiresAt         time.Time
}

func (v AttemptContext) Authorizes(e domain.MachineEnvelope, lease string, now time.Time) bool {
	return v.WorkerID == e.WorkerID && v.InstanceID == e.InstanceID && v.HostBootID == e.HostBootID &&
		v.Generation == e.Generation && v.OwnerEpoch == e.OwnerEpoch && v.Lease == hashToken(lease) && v.LeaseExpiresAt.After(now)
}

func (s *Store) AttemptContext(ctx context.Context, attemptID int64) (AttemptContext, error) {
	var v AttemptContext
	err := s.db.QueryRow(ctx, `SELECT a.task_id,a.task_turn_id,t.tenant_id,
COALESCE(mc.name,''),COALESCE(a.resolved_execution_snapshot_json->>'primaryModel',''),
a.worker_id,a.worker_instance_id,a.host_boot_id,a.lease_token,a.generation,a.owner_epoch,a.lease_expires_at
FROM workforce_task_attempts a
JOIN workforce_tasks t ON t.id=a.task_id
LEFT JOIN workforce_model_credentials mc ON mc.id=COALESCE((a.resolved_execution_snapshot_json->>'modelCredentialId')::BIGINT,0) AND mc.tenant_id=t.tenant_id
WHERE a.id=$1`, attemptID).Scan(&v.TaskID, &v.TaskTurnID, &v.TenantID, &v.ModelCredentialName, &v.PrimaryModel, &v.WorkerID, &v.InstanceID, &v.HostBootID, &v.Lease, &v.Generation, &v.OwnerEpoch, &v.LeaseExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, application.ErrNotFound
	}
	return v, err
}
