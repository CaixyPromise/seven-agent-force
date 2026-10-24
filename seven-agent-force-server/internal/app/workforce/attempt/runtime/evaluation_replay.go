package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	processor "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	"io"
	"os"
	"strings"
)

func pendingCandidatePath(root string, taskID int64, attempt domain.Attempt, cycleID int64) (string, error) {
	p, err := authoritativeEvaluationPath(root, taskID, attempt, cycleID)
	return strings.TrimSuffix(p, ".json") + ".candidate.json", err
}

// This is a Worker-private, post-secret-scan request, never container input.
// In particular EvaluationStartedAt and tree/findings digests stay unchanged.
func persistPendingCandidate(root string, taskID int64, attempt domain.Attempt, candidate facade.EvaluationCandidate) error {
	p, err := pendingCandidatePath(root, taskID, attempt, candidate.CycleID)
	if err != nil {
		return err
	}
	body, err := json.Marshal(candidate)
	if err != nil {
		return err
	}
	if previous, err := os.ReadFile(p); err == nil {
		if !bytes.Equal(previous, body) {
			return errors.New("EVALUATION_PENDING_CANDIDATE_CONFLICT")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return atomicPrivateWriteFile(p, body)
}

func recoverPendingEvaluation(ctx context.Context, root string, item domain.WorkItem, attempt domain.Attempt, state supervisorStateSnapshot, gate RuntimeGate) (facade.EvaluationDirective, error) {
	if state.EvaluationCycleID <= 0 || state.EvaluationCycle < 1 || state.EvaluationCycle > item.ExecutionSnapshot.MaxEvaluationCycles || state.TaskID != item.TaskID || state.AttemptID != attempt.ID || state.Generation != attempt.Generation || state.OwnerEpoch != attempt.OwnerEpoch || len(state.SessionDigest) > 128 {
		return facade.EvaluationDirective{}, errors.New("EVALUATION_STATE_IDENTITY_INVALID")
	}
	if gate.ReadEvaluation == nil || gate.PersistEvaluation == nil {
		return facade.EvaluationDirective{}, errors.New("EVALUATION_REPLAY_GATE_UNAVAILABLE")
	}
	if err := ctx.Err(); err != nil {
		return facade.EvaluationDirective{}, err
	}
	result, err := gate.ReadEvaluation(ctx, facade.EvaluationReplayIdentity{CycleID: state.EvaluationCycleID, CycleNumber: state.EvaluationCycle, SessionDigest: state.SessionDigest})
	if err != nil {
		return facade.EvaluationDirective{}, err
	}
	if result.Found {
		return result.Directive, nil
	}
	if err := ctx.Err(); err != nil {
		return facade.EvaluationDirective{}, err
	}
	p, err := pendingCandidatePath(root, item.TaskID, attempt, state.EvaluationCycleID)
	if err != nil {
		return facade.EvaluationDirective{}, err
	}
	f, err := os.Open(p)
	if err != nil {
		return facade.EvaluationDirective{}, errors.New("EVALUATION_PENDING_CANDIDATE_UNAVAILABLE")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > processor.MaxContractBytes+8192 {
		return facade.EvaluationDirective{}, errors.New("EVALUATION_PENDING_CANDIDATE_INVALID")
	}
	decoder := json.NewDecoder(io.LimitReader(f, processor.MaxContractBytes+8192))
	decoder.DisallowUnknownFields()
	var candidate facade.EvaluationCandidate
	if err = decoder.Decode(&candidate); err != nil {
		return facade.EvaluationDirective{}, errors.New("EVALUATION_PENDING_CANDIDATE_INVALID")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || candidate.CycleID != state.EvaluationCycleID || candidate.CycleNumber != state.EvaluationCycle || candidate.SessionDigest != state.SessionDigest || candidate.ProcessorReleaseID != item.ExecutionSnapshot.ResultProcessorReleaseID || candidate.ProcessorDigest != item.ExecutionSnapshot.ResultProcessorDigest {
		return facade.EvaluationDirective{}, errors.New("EVALUATION_PENDING_CANDIDATE_IDENTITY_INVALID")
	}
	contract, err := processor.ValidateContract(candidate.Contract)
	if err != nil {
		return facade.EvaluationDirective{}, errors.New("EVALUATION_PENDING_CANDIDATE_INVALID")
	}
	digest, err := processor.ContractDigest(contract)
	if err != nil || digest != candidate.ContractDigest {
		return facade.EvaluationDirective{}, errors.New("EVALUATION_PENDING_CANDIDATE_INVALID")
	}
	return gate.PersistEvaluation(candidate)
}
