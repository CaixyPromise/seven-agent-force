package external_login

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	externalapp "github.com/CaixyPromise/seven-agent/force/internal/app/external_login/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/facade"
	externalloginhandler "github.com/CaixyPromise/seven-agent/force/internal/app/external_login/handler"
	externallogininfra "github.com/CaixyPromise/seven-agent/force/internal/app/external_login/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/infrastructure/drivers"
	platformfacade "github.com/CaixyPromise/seven-agent/force/internal/app/platform/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	microserviceinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/microservice"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
	keyringinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/keyring"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
)

// enterpriseLoginPublished keeps the enterprise LOGIN grant and OAuth engine
// dormant until the product flow is ready to be exposed. Enterprise identity
// binding and notification delivery remain available.
const enterpriseLoginPublished = false

type Dependencies struct {
	Subjects                 userfacade.SubjectFacade
	Profiles                 userfacade.ProfileFacade
	AuthorizationSessions    ssofacade.AuthorizationSessionFacade
	AuthenticationCompletion ssofacade.AuthenticationCompletionFacade
	BootstrapSession         ssofacade.BootstrapSessionFacade
	Sessions                 ssofacade.SessionFacade
	Platform                 platformfacade.PublicFacade
	Relations                userfacade.UserRelationFacade
	Organizations            userfacade.OrgFacade
}

type Facades struct {
	LoginMethods facade.LoginMethodFacade
	LoginFlow    facade.ExternalLoginFlowFacade
	Providers    facade.ProviderAdminFacade
	Identities   facade.IdentityBindingFacade
	Tokens       facade.ExternalOAuthTokenFacade
	Capabilities facade.CapabilityIndexFacade
	ManagedOIDC  facade.ManagedOIDCProviderFacade
	Enterprise   facade.EnterpriseIdentityFacade
}

type Module struct {
	service    *externalapp.Service
	enterprise *externalapp.EnterpriseService
	handler    *externalloginhandler.Handler
	oplog      adminfacade.OperationLogger
	facades    Facades
	outbound   *microserviceinfra.HTTPServiceClient
}

func (m *Module) BindEnterpriseNotificationRoutes(routes externalapp.EnterpriseNotificationRouteProvisioner) {
	if m != nil && m.enterprise != nil {
		m.enterprise.BindNotificationRoutes(routes)
	}
}

func Install(deps bootstrapruntime.ModuleDeps, refs Dependencies) (*Module, Facades, error) {
	if !deps.Config.ExternalLogin.Enabled {
		return nil, Facades{}, nil
	}
	if deps.Infra.Datasource == nil {
		return nil, Facades{}, fmt.Errorf("external login module requires datasource provider")
	}
	if deps.Infra.CacheMgr == nil {
		return nil, Facades{}, fmt.Errorf("external login module requires cache manager")
	}
	if deps.Security.SecretValue == nil {
		return nil, Facades{}, fmt.Errorf("external login module requires secret value service")
	}
	if deps.Security.Random == nil {
		return nil, Facades{}, fmt.Errorf("external login module requires random service")
	}
	if refs.Subjects == nil || refs.Profiles == nil || refs.AuthenticationCompletion == nil || refs.BootstrapSession == nil || refs.Sessions == nil {
		return nil, Facades{}, fmt.Errorf("external login module requires user and sso facades")
	}
	repository, err := externallogininfra.NewRepository(deps.Infra.Datasource)
	if err != nil {
		return nil, Facades{}, err
	}
	timeout := deps.Config.ExternalLogin.HTTPTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	trust, err := microserviceinfra.NewOutboundTrustPolicy(microserviceinfra.OutboundTrustConfig{
		TrustedHosts: deps.Config.Microservice.Outbound.TrustedHosts, TrustedCIDRs: deps.Config.Microservice.Outbound.TrustedCIDRs,
		RegistryTrustedHosts: deps.Config.Microservice.Outbound.RegistryTrustedHosts, RegistryTrustedCIDRs: deps.Config.Microservice.Outbound.RegistryTrustedCIDRs,
	}, nil)
	if err != nil {
		return nil, Facades{}, fmt.Errorf("build external login outbound trust policy: %w", err)
	}
	outbound := microserviceinfra.NewHTTPServiceClient(nil, microserviceinfra.NewRoundRobin(), microserviceinfra.HTTPClientOptions{
		RequestTimeout: timeout, MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20, OutboundPolicy: trust,
	})
	httpClient := &http.Client{
		Transport: oidcOutboundRoundTripper{client: outbound}, Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	oidcDriver := drivers.NewOIDCDriver(
		drivers.WithOIDCHTTPClient(httpClient),
		drivers.WithOIDCDevelopmentHTTP(!productionProfile(deps.Config.Profile, deps.Config.Seven.Env)),
	)
	registry := drivers.NewRegistry(
		drivers.NewGitHubDriver(drivers.WithGitHubHTTPClient(httpClient)),
		drivers.NewGoogleDriver(drivers.WithOIDCHTTPClient(httpClient)),
		oidcDriver,
	)
	service := externalapp.NewService(externalapp.ServiceDeps{
		Config:                 deps.Config.ExternalLogin,
		Transactor:             deps.Infra.Transactor,
		IDGen:                  deps.IDGen,
		Repository:             repositoryAdapter{Repository: repository},
		StateCache:             externallogininfra.NewStateCache(deps.Infra.CacheMgr),
		Drivers:                driverRegistryAdapter{registry: registry},
		Discovery:              oidcDiscoveryAdapter{driver: oidcDriver},
		SecretValue:            secretValueAdapter{service: deps.Security.SecretValue},
		IdentityDigester:       systemIdentitySubjectDigestAdapter{keys: deps.Security.MasterKeys},
		Random:                 deps.Security.Random,
		Subjects:               refs.Subjects,
		Profiles:               refs.Profiles,
		AuthorizationSessions:  refs.AuthorizationSessions,
		AuthenticationComplete: refs.AuthenticationCompletion,
		BootstrapSession:       refs.BootstrapSession,
		Sessions:               refs.Sessions,
		Platform:               refs.Platform,
	})
	if deps.Security.MasterKeys == nil {
		return nil, Facades{}, fmt.Errorf("external login enterprise identity requires master key provider")
	}
	fakeBaseURL := strings.TrimSpace(deps.Config.ExternalLogin.EnterpriseFakeBaseURL)
	if fakeBaseURL != "" && productionProfile(deps.Config.Profile, deps.Config.Seven.Env) {
		return nil, Facades{}, fmt.Errorf("enterprise fake OAuth endpoint is forbidden in production")
	}
	if fakeBaseURL != "" {
		parsed, parseErr := url.Parse(fakeBaseURL)
		if parseErr != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !isLoopbackHostname(parsed.Hostname()) {
			return nil, Facades{}, fmt.Errorf("enterprise fake OAuth endpoint must be a loopback HTTP origin")
		}
	}
	enterpriseService := externalapp.NewEnterpriseService(externalapp.EnterpriseServiceDeps{
		Config:                 moduleFirstNonBlank(deps.Config.Platform.PublicWebBaseURL, deps.Config.ExternalLogin.CallbackBaseURL),
		Transactor:             deps.Infra.Transactor,
		IDGen:                  deps.IDGen,
		Repository:             repository,
		SecretValue:            secretValueAdapter{service: deps.Security.SecretValue},
		Random:                 deps.Security.Random,
		Digester:               enterpriseSubjectDigestAdapter{keys: deps.Security.MasterKeys},
		Providers:              newEnterpriseProviderRegistry(httpClient, fakeBaseURL),
		Sessions:               refs.Sessions,
		Subjects:               refs.Subjects,
		Relations:              refs.Relations,
		Orgs:                   refs.Organizations,
		Bootstrap:              refs.BootstrapSession,
		EnterpriseLoginEnabled: enterpriseLoginPublished,
	})
	handler := externalloginhandler.NewHandler(service)
	handler.BindEnterpriseIdentity(enterpriseService)
	module := &Module{
		service:    service,
		enterprise: enterpriseService,
		handler:    handler,
		outbound:   outbound,
	}
	module.facades = Facades{
		LoginMethods: service,
		LoginFlow:    service,
		Providers:    service,
		Identities:   service,
		Tokens:       service,
		Capabilities: service,
		ManagedOIDC:  service,
		Enterprise:   enterpriseService,
	}
	return module, module.facades, nil
}

func isLoopbackHostname(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func moduleFirstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

type enterpriseSubjectDigestAdapter struct {
	keys keyringinfra.MasterKeyProvider
}

type systemIdentitySubjectDigestAdapter struct {
	keys keyringinfra.MasterKeyProvider
}

func (d systemIdentitySubjectDigestAdapter) DigestSystemSubject(ctx context.Context, keyRef, providerCode, issuer, subject string) (string, string, error) {
	var key *keyringinfra.MasterKey
	var err error
	if strings.TrimSpace(keyRef) == "" {
		key, err = d.keys.Current(ctx)
	} else {
		key, err = d.keys.ByKID(ctx, keyRef)
	}
	if err != nil {
		return "", "", err
	}
	if key == nil || key.KID == "" || len(key.Material) == 0 {
		return "", "", fmt.Errorf("system identity subject digest key is invalid")
	}
	mac := hmac.New(sha256.New, key.Material)
	_, _ = mac.Write([]byte("seven-system-external-subject-v1\x00"))
	_, _ = mac.Write([]byte(strings.ToLower(strings.TrimSpace(providerCode))))
	_, _ = mac.Write([]byte("\x00"))
	_, _ = mac.Write([]byte(strings.TrimSpace(issuer)))
	_, _ = mac.Write([]byte("\x00"))
	_, _ = mac.Write([]byte(strings.TrimSpace(subject)))
	return hex.EncodeToString(mac.Sum(nil)), key.KID, nil
}

func (d enterpriseSubjectDigestAdapter) Digest(ctx context.Context, keyRef string, organizationID, connectionID int64, provider domain.EnterpriseProvider, subjectKind, subject string) (string, string, error) {
	var key *keyringinfra.MasterKey
	var err error
	if strings.TrimSpace(keyRef) == "" {
		key, err = d.keys.Current(ctx)
	} else {
		key, err = d.keys.ByKID(ctx, keyRef)
	}
	if err != nil {
		return "", "", err
	}
	if key == nil || key.KID == "" || len(key.Material) == 0 {
		return "", "", fmt.Errorf("enterprise subject digest key is invalid")
	}
	mac := hmac.New(sha256.New, key.Material)
	_, _ = mac.Write([]byte("seven-enterprise-external-subject-v1\x00"))
	_, _ = mac.Write([]byte(strconv.FormatInt(organizationID, 10)))
	_, _ = mac.Write([]byte("\x00"))
	_, _ = mac.Write([]byte(strconv.FormatInt(connectionID, 10)))
	_, _ = mac.Write([]byte("\x00"))
	_, _ = mac.Write([]byte(provider))
	_, _ = mac.Write([]byte("\x00"))
	_, _ = mac.Write([]byte(subjectKind))
	_, _ = mac.Write([]byte("\x00"))
	_, _ = mac.Write([]byte(subject))
	return hex.EncodeToString(mac.Sum(nil)), key.KID, nil
}

func (m *Module) Shutdown(context.Context) error {
	if m != nil && m.outbound != nil {
		m.outbound.CloseIdleConnections()
	}
	return nil
}

func productionProfile(profile, environment string) bool {
	for _, value := range []string{profile, environment} {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "prod", "production":
			return true
		}
	}
	return false
}

type oidcOutboundRoundTripper struct {
	client *microserviceinfra.HTTPServiceClient
}

func (t oidcOutboundRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if t.client == nil || request == nil || request.URL == nil || request.URL.Host == "" || request.URL.User != nil || request.URL.Fragment != "" || (request.URL.Scheme != "https" && request.URL.Scheme != "http") {
		return nil, fmt.Errorf("invalid external OIDC outbound request")
	}
	port := 443
	if request.URL.Scheme == "http" {
		port = 80
	}
	if rawPort := request.URL.Port(); rawPort != "" {
		parsed, err := strconv.Atoi(rawPort)
		if err != nil || parsed < 1 || parsed > 65535 {
			return nil, fmt.Errorf("invalid external OIDC outbound port")
		}
		port = parsed
	}
	var body []byte
	if request.Body != nil {
		var err error
		body, err = io.ReadAll(io.LimitReader(request.Body, (1<<20)+1))
		if err != nil {
			return nil, err
		}
		if len(body) > 1<<20 {
			return nil, fmt.Errorf("external OIDC request exceeds limit")
		}
	}
	path := request.URL.EscapedPath()
	if path == "" {
		path = "/"
	}
	if request.URL.RawQuery != "" {
		path += "?" + request.URL.RawQuery
	}
	response, err := t.client.Do(request.Context(), microserviceinfra.ServiceRequest{
		ServiceName: "external-oidc", Method: request.Method, Path: path, Header: request.Header.Clone(), Body: body,
		ResolvedInstances: []microserviceinfra.ServiceInstance{{
			ID: "external-oidc@" + request.URL.Host, ServiceName: "external-oidc", Host: request.URL.Hostname(), Port: port, Scheme: request.URL.Scheme, Healthy: true,
		}},
		MaxResponseBytes: 1 << 20,
	})
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: response.StatusCode, Status: strconv.Itoa(response.StatusCode) + " " + http.StatusText(response.StatusCode),
		Header: response.Header.Clone(), Body: io.NopCloser(bytes.NewReader(response.Body)), Request: request,
	}, nil
}

func (m *Module) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: "external-login", Prefix: "/external-login"}
}

func (m *Module) Mount(engine route.IRouter) {
	m.MountPublic(engine)
}

func (m *Module) MountPublic(engine route.IRouter) {
	if engine == nil || m == nil || m.handler == nil {
		return
	}
	engine.GET("/login/external/providers", m.handler.ListLoginMethods)
	engine.GET("/login/external/:providerCode/start", m.handler.StartExternalLogin)
	engine.GET("/login/external/:providerCode/callback", m.handler.CompleteExternalCallback)
	if enterpriseLoginPublished {
		// Enterprise login discovery, authorization start, and OAuth completion
		// are public only after the product capability is explicitly published.
		engine.GET("/login/enterprise/options", m.handler.EnterpriseLoginOptions)
		engine.POST("/login/enterprise/:provider/authorizations", m.handler.StartEnterpriseLogin)
		engine.GET("/login/enterprise/:provider/callback", m.handler.CompleteEnterpriseAuthorization)
	}
	engine.GET("/external-login/me/bindings", m.handler.ListCurrentUserBindings)
	engine.GET("/external-login/me/:providerCode/start", m.handler.StartCurrentUserBinding)
	engine.GET("/external-identities/me", m.handler.ListMyEnterpriseIdentities)
	engine.POST("/external-identities/me/:provider/authorizations", m.handler.StartMyEnterpriseBinding)
	if enterpriseLoginPublished {
		engine.POST("/external-identities/login/:provider/authorizations", m.handler.StartEnterpriseLogin)
		engine.GET("/external-identities/login/options", m.handler.EnterpriseLoginOptions)
	}
	engine.GET("/external-identities/oauth/callback/:provider", m.handler.CompleteEnterpriseAuthorization)
	engine.PUT("/external-identities/me/:identityId/notification-settings", m.handler.UpdateMyEnterpriseNotificationSettings)
	engine.POST("/external-identities/me/:identityId/test-message", m.handler.SendMyEnterpriseTestMessage)
	engine.DELETE("/external-identities/me/:identityId", m.handler.UnbindMyEnterpriseIdentity)

	adminGate := func(handler app.HandlerFunc) app.HandlerFunc {
		return handler
	}
	engine.GET("/external-login/admin/capabilities", adminGate(m.wrapPermission("system:external-login-provider:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询外部登录Provider能力",
		IncludeResult: true,
	}, m.handler.Capabilities))))
	engine.GET("/external-login/admin/providers", adminGate(m.wrapPermission("system:external-login-provider:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询外部登录Provider列表",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.ListProviders))))
	engine.POST("/external-login/admin/providers", adminGate(m.wrapPermission("system:external-login-provider:add", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "新增外部登录Provider",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.CreateProvider))))
	engine.GET("/external-login/admin/providers/:providerCode", adminGate(m.wrapPermission("system:external-login-provider:query", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询外部登录Provider详情",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.GetProvider))))
	engine.PUT("/external-login/admin/providers/:providerCode", adminGate(m.wrapPermission("system:external-login-provider:edit", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "更新外部登录Provider",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.UpdateProvider))))
	engine.PUT("/external-login/admin/providers/:providerCode/status", adminGate(m.wrapPermission("system:external-login-provider:status", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "修改外部登录Provider状态",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.UpdateProviderStatus))))
	engine.POST("/external-login/admin/providers/:providerCode/client-secret/rotate", adminGate(m.wrapPermission("system:external-login-provider:secret:rotate", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "轮换外部登录Provider密钥",
		IncludeParams: true,
	}, m.handler.RotateClientSecret))))
	engine.GET("/external-login/admin/providers/:providerCode/methods", adminGate(m.wrapPermission("system:external-login-provider:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询外部登录Provider方法",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.ListProviderMethods))))
	engine.GET("/external-login/admin/identities", adminGate(m.wrapPermission("system:external-login-identity:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询外部登录身份绑定",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.ListIdentities))))
	engine.PUT("/external-login/admin/identities/:identityId/status", adminGate(m.wrapPermission("system:external-login-identity:status", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "修改外部登录身份状态",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.UpdateIdentityStatus))))
	engine.GET("/external-login/admin/tokens", adminGate(m.wrapPermission("system:external-oauth-token:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "查询外部OAuth令牌",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.ListTokens))))
	engine.POST("/external-login/admin/tokens/:tokenId/revoke", adminGate(m.wrapPermission("system:external-oauth-token:revoke", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeOther,
		Description:   "撤销外部OAuth令牌",
		IncludeParams: true,
		IncludeResult: true,
	}, m.handler.RevokeToken))))
	engine.GET("/system/enterprise-connections", adminGate(m.wrapPermission("system:enterprise-connection:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation: adminfacade.OperationTypeOther, Description: "查询组织企业连接", IncludeResult: true,
	}, m.handler.ListEnterpriseConnections))))
	engine.GET("/system/enterprise-connections/setup", adminGate(m.wrapPermission("system:enterprise-connection:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation: adminfacade.OperationTypeOther, Description: "查询企业连接配置入口", IncludeResult: true,
	}, m.handler.GetEnterpriseConnectionSetup))))
	engine.GET("/system/enterprise-identities", adminGate(m.wrapPermission("system:enterprise-identity:list", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation: adminfacade.OperationTypeOther, Description: "查询组织企业账号", IncludeResult: true,
	}, m.handler.ListEnterpriseIdentities))))
	engine.PUT("/system/enterprise-connections", adminGate(m.wrapPermission("system:enterprise-connection:edit", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation: adminfacade.OperationTypeOther, Description: "保存组织企业连接", IncludeParams: true, IncludeResult: true,
	}, m.handler.SaveEnterpriseConnection))))
	engine.PUT("/system/enterprise-connections/:connectionId/status", adminGate(m.wrapPermission("system:enterprise-connection:status", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation: adminfacade.OperationTypeOther, Description: "修改组织企业连接状态", IncludeParams: true,
	}, m.handler.SetEnterpriseConnectionStatus))))
	if enterpriseLoginPublished {
		engine.PUT("/system/enterprise-identities/:identityId/login-access", adminGate(m.wrapPermission("system:enterprise-identity:login-access", m.wrapOperation(adminfacade.OperationLogSpec{
			Operation: adminfacade.OperationTypeOther, Description: "修改企业账号登录权限", IncludeParams: true,
		}, m.handler.SetEnterpriseLoginAccess))))
	}
}

func (m *Module) BindAuthorization(auth authorizationfacade.AuthFacade) {
	if m == nil || m.handler == nil {
		return
	}
	m.handler.BindAuthorization(auth)
}

func (m *Module) BindOperationLogger(oplog adminfacade.OperationLogger) {
	if m == nil {
		return
	}
	m.oplog = oplog
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

func (m *Module) wrapOperation(spec adminfacade.OperationLogSpec, handler app.HandlerFunc) app.HandlerFunc {
	if m == nil || m.oplog == nil {
		return handler
	}
	return m.oplog.Wrap(spec, handler)
}

type repositoryAdapter struct {
	*externallogininfra.Repository
}

func (r repositoryAdapter) ListIdentities(ctx context.Context, query domain.IdentityQuery) ([]domain.ExternalIdentity, int64, error) {
	if r.Repository == nil {
		return nil, 0, apperrors.System("external login repository未配置")
	}
	return r.Repository.ListIdentities(ctx, query)
}

type secretValueAdapter struct {
	service secretvalueinfra.Service
}

func (a secretValueAdapter) EncryptString(ctx context.Context, plain string) (externalapp.EncryptedSecretValue, error) {
	value, err := a.service.EncryptString(ctx, plain)
	return toAppSecret(value), err
}

func (a secretValueAdapter) DecryptString(ctx context.Context, value externalapp.EncryptedSecretValue) (string, error) {
	return a.service.DecryptString(ctx, toInfraSecret(value))
}

func (a secretValueAdapter) EncryptBytes(ctx context.Context, plain []byte) (externalapp.EncryptedSecretValue, error) {
	value, err := a.service.EncryptBytes(ctx, plain)
	return toAppSecret(value), err
}

func (a secretValueAdapter) DecryptBytes(ctx context.Context, value externalapp.EncryptedSecretValue) ([]byte, error) {
	return a.service.DecryptBytes(ctx, toInfraSecret(value))
}

func toAppSecret(value secretvalueinfra.SecretValue) externalapp.EncryptedSecretValue {
	return externalapp.EncryptedSecretValue{
		CiphertextB64: value.CiphertextB64,
		EDEKB64:       value.EDEKB64,
		WrapKeyRef:    value.WrapKeyRef,
	}
}

func toInfraSecret(value externalapp.EncryptedSecretValue) secretvalueinfra.SecretValue {
	return secretvalueinfra.SecretValue{
		CiphertextB64: value.CiphertextB64,
		EDEKB64:       value.EDEKB64,
		WrapKeyRef:    value.WrapKeyRef,
	}
}

type driverRegistryAdapter struct {
	registry *drivers.Registry
}

type oidcDiscoveryAdapter struct {
	driver *drivers.OIDCDriver
}

func (a oidcDiscoveryAdapter) DiscoverOIDC(ctx context.Context, issuer string) (externalapp.OIDCDiscoveryResult, error) {
	document, err := a.driver.Discover(ctx, issuer)
	if err != nil {
		return externalapp.OIDCDiscoveryResult{}, err
	}
	return externalapp.OIDCDiscoveryResult{
		Issuer: document.Issuer, AuthorizationEndpoint: document.AuthorizationEndpoint, TokenEndpoint: document.TokenEndpoint,
		UserinfoEndpoint: document.UserinfoEndpoint, JWKSURI: document.JWKSURI,
	}, nil
}

func (a driverRegistryAdapter) Get(providerCode string) (externalapp.ProviderDriverPort, bool) {
	driver, ok := a.registry.Get(providerCode)
	if !ok {
		return nil, false
	}
	return driverAdapter{driver: driver}, true
}

func (a driverRegistryAdapter) Capabilities() map[string]domain.ProviderCapability {
	return a.registry.Capabilities()
}

type driverAdapter struct {
	driver drivers.Driver
}

func (a driverAdapter) BuildAuthorizationURL(ctx context.Context, provider domain.Provider, request externalapp.AuthorizationRequest) (string, error) {
	return a.driver.BuildAuthorizationURL(ctx, provider, drivers.AuthorizationRequest{
		State:               request.State,
		Nonce:               request.Nonce,
		CodeChallenge:       request.CodeChallenge,
		CodeChallengeMethod: request.CodeChallengeMethod,
		RedirectURI:         request.RedirectURI,
		Scopes:              request.Scopes,
		Issuer:              request.Issuer,
	})
}

func (a driverAdapter) ExchangeCode(ctx context.Context, provider domain.Provider, request externalapp.TokenExchangeRequest) (*externalapp.TokenExchangeResult, error) {
	result, err := a.driver.ExchangeCode(ctx, provider, drivers.TokenExchangeRequest{
		Code:           request.Code,
		State:          request.State,
		CodeVerifier:   request.CodeVerifier,
		RedirectURI:    request.RedirectURI,
		ClientSecret:   request.ClientSecret,
		Nonce:          request.Nonce,
		ExpectedIssuer: request.ExpectedIssuer,
		CallbackIssuer: request.CallbackIssuer,
		Scopes:         request.Scopes,
	})
	if err != nil {
		return nil, err
	}
	return &externalapp.TokenExchangeResult{
		TokenSet:         result.TokenSet,
		RawTokenResponse: result.RawTokenResponse,
		ExpectedIssuer:   result.ExpectedIssuer,
		CallbackIssuer:   result.CallbackIssuer,
		ExpectedNonce:    result.ExpectedNonce,
	}, nil
}

func (a driverAdapter) ResolveProfile(ctx context.Context, provider domain.Provider, tokens externalapp.TokenExchangeResult) (*domain.ExternalProfile, error) {
	return a.driver.ResolveProfile(ctx, provider, drivers.TokenExchangeResult{
		TokenSet:         tokens.TokenSet,
		RawTokenResponse: tokens.RawTokenResponse,
		ExpectedIssuer:   tokens.ExpectedIssuer,
		CallbackIssuer:   tokens.CallbackIssuer,
		ExpectedNonce:    tokens.ExpectedNonce,
	})
}

func (a driverAdapter) RevokeToken(ctx context.Context, provider domain.Provider, tokenSet domain.TokenSet) error {
	return a.driver.RevokeToken(ctx, provider, tokenSet)
}
