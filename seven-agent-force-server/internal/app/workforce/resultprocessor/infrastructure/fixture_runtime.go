package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	processor "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
)

// ClaimFixtureRun fences a Fixture to the Worker instance that owns the
// exact admitted Harness image. It freezes the Revision snapshot that was
// stored when the run was requested; changing the authoring draft later cannot
// affect an already queued Fixture.
func (s *Store) ClaimFixtureRun(ctx context.Context, identity facade.MachineIdentity, runID int64) (facade.FixtureExecution, error) {
	if err := validFixtureIdentity(identity, runID); err != nil {
		return facade.FixtureExecution{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return facade.FixtureExecution{}, err
	}
	defer tx.Rollback(ctx)
	if err = lockFixtureWorker(ctx, tx, identity); err != nil {
		return facade.FixtureExecution{}, err
	}
	var item facade.FixtureExecution
	var status, assignedInstance string
	var assignedWorker int64
	err = tx.QueryRow(ctx, `SELECT status,worker_id,COALESCE(worker_instance_id,''),generation,harness_image_digest,revision_id,revision_digest,
		revision_manifest_json,revision_files_json,evaluation_context_json,workspace_files_json,config_overrides_json,expected_decision
		FROM workforce_result_processor_fixture_runs WHERE id=$1 FOR UPDATE`, runID).
		Scan(&status, &assignedWorker, &assignedInstance, &item.Generation, &item.HarnessImageDigest, &item.RevisionID, &item.RevisionDigest,
			&item.Manifest, &item.Files, &item.EvaluationContext, &item.WorkspaceFiles, &item.ConfigOverrides, &item.ExpectedDecision)
	if errors.Is(err, pgx.ErrNoRows) {
		return facade.FixtureExecution{}, ErrNotFound
	}
	if err != nil {
		return facade.FixtureExecution{}, err
	}
	if assignedWorker != identity.WorkerID || item.Generation != identity.Generation {
		return facade.FixtureExecution{}, ErrConflict
	}
	if status != "PENDING" && status != "RUNNING" {
		return facade.FixtureExecution{}, ErrConflict
	}
	if status == "RUNNING" && assignedInstance != identity.InstanceID {
		return facade.FixtureExecution{}, ErrConflict
	}
	validation := processor.Validate(item.Manifest, item.Files)
	if !validation.Valid {
		return facade.FixtureExecution{}, fmt.Errorf("RESULT_PROCESSOR_FIXTURE_REVISION_INVALID")
	}
	// The exact digest is checked again at claim time.  A Fixture is never
	// allowed to silently fall back to an image tag or an older inventory
	// observation after the author submitted it.
	var imageAvailable bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM workforce_worker_images
		 WHERE worker_id=$1 AND image_digest=$2 AND role='attempt'
		   AND admission_status='ADMITTED' AND presence_status='PRESENT'
	)`, identity.WorkerID, item.HarnessImageDigest).Scan(&imageAvailable)
	if err != nil || !imageAvailable {
		return facade.FixtureExecution{}, ErrConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_result_processor_fixture_runs
		SET status='RUNNING',worker_instance_id=$2,started_at=COALESCE(started_at,CURRENT_TIMESTAMP)
		WHERE id=$1`, runID, identity.InstanceID); err != nil {
		return facade.FixtureExecution{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return facade.FixtureExecution{}, err
	}
	item.RunID = runID
	return item, nil
}

// RecordFixtureResult accepts only the Result Contract produced by the
// protected bootstrap.  A failed fixture remains useful evidence but cannot
// mutate authoring assets or a Task execution state.
func (s *Store) RecordFixtureResult(ctx context.Context, identity facade.MachineIdentity, runID int64, result facade.FixtureResult) (FixtureRun, error) {
	if err := validFixtureIdentity(identity, runID); err != nil {
		return FixtureRun{}, err
	}
	if len(result.Diagnostics) > 4096 || len(result.Contract) > processor.MaxContractBytes {
		return FixtureRun{}, ErrConflict
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return FixtureRun{}, err
	}
	defer tx.Rollback(ctx)
	if err = lockFixtureWorker(ctx, tx, identity); err != nil {
		return FixtureRun{}, err
	}
	var tenantID, workerID int64
	var status, instanceID, expectedDecision, existingDigest string
	var generation int64
	err = tx.QueryRow(ctx, `SELECT tenant_id,status,COALESCE(worker_id,0),COALESCE(worker_instance_id,''),generation,expected_decision,result_contract_sha256
		FROM workforce_result_processor_fixture_runs WHERE id=$1 FOR UPDATE`, runID).
		Scan(&tenantID, &status, &workerID, &instanceID, &generation, &expectedDecision, &existingDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return FixtureRun{}, ErrNotFound
	}
	if err != nil {
		return FixtureRun{}, err
	}
	if workerID != identity.WorkerID || instanceID != identity.InstanceID || generation != identity.Generation {
		return FixtureRun{}, ErrConflict
	}
	contractBody := json.RawMessage(nil)
	contractDigest := ""
	reasonCode := strings.TrimSpace(result.ReasonCode)
	fixtureStatus := "FAILED"
	if len(result.Contract) != 0 {
		validated, validationErr := processor.ValidateContract(result.Contract)
		if validationErr != nil {
			reasonCode = "RESULT_PROCESSOR_FIXTURE_CONTRACT_INVALID"
		} else {
			sum := sha256.Sum256(result.Contract)
			contractDigest = hex.EncodeToString(sum[:])
			if strings.TrimSpace(result.ContractDigest) != "" && result.ContractDigest != contractDigest {
				return FixtureRun{}, ErrConflict
			}
			contractBody = append(json.RawMessage(nil), result.Contract...)
			fixtureStatus = "SUCCEEDED"
			if expectedDecision != "" && validated.Decision != expectedDecision {
				fixtureStatus = "FAILED"
				reasonCode = "FIXTURE_EXPECTED_DECISION_MISMATCH"
			}
		}
	}
	if reasonCode == "" && fixtureStatus == "FAILED" {
		reasonCode = "RESULT_PROCESSOR_FIXTURE_FAILED"
	}
	if status == "SUCCEEDED" || status == "FAILED" {
		if existingDigest == contractDigest {
			if err = tx.Commit(ctx); err != nil {
				return FixtureRun{}, err
			}
			return s.FixtureRun(ctx, tenantID, runID)
		}
		return FixtureRun{}, ErrConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_result_processor_fixture_runs
		SET status=$2,result_contract_json=$3,result_contract_sha256=$4,reason_code=$5,diagnostics=$6,finished_at=CURRENT_TIMESTAMP
		WHERE id=$1 AND status IN ('PENDING','RUNNING')`, runID, fixtureStatus, contractBody, contractDigest, reasonCode, boundedFixtureDiagnostic(result.Diagnostics)); err != nil {
		return FixtureRun{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return FixtureRun{}, err
	}
	return s.FixtureRun(ctx, tenantID, runID)
}

func validFixtureIdentity(identity facade.MachineIdentity, runID int64) error {
	if identity.WorkerID <= 0 || strings.TrimSpace(identity.InstanceID) == "" || strings.TrimSpace(identity.HostBootID) == "" || identity.Generation < 1 || runID <= 0 {
		return ErrConflict
	}
	return nil
}

func lockFixtureWorker(ctx context.Context, tx pgx.Tx, identity facade.MachineIdentity) error {
	var currentInstance, desiredState, hostBoot, consumerState string
	var dockerReady bool
	err := tx.QueryRow(ctx, `SELECT w.current_instance_id,w.desired_state,s.host_boot_id,s.consumer_state,s.docker_ready
		FROM workforce_workers w JOIN workforce_worker_sessions s ON s.worker_id=w.id AND s.instance_id=w.current_instance_id
		WHERE w.id=$1 AND w.decommissioned_at IS NULL FOR UPDATE`, identity.WorkerID).
		Scan(&currentInstance, &desiredState, &hostBoot, &consumerState, &dockerReady)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if currentInstance != identity.InstanceID || hostBoot != identity.HostBootID || desiredState != "ENABLED" || consumerState != "CONSUMING" || !dockerReady {
		return ErrConflict
	}
	return nil
}

func boundedFixtureDiagnostic(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 4096 {
		return value[:4096]
	}
	return value
}
