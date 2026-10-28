package infrastructure

import (
	"context"
	"time"

	executionaccessdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/domain"
	executionaccess "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/facade"
	interaction "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	principal "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/facade"
	registry "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/facade"
	"github.com/jackc/pgx/v5"
)

// executionAccessAuthority deliberately reuses the exact Task/Attempt/Worker
// fencing implementation used by persistent interactions. It adapts contracts;
// it does not introduce a second interpretation of owner, lease, or session.
type executionAccessAuthority struct{ base *interactionAuthority }

func NewExecutionAccessAuthority(ids principal.IDGenerator, principals principal.Resolver, images registry.AdapterCapabilityReader) *executionAccessAuthority {
	return &executionAccessAuthority{base: &interactionAuthority{ids: ids, principals: principals, images: images}}
}

func (a *executionAccessAuthority) AuthorizeUser(ctx context.Context, tx pgx.Tx, u executionaccess.User, action string) (executionaccess.Target, error) {
	// Resolve the exact Task/Attempt/session using the shared read authority,
	// then apply the execution-access mutation contract here. Network access is
	// deliberately independent from interaction.respond: a task may support a
	// Dynamic Tool without supporting native question answering (and vice versa).
	t, err := a.base.AuthorizeUser(ctx, tx, interaction.User{TenantID: u.TenantID, TaskID: u.TaskID, UserID: u.UserID, CanContinue: u.CanContinue}, interaction.Read)
	if err != nil {
		return executionaccess.Target{}, err
	}
	if action != executionaccess.ActionRead {
		if !u.CanContinue || t.AttemptID <= 0 || t.AdapterSessionGeneration <= 0 || !containsCapability(t.Capabilities, "tool.request") || !containsCapability(t.Capabilities, "tool.result") {
			return executionaccess.Target{}, executionaccessdomain.ErrActionNotAllowed
		}
	}
	return executionaccess.Target{TenantID: t.TenantID, TaskID: t.TaskID, TaskTurnID: t.TaskTurnID, AttemptID: t.AttemptID, AdapterSessionGeneration: t.AdapterSessionGeneration, PrincipalID: t.PrincipalID, Capabilities: append([]string(nil), t.Capabilities...)}, nil
}

func (a *executionAccessAuthority) ValidateMachine(ctx context.Context, tx pgx.Tx, m executionaccess.Machine, action string) (executionaccess.Target, error) {
	if m.ColdResumeOperationID > 0 {
		t, err := a.validateColdResume(ctx, tx, m)
		if err != nil {
			return t, err
		}
		return t, checkPlanningOperation(ctx, tx, t.AttemptID, action)
	}
	t, err := a.base.Validate(ctx, tx, interaction.Machine{TenantID: m.TenantID, TaskID: m.TaskID, TaskTurnID: m.TaskTurnID, WorkerID: m.WorkerID, InstanceID: m.InstanceID, HostBootID: m.HostBootID, AttemptID: m.AttemptID, Generation: m.Generation, OwnerEpoch: m.OwnerEpoch, AdapterSessionGeneration: m.AdapterSessionGeneration, ExecutionLease: m.ExecutionLease})
	if err != nil {
		return executionaccess.Target{}, err
	}
	target := executionaccess.Target{TenantID: t.TenantID, TaskID: t.TaskID, TaskTurnID: t.TaskTurnID, AttemptID: t.AttemptID, AdapterSessionGeneration: t.AdapterSessionGeneration, PrincipalID: t.PrincipalID, WorkerID: m.WorkerID, OwnerEpoch: m.OwnerEpoch, Capabilities: append([]string(nil), t.Capabilities...)}
	return target, checkPlanningOperation(ctx, tx, target.AttemptID, action)
}

// The frozen server-owned attempt snapshot, not a tool list supplied by an
// adapter, decides whether a planning task may request an external operation.
func checkPlanningOperation(ctx context.Context, tx pgx.Tx, attemptID int64, action string) error {
	if action != executionaccess.ActionPublish {
		return nil
	}
	var purpose string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(resolved_execution_snapshot_json->>'stagePurpose','') FROM workforce_task_attempts WHERE id=$1`, attemptID).Scan(&purpose); err != nil {
		return err
	}
	if purpose == "PLAN" {
		return executionaccessdomain.ErrActionNotAllowed
	}
	return nil
}

func (a *executionAccessAuthority) validateColdResume(ctx context.Context, tx pgx.Tx, m executionaccess.Machine) (executionaccess.Target, error) {
	var target executionaccess.Target
	var currentTurn, worker, generation, owner, sourceOperation, originalAdapterGeneration int64
	var instance, boot, lease, taskStatus, attemptStatus, runtimeStatus, imageDigest string
	var expiry time.Time
	var cancelled bool
	// First lock the current Task, Attempt, and runtime authority. The consumed
	// checkpoint and target Turn are verified in a second, shared-lock read so
	// each logical read stays within the three-source governance boundary.
	err := tx.QueryRow(ctx, `SELECT t.id,t.tenant_id,t.current_task_turn_id,t.status,t.cancel_requested_at IS NOT NULL,
		a.task_turn_id,a.worker_id,a.worker_instance_id,a.host_boot_id,a.generation,a.owner_epoch,a.lease_token,a.lease_expires_at,a.status,
		r.status,r.harness_image_digest
		FROM workforce_tasks t
		JOIN workforce_task_attempts a ON a.id=$1 AND a.task_id=t.id AND a.task_turn_id=t.current_task_turn_id AND t.latest_attempt_id=a.id
		JOIN workforce_attempt_runtimes r ON r.attempt_id=a.id
		FOR UPDATE OF t,a,r`, m.AttemptID).Scan(
		&target.TaskID, &target.TenantID, &currentTurn, &taskStatus, &cancelled,
		&target.TaskTurnID, &worker, &instance, &boot, &generation, &owner, &lease, &expiry, &attemptStatus,
		&runtimeStatus, &imageDigest)
	if err != nil || cancelled || target.TaskTurnID != currentTurn || worker != m.WorkerID || instance != m.InstanceID || boot != m.HostBootID || generation != m.Generation || owner != m.OwnerEpoch || lease != hashToken(m.ExecutionLease) || !expiry.After(time.Now()) || (attemptStatus != "PREPARING" && attemptStatus != "RUNNING") || (runtimeStatus != "PREPARING" && runtimeStatus != "RUNNING") || (taskStatus != "PREPARING" && taskStatus != "RUNNING") {
		return target, executionaccessdomain.ErrFenced
	}
	// No generic Operation endpoint accepts this authority. The exact successor
	// Turn, consumed checkpoint, and source Operation must still agree while
	// held under shared locks before the old Adapter identity is reused.
	err = tx.QueryRow(ctx, `SELECT tr.source_execution_operation_id,o.adapter_session_generation
		FROM workforce_task_turns tr
		JOIN workforce_continuation_checkpoints c ON c.id=tr.source_checkpoint_id AND c.consumed_by_turn_id=tr.id AND c.task_id=tr.task_id
		JOIN workforce_execution_operations o ON o.id=tr.source_execution_operation_id AND o.id=c.operation_id AND o.task_id=tr.task_id AND o.tenant_id=tr.tenant_id
		WHERE tr.id=$1 AND tr.task_id=$2 AND tr.tenant_id=$3 AND tr.source_execution_operation_id=$4
		AND c.purpose='INTERACTION_SUSPEND' AND c.status='AVAILABLE' AND c.interaction_phase='RESUMED' AND c.expires_at>CURRENT_TIMESTAMP
		FOR SHARE OF tr,c,o`, target.TaskTurnID, target.TaskID, target.TenantID, m.ColdResumeOperationID).Scan(&sourceOperation, &originalAdapterGeneration)
	if err != nil || sourceOperation != m.ColdResumeOperationID || originalAdapterGeneration != m.AdapterSessionGeneration {
		return target, executionaccessdomain.ErrFenced
	}
	var runtimeMatches bool
	if err = tx.QueryRow(ctx, `SELECT worker_id=$2 AND worker_instance_id=$3 AND host_boot_id=$4 AND generation=$5 AND owner_epoch=$6 AND execution_lease_sha256=$7 AND execution_lease_expires_at>CURRENT_TIMESTAMP AND terminating_at IS NULL AND lost_at IS NULL AND fenced_at IS NULL FROM workforce_attempt_runtimes WHERE attempt_id=$1`, m.AttemptID, worker, instance, boot, generation, owner, lease).Scan(&runtimeMatches); err != nil || !runtimeMatches {
		return target, executionaccessdomain.ErrFenced
	}
	var currentInstance, currentBoot string
	if err = tx.QueryRow(ctx, `SELECT w.current_instance_id,s.host_boot_id FROM workforce_workers w JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id AND s.worker_id=w.id WHERE w.id=$1 AND w.decommissioned_at IS NULL AND w.desired_state IN ('ENABLED','DRAINING') AND s.ended_at IS NULL AND s.session_state='REGISTERED' FOR SHARE OF w,s`, worker).Scan(&currentInstance, &currentBoot); err != nil || currentInstance != instance || currentBoot != boot {
		return target, executionaccessdomain.ErrFenced
	}
	if a.base.images == nil {
		return target, executionaccessdomain.ErrUnavailable
	}
	target.Capabilities, err = a.base.images.AdapterCapabilities(ctx, tx, worker, instance, imageDigest)
	if err != nil || !containsCapability(target.Capabilities, "tool.request") || !containsCapability(target.Capabilities, "tool.result") {
		return target, executionaccessdomain.ErrFenced
	}
	resolver, ok := a.base.principals.(principal.WorkerResolver)
	if !ok {
		return target, executionaccessdomain.ErrUnavailable
	}
	target.PrincipalID, err = resolver.EnsureWorker(ctx, tx, a.base.ids, worker)
	target.AttemptID, target.AdapterSessionGeneration = m.AttemptID, originalAdapterGeneration
	target.WorkerID, target.OwnerEpoch, target.SourceOperationID = worker, owner, sourceOperation
	return target, err
}

func containsCapability(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
