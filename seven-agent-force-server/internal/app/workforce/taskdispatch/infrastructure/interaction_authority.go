package infrastructure

import (
	"context"
	"errors"
	interactiondomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	interaction "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	principal "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/facade"
	dispatchfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/facade"
	registry "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/facade"
	"github.com/jackc/pgx/v5"
	"time"
)

type interactionAuthority struct {
	ids        principal.IDGenerator
	principals principal.Resolver
	images     registry.AdapterCapabilityReader
}

func NewInteractionAuthority(ids principal.IDGenerator, principals principal.Resolver, images registry.AdapterCapabilityReader) dispatchfacade.InteractionAuthority {
	return &interactionAuthority{ids: ids, principals: principals, images: images}
}

func (a *interactionAuthority) AuthorizeSummary(ctx context.Context, tx pgx.Tx, tenant, user int64, ids []int64) (interaction.SummaryScope, error) {
	var scope interaction.SummaryScope
	if tenant <= 0 || user <= 0 || len(ids) > 200 {
		return scope, interactiondomain.ErrInvalid
	}
	for _, id := range ids {
		if id <= 0 {
			return scope, interactiondomain.ErrInvalid
		}
	}
	var err error
	scope.PrincipalID, err = a.principals.EnsureUser(ctx, tx, a.ids, user)
	if err != nil {
		return scope, err
	}
	rows, err := tx.Query(ctx, `SELECT id,status FROM workforce_tasks WHERE tenant_id=$1 AND id=ANY($2::bigint[])`, tenant, ids)
	if err != nil {
		return scope, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var status string
		if err = rows.Scan(&id, &status); err != nil {
			return scope, err
		}
		scope.TaskIDs = append(scope.TaskIDs, id)
		if status == "RUNNING" || status == "QUEUED" || status == "INTERACTION_SUSPENDED" || status == "REVIEWING" {
			scope.ActiveTaskIDs = append(scope.ActiveTaskIDs, id)
		}
	}
	return scope, rows.Err()
}

func (a *interactionAuthority) Validate(ctx context.Context, tx pgx.Tx, m interaction.Machine) (interaction.Target, error) {
	var target interaction.Target
	if m.WorkerID <= 0 || m.AttemptID <= 0 || m.AdapterSessionGeneration <= 0 || m.ExecutionLease == "" {
		return target, interactiondomain.ErrFenced
	}
	var current, worker, generation, owner, adapterGeneration int64
	var instance, boot, lease, taskStatus, attemptStatus, runtimeStatus string
	var expiry time.Time
	var cancelled bool
	// Lock the task before runtime/attempt, matching cancel and response order.
	err := tx.QueryRow(ctx, `SELECT t.id,t.tenant_id,COALESCE(t.current_task_turn_id,0),t.status,t.cancel_requested_at IS NOT NULL FROM workforce_tasks t JOIN workforce_task_attempts a ON a.task_id=t.id WHERE a.id=$1 FOR UPDATE OF t`, m.AttemptID).Scan(&target.TaskID, &target.TenantID, &current, &taskStatus, &cancelled)
	if err != nil {
		return target, interactiondomain.ErrFenced
	}
	err = tx.QueryRow(ctx, `SELECT a.task_turn_id,a.worker_id,a.worker_instance_id,a.host_boot_id,a.generation,a.owner_epoch,a.lease_token,a.lease_expires_at,a.status,r.status,COALESCE(r.adapter_session_generation,0) FROM workforce_task_attempts a JOIN workforce_attempt_runtimes r ON r.attempt_id=a.id WHERE a.id=$1 FOR UPDATE OF a,r`, m.AttemptID).Scan(&target.TaskTurnID, &worker, &instance, &boot, &generation, &owner, &lease, &expiry, &attemptStatus, &runtimeStatus, &adapterGeneration)
	if err != nil {
		return target, interactiondomain.ErrFenced
	}
	// VALIDATING is the persisted evaluation phase; REVIEWING may be entered
	// before the Worker drains final observations. Neither authorizes new input.
	if cancelled || target.TenantID <= 0 || current <= 0 || target.TaskTurnID != current || worker != m.WorkerID || instance != m.InstanceID || boot != m.HostBootID || generation != m.Generation || owner != m.OwnerEpoch || lease != hashToken(m.ExecutionLease) || !expiry.After(time.Now()) || attemptStatus != "RUNNING" || runtimeStatus != "RUNNING" || (taskStatus != "RUNNING" && taskStatus != "VALIDATING" && taskStatus != "REVIEWING") || (adapterGeneration != 0 && adapterGeneration != m.AdapterSessionGeneration) {
		return target, interactiondomain.ErrFenced
	}
	if (m.TenantID != 0 && m.TenantID != target.TenantID) || (m.TaskID != 0 && m.TaskID != target.TaskID) || (m.TaskTurnID != 0 && m.TaskTurnID != target.TaskTurnID) {
		return target, interactiondomain.ErrFenced
	}
	// Both persisted copies of execution authority must agree. Do not implicitly
	// upgrade an old runtime after an Attempt owner/session/lease changes.
	var runtimeMatches bool
	err = tx.QueryRow(ctx, `SELECT worker_id=$2 AND worker_instance_id=$3 AND host_boot_id=$4 AND generation=$5 AND owner_epoch=$6 AND execution_lease_sha256=$7 AND execution_lease_expires_at>CURRENT_TIMESTAMP AND terminating_at IS NULL AND lost_at IS NULL AND fenced_at IS NULL FROM workforce_attempt_runtimes WHERE attempt_id=$1`, m.AttemptID, worker, instance, boot, generation, owner, lease).Scan(&runtimeMatches)
	if err != nil || !runtimeMatches {
		return target, interactiondomain.ErrFenced
	}
	var currentInstance, currentBoot string
	err = tx.QueryRow(ctx, `SELECT w.current_instance_id,s.host_boot_id FROM workforce_workers w JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id AND s.worker_id=w.id WHERE w.id=$1 AND w.decommissioned_at IS NULL AND w.desired_state IN ('ENABLED','DRAINING') AND s.ended_at IS NULL AND s.session_state='REGISTERED' FOR SHARE OF w,s`, worker).Scan(&currentInstance, &currentBoot)
	if err != nil || currentInstance != instance || currentBoot != boot {
		return target, interactiondomain.ErrFenced
	}
	if adapterGeneration == 0 {
		if _, err = tx.Exec(ctx, `UPDATE workforce_attempt_runtimes SET adapter_session_generation=$2 WHERE attempt_id=$1`, m.AttemptID, m.AdapterSessionGeneration); err != nil {
			return target, err
		}
	}
	var imageDigest string
	if a.images == nil {
		return target, interactiondomain.ErrUnavailable
	}
	if err = tx.QueryRow(ctx, `SELECT harness_image_digest FROM workforce_attempt_runtimes WHERE attempt_id=$1`, m.AttemptID).Scan(&imageDigest); err != nil {
		return target, err
	}
	target.Capabilities, err = a.images.AdapterCapabilities(ctx, tx, worker, instance, imageDigest)
	if err != nil {
		return target, interactiondomain.ErrFenced
	}
	resolver, ok := a.principals.(principal.WorkerResolver)
	if !ok {
		return target, interactiondomain.ErrUnavailable
	}
	target.PrincipalID, err = resolver.EnsureWorker(ctx, tx, a.ids, worker)
	target.AttemptID = m.AttemptID
	target.AdapterSessionGeneration = m.AdapterSessionGeneration
	target.AcceptsNativeInput = taskStatus == "RUNNING"
	return target, err
}

func (a *interactionAuthority) AuthorizeUser(ctx context.Context, tx pgx.Tx, u interaction.User, action string) (interaction.Target, error) {
	var target interaction.Target
	if u.TenantID <= 0 || u.TaskID <= 0 || u.UserID <= 0 {
		return target, interactiondomain.ErrNotFound
	}
	var status string
	err := tx.QueryRow(ctx, `SELECT id,tenant_id,COALESCE(current_task_turn_id,0),status FROM workforce_tasks WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, u.TaskID, u.TenantID).Scan(&target.TaskID, &target.TenantID, &target.TaskTurnID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return target, interactiondomain.ErrNotFound
	}
	if err != nil {
		return target, err
	}
	// HTTP wrappers enforce query/continue permissions. Read summaries additionally
	// carry the authenticated caller's continuation permission, never client JSON.
	active := status == "RUNNING" || status == "QUEUED" || status == "INTERACTION_SUSPENDED"
	if action != interaction.Read && (!u.CanContinue || !active) {
		return target, interactiondomain.ErrNotAllowed
	}
	// Read watermarks are principal-scoped too. Resolution never uses a username
	// or an untrusted actor value supplied in the request body.
	target.PrincipalID, err = a.principals.EnsureUser(ctx, tx, a.ids, u.UserID)
	if err != nil {
		return target, err
	}
	if u.CanContinue {
		// Card-level actions and the Delivery HTTP permission remain authoritative;
		// these values only let the conversation render the corresponding actions.
		target.AllowedActions = append(target.AllowedActions, interaction.DecideDelivery, interaction.RestartDelivery)
	}
	err = tx.QueryRow(ctx, `SELECT a.id,COALESCE(r.adapter_session_generation,0) FROM workforce_task_attempts a LEFT JOIN workforce_attempt_runtimes r ON r.attempt_id=a.id WHERE a.task_id=$1 AND a.task_turn_id=$2 ORDER BY a.id DESC LIMIT 1`, u.TaskID, target.TaskTurnID).Scan(&target.AttemptID, &target.AdapterSessionGeneration)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return target, err
	}
	if isTerminalTaskStatus(status) {
		target.ClosedAttemptState = interactiondomain.Superseded
		if status == "CANCELLED" {
			target.ClosedAttemptState = interactiondomain.Cancelled
		}
	}
	if target.AttemptID > 0 && target.AdapterSessionGeneration > 0 && active {
		var worker int64
		var instance, digest string
		if err = tx.QueryRow(ctx, `SELECT worker_id,worker_instance_id,harness_image_digest FROM workforce_attempt_runtimes WHERE attempt_id=$1`, target.AttemptID).Scan(&worker, &instance, &digest); err != nil {
			return target, err
		}
		if a.images == nil {
			return target, interactiondomain.ErrUnavailable
		}
		target.Capabilities, err = a.images.AdapterCapabilities(ctx, tx, worker, instance, digest)
		if err != nil {
			if action == interaction.Read {
				// Runtime inventory gates new input, not access to persisted history.
				// Never infer capabilities or grant input while admission is unavailable.
				target.Capabilities = nil
				return target, nil
			}
			return target, interactiondomain.ErrUnavailable
		}
		if u.CanContinue {
			canRequestTool := false
			canReceiveToolResult := false
			for _, capability := range target.Capabilities {
				switch capability {
				case "interaction.respond":
					target.AllowedActions = append(target.AllowedActions, interaction.Answer)
				case "user.message":
					if status != "INTERACTION_SUSPENDED" {
						target.AllowedActions = append(target.AllowedActions, interaction.SendMessage)
					}
				case "tool.request":
					canRequestTool = true
				case "tool.result":
					canReceiveToolResult = true
				}
			}
			if canRequestTool && canReceiveToolResult {
				target.AllowedActions = append(target.AllowedActions, interaction.DecideOperation)
			}
		}
	}
	if action != interaction.Read {
		allowed := false
		for _, value := range target.AllowedActions {
			if action == value {
				allowed = true
			}
		}
		if !allowed {
			return target, interactiondomain.ErrNotAllowed
		}
	}
	return target, err
}
