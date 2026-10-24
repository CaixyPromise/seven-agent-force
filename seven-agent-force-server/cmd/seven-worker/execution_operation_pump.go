package main

import (
	"context"
	"errors"
	"time"
)

// executionOperationControl is the narrow seam that the parent change wires
// to the authenticated machine API. Claim remains Server-authoritative;
// Submit is idempotent by Operation and ResultDigest.
type executionOperationControl interface {
	ClaimNetworkOperation(context.Context) (networkOperationClaim, bool, error)
	MarkNetworkOperationRunning(context.Context, networkOperationClaim) error
	RenewNetworkOperation(context.Context, networkOperationClaim) error
	SubmitNetworkOperationResult(context.Context, networkOperationClaim, networkOperationResult) error
	ConfirmNetworkOperationResult(context.Context, networkOperationClaim, networkOperationResult) (bool, error)
}

type executionOperationExecutor interface {
	Prepare(context.Context, networkOperationClaim) (networkOperationClaim, error)
	Execute(context.Context, networkOperationClaim) (networkOperationResult, error)
	Cleanup(int64) error
}

type executionOperationPump struct {
	control  executionOperationControl
	executor executionOperationExecutor
}

func (p executionOperationPump) RunOnce(ctx context.Context) (bool, error) {
	if p.control == nil || p.executor == nil {
		return false, errors.New("OPERATION_PUMP_UNAVAILABLE")
	}
	claim, available, err := p.control.ClaimNetworkOperation(ctx)
	if err != nil || !available {
		return false, err
	}
	claim, err = p.executor.Prepare(ctx, claim)
	if err != nil {
		return true, err
	}
	if err = p.control.MarkNetworkOperationRunning(ctx, claim); err != nil {
		return true, err
	}
	executionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type outcome struct {
		result networkOperationResult
		err    error
	}
	finished := make(chan outcome, 1)
	go func() { result, runErr := p.executor.Execute(executionCtx, claim); finished <- outcome{result, runErr} }()
	// A task-scope grant revocation must tear down established relays promptly,
	// not merely prevent the next command. Renewal is the authoritative fence.
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var result networkOperationResult
	for {
		select {
		case completed := <-finished:
			if completed.err != nil {
				return true, completed.err
			}
			result = completed.result
			goto submit
		case <-ticker.C:
			if err = p.control.RenewNetworkOperation(ctx, claim); err != nil {
				cancel()
				<-finished
				_ = p.executor.Cleanup(claim.OperationID)
				return true, err
			}
		case <-ctx.Done():
			cancel()
			<-finished
			_ = p.executor.Cleanup(claim.OperationID)
			return true, ctx.Err()
		}
	}

submit:
	// A failed submit deliberately leaves the journal and readback intact. A
	// later claim reads result.json and cannot execute the command again.
	if err = p.control.SubmitNetworkOperationResult(ctx, claim, result); err != nil {
		if confirmed, confirmErr := p.control.ConfirmNetworkOperationResult(ctx, claim, result); confirmErr == nil && confirmed {
			if cleanupErr := p.executor.Cleanup(claim.OperationID); cleanupErr != nil {
				return true, cleanupErr
			}
			return true, nil
		}
		if operationSubmitIsTerminal(err) {
			_ = p.executor.Cleanup(claim.OperationID)
		}
		return true, err
	}
	if err = p.executor.Cleanup(claim.OperationID); err != nil {
		// Business result is already persisted; cleanup remains a durable local
		// reservation and the existing retry loop fences capacity.
		return true, err
	}
	return true, nil
}
