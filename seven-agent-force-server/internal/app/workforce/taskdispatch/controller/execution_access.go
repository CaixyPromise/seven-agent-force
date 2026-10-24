package controller

import (
	"context"
	"errors"
	"net/http"

	executionaccessdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/domain"
	executionaccess "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
)

func (h *MachineHandler) mountExecutionAccess(r route.IRouter) {
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/claim-next", h.claimNextExecutionOperation)
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/pending-suspend", h.pendingSuspendExecutionOperations)
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/start", h.startExecutionOperation)
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/renew", h.renewExecutionOperation)
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/result", h.resultExecutionOperation)
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/result-receipt", h.readExecutionOperationResultReceipt)
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/results/pull", h.pullExecutionOperationResults)
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/result-ack", h.ackExecutionOperationResult)
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/cold/claim", h.claimColdExecutionOperation)
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/cold/start", h.startColdExecutionOperation)
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/cold/renew", h.renewColdExecutionOperation)
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/cold/result", h.resultColdExecutionOperation)
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/cold/result-receipt", h.readColdExecutionOperationResultReceipt)
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/cold/readback", h.readbackColdExecutionOperation)
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/cold/result-ack", h.ackColdExecutionOperationResult)
}

func (h *MachineHandler) coldExecutionAccessAuthorized(ctx context.Context, c *app.RequestContext, e interactionEnvelope) (executionaccess.Machine, bool) {
	machine, ok := h.executionAccessAuthorized(ctx, c, e)
	if !ok {
		return machine, false
	}
	machine.ColdResumeOperationID = pathID(c, "operationId")
	if machine.ColdResumeOperationID <= 0 {
		writeExecutionAccessError(c, executionaccessdomain.ErrInvalid)
		return executionaccess.Machine{}, false
	}
	return machine, true
}

func (h *MachineHandler) claimColdExecutionOperation(ctx context.Context, c *app.RequestContext) {
	var req interactionEnvelope
	if !decode(c, &req, 16<<10) {
		return
	}
	machine, ok := h.coldExecutionAccessAuthorized(ctx, c, req)
	if !ok {
		return
	}
	value, err := h.executionAccess.Claim(ctx, machine, executionaccess.ClaimRequest{OperationID: machine.ColdResumeOperationID})
	if err != nil {
		writeExecutionAccessError(c, err)
		return
	}
	response.Success(c, value)
}

func (h *MachineHandler) startColdExecutionOperation(ctx context.Context, c *app.RequestContext) {
	var req struct {
		interactionEnvelope
		Start executionaccess.StartRequest `json:"start"`
	}
	if !decode(c, &req, 16<<10) {
		return
	}
	machine, ok := h.coldExecutionAccessAuthorized(ctx, c, req.interactionEnvelope)
	if !ok {
		return
	}
	req.Start.OperationID = machine.ColdResumeOperationID
	value, err := h.executionAccess.Start(ctx, machine, req.Start)
	if err != nil {
		writeExecutionAccessError(c, err)
		return
	}
	response.Success(c, value)
}

func (h *MachineHandler) renewColdExecutionOperation(ctx context.Context, c *app.RequestContext) {
	var req struct {
		interactionEnvelope
		Renew executionaccess.RenewRequest `json:"renew"`
	}
	if !decode(c, &req, 16<<10) {
		return
	}
	machine, ok := h.coldExecutionAccessAuthorized(ctx, c, req.interactionEnvelope)
	if !ok {
		return
	}
	req.Renew.OperationID = machine.ColdResumeOperationID
	value, err := h.executionAccess.Renew(ctx, machine, req.Renew)
	if err != nil {
		writeExecutionAccessError(c, err)
		return
	}
	response.Success(c, value)
}

func (h *MachineHandler) resultColdExecutionOperation(ctx context.Context, c *app.RequestContext) {
	var req struct {
		interactionEnvelope
		Result executionaccess.ResultRequest `json:"result"`
	}
	if !decode(c, &req, 96<<10) {
		return
	}
	machine, ok := h.coldExecutionAccessAuthorized(ctx, c, req.interactionEnvelope)
	if !ok {
		return
	}
	req.Result.OperationID = machine.ColdResumeOperationID
	value, err := h.executionAccess.SubmitResult(ctx, machine, req.Result)
	if err != nil {
		writeExecutionAccessError(c, err)
		return
	}
	response.Success(c, value)
}

func (h *MachineHandler) readColdExecutionOperationResultReceipt(ctx context.Context, c *app.RequestContext) {
	var req struct {
		interactionEnvelope
		Receipt executionaccess.ResultReceiptRequest `json:"receipt"`
	}
	if !decode(c, &req, 16<<10) {
		return
	}
	machine, ok := h.coldExecutionAccessAuthorized(ctx, c, req.interactionEnvelope)
	if !ok {
		return
	}
	req.Receipt.OperationID = machine.ColdResumeOperationID
	value, err := h.executionAccess.ReadResultReceipt(ctx, machine, req.Receipt)
	if err != nil {
		writeExecutionAccessError(c, err)
		return
	}
	response.Success(c, value)
}

func (h *MachineHandler) readbackColdExecutionOperation(ctx context.Context, c *app.RequestContext) {
	var req interactionEnvelope
	if !decode(c, &req, 16<<10) {
		return
	}
	machine, ok := h.coldExecutionAccessAuthorized(ctx, c, req)
	if !ok {
		return
	}
	value, err := h.executionAccess.ReadResult(ctx, machine, machine.ColdResumeOperationID)
	if err != nil {
		writeExecutionAccessError(c, err)
		return
	}
	response.Success(c, value)
}

func (h *MachineHandler) ackColdExecutionOperationResult(ctx context.Context, c *app.RequestContext) {
	var req struct {
		interactionEnvelope
		Ack executionaccess.ResultAckRequest `json:"ack"`
	}
	if !decode(c, &req, 16<<10) {
		return
	}
	machine, ok := h.coldExecutionAccessAuthorized(ctx, c, req.interactionEnvelope)
	if !ok {
		return
	}
	req.Ack.OperationID = machine.ColdResumeOperationID
	value, err := h.executionAccess.AckResult(ctx, machine, req.Ack)
	if err != nil {
		writeExecutionAccessError(c, err)
		return
	}
	response.Success(c, value)
}

func (h *MachineHandler) pendingSuspendExecutionOperations(ctx context.Context, c *app.RequestContext) {
	var envelope interactionEnvelope
	if !decode(c, &envelope, 16<<10) {
		return
	}
	machine, ok := h.executionAccessAuthorized(ctx, c, envelope)
	if !ok {
		return
	}
	value, err := h.executionAccess.PendingSuspendOperations(ctx, machine)
	if err != nil {
		writeExecutionAccessError(c, err)
		return
	}
	response.Success(c, value)
}

func executionAccessMachine(e interactionEnvelope, attemptID int64) executionaccess.Machine {
	return executionaccess.Machine{
		WorkerID: e.WorkerID, InstanceID: e.InstanceID, HostBootID: e.HostBootID,
		AttemptID: attemptID, Generation: e.Generation, OwnerEpoch: e.OwnerEpoch,
		AdapterSessionGeneration: e.AdapterSessionGeneration, ExecutionLease: e.AttemptLeaseToken,
	}
}

func (h *MachineHandler) executionAccessAuthorized(ctx context.Context, c *app.RequestContext, e interactionEnvelope) (executionaccess.Machine, bool) {
	if !h.authAttempt(ctx, c, e.MachineEnvelope, e.AttemptLeaseToken) {
		return executionaccess.Machine{}, false
	}
	if h.executionAccess == nil {
		writeExecutionAccessError(c, executionaccessdomain.ErrUnavailable)
		return executionaccess.Machine{}, false
	}
	attemptID := pathID(c, "attemptId")
	if attemptID <= 0 {
		writeExecutionAccessError(c, executionaccessdomain.ErrInvalid)
		return executionaccess.Machine{}, false
	}
	return executionAccessMachine(e, attemptID), true
}

func writeExecutionAccessError(c *app.RequestContext, err error) {
	status, code := http.StatusServiceUnavailable, executionaccessdomain.ErrUnavailable.Error()
	switch {
	case errors.Is(err, executionaccessdomain.ErrInvalid), errors.Is(err, executionaccessdomain.ErrTargetDenied):
		status, code = http.StatusBadRequest, err.Error()
	case errors.Is(err, executionaccessdomain.ErrNotFound):
		status, code = http.StatusNotFound, err.Error()
	case errors.Is(err, executionaccessdomain.ErrStale), errors.Is(err, executionaccessdomain.ErrActionNotAllowed), errors.Is(err, executionaccessdomain.ErrIdempotencyConflict), errors.Is(err, executionaccessdomain.ErrGrantRevoked), errors.Is(err, executionaccessdomain.ErrGrantExpired), errors.Is(err, executionaccessdomain.ErrFenced), errors.Is(err, executionaccessdomain.ErrOutcomeUnknown):
		status, code = http.StatusConflict, err.Error()
	}
	writeError(c, status, code, "execution access request could not be completed")
}

func (h *MachineHandler) claimNextExecutionOperation(ctx context.Context, c *app.RequestContext) {
	var envelope interactionEnvelope
	if !decode(c, &envelope, 16<<10) {
		return
	}
	machine, ok := h.executionAccessAuthorized(ctx, c, envelope)
	if !ok {
		return
	}
	value, err := h.executionAccess.ClaimNext(ctx, machine)
	if err != nil {
		writeExecutionAccessError(c, err)
		return
	}
	response.Success(c, value)
}

func (h *MachineHandler) startExecutionOperation(ctx context.Context, c *app.RequestContext) {
	var req struct {
		interactionEnvelope
		Start executionaccess.StartRequest `json:"start"`
	}
	if !decode(c, &req, 16<<10) {
		return
	}
	machine, ok := h.executionAccessAuthorized(ctx, c, req.interactionEnvelope)
	if !ok {
		return
	}
	req.Start.OperationID = pathID(c, "operationId")
	value, err := h.executionAccess.Start(ctx, machine, req.Start)
	if err != nil {
		writeExecutionAccessError(c, err)
		return
	}
	response.Success(c, value)
}

func (h *MachineHandler) renewExecutionOperation(ctx context.Context, c *app.RequestContext) {
	var req struct {
		interactionEnvelope
		Renew executionaccess.RenewRequest `json:"renew"`
	}
	if !decode(c, &req, 16<<10) {
		return
	}
	machine, ok := h.executionAccessAuthorized(ctx, c, req.interactionEnvelope)
	if !ok {
		return
	}
	req.Renew.OperationID = pathID(c, "operationId")
	value, err := h.executionAccess.Renew(ctx, machine, req.Renew)
	if err != nil {
		writeExecutionAccessError(c, err)
		return
	}
	response.Success(c, value)
}

func (h *MachineHandler) resultExecutionOperation(ctx context.Context, c *app.RequestContext) {
	var req struct {
		interactionEnvelope
		Result executionaccess.ResultRequest `json:"result"`
	}
	if !decode(c, &req, 96<<10) {
		return
	}
	machine, ok := h.executionAccessAuthorized(ctx, c, req.interactionEnvelope)
	if !ok {
		return
	}
	req.Result.OperationID = pathID(c, "operationId")
	value, err := h.executionAccess.SubmitResult(ctx, machine, req.Result)
	if err != nil {
		writeExecutionAccessError(c, err)
		return
	}
	response.Success(c, value)
}

func (h *MachineHandler) readExecutionOperationResultReceipt(ctx context.Context, c *app.RequestContext) {
	var req struct {
		interactionEnvelope
		Receipt executionaccess.ResultReceiptRequest `json:"receipt"`
	}
	if !decode(c, &req, 16<<10) {
		return
	}
	machine, ok := h.executionAccessAuthorized(ctx, c, req.interactionEnvelope)
	if !ok {
		return
	}
	req.Receipt.OperationID = pathID(c, "operationId")
	value, err := h.executionAccess.ReadResultReceipt(ctx, machine, req.Receipt)
	if err != nil {
		writeExecutionAccessError(c, err)
		return
	}
	response.Success(c, value)
}

func (h *MachineHandler) pullExecutionOperationResults(ctx context.Context, c *app.RequestContext) {
	var req struct {
		interactionEnvelope
		Limit int `json:"limit"`
	}
	if !decode(c, &req, 16<<10) {
		return
	}
	machine, ok := h.executionAccessAuthorized(ctx, c, req.interactionEnvelope)
	if !ok {
		return
	}
	value, err := h.executionAccess.PullResults(ctx, machine, req.Limit)
	if err != nil {
		writeExecutionAccessError(c, err)
		return
	}
	response.Success(c, value)
}

func (h *MachineHandler) ackExecutionOperationResult(ctx context.Context, c *app.RequestContext) {
	var req struct {
		interactionEnvelope
		Ack executionaccess.ResultAckRequest `json:"ack"`
	}
	if !decode(c, &req, 16<<10) {
		return
	}
	machine, ok := h.executionAccessAuthorized(ctx, c, req.interactionEnvelope)
	if !ok {
		return
	}
	req.Ack.OperationID = pathID(c, "operationId")
	value, err := h.executionAccess.AckResult(ctx, machine, req.Ack)
	if err != nil {
		writeExecutionAccessError(c, err)
		return
	}
	response.Success(c, value)
}
