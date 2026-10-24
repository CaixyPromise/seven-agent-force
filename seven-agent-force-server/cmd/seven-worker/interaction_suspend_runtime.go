package main

import (
	"context"
	"errors"
	"fmt"
	runtime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
	continuation "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	continuationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	checkpoint "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/checkpoint"
	task "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	wcp "github.com/CaixyPromise/seven-agent/force/internal/worker/checkpoint"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/control"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type interactionSuspendRuntimeChannel interface {
	State(context.Context) (runtime.InteractionExecutionState, error)
	DecideSuspension(context.Context, runtime.InteractionExecutionState, bool) error
	AwaitSuspension(context.Context, runtime.InteractionExecutionState) (runtime.InteractionCapture, error)
	WithStoppedQuiescentWorkspace(context.Context, int64, func() error) error
}

func (w *worker) suspendInteractionRuntime(ctx context.Context, item task.WorkItem, a task.Attempt, workspace runtime.Workspace, channel interactionSuspendRuntimeChannel) error {
	if w.interactions == nil {
		return errInteractionTransport
	}
	ctx, cancel := context.WithTimeout(ctx, runtime.InteractionCaptureAllowance)
	defer cancel()
	prepared, err := channel.State(ctx)
	if err != nil {
		return err
	}
	if prepared.State != "PAUSING_PREPARE" {
		return errors.New("INTERACTION_RESERVATION_BEFORE_STOP_REQUIRED")
	}
	round, err := strconv.ParseInt(strings.TrimPrefix(prepared.RoundKey, "round:"), 10, 64)
	if err != nil || round <= 0 || prepared.RoundKey != "round:"+strconv.FormatInt(round, 10) {
		return errors.New("INTERACTION_SUSPEND_TARGET_CONFLICT")
	}
	var interactionID, operationID int64
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for interactionID == 0 && operationID == 0 {
		current, err := channel.State(ctx)
		if err != nil {
			return err
		}
		if current.RoundKey != prepared.RoundKey || current.SessionGeneration != prepared.SessionGeneration || current.SuspendControlKey != prepared.SuspendControlKey {
			return errors.New("INTERACTION_SUSPEND_TARGET_CONFLICT")
		}
		if len(current.PendingInteractions) == 0 && len(current.PendingTools) == 0 {
			// An already-written answer may be receipted before reservation.
			// Abandoning capture grants no Server state or completion authority.
			if err := channel.DecideSuspension(ctx, prepared, false); err != nil {
				return err
			}
			return runtime.ErrInteractionSuspendAborted
		}
		pending, err := w.control.PendingInteractions(ctx, a, prepared.SessionGeneration)
		if err != nil {
			return err
		}
		for _, q := range pending {
			if q.RoundKey == prepared.RoundKey {
				for _, key := range prepared.PendingInteractions {
					if q.LocalRequestKey == key {
						if interactionID != 0 {
							return errors.New("INTERACTION_SUSPEND_TARGET_CONFLICT")
						}
						interactionID = q.InteractionID
					}
				}
			}
		}
		if interactionID == 0 && len(prepared.PendingTools) > 0 {
			operations, operationErr := w.control.PendingSuspendExecutionOperations(ctx, a, prepared.SessionGeneration)
			if operationErr != nil {
				return operationErr
			}
			for _, operation := range operations {
				if operation.RoundKey != prepared.RoundKey {
					continue
				}
				for _, key := range prepared.PendingTools {
					if operation.LocalToolCallKey == key {
						if operationID != 0 {
							return errors.New("INTERACTION_SUSPEND_TARGET_CONFLICT")
						}
						operationID = operation.OperationID
					}
				}
			}
		}
		if interactionID != 0 || operationID != 0 {
			break
		}
		// The question may still be publishing, or a saved answer already won.
		// Do not stop/drain the pump while resolving that distinction.
		deliveries, err := w.control.PullInteractions(ctx, a, prepared.SessionGeneration)
		if err != nil {
			return err
		}
		for _, delivery := range deliveries {
			if delivery.ResponseID > 0 && delivery.RoundKey == prepared.RoundKey {
				for _, key := range prepared.PendingInteractions {
					if delivery.LocalRequestKey == key {
						if err := channel.DecideSuspension(ctx, prepared, false); err != nil {
							return err
						}
						return runtime.ErrInteractionSuspendAborted
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	var reservation continuationfacade.InteractionReservationResult
	if operationID > 0 {
		reservation, err = w.control.ReserveOperationSuspend(ctx, a, prepared.SessionGeneration, operationID, round, prepared.SuspendControlKey)
	} else {
		reservation, err = w.control.ReserveInteractionSuspend(ctx, a, prepared.SessionGeneration, interactionID, round, prepared.SuspendControlKey)
	}
	if err != nil {
		return err
	}
	if reservation.Disposition == "ANSWER_WON" {
		if err := channel.DecideSuspension(ctx, prepared, false); err != nil {
			return err
		}
		return runtime.ErrInteractionSuspendAborted
	}
	if reservation.Disposition != "RESERVED" {
		return errors.New("INTERACTION_RESERVATION_INVALID")
	}
	if err := channel.DecideSuspension(ctx, prepared, true); err != nil {
		return err
	}
	capture, err := channel.AwaitSuspension(ctx, prepared)
	if err != nil {
		return err
	}
	drain, stop := context.WithTimeout(ctx, 2*time.Second)
	err = w.interactions.Drain(drain, a)
	stop()
	if err != nil {
		return err
	}
	budget, err := runtime.BudgetSnapshot(ctx)
	if err != nil {
		return err
	}
	if w.interactionBudgets == nil {
		return runtime.ErrBudgetRecovery
	}
	if err := w.interactionBudgets.Save(ctx); err != nil {
		return err
	}
	input := interactionSuspendInput{InteractionID: interactionID, OperationID: operationID, ExecutionBudget: wcp.ExecutionBudget{Version: budget.Version, TaskID: budget.TaskID, AttemptID: budget.AttemptID, Generation: budget.Generation, OwnerEpoch: budget.OwnerEpoch, ActiveLimitMillis: budget.ActiveLimitMillis, ActiveElapsedMillis: budget.ActiveElapsedMillis, InteractionWaitingMillis: budget.InteractionWaitingMillis, AbsoluteDeadlineUnixMillis: budget.AbsoluteDeadlineUnixMillis}, Checkpoint: checkpoint.Input{
		TaskID: item.TaskID, TaskTurnID: item.TaskTurnID, AttemptID: a.ID, Generation: a.Generation, Decision: continuation.PurposeInteractionSuspend, SessionDigest: capture.SessionDigest,
		RepoRoot: workspace.Repo, StateRoot: workspace.State, BaseCommit: item.ResolvedBaseCommit, ExecutionSnapshotDigest: item.TaskExecutionSnapshotDigest,
		HarnessImageDigest: item.ExecutionSnapshot.HarnessImageDigest, HookConfigurationDigest: item.ExecutionSnapshot.HookConfigurationDigest, ResultProcessorDigest: item.ExecutionSnapshot.ResultProcessorDigest, ModelCredentialIdentity: fmt.Sprintf("%d:%d:%s", item.ExecutionSnapshot.ModelCredentialID, item.ExecutionSnapshot.CredentialVersion, item.ExecutionSnapshot.ModelProtocol), IsolationProfile: item.ExecutionSnapshot.IsolationProfile,
	}}
	targetID := interactionID
	if operationID > 0 {
		targetID = operationID
	}
	spool := filepath.Join(w.cfg.Worker.WorkspaceRoot, ".seven", "interaction-checkpoints", strconv.FormatInt(a.ID, 10), strconv.FormatInt(targetID, 10))
	stored, err := w.captureReservedInteractionSuspend(ctx, channel, reservation, input, spool)
	if err != nil {
		return err
	}
	blob := func(b interactionSuspendBlob) control.InteractionBlob {
		return control.InteractionBlob{ObjectKey: b.ObjectKey, SHA256: b.ObjectSHA256, SizeBytes: b.SizeBytes}
	}
	return w.control.PublishInteractionSuspend(ctx, a, control.InteractionCapturePublication{SessionGeneration: capture.SessionGeneration, InteractionID: interactionID, OperationID: operationID, CheckpointID: reservation.Reservation.ID, Identity: stored.Identity, Bundle: blob(stored.Bundle), Overlay: blob(stored.Overlay), Session: blob(stored.Session), Manifest: blob(stored.Manifest)})
}
