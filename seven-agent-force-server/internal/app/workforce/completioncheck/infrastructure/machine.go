package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Store) machineScope(ctx context.Context, tx pgx.Tx, a facade.MachineAuthority, attempt int64, lease string) (facade.Evidence, string, error) {
	e := facade.Evidence{AttemptID: attempt}
	var worker, generation, epoch, currentTurn int64
	var instance, boot, leaseHash, status, taskStatus string
	var expires time.Time
	err := tx.QueryRow(ctx, `SELECT t.tenant_id,t.id,a.task_turn_id,a.worker_id,a.worker_instance_id,a.host_boot_id,a.generation,a.owner_epoch,a.lease_token,a.lease_expires_at,a.status,t.current_task_turn_id,t.status FROM workforce_task_attempts a JOIN workforce_tasks t ON t.id=a.task_id WHERE a.id=$1 FOR UPDATE OF t,a`, attempt).Scan(&e.TenantID, &e.TaskID, &e.TaskTurnID, &worker, &instance, &boot, &generation, &epoch, &leaseHash, &expires, &status, &currentTurn, &taskStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, status, facade.ErrNotFound
	}
	if err != nil {
		return e, status, err
	}
	sum := sha256.Sum256([]byte(lease))
	if taskStatus != "PREPARING" && taskStatus != "RUNNING" && taskStatus != "VALIDATING" {
		return e, status, facade.ErrConflict
	}
	if status != "PREPARING" && status != "RUNNING" && status != "VALIDATING" {
		return e, status, facade.ErrConflict
	}
	if a.WorkerID <= 0 || a.Generation <= 0 || a.OwnerEpoch <= 0 || lease == "" || a.WorkerID != worker || a.InstanceID != instance || a.HostBootID != boot || a.Generation != generation || a.OwnerEpoch != epoch || hex.EncodeToString(sum[:]) != leaseHash || !expires.After(time.Now()) || currentTurn != e.TaskTurnID {
		return e, status, facade.ErrConflict
	}
	return e, status, nil
}
func (s *Store) RecordMachineEvidence(ctx context.Context, a facade.MachineAuthority, attempt int64, lease string, submitted facade.Evidence) (facade.Evidence, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return facade.Evidence{}, err
	}
	defer tx.Rollback(ctx)
	bound, status, err := s.machineScope(ctx, tx, a, attempt, lease)
	if err != nil {
		return bound, err
	}
	if submitted.TaskID != bound.TaskID || submitted.TaskTurnID != bound.TaskTurnID || submitted.AttemptID != attempt {
		return bound, facade.ErrConflict
	}
	submitted.TenantID = bound.TenantID
	if submitted.Phase == "BASELINE" {
		if status != "PREPARING" {
			return bound, facade.ErrConflict
		}
		var started bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workforce_attempt_runtimes WHERE attempt_id=$1 AND harness_started_at IS NOT NULL)`, attempt).Scan(&started); err != nil {
			return bound, err
		}
		// Replays are checked against immutable evidence; newly recording a baseline
		// after the Harness started would measure modified code and is forbidden.
		if started {
			existing, e := readEvidence(ctx, tx, bound, submitted.Phase, 0, submitted.CheckDigest)
			if e != nil {
				return bound, e
			}
			if existing == nil {
				return bound, facade.ErrConflict
			}
		}
	} else {
		if status != "RUNNING" && status != "VALIDATING" {
			return bound, facade.ErrConflict
		}
		var ready bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workforce_completion_check_cycle_authorizations WHERE attempt_id=$1 AND cycle_id=$2 AND tenant_id=$3 AND task_id=$4 AND task_turn_id=$5)`, attempt, submitted.EvaluationCycleID, bound.TenantID, bound.TaskID, bound.TaskTurnID).Scan(&ready)
		if err != nil {
			return bound, err
		}
		if !ready {
			return bound, facade.ErrConflict
		}
	}
	result, _, err := s.RecordEvidence(ctx, tx, submitted)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
func readEvidence(ctx context.Context, tx pgx.Tx, bound facade.Evidence, phase string, cycle int64, digest string) (*facade.Evidence, error) {
	e := bound
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT id,task_turn_id,attempt_id,phase,COALESCE(evaluation_cycle_id,0),check_digest,status,summary,observation_json,source_commit,source_identity_digest FROM workforce_completion_check_results WHERE tenant_id=$1 AND task_id=$6 AND phase=$3 AND COALESCE(evaluation_cycle_id,0)=$4 AND check_digest=$5 AND (($3='BASELINE' AND id=(SELECT completion_check_baseline_result_id FROM workforce_task_turns WHERE tenant_id=$1 AND task_id=$6 AND id=$7)) OR ($3='CURRENT' AND attempt_id=$2))`, bound.TenantID, bound.AttemptID, phase, cycle, digest, bound.TaskID, bound.TaskTurnID).Scan(&e.ID, &e.TaskTurnID, &e.AttemptID, &e.Phase, &e.EvaluationCycleID, &e.CheckDigest, &e.Status, &e.Summary, &raw, &e.SourceCommit, &e.SourceIdentityDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &e.Observation); err != nil {
		return nil, err
	}
	return &e, nil
}
func (s *Store) ReadMachineEvidence(ctx context.Context, a facade.MachineAuthority, attempt int64, lease, phase string, cycle int64, digest string) (*facade.Evidence, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	bound, _, err := s.machineScope(ctx, tx, a, attempt, lease)
	if err != nil {
		return nil, err
	}
	result, err := readEvidence(ctx, tx, bound, phase, cycle, digest)
	if err != nil {
		return nil, err
	}
	return result, tx.Commit(ctx)
}
