package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"

	processor "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
)

type authoritativeEvaluation struct {
	TaskID     int64           `json:"taskId,string"`
	AttemptID  int64           `json:"attemptId,string"`
	Generation int64           `json:"generation,string"`
	CycleID    int64           `json:"cycleId,string"`
	Decision   string          `json:"decision"`
	Contract   json.RawMessage `json:"contract"`
	Digest     string          `json:"contractDigest"`
}

func authoritativeEvaluationPath(root string, taskID int64, attempt domain.Attempt, cycleID int64) (string, error) {
	if root == "" || taskID <= 0 || attempt.ID <= 0 || attempt.Generation <= 0 || cycleID <= 0 {
		return "", errors.New("AUTHORITATIVE_EVALUATION_IDENTITY_INVALID")
	}
	return filepath.Join(root, ".seven", "evaluations", strconv.FormatInt(taskID, 10), strconv.FormatInt(attempt.ID, 10), strconv.FormatInt(attempt.Generation, 10), strconv.FormatInt(cycleID, 10)+".json"), nil
}

func validateAuthoritativeContract(raw json.RawMessage, digest, decision string) (processor.ResultContract, error) {
	contract, err := processor.ValidateContract(raw)
	if err != nil {
		return processor.ResultContract{}, errors.New("AUTHORITATIVE_EVALUATION_CONTRACT_INVALID")
	}
	actual, err := processor.ContractDigest(contract)
	if err != nil || actual != digest || contract.Decision != decision {
		return processor.ResultContract{}, errors.New("AUTHORITATIVE_EVALUATION_CONTRACT_INVALID")
	}
	return contract, nil
}

// PersistAuthoritativeEvaluation consumes only the authenticated Server response.
// The .seven tree is Worker-private and never mounted into the Agent container.
// Candidate files remain unchanged, and must never substitute for this record.
func PersistAuthoritativeEvaluation(root string, taskID int64, attempt domain.Attempt, cycleID int64, d facade.EvaluationDirective) error {
	path, err := authoritativeEvaluationPath(root, taskID, attempt, cycleID)
	if err != nil {
		return err
	}
	if d.CycleID != cycleID {
		return errors.New("AUTHORITATIVE_EVALUATION_IDENTITY_INVALID")
	}
	expected := map[string]string{"PASS": facade.DirectiveProposeCompletion, "WAIT_USER": facade.DirectiveWaitForUser, "MANUAL_REVIEW": facade.DirectiveStopManualReview, "REJECT": facade.DirectiveStopRejected, "REVISE": facade.DirectiveResumeSameSession}
	if expected[d.Decision] == "" || expected[d.Decision] != d.Directive {
		return errors.New("AUTHORITATIVE_EVALUATION_DIRECTIVE_INVALID")
	}
	contract, err := validateAuthoritativeContract(d.Contract, d.ContractDigest, d.Decision)
	if err != nil {
		return err
	}
	canonical, err := json.Marshal(contract)
	if err != nil {
		return err
	}
	body, err := json.Marshal(authoritativeEvaluation{taskID, attempt.ID, attempt.Generation, cycleID, d.Decision, canonical, d.ContractDigest})
	if err != nil {
		return err
	}
	if existing, e := os.ReadFile(path); e == nil {
		if !bytes.Equal(existing, body) {
			return errors.New("AUTHORITATIVE_EVALUATION_REPLAY_CONFLICT")
		}
		return nil
	} else if !os.IsNotExist(e) {
		return e
	}
	return atomicPrivateWriteFile(path, body)
}

func ReadAuthoritativeEvaluation(root string, taskID int64, attempt domain.Attempt, cycleID int64, decision string) (processor.ResultContract, error) {
	path, err := authoritativeEvaluationPath(root, taskID, attempt, cycleID)
	if err != nil {
		return processor.ResultContract{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return processor.ResultContract{}, errors.New("CONTINUATION_RESULT_CONTRACT_UNAVAILABLE")
	}
	defer f.Close()
	decoder := json.NewDecoder(io.LimitReader(f, processor.MaxContractBytes+1))
	decoder.DisallowUnknownFields()
	var record authoritativeEvaluation
	if err = decoder.Decode(&record); err != nil {
		return processor.ResultContract{}, errors.New("AUTHORITATIVE_EVALUATION_CONTRACT_INVALID")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || record.TaskID != taskID || record.AttemptID != attempt.ID || record.Generation != attempt.Generation || record.CycleID != cycleID || record.Decision != decision {
		return processor.ResultContract{}, errors.New("AUTHORITATIVE_EVALUATION_IDENTITY_INVALID")
	}
	return validateAuthoritativeContract(record.Contract, record.Digest, decision)
}
