package notification

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	notificationapp "github.com/CaixyPromise/seven-agent/force/internal/app/notification/application"
	notificationdomain "github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	notificationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	notificationhandler "github.com/CaixyPromise/seven-agent/force/internal/app/notification/handler"
	notificationinfra "github.com/CaixyPromise/seven-agent/force/internal/app/notification/infrastructure"
	notificationjob "github.com/CaixyPromise/seven-agent/force/internal/app/notification/job"
	notificationlistener "github.com/CaixyPromise/seven-agent/force/internal/app/notification/listener"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
	keyringinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/keyring"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/outboundurl"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
)

type Module struct {
	service *notificationapp.Service
	handler *notificationhandler.Handler
	oplog   adminfacade.OperationLogger
	scopeID string
	cancel  context.CancelFunc
}

func (m *Module) BindEnterpriseIdentityResolver(resolver notificationapp.EnterpriseIdentityReadinessResolver) {
	if m != nil && m.service != nil {
		m.service.BindEnterpriseIdentityResolver(resolver)
	}
}

func (m *Module) EnsureEnterpriseDirectRoute(ctx context.Context, organizationID, connectionID, connectionVersion int64, connectionDigest, provider string, enabled bool, actorID int64) error {
	if m == nil || m.service == nil {
		return fmt.Errorf("notification module is not configured")
	}
	return m.service.EnsureEnterpriseDirectRoute(ctx, organizationID, connectionID, connectionVersion, connectionDigest, provider, enabled, actorID)
}

// Dependencies are explicit cross-module API ports. The notification module
// never imports another module's application or infrastructure package.
type Dependencies struct {
	Audiences                notificationapp.AudienceResolver
	VerifiedEmails           notificationapp.VerifiedEmailResolver
	DisableBackgroundWorkers bool
	// OutboundGuard is an explicit composition-time override for an isolated
	// acceptance harness. Ordinary runtime wiring leaves it nil and receives
	// the default guard; channel rows never control this dependency.
	OutboundGuard *outboundurl.OutboundURLGuard
}

func Install(deps bootstrapruntime.ModuleDeps, options ...Dependencies) (*Module, notificationfacade.NotificationFacade, error) {
	if deps.Infra.Datasource == nil {
		return nil, nil, fmt.Errorf("notification module requires datasource provider")
	}
	if deps.Security.SecretValue == nil {
		return nil, nil, fmt.Errorf("notification module requires secret value service")
	}
	dependencies := Dependencies{}
	if len(options) > 0 {
		dependencies = options[0]
	}
	if dependencies.Audiences == nil {
		return nil, nil, fmt.Errorf("notification module requires notification audience facade")
	}
	scopeID, err := notificationScopeID(deps.Config)
	if err != nil {
		return nil, nil, err
	}
	dataDB := deps.Infra.Datasource.SQLX()
	repo := notificationinfra.NewRepository(dataDB)
	brokerClient := deps.Infra.RabbitMQ
	brokerDeclare := deps.Config.RabbitMQ.Declare
	if dependencies.DisableBackgroundWorkers {
		// A controlled acceptance run must not connect to, declare on, publish
		// to, or consume from the shared notification broker. Its exact selected
		// Outbox events use the bounded local dispatch path instead.
		brokerClient = nil
		brokerDeclare = false
	}
	broker, err := notificationinfra.NewScopedRabbitMQ(brokerClient, brokerDeclare, scopeID)
	if err != nil {
		return nil, nil, err
	}
	policyRegistry, err := notificationOutboundPolicyRegistry(deps.Config)
	if err != nil {
		return nil, nil, err
	}
	outboundGuard := dependencies.OutboundGuard
	if outboundGuard == nil {
		outboundGuard = outboundurl.NewOutboundURLGuard(outboundurl.Options{})
	}
	drivers := notificationinfra.NewDriverRegistryWithOutboundGuard(deps.Infra.CacheMgr, outboundGuard, policyRegistry)
	urlValidator := notificationinfra.NewChannelURLValidatorWithPolicyRegistry(outboundGuard, policyRegistry)
	service := notificationapp.NewService(
		deps.Infra.Transactor,
		repo,
		notificationdomain.NewService(),
		secretValueAdapter{service: deps.Security.SecretValue},
		drivers,
		urlValidator,
		broker,
		deps.IDGen,
	)
	service.SetLogger(deps.Logger)
	service.SetScopeID(scopeID)
	publicWebBaseURL, err := notificationdomain.ParsePublicWebBaseURL(deps.Config.Platform.PublicWebBaseURL, !isLocalNotificationDevelopment(deps.Config))
	if err != nil {
		return nil, nil, fmt.Errorf("notification public web base URL: %w", err)
	}
	service.BindPublicWebBaseURL(publicWebBaseURL)
	service.BindAudienceResolver(dependencies.Audiences)
	service.BindVerifiedEmailResolver(dependencies.VerifiedEmails)
	if deps.Security.MasterKeys != nil {
		service.BindExternalTargetDigester(externalTargetDigestAdapter{keys: deps.Security.MasterKeys})
	}
	// Workforce Gate 2 publishes through versioned scenes. Bootstrap the five
	// platform-owned in-app baselines before any publisher can accept work. The
	// bootstrap is idempotent and never replaces an administrator-published
	// revision with different content.
	if dataDB != nil {
		if err := service.EnsureBuiltinWorkforceScenes(context.Background()); err != nil {
			return nil, nil, fmt.Errorf("bootstrap workforce notification scenes: %w", err)
		}
	}
	realtime := notificationinfra.NewInboxRealtimeBus(nil, deps.Config.Cache.Redis.KeyPrefix, scopeID, deps.Logger)
	if deps.Infra.Cache != nil && deps.Infra.Cache.Configured() {
		realtime = notificationinfra.NewInboxRealtimeBus(deps.Infra.Cache.Client(), deps.Config.Cache.Redis.KeyPrefix, scopeID, deps.Logger)
	}
	service.BindInboxRealtime(realtime)
	if !dependencies.DisableBackgroundWorkers && deps.Infra.Jobs != nil {
		if err := deps.Infra.Jobs.Register(notificationjob.NewOutboxRelayJob(service, 5000, 50)); err != nil {
			return nil, nil, err
		}
		if err := deps.Infra.Jobs.Register(notificationjob.NewInboxExpiryJob(service, 5000, 50)); err != nil {
			return nil, nil, err
		}
	}
	consumerCtx, cancel := context.WithCancel(context.Background())
	startNotificationBackground(consumerCtx, dependencies.DisableBackgroundWorkers, broker, service, realtime)
	handler := notificationhandler.NewHandler(service)
	handler.BindDiagnosticTransportPolicy(notificationhandler.DiagnosticTransportPolicy{
		AllowLoopbackInsecure: isLocalNotificationDevelopment(deps.Config),
		TrustedProxies:        append([]string(nil), deps.Config.Authorization.Network.TrustedProxies...),
		TrustedCIDRs:          append([]string(nil), deps.Config.Authorization.Network.TrustedCIDRs...),
	})
	module := &Module{service: service, handler: handler, scopeID: scopeID, cancel: cancel}
	return module, service, nil
}

type notificationRealtimeStarter interface {
	Start(context.Context)
}

func startNotificationBackground(ctx context.Context, disabled bool, broker notificationlistener.Broker, handler notificationlistener.DispatchHandler, realtime notificationRealtimeStarter) {
	if disabled {
		return
	}
	notificationlistener.StartRabbitConsumers(ctx, broker, handler)
	if realtime != nil {
		realtime.Start(ctx)
	}
}

// externalTargetDigestAdapter computes a domain-separated keyed digest for a
// dynamic third-party subject. It is used only for uniqueness and semantic
// idempotency; the subject itself is stored only in the separate encrypted
// snapshot.
type externalTargetDigestAdapter struct {
	keys keyringinfra.MasterKeyProvider
}

func (d externalTargetDigestAdapter) Digest(ctx context.Context, keyRef, scopeID, connectionRef, identityKind, subject string) (string, string, error) {
	if d.keys == nil {
		return "", "", fmt.Errorf("notification target digest key provider is not configured")
	}
	var (
		key *keyringinfra.MasterKey
		err error
	)
	if strings.TrimSpace(keyRef) == "" {
		key, err = d.keys.Current(ctx)
	} else {
		key, err = d.keys.ByKID(ctx, keyRef)
	}
	if err != nil {
		return "", "", err
	}
	if key == nil || strings.TrimSpace(key.KID) == "" || len(key.Material) == 0 {
		return "", "", fmt.Errorf("notification target digest key is invalid")
	}
	mac := hmac.New(sha256.New, key.Material)
	_, _ = mac.Write([]byte("seven-notification-external-target-v1\x00"))
	_, _ = mac.Write([]byte(strings.TrimSpace(scopeID)))
	_, _ = mac.Write([]byte("\x00"))
	_, _ = mac.Write([]byte(strings.TrimSpace(connectionRef)))
	_, _ = mac.Write([]byte("\x00"))
	_, _ = mac.Write([]byte(strings.TrimSpace(identityKind)))
	_, _ = mac.Write([]byte("\x00"))
	_, _ = mac.Write([]byte(strings.TrimSpace(subject)))
	return hex.EncodeToString(mac.Sum(nil)), key.KID, nil
}

func notificationScopeID(cfg config.Config) (string, error) {
	switch cfg.Platform.Mode {
	case "", config.PlatformModeLocal:
		return "local", nil
	case config.PlatformModeHub:
		return "hub", nil
	case config.PlatformModeNode:
		nodeCode := strings.TrimSpace(cfg.Platform.Node.Code)
		if nodeCode == "" {
			return "", fmt.Errorf("notification node scope requires configured node code")
		}
		return "node:" + nodeCode, nil
	default:
		return "", fmt.Errorf("notification scope does not support platform mode %q", cfg.Platform.Mode)
	}
}

// notificationOutboundPolicyRegistry translates deployment configuration into
// the guard's immutable environment policy registry. A fake-IP proxy entry is
// permitted only for an explicit local development/test runtime; a channel
// cannot enable it by persisting a proxy URL or policy payload.
func notificationOutboundPolicyRegistry(cfg config.Config) (*outboundurl.EnvironmentPolicyRegistry, error) {
	return OutboundPolicyRegistry(cfg)
}

// OutboundPolicyRegistry builds the immutable, environment-owned URL policy
// registry used by notification URL channels. It is exported for controlled
// local acceptance commands so those commands validate a fixture through the
// same fake-IP and private-network rules as the runtime module.
func OutboundPolicyRegistry(cfg config.Config) (*outboundurl.EnvironmentPolicyRegistry, error) {
	entries := make([]outboundurl.EnvironmentPolicy, 0, len(cfg.Notification.Outbound.Policies))
	for _, policy := range cfg.Notification.Outbound.Policies {
		entries = append(entries, outboundurl.EnvironmentPolicy{
			Name:             policy.Name,
			Mode:             outboundurl.Mode(policy.Mode),
			AllowedHostnames: append([]string(nil), policy.AllowedHostnames...),
			AllowedCIDRs:     append([]string(nil), policy.AllowedCIDRs...),
			AllowedPorts:     append([]int(nil), policy.AllowedPorts...),
			ProxyURL:         policy.ProxyURL,
		})
	}
	return outboundurl.NewEnvironmentPolicyRegistry(entries, outboundurl.EnvironmentPolicyRegistryOptions{
		AllowFakeIPProxy: isLocalNotificationDevelopment(cfg),
	})
}

func isLocalNotificationDevelopment(cfg config.Config) bool {
	env := strings.ToLower(strings.TrimSpace(cfg.Seven.Env))
	if env != "dev" && env != "development" && env != "test" {
		return false
	}
	return cfg.Platform.Mode == "" || cfg.Platform.Mode == config.PlatformModeLocal
}

func (m *Module) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: "notification", Prefix: "/notification"}
}

func (m *Module) Mount(engine route.IRouter) {
	if engine == nil || m == nil || m.handler == nil {
		return
	}
	engine.GET("/notification/channels", m.wrapPermission("system:notification:channel:list", m.handler.ListChannels))
	engine.POST("/notification/channels", m.wrapPermission("system:notification:channel:edit", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeConfigUpdate, Description: "保存通知渠道", IncludeParams: true}, m.handler.UpsertChannel)))
	// Template operation records omit
	// request params so template bodies and preview variable values never enter
	// the generic operation log.
	engine.GET("/notification/template-definitions", m.wrapPermission("system:notification:template:list", m.handler.ListTemplateDefinitions))
	engine.POST("/notification/template-definitions", m.wrapPermission("system:notification:template:edit", m.wrapOperation(m.templateOperationLogSpec(adminfacade.OperationTypeConfigUpdate, "创建版本化通知模板草稿", "CREATE_DRAFT", "/notification/template-definitions"), m.handler.CreateTemplateDefinition)))
	engine.POST("/notification/template-definitions/:templateCode/drafts", m.wrapPermission("system:notification:template:edit", m.wrapOperation(m.templateOperationLogSpec(adminfacade.OperationTypeConfigUpdate, "新建通知模板版本草稿", "CREATE_DRAFT_FROM_PUBLISHED", "/notification/template-definitions/:templateCode/drafts"), m.handler.CreateTemplateDraftFromPublished)))
	engine.GET("/notification/template-definitions/:templateCode", m.wrapPermission("system:notification:template:list", m.handler.GetTemplateDefinition))
	engine.POST("/notification/template-revisions/preview", m.wrapPermission("system:notification:template:edit", m.wrapOperation(m.templateOperationLogSpec(adminfacade.OperationTypeOther, "预览版本化通知模板", "PREVIEW", "/notification/template-revisions/preview"), m.handler.PreviewTemplateRevision)))
	engine.POST("/notification/template-revisions/:revisionId/publish", m.wrapPermission("system:notification:template:edit", m.wrapOperation(m.templateOperationLogSpec(adminfacade.OperationTypeConfigUpdate, "发布通知模板版本", "PUBLISH", "/notification/template-revisions/:revisionId/publish"), m.handler.PublishTemplateRevision)))
	engine.POST("/notification/template-revisions/:revisionId", m.wrapPermission("system:notification:template:edit", m.wrapOperation(m.templateOperationLogSpec(adminfacade.OperationTypeConfigUpdate, "保存通知模板草稿", "SAVE_DRAFT", "/notification/template-revisions/:revisionId"), m.handler.SaveTemplateRevisionDraft)))
	// Scene routes deliberately expose only a template revision plus one
	// sending way.
	engine.GET("/notification/scene-definitions", m.wrapPermission("system:notification:scene:list", m.handler.ListSceneDefinitions))
	engine.POST("/notification/scene-definitions", m.wrapPermission("system:notification:scene:edit", m.wrapOperation(m.templateOperationLogSpec(adminfacade.OperationTypeConfigUpdate, "创建新版通知场景草稿", "CREATE_DRAFT", "/notification/scene-definitions"), m.handler.CreateSceneDefinition)))
	engine.POST("/notification/scene-definitions/:sceneCode/drafts", m.wrapPermission("system:notification:scene:edit", m.wrapOperation(m.templateOperationLogSpec(adminfacade.OperationTypeConfigUpdate, "新建通知场景版本草稿", "CREATE_DRAFT_FROM_PUBLISHED", "/notification/scene-definitions/:sceneCode/drafts"), m.handler.CreateSceneDraftFromPublished)))
	engine.POST("/notification/scene-definitions/:sceneCode/stop", m.wrapPermission("system:notification:scene:edit", m.wrapOperation(m.templateOperationLogSpec(adminfacade.OperationTypeConfigUpdate, "停用通知场景", "STOP", "/notification/scene-definitions/:sceneCode/stop"), m.handler.StopSceneDefinition)))
	engine.GET("/notification/scene-definitions/:sceneCode", m.wrapPermission("system:notification:scene:list", m.handler.GetSceneDefinition))
	engine.POST("/notification/scene-revisions/:revisionId/publish", m.wrapPermission("system:notification:scene:edit", m.wrapOperation(m.templateOperationLogSpec(adminfacade.OperationTypeConfigUpdate, "发布通知场景版本", "PUBLISH", "/notification/scene-revisions/:revisionId/publish"), m.handler.PublishSceneRevision)))
	engine.POST("/notification/scene-revisions/:revisionId", m.wrapPermission("system:notification:scene:edit", m.wrapOperation(m.templateOperationLogSpec(adminfacade.OperationTypeConfigUpdate, "保存通知场景草稿", "SAVE_DRAFT", "/notification/scene-revisions/:revisionId"), m.handler.SaveSceneRevisionDraft)))
	engine.GET("/notification/deliveries", m.wrapPermission("system:notification:delivery:list", m.handler.ListDeliveries))
	// The handler performs the general and tier-specific capability checks so a
	// denied attempt can be recorded in the dedicated content-free audit trail.
	// Do not use the generic permission wrapper here: it would short-circuit
	// before the handler can safely record the reason and denial outcome.
	engine.POST("/notification/deliveries/:deliveryId/diagnostic-content", m.wrapLogin(m.wrapOperation(m.deliveryDiagnosticOperationLogSpec(), m.handler.ReadDeliveryDiagnosticContent)))
	engine.POST("/notification/channels/test-connection", m.wrapPermission("system:notification:test", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "测试企业应用连接", IncludeParams: false}, m.handler.TestEnterpriseConnection)))
	engine.POST("/notification/channels/test-static-connection", m.wrapPermission("system:notification:test", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "测试受控 HTTP 连接", IncludeParams: false}, m.handler.TestStaticConnection)))
	engine.GET("/notification/group-routes", m.wrapPermission("system:notification-group-route:list", m.handler.ListGroupRoutes))
	engine.POST("/notification/group-routes", m.wrapPermission("system:notification-group-route:create", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeConfigUpdate, Description: "创建组织通知群", IncludeParams: false}, m.handler.SaveGroupRoute)))
	engine.GET("/notification/group-routes/available", m.wrapLogin(m.handler.AvailableGroupRoutes))
	engine.GET("/notification/group-routes/feishu-group-candidates", m.wrapPermission("system:notification-group-route:create", m.handler.FeishuGroupCandidates))
	engine.GET("/notification/group-routes/:routeId", m.wrapPermission("system:notification-group-route:query", m.handler.GetGroupRoute))
	engine.PUT("/notification/group-routes/:routeId", m.wrapPermission("system:notification-group-route:update", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeConfigUpdate, Description: "更新组织通知群", IncludeParams: false}, m.handler.SaveGroupRoute)))
	engine.POST("/notification/group-routes/:routeId/rotate", m.wrapPermission("system:notification-group-route:update", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeConfigUpdate, Description: "轮换组织通知群目标", IncludeParams: false}, m.handler.SaveGroupRoute)))
	engine.POST("/notification/group-routes/:routeId/probe", m.wrapPermission("system:notification-group-route:probe", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "测试组织通知群", IncludeParams: false}, m.handler.ProbeGroupRoute)))
	engine.POST("/notification/group-routes/:routeId/enable", m.wrapPermission("system:notification-group-route:enable", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeConfigUpdate, Description: "启用组织通知群", IncludeParams: false}, m.handler.SetGroupRouteState("ENABLE"))))
	engine.POST("/notification/group-routes/:routeId/disable", m.wrapPermission("system:notification-group-route:disable", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeConfigUpdate, Description: "停用组织通知群", IncludeParams: false}, m.handler.SetGroupRouteState("DISABLE"))))
	engine.POST("/notification/group-routes/:routeId/retire", m.wrapPermission("system:notification-group-route:retire", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeConfigUpdate, Description: "退役组织通知群", IncludeParams: false}, m.handler.SetGroupRouteState("RETIRE"))))
	engine.GET("/notification/group-routes/:routeId/impact", m.wrapPermission("system:notification-group-route:query", m.handler.GroupRouteImpact))
	engine.DELETE("/notification/group-routes/:routeId", m.wrapPermission("system:notification-group-route:delete", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeConfigUpdate, Description: "删除未使用的组织通知群", IncludeParams: false}, m.handler.DeleteGroupRoute)))

	// User inbox routes have a login-only boundary. They must never inherit
	// management permissions or accept a caller-supplied mailbox owner ID.
	engine.GET("/notification/readiness", m.wrapLogin(m.handler.Readiness))
	engine.GET("/notification/me/work-preferences", m.wrapLogin(m.handler.MyWorkPreferences))
	engine.PUT("/notification/me/work-preferences/:eventFamily/:channelKind", m.wrapLogin(m.handler.UpdateMyWorkPreference))
	engine.GET("/notification/inbox", m.wrapLogin(m.handler.ListInbox))
	engine.GET("/notification/inbox/unread-count", m.wrapLogin(m.handler.UnreadCount))
	engine.GET("/notification/inbox/unread-preview", m.wrapLogin(m.handler.UnreadPreview))
	engine.GET("/notification/inbox/changes", m.wrapLogin(m.handler.ListInboxChanges))
	engine.GET("/notification/inbox/stream", m.wrapLogin(m.handler.StreamInbox))
	engine.GET("/notification/inbox/:recipientId", m.wrapLogin(m.handler.GetInboxRecipient))
	engine.POST("/notification/inbox/:recipientId/seen", m.wrapLogin(m.handler.MarkInboxSeen))
	engine.POST("/notification/inbox/:recipientId/read", m.wrapLogin(m.handler.MarkInboxRead))
	engine.POST("/notification/inbox/:recipientId/unread", m.wrapLogin(m.handler.MarkInboxUnread))
	engine.POST("/notification/inbox/:recipientId/archive", m.wrapLogin(m.handler.ArchiveInbox))
	engine.POST("/notification/inbox/:recipientId/restore", m.wrapLogin(m.handler.RestoreInbox))
}

func (m *Module) Shutdown(ctx context.Context) error {
	if m != nil && m.cancel != nil {
		m.cancel()
	}
	if ctx != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	return nil
}

// RelayOutboxForAcceptance relays only the two durable events owned by one
// operator-approved external delivery. It does not expose an HTTP route, scan
// the broader notification queue, or materialize an unrelated inbox audience.
func (m *Module) RelayOutboxForAcceptance(ctx context.Context, notificationID, deliveryID string) error {
	if m == nil || m.service == nil {
		return fmt.Errorf("notification module is not configured")
	}
	notificationID = strings.TrimSpace(notificationID)
	deliveryID = strings.TrimSpace(deliveryID)
	if notificationID == "" || deliveryID == "" {
		return fmt.Errorf("acceptance notification and delivery ids are required")
	}
	return m.service.RelaySelectedOutbox(ctx, []notificationdomain.OutboxEventSelection{
		{EventID: "notification-intent:" + notificationID, EventType: notificationdomain.OutboxEventNotificationIntent},
		{EventID: "notification:" + deliveryID, EventType: notificationdomain.OutboxEventNotificationDispatch},
	})
}

func (m *Module) BindOperationLogger(oplog adminfacade.OperationLogger) {
	if m != nil {
		m.oplog = oplog
	}
}

// BindAuthorization connects the diagnostic-content step-up adapter only
// after the authorization module is installed. This preserves the module
// construction order while keeping the dependency on the public facade.
func (m *Module) BindAuthorization(auth authorizationfacade.AuthFacade) {
	if m != nil && m.handler != nil {
		m.handler.BindAuthorization(auth)
	}
}

func (m *Module) BindPermissions(permissions authorizationfacade.PermissionFacade) {
	if m != nil && m.service != nil {
		m.service.BindPermissionResolver(permissions)
	}
}

func (m *Module) wrapPermission(permission string, handler app.HandlerFunc) app.HandlerFunc {
	return func(ctx context.Context, reqCtx *app.RequestContext) {
		if !securitycontext.IsLogin(reqCtx) {
			response.Error(reqCtx, apperrors.Unauthorized("未登录"))
			return
		}
		if !securitycontext.HasPermission(reqCtx, permission) {
			response.Error(reqCtx, apperrors.PermissionDenied(permission))
			return
		}
		handler(ctx, reqCtx)
	}
}

func (m *Module) wrapLogin(handler app.HandlerFunc) app.HandlerFunc {
	return func(ctx context.Context, reqCtx *app.RequestContext) {
		if !securitycontext.IsLogin(reqCtx) {
			response.Error(reqCtx, apperrors.Unauthorized("未登录"))
			return
		}
		handler(ctx, reqCtx)
	}
}

func (m *Module) wrapOperation(spec adminfacade.OperationLogSpec, handler app.HandlerFunc) app.HandlerFunc {
	if m == nil || m.oplog == nil {
		return handler
	}
	return m.oplog.Wrap(spec, handler)
}

// templateOperationLogSpec records only safe, low-cardinality template or
// scene version metadata. It deliberately replaces generic request/response
// capture because those bodies may contain template content or one-time
// preview values.
func (m *Module) templateOperationLogSpec(operation adminfacade.OperationTypeEnum, description, action, route string) adminfacade.OperationLogSpec {
	scopeID := "local"
	if m != nil && strings.TrimSpace(m.scopeID) != "" {
		scopeID = strings.TrimSpace(m.scopeID)
	}
	return adminfacade.OperationLogSpec{
		Operation:     operation,
		Description:   description,
		IncludeParams: false,
		IncludeResult: false,
		OmitQuery:     true,
		CompletionEnrichers: []adminfacade.OperationLogEnricher{
			templateOperationAuditCompletionEnricher{scopeID: scopeID, action: action, route: route},
		},
	}
}

// deliveryDiagnosticOperationLogSpec removes generic request/result capture
// for the sole endpoint that may return message content. The dedicated domain
// audit stores accountable metadata; this operation log stores only an action,
// scope and response code, never a delivery ID, reason, ticket or content.
func (m *Module) deliveryDiagnosticOperationLogSpec() adminfacade.OperationLogSpec {
	scopeID := "local"
	if m != nil && strings.TrimSpace(m.scopeID) != "" {
		scopeID = strings.TrimSpace(m.scopeID)
	}
	return adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查看通知投递诊断内容",
		IncludeParams: false,
		IncludeResult: false,
		OmitQuery:     true,
		CompletionEnrichers: []adminfacade.OperationLogEnricher{
			deliveryDiagnosticOperationAuditCompletionEnricher{scopeID: scopeID},
		},
	}
}

type deliveryDiagnosticOperationAuditCompletionEnricher struct {
	scopeID string
}

func (e deliveryDiagnosticOperationAuditCompletionEnricher) Enrich(_ context.Context, reqCtx *app.RequestContext, entry *adminfacade.OperationLogEntry) {
	if reqCtx == nil || entry == nil {
		return
	}
	metadata := map[string]any{
		"action":  "READ_CONTENT",
		"scopeId": strings.TrimSpace(e.scopeID),
	}
	code, _ := xcontext.ResponseError(reqCtx)
	if code != 0 {
		metadata["errorCode"] = code
		entry.ErrorMsg = "delivery diagnostic failed"
	}
	entry.MethodName = "/notification/deliveries/:deliveryId/diagnostic-content"
	entry.RequestURL = "/notification/deliveries/:deliveryId/diagnostic-content"
	if encoded, err := json.Marshal(metadata); err == nil {
		entry.RequestParams = string(encoded)
	}
	if encoded, err := json.Marshal(map[string]int{"code": code}); err == nil {
		entry.ResponseResult = string(encoded)
	}
}

type templateOperationAuditCompletionEnricher struct {
	scopeID string
	action  string
	route   string
}

// Enrich replaces generic operation-body capture with a fixed metadata shape.
// It never reads request bodies, and it never persists a response's rendered
// content, variable schema, preview values, or error message.
func (e templateOperationAuditCompletionEnricher) Enrich(_ context.Context, reqCtx *app.RequestContext, entry *adminfacade.OperationLogEntry) {
	if reqCtx == nil || entry == nil {
		return
	}
	metadata := map[string]any{
		"action":  strings.TrimSpace(e.action),
		"scopeId": strings.TrimSpace(e.scopeID),
	}
	entry.MethodName = e.route
	entry.RequestURL = e.route

	code, _ := xcontext.ResponseError(reqCtx)
	if body := reqCtx.Response.Body(); len(body) > 0 && len(body) <= 64<<10 {
		var envelope struct {
			Code int             `json:"code"`
			Data json.RawMessage `json:"data"`
		}
		if json.Unmarshal(body, &envelope) == nil {
			if code == 0 {
				code = envelope.Code
			}
			if code == 0 {
				var data struct {
					TemplateCode string `json:"templateCode"`
					SceneCode    string `json:"sceneCode"`
					RevisionNo   int    `json:"revisionNo"`
					CurrentDraft *struct {
						RevisionNo int `json:"revisionNo"`
					} `json:"currentDraft"`
					CurrentPublished *struct {
						RevisionNo int `json:"revisionNo"`
					} `json:"currentPublished"`
				}
				if len(envelope.Data) > 0 && json.Unmarshal(envelope.Data, &data) == nil {
					if templateCode := safeTemplateOperationCode(data.TemplateCode); templateCode != "" {
						metadata["templateCode"] = templateCode
					}
					if sceneCode := safeSceneOperationCode(data.SceneCode); sceneCode != "" {
						metadata["sceneCode"] = sceneCode
					}
					if revisionNo := firstTemplateOperationRevisionNo(data.RevisionNo, data.CurrentDraft, data.CurrentPublished); revisionNo > 0 {
						metadata["revisionNo"] = revisionNo
					}
				}
			}
		}
	}
	if code != 0 {
		metadata["errorCode"] = code
		entry.ErrorMsg = "template operation failed"
	}
	if encoded, err := json.Marshal(metadata); err == nil {
		entry.RequestParams = string(encoded)
	}
	if encoded, err := json.Marshal(map[string]int{"code": code}); err == nil {
		entry.ResponseResult = string(encoded)
	}
}

func safeTemplateOperationCode(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || notificationdomain.ValidateTemplateDefinitionCode(value) != nil {
		return ""
	}
	return value
}

func safeSceneOperationCode(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || notificationdomain.ValidateSceneDefinitionCode(value) != nil {
		return ""
	}
	return value
}

func firstTemplateOperationRevisionNo(direct int, draft, published *struct {
	RevisionNo int `json:"revisionNo"`
}) int {
	for _, candidate := range []int{direct, revisionNoFromTemplateOperationRecord(draft), revisionNoFromTemplateOperationRecord(published)} {
		if candidate > 0 && candidate <= 1_000_000_000 {
			return candidate
		}
	}
	return 0
}

func revisionNoFromTemplateOperationRecord(record *struct {
	RevisionNo int `json:"revisionNo"`
}) int {
	if record == nil {
		return 0
	}
	return record.RevisionNo
}

type secretValueAdapter struct {
	service secretvalueinfra.Service
}

func (a secretValueAdapter) EncryptString(ctx context.Context, plain string) (secretvalueinfra.SecretValue, error) {
	return a.service.EncryptString(ctx, plain)
}

func (a secretValueAdapter) DecryptString(ctx context.Context, value secretvalueinfra.SecretValue) (string, error) {
	return a.service.DecryptString(ctx, value)
}
