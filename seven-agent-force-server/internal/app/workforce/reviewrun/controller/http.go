package controller

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	challengedomain "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	continuationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/httpx"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
)

type PermissionWrapper func(string, app.HandlerFunc) app.HandlerFunc

type Handler struct {
	closure facade.HumanClosure
	reader  facade.TaskReviewReader
	auth    authorizationfacade.AuthFacade
}

func New(closure facade.HumanClosure, reader facade.TaskReviewReader) *Handler {
	return &Handler{closure: closure, reader: reader}
}

func (h *Handler) BindAuthorization(auth authorizationfacade.AuthFacade) { h.auth = auth }

func (h *Handler) Mount(router route.IRouter, wrap PermissionWrapper) {
	router.POST("/workforce/review-runs/:reviewRunId/decisions", wrap("workforce:review:approve", h.decide))
	router.GET("/workforce/tasks/:taskId/independent-review/runs", wrap("workforce:task:query", h.listRuns))
	router.GET("/workforce/review-runs/:reviewRunId", wrap("workforce:task:query", h.runDetail))
}

func (h *Handler) listRuns(ctx context.Context, c *app.RequestContext) {
	taskID, err := strconv.ParseInt(strings.TrimSpace(c.Param("taskId")), 10, 64)
	if err != nil || taskID < 1 {
		writeError(c, apperrors.NotFound("independent review not found"), "INDEPENDENT_REVIEW_NOT_FOUND")
		return
	}
	tenantID, ok := requestTenant(c)
	if !ok {
		writeError(c, apperrors.NotFound("independent review not found"), "INDEPENDENT_REVIEW_NOT_FOUND")
		return
	}
	var cursor int64
	if raw := strings.TrimSpace(string(c.Query("cursor"))); raw != "" {
		cursor, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || cursor < 1 {
			writeError(c, apperrors.Params("cursor is invalid"), "INVALID_CURSOR")
			return
		}
	}
	limit := 20
	if raw := strings.TrimSpace(string(c.Query("limit"))); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 50 {
			writeError(c, apperrors.Params("limit is invalid"), "INVALID_LIMIT")
			return
		}
	}
	page, err := h.reader.ListRunsForTask(ctx, tenantID, taskID, cursor, limit)
	if err != nil {
		h.writeDecisionError(c, err)
		return
	}
	response.Success(c, page)
}

func (h *Handler) runDetail(ctx context.Context, c *app.RequestContext) {
	runID, err := strconv.ParseInt(strings.TrimSpace(c.Param("reviewRunId")), 10, 64)
	tenantID, ok := requestTenant(c)
	if err != nil || runID < 1 || !ok {
		writeError(c, apperrors.NotFound("independent review not found"), "INDEPENDENT_REVIEW_NOT_FOUND")
		return
	}
	item, err := h.reader.RunDetailForTask(ctx, tenantID, runID)
	if err != nil {
		h.writeDecisionError(c, err)
		return
	}
	response.Success(c, item)
}

func requestTenant(c *app.RequestContext) (int64, bool) {
	scope, err := securitycontext.ResolveOrganizationScope(securitycontext.Get(c))
	return scope.OrgID, err == nil && scope.OrgID > 0
}

type decisionRequest struct {
	Action           string `json:"action" validate:"required"`
	Reason           string `json:"reason" validate:"required"`
	RevisionGuidance string `json:"revisionGuidance"`
	IdempotencyKey   string `json:"idempotencyKey" validate:"required"`
}

func (h *Handler) decide(ctx context.Context, c *app.RequestContext) {
	runID, err := strconv.ParseInt(strings.TrimSpace(c.Param("reviewRunId")), 10, 64)
	if err != nil || runID < 1 {
		writeError(c, apperrors.Params("review run id is invalid"), "INDEPENDENT_REVIEW_NOT_FOUND")
		return
	}
	var body decisionRequest
	if err = httpx.BindAndValidate(c, &body); err != nil {
		writeError(c, apperrors.Params("independent review decision is invalid"), "INDEPENDENT_REVIEW_REASON_INVALID")
		return
	}
	body.Action = strings.ToUpper(strings.TrimSpace(body.Action))
	body.IdempotencyKey = strings.TrimSpace(body.IdempotencyKey)
	body.Reason, err = domain.NormalizeHumanReason(body.Reason)
	if err != nil || !domain.ValidateHumanAction(body.Action) {
		writeError(c, apperrors.Params("independent review decision is invalid"), "INDEPENDENT_REVIEW_REASON_INVALID")
		return
	}
	body.RevisionGuidance, err = domain.NormalizeRevisionGuidance(body.Action, body.RevisionGuidance)
	if err != nil {
		writeError(c, apperrors.Params("independent review revision guidance is invalid"), "INDEPENDENT_REVIEW_GUIDANCE_INVALID")
		return
	}
	user := securitycontext.Require(c)
	scope, scopeErr := securitycontext.ResolveOrganizationScope(securitycontext.Get(c))
	if scopeErr != nil || scope.OrgID < 1 || user.UserID < 1 {
		writeError(c, apperrors.NotFound("independent review not found"), "INDEPENDENT_REVIEW_NOT_FOUND")
		return
	}
	binding := domain.HumanDecisionOperationBinding(runID, body.Action, body.Reason, body.RevisionGuidance, body.IdempotencyKey)
	proof, err := h.ensureStepUp(ctx, c, user.UserID, user.Username, user.SessionID, user.Source, binding)
	if err != nil {
		response.Error(c, err)
		return
	}
	result, _, err := h.closure.Decide(ctx, facade.HumanDecisionRequest{TenantID: scope.OrgID, ReviewRunID: runID, ActorUserID: user.UserID, Action: body.Action, Reason: body.Reason, RevisionGuidance: body.RevisionGuidance, IdempotencyKey: body.IdempotencyKey, Proof: proof})
	if err != nil {
		h.writeDecisionError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) ensureStepUp(ctx context.Context, c *app.RequestContext, userID int64, username, sessionID, source, binding string) (stepup.ProofMetadata, error) {
	if h.auth == nil {
		return stepup.ProofMetadata{}, apperrors.ServiceUnavailable("independent review step-up is unavailable").WithDetails(map[string]string{"reasonCode": "INDEPENDENT_REVIEW_STEP_UP_UNAVAILABLE"})
	}
	scope := authorizationfacade.RequestScope{UserID: userID, Username: username, IPAddress: c.ClientIP(), UserAgent: string(c.UserAgent()), DeviceID: strings.TrimSpace(string(c.Request.Header.Peek("X-Device-Id"))), TenantID: strings.TrimSpace(string(c.Request.Header.Peek("X-Tenant-Id"))), SessionID: sessionID, Source: source}
	flowNonce := strings.TrimSpace(string(c.Request.Header.Peek("Flow-Nonce")))
	if flowNonce == "" {
		flowNonce = "workforce-review-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	}
	proofToken := strings.TrimSpace(string(c.Request.Header.Peek("Proof-Token")))
	if proofToken == "" {
		challenge, err := h.auth.CreateStepUpChallenge(ctx, scope, authorizationfacade.StepUpChallengeRequest{BusinessAction: string(challengedomain.BusinessActionWorkforceIndependentReviewDecision), FlowNonce: flowNonce, OperationBinding: binding})
		if err != nil {
			return stepup.ProofMetadata{}, err
		}
		return stepup.ProofMetadata{}, apperrors.ChallengeRequired("", map[string]any{"challengeIdentifier": challenge.ChallengeIdentifier, "challengeState": challenge.ChallengeState, "effectiveTimeToLiveSeconds": challenge.EffectiveTimeToLiveSeconds, "requiredAssuranceLevel": challenge.RequiredAssuranceLevel, "actualChallengeTypeNames": challenge.ActualChallengeTypeNames, "flowNonce": flowNonce, "steps": challenge.Steps, "businessAction": domain.BusinessActionHumanDecision, "operationBinding": binding})
	}
	token, err := h.auth.VerifyStepUp(ctx, scope, authorizationfacade.StepUpVerifyRequest{ProofToken: proofToken, BusinessAction: domain.BusinessActionHumanDecision, FlowNonce: flowNonce, OperationBinding: binding, ConsumeOnce: true})
	if err != nil || token == nil {
		if err != nil {
			return stepup.ProofMetadata{}, err
		}
		return stepup.ProofMetadata{}, apperrors.Forbidden("step-up proof verification failed")
	}
	if (token.BusinessAction != "" && token.BusinessAction != domain.BusinessActionHumanDecision) || (token.OperationBinding != "" && token.OperationBinding != binding) {
		return stepup.ProofMetadata{}, apperrors.Forbidden("step-up proof does not match this decision")
	}
	proof := stepup.ProofMetadata{BusinessAction: domain.BusinessActionHumanDecision, OperationBinding: binding, ProofIdentifier: token.TokenUniqueIdentifier, ChallengeIdentifier: token.ChallengeID, AssuranceLevel: "AAL2", AuthenticationMethods: append([]string(nil), token.AuthenticationMethodNames...)}
	securitycontext.SetStepUpProofAudit(c, securitycontext.StepUpProofAudit{BusinessAction: proof.BusinessAction, OperationBinding: proof.OperationBinding, ProofIdentifier: proof.ProofIdentifier, ChallengeIdentifier: proof.ChallengeIdentifier, AssuranceLevel: proof.AssuranceLevel, AuthenticationMethods: proof.AuthenticationMethods})
	return proof, nil
}

func (h *Handler) writeDecisionError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidReviewInput):
		writeError(c, apperrors.ObjectState("independent review context is unavailable"), "INDEPENDENT_REVIEW_CONTEXT_UNAVAILABLE")
	case errors.Is(err, domain.ErrNotFound):
		writeError(c, apperrors.NotFound("independent review not found"), "INDEPENDENT_REVIEW_NOT_FOUND")
	case errors.Is(err, domain.ErrHumanReasonInvalid):
		writeError(c, apperrors.Params("independent review reason is invalid"), "INDEPENDENT_REVIEW_REASON_INVALID")
	case errors.Is(err, domain.ErrHumanGuidanceInvalid):
		writeError(c, apperrors.Params("independent review revision guidance is invalid"), "INDEPENDENT_REVIEW_GUIDANCE_INVALID")
	case errors.Is(err, domain.ErrHumanGuidanceRequired):
		writeError(c, apperrors.Params("independent review revision guidance is required"), "INDEPENDENT_REVIEW_GUIDANCE_REQUIRED")
	case errors.Is(err, domain.ErrHumanIdempotency):
		writeError(c, apperrors.ObjectState("independent review decision conflicts with an existing request"), "INDEPENDENT_REVIEW_IDEMPOTENCY_CONFLICT")
	case errors.Is(err, continuationfacade.ErrReviewRevisionCheckpointUnavailable):
		writeError(c, apperrors.ObjectState("trusted continuation checkpoint is unavailable"), "INDEPENDENT_REVIEW_CHECKPOINT_UNAVAILABLE")
	case errors.Is(err, domain.ErrHumanDecisionStale):
		writeError(c, apperrors.ObjectState("independent review decision is stale"), "INDEPENDENT_REVIEW_STALE")
	case errors.Is(err, domain.ErrDeterministicAuthorityInvalid):
		writeError(c, apperrors.ObjectState("trusted deterministic validation cannot authorize delivery"), "INDEPENDENT_REVIEW_DETERMINISTIC_AUTHORITY_INVALID")
	case errors.Is(err, domain.ErrHumanActionNotAllowed):
		writeError(c, apperrors.ObjectState("independent review action is not allowed"), "INDEPENDENT_REVIEW_ACTION_NOT_ALLOWED")
	default:
		writeError(c, apperrors.System("independent review decision failed"), "INTERNAL_ERROR")
	}
}

func writeError(c *app.RequestContext, err *apperrors.AppError, reason string) {
	response.Error(c, err.WithDetails(map[string]string{"reasonCode": reason}))
}
