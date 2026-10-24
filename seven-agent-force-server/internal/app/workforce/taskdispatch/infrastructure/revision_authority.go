package infrastructure

import (
	"context"
	"strconv"
	"time"

	completion "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/checkpoint"
	"github.com/jackc/pgx/v5"
)

type revisionSafePointAuthority struct {
	lookup completion.SafePointRevisionLookup
}

// WithRevisionLookup is a startup-only dependency binding, before serving calls.
// CompletionCheck owns revision persistence; dispatch never reads its tables.
func (a *revisionSafePointAuthority) WithRevisionLookup(lookup completion.SafePointRevisionLookup) *revisionSafePointAuthority {
	a.lookup = lookup
	return a
}

// NewRevisionSafePointAuthority validates the authenticated Worker finish hook,
// not an Adapter frame. The hook runs only after Run has returned, stopped the
// native process and frozen/read back the genuine RESULT checkpoint. AVAILABLE
// checkpoint metadata alone must never be exposed as a container apply API.
func NewRevisionSafePointAuthority() *revisionSafePointAuthority {
	return &revisionSafePointAuthority{}
}

func (a *revisionSafePointAuthority) ValidateRevisionSafePoint(ctx context.Context, tx pgx.Tx, p completion.RevisionSafePoint) error {
	if p.RevisionID <= 0 || p.ExpectedVersion <= 0 {
		return completion.ErrStale
	}
	return a.validate(ctx, tx, p)
}

func (a *revisionSafePointAuthority) validate(ctx context.Context, tx pgx.Tx, p completion.RevisionSafePoint) error {
	m := p.Authority
	if tx == nil || p.TenantID <= 0 || p.TaskID <= 0 || p.TaskTurnID <= 0 || p.AttemptID <= 0 || p.CheckpointID <= 0 || m.WorkerID <= 0 || m.InstanceID == "" || m.HostBootID == "" || m.Generation <= 0 || m.OwnerEpoch <= 0 || p.ExecutionLease == "" {
		return completion.ErrStale
	}
	var taskStatus string
	var current int64
	var cancelled bool
	if err := tx.QueryRow(ctx, `SELECT current_task_turn_id,status,cancel_requested_at IS NOT NULL FROM workforce_tasks WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, p.TaskID, p.TenantID).Scan(&current, &taskStatus, &cancelled); err != nil || cancelled {
		return completion.ErrStale
	}
	switch taskStatus {
	case "QUEUED", "PREPARING", "RUNNING", "VALIDATING", "REVIEWING":
	default:
		return completion.ErrStale
	}
	var attemptStatus, runtimeStatus string
	var matches bool
	// The expired/old-owner runtime is not implicitly promoted by a new Worker.
	if err := tx.QueryRow(ctx, `SELECT a.status,r.status,
 a.task_id=$2 AND a.task_turn_id=$3 AND a.worker_id=$4 AND a.worker_instance_id=$5 AND a.host_boot_id=$6 AND a.generation=$7 AND a.owner_epoch=$8 AND a.lease_token=$9 AND a.lease_expires_at>clock_timestamp()
 AND r.worker_id=a.worker_id AND r.worker_instance_id=a.worker_instance_id AND r.host_boot_id=a.host_boot_id AND r.generation=a.generation AND r.owner_epoch=a.owner_epoch AND r.execution_lease_sha256=a.lease_token AND r.execution_lease_expires_at>clock_timestamp()
 AND r.fenced_at IS NULL AND r.lost_at IS NULL AND r.terminating_at IS NULL
 FROM workforce_task_attempts a JOIN workforce_attempt_runtimes r ON r.attempt_id=a.id WHERE a.id=$1 AND NOT EXISTS(SELECT 1 FROM workforce_task_attempts newer WHERE newer.task_id=a.task_id AND newer.task_turn_id=a.task_turn_id AND newer.id>a.id) FOR UPDATE OF a,r`, p.AttemptID, p.TaskID, p.TaskTurnID, m.WorkerID, m.InstanceID, m.HostBootID, m.Generation, m.OwnerEpoch, hashToken(p.ExecutionLease)).Scan(&attemptStatus, &runtimeStatus, &matches); err != nil || !matches {
		return completion.ErrStale
	}
	var instance, boot string
	if err := tx.QueryRow(ctx, `SELECT s.instance_id,s.host_boot_id FROM workforce_workers w JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id AND s.worker_id=w.id WHERE w.id=$1 AND w.desired_state IN ('ENABLED','DRAINING') AND w.decommissioned_at IS NULL AND s.ended_at IS NULL AND s.session_state='REGISTERED' FOR SHARE OF w,s`, m.WorkerID).Scan(&instance, &boot); err != nil || instance != m.InstanceID || boot != m.HostBootID {
		return completion.ErrStale
	}
	var cycle, consumed int64
	var cycleNumber int
	var decision, session, identity, bundle, overlay, sessionKey, manifest string
	var expires time.Time
	if err := tx.QueryRow(ctx, `SELECT c.evaluation_cycle_id,c.decision,e.session_digest,c.checkpoint_identity_sha256,c.repository_bundle_object_key,c.workspace_overlay_object_key,c.session_object_key,c.identity_manifest_object_key,c.expires_at,COALESCE(c.consumed_by_turn_id,0),e.cycle_number
 FROM workforce_continuation_checkpoints c JOIN workforce_result_evaluation_cycles e ON e.id=c.evaluation_cycle_id AND e.task_id=c.task_id AND e.task_turn_id=c.task_turn_id AND e.attempt_id=c.attempt_id
 WHERE c.id=$1 AND c.task_id=$2 AND c.task_turn_id=$3 AND c.attempt_id=$4 AND c.purpose='RESULT' AND c.interaction_id IS NULL AND c.format_version=2 AND c.status='AVAILABLE' AND c.available_at IS NOT NULL AND ((c.consumed_by_turn_id IS NULL AND c.consumed_at IS NULL) OR (c.consumed_by_turn_id IS NOT NULL AND c.consumed_at IS NOT NULL))
 AND c.decision IN ('PASS','WAIT_USER') AND e.decision=c.decision AND e.status='PERSISTED' AND e.persisted_at IS NOT NULL AND e.completed_at IS NOT NULL
 AND c.encryption_algorithm='AES-256-GCM-CHUNKED' AND c.encryption_key_id<>'' AND c.wrapped_data_key<>''
 AND c.repository_bundle_object_sha256<>'' AND c.workspace_overlay_object_sha256<>'' AND c.session_object_sha256<>'' AND c.manifest_object_sha256<>'' AND c.task_execution_snapshot_digest<>''
 AND EXISTS(SELECT 1 FROM workforce_task_turns tr WHERE tr.id=c.task_turn_id AND tr.task_id=c.task_id AND tr.tenant_id=$5 AND tr.execution_snapshot_digest=c.task_execution_snapshot_digest)
 FOR UPDATE OF c,e`, p.CheckpointID, p.TaskID, p.TaskTurnID, p.AttemptID, p.TenantID).Scan(&cycle, &decision, &session, &identity, &bundle, &overlay, &sessionKey, &manifest, &expires, &consumed, &cycleNumber); err != nil || !expires.After(time.Now()) {
		return completion.ErrStale
	}
	// The Task and checkpoint/cycle locks remain held across this same-tx
	// latest-cycle check; the original cycle_number ordering is unchanged.
	var newerCycle bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workforce_result_evaluation_cycles WHERE attempt_id=$1 AND cycle_number>$2)`, p.AttemptID, cycleNumber).Scan(&newerCycle); err != nil || newerCycle {
		return completion.ErrStale
	}
	if consumed > 0 {
		if current != consumed || attemptStatus != "SUPERSEDED" || runtimeStatus != "SUCCEEDED" {
			return completion.ErrStale
		}
		var key string
		if err := tx.QueryRow(ctx, `SELECT idempotency_key FROM workforce_task_turns WHERE id=$1 AND task_id=$2 AND tenant_id=$3 AND source_turn_id=$4 AND source_checkpoint_id=$5 FOR SHARE`, consumed, p.TaskID, p.TenantID, p.TaskTurnID, p.CheckpointID).Scan(&key); err != nil || (p.RevisionID > 0 && key != "completion-revision:"+strconv.FormatInt(p.RevisionID, 10)) {
			return completion.ErrStale
		}
	} else if current != p.TaskTurnID {
		return completion.ErrStale
	} else if decision == "PASS" {
		if taskStatus != "VALIDATING" || attemptStatus != "RUNNING" || runtimeStatus != "RUNNING" {
			return completion.ErrStale
		}
	} else if taskStatus != "REVIEWING" || attemptStatus != "WAITING_USER" || runtimeStatus != "SUCCEEDED" {
		return completion.ErrStale
	}
	in := checkpoint.Input{TaskID: p.TaskID, TaskTurnID: p.TaskTurnID, AttemptID: p.AttemptID, Generation: m.Generation, EvaluationCycleID: cycle, Decision: decision, SessionDigest: session}
	expected := checkpoint.Identity(in)
	b, o, s, f := checkpoint.ObjectKeysV2(in, expected)
	if identity != expected || bundle != b || overlay != o || sessionKey != s || manifest != f {
		return completion.ErrStale
	}
	return nil
}

func (a *revisionSafePointAuthority) ResolveRevisionSafePoint(ctx context.Context, tx pgx.Tx, m completion.MachineAuthority, attemptID int64, lease string, cycleID int64) (*completion.RevisionSafePoint, error) {
	if tx == nil || attemptID <= 0 || cycleID <= 0 {
		return nil, completion.ErrStale
	}
	p := completion.RevisionSafePoint{AttemptID: attemptID, Authority: m, ExecutionLease: lease}
	// Discover and lock the Task first; these identifiers confer no authority
	// until validate checks both persisted machine bindings and latest cycle.
	if err := tx.QueryRow(ctx, `SELECT t.tenant_id,t.id,a.task_turn_id FROM workforce_tasks t JOIN workforce_task_attempts a ON a.task_id=t.id WHERE a.id=$1 FOR UPDATE OF t`, attemptID).Scan(&p.TenantID, &p.TaskID, &p.TaskTurnID); err != nil {
		return nil, completion.ErrStale
	}
	var consumed int64
	if err := tx.QueryRow(ctx, `SELECT id,COALESCE(consumed_by_turn_id,0) FROM workforce_continuation_checkpoints WHERE attempt_id=$1 AND task_turn_id=$2 AND task_id=$3 AND evaluation_cycle_id=$4 AND purpose='RESULT'`, attemptID, p.TaskTurnID, p.TaskID, cycleID).Scan(&p.CheckpointID, &consumed); err != nil {
		return nil, completion.ErrStale
	}
	if err := a.validate(ctx, tx, p); err != nil {
		return nil, err
	}
	if a.lookup == nil {
		return nil, completion.ErrRevisionUnavailable
	}
	r, err := a.lookup.FindSafePointRevisionTx(ctx, tx, p.TenantID, p.TaskID, p.TaskTurnID, p.AttemptID)
	if err == nil && r == nil && consumed > 0 {
		return nil, completion.ErrStale
	}
	if err != nil || r == nil {
		return nil, err
	}
	if r.TenantID != p.TenantID || r.TaskID != p.TaskID || r.SourceTaskTurnID != p.TaskTurnID || r.SourceAttemptID != p.AttemptID {
		return nil, completion.ErrStale
	}
	p.RevisionID, p.ExpectedVersion = r.ID, r.Version
	if r.Status == "APPLIED" {
		if r.AppliedTurnID != consumed || consumed <= 0 {
			return nil, completion.ErrStale
		}
		p.ExpectedVersion--
	} else if r.Status != "PENDING" || consumed != 0 {
		return nil, completion.ErrStale
	}
	if err = a.ValidateRevisionSafePoint(ctx, tx, p); err != nil {
		return nil, err
	}
	return &p, nil
}
