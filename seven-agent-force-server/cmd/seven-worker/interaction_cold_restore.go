package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	runtime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
	continuation "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	task "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
	wcp "github.com/CaixyPromise/seven-agent/force/internal/worker/checkpoint"
)

// The private key response is authenticated independently from the encrypted
// archives. Neither a Task field nor a container-provided answer enters here.
func (w *worker) restoreColdInteraction(ctx context.Context, item *task.WorkItem, attempt task.Attempt, workspace runtime.Workspace, input *wcp.RestoreInput, key []byte) (runtime.Workspace, string, error) {
	bad := func(err error) (runtime.Workspace, string, error) { return runtime.Workspace{}, "", err }
	if input == nil || item.ContinuationRestore == nil || (input.Manifest.InteractionID > 0) == (input.Manifest.OperationID > 0) {
		return bad(errors.New("INTERACTION_RESTORE_INPUT_INVALID"))
	}
	if input.Manifest.OperationID > 0 {
		if item.SourceExecutionOperationID != input.Manifest.OperationID || len(input.InteractionResponse) != 0 || len(input.QuestionRequest) != 0 {
			return bad(errors.New("INTERACTION_RESTORE_INPUT_INVALID"))
		}
	} else {
		if len(input.InteractionResponse) == 0 || len(input.InteractionResponse) > 16<<10 || !json.Valid(input.InteractionResponse) {
			return bad(errors.New("INTERACTION_RESTORE_INPUT_INVALID"))
		}
		if _, _, err := agentprotocol.ValidateRestoredInteraction(input.QuestionRequest, input.InteractionResponse, "round:1"); err != nil {
			return bad(errors.New("INTERACTION_RESTORE_INPUT_INVALID"))
		}
	}
	r := item.ContinuationRestore
	var contract continuation.RestoreContract
	body, err := json.Marshal(r)
	if err != nil {
		return bad(err)
	}
	if err = json.Unmarshal(body, &contract); err != nil {
		return bad(err)
	}
	resultDigest := sha256Hex([]byte(strings.Join([]string{strconv.FormatInt(r.CheckpointID, 10), r.TaskExecutionSnapshotDigest, r.ExpectedResultCommit, r.ExpectedResultCommit, r.ExpectedResultTree, r.SessionLineageDigest}, "\x00")))
	// Bind the private record to independently authenticated key metadata and
	// exact target paths. Neither an Attempt-local marker nor a container ACK
	// permits reuse. The journal stays outside every Attempt mount.
	binding, err := json.Marshal([]any{r, input, workspace.Repo, workspace.State})
	if err != nil {
		return bad(err)
	}
	id := coldRestoreIdentity{BudgetIdentity: runtime.BudgetIdentity{TaskID: item.TaskID, AttemptID: attempt.ID, Generation: attempt.Generation, OwnerEpoch: attempt.OwnerEpoch}, RestoreRunID: r.RestoreRunID, CheckpointID: r.CheckpointID, BindingSHA256: sha256Hex(binding), ResultSHA256: resultDigest}
	err = recoverColdRestore(ctx, w.interactionBudgets, id, workspace, func() error {
		// recoverColdRestore already proved both roots empty. Remove only the
		// empty State root required by Extract; never reset existing contents.
		if e := os.Remove(workspace.State); e != nil && !os.IsNotExist(e) {
			return e
		}
		spool := filepath.Join(workspace.AttemptRoot, ".interaction-restore")
		if e := os.MkdirAll(spool, 0700); e != nil {
			return e
		}
		if _, e := w.restoreInteractionSuspend(ctx, contract, input.Manifest, key, spool, workspace.Repo, workspace.State); e != nil {
			return e
		}
		if e := prepareRestoredSessionState(ctx, workspace.State, item.TaskID, attempt.ID, r.SessionLineageDigest); e != nil {
			return e
		}
		if e := prepareRestoredBindPermissions(workspace.Repo, workspace.State); e != nil {
			return e
		}
		return nil
	}, func() error {
		// PrepareAttempt already keeps State private on redelivery. Only the
		// repository needs the writable bind contract; never widen session data.
		if e := os.Chmod(workspace.Repo, 0777); e != nil {
			return e
		}
		if e := w.acceptVerifiedInteractionBudget(ctx, *item, attempt, *input); e != nil {
			return e
		}
		return confirmColdRestore(ctx, func() error {
			return w.control.CompleteContinuationRestore(ctx, attempt, r.RestoreRunID, resultDigest, r.ExpectedBaseCommit, r.ExpectedResultCommit, r.ExpectedResultTree)
		})
	})
	if err != nil {
		return bad(err)
	}
	if input.Manifest.InteractionID > 0 {
		item.InteractionResponse = append(json.RawMessage(nil), input.InteractionResponse...)
		item.InteractionQuestionRequest = append(json.RawMessage(nil), input.QuestionRequest...)
	}
	item.SourceAdapterSessionGeneration = input.Manifest.AdapterSessionGeneration
	item.InteractionBudgetRestored = true
	return workspace, r.ExpectedResultCommit, nil
}

// Read back the identical receipt once after ambiguous transport failure. A
// pending confirmation must not be converted into terminal workspace failure.
func confirmColdRestore(ctx context.Context, confirm func() error) error {
	err := confirm()
	if err != nil && ctx.Err() == nil && errors.Is(interactionRemoteError(err), errInteractionRetry) {
		err = confirm()
	}
	if err != nil && errors.Is(interactionRemoteError(err), errInteractionRetry) {
		return errColdRestoreConfirmationPending
	}
	return err
}
