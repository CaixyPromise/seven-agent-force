package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"

	attemptruntime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
	executionaccessdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/domain"
	executionaccess "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/facade"
	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/control"
)

type boundExecutionOperationControl struct {
	client    *control.Client
	attempt   taskdomain.Attempt
	channel   *attemptruntime.ExecutionChannel
	item      taskdomain.WorkItem
	workspace attemptruntime.Workspace
}

type coldExecutionOperationControl struct {
	client                  *control.Client
	attempt                 taskdomain.Attempt
	sourceSessionGeneration int64
	operationID             int64
	item                    taskdomain.WorkItem
	workspace               attemptruntime.Workspace
}

func (b coldExecutionOperationControl) ClaimNetworkOperation(ctx context.Context) (networkOperationClaim, bool, error) {
	claim, err := b.client.ClaimColdExecutionOperation(ctx, b.attempt, b.sourceSessionGeneration, b.operationID)
	if err != nil {
		return networkOperationClaim{}, false, err
	}
	if claim.Operation.ID != b.operationID || claim.AttemptID <= 0 || claim.Operation.TaskID != b.item.TaskID || claim.Operation.Command == "" {
		return networkOperationClaim{}, false, errInteractionBinding
	}
	targets := make([]networkOperationTarget, 0, len(claim.Operation.Targets))
	for _, target := range claim.Operation.Targets {
		targets = append(targets, networkOperationTarget{Host: target.Host, Protocol: target.Protocol, Port: target.Port})
	}
	deadline := time.Now().UTC().Add(operationDefaultTimeout)
	if claim.Operation.ExpiresAt.Before(deadline) {
		deadline = claim.Operation.ExpiresAt
	}
	return networkOperationClaim{OperationID: claim.Operation.ID, OperationAttemptID: claim.AttemptID, TaskID: claim.Operation.TaskID, TaskTurnID: b.item.TaskTurnID, AttemptID: b.attempt.ID, OwnerEpoch: b.attempt.OwnerEpoch, DescriptorDigest: claim.Operation.DescriptorDigest, Purpose: claim.Operation.Purpose, Command: claim.Operation.Command, CWD: claim.Operation.Cwd, Targets: targets, HarnessImage: b.item.HarnessImageDigest, RepositoryRoot: b.workspace.Repo, Deadline: deadline}, true, nil
}

func (b coldExecutionOperationControl) MarkNetworkOperationRunning(ctx context.Context, claim networkOperationClaim) error {
	_, err := b.client.StartColdExecutionOperation(ctx, b.attempt, b.sourceSessionGeneration, b.operationID, executionaccess.StartRequest{OperationID: b.operationID, AttemptID: claim.OperationAttemptID, ContainerID: claim.RuntimeContainerID, RelayIdentity: claim.RelayIdentity})
	return err
}

func (b coldExecutionOperationControl) RenewNetworkOperation(ctx context.Context, claim networkOperationClaim) error {
	_, err := b.client.RenewColdExecutionOperation(ctx, b.attempt, b.sourceSessionGeneration, b.operationID, executionaccess.RenewRequest{OperationID: b.operationID, AttemptID: claim.OperationAttemptID})
	return err
}

func (b coldExecutionOperationControl) SubmitNetworkOperationResult(ctx context.Context, claim networkOperationClaim, result networkOperationResult) error {
	var exitCode *int
	if result.Status == executionaccessdomain.StatusSucceeded || result.Status == executionaccessdomain.StatusFailed {
		value := result.ExitCode
		exitCode = &value
	}
	summary := strings.TrimSpace(result.ReasonCode)
	if result.Status == executionaccessdomain.StatusSucceeded {
		summary = "Command completed successfully."
	} else if result.Status == executionaccessdomain.StatusFailed {
		summary = "Command exited unsuccessfully."
	}
	digest := executionaccessdomain.ResultDigest(result.Status, exitCode, summary, result.Output)
	_, err := b.client.SubmitColdExecutionOperationResult(ctx, b.attempt, b.sourceSessionGeneration, b.operationID, executionaccess.ResultRequest{OperationID: b.operationID, AttemptID: claim.OperationAttemptID, Status: result.Status, ResultDigest: digest, ExitCode: exitCode, Summary: summary, Output: result.Output})
	return err
}

func (b coldExecutionOperationControl) ConfirmNetworkOperationResult(ctx context.Context, claim networkOperationClaim, result networkOperationResult) (bool, error) {
	digest := serverOperationResultDigest(result)
	receipt, err := b.client.ReadColdExecutionOperationResultReceipt(ctx, b.attempt, b.sourceSessionGeneration, b.operationID, executionaccess.ResultReceiptRequest{OperationID: b.operationID, AttemptID: claim.OperationAttemptID, ResultDigest: digest})
	return err == nil && receipt.ResultDigest == digest, err
}

func (w *worker) prepareColdExecutionOperation(ctx context.Context, item *taskdomain.WorkItem, attempt taskdomain.Attempt, workspace attemptruntime.Workspace) error {
	if item.SourceExecutionOperationID <= 0 {
		return nil
	}
	if item.SourceAdapterSessionGeneration <= 0 {
		return errors.New("OPERATION_RESTORE_IDENTITY_INVALID")
	}
	read := func() (executionaccess.ToolResult, error) {
		return w.control.ReadbackColdExecutionOperation(ctx, attempt, item.SourceAdapterSessionGeneration, item.SourceExecutionOperationID)
	}
	result, err := read()
	if err != nil {
		var api *control.APIError
		if !errors.As(err, &api) || api.ReasonCode != executionaccessdomain.ErrStale.Error() {
			return err
		}
		bound := coldExecutionOperationControl{client: w.control, attempt: attempt, sourceSessionGeneration: item.SourceAdapterSessionGeneration, operationID: item.SourceExecutionOperationID, item: *item, workspace: workspace}
		processed, runErr := (executionOperationPump{control: bound, executor: newNetworkOperationRuntime(w.cfg.Worker.WorkspaceRoot, w.cleanupJournal, w.cfg.Worker.OperationDNSOverHTTPSURL)}).RunOnce(ctx)
		if runErr != nil || !processed {
			if runErr != nil {
				return runErr
			}
			return errors.New("OPERATION_RESTORE_NOT_READY")
		}
		result, err = read()
		if err != nil {
			return err
		}
	}
	if result.OperationID != item.SourceExecutionOperationID || result.AdapterSessionGeneration != item.SourceAdapterSessionGeneration {
		return errors.New("OPERATION_RESTORE_RESULT_INVALID")
	}
	body, err := marshalOperationToolResult(result)
	if err != nil || len(body) > 80<<10 {
		return errors.New("OPERATION_RESTORE_RESULT_INVALID")
	}
	if err = agentprotocol.ValidateToolPayload("tool.result", body); err != nil {
		return errors.New("OPERATION_RESTORE_RESULT_INVALID")
	}
	item.OperationToolResult = body
	return nil
}

func marshalOperationToolResult(result executionaccess.ToolResult) ([]byte, error) {
	body, err := json.Marshal(agentprotocol.ToolResultPayload{RoundKey: result.RoundKey, LocalToolCallKey: result.LocalToolCallKey, NativeToolCallID: result.NativeToolCallID, OperationID: result.OperationID, Status: result.Status, ExitCode: result.ExitCode, Summary: result.Summary, Output: result.Output, ResultDigest: result.ResultDigest, ReasonCode: result.ReasonCode})
	if err != nil || agentprotocol.ValidateToolPayload("tool.result", body) != nil {
		return nil, errors.New("OPERATION_RESULT_IDENTITY_INVALID")
	}
	return body, nil
}

func (b boundExecutionOperationControl) current(ctx context.Context) (attemptruntime.InteractionExecutionState, error) {
	state, err := b.channel.State(ctx)
	if err != nil || state.SessionGeneration <= 0 || state.RoundKey == "" || state.State != "RUNNING" {
		return state, errInteractionBinding
	}
	return state, nil
}

func (b boundExecutionOperationControl) ClaimNetworkOperation(ctx context.Context) (networkOperationClaim, bool, error) {
	state, err := b.channel.State(ctx)
	ready, err := executionOperationSessionReady(state, err)
	if err != nil {
		return networkOperationClaim{}, false, err
	}
	if !ready {
		// The operation poller is attached before the native round starts and is
		// stopped after it leaves RUNNING. Those edge states mean there is no
		// command to claim; they are not an authority or identity failure.
		return networkOperationClaim{}, false, nil
	}
	claim, err := b.client.ClaimNextExecutionOperation(ctx, b.attempt, state.SessionGeneration)
	if err != nil {
		var api *control.APIError
		if errors.As(err, &api) && api.ReasonCode == executionaccessdomain.ErrNotFound.Error() {
			return networkOperationClaim{}, false, nil
		}
		return networkOperationClaim{}, false, err
	}
	if claim.Operation.ID <= 0 || claim.AttemptID <= 0 || claim.Operation.AttemptID != b.attempt.ID || claim.Operation.TaskID != b.item.TaskID || claim.Operation.TaskTurnID != b.item.TaskTurnID ||
		(claim.Operation.Status != executionaccessdomain.StatusDispatched && claim.Operation.Status != executionaccessdomain.StatusRunning) || claim.Operation.Command == "" {
		return networkOperationClaim{}, false, errInteractionBinding
	}
	targets := make([]networkOperationTarget, 0, len(claim.Operation.Targets))
	for _, target := range claim.Operation.Targets {
		targets = append(targets, networkOperationTarget{Host: target.Host, Protocol: target.Protocol, Port: target.Port})
	}
	deadline := time.Now().UTC().Add(operationDefaultTimeout)
	if claim.Operation.ExpiresAt.Before(deadline) {
		deadline = claim.Operation.ExpiresAt
	}
	return networkOperationClaim{OperationID: claim.Operation.ID, OperationAttemptID: claim.AttemptID, TaskID: claim.Operation.TaskID, TaskTurnID: claim.Operation.TaskTurnID, AttemptID: b.attempt.ID, OwnerEpoch: b.attempt.OwnerEpoch, DescriptorDigest: claim.Operation.DescriptorDigest, Purpose: claim.Operation.Purpose, Command: claim.Operation.Command, CWD: claim.Operation.Cwd, Targets: targets, HarnessImage: b.item.HarnessImageDigest, RepositoryRoot: b.workspace.Repo, Deadline: deadline}, true, nil
}

func executionOperationSessionReady(state attemptruntime.InteractionExecutionState, stateErr error) (bool, error) {
	if stateErr != nil {
		return false, stateErr
	}
	if state.SessionGeneration <= 0 || state.RoundKey == "" {
		return false, errInteractionBinding
	}
	return state.State == "RUNNING", nil
}

func operationSubmitIsTerminal(err error) bool {
	var api *control.APIError
	if !errors.As(err, &api) {
		return false
	}
	switch api.ReasonCode {
	case executionaccessdomain.ErrFenced.Error(), executionaccessdomain.ErrGrantRevoked.Error(),
		executionaccessdomain.ErrGrantExpired.Error(), executionaccessdomain.ErrStale.Error(),
		executionaccessdomain.ErrActionNotAllowed.Error():
		return true
	default:
		return false
	}
}

func (b boundExecutionOperationControl) MarkNetworkOperationRunning(ctx context.Context, claim networkOperationClaim) error {
	state, err := b.current(ctx)
	if err != nil {
		return err
	}
	_, err = b.client.StartExecutionOperation(ctx, b.attempt, state.SessionGeneration, executionaccess.StartRequest{OperationID: claim.OperationID, AttemptID: claim.OperationAttemptID, ContainerID: claim.RuntimeContainerID, RelayIdentity: claim.RelayIdentity})
	return err
}

func (b boundExecutionOperationControl) RenewNetworkOperation(ctx context.Context, claim networkOperationClaim) error {
	state, err := b.current(ctx)
	if err != nil {
		return err
	}
	_, err = b.client.RenewExecutionOperation(ctx, b.attempt, state.SessionGeneration, executionaccess.RenewRequest{OperationID: claim.OperationID, AttemptID: claim.OperationAttemptID})
	return err
}

func (b boundExecutionOperationControl) SubmitNetworkOperationResult(ctx context.Context, claim networkOperationClaim, result networkOperationResult) error {
	state, err := b.current(ctx)
	if err != nil {
		return err
	}
	var exitCode *int
	if result.Status == executionaccessdomain.StatusSucceeded || result.Status == executionaccessdomain.StatusFailed {
		value := result.ExitCode
		exitCode = &value
	}
	// Full bounded output remains encrypted and is returned only to the active
	// Adapter. The ordinary database/chat projection uses an allowlisted fact;
	// repository output may itself contain secrets that regex redaction cannot
	// reliably classify.
	var summary string
	switch result.Status {
	case executionaccessdomain.StatusSucceeded:
		summary = "Command completed successfully."
	case executionaccessdomain.StatusFailed:
		summary = "Command exited unsuccessfully."
	default:
		summary = strings.TrimSpace(result.ReasonCode)
	}
	serverDigest := executionaccessdomain.ResultDigest(result.Status, exitCode, summary, result.Output)
	_, err = b.client.SubmitExecutionOperationResult(ctx, b.attempt, state.SessionGeneration, executionaccess.ResultRequest{OperationID: claim.OperationID, AttemptID: claim.OperationAttemptID, Status: result.Status, ResultDigest: serverDigest, ExitCode: exitCode, Summary: summary, Output: result.Output})
	return err
}

func (b boundExecutionOperationControl) ConfirmNetworkOperationResult(ctx context.Context, claim networkOperationClaim, result networkOperationResult) (bool, error) {
	state, err := b.current(ctx)
	if err != nil {
		return false, err
	}
	digest := serverOperationResultDigest(result)
	receipt, err := b.client.ReadExecutionOperationResultReceipt(ctx, b.attempt, state.SessionGeneration, executionaccess.ResultReceiptRequest{OperationID: claim.OperationID, AttemptID: claim.OperationAttemptID, ResultDigest: digest})
	return err == nil && receipt.ResultDigest == digest, err
}

func serverOperationResultDigest(result networkOperationResult) string {
	var exitCode *int
	if result.Status == executionaccessdomain.StatusSucceeded || result.Status == executionaccessdomain.StatusFailed {
		value := result.ExitCode
		exitCode = &value
	}
	summary := strings.TrimSpace(result.ReasonCode)
	if result.Status == executionaccessdomain.StatusSucceeded {
		summary = "Command completed successfully."
	}
	if result.Status == executionaccessdomain.StatusFailed {
		summary = "Command exited unsuccessfully."
	}
	return executionaccessdomain.ResultDigest(result.Status, exitCode, summary, result.Output)
}

func (w *worker) startExecutionOperationLoop(ctx context.Context, item taskdomain.WorkItem, attempt taskdomain.Attempt, workspace attemptruntime.Workspace, channel *attemptruntime.ExecutionChannel) func() {
	lifetime, cancel := context.WithCancel(ctx)
	control := boundExecutionOperationControl{client: w.control, attempt: attempt, channel: channel, item: item, workspace: workspace}
	pump := executionOperationPump{control: control, executor: newNetworkOperationRuntime(w.cfg.Worker.WorkspaceRoot, w.cleanupJournal, w.cfg.Worker.OperationDNSOverHTTPSURL)}
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			processed, err := pump.RunOnce(lifetime)
			if err != nil && lifetime.Err() == nil {
				w.logger.Error("controlled network operation failed", zap.Int64("task_id", item.TaskID), zap.Int64("attempt_id", attempt.ID), zap.Error(err))
				// A response-loss recovery must reclaim the same Operation and read
				// result.json. Stopping this loop here would strand the native tool
				// call forever. Authoritative fences have already stopped and cleaned
				// the exact runner; they are equally safe to observe on the next poll.
				select {
				case <-lifetime.Done():
					return
				case <-time.After(time.Second):
				}
				continue
			}
			if processed {
				continue
			}
			select {
			case <-lifetime.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return func() { cancel(); <-done }
}
