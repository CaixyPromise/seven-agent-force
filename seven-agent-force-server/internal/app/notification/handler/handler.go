package handler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	notificationapp "github.com/CaixyPromise/seven-agent/force/internal/app/notification/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/adaptor"
)

type Handler struct {
	service             *notificationapp.Service
	auth                authorizationfacade.AuthFacade
	diagnosticTransport DiagnosticTransportPolicy
}

func NewHandler(service *notificationapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Readiness(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := requireCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	organizationID := int64(0)
	if scope, scopeErr := securitycontext.ResolveOrganizationScope(securitycontext.Get(reqCtx)); scopeErr == nil {
		organizationID = scope.OrgID
	}
	result, err := h.service.ResolveReadinessForOrganization(ctx, organizationID, reqCtx.Query("sceneCode"), userID)
	write(reqCtx, result, err)
}

func (h *Handler) MyWorkPreferences(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := requireCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	organizationID, err := currentOrganizationID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.service.ListMyWorkPreferences(ctx, organizationID, userID)
	write(reqCtx, result, err)
}

func (h *Handler) UpdateMyWorkPreference(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := requireCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	organizationID, err := currentOrganizationID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request facade.WorkPreferenceUpdate
	if err = reqCtx.BindAndValidate(&request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.service.UpdateMyWorkPreference(ctx, organizationID, userID, string(reqCtx.Param("eventFamily")), string(reqCtx.Param("channelKind")), request)
	write(reqCtx, result, err)
}

func (h *Handler) ListGroupRoutes(ctx context.Context, reqCtx *app.RequestContext) {
	orgID, err := currentOrganizationID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.service.ListGroupRoutes(ctx, orgID)
	write(reqCtx, result, err)
}

func (h *Handler) GetGroupRoute(ctx context.Context, reqCtx *app.RequestContext) {
	orgID, err := currentOrganizationID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	routeID, err := notificationRouteID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.service.GetGroupRoute(ctx, orgID, routeID)
	write(reqCtx, result, err)
}

func (h *Handler) SaveGroupRoute(ctx context.Context, reqCtx *app.RequestContext) {
	orgID, err := currentOrganizationID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	actorID, err := requireCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request facade.GroupRouteSaveRequest
	if err = reqCtx.BindAndValidate(&request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if raw := strings.TrimSpace(string(reqCtx.Param("routeId"))); raw != "" {
		request.ID, err = strictPositiveInt64(raw)
		if err != nil {
			response.Error(reqCtx, err)
			return
		}
	}
	bindingDigest := sha256.Sum256([]byte(strings.TrimSpace(request.Name)))
	if _, err = h.ensureNotificationMutationStepUp(ctx, reqCtx, "NOTIFICATION_GROUP_ROUTE_SAVE", fmt.Sprintf("group-route:save:%d:%d:%x", request.ID, request.ExpectedVersion, bindingDigest[:8])); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.service.SaveGroupRoute(ctx, orgID, actorID, request)
	write(reqCtx, result, err)
}

func (h *Handler) ProbeGroupRoute(ctx context.Context, reqCtx *app.RequestContext) {
	orgID, err := currentOrganizationID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	actorID, err := requireCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	routeID, err := notificationRouteID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request facade.GroupRouteStateRequest
	if err = reqCtx.BindAndValidate(&request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.service.ProbeGroupRoute(ctx, orgID, actorID, routeID, request.ExpectedVersion)
	write(reqCtx, result, err)
}

func (h *Handler) SetGroupRouteState(action string) app.HandlerFunc {
	return func(ctx context.Context, reqCtx *app.RequestContext) {
		orgID, err := currentOrganizationID(reqCtx)
		if err != nil {
			response.Error(reqCtx, err)
			return
		}
		actorID, err := requireCurrentUserID(reqCtx)
		if err != nil {
			response.Error(reqCtx, err)
			return
		}
		routeID, err := notificationRouteID(reqCtx)
		if err != nil {
			response.Error(reqCtx, err)
			return
		}
		var request facade.GroupRouteStateRequest
		if err = reqCtx.BindAndValidate(&request); err != nil {
			response.Error(reqCtx, err)
			return
		}
		if _, err = h.ensureNotificationMutationStepUp(ctx, reqCtx, "NOTIFICATION_GROUP_ROUTE_"+strings.ToUpper(action), fmt.Sprintf("group-route:%s:%d:%d", strings.ToLower(action), routeID, request.ExpectedVersion)); err != nil {
			response.Error(reqCtx, err)
			return
		}
		result, err := h.service.SetGroupRouteState(ctx, orgID, actorID, routeID, request, action)
		write(reqCtx, result, err)
	}
}

func (h *Handler) GroupRouteImpact(ctx context.Context, reqCtx *app.RequestContext) {
	orgID, err := currentOrganizationID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	routeID, err := notificationRouteID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.service.GetGroupRouteImpact(ctx, orgID, routeID)
	write(reqCtx, result, err)
}

func (h *Handler) DeleteGroupRoute(ctx context.Context, reqCtx *app.RequestContext) {
	orgID, err := currentOrganizationID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	routeID, err := notificationRouteID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if _, err = h.ensureNotificationMutationStepUp(ctx, reqCtx, "NOTIFICATION_GROUP_ROUTE_DELETE", fmt.Sprintf("group-route:delete:%d", routeID)); err != nil {
		response.Error(reqCtx, err)
		return
	}
	write(reqCtx, map[string]bool{"deleted": true}, h.service.DeleteGroupRoute(ctx, orgID, routeID))
}

func (h *Handler) ensureNotificationMutationStepUp(ctx context.Context, reqCtx *app.RequestContext, businessAction, operationBinding string) (stepup.ProofMetadata, error) {
	if h == nil || h.auth == nil {
		return stepup.ProofMetadata{}, apperrors.System("authorization auth facade未配置")
	}
	scope, err := deliveryDiagnosticRequestScope(reqCtx)
	if err != nil {
		return stepup.ProofMetadata{}, err
	}
	proofToken := strings.TrimSpace(string(reqCtx.Request.Header.Peek("Proof-Token")))
	flowNonce := deliveryDiagnosticFlowNonce(strings.TrimSpace(string(reqCtx.Request.Header.Peek("Flow-Nonce"))), businessAction)
	if proofToken != "" {
		token, verifyErr := h.auth.VerifyStepUp(ctx, scope, authorizationfacade.StepUpVerifyRequest{ProofToken: proofToken, BusinessAction: businessAction, FlowNonce: flowNonce, OperationBinding: operationBinding, ConsumeOnce: true})
		if verifyErr != nil {
			return stepup.ProofMetadata{}, verifyErr
		}
		proof := deliveryDiagnosticProofMetadata(token, businessAction, operationBinding)
		if err = stepup.Require(proof, businessAction, operationBinding); err != nil {
			return stepup.ProofMetadata{}, err
		}
		securitycontext.SetStepUpProofAudit(reqCtx, securitycontext.StepUpProofAudit{BusinessAction: proof.BusinessAction, OperationBinding: proof.OperationBinding, ProofIdentifier: proof.ProofIdentifier, ChallengeIdentifier: proof.ChallengeIdentifier, AssuranceLevel: proof.AssuranceLevel, AuthenticationMethods: append([]string(nil), proof.AuthenticationMethods...)})
		return proof, nil
	}
	challenge, err := h.auth.CreateStepUpChallenge(ctx, scope, authorizationfacade.StepUpChallengeRequest{BusinessAction: businessAction, FlowNonce: flowNonce, OperationBinding: operationBinding})
	if err != nil {
		return stepup.ProofMetadata{}, err
	}
	if challenge == nil {
		return stepup.ProofMetadata{}, apperrors.System("step-up challenge未返回")
	}
	return stepup.ProofMetadata{}, apperrors.ChallengeRequired("", map[string]any{"challengeIdentifier": challenge.ChallengeIdentifier, "challengeState": challenge.ChallengeState, "effectiveTimeToLiveSeconds": challenge.EffectiveTimeToLiveSeconds, "requiredAssuranceLevel": challenge.RequiredAssuranceLevel, "resolvedAssuranceLevel": challenge.ResolvedAssuranceLevel, "recommendedStepIdentifier": challenge.RecommendedStepIdentifier, "actualChallengeTypeNames": challenge.ActualChallengeTypeNames, "flowNonce": flowNonce, "steps": challenge.Steps, "operationBinding": operationBinding})
}

func (h *Handler) AvailableGroupRoutes(ctx context.Context, reqCtx *app.RequestContext) {
	orgID, err := currentOrganizationID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	userID, err := requireCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.service.ListAvailableGroupRoutes(ctx, orgID, userID, strings.TrimSpace(reqCtx.Query("sceneCode")))
	write(reqCtx, result, err)
}

func (h *Handler) FeishuGroupCandidates(ctx context.Context, reqCtx *app.RequestContext) {
	orgID, err := currentOrganizationID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	connectionID, err := strictPositiveInt64(strings.TrimSpace(reqCtx.Query("connectionId")))
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.service.DiscoverFeishuGroupCandidates(ctx, orgID, connectionID)
	write(reqCtx, result, err)
}

func currentOrganizationID(reqCtx *app.RequestContext) (int64, error) {
	scope, err := securitycontext.ResolveOrganizationScope(securitycontext.Get(reqCtx))
	if err != nil {
		return 0, err
	}
	if scope.OrgID <= 0 {
		return 0, apperrors.Params("组织身份无效")
	}
	return scope.OrgID, nil
}

func notificationRouteID(reqCtx *app.RequestContext) (int64, error) {
	return strictPositiveInt64(strings.TrimSpace(string(reqCtx.Param("routeId"))))
}

func strictPositiveInt64(raw string) (int64, error) {
	if raw == "" || strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "+-.eE") {
		return 0, apperrors.Params("通知群标识无效")
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, apperrors.Params("通知群标识无效")
	}
	return value, nil
}

// BindAuthorization supplies the cross-module facade used only to issue or
// consume a fresh, single-use step-up proof for a diagnostic-content read.
func (h *Handler) BindAuthorization(auth authorizationfacade.AuthFacade) {
	if h != nil {
		h.auth = auth
	}
}

// BindDiagnosticTransportPolicy binds deployment-owned transport trust. A
// caller cannot opt into a proxy or HTTPS assertion through the API request.
func (h *Handler) BindDiagnosticTransportPolicy(policy DiagnosticTransportPolicy) {
	if h != nil {
		h.diagnosticTransport = policy.normalized()
	}
}

// DiagnosticTransportPolicy accepts a diagnostic-content response only when
// the raw network peer is trusted and has asserted HTTPS. It intentionally
// does not use ClientIP or X-Forwarded-For, because those may be supplied by
// an untrusted client. TrustedCIDRs are deliberately limited to narrow,
// private proxy pools; public proxies must be listed by exact address. A
// loopback exception exists solely for explicit local dev/test wiring and
// never applies to arbitrary private-network addresses.
type DiagnosticTransportPolicy struct {
	AllowLoopbackInsecure bool
	TrustedProxies        []string
	TrustedCIDRs          []string
}

func (p DiagnosticTransportPolicy) normalized() DiagnosticTransportPolicy {
	p.TrustedProxies = append([]string(nil), p.TrustedProxies...)
	trustedCIDRs := make([]string, 0, len(p.TrustedCIDRs))
	for _, raw := range p.TrustedCIDRs {
		if _, ok := diagnosticTrustedProxyCIDR(raw); ok {
			trustedCIDRs = append(trustedCIDRs, strings.TrimSpace(raw))
		}
	}
	p.TrustedCIDRs = trustedCIDRs
	return p
}

func (p DiagnosticTransportPolicy) Allows(reqCtx *app.RequestContext) bool {
	peer := diagnosticRawPeerIP(reqCtx)
	if peer == nil {
		return false
	}
	if p.AllowLoopbackInsecure && peer.IsLoopback() {
		return true
	}
	if !p.isTrustedPeer(peer) {
		return false
	}
	// The direct raw peer is a configured proxy, but an upstream client could
	// still inject a comma-separated value if that proxy merely appends the
	// header. Require the proxy to replace it with exactly "https".
	forwardedProto := strings.TrimSpace(string(reqCtx.Request.Header.Peek("X-Forwarded-Proto")))
	return strings.EqualFold(forwardedProto, "https")
}

func (p DiagnosticTransportPolicy) isTrustedPeer(peer net.IP) bool {
	if peer == nil {
		return false
	}
	for _, raw := range p.TrustedProxies {
		if configured := net.ParseIP(strings.TrimSpace(raw)); configured != nil && configured.Equal(peer) {
			return true
		}
	}
	for _, raw := range p.TrustedCIDRs {
		if diagnosticTrustedProxyCIDRContains(raw, peer) {
			return true
		}
	}
	return false
}

var diagnosticPrivateProxyCIDRs = []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("fc00::/7"),
}

// diagnosticTrustedProxyCIDR accepts only a narrow, wholly private subnet.
// A public reverse proxy must use TrustedProxies with its exact raw peer IP;
// accepting a broad public or catch-all range would allow arbitrary clients to
// forge X-Forwarded-Proto directly to the application.
func diagnosticTrustedProxyCIDR(raw string) (netip.Prefix, bool) {
	prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
	if err != nil {
		return netip.Prefix{}, false
	}
	prefix = prefix.Masked()
	address := prefix.Addr()
	if address.Is4In6() {
		return netip.Prefix{}, false
	}
	if address.Is4() && prefix.Bits() < 24 {
		return netip.Prefix{}, false
	}
	if address.Is6() && prefix.Bits() < 64 {
		return netip.Prefix{}, false
	}
	for _, privateBlock := range diagnosticPrivateProxyCIDRs {
		if address.BitLen() == privateBlock.Addr().BitLen() &&
			prefix.Bits() >= privateBlock.Bits() &&
			privateBlock.Contains(address) {
			return prefix, true
		}
	}
	return netip.Prefix{}, false
}

func diagnosticTrustedProxyCIDRContains(raw string, peer net.IP) bool {
	prefix, ok := diagnosticTrustedProxyCIDR(raw)
	if !ok {
		return false
	}
	address, ok := netip.AddrFromSlice(peer)
	if !ok {
		return false
	}
	// net.ParseIP commonly represents an ordinary IPv4 socket peer in its
	// 16-byte IPv4-mapped form. Normalize the observed peer, while still
	// rejecting IPv4-mapped *configured CIDRs* above so trust configuration is
	// explicit and canonical.
	address = address.Unmap()
	return prefix.Contains(address)
}

func diagnosticRawPeerIP(reqCtx *app.RequestContext) net.IP {
	if reqCtx == nil || reqCtx.RemoteAddr() == nil {
		return nil
	}
	raw := strings.TrimSpace(reqCtx.RemoteAddr().String())
	if host, _, err := net.SplitHostPort(raw); err == nil {
		raw = host
	}
	return net.ParseIP(strings.Trim(strings.TrimSpace(raw), "[]"))
}

func (h *Handler) ListChannels(ctx context.Context, reqCtx *app.RequestContext) {
	status := optionalInt(reqCtx.Query("status"))
	result, err := h.service.ListChannels(ctx, domain.ChannelQuery{
		Keyword:     reqCtx.Query("keyword"),
		ChannelType: reqCtx.Query("channelType"),
		Status:      status,
		Current:     queryInt(reqCtx, "current", 1),
		PageSize:    queryInt(reqCtx, "pageSize", queryInt(reqCtx, "size", 20)),
	})
	write(reqCtx, result, err)
}

func (h *Handler) UpsertChannel(ctx context.Context, reqCtx *app.RequestContext) {
	var request facade.ChannelUpsertRequest
	if err := reqCtx.BindAndValidate(&request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.service.UpsertChannel(ctx, request, currentUserID(reqCtx))
	write(reqCtx, result, err)
}

// ListTemplateDefinitions exposes the versioned-template workspace.
func (h *Handler) ListTemplateDefinitions(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := h.service.ListTemplateDefinitions(ctx, domain.TemplateDefinitionQuery{
		Keyword:  reqCtx.Query("keyword"),
		Current:  queryInt(reqCtx, "current", 1),
		PageSize: queryInt(reqCtx, "pageSize", queryInt(reqCtx, "size", 20)),
	})
	write(reqCtx, result, err)
}

func (h *Handler) GetTemplateDefinition(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := h.service.GetTemplateDefinition(ctx, strings.TrimSpace(string(reqCtx.Param("templateCode"))))
	write(reqCtx, result, err)
}

func (h *Handler) CreateTemplateDefinition(ctx context.Context, reqCtx *app.RequestContext) {
	var request facade.TemplateDefinitionCreateRequest
	if err := reqCtx.BindAndValidate(&request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.service.CreateTemplateDefinition(ctx, request, currentUserID(reqCtx))
	write(reqCtx, result, err)
}

func (h *Handler) SaveTemplateRevisionDraft(ctx context.Context, reqCtx *app.RequestContext) {
	revisionID, err := notificationRevisionID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request facade.TemplateRevisionSaveRequest
	if err := reqCtx.BindAndValidate(&request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.service.SaveTemplateRevisionDraft(ctx, revisionID, request, currentUserID(reqCtx))
	write(reqCtx, result, err)
}

func (h *Handler) CreateTemplateDraftFromPublished(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := h.service.CreateTemplateDraftFromPublished(ctx, strings.TrimSpace(string(reqCtx.Param("templateCode"))), currentUserID(reqCtx))
	write(reqCtx, result, err)
}

func (h *Handler) PreviewTemplateRevision(ctx context.Context, reqCtx *app.RequestContext) {
	var request facade.TemplateRevisionPreviewRequest
	if err := reqCtx.BindAndValidate(&request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.service.PreviewTemplateRevision(ctx, request)
	write(reqCtx, result, err)
}

func (h *Handler) PublishTemplateRevision(ctx context.Context, reqCtx *app.RequestContext) {
	revisionID, err := notificationRevisionID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request facade.TemplateRevisionPublishRequest
	if err := reqCtx.BindAndValidate(&request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.service.PublishTemplateRevision(ctx, revisionID, request, currentUserID(reqCtx))
	write(reqCtx, result, err)
}

// ListSceneDefinitions exposes the versioned scene workspace.
func (h *Handler) ListSceneDefinitions(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := h.service.ListSceneDefinitions(ctx, domain.SceneDefinitionQuery{
		Keyword:  reqCtx.Query("keyword"),
		Current:  queryInt(reqCtx, "current", 1),
		PageSize: queryInt(reqCtx, "pageSize", queryInt(reqCtx, "size", 20)),
	})
	write(reqCtx, result, err)
}

func (h *Handler) GetSceneDefinition(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := h.service.GetSceneDefinition(ctx, strings.TrimSpace(string(reqCtx.Param("sceneCode"))), reqCtx.Query("receiverKind"))
	write(reqCtx, result, err)
}

func (h *Handler) CreateSceneDefinition(ctx context.Context, reqCtx *app.RequestContext) {
	var request facade.SceneDefinitionCreateRequest
	if err := reqCtx.BindAndValidate(&request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.service.CreateSceneDefinition(ctx, request, currentUserID(reqCtx))
	write(reqCtx, result, err)
}

func (h *Handler) SaveSceneRevisionDraft(ctx context.Context, reqCtx *app.RequestContext) {
	revisionID, err := notificationRevisionID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request facade.SceneRevisionSaveRequest
	if err := reqCtx.BindAndValidate(&request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.service.SaveSceneRevisionDraft(ctx, revisionID, request, currentUserID(reqCtx))
	write(reqCtx, result, err)
}

func (h *Handler) CreateSceneDraftFromPublished(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := h.service.CreateSceneDraftFromPublished(ctx, strings.TrimSpace(string(reqCtx.Param("sceneCode"))), reqCtx.Query("receiverKind"), currentUserID(reqCtx))
	write(reqCtx, result, err)
}

func (h *Handler) PublishSceneRevision(ctx context.Context, reqCtx *app.RequestContext) {
	revisionID, err := notificationRevisionID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request facade.SceneRevisionPublishRequest
	if err := reqCtx.BindAndValidate(&request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.service.PublishSceneRevision(ctx, revisionID, request, currentUserID(reqCtx))
	write(reqCtx, result, err)
}

func (h *Handler) StopSceneDefinition(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := h.service.StopSceneDefinition(ctx, strings.TrimSpace(string(reqCtx.Param("sceneCode"))), reqCtx.Query("receiverKind"), currentUserID(reqCtx))
	write(reqCtx, result, err)
}

func (h *Handler) ListDeliveries(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := h.service.ListDeliveries(ctx, domain.DeliveryQuery{
		Keyword:     reqCtx.Query("keyword"),
		SceneCode:   reqCtx.Query("sceneCode"),
		ChannelCode: reqCtx.Query("channelCode"),
		Status:      strings.ToUpper(reqCtx.Query("status")),
		Current:     queryInt(reqCtx, "current", 1),
		PageSize:    queryInt(reqCtx, "pageSize", queryInt(reqCtx, "size", 20)),
	})
	write(reqCtx, result, err)
}

// ReadDeliveryDiagnosticContent is the only management endpoint that can
// return rendered delivery content. It is intentionally one-record-only,
// requires a stated reason, checks a tier-specific capability, and refuses
// ordinary or untrusted transport before plaintext is read from storage.
func (h *Handler) ReadDeliveryDiagnosticContent(ctx context.Context, reqCtx *app.RequestContext) {
	setDiagnosticNoStoreHeaders(reqCtx)
	actorID, err := requireCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request facade.DeliveryDiagnosticContentRequest
	if err := reqCtx.BindAndValidate(&request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	deliveryID := strings.TrimSpace(string(reqCtx.Param("deliveryId")))
	traceID := xcontext.EnsureTraceID(reqCtx)
	reasonCode, ticketReference, err := domain.ValidateDeliveryDiagnosticReason(request.ReasonCode, request.TicketReference)
	if err != nil {
		// Keep the audit useful without preserving an invalid free-form reason or
		// ticket. This request cannot reach a content read.
		_ = h.auditDeliveryDiagnostic(ctx, notificationapp.DeliveryDiagnosticAuditCommand{
			DeliveryID: deliveryID, ReasonCode: domain.DeliveryDiagnosticReasonOther, ActorID: actorID,
			TraceID: traceID, ContentTier: domain.DeliveryContentTierSensitive,
			ResultCode: domain.DeliveryDiagnosticResultDenied,
		})
		response.Error(reqCtx, apperrors.Params("诊断用途或工单编号不合法"))
		return
	}
	if deliveryID == "" {
		response.Error(reqCtx, apperrors.Params("投递标识不合法"))
		return
	}
	// Check the general capability before resolving the delivery. Otherwise a
	// caller without diagnostic access could compare a not-found response with
	// a permission-denied response and enumerate current-scope delivery IDs.
	if !hasExplicitDeliveryDiagnosticPermission(reqCtx, "system:notification:delivery:diagnostic") {
		h.writeDeliveryDiagnosticDenied(ctx, reqCtx, notificationapp.DeliveryDiagnosticAuditCommand{
			DeliveryID: deliveryID, ReasonCode: reasonCode, TicketReference: ticketReference, ActorID: actorID,
			TraceID: traceID, ContentTier: domain.DeliveryContentTierSensitive, ResultCode: domain.DeliveryDiagnosticResultDenied,
		}, apperrors.Forbidden("无权限查看投递内容"))
		return
	}
	tier, err := h.service.DeliveryDiagnosticTier(ctx, deliveryID)
	if err != nil {
		h.writeDeliveryDiagnosticDenied(ctx, reqCtx, notificationapp.DeliveryDiagnosticAuditCommand{
			DeliveryID: deliveryID, ReasonCode: reasonCode, TicketReference: ticketReference, ActorID: actorID,
			TraceID: traceID, ContentTier: domain.DeliveryContentTierSensitive,
			ResultCode: domain.DeliveryDiagnosticResultNotFound,
		}, apperrors.Forbidden("无权限查看投递内容"))
		return
	}
	tierPermission := domain.DeliveryDiagnosticPermission(tier)
	if !hasExplicitDeliveryDiagnosticPermission(reqCtx, tierPermission) {
		h.writeDeliveryDiagnosticDenied(ctx, reqCtx, notificationapp.DeliveryDiagnosticAuditCommand{
			DeliveryID: deliveryID, ReasonCode: reasonCode, TicketReference: ticketReference, ActorID: actorID,
			TraceID: traceID, ContentTier: tier, ResultCode: domain.DeliveryDiagnosticResultDenied,
		}, apperrors.Forbidden("无权限查看投递内容"))
		return
	}
	if !h.diagnosticTransport.Allows(reqCtx) {
		h.writeDeliveryDiagnosticDenied(ctx, reqCtx, notificationapp.DeliveryDiagnosticAuditCommand{
			DeliveryID: deliveryID, ReasonCode: reasonCode, TicketReference: ticketReference, ActorID: actorID,
			TraceID: traceID, ContentTier: tier, ResultCode: domain.DeliveryDiagnosticResultTransportDenied,
		}, apperrors.Forbidden("该操作只能通过受保护连接完成"))
		return
	}
	proof := stepup.ProofMetadata{}
	if domain.DeliveryDiagnosticRequiresStepUp(tier) {
		proof, err = h.ensureDeliveryDiagnosticStepUp(ctx, reqCtx, deliveryID, reasonCode, ticketReference)
		if err != nil {
			h.writeDeliveryDiagnosticDenied(ctx, reqCtx, notificationapp.DeliveryDiagnosticAuditCommand{
				DeliveryID: deliveryID, ReasonCode: reasonCode, TicketReference: ticketReference, ActorID: actorID,
				TraceID: traceID, ContentTier: tier, ResultCode: domain.DeliveryDiagnosticResultStepUpRequired,
			}, err)
			return
		}
	}
	result, err := h.service.ReadDeliveryDiagnosticContent(ctx, notificationapp.DeliveryDiagnosticReadCommand{
		DeliveryID: deliveryID, ReasonCode: reasonCode, TicketReference: ticketReference, ActorID: actorID,
		TraceID: traceID, GrantedPermission: tierPermission, StepUpProof: proof,
	})
	write(reqCtx, result, err)
}

func (h *Handler) auditDeliveryDiagnostic(ctx context.Context, command notificationapp.DeliveryDiagnosticAuditCommand) error {
	if h == nil || h.service == nil {
		return fmt.Errorf("notification delivery diagnostics is not configured")
	}
	return h.service.AuditDeliveryDiagnosticAttempt(ctx, command)
}

// hasExplicitDeliveryDiagnosticPermission deliberately does not reuse the
// framework-wide administrator shortcut. Reading one rendered delivery is a
// separate break-glass capability: a principal must carry the actual
// diagnostic permission (or an explicitly assigned matching wildcard) in its
// authenticated permission set. This keeps a broad "developer" or generic
// administrator role from silently gaining plaintext access.
func hasExplicitDeliveryDiagnosticPermission(reqCtx *app.RequestContext, permission string) bool {
	user := securitycontext.Get(reqCtx)
	if user == nil || user.IsAnonymous {
		return false
	}
	for _, granted := range user.Permissions {
		if securitycontext.PermissionMatches(granted, permission) {
			return true
		}
	}
	return false
}

func (h *Handler) writeDeliveryDiagnosticDenied(ctx context.Context, reqCtx *app.RequestContext, command notificationapp.DeliveryDiagnosticAuditCommand, denied error) {
	if auditErr := h.auditDeliveryDiagnostic(ctx, command); auditErr != nil {
		response.Error(reqCtx, auditErr)
		return
	}
	response.Error(reqCtx, denied)
}

func (h *Handler) ensureDeliveryDiagnosticStepUp(ctx context.Context, reqCtx *app.RequestContext, deliveryID, reasonCode, ticketReference string) (stepup.ProofMetadata, error) {
	if h == nil || h.auth == nil {
		return stepup.ProofMetadata{}, apperrors.System("authorization auth facade未配置")
	}
	requestScope, err := deliveryDiagnosticRequestScope(reqCtx)
	if err != nil {
		return stepup.ProofMetadata{}, err
	}
	businessAction := notificationapp.DeliveryDiagnosticBusinessAction()
	operationBinding := notificationapp.DeliveryDiagnosticOperationBinding(deliveryID, reasonCode, ticketReference)
	proofToken := strings.TrimSpace(string(reqCtx.Request.Header.Peek("Proof-Token")))
	flowNonce := deliveryDiagnosticFlowNonce(strings.TrimSpace(string(reqCtx.Request.Header.Peek("Flow-Nonce"))), businessAction)
	if proofToken != "" {
		token, err := h.auth.VerifyStepUp(ctx, requestScope, authorizationfacade.StepUpVerifyRequest{
			ProofToken:       proofToken,
			BusinessAction:   businessAction,
			FlowNonce:        flowNonce,
			OperationBinding: operationBinding,
			ConsumeOnce:      true,
		})
		if err != nil {
			return stepup.ProofMetadata{}, err
		}
		if token == nil {
			return stepup.ProofMetadata{}, apperrors.Forbidden("step-up proof验证失败")
		}
		proof := deliveryDiagnosticProofMetadata(token, businessAction, operationBinding)
		if err := stepup.Require(proof, businessAction, operationBinding); err != nil {
			return stepup.ProofMetadata{}, err
		}
		securitycontext.SetStepUpProofAudit(reqCtx, securitycontext.StepUpProofAudit{
			BusinessAction: proof.BusinessAction, OperationBinding: proof.OperationBinding,
			ProofIdentifier: proof.ProofIdentifier, ChallengeIdentifier: proof.ChallengeIdentifier,
			AssuranceLevel: proof.AssuranceLevel, AuthenticationMethods: append([]string(nil), proof.AuthenticationMethods...),
		})
		return proof, nil
	}
	challenge, err := h.auth.CreateStepUpChallenge(ctx, requestScope, authorizationfacade.StepUpChallengeRequest{
		BusinessAction: businessAction, FlowNonce: flowNonce, OperationBinding: operationBinding,
	})
	if err != nil {
		return stepup.ProofMetadata{}, err
	}
	if challenge == nil {
		return stepup.ProofMetadata{}, apperrors.System("step-up challenge未返回")
	}
	return stepup.ProofMetadata{}, apperrors.ChallengeRequired("", map[string]any{
		"challengeIdentifier":        challenge.ChallengeIdentifier,
		"challengeState":             challenge.ChallengeState,
		"effectiveTimeToLiveSeconds": challenge.EffectiveTimeToLiveSeconds,
		"requiredAssuranceLevel":     challenge.RequiredAssuranceLevel,
		"resolvedAssuranceLevel":     challenge.ResolvedAssuranceLevel,
		"recommendedStepIdentifier":  challenge.RecommendedStepIdentifier,
		"actualChallengeTypeNames":   challenge.ActualChallengeTypeNames,
		"flowNonce":                  flowNonce,
		"steps":                      challenge.Steps,
		"operationBinding":           operationBinding,
	})
}

func deliveryDiagnosticRequestScope(reqCtx *app.RequestContext) (authorizationfacade.RequestScope, error) {
	user := securitycontext.Require(reqCtx)
	if user.UserID <= 0 {
		return authorizationfacade.RequestScope{}, apperrors.Unauthorized("未登录或登录信息失效")
	}
	return authorizationfacade.RequestScope{
		UserID: user.UserID, Username: user.Username, IPAddress: reqCtx.ClientIP(), UserAgent: string(reqCtx.UserAgent()),
		DeviceID:  strings.TrimSpace(string(reqCtx.Request.Header.Peek("X-Device-Id"))),
		TenantID:  strings.TrimSpace(string(reqCtx.Request.Header.Peek("X-Tenant-Id"))),
		SessionID: user.SessionID, Source: user.Source,
	}, nil
}

func deliveryDiagnosticProofMetadata(token *authorizationfacade.StepUpTokenVO, businessAction, operationBinding string) stepup.ProofMetadata {
	if token == nil {
		return stepup.ProofMetadata{}
	}
	return stepup.ProofMetadata{
		BusinessAction:   firstNonBlank(token.BusinessAction, businessAction),
		OperationBinding: firstNonBlank(token.OperationBinding, operationBinding),
		ProofIdentifier:  token.TokenUniqueIdentifier, ChallengeIdentifier: token.ChallengeID,
		AssuranceLevel: "AAL2", AuthenticationMethods: append([]string(nil), token.AuthenticationMethodNames...),
	}
}

func deliveryDiagnosticFlowNonce(flowNonce, businessAction string) string {
	if value := strings.TrimSpace(flowNonce); value != "" {
		return value
	}
	return strings.ToLower(strings.TrimSpace(businessAction)) + "-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
}

func setDiagnosticNoStoreHeaders(reqCtx *app.RequestContext) {
	if reqCtx == nil {
		return
	}
	reqCtx.Response.Header.Set("Cache-Control", "no-store, max-age=0")
	reqCtx.Response.Header.Set("Pragma", "no-cache")
	reqCtx.Response.Header.Set("X-Content-Type-Options", "nosniff")
	reqCtx.Response.Header.Set("Referrer-Policy", "no-referrer")
}

// TestEnterpriseConnection is the non-persistent, privileged probe for a
// saved Feishu or WeCom application connection.
func (h *Handler) TestEnterpriseConnection(ctx context.Context, reqCtx *app.RequestContext) {
	var request facade.EnterpriseConnectionTestRequest
	if err := reqCtx.BindAndValidate(&request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.service.TestEnterpriseConnection(ctx, request)
	write(reqCtx, result, err)
}

// TestStaticConnection probes a saved HTTP Connector or fixed group webhook
// without materializing a notification or a recipient-facing state change.
func (h *Handler) TestStaticConnection(ctx context.Context, reqCtx *app.RequestContext) {
	var request facade.StaticConnectionTestRequest
	if err := reqCtx.BindAndValidate(&request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.service.TestStaticConnection(ctx, request)
	write(reqCtx, result, err)
}

// ListInbox returns the authenticated user's in-app notification projections.
func (h *Handler) ListInbox(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := requireCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	archived := false
	if value := optionalBool(reqCtx.Query("archived")); value != nil {
		archived = *value
	}
	result, err := h.service.ListInbox(ctx, userID, facade.InboxQuery{
		Archived:   archived,
		PageCursor: reqCtx.Query("pageCursor"),
		PageSize:   queryInt(reqCtx, "pageSize", queryInt(reqCtx, "size", 20)),
	})
	write(reqCtx, result, err)
}

// GetInboxRecipient returns one recipient only when it belongs to the current user.
func (h *Handler) GetInboxRecipient(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := requireCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.service.GetInboxRecipient(ctx, userID, strings.TrimSpace(string(reqCtx.Param("recipientId"))))
	write(reqCtx, result, err)
}

// UnreadCount returns the current user's non-archived unread count.
func (h *Handler) UnreadCount(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := requireCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.service.UnreadCount(ctx, userID)
	write(reqCtx, result, err)
}

// UnreadPreview returns a bounded safe preview only after the user opens the
// bell Popover. It never returns full notification content or deep links.
func (h *Handler) UnreadPreview(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := requireCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.service.UnreadPreview(ctx, userID, queryInt(reqCtx, "limit", 5))
	write(reqCtx, result, err)
}

// ListInboxChanges returns compact recipient changes for an already-open
// message center. Invalid tokens request a safe resynchronization response.
func (h *Handler) ListInboxChanges(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := requireCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := h.service.ListInboxChanges(ctx, userID, facade.InboxChangeQuery{
		AfterChangeToken: reqCtx.Query("afterChangeToken"),
		UntilChangeToken: reqCtx.Query("untilChangeToken"),
		Limit:            queryInt(reqCtx, "limit", 50),
	})
	write(reqCtx, result, err)
}

// StreamInbox emits content-free mailbox hints for the authenticated user. It
// deliberately does not read or render preview, list or detail data.
func (h *Handler) StreamInbox(ctx context.Context, reqCtx *app.RequestContext) {
	userID, err := requireCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	events, stop := h.service.SubscribeInboxChanges(userID)
	adaptor.HertzHandler(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer stop()
		flusher, ok := writer.(http.Flusher)
		if !ok {
			http.Error(writer, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("Cache-Control", "no-cache")
		writer.Header().Set("Connection", "keep-alive")
		writer.Header().Set("X-Accel-Buffering", "no")
		writer.WriteHeader(http.StatusOK)
		if err := writeInboxSSEEvent(writer, "connected", map[string]any{}); err != nil {
			return
		}
		flusher.Flush()

		heartbeat := time.NewTicker(20 * time.Second)
		defer heartbeat.Stop()
		for {
			select {
			case <-request.Context().Done():
				return
			case intent, ok := <-events:
				if !ok {
					events = nil
					continue
				}
				hint, hintErr := h.service.InboxRealtimeHint(request.Context(), userID, intent)
				if hintErr != nil {
					continue
				}
				if err := writeInboxSSEEvent(writer, "notification.changed", hint); err != nil {
					return
				}
				flusher.Flush()
			case <-heartbeat.C:
				if err := writeInboxSSEEvent(writer, "heartbeat", map[string]any{}); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}))(ctx, reqCtx)
}

func (h *Handler) MarkInboxSeen(ctx context.Context, reqCtx *app.RequestContext) {
	h.mutateInbox(ctx, reqCtx, domain.InboxActionSeen)
}

func (h *Handler) MarkInboxRead(ctx context.Context, reqCtx *app.RequestContext) {
	h.mutateInbox(ctx, reqCtx, domain.InboxActionRead)
}

func (h *Handler) MarkInboxUnread(ctx context.Context, reqCtx *app.RequestContext) {
	h.mutateInbox(ctx, reqCtx, domain.InboxActionUnread)
}

func (h *Handler) ArchiveInbox(ctx context.Context, reqCtx *app.RequestContext) {
	h.mutateInbox(ctx, reqCtx, domain.InboxActionArchive)
}

func (h *Handler) RestoreInbox(ctx context.Context, reqCtx *app.RequestContext) {
	h.mutateInbox(ctx, reqCtx, domain.InboxActionRestore)
}

func (h *Handler) mutateInbox(ctx context.Context, reqCtx *app.RequestContext, action string) {
	userID, err := requireCurrentUserID(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request facade.InboxMutationRequest
	if len(reqCtx.Request.Body()) > 0 {
		if err := reqCtx.BindAndValidate(&request); err != nil {
			response.Error(reqCtx, err)
			return
		}
	}
	result, err := h.service.MutateInboxRecipient(ctx, userID, strings.TrimSpace(string(reqCtx.Param("recipientId"))), action, request)
	write(reqCtx, result, err)
}

func write(reqCtx *app.RequestContext, data any, err error) {
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, data)
}

func writeInboxSSEEvent(writer http.ResponseWriter, event string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", event, raw)
	return err
}

func currentUserID(reqCtx *app.RequestContext) int64 {
	userID, _ := securitycontext.CurrentUserID(reqCtx)
	return userID
}

func requireCurrentUserID(reqCtx *app.RequestContext) (int64, error) {
	userID, ok := securitycontext.CurrentUserID(reqCtx)
	if !ok || userID <= 0 {
		return 0, apperrors.Unauthorized("未登录或登录信息失效")
	}
	return userID, nil
}

func queryInt(reqCtx *app.RequestContext, key string, fallback int) int {
	value, err := strconv.Atoi(reqCtx.Query(key))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func notificationRevisionID(reqCtx *app.RequestContext) (int64, error) {
	revisionID, err := strconv.ParseInt(strings.TrimSpace(string(reqCtx.Param("revisionId"))), 10, 64)
	if err != nil || revisionID <= 0 {
		return 0, apperrors.Params("版本标识不合法")
	}
	return revisionID, nil
}

func optionalInt(value string) *int {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return nil
	}
	return &parsed
}

func optionalBool(value string) *bool {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	normalized := strings.ToLower(strings.TrimSpace(value))
	parsed := normalized == "1" || normalized == "true"
	return &parsed
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
