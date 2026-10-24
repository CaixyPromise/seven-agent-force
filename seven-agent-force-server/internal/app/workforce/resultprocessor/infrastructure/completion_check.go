package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	check "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	processor "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
	"github.com/jackc/pgx/v5"
)

func (s *Store) SetCompletionCheckEvidenceReader(reader check.EvidenceReader) {
	s.completionEvidence = reader
}

// The caller holds the fenced Attempt lock, serializing cycle allocation even
// across machine response loss. A caller cannot invent an authorized cycle ID.
func (s *Store) authorizeCompletionCheckCycle(ctx context.Context, tx pgx.Tx, scope evaluationScope, attemptID, requested int64, number int) (int64, error) {
	var existing int64
	err := tx.QueryRow(ctx, `SELECT cycle_id FROM workforce_completion_check_cycle_authorizations WHERE tenant_id=$1 AND task_id=$2 AND task_turn_id=$3 AND attempt_id=$4 AND cycle_number=$5`, scope.TenantID, scope.TaskID, scope.TurnID, attemptID, number).Scan(&existing)
	if err == nil {
		if requested != 0 && requested != existing {
			return 0, ErrConflict
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	if requested != 0 {
		return 0, ErrConflict
	}
	id := s.idGen.NextID()
	if id <= 0 {
		return 0, errors.New("RESULT_PROCESSOR_CYCLE_ID_UNAVAILABLE")
	}
	_, err = tx.Exec(ctx, `INSERT INTO workforce_completion_check_cycle_authorizations(cycle_id,tenant_id,task_id,task_turn_id,attempt_id,cycle_number) VALUES($1,$2,$3,$4,$5,$6)`, id, scope.TenantID, scope.TaskID, scope.TurnID, attemptID, number)
	return id, err
}

// The existing fenced transaction owns this call. No candidate-provided
// observation or historical-independence boolean is accepted here.
func (s *Store) completionCheckGate(ctx context.Context, tx pgx.Tx, scope evaluationScope, attemptID, cycleID int64, contract processor.ResultContract) (processor.ResultContract, error) {
	var raw, referenceJSON []byte
	var digest, inputDigest string
	err := tx.QueryRow(ctx, `SELECT COALESCE(completion_check_json,'null'::jsonb),COALESCE(completion_check_digest,''),COALESCE(completion_check_input_digest,''),COALESCE(completion_check_references_json,'[]'::jsonb) FROM workforce_task_turns WHERE tenant_id=$1 AND task_id=$2 AND id=$3`, scope.TenantID, scope.TaskID, scope.TurnID).Scan(&raw, &digest, &inputDigest, &referenceJSON)
	if err != nil {
		return contract, err
	}
	var input *check.Input
	var references []check.Reference
	if json.Unmarshal(referenceJSON, &references) != nil {
		return contract, errors.New("CHECK_REFERENCES_INVALID")
	}
	if json.Unmarshal(raw, &input) != nil {
		return contract, errors.New("CHECK_SNAPSHOT_INVALID")
	}
	if input == nil {
		if digest != "" || inputDigest != "" || len(references) != 0 {
			return contract, errors.New("CHECK_SNAPSHOT_INVALID")
		}
		return contract, nil
	}
	actual, err := check.Digest(*input)
	if inputDigest == "" && len(references) == 0 {
		inputDigest = digest
	}
	frozen, freezeErr := check.FrozenDigest(*input, references)
	if err != nil || actual != inputDigest || freezeErr != nil || frozen != digest {
		return contract, errors.New("CHECK_SNAPSHOT_INVALID")
	}
	before := check.Observation{CheckDigest: digest, Status: "UNAVAILABLE"}
	after := before
	if input.Mode == "NONE" {
		before.Status = "NOT_CONFIGURED"
		after.Status = "NOT_CONFIGURED"
	} else if s.completionEvidence != nil {
		before, after, err = s.completionEvidence.LoadComparison(ctx, tx, scope.TenantID, scope.TaskID, scope.TurnID, attemptID, cycleID, digest)
		if err != nil {
			return contract, err
		}
	}
	definitionChanged := before.DefinitionDigest != after.DefinitionDigest
	if definitionChanged {
		return check.GateEvaluation(contract, before, after, check.ComparisonContext{DefinitionChanged: true}), nil
	}
	if contract.Decision == "PASS" && after.Status == "FAILED" {
		if reader, ok := s.completionEvidence.(check.LimitationsReader); ok {
			accepted, err := reader.AcceptedLimitations(ctx, tx, scope.TenantID, scope.TaskID, scope.TurnID, digest, after)
			if err != nil {
				return contract, err
			}
			if accepted {
				return contract, nil
			}
		}
	}
	// No proven dependency/independence evidence exists in Batch1; the zero
	// context deliberately cannot forgive historical failures by count/path.
	return check.GateEvaluation(contract, before, after, check.ComparisonContext{}), nil
}
