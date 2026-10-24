package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
	"github.com/jackc/pgx/v5"
)

type interactionReattachCommit struct {
	ReconciliationID int64
	Completed        bool
	Binding          *domain.InteractionReplayBinding
}

// Called with the runtime/attempt locks held. Existing event JSON is the safe
// readback record; leases, permit tokens and execution contracts are never stored
// in event payloads. Reconciliation hashes bind both observations.
func validateInteractionReattachCommit(ctx context.Context, tx pgx.Tx, e domain.MachineEnvelope, taskID, attemptID int64, ack domain.ReattachAck, ackDigest string, startNeeded bool) (interactionReattachCommit, error) {
	var result interactionReattachCommit
	var status, evidenceDigest, storedAck, containerID, identityDigest, taskStatus string
	var runtimeOwner, runtimeGeneration int64
	err := tx.QueryRow(ctx, `SELECT c.id,c.status,c.runtime_evidence_sha256,COALESCE(c.supervisor_ack_sha256,''),
 COALESCE(r.container_id,''),r.execution_identity_digest,r.owner_epoch,r.generation
 FROM workforce_runtime_reconciliation_runs c
 JOIN workforce_attempt_runtimes r ON r.attempt_id=c.attempt_id
 WHERE c.attempt_id=$1 AND c.worker_id=$2 AND c.new_instance_id=$3 AND c.host_boot_id=$4
 AND c.old_owner_epoch=$5 AND c.new_owner_epoch=$6
 AND r.worker_id=$2 AND r.host_boot_id=$4
 ORDER BY c.id DESC LIMIT 1 FOR UPDATE OF c,r`, attemptID, e.WorkerID, e.InstanceID, e.HostBootID, ack.OldOwnerEpoch, e.OwnerEpoch).Scan(
		&result.ReconciliationID, &status, &evidenceDigest, &storedAck, &containerID, &identityDigest, &runtimeOwner, &runtimeGeneration)
	if err != nil {
		return result, application.ErrIdentityConflict
	}
	// Preserve Task/Attempt linkage and locks without a four-owner graph join.
	if err = tx.QueryRow(ctx, `SELECT t.status FROM workforce_task_attempts a JOIN workforce_tasks t ON t.id=a.task_id WHERE a.id=$1 AND t.id=$2 FOR UPDATE OF a,t`, attemptID, taskID).Scan(&taskStatus); err != nil {
		return result, application.ErrIdentityConflict
	}
	if runtimeOwner != e.OwnerEpoch || runtimeGeneration != e.Generation || containerID == "" || identityDigest == "" ||
		taskStatus == "CANCELLING" || taskStatus == "INTERACTION_SUSPENDED" || isTerminalTaskStatus(taskStatus) {
		return result, application.ErrIdentityConflict
	}
	var body []byte
	if err = tx.QueryRow(ctx, `SELECT payload_json->'runtimeEvidence' FROM workforce_task_events WHERE task_id=$1 AND attempt_id=$2 AND event_type='runtime.reattach_permit_issued' AND payload_json->>'reconciliationId'=$3 ORDER BY sequence DESC LIMIT 1`, taskID, attemptID, strconv.FormatInt(result.ReconciliationID, 10)).Scan(&body); err != nil {
		return result, application.ErrIdentityConflict
	}
	var before domain.RuntimeEvidence
	if json.Unmarshal(body, &before) != nil {
		return result, application.ErrIdentityConflict
	}
	canonical, _ := json.Marshal(before)
	sum := sha256.Sum256(canonical)
	if hex.EncodeToString(sum[:]) != evidenceDigest || before.AttemptID != attemptID || before.Generation != e.Generation || before.WorkerID != e.WorkerID || before.HostBootID != e.HostBootID {
		return result, application.ErrIdentityConflict
	}
	if before.InteractionReplayRequested {
		if before.AdapterSessionGeneration <= 0 || before.RoundKey == "" || ack.ContainerID != containerID || ack.ExecutionIdentityDigest != identityDigest || domain.ValidateReattachContinuity(before, ack) != nil {
			return result, application.ErrIdentityConflict
		}
	}
	if !startNeeded && before.InteractionReplayRequested {
		result.Binding = &domain.InteractionReplayBinding{ReconciliationID: result.ReconciliationID, TaskID: taskID, AttemptID: attemptID, Generation: e.Generation, OldOwnerEpoch: ack.OldOwnerEpoch, NewOwnerEpoch: e.OwnerEpoch, ContainerID: containerID, ExecutionIdentityDigest: identityDigest, AdapterSessionGeneration: before.AdapterSessionGeneration, RoundKey: before.RoundKey}
	}
	switch status {
	case "PERMIT_ISSUED":
	case "COMPLETED":
		if storedAck != ackDigest {
			return result, application.ErrIdentityConflict
		}
		if err = tx.QueryRow(ctx, `SELECT payload_json->'interactionReplayBinding' FROM workforce_task_events WHERE task_id=$1 AND attempt_id=$2 AND event_type='runtime.reattached' AND payload_json->>'reconciliationId'=$3 ORDER BY sequence DESC LIMIT 1`, taskID, attemptID, strconv.FormatInt(result.ReconciliationID, 10)).Scan(&body); err != nil {
			return result, application.ErrIdentityConflict
		}
		var stored *domain.InteractionReplayBinding
		if json.Unmarshal(body, &stored) != nil || (stored == nil) != (result.Binding == nil) || (stored != nil && *stored != *result.Binding) {
			return result, application.ErrIdentityConflict
		}
		result.Binding = stored
		result.Completed = true
	default:
		return result, application.ErrIdentityConflict
	}
	return result, nil
}
