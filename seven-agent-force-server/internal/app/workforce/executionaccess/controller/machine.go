package controller

import (
	"context"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/facade"
)

// MachineResolver is supplied by authenticated Worker middleware. It must not
// derive authority from request JSON or path values alone.
type MachineResolver func(context.Context, *app.RequestContext, int64) (facade.Machine, error)

type MachineHandler struct {
	service facade.MachineService
	resolve MachineResolver
}

func NewMachine(service facade.MachineService, resolve MachineResolver) *MachineHandler {
	return &MachineHandler{service: service, resolve: resolve}
}
func (h *MachineHandler) Mount(r route.IRouter) {
	if h.resolve == nil {
		panic("execution access machine resolver is required")
	}
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/publish", h.publish)
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/close", h.close)
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/claim-next", h.claimNext)
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/claim", h.claim)
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/start", h.start)
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/renew", h.renew)
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/result", h.result)
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/result-receipt", h.resultReceipt)
	r.GET("/internal/workforce/v1/attempts/:attemptId/execution-operations/results", h.pull)
	r.POST("/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/result-ack", h.ack)
}
func (h *MachineHandler) machine(ctx context.Context, c *app.RequestContext) (facade.Machine, error) {
	attempt, err := positive(c.Param("attemptId"))
	if err != nil {
		return facade.Machine{}, err
	}
	m, err := h.resolve(ctx, c, attempt)
	if err != nil {
		return m, domain.ErrFenced
	}
	if m.AttemptID != attempt {
		return m, domain.ErrFenced
	}
	return m, nil
}
func (h *MachineHandler) publish(ctx context.Context, c *app.RequestContext) {
	m, e := h.machine(ctx, c)
	if e != nil {
		problem(c, e)
		return
	}
	var req facade.PublishRequest
	if e = decode(c, &req, 32<<10); e != nil {
		problem(c, e)
		return
	}
	v, e := h.service.Publish(ctx, m, req)
	write(c, v, e)
}
func (h *MachineHandler) close(ctx context.Context, c *app.RequestContext) {
	m, e := h.machine(ctx, c)
	if e != nil {
		problem(c, e)
		return
	}
	var req facade.CloseRequest
	if e = decode(c, &req, 8<<10); e != nil {
		problem(c, e)
		return
	}
	v, e := h.service.CloseToolCall(ctx, m, req)
	write(c, v, e)
}
func (h *MachineHandler) claim(ctx context.Context, c *app.RequestContext) {
	m, id, e := h.machineResource(ctx, c)
	if e != nil {
		problem(c, e)
		return
	}
	v, e := h.service.Claim(ctx, m, facade.ClaimRequest{OperationID: id})
	write(c, v, e)
}
func (h *MachineHandler) claimNext(ctx context.Context, c *app.RequestContext) {
	m, e := h.machine(ctx, c)
	if e != nil {
		problem(c, e)
		return
	}
	v, e := h.service.ClaimNext(ctx, m)
	write(c, v, e)
}
func (h *MachineHandler) start(ctx context.Context, c *app.RequestContext) {
	m, id, e := h.machineResource(ctx, c)
	if e != nil {
		problem(c, e)
		return
	}
	var req facade.StartRequest
	if e = decode(c, &req, 8<<10); e != nil {
		problem(c, e)
		return
	}
	req.OperationID = id
	v, e := h.service.Start(ctx, m, req)
	write(c, v, e)
}
func (h *MachineHandler) renew(ctx context.Context, c *app.RequestContext) {
	m, id, e := h.machineResource(ctx, c)
	if e != nil {
		problem(c, e)
		return
	}
	var req facade.RenewRequest
	if e = decode(c, &req, 8<<10); e != nil {
		problem(c, e)
		return
	}
	req.OperationID = id
	v, e := h.service.Renew(ctx, m, req)
	write(c, v, e)
}
func (h *MachineHandler) result(ctx context.Context, c *app.RequestContext) {
	m, id, e := h.machineResource(ctx, c)
	if e != nil {
		problem(c, e)
		return
	}
	var req facade.ResultRequest
	if e = decode(c, &req, 512<<10); e != nil {
		problem(c, e)
		return
	}
	req.OperationID = id
	v, e := h.service.SubmitResult(ctx, m, req)
	write(c, v, e)
}
func (h *MachineHandler) resultReceipt(ctx context.Context, c *app.RequestContext) {
	m, id, e := h.machineResource(ctx, c)
	if e != nil {
		problem(c, e)
		return
	}
	var req facade.ResultReceiptRequest
	if e = decode(c, &req, 8<<10); e != nil {
		problem(c, e)
		return
	}
	req.OperationID = id
	v, e := h.service.ReadResultReceipt(ctx, m, req)
	write(c, v, e)
}
func (h *MachineHandler) pull(ctx context.Context, c *app.RequestContext) {
	m, e := h.machine(ctx, c)
	if e != nil {
		problem(c, e)
		return
	}
	limit := 20
	if raw := c.Query("limit"); raw != "" {
		limit, e = strconv.Atoi(raw)
		if e != nil {
			problem(c, domain.ErrInvalid)
			return
		}
	}
	v, e := h.service.PullResults(ctx, m, limit)
	write(c, v, e)
}
func (h *MachineHandler) ack(ctx context.Context, c *app.RequestContext) {
	m, id, e := h.machineResource(ctx, c)
	if e != nil {
		problem(c, e)
		return
	}
	var req facade.ResultAckRequest
	if e = decode(c, &req, 8<<10); e != nil {
		problem(c, e)
		return
	}
	req.OperationID = id
	v, e := h.service.AckResult(ctx, m, req)
	write(c, v, e)
}
func (h *MachineHandler) machineResource(ctx context.Context, c *app.RequestContext) (facade.Machine, int64, error) {
	m, e := h.machine(ctx, c)
	if e != nil {
		return m, 0, e
	}
	id, e := positive(c.Param("operationId"))
	return m, id, e
}
