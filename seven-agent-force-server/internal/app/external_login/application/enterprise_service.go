package application

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
)

const (
	StepUpActionEnterpriseConnectionSave   = "ENTERPRISE_CONNECTION_SAVE"
	StepUpActionEnterpriseConnectionStatus = "ENTERPRISE_CONNECTION_STATUS"
	StepUpActionEnterpriseLoginAccess      = "ENTERPRISE_IDENTITY_LOGIN_ACCESS"
	StepUpActionEnterpriseIdentityUnbind   = "ENTERPRISE_IDENTITY_UNBIND"
)

type EnterpriseRepositoryPort interface {
	ListEnterpriseConnections(context.Context, int64) ([]domain.EnterpriseConnection, error)
	FindEnterpriseConnection(context.Context, int64, domain.EnterpriseProvider) (*domain.EnterpriseConnection, error)
	FindEnterpriseConnectionByID(context.Context, int64, int64) (*domain.EnterpriseConnection, error)
	InsertEnterpriseConnection(context.Context, domain.EnterpriseConnection) error
	UpdateEnterpriseConnection(context.Context, domain.EnterpriseConnection, int64) (bool, error)
	InsertEnterpriseAuthorization(context.Context, domain.EnterpriseAuthorization) error
	ConsumeEnterpriseAuthorization(context.Context, string, time.Time) (*domain.EnterpriseAuthorization, error)
	CompleteEnterpriseAuthorization(context.Context, int64, domain.EnterpriseAuthorizationStatus, *int64, string, time.Time) error
	FindEnterpriseIdentityByDigest(context.Context, int64, int64, string) (*domain.EnterpriseExternalIdentity, error)
	FindEnterpriseIdentityByUser(context.Context, int64, int64, int64) (*domain.EnterpriseExternalIdentity, error)
	FindEnterpriseIdentityByID(context.Context, int64, int64) (*domain.EnterpriseExternalIdentity, error)
	ListEnterpriseIdentitiesByUser(context.Context, int64, int64) ([]domain.EnterpriseExternalIdentity, error)
	ListEnterpriseIdentitiesByOrganization(context.Context, int64) ([]domain.EnterpriseExternalIdentity, error)
	InsertEnterpriseIdentity(context.Context, domain.EnterpriseExternalIdentity) error
	UpdateEnterpriseIdentityStatus(context.Context, int64, int64, int64, int64, domain.EnterpriseIdentityStatus, time.Time) (bool, error)
	AdvanceEnterpriseIdentityVersion(context.Context, int64, int64, int64, int64, time.Time) (bool, error)
	ListEnterpriseGrants(context.Context, int64, int64) ([]domain.EnterpriseIdentityGrant, error)
	UpsertEnterpriseGrant(context.Context, domain.EnterpriseIdentityGrant) error
}

type EnterpriseSubjectDigester interface {
	Digest(ctx context.Context, keyRef string, organizationID, connectionID int64, provider domain.EnterpriseProvider, subjectKind, subject string) (digest, resolvedKeyRef string, err error)
}

type EnterpriseProviderAdapter interface {
	Provider() domain.EnterpriseProvider
	Capabilities() domain.EnterpriseProtocolCapabilities
	BuildAuthorizationURL(connection domain.EnterpriseConnection, state, nonce, codeChallenge, redirectURI string) (string, error)
	ExchangeAndResolve(ctx context.Context, connection domain.EnterpriseConnection, clientSecret, code, codeVerifier, redirectURI string) (EnterpriseResolvedSubject, error)
	Probe(ctx context.Context, connection domain.EnterpriseConnection, clientSecret string) error
	SendPersonalTest(ctx context.Context, connection domain.EnterpriseConnection, clientSecret, subject string) error
}

type EnterpriseResolvedSubject struct {
	Subject     string
	SubjectKind string
	Nonce       string
}

type EnterpriseDiscoveredGroup struct {
	DisplayName string
	Description string
	Target      string
}

type EnterpriseGroupDiscoveryAdapter interface {
	DiscoverGroups(context.Context, domain.EnterpriseConnection, string) ([]EnterpriseDiscoveredGroup, error)
}

type enterpriseAuthorizationOutcome interface{ OutcomeUnknown() bool }

type enterpriseAuthorizationRejection interface {
	AuthorizationRejection() (reason, message string)
}

type enterpriseConnectionRejection interface {
	ConnectionRejection() (reason, message string)
}

type enterpriseGroupDiscoveryRejection interface {
	GroupDiscoveryRejection() (reason, message string)
}

type EnterpriseProviderRegistry interface {
	GetEnterprise(provider domain.EnterpriseProvider) (EnterpriseProviderAdapter, bool)
}

type EnterpriseServiceDeps struct {
	Config      string
	Transactor  TransactorPort
	IDGen       *xid.Generator
	Repository  EnterpriseRepositoryPort
	SecretValue SecretValueService
	Random      RandomTokenGenerator
	Digester    EnterpriseSubjectDigester
	Providers   EnterpriseProviderRegistry
	Sessions    interface {
		RevokeSessionsByExternalIdentity(context.Context, int64) (int64, error)
	}
	Subjects               userfacade.SubjectFacade
	Relations              userfacade.UserRelationFacade
	Orgs                   userfacade.OrgFacade
	Bootstrap              ssofacade.BootstrapSessionFacade
	NotificationRoutes     EnterpriseNotificationRouteProvisioner
	EnterpriseLoginEnabled bool
}

type EnterpriseNotificationRouteProvisioner interface {
	EnsureEnterpriseDirectRoute(ctx context.Context, organizationID, connectionID, connectionVersion int64, connectionDigest, provider string, enabled bool, actorID int64) error
}

type EnterpriseService struct {
	callbackBaseURL string
	tx              TransactorPort
	idGen           *xid.Generator
	repo            EnterpriseRepositoryPort
	secrets         SecretValueService
	random          RandomTokenGenerator
	digester        EnterpriseSubjectDigester
	providers       EnterpriseProviderRegistry
	sessions        interface {
		RevokeSessionsByExternalIdentity(context.Context, int64) (int64, error)
	}
	subjects               userfacade.SubjectFacade
	relations              userfacade.UserRelationFacade
	orgs                   userfacade.OrgFacade
	bootstrap              ssofacade.BootstrapSessionFacade
	notificationRoutes     EnterpriseNotificationRouteProvisioner
	enterpriseLoginEnabled bool
	now                    func() time.Time
}

func NewEnterpriseService(deps EnterpriseServiceDeps) *EnterpriseService {
	if deps.IDGen == nil {
		panic("external_login: enterprise identity requires the framework Snowflake ID generator")
	}
	return &EnterpriseService{callbackBaseURL: strings.TrimRight(strings.TrimSpace(deps.Config), "/"), tx: deps.Transactor, idGen: deps.IDGen, repo: deps.Repository, secrets: deps.SecretValue, random: deps.Random, digester: deps.Digester, providers: deps.Providers, sessions: deps.Sessions, subjects: deps.Subjects, relations: deps.Relations, orgs: deps.Orgs, bootstrap: deps.Bootstrap, notificationRoutes: deps.NotificationRoutes, enterpriseLoginEnabled: deps.EnterpriseLoginEnabled, now: func() time.Time { return time.Now().UTC() }}
}

func (s *EnterpriseService) BindNotificationRoutes(routes EnterpriseNotificationRouteProvisioner) {
	if s != nil {
		s.notificationRoutes = routes
	}
}

func (s *EnterpriseService) ResolveEnterpriseLoginOptions(ctx context.Context, organizationCode string) (*facade.EnterpriseLoginOptions, error) {
	if !s.enterpriseLoginEnabled {
		return nil, reasonError(apperrors.NotFound("企业账号登录暂未开放"), "ENTERPRISE_LOGIN_NOT_PUBLISHED")
	}
	code := strings.TrimSpace(organizationCode)
	if code == "" || s.orgs == nil {
		return nil, reasonError(apperrors.NotFound("组织登录入口不可用"), "EXTERNAL_LOGIN_USER_NOT_PREBOUND")
	}
	organization, err := s.orgs.GetOrgByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if organization == nil || organization.ID <= 0 || organization.Status != 0 {
		return nil, reasonError(apperrors.NotFound("组织登录入口不可用"), "EXTERNAL_LOGIN_USER_NOT_PREBOUND")
	}
	connections, err := s.repo.ListEnterpriseConnections(ctx, organization.ID)
	if err != nil {
		return nil, err
	}
	result := &facade.EnterpriseLoginOptions{OrganizationName: strings.TrimSpace(organization.Name), Methods: []facade.EnterpriseLoginOption{}}
	for _, connection := range connections {
		if connection.Status != domain.EnterpriseConnectionActive || connection.LastProbeStatus != "PASSED" || !connection.LoginEnabled {
			continue
		}
		result.Methods = append(result.Methods, facade.EnterpriseLoginOption{Provider: string(connection.Provider), Label: "使用" + providerLabel(connection.Provider) + "登录"})
	}
	return result, nil
}

func (s *EnterpriseService) ListEnterpriseConnections(ctx context.Context, organizationID int64) ([]facade.EnterpriseConnectionRecord, error) {
	if organizationID <= 0 {
		return nil, reasonError(apperrors.Params("组织身份无效"), "EXTERNAL_IDENTITY_CROSS_ORGANIZATION_FORBIDDEN")
	}
	items, err := s.repo.ListEnterpriseConnections(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	out := make([]facade.EnterpriseConnectionRecord, 0, len(items))
	for _, item := range items {
		record := mapEnterpriseConnectionRecord(item)
		record.CallbackURL = s.enterpriseCallbackURLIfConfigured(item.Provider)
		out = append(out, record)
	}
	return out, nil
}

func (s *EnterpriseService) GetEnterpriseConnectionSetup(ctx context.Context, organizationID int64) (*facade.EnterpriseConnectionSetup, error) {
	if organizationID <= 0 {
		return nil, reasonError(apperrors.Params("组织身份无效"), "EXTERNAL_IDENTITY_CROSS_ORGANIZATION_FORBIDDEN")
	}
	if s.orgs == nil {
		return nil, reasonError(apperrors.Operation("组织登录入口暂不可用"), "ENTERPRISE_CONNECTION_NOT_CONFIGURED")
	}
	organization, err := s.orgs.GetOrgByID(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	if organization == nil || organization.ID != organizationID || organization.Status != 0 || strings.TrimSpace(organization.Code) == "" {
		return nil, reasonError(apperrors.Operation("组织登录入口暂不可用"), "ENTERPRISE_CONNECTION_NOT_CONFIGURED")
	}
	providers := []domain.EnterpriseProvider{domain.EnterpriseProviderFeishu, domain.EnterpriseProviderWeCom}
	organizationCode := strings.TrimSpace(organization.Code)
	result := &facade.EnterpriseConnectionSetup{
		OrganizationCode: organizationCode,
		LoginURL:         "",
		Providers:        make([]facade.EnterpriseConnectionProviderSetup, 0, len(providers)),
	}
	if s.enterpriseLoginEnabled {
		result.LoginURL = s.enterpriseOrganizationLoginURL(organizationCode)
	}
	for _, provider := range providers {
		result.Providers = append(result.Providers, facade.EnterpriseConnectionProviderSetup{
			Provider:    string(provider),
			CallbackURL: s.enterpriseCallbackURLIfConfigured(provider),
		})
	}
	return result, nil
}

func (s *EnterpriseService) enterpriseOrganizationLoginURL(organizationCode string) string {
	base := strings.TrimRight(strings.TrimSpace(s.callbackBaseURL), "/")
	if base == "" || strings.TrimSpace(organizationCode) == "" {
		return ""
	}
	return base + "/login?org=" + url.QueryEscape(strings.TrimSpace(organizationCode))
}

func (s *EnterpriseService) SaveEnterpriseConnection(ctx context.Context, organizationID, actorID int64, req facade.SaveEnterpriseConnectionRequest, proof stepup.ProofMetadata) (*facade.EnterpriseConnectionRecord, error) {
	if !s.enterpriseLoginEnabled {
		req.LoginEnabled = false
	}
	provider, err := domain.NormalizeEnterpriseProvider(req.Provider)
	if err != nil {
		return nil, apperrors.Params("企业连接类型无效")
	}
	if organizationID <= 0 || actorID <= 0 {
		return nil, apperrors.Forbidden("组织或操作者身份无效")
	}
	binding := fmt.Sprintf("enterprise-connection:%d:%s|version:%d", organizationID, provider, req.ExpectedVersion)
	if err := stepup.Require(proof, StepUpActionEnterpriseConnectionSave, binding); err != nil {
		return nil, err
	}
	adapter, ok := s.providers.GetEnterprise(provider)
	if !ok {
		return nil, reasonError(apperrors.Operation("企业连接暂不可用"), "ENTERPRISE_CONNECTION_NOT_CONFIGURED")
	}
	existing, err := s.repo.FindEnterpriseConnection(ctx, organizationID, provider)
	if err != nil {
		return nil, err
	}
	secret := strings.TrimSpace(req.ClientSecret)
	if existing == nil && secret == "" {
		return nil, apperrors.Params("请填写应用凭证")
	}
	if existing != nil && req.ExpectedVersion != existing.Version {
		return nil, reasonError(apperrors.ObjectState("连接已在其他位置更新"), "ENTERPRISE_CONNECTION_DRIFT")
	}
	var encrypted EncryptedSecretValue
	credentialVersion := int64(1)
	if existing != nil {
		encrypted = EncryptedSecretValue{CiphertextB64: existing.ClientSecretCiphertext, EDEKB64: existing.ClientSecretEDEK, WrapKeyRef: existing.ClientSecretWrapKeyRef}
		credentialVersion = existing.CredentialVersion
	}
	if secret != "" {
		encrypted, err = s.secrets.EncryptString(ctx, secret)
		if err != nil {
			return nil, fmt.Errorf("encrypt enterprise application credential: %w", err)
		}
		if existing != nil {
			credentialVersion++
		}
	}
	item := domain.EnterpriseConnection{OrganizationID: organizationID, Provider: provider, DisplayName: boundedDisplayName(req.DisplayName, provider), AppID: strings.TrimSpace(req.AppID), CorpID: strings.TrimSpace(req.CorpID), AgentID: strings.TrimSpace(req.AgentID), EndpointProfile: "OFFICIAL_CN", ClientSecretCiphertext: encrypted.CiphertextB64, ClientSecretEDEK: encrypted.EDEKB64, ClientSecretWrapKeyRef: encrypted.WrapKeyRef, CredentialVersion: credentialVersion, BindingEnabled: req.BindingEnabled, NotificationDirectEnabled: req.NotificationDirectEnabled, NotificationMentionEnabled: req.NotificationMentionEnabled, LoginEnabled: req.LoginEnabled, ProtocolCapabilities: adapter.Capabilities(), Status: domain.EnterpriseConnectionActive, LastProbeStatus: "NOT_PROBED", Version: 1, CreatorID: actorID, UpdaterID: actorID, CreatedAt: s.now(), UpdatedAt: s.now()}
	if existing != nil {
		if item.AppID == "" {
			item.AppID = existing.AppID
		}
		if item.CorpID == "" {
			item.CorpID = existing.CorpID
		}
		if item.AgentID == "" {
			item.AgentID = existing.AgentID
		}
	}
	item.ConfigDigest = enterpriseConnectionDigest(item)
	clientSecret, err := s.secrets.DecryptString(ctx, encrypted)
	if err != nil {
		return nil, err
	}
	if err := adapter.Probe(ctx, item, clientSecret); err != nil {
		var rejected enterpriseConnectionRejection
		if errors.As(err, &rejected) {
			reason, message := rejected.ConnectionRejection()
			return nil, reasonError(apperrors.Operation(message), reason)
		}
		return nil, reasonError(apperrors.Operation("连接检查未通过"), "ENTERPRISE_CONNECTION_NOT_VERIFIED")
	}
	item.LastProbeStatus = "PASSED"
	probeAt := s.now()
	item.LastProbeAt = &probeAt
	if err := s.withTransaction(ctx, func(txCtx context.Context) error {
		if existing == nil {
			item.ID = s.nextID()
			if err := s.repo.InsertEnterpriseConnection(txCtx, item); err != nil {
				return err
			}
		} else {
			item.ID = existing.ID
			item.CreatorID = existing.CreatorID
			item.CreatedAt = existing.CreatedAt
			item.Version = existing.Version
			updated, updateErr := s.repo.UpdateEnterpriseConnection(txCtx, item, existing.Version)
			if updateErr != nil {
				return updateErr
			}
			if !updated {
				return reasonError(apperrors.ObjectState("连接已在其他位置更新"), "ENTERPRISE_CONNECTION_DRIFT")
			}
			item.Version++
		}
		if s.notificationRoutes != nil {
			if err := s.notificationRoutes.EnsureEnterpriseDirectRoute(txCtx, item.OrganizationID, item.ID, item.Version, item.ConfigDigest, string(item.Provider), item.Status == domain.EnterpriseConnectionActive && item.NotificationDirectEnabled, actorID); err != nil {
				return enterpriseRouteProvisionError("连接已验证，但个人通知通道初始化失败，请稍后重试", err)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	record := mapEnterpriseConnectionRecord(item)
	record.CallbackURL = s.enterpriseCallbackURLIfConfigured(item.Provider)
	return &record, nil
}

func (s *EnterpriseService) SetEnterpriseConnectionStatus(ctx context.Context, organizationID, actorID, connectionID int64, req facade.EnterpriseConnectionStatusRequest, proof stepup.ProofMetadata) error {
	binding := fmt.Sprintf("enterprise-connection:%d|enabled:%t|version:%d", connectionID, req.Enabled, req.ExpectedVersion)
	if err := stepup.Require(proof, StepUpActionEnterpriseConnectionStatus, binding); err != nil {
		return err
	}
	item, err := s.repo.FindEnterpriseConnectionByID(ctx, organizationID, connectionID)
	if err != nil {
		return err
	}
	if item == nil {
		return apperrors.NotFound("企业连接不存在")
	}
	if item.Version != req.ExpectedVersion {
		return reasonError(apperrors.ObjectState("连接已在其他位置更新"), "ENTERPRISE_CONNECTION_DRIFT")
	}
	item.Status = domain.EnterpriseConnectionDisabled
	if req.Enabled {
		item.Status = domain.EnterpriseConnectionActive
	}
	item.UpdaterID = actorID
	item.UpdatedAt = s.now()
	item.ConfigDigest = enterpriseConnectionDigest(*item)
	return s.withTransaction(ctx, func(txCtx context.Context) error {
		ok, updateErr := s.repo.UpdateEnterpriseConnection(txCtx, *item, item.Version)
		if updateErr != nil {
			return updateErr
		}
		if !ok {
			return reasonError(apperrors.ObjectState("连接已在其他位置更新"), "ENTERPRISE_CONNECTION_DRIFT")
		}
		if s.notificationRoutes != nil {
			if routeErr := s.notificationRoutes.EnsureEnterpriseDirectRoute(txCtx, item.OrganizationID, item.ID, item.Version+1, item.ConfigDigest, string(item.Provider), req.Enabled && item.NotificationDirectEnabled, actorID); routeErr != nil {
				return enterpriseRouteProvisionError("连接状态暂时无法同步到个人通知通道，请稍后重试", routeErr)
			}
		}
		if req.Enabled || s.sessions == nil {
			return nil
		}
		identities, listErr := s.repo.ListEnterpriseIdentitiesByOrganization(txCtx, organizationID)
		if listErr != nil {
			return listErr
		}
		for _, identity := range identities {
			if identity.ConnectionID != connectionID {
				continue
			}
			if _, revokeErr := s.sessions.RevokeSessionsByExternalIdentity(txCtx, identity.ID); revokeErr != nil {
				return revokeErr
			}
		}
		return nil
	})
}

func (s *EnterpriseService) StartEnterpriseBinding(ctx context.Context, organizationID, userID int64, req facade.EnterpriseAuthorizationStartRequest) (*facade.EnterpriseAuthorizationStartResult, error) {
	if err := s.validateCallbackBaseURL(); err != nil {
		return nil, reasonError(apperrors.Operation("企业账号回调地址尚未配置"), "ENTERPRISE_AUTHORIZATION_REDIRECT_INVALID")
	}
	provider, err := domain.NormalizeEnterpriseProvider(req.Provider)
	if err != nil {
		return nil, apperrors.Params("企业账号类型无效")
	}
	if organizationID <= 0 || userID <= 0 {
		return nil, apperrors.Unauthorized("用户身份无效")
	}
	connection, err := s.repo.FindEnterpriseConnection(ctx, organizationID, provider)
	if err != nil {
		return nil, err
	}
	if connection == nil {
		return nil, reasonError(apperrors.Operation("组织尚未配置该企业账号"), "ENTERPRISE_CONNECTION_NOT_CONFIGURED")
	}
	if connection.Status != domain.EnterpriseConnectionActive || !connection.BindingEnabled || connection.LastProbeStatus != "PASSED" {
		return nil, reasonError(apperrors.Operation("企业账号连接暂不可用"), "ENTERPRISE_CONNECTION_DISABLED")
	}
	adapter, ok := s.providers.GetEnterprise(provider)
	if !ok {
		return nil, reasonError(apperrors.Operation("企业账号连接暂不可用"), "ENTERPRISE_CONNECTION_NOT_CONFIGURED")
	}
	state, err := s.randomToken(ctx)
	if err != nil {
		return nil, err
	}
	authorizationIdentity, err := s.randomToken(ctx)
	if err != nil {
		return nil, err
	}
	nonce := ""
	if adapter.Capabilities().SupportsOIDCNonce {
		nonce, err = s.randomToken(ctx)
		if err != nil {
			return nil, err
		}
	}
	verifier := ""
	challenge := ""
	var encrypted EncryptedSecretValue
	if adapter.Capabilities().SupportsPKCE {
		verifier, err = s.randomToken(ctx)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256([]byte(verifier))
		challenge = base64RawURL(sum[:])
		encrypted, err = s.secrets.EncryptString(ctx, verifier)
		if err != nil {
			return nil, err
		}
	}
	grants, _ := domain.NormalizeRequestedGrants([]domain.EnterpriseGrantType{domain.EnterpriseGrantIdentityLink, domain.EnterpriseGrantNotificationDirect, domain.EnterpriseGrantNotificationMention}, domain.AuthorizationPurposeNotificationBind)
	grantsDigest := digestJSON(grants)
	redirectAfter, err := normalizeInternalRedirect(req.RedirectAfter, "/account/settings")
	if err != nil {
		return nil, reasonError(apperrors.Params("返回地址无效"), "ENTERPRISE_AUTHORIZATION_REDIRECT_INVALID")
	}
	redirectURI := s.enterpriseCallbackURI(provider)
	redirectURL, err := adapter.BuildAuthorizationURL(*connection, state, nonce, challenge, redirectURI)
	if err != nil {
		return nil, err
	}
	now := s.now()
	ttl := 10 * time.Minute
	authorization := domain.EnterpriseAuthorization{ID: s.nextID(), AuthorizationIdentity: authorizationIdentity, Purpose: domain.AuthorizationPurposeNotificationBind, OrganizationID: organizationID, ConnectionID: connection.ID, ConnectionVersion: connection.Version, ConnectionConfigDigest: connection.ConfigDigest, BindUserID: userID, RequestedGrants: grants, RequestedGrantsDigest: grantsDigest, StateHash: hashEnterpriseValue(state), NonceHash: hashEnterpriseValue(nonce), CodeVerifierCiphertext: encrypted.CiphertextB64, CodeVerifierEDEK: encrypted.EDEKB64, CodeVerifierWrapKeyRef: encrypted.WrapKeyRef, RedirectAfter: redirectAfter, ExpiresAt: now.Add(ttl), Status: domain.EnterpriseAuthorizationActive, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.InsertEnterpriseAuthorization(ctx, authorization); err != nil {
		return nil, err
	}
	return &facade.EnterpriseAuthorizationStartResult{AuthorizationIdentity: authorizationIdentity, RedirectURL: redirectURL, ExpiresAt: authorization.ExpiresAt}, nil
}

// StartEnterpriseLogin creates a server-frozen organization authorization. It
// never accepts a local user identity: the callback must resolve an already
// linked subject with an independent LOGIN grant.
func (s *EnterpriseService) StartEnterpriseLogin(ctx context.Context, req facade.EnterpriseLoginStartRequest) (*facade.EnterpriseAuthorizationStartResult, error) {
	if !s.enterpriseLoginEnabled {
		return nil, reasonError(apperrors.NotFound("企业账号登录暂未开放"), "ENTERPRISE_LOGIN_NOT_PUBLISHED")
	}
	if err := s.validateCallbackBaseURL(); err != nil {
		return nil, reasonError(apperrors.Operation("企业账号回调地址尚未配置"), "ENTERPRISE_AUTHORIZATION_REDIRECT_INVALID")
	}
	provider, err := domain.NormalizeEnterpriseProvider(req.Provider)
	if err != nil || s.orgs == nil {
		return nil, reasonError(apperrors.Operation("该组织暂不支持此登录方式"), "ENTERPRISE_CONNECTION_NOT_CONFIGURED")
	}
	organization, err := s.orgs.GetOrgByCode(ctx, strings.TrimSpace(req.OrganizationCode))
	if err != nil || organization == nil || organization.ID <= 0 || organization.Status != 0 {
		return nil, reasonError(apperrors.Operation("该组织暂不支持此登录方式"), "ENTERPRISE_CONNECTION_NOT_CONFIGURED")
	}
	connection, err := s.repo.FindEnterpriseConnection(ctx, organization.ID, provider)
	if err != nil {
		return nil, err
	}
	if connection == nil || connection.Status != domain.EnterpriseConnectionActive || !connection.LoginEnabled || connection.LastProbeStatus != "PASSED" {
		return nil, reasonError(apperrors.Operation("该组织暂不支持此登录方式"), "ENTERPRISE_CONNECTION_NOT_VERIFIED")
	}
	adapter, ok := s.providers.GetEnterprise(provider)
	if !ok {
		return nil, reasonError(apperrors.Operation("该组织暂不支持此登录方式"), "ENTERPRISE_CONNECTION_NOT_CONFIGURED")
	}
	state, err := s.randomToken(ctx)
	if err != nil {
		return nil, err
	}
	authorizationIdentity, err := s.randomToken(ctx)
	if err != nil {
		return nil, err
	}
	nonce, verifier, challenge := "", "", ""
	var encrypted EncryptedSecretValue
	if adapter.Capabilities().SupportsOIDCNonce {
		nonce, err = s.randomToken(ctx)
		if err != nil {
			return nil, err
		}
	}
	if adapter.Capabilities().SupportsPKCE {
		verifier, err = s.randomToken(ctx)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256([]byte(verifier))
		challenge = base64RawURL(sum[:])
		encrypted, err = s.secrets.EncryptString(ctx, verifier)
		if err != nil {
			return nil, err
		}
	}
	redirectAfter, err := normalizeInternalRedirect(req.RedirectAfter, "/")
	if err != nil {
		return nil, reasonError(apperrors.Params("返回地址无效"), "ENTERPRISE_AUTHORIZATION_REDIRECT_INVALID")
	}
	redirectURL, err := adapter.BuildAuthorizationURL(*connection, state, nonce, challenge, s.enterpriseCallbackURI(provider))
	if err != nil {
		return nil, err
	}
	requested := []domain.EnterpriseGrantType{domain.EnterpriseGrantIdentityLink, domain.EnterpriseGrantLogin}
	now := s.now()
	authorization := domain.EnterpriseAuthorization{ID: s.nextID(), AuthorizationIdentity: authorizationIdentity, Purpose: domain.AuthorizationPurposeLogin, OrganizationID: organization.ID, ConnectionID: connection.ID, ConnectionVersion: connection.Version, ConnectionConfigDigest: connection.ConfigDigest, RequestedGrants: requested, RequestedGrantsDigest: digestJSON(requested), StateHash: hashEnterpriseValue(state), NonceHash: hashEnterpriseValue(nonce), CodeVerifierCiphertext: encrypted.CiphertextB64, CodeVerifierEDEK: encrypted.EDEKB64, CodeVerifierWrapKeyRef: encrypted.WrapKeyRef, RedirectAfter: redirectAfter, ExpiresAt: now.Add(10 * time.Minute), Status: domain.EnterpriseAuthorizationActive, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.InsertEnterpriseAuthorization(ctx, authorization); err != nil {
		return nil, err
	}
	return &facade.EnterpriseAuthorizationStartResult{AuthorizationIdentity: authorizationIdentity, RedirectURL: redirectURL, ExpiresAt: authorization.ExpiresAt}, nil
}

func (s *EnterpriseService) CompleteEnterpriseAuthorization(ctx context.Context, req facade.EnterpriseAuthorizationCallbackRequest) (out *facade.EnterpriseAuthorizationResult, returnErr error) {
	provider, err := domain.NormalizeEnterpriseProvider(req.Provider)
	if err != nil {
		return nil, apperrors.Params("企业账号类型无效")
	}
	if strings.TrimSpace(req.State) == "" {
		return nil, reasonError(apperrors.Params("授权回调无效"), "ENTERPRISE_AUTHORIZATION_STATE_INVALID")
	}
	authorization, err := s.repo.ConsumeEnterpriseAuthorization(ctx, hashEnterpriseValue(req.State), s.now())
	if err != nil {
		return nil, err
	}
	if authorization == nil {
		return nil, reasonError(apperrors.ObjectState("授权已失效，请重新开始"), "ENTERPRISE_AUTHORIZATION_STATE_REPLAYED")
	}
	if authorization.Purpose == domain.AuthorizationPurposeLogin && !s.enterpriseLoginEnabled {
		_ = s.repo.CompleteEnterpriseAuthorization(ctx, authorization.ID, domain.EnterpriseAuthorizationFailed, nil, "ENTERPRISE_LOGIN_NOT_PUBLISHED", s.now())
		return nil, reasonError(apperrors.NotFound("企业账号登录暂未开放"), "ENTERPRISE_LOGIN_NOT_PUBLISHED")
	}
	// Consuming the state moves the authorization to PROCESSING before any
	// provider or persistence work begins. Every later error must therefore
	// converge the durable transaction to a terminal state. Explicit failure
	// paths below retain their more specific reason because the repository only
	// updates PROCESSING rows; this fallback catches unexpected database,
	// crypto, and internal integration failures without leaving a state that can
	// never be retried or diagnosed.
	defer func() {
		if returnErr != nil {
			_ = s.repo.CompleteEnterpriseAuthorization(ctx, authorization.ID, domain.EnterpriseAuthorizationFailed, nil, "ENTERPRISE_AUTHORIZATION_INTERNAL_ERROR", s.now())
		}
	}()
	if strings.TrimSpace(req.Error) != "" {
		_ = s.repo.CompleteEnterpriseAuthorization(ctx, authorization.ID, domain.EnterpriseAuthorizationFailed, nil, "ENTERPRISE_AUTHORIZATION_STATE_INVALID", s.now())
		return nil, reasonError(apperrors.Operation("授权未完成"), "ENTERPRISE_AUTHORIZATION_STATE_INVALID")
	}
	if strings.TrimSpace(req.Code) == "" {
		_ = s.repo.CompleteEnterpriseAuthorization(ctx, authorization.ID, domain.EnterpriseAuthorizationFailed, nil, "ENTERPRISE_AUTHORIZATION_STATE_INVALID", s.now())
		return nil, reasonError(apperrors.Params("授权回调无效"), "ENTERPRISE_AUTHORIZATION_STATE_INVALID")
	}
	connection, err := s.repo.FindEnterpriseConnectionByID(ctx, authorization.OrganizationID, authorization.ConnectionID)
	if err != nil {
		return nil, err
	}
	if connection == nil || connection.Provider != provider || connection.Version != authorization.ConnectionVersion || connection.ConfigDigest != authorization.ConnectionConfigDigest || connection.Status != domain.EnterpriseConnectionActive {
		_ = s.repo.CompleteEnterpriseAuthorization(ctx, authorization.ID, domain.EnterpriseAuthorizationFailed, nil, "ENTERPRISE_CONNECTION_DRIFT", s.now())
		return nil, reasonError(apperrors.ObjectState("企业连接已更新，请重新开始"), "ENTERPRISE_CONNECTION_DRIFT")
	}
	if authorization.Purpose == domain.AuthorizationPurposeLogin && !connection.LoginEnabled {
		_ = s.repo.CompleteEnterpriseAuthorization(ctx, authorization.ID, domain.EnterpriseAuthorizationFailed, nil, "EXTERNAL_LOGIN_GRANT_REQUIRED", s.now())
		return nil, reasonError(apperrors.Forbidden("企业登录已停用"), "EXTERNAL_LOGIN_GRANT_REQUIRED")
	}
	adapter, ok := s.providers.GetEnterprise(provider)
	if !ok {
		return nil, reasonError(apperrors.Operation("企业账号连接暂不可用"), "ENTERPRISE_CONNECTION_NOT_CONFIGURED")
	}
	secret, err := s.secrets.DecryptString(ctx, EncryptedSecretValue{CiphertextB64: connection.ClientSecretCiphertext, EDEKB64: connection.ClientSecretEDEK, WrapKeyRef: connection.ClientSecretWrapKeyRef})
	if err != nil {
		return nil, err
	}
	verifier := ""
	if authorization.CodeVerifierCiphertext != "" {
		verifier, err = s.secrets.DecryptString(ctx, EncryptedSecretValue{CiphertextB64: authorization.CodeVerifierCiphertext, EDEKB64: authorization.CodeVerifierEDEK, WrapKeyRef: authorization.CodeVerifierWrapKeyRef})
		if err != nil {
			return nil, err
		}
	}
	resolved, err := adapter.ExchangeAndResolve(ctx, *connection, secret, strings.TrimSpace(req.Code), verifier, s.enterpriseCallbackURI(provider))
	if err != nil {
		status, reason, message := domain.EnterpriseAuthorizationFailed, "ENTERPRISE_AUTHORIZATION_STATE_INVALID", "授权未完成，请重新开始"
		var uncertain enterpriseAuthorizationOutcome
		var rejected enterpriseAuthorizationRejection
		if errors.As(err, &uncertain) && uncertain.OutcomeUnknown() {
			status, reason, message = domain.EnterpriseAuthorizationOutcomeUnknown, "ENTERPRISE_AUTHORIZATION_OUTCOME_UNKNOWN", "授权结果无法确认，请重新开始"
		} else if errors.As(err, &rejected) {
			reason, message = rejected.AuthorizationRejection()
		}
		_ = s.repo.CompleteEnterpriseAuthorization(ctx, authorization.ID, status, nil, reason, s.now())
		return nil, reasonError(apperrors.Operation(message), reason)
	}
	if adapter.Capabilities().SupportsOIDCNonce && (authorization.NonceHash == "" || subtle.ConstantTimeCompare([]byte(hashEnterpriseValue(resolved.Nonce)), []byte(authorization.NonceHash)) != 1) {
		_ = s.repo.CompleteEnterpriseAuthorization(ctx, authorization.ID, domain.EnterpriseAuthorizationFailed, nil, "ENTERPRISE_AUTHORIZATION_NONCE_MISMATCH", s.now())
		return nil, reasonError(apperrors.Forbidden("企业账号授权校验失败"), "ENTERPRISE_AUTHORIZATION_NONCE_MISMATCH")
	}
	subject, subjectKind := strings.TrimSpace(resolved.Subject), resolved.SubjectKind
	if subject == "" || subjectKind != domain.SubjectKindForProvider(provider) {
		_ = s.repo.CompleteEnterpriseAuthorization(ctx, authorization.ID, domain.EnterpriseAuthorizationFailed, nil, "EXTERNAL_IDENTITY_CONFLICT", s.now())
		return nil, reasonError(apperrors.Operation("企业账号身份无效"), "EXTERNAL_IDENTITY_CONFLICT")
	}
	digest, keyRef, err := s.digester.Digest(ctx, "", authorization.OrganizationID, connection.ID, provider, subjectKind, subject)
	if err != nil {
		return nil, err
	}
	existing, err := s.repo.FindEnterpriseIdentityByDigest(ctx, authorization.OrganizationID, connection.ID, digest)
	if err != nil {
		return nil, err
	}
	if authorization.Purpose == domain.AuthorizationPurposeLogin {
		return s.completeEnterpriseLogin(ctx, authorization, connection, provider, existing)
	}
	if existing != nil && existing.UserID != authorization.BindUserID {
		return nil, reasonError(apperrors.ObjectState("该企业账号已绑定其他用户"), "EXTERNAL_IDENTITY_CONFLICT")
	}
	var identity domain.EnterpriseExternalIdentity
	now := s.now()
	err = s.withTransaction(ctx, func(txCtx context.Context) error {
		if existing != nil {
			identity = *existing
			if identity.Status != domain.EnterpriseIdentityActive {
				return reasonError(apperrors.ObjectState("企业账号已解除绑定"), "EXTERNAL_IDENTITY_REVOKED")
			}
		} else {
			encryptedSubject, encryptErr := s.secrets.EncryptString(txCtx, subject)
			if encryptErr != nil {
				return encryptErr
			}
			identity = domain.EnterpriseExternalIdentity{ID: s.nextID(), OrganizationID: authorization.OrganizationID, ConnectionID: connection.ID, Provider: provider, UserID: authorization.BindUserID, SubjectKind: subjectKind, SubjectCiphertext: encryptedSubject.CiphertextB64, SubjectEDEK: encryptedSubject.EDEKB64, SubjectWrapKeyRef: encryptedSubject.WrapKeyRef, SubjectDigest: digest, SubjectDigestKeyRef: keyRef, DisplayHint: "已绑定" + providerLabel(provider) + "账号", Status: domain.EnterpriseIdentityActive, IdentityVersion: 1, VerifiedAt: now, CreatorID: authorization.BindUserID, UpdaterID: authorization.BindUserID, CreatedAt: now, UpdatedAt: now}
			if err := s.repo.InsertEnterpriseIdentity(txCtx, identity); err != nil {
				return err
			}
		}
		for _, grantType := range authorization.RequestedGrants {
			grant := domain.EnterpriseIdentityGrant{ID: s.nextID(), OrganizationID: identity.OrganizationID, ExternalIdentityID: identity.ID, GrantType: grantType, Status: domain.EnterpriseGrantActive, Version: 1, GrantedBy: authorization.BindUserID, GrantedAt: now, CreatedAt: now, UpdatedAt: now}
			if err := s.repo.UpsertEnterpriseGrant(txCtx, grant); err != nil {
				return err
			}
		}
		return s.repo.CompleteEnterpriseAuthorization(txCtx, authorization.ID, domain.EnterpriseAuthorizationSucceeded, &identity.ID, "", now)
	})
	if err != nil {
		return nil, err
	}
	return &facade.EnterpriseAuthorizationResult{Outcome: "BOUND", Purpose: string(authorization.Purpose), Provider: string(provider), ExternalIdentityID: identity.ID, RedirectAfter: authorization.RedirectAfter}, nil
}

func (s *EnterpriseService) completeEnterpriseLogin(ctx context.Context, authorization *domain.EnterpriseAuthorization, connection *domain.EnterpriseConnection, provider domain.EnterpriseProvider, identity *domain.EnterpriseExternalIdentity) (*facade.EnterpriseAuthorizationResult, error) {
	fail := func(code, message string) (*facade.EnterpriseAuthorizationResult, error) {
		_ = s.repo.CompleteEnterpriseAuthorization(ctx, authorization.ID, domain.EnterpriseAuthorizationFailed, nil, code, s.now())
		return nil, reasonError(apperrors.Forbidden(message), code)
	}
	if identity == nil || identity.Status != domain.EnterpriseIdentityActive {
		return fail("EXTERNAL_LOGIN_USER_NOT_PREBOUND", "该企业账号尚未绑定平台用户")
	}
	grants, err := s.repo.ListEnterpriseGrants(ctx, authorization.OrganizationID, identity.ID)
	if err != nil {
		return nil, err
	}
	if !hasActiveGrant(grants, domain.EnterpriseGrantIdentityLink) || !hasActiveGrant(grants, domain.EnterpriseGrantLogin) {
		return fail("EXTERNAL_LOGIN_GRANT_REQUIRED", "该企业账号尚未获得登录权限")
	}
	if s.subjects == nil || s.relations == nil || s.bootstrap == nil {
		return fail("EXTERNAL_LOGIN_USER_NOT_PREBOUND", "企业登录暂不可用")
	}
	user, err := s.subjects.FindSubjectByID(ctx, identity.UserID)
	if err != nil || user == nil || !user.Enabled || user.LockStatus {
		return fail("EXTERNAL_LOGIN_USER_NOT_PREBOUND", "平台账号不可用")
	}
	orgIDs, err := s.relations.ListUserOrgIDs(ctx, identity.UserID)
	if err != nil {
		return nil, err
	}
	if !containsInt64(orgIDs, authorization.OrganizationID) {
		return fail("EXTERNAL_IDENTITY_CROSS_ORGANIZATION_FORBIDDEN", "平台账号不属于该组织")
	}
	result, err := s.bootstrap.BootstrapFirstPartySession(ctx, ssofacade.BootstrapSessionCommand{UserID: identity.UserID, PlatformCode: "seven-admin", ACR: "LEVEL_1", AMR: []string{"oauth", "oauth:" + strings.ToLower(string(provider))}, LoginMethod: "EXTERNAL_OAUTH", ExternalProviderCode: strings.ToLower(string(provider)), ExternalIdentityID: identity.ID})
	if err != nil {
		return nil, err
	}
	if err := s.repo.CompleteEnterpriseAuthorization(ctx, authorization.ID, domain.EnterpriseAuthorizationSucceeded, &identity.ID, "", s.now()); err != nil {
		return nil, err
	}
	return &facade.EnterpriseAuthorizationResult{Outcome: "AUTHENTICATED", Purpose: string(authorization.Purpose), Provider: string(provider), ExternalIdentityID: identity.ID, RedirectAfter: authorization.RedirectAfter, Authenticated: true, SessionCookieHeaderValue: result.SessionCookieHeaderValue, RefreshCookieHeaderValue: result.RefreshCookieHeaderValue}, nil
}

func (s *EnterpriseService) ListMyEnterpriseIdentities(ctx context.Context, organizationID, userID int64) ([]facade.EnterpriseIdentityRecord, error) {
	items, err := s.repo.ListEnterpriseIdentitiesByUser(ctx, organizationID, userID)
	if err != nil {
		return nil, err
	}
	out := make([]facade.EnterpriseIdentityRecord, 0, len(items))
	for _, item := range items {
		grants, err := s.repo.ListEnterpriseGrants(ctx, organizationID, item.ID)
		if err != nil {
			return nil, err
		}
		record := mapEnterpriseIdentityRecord(item, grants)
		if s.subjects != nil {
			if user, lookupErr := s.subjects.BuildPrincipalSeed(ctx, item.UserID); lookupErr == nil && user != nil {
				record.UserDisplayName = strings.TrimSpace(user.NickName)
				if record.UserDisplayName == "" {
					record.UserDisplayName = strings.TrimSpace(user.AccountName)
				}
			}
		}
		out = append(out, record)
	}
	return out, nil
}

func (s *EnterpriseService) ListEnterpriseIdentities(ctx context.Context, organizationID int64) ([]facade.EnterpriseIdentityRecord, error) {
	items, err := s.repo.ListEnterpriseIdentitiesByOrganization(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	out := make([]facade.EnterpriseIdentityRecord, 0, len(items))
	for _, item := range items {
		grants, err := s.repo.ListEnterpriseGrants(ctx, organizationID, item.ID)
		if err != nil {
			return nil, err
		}
		record := mapEnterpriseIdentityRecord(item, grants)
		if s.subjects != nil {
			if user, lookupErr := s.subjects.BuildPrincipalSeed(ctx, item.UserID); lookupErr == nil && user != nil {
				record.UserDisplayName = strings.TrimSpace(user.NickName)
				if record.UserDisplayName == "" {
					record.UserDisplayName = strings.TrimSpace(user.AccountName)
				}
			}
		}
		out = append(out, record)
	}
	return out, nil
}

func (s *EnterpriseService) SendMyEnterpriseTestMessage(ctx context.Context, organizationID, userID, identityID int64) error {
	identity, err := s.repo.FindEnterpriseIdentityByID(ctx, organizationID, identityID)
	if err != nil {
		return err
	}
	if identity == nil || identity.UserID != userID || identity.Status != domain.EnterpriseIdentityActive {
		return reasonError(apperrors.NotFound("企业账号不存在"), "EXTERNAL_IDENTITY_CROSS_ORGANIZATION_FORBIDDEN")
	}
	grants, err := s.repo.ListEnterpriseGrants(ctx, organizationID, identityID)
	if err != nil {
		return err
	}
	if !hasActiveGrant(grants, domain.EnterpriseGrantIdentityLink) || !hasActiveGrant(grants, domain.EnterpriseGrantNotificationDirect) {
		return reasonError(apperrors.Forbidden("个人消息已关闭"), "ENTERPRISE_IDENTITY_GRANT_REQUIRED")
	}
	connection, err := s.repo.FindEnterpriseConnectionByID(ctx, organizationID, identity.ConnectionID)
	if err != nil {
		return err
	}
	if connection == nil || connection.Status != domain.EnterpriseConnectionActive || !connection.NotificationDirectEnabled || connection.LastProbeStatus != "PASSED" {
		return reasonError(apperrors.Operation("个人消息连接不可用"), "ENTERPRISE_CONNECTION_NOT_VERIFIED")
	}
	adapter, ok := s.providers.GetEnterprise(connection.Provider)
	if !ok {
		return reasonError(apperrors.Operation("个人消息连接不可用"), "ENTERPRISE_CONNECTION_NOT_CONFIGURED")
	}
	secret, err := s.secrets.DecryptString(ctx, EncryptedSecretValue{CiphertextB64: connection.ClientSecretCiphertext, EDEKB64: connection.ClientSecretEDEK, WrapKeyRef: connection.ClientSecretWrapKeyRef})
	if err != nil {
		return err
	}
	subject, err := s.secrets.DecryptString(ctx, EncryptedSecretValue{CiphertextB64: identity.SubjectCiphertext, EDEKB64: identity.SubjectEDEK, WrapKeyRef: identity.SubjectWrapKeyRef})
	if err != nil {
		return err
	}
	if err := adapter.SendPersonalTest(ctx, *connection, secret, subject); err != nil {
		return reasonError(apperrors.Operation("测试消息发送失败，请检查连接后重试"), "ENTERPRISE_CONNECTION_NOT_VERIFIED")
	}
	return nil
}

func (s *EnterpriseService) UpdateMyEnterpriseNotificationSettings(ctx context.Context, organizationID, userID, identityID int64, req facade.EnterpriseNotificationSettingsRequest) error {
	identity, err := s.repo.FindEnterpriseIdentityByID(ctx, organizationID, identityID)
	if err != nil {
		return err
	}
	if identity == nil || identity.UserID != userID {
		return reasonError(apperrors.NotFound("企业账号不存在"), "EXTERNAL_IDENTITY_CROSS_ORGANIZATION_FORBIDDEN")
	}
	if identity.IdentityVersion != req.ExpectedIdentityVersion {
		return reasonError(apperrors.ObjectState("设置已在其他位置更新"), "EXTERNAL_IDENTITY_VERSION_CONFLICT")
	}
	now := s.now()
	return s.withTransaction(ctx, func(txCtx context.Context) error {
		for grantType, enabled := range map[domain.EnterpriseGrantType]bool{domain.EnterpriseGrantNotificationDirect: req.DirectEnabled, domain.EnterpriseGrantNotificationMention: req.MentionEnabled} {
			status := domain.EnterpriseGrantRevoked
			if enabled {
				status = domain.EnterpriseGrantActive
			}
			grant := domain.EnterpriseIdentityGrant{ID: s.nextID(), OrganizationID: organizationID, ExternalIdentityID: identityID, GrantType: grantType, Status: status, Version: 1, GrantedBy: userID, GrantedAt: now, ReasonCode: "USER_NOTIFICATION_SETTING", CreatedAt: now, UpdatedAt: now}
			if !enabled {
				grant.RevokedBy = &userID
				grant.RevokedAt = &now
			}
			if err := s.repo.UpsertEnterpriseGrant(txCtx, grant); err != nil {
				return err
			}
		}
		updated, err := s.repo.AdvanceEnterpriseIdentityVersion(txCtx, organizationID, identityID, req.ExpectedIdentityVersion, userID, now)
		if err != nil {
			return err
		}
		if !updated {
			return reasonError(apperrors.ObjectState("设置已在其他位置更新"), "EXTERNAL_IDENTITY_VERSION_CONFLICT")
		}
		return nil
	})
}

func (s *EnterpriseService) UnbindMyEnterpriseIdentity(ctx context.Context, organizationID, userID, identityID int64, req facade.EnterpriseUnbindRequest, proof stepup.ProofMetadata) error {
	binding := fmt.Sprintf("enterprise-identity:%d|unbind|version:%d", identityID, req.ExpectedIdentityVersion)
	if err := stepup.Require(proof, StepUpActionEnterpriseIdentityUnbind, binding); err != nil {
		return err
	}
	identity, err := s.repo.FindEnterpriseIdentityByID(ctx, organizationID, identityID)
	if err != nil {
		return err
	}
	if identity == nil || identity.UserID != userID {
		return reasonError(apperrors.NotFound("企业账号不存在"), "EXTERNAL_IDENTITY_CROSS_ORGANIZATION_FORBIDDEN")
	}
	now := s.now()
	return s.withTransaction(ctx, func(txCtx context.Context) error {
		grants, err := s.repo.ListEnterpriseGrants(txCtx, organizationID, identityID)
		if err != nil {
			return err
		}
		for _, grant := range grants {
			grant.ID = s.nextID()
			grant.Status = domain.EnterpriseGrantRevoked
			grant.RevokedBy = &userID
			grant.RevokedAt = &now
			grant.ReasonCode = "USER_UNBOUND"
			grant.UpdatedAt = now
			if err := s.repo.UpsertEnterpriseGrant(txCtx, grant); err != nil {
				return err
			}
		}
		updated, err := s.repo.UpdateEnterpriseIdentityStatus(txCtx, organizationID, identityID, req.ExpectedIdentityVersion, userID, domain.EnterpriseIdentityRevoked, now)
		if err != nil {
			return err
		}
		if !updated {
			return reasonError(apperrors.ObjectState("企业账号已在其他位置更新"), "EXTERNAL_IDENTITY_VERSION_CONFLICT")
		}
		if s.sessions != nil {
			_, err = s.sessions.RevokeSessionsByExternalIdentity(txCtx, identityID)
		}
		return err
	})
}

func (s *EnterpriseService) SetEnterpriseLoginAccess(ctx context.Context, organizationID, actorID, identityID int64, req facade.EnterpriseLoginAccessRequest, proof stepup.ProofMetadata) error {
	if !s.enterpriseLoginEnabled {
		return reasonError(apperrors.NotFound("企业账号登录暂未开放"), "ENTERPRISE_LOGIN_NOT_PUBLISHED")
	}
	binding := fmt.Sprintf("enterprise-identity:%d|login:%t|version:%d", identityID, req.Enabled, req.ExpectedIdentityVersion)
	if err := stepup.Require(proof, StepUpActionEnterpriseLoginAccess, binding); err != nil {
		return err
	}
	identity, err := s.repo.FindEnterpriseIdentityByID(ctx, organizationID, identityID)
	if err != nil {
		return err
	}
	if identity == nil {
		return apperrors.NotFound("企业账号不存在")
	}
	if identity.IdentityVersion != req.ExpectedIdentityVersion {
		return reasonError(apperrors.ObjectState("企业账号已在其他位置更新"), "EXTERNAL_IDENTITY_VERSION_CONFLICT")
	}
	if req.Enabled {
		connection, findErr := s.repo.FindEnterpriseConnectionByID(ctx, organizationID, identity.ConnectionID)
		if findErr != nil {
			return findErr
		}
		if connection == nil || connection.Status != domain.EnterpriseConnectionActive || connection.LastProbeStatus != "PASSED" || !connection.LoginEnabled {
			return reasonError(apperrors.Operation("请先在企业连接中开启并验证企业登录"), "ENTERPRISE_LOGIN_CONNECTION_REQUIRED")
		}
	}
	now := s.now()
	status := domain.EnterpriseGrantRevoked
	if req.Enabled {
		status = domain.EnterpriseGrantActive
	}
	grant := domain.EnterpriseIdentityGrant{ID: s.nextID(), OrganizationID: organizationID, ExternalIdentityID: identityID, GrantType: domain.EnterpriseGrantLogin, Status: status, Version: 1, GrantedBy: actorID, GrantedAt: now, ReasonCode: strings.TrimSpace(req.Reason), CreatedAt: now, UpdatedAt: now}
	if !req.Enabled {
		grant.RevokedBy = &actorID
		grant.RevokedAt = &now
	}
	return s.withTransaction(ctx, func(txCtx context.Context) error {
		if err := s.repo.UpsertEnterpriseGrant(txCtx, grant); err != nil {
			return err
		}
		updated, updateErr := s.repo.AdvanceEnterpriseIdentityVersion(txCtx, organizationID, identityID, req.ExpectedIdentityVersion, actorID, now)
		if updateErr != nil {
			return updateErr
		}
		if !updated {
			return reasonError(apperrors.ObjectState("企业账号已在其他位置更新"), "EXTERNAL_IDENTITY_VERSION_CONFLICT")
		}
		if !req.Enabled && s.sessions != nil {
			_, updateErr = s.sessions.RevokeSessionsByExternalIdentity(txCtx, identityID)
		}
		return updateErr
	})
}

func (s *EnterpriseService) ResolveNotificationAuthority(ctx context.Context, req facade.NotificationAuthorityRequest) (*facade.NotificationAuthority, error) {
	provider, err := domain.NormalizeEnterpriseProvider(req.Provider)
	if err != nil {
		return nil, err
	}
	grantType, err := domain.NormalizeEnterpriseGrant(req.GrantType)
	if err != nil {
		return nil, err
	}
	if grantType != domain.EnterpriseGrantNotificationDirect && grantType != domain.EnterpriseGrantNotificationMention {
		return nil, reasonError(apperrors.Forbidden("企业通知授权类型无效"), "ENTERPRISE_IDENTITY_GRANT_REQUIRED")
	}
	connection, err := s.repo.FindEnterpriseConnection(ctx, req.OrganizationID, provider)
	if err != nil {
		return nil, err
	}
	if connection == nil || connection.Status != domain.EnterpriseConnectionActive || connection.LastProbeStatus != "PASSED" {
		return nil, reasonError(apperrors.Operation("企业通知连接不可用"), "ENTERPRISE_CONNECTION_NOT_VERIFIED")
	}
	identity, err := s.repo.FindEnterpriseIdentityByUser(ctx, req.OrganizationID, connection.ID, req.UserID)
	if err != nil {
		return nil, err
	}
	if identity == nil || identity.Status != domain.EnterpriseIdentityActive {
		return nil, reasonError(apperrors.Operation("尚未绑定企业账号"), "EXTERNAL_IDENTITY_NOT_VERIFIED")
	}
	grants, err := s.repo.ListEnterpriseGrants(ctx, req.OrganizationID, identity.ID)
	if err != nil {
		return nil, err
	}
	if !hasActiveGrant(grants, domain.EnterpriseGrantIdentityLink) || !hasActiveGrant(grants, grantType) {
		return nil, reasonError(apperrors.Forbidden("企业通知授权已关闭"), "ENTERPRISE_IDENTITY_GRANT_REQUIRED")
	}
	subject, err := s.secrets.DecryptString(ctx, EncryptedSecretValue{CiphertextB64: identity.SubjectCiphertext, EDEKB64: identity.SubjectEDEK, WrapKeyRef: identity.SubjectWrapKeyRef})
	if err != nil {
		return nil, err
	}
	expires := s.now().Add(2 * time.Minute)
	authorityDigest := digestJSON([]any{req.OrganizationID, req.UserID, identity.ID, identity.IdentityVersion, connection.ID, connection.Version, grantType, identity.SubjectDigest})
	return &facade.NotificationAuthority{OrganizationID: req.OrganizationID, UserID: req.UserID, ExternalIdentityID: identity.ID, IdentityVersion: identity.IdentityVersion, EnterpriseConnectionID: connection.ID, ConnectionVersion: connection.Version, ConnectionConfigDigest: connection.ConfigDigest, IdentityKind: identity.SubjectKind, Subject: subject, GrantType: string(grantType), AuthorityDigest: authorityDigest, ExpiresAt: expires}, nil
}

func (s *EnterpriseService) ValidateNotificationAuthority(ctx context.Context, snapshot facade.NotificationAuthoritySnapshot) error {
	if snapshot.OrganizationID <= 0 || snapshot.UserID <= 0 || snapshot.ExternalIdentityID <= 0 || snapshot.EnterpriseConnectionID <= 0 || snapshot.IdentityVersion <= 0 || snapshot.ConnectionVersion <= 0 || strings.TrimSpace(snapshot.AuthorityDigest) == "" {
		return reasonError(apperrors.Forbidden("企业通知授权无效"), "ENTERPRISE_NOTIFICATION_AUTHORITY_REVOKED")
	}
	identity, err := s.repo.FindEnterpriseIdentityByID(ctx, snapshot.OrganizationID, snapshot.ExternalIdentityID)
	if err != nil {
		return err
	}
	if identity == nil || identity.Status != domain.EnterpriseIdentityActive || identity.UserID != snapshot.UserID || identity.ConnectionID != snapshot.EnterpriseConnectionID || identity.IdentityVersion != snapshot.IdentityVersion {
		return reasonError(apperrors.Forbidden("企业通知授权已失效"), "ENTERPRISE_NOTIFICATION_AUTHORITY_REVOKED")
	}
	connection, err := s.repo.FindEnterpriseConnectionByID(ctx, snapshot.OrganizationID, snapshot.EnterpriseConnectionID)
	if err != nil {
		return err
	}
	if connection == nil || connection.Status != domain.EnterpriseConnectionActive || connection.Version != snapshot.ConnectionVersion || connection.LastProbeStatus != "PASSED" {
		return reasonError(apperrors.Forbidden("企业通知连接已失效"), "ENTERPRISE_NOTIFICATION_AUTHORITY_REVOKED")
	}
	grantType, err := domain.NormalizeEnterpriseGrant(snapshot.GrantType)
	if err != nil || (grantType != domain.EnterpriseGrantNotificationDirect && grantType != domain.EnterpriseGrantNotificationMention) {
		return reasonError(apperrors.Forbidden("企业通知授权已失效"), "ENTERPRISE_NOTIFICATION_AUTHORITY_REVOKED")
	}
	grants, err := s.repo.ListEnterpriseGrants(ctx, snapshot.OrganizationID, snapshot.ExternalIdentityID)
	if err != nil {
		return err
	}
	if !hasActiveGrant(grants, domain.EnterpriseGrantIdentityLink) || !hasActiveGrant(grants, grantType) {
		return reasonError(apperrors.Forbidden("企业通知授权已撤销"), "ENTERPRISE_NOTIFICATION_AUTHORITY_REVOKED")
	}
	expected := digestJSON([]any{snapshot.OrganizationID, snapshot.UserID, identity.ID, identity.IdentityVersion, connection.ID, connection.Version, grantType, identity.SubjectDigest})
	if subtle.ConstantTimeCompare([]byte(expected), []byte(snapshot.AuthorityDigest)) != 1 {
		return reasonError(apperrors.Forbidden("企业通知授权已漂移"), "ENTERPRISE_NOTIFICATION_AUTHORITY_REVOKED")
	}
	return nil
}

func (s *EnterpriseService) AcquireApplicationCredentialLease(ctx context.Context, organizationID, connectionID, connectionVersion int64, connectionConfigDigest string) (*facade.EnterpriseApplicationCredentialLease, error) {
	connection, err := s.repo.FindEnterpriseConnectionByID(ctx, organizationID, connectionID)
	if err != nil {
		return nil, err
	}
	if connection == nil || connection.Status != domain.EnterpriseConnectionActive || connection.LastProbeStatus != "PASSED" || connection.Version != connectionVersion || connection.ConfigDigest != strings.TrimSpace(connectionConfigDigest) {
		return nil, reasonError(apperrors.Forbidden("企业应用凭证授权已失效"), "ENTERPRISE_CONNECTION_DRIFT")
	}
	secret, err := s.secrets.DecryptString(ctx, EncryptedSecretValue{CiphertextB64: connection.ClientSecretCiphertext, EDEKB64: connection.ClientSecretEDEK, WrapKeyRef: connection.ClientSecretWrapKeyRef})
	if err != nil || strings.TrimSpace(secret) == "" {
		return nil, reasonError(apperrors.Operation("企业应用凭证不可用"), "ENTERPRISE_CONNECTION_NOT_VERIFIED")
	}
	return &facade.EnterpriseApplicationCredentialLease{OrganizationID: organizationID, EnterpriseConnectionID: connection.ID, ConnectionVersion: connection.Version, ConnectionConfigDigest: connection.ConfigDigest, Provider: string(connection.Provider), AppID: connection.AppID, CorpID: connection.CorpID, AgentID: connection.AgentID, ClientSecret: secret, CredentialVersion: connection.CredentialVersion, ExpiresAt: s.now().Add(time.Minute)}, nil
}

func (s *EnterpriseService) DiscoverEnterpriseGroups(ctx context.Context, organizationID, connectionID int64) ([]facade.EnterpriseGroupTarget, error) {
	connection, err := s.repo.FindEnterpriseConnectionByID(ctx, organizationID, connectionID)
	if err != nil {
		return nil, err
	}
	if connection == nil || connection.Provider != domain.EnterpriseProviderFeishu || connection.Status != domain.EnterpriseConnectionActive || connection.LastProbeStatus != "PASSED" {
		return nil, reasonError(apperrors.ObjectState("飞书应用连接不可用于发现群"), "ENTERPRISE_CONNECTION_NOT_VERIFIED")
	}
	adapter, ok := s.providers.GetEnterprise(connection.Provider)
	if !ok {
		return nil, apperrors.System("飞书企业身份适配器未配置")
	}
	discovery, ok := adapter.(EnterpriseGroupDiscoveryAdapter)
	if !ok {
		return nil, apperrors.Operation("当前飞书连接不支持群发现")
	}
	secret, err := s.secrets.DecryptString(ctx, EncryptedSecretValue{CiphertextB64: connection.ClientSecretCiphertext, EDEKB64: connection.ClientSecretEDEK, WrapKeyRef: connection.ClientSecretWrapKeyRef})
	if err != nil {
		return nil, err
	}
	groups, err := discovery.DiscoverGroups(ctx, *connection, secret)
	if err != nil {
		var rejected enterpriseGroupDiscoveryRejection
		if errors.As(err, &rejected) {
			reason, message := rejected.GroupDiscoveryRejection()
			return nil, apperrors.Operation(message).WithDetails(map[string]string{
				"reasonCode":    reason,
				"managementUrl": "https://open.feishu.cn/app/" + url.PathEscape(connection.AppID) + "/auth",
			})
		}
		return nil, reasonError(apperrors.Operation("飞书应用可见群暂时无法加载，请稍后重试"), "ENTERPRISE_GROUP_DISCOVERY_FAILED")
	}
	result := make([]facade.EnterpriseGroupTarget, 0, len(groups))
	for _, group := range groups {
		if strings.TrimSpace(group.Target) == "" || strings.TrimSpace(group.DisplayName) == "" {
			continue
		}
		result = append(result, facade.EnterpriseGroupTarget{DisplayName: boundedDisplayName(group.DisplayName, connection.Provider), Description: boundedDisplayName(group.Description, connection.Provider), Target: group.Target, ConnectionID: connection.ID, ConnectionVersion: connection.Version, ConnectionConfigDigest: connection.ConfigDigest})
		if len(result) == 100 {
			break
		}
	}
	return result, nil
}

func (s *EnterpriseService) ResolveEnterpriseReadiness(ctx context.Context, organizationID, userID int64, providerRaw string) (*facade.EnterpriseIdentityReadiness, error) {
	provider, err := domain.NormalizeEnterpriseProvider(providerRaw)
	if err != nil {
		return nil, err
	}
	result := &facade.EnterpriseIdentityReadiness{}
	connection, err := s.repo.FindEnterpriseConnection(ctx, organizationID, provider)
	if err != nil {
		return nil, err
	}
	if connection == nil {
		result.ReasonCode = "ENTERPRISE_CONNECTION_NOT_CONFIGURED"
		return result, nil
	}
	result.Available = connection.Status == domain.EnterpriseConnectionActive && connection.LastProbeStatus == "PASSED"
	if !result.Available {
		result.ReasonCode = "ENTERPRISE_CONNECTION_NOT_VERIFIED"
		return result, nil
	}
	identity, err := s.repo.FindEnterpriseIdentityByUser(ctx, organizationID, connection.ID, userID)
	if err != nil {
		return nil, err
	}
	if identity == nil || identity.Status != domain.EnterpriseIdentityActive {
		result.ReasonCode = "EXTERNAL_IDENTITY_NOT_VERIFIED"
		return result, nil
	}
	grants, err := s.repo.ListEnterpriseGrants(ctx, organizationID, identity.ID)
	if err != nil {
		return nil, err
	}
	linked := hasActiveGrant(grants, domain.EnterpriseGrantIdentityLink)
	result.DirectReady = linked && connection.NotificationDirectEnabled && hasActiveGrant(grants, domain.EnterpriseGrantNotificationDirect)
	result.MentionReady = linked && connection.NotificationMentionEnabled && hasActiveGrant(grants, domain.EnterpriseGrantNotificationMention)
	if !result.DirectReady && !result.MentionReady {
		result.ReasonCode = "ENTERPRISE_IDENTITY_GRANT_REQUIRED"
	}
	return result, nil
}

func (s *EnterpriseService) withTransaction(ctx context.Context, fn func(context.Context) error) error {
	if s.tx != nil && s.tx.Enabled() {
		return s.tx.WithinTransaction(ctx, fn)
	}
	return fn(ctx)
}
func (s *EnterpriseService) nextID() int64 {
	return s.idGen.NextID()
}
func (s *EnterpriseService) randomToken(ctx context.Context) (string, error) {
	if s.random == nil {
		return "", fmt.Errorf("enterprise identity random token generator is not configured")
	}
	return s.random.Token(ctx)
}
func (s *EnterpriseService) enterpriseCallbackURI(provider domain.EnterpriseProvider) string {
	return s.callbackBaseURL + "/external-identities/oauth/callback/" + strings.ToLower(string(provider))
}

func (s *EnterpriseService) enterpriseCallbackURLIfConfigured(provider domain.EnterpriseProvider) string {
	if s.validateCallbackBaseURL() != nil {
		return ""
	}
	return s.enterpriseCallbackURI(provider)
}
func (s *EnterpriseService) validateCallbackBaseURL() error {
	parsed, err := url.Parse(s.callbackBaseURL)
	if err != nil || parsed == nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("invalid enterprise callback base URL")
	}
	return nil
}
func hasActiveGrant(items []domain.EnterpriseIdentityGrant, target domain.EnterpriseGrantType) bool {
	for _, item := range items {
		if item.GrantType == target && item.Status == domain.EnterpriseGrantActive {
			return true
		}
	}
	return false
}
func containsInt64(items []int64, target int64) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
func enterpriseConnectionDigest(item domain.EnterpriseConnection) string {
	return digestJSON([]any{item.OrganizationID, item.Provider, item.AppID, item.CorpID, item.AgentID, item.EndpointProfile, item.CredentialVersion, item.BindingEnabled, item.NotificationDirectEnabled, item.NotificationMentionEnabled, item.LoginEnabled, item.ProtocolCapabilities})
}
func digestJSON(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func hashEnterpriseValue(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}
func base64RawURL(value []byte) string { return base64.RawURLEncoding.EncodeToString(value) }
func normalizeInternalRedirect(raw, fallback string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = fallback
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Fragment != "" || !strings.HasPrefix(parsed.Path, "/") || strings.HasPrefix(parsed.Path, "//") || strings.Contains(parsed.Path, "\\") || strings.Contains(parsed.Path, "..") {
		return "", fmt.Errorf("invalid internal redirect")
	}
	return parsed.String(), nil
}
func boundedDisplayName(value string, provider domain.EnterpriseProvider) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return providerLabel(provider)
	}
	runes := []rune(value)
	if len(runes) > 128 {
		runes = runes[:128]
	}
	return string(runes)
}
func providerLabel(provider domain.EnterpriseProvider) string {
	if provider == domain.EnterpriseProviderWeCom {
		return "企业微信"
	}
	return "飞书"
}
func maskedHint(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= 6 {
		return "已配置"
	}
	return string(runes[:3]) + "…" + string(runes[len(runes)-2:])
}
func mapEnterpriseConnectionRecord(item domain.EnterpriseConnection) facade.EnterpriseConnectionRecord {
	return facade.EnterpriseConnectionRecord{ID: item.ID, Provider: string(item.Provider), DisplayName: item.DisplayName, AppIDHint: maskedHint(item.AppID), CorpIDHint: maskedHint(item.CorpID), AgentIDHint: maskedHint(item.AgentID), CredentialConfigured: item.ClientSecretCiphertext != "", CredentialVersion: item.CredentialVersion, BindingEnabled: item.BindingEnabled, NotificationDirectEnabled: item.NotificationDirectEnabled, NotificationMentionEnabled: item.NotificationMentionEnabled, LoginEnabled: item.LoginEnabled, Status: string(item.Status), LastProbeStatus: item.LastProbeStatus, LastProbeReasonCode: item.LastProbeReasonCode, LastProbeAt: item.LastProbeAt, Version: item.Version, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
}
func mapEnterpriseIdentityRecord(item domain.EnterpriseExternalIdentity, grants []domain.EnterpriseIdentityGrant) facade.EnterpriseIdentityRecord {
	return facade.EnterpriseIdentityRecord{ID: item.ID, UserID: item.UserID, Provider: string(item.Provider), DisplayHint: item.DisplayHint, Status: string(item.Status), IdentityVersion: item.IdentityVersion, DirectEnabled: hasActiveGrant(grants, domain.EnterpriseGrantNotificationDirect), MentionEnabled: hasActiveGrant(grants, domain.EnterpriseGrantNotificationMention), LoginEnabled: hasActiveGrant(grants, domain.EnterpriseGrantLogin), VerifiedAt: item.VerifiedAt, RevokedAt: item.RevokedAt}
}
func reasonError(err *apperrors.AppError, reason string) *apperrors.AppError {
	return err.WithDetails(map[string]string{"reasonCode": reason})
}

func enterpriseRouteProvisionError(message string, cause error) *apperrors.AppError {
	return apperrors.Wrap(apperrors.CodeOperateError, apperrors.KindOperation, message, cause).
		WithDetails(map[string]string{"reasonCode": "ENTERPRISE_NOTIFICATION_ROUTE_PROVISION_FAILED"})
}
