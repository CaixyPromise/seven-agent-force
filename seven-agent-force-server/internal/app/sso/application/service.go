package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	challengedomain "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/sso/domain"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	cachefacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	jwtinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/jwt"
	passwordinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/password"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/federation"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/ssohelper"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
	"github.com/bytedance/sonic"
)

type Service struct {
	cfg                   config.SSOConfig
	idGen                 *xid.Generator
	repository            SessionRepository
	cache                 AuthorizationSessionCachePort
	validityCache         ActiveSessionValidityCachePort
	validityInvalidations cachefacade.TargetedInvalidationRegistrar
	jwt                   *jwtinfra.Service
	password              *passwordinfra.Service
	profiles              userfacade.ProfileFacade
	subjects              userfacade.SubjectFacade
	transactor            store.Transactor
}

// ActiveSessionCandidate is the request-local result of the DG6.2 eligible
// cache read. It is deliberately not domain.Session: its SessionID is copied
// only from the current cookie/JWT request, while every other field maps to
// the minimal active-session-validity projection. Management, audit, refresh
// rotation, and session-list code must continue to use ResolveActiveSession.
type ActiveSessionCandidate struct {
	SessionID string
	UserID    int64
	ClientID  string
	ACR       string
	AMR       []string
	ExpiresAt time.Time
}

func (s *Service) BindActiveSessionValidityInvalidations(registrar cachefacade.TargetedInvalidationRegistrar) {
	if s != nil {
		s.validityInvalidations = registrar
	}
}

// registerActiveSessionValidityTargets is called from the same transaction
// that changes session facts. It registers every page entry; the bounded audit
// snapshot is deliberately not the invalidation set.
func (s *Service) registerActiveSessionValidityTargets(ctx context.Context, sessions []domain.Session) error {
	if s == nil || s.validityInvalidations == nil || !s.validityInvalidations.Enabled() {
		return nil
	}
	if store.SQLXFromContext(ctx) == nil {
		return apperrors.System("会话缓存失效要求活动数据库事务")
	}
	// Single-session paths retain an exact target-only lease. The global batch
	// writer order is reserved for the paged collectors below, where more than
	// one digest could otherwise create a lock-order cycle.
	for _, item := range sessions {
		request, ok := cachepolicy.ActiveSessionValidityReadRequest(item.SessionID)
		if !ok {
			return apperrors.System("会话缓存失效目标无效")
		}
		fence, err := s.validityInvalidations.AcquireTargetMutationFence(ctx, request.Entry.DataClass, request.TargetKind, request.TargetDigest)
		if err != nil || fence == nil {
			if err != nil {
				return err
			}
			return apperrors.System("会话缓存失效围栏不可用")
		}
		registration, err := s.validityInvalidations.RegisterTarget(ctx, request.Entry.DataClass, request.TargetKind, request.TargetDigest)
		if err != nil {
			fence.Release()
			return err
		}
		if !store.RegisterAfterRollback(ctx, fence.Release) {
			fence.Release()
			return apperrors.System("会话缓存失效回滚回调不可用")
		}
		if !store.RegisterAfterCommit(ctx, func() { s.validityInvalidations.AfterTargetCommit(context.Background(), registration); fence.Release() }) {
			fence.Release()
			return apperrors.System("会话缓存失效提交回调不可用")
		}
	}
	return nil
}

// beginActiveSessionValidityMutationFence owns one physical advisory-lock
// connection for every target discovered by one business transaction. Its
// release callbacks are registered exactly once; event-specific callbacks only
// dirty local L1 after a successful commit.
func (s *Service) beginActiveSessionValidityMutationFence(ctx context.Context) (cachepolicy.TargetedMutationFence, error) {
	if s == nil || s.validityInvalidations == nil || !s.validityInvalidations.Enabled() {
		return nil, nil
	}
	if store.SQLXFromContext(ctx) == nil {
		return nil, apperrors.System("会话缓存失效要求活动数据库事务")
	}
	fence, err := s.validityInvalidations.BeginTargetMutationFence(ctx)
	if err != nil || fence == nil {
		if err != nil {
			return nil, err
		}
		return nil, apperrors.System("会话缓存失效围栏不可用")
	}
	if !store.RegisterAfterRollback(ctx, fence.Release) {
		fence.Release()
		return nil, apperrors.System("会话缓存失效回滚回调不可用")
	}
	if !store.RegisterAfterCommit(ctx, fence.Release) {
		fence.Release()
		return nil, apperrors.System("会话缓存失效提交回调不可用")
	}
	return fence, nil
}

func (s *Service) registerActiveSessionValidityTargetsWithFence(ctx context.Context, sessions []domain.Session, fence cachepolicy.TargetedMutationFence) error {
	if s == nil || s.validityInvalidations == nil || !s.validityInvalidations.Enabled() {
		return nil
	}
	if fence == nil {
		return apperrors.System("会话缓存失效围栏不可用")
	}
	for _, item := range sessions {
		request, ok := cachepolicy.ActiveSessionValidityReadRequest(item.SessionID)
		if !ok {
			return apperrors.System("会话缓存失效目标无效")
		}
		if err := fence.AcquireTargetedMutation(ctx, request.Entry.DataClass, request.TargetKind, request.TargetDigest); err != nil {
			return err
		}
		registration, err := s.validityInvalidations.RegisterTarget(ctx, request.Entry.DataClass, request.TargetKind, request.TargetDigest)
		if err != nil {
			return err
		}
		if !store.RegisterAfterCommit(ctx, func() { s.validityInvalidations.AfterTargetCommit(context.Background(), registration) }) {
			return apperrors.System("会话缓存失效提交回调不可用")
		}
	}
	return nil
}

func (s *Service) requireActiveSessionValidityTransaction(ctx context.Context, operation func(context.Context) error) error {
	if s == nil || s.validityInvalidations == nil || !s.validityInvalidations.Enabled() || store.SQLXFromContext(ctx) != nil {
		return operation(ctx)
	}
	if s.transactor == nil || !s.transactor.Enabled() {
		return apperrors.System("会话缓存失效事务未配置")
	}
	return s.transactor.WithinTransaction(ctx, operation)
}

// BindActiveSessionValidityCache is called only after cache-governance
// composition. Until then all candidate reads remain authoritative.
func (s *Service) BindActiveSessionValidityCache(cache ActiveSessionValidityCachePort) {
	if s != nil {
		s.validityCache = cache
	}
}

// BindTransactor enables atomic system-managed client provisioning.
func (s *Service) BindTransactor(transactor store.Transactor) {
	if s != nil {
		s.transactor = transactor
	}
}

const (
	ssoAuditEventTokenExchanged                  = "TOKEN_EXCHANGED"
	ssoAuditEventTokenRefreshed                  = "TOKEN_REFRESHED"
	ssoAuditEventRefreshReuse                    = "TOKEN_REFRESH_REUSE_DETECTED"
	ssoAuditEventTokenRevoked                    = "TOKEN_REVOKED"
	ssoAuditEventTokenIntrospected               = "TOKEN_INTROSPECTED"
	ssoAuditEventUserInfoAccessed                = "USERINFO_ACCESSED"
	ssoAuditEventSessionRevoked                  = "SESSION_REVOKED"
	ssoAuditEventUserSessionsRevoked             = "USER_SESSIONS_REVOKED"
	ssoAuditEventPlatformSessionsRevoked         = "PLATFORM_SESSIONS_REVOKED"
	ssoAuditEventPlatformLoginMethodRevoked      = "PLATFORM_LOGIN_METHOD_SESSIONS_REVOKED"
	ssoAuditEventExternalProviderSessionsRevoked = "EXTERNAL_PROVIDER_SESSIONS_REVOKED"
	ssoAuditEventExternalIdentitySessionsRevoked = "EXTERNAL_IDENTITY_SESSIONS_REVOKED"

	ssoAuditResultSuccess = "SUCCESS"
	ssoAuditResultFailure = "FAILURE"

	clientRedirectURIInputMax     = 100
	sessionRevocationPageSize     = 100
	sessionRevocationAuditItemMax = 100
)

var clientIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._:-]{3,128}$`)
var externalProviderCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
var managedClientOwnerPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,63}$`)

type managedClientMetadata struct {
	ManagedBy     string `json:"managedBy"`
	OwnerNodeCode string `json:"ownerNodeCode"`
}

type clientAdminBindingPayload struct {
	ClientID           string   `json:"clientId"`
	ClientName         string   `json:"clientName"`
	ClientType         string   `json:"clientType"`
	ClientAuthMethod   string   `json:"clientAuthMethod"`
	GrantTypes         []string `json:"grantTypes"`
	Scopes             []string `json:"scopes"`
	RequirePKCE        bool     `json:"requirePkce"`
	RequireConsent     bool     `json:"requireConsent"`
	TrustedFirstParty  bool     `json:"trustedFirstParty"`
	AccessTokenTTLSec  int      `json:"accessTokenTtlSec"`
	RefreshTokenTTLSec int      `json:"refreshTokenTtlSec"`
	MetadataJSON       string   `json:"metadataJson"`
}

type clientStatusBindingPayload struct {
	ClientID             string `json:"clientId"`
	Status               int    `json:"status"`
	Reason               string `json:"reason"`
	RevokeActiveSessions bool   `json:"revokeActiveSessions"`
}

type clientSecretGenerateBindingPayload struct {
	ClientID      string `json:"clientId"`
	ExpiresInDays int    `json:"expiresInDays"`
	Reason        string `json:"reason"`
}

type clientSecretStatusBindingPayload struct {
	ClientID            string `json:"clientId"`
	SecretID            int64  `json:"secretId"`
	Status              int    `json:"status"`
	Reason              string `json:"reason"`
	AllowNoActiveSecret bool   `json:"allowNoActiveSecret"`
}

func NewService(
	cfg config.SSOConfig,
	idGen *xid.Generator,
	repository SessionRepository,
	cache AuthorizationSessionCachePort,
	jwtService *jwtinfra.Service,
	password *passwordinfra.Service,
	profiles userfacade.ProfileFacade,
	subjects userfacade.SubjectFacade,
) *Service {
	return &Service{
		cfg:        cfg,
		idGen:      idGen,
		repository: repository,
		cache:      cache,
		jwt:        jwtService,
		password:   password,
		profiles:   profiles,
		subjects:   subjects,
	}
}

func (s *Service) ResolveActiveSession(ctx context.Context, sessionID string) (*domain.Session, error) {
	item, err := s.repository.FindSessionBySessionID(ctx, strings.TrimSpace(sessionID))
	if err != nil || item == nil {
		return nil, err
	}
	now := time.Now().UTC()
	if item.Status != domain.SessionStatusActive || item.ExpiresAt.Before(now) || (item.RevokedAt != nil && !item.RevokedAt.IsZero()) {
		return nil, nil
	}
	if s.cache != nil {
		revokedAt, cacheErr := s.cache.UserRevokedAt(ctx, item.UserID)
		if cacheErr != nil {
			return nil, cacheErr
		}
		if revokedAt != nil && (item.CreateTime.IsZero() || revokedByUserWatermark(item.CreateTime, *revokedAt)) {
			return nil, nil
		}
	}
	return item, nil
}

// ResolveActiveSessionForCandidateUse is limited to cookie automatic
// authorization and post-cryptographic JWT access-token validation. Refresh
// rotation and management/audit paths keep using ResolveActiveSession.
func (s *Service) ResolveActiveSessionForCandidateUse(ctx context.Context, sessionID string) (*ActiveSessionCandidate, error) {
	trimmed := strings.TrimSpace(sessionID)
	if s == nil || s.validityCache == nil {
		item, err := s.ResolveActiveSession(ctx, trimmed)
		return activeSessionCandidateFromAuthoritative(trimmed, item), err
	}
	snapshot, err := s.validityCache.Resolve(ctx, trimmed, func(loadCtx context.Context) (*cachepolicy.ActiveSessionValiditySnapshot, bool, error) {
		item, loadErr := s.ResolveActiveSession(loadCtx, trimmed)
		if loadErr != nil || item == nil {
			return nil, false, loadErr
		}
		return &cachepolicy.ActiveSessionValiditySnapshot{UserID: item.UserID, ClientID: item.ClientID, ACR: item.ACR, AMR: append([]string(nil), item.AMR...), CreateTime: item.CreateTime, ExpiresAt: item.ExpiresAt, Active: item.Status == domain.SessionStatusActive}, item.Status == domain.SessionStatusActive && item.ExpiresAt.After(time.Now().UTC()), nil
	})
	if err != nil || snapshot == nil {
		return nil, err
	}
	if !snapshot.Active || snapshot.ExpiresAt.IsZero() || !snapshot.ExpiresAt.After(time.Now().UTC()) || strings.TrimSpace(snapshot.ClientID) == "" || snapshot.UserID <= 0 {
		return nil, nil
	}
	// The pre-existing watermark remains fail-closed even on a snapshot hit.
	if s.cache != nil {
		revokedAt, cacheErr := s.cache.UserRevokedAt(ctx, snapshot.UserID)
		if cacheErr != nil {
			return nil, cacheErr
		}
		if revokedAt != nil && (snapshot.CreateTime.IsZero() || revokedByUserWatermark(snapshot.CreateTime, *revokedAt)) {
			return nil, nil
		}
	}
	return &ActiveSessionCandidate{SessionID: trimmed, UserID: snapshot.UserID, ClientID: snapshot.ClientID, ACR: snapshot.ACR, AMR: append([]string(nil), snapshot.AMR...), ExpiresAt: snapshot.ExpiresAt}, nil
}

func activeSessionCandidateFromAuthoritative(sessionID string, item *domain.Session) *ActiveSessionCandidate {
	if item == nil {
		return nil
	}
	return &ActiveSessionCandidate{SessionID: strings.TrimSpace(sessionID), UserID: item.UserID, ClientID: item.ClientID, ACR: item.ACR, AMR: append([]string(nil), item.AMR...), ExpiresAt: item.ExpiresAt}
}

func revokedByUserWatermark(issuedAt, cutoff time.Time) bool {
	return !issuedAt.UTC().After(cutoff.UTC())
}

func (s *Service) ResolveActiveSessionRecord(ctx context.Context, sessionID string) (*ssofacade.SessionRecord, error) {
	item, err := s.ResolveActiveSession(ctx, sessionID)
	if err != nil || item == nil {
		return nil, err
	}
	record := toSessionRecords([]domain.Session{*item})
	if len(record) == 0 {
		return nil, nil
	}
	return &record[0], nil
}

func (s *Service) AuthorizeWithActiveSession(ctx context.Context, clientID, redirectURI, scope, state, nonce, codeChallenge, codeChallengeMethod string, session *ActiveSessionCandidate) (string, error) {
	client, err := s.requireClient(ctx, clientID)
	if err != nil {
		return "", err
	}
	if err := s.validateAuthorizeRequest(client, "code", redirectURI, strings.Fields(strings.TrimSpace(scope)), codeChallenge, codeChallengeMethod, ""); err != nil {
		return "", err
	}
	scopes, err := s.normalizeScopes(client, strings.Fields(strings.TrimSpace(scope)))
	if err != nil {
		return "", err
	}
	code := &domain.AuthorizationCode{
		Code:                s.newID("sso_code"),
		ClientID:            client.ClientID,
		UserID:              session.UserID,
		SessionID:           session.SessionID,
		RedirectURI:         redirectURI,
		Scopes:              scopes,
		CodeChallenge:       strings.TrimSpace(codeChallenge),
		CodeChallengeMethod: strings.ToUpper(strings.TrimSpace(codeChallengeMethod)),
		Nonce:               strings.TrimSpace(nonce),
		ACR:                 session.ACR,
		AMR:                 session.AMR,
		ExpiresAt:           time.Now().UTC().Add(5 * time.Minute),
		Status:              domain.CodeStatusActive,
	}
	if err := s.repository.InsertAuthorizationCode(ctx, code); err != nil {
		return "", err
	}
	snapshot := &domain.AuthorizationSessionSnapshot{RedirectURI: redirectURI, State: state}
	return s.buildCodeRedirect(snapshot, code.Code), nil
}

func (s *Service) BuildExpiredSessionCookie() string {
	return ssohelper.BuildExpiredSessionCookie(s.cfg.SessionCookie)
}

func (s *Service) BuildExpiredRefreshCookies() []string {
	return ssohelper.BuildExpiredRefreshCookies(s.cfg.RefreshCookie)
}

func (s *Service) BuildRefreshCookie(refreshToken string, expiresAt time.Time) string {
	return ssohelper.BuildRefreshCookie(s.cfg.RefreshCookie, strings.TrimSpace(refreshToken), expiresAt)
}

func (s *Service) CreateAuthorizationSession(ctx context.Context, request ssofacade.CreateAuthorizationSessionRequest) (*ssofacade.AuthorizationSessionSnapshot, error) {
	client, err := s.requireClient(ctx, request.ClientID)
	if err != nil {
		return nil, err
	}
	if err := s.validateAuthorizeRequest(
		client,
		request.ResponseType,
		request.RedirectURI,
		request.Scopes,
		request.CodeChallenge,
		request.CodeChallengeMethod,
		request.Prompt,
	); err != nil {
		return nil, err
	}
	scopes, err := s.normalizeScopes(client, request.Scopes)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(s.cfg.LoginTransactionTTLSeconds) * time.Second)
	snapshot := &domain.AuthorizationSessionSnapshot{
		LoginTransactionID:  s.newID("login_txn"),
		ClientID:            client.ClientID,
		RedirectURI:         strings.TrimSpace(request.RedirectURI),
		Scopes:              scopes,
		State:               strings.TrimSpace(request.State),
		Nonce:               strings.TrimSpace(request.Nonce),
		CodeChallenge:       strings.TrimSpace(request.CodeChallenge),
		CodeChallengeMethod: strings.ToUpper(strings.TrimSpace(request.CodeChallengeMethod)),
		CreatedAt:           &now,
		ExpiresAt:           &expiresAt,
	}
	if request.RequestContext != nil {
		snapshot.DeviceID = strings.TrimSpace(request.RequestContext.DeviceID)
		snapshot.TenantID = strings.TrimSpace(request.RequestContext.TenantID)
		snapshot.LoginIP = strings.TrimSpace(request.RequestContext.LoginIP)
		snapshot.UserAgent = strings.TrimSpace(request.RequestContext.UserAgent)
		snapshot.TraceID = strings.TrimSpace(request.RequestContext.TraceID)
	}
	if err := s.cache.SaveSession(ctx, snapshot, time.Until(expiresAt)); err != nil {
		return nil, err
	}
	return toFacadeAuthSession(snapshot), nil
}

func (s *Service) GetAuthorizationSession(ctx context.Context, loginTransactionID string) (*ssofacade.AuthorizationSessionSnapshot, error) {
	item, err := s.cache.GetSession(ctx, strings.TrimSpace(loginTransactionID))
	if err != nil || item == nil {
		return nil, err
	}
	if item.ExpiresAt != nil && item.ExpiresAt.Before(time.Now().UTC()) {
		_ = s.cache.RemoveSession(ctx, loginTransactionID)
		return nil, nil
	}
	return toFacadeAuthSession(item), nil
}

func (s *Service) RemoveAuthorizationSession(ctx context.Context, loginTransactionID string) error {
	return s.cache.RemoveSession(ctx, strings.TrimSpace(loginTransactionID))
}

func (s *Service) AcquireCompletionLock(ctx context.Context, loginTransactionID string) (bool, error) {
	return s.cache.AcquireCompletionLock(ctx, strings.TrimSpace(loginTransactionID))
}

func (s *Service) ReleaseCompletionLock(ctx context.Context, loginTransactionID string) error {
	return s.cache.ReleaseCompletionLock(ctx, strings.TrimSpace(loginTransactionID))
}

func (s *Service) MarkSessionFinalized(ctx context.Context, loginTransactionID string) (bool, error) {
	return s.cache.MarkSessionFinalized(ctx, strings.TrimSpace(loginTransactionID), time.Duration(s.cfg.LoginTransactionTTLSeconds)*time.Second)
}

func (s *Service) ReleaseSessionFinalized(ctx context.Context, loginTransactionID string) error {
	return s.cache.ReleaseSessionFinalized(ctx, strings.TrimSpace(loginTransactionID))
}

func (s *Service) CompleteInteractiveAuthentication(ctx context.Context, command ssofacade.CompleteInteractiveAuthenticationCommand) (*ssofacade.AuthenticationCompletionResult, error) {
	if command.UserID <= 0 {
		return nil, apperrors.Params("userId不能为空")
	}
	loginTransactionID := strings.TrimSpace(command.LoginTransactionID)
	if loginTransactionID == "" {
		return nil, apperrors.Params("loginTransactionId不能为空")
	}
	loginMethod, externalProviderCode, externalIdentityID, acr, amr, sourceErr := normalizeSessionSource(
		command.LoginMethod,
		command.ExternalProviderCode,
		command.ExternalIdentityID,
		command.ACR,
		command.AMR,
	)
	if sourceErr != nil {
		return nil, sourceErr
	}
	if locked, err := s.cache.AcquireCompletionLock(ctx, loginTransactionID); err != nil {
		return nil, err
	} else if !locked {
		return nil, apperrors.Operation("登录完成流程正在处理中")
	}
	defer func() { _ = s.cache.ReleaseCompletionLock(ctx, loginTransactionID) }()

	var cached ssofacade.AuthenticationCompletionResult
	if hit, err := s.cache.GetCompletionResult(ctx, loginTransactionID, &cached); err == nil && hit {
		return &cached, nil
	}
	snapshot, err := s.cache.GetSession(ctx, loginTransactionID)
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, apperrors.Operation("登录事务不存在或已失效")
	}
	if snapshot.ExpiresAt != nil && snapshot.ExpiresAt.Before(time.Now().UTC()) {
		_ = s.cache.RemoveSession(ctx, loginTransactionID)
		return nil, apperrors.Operation("登录事务不存在或已失效")
	}
	finalized, err := s.cache.MarkSessionFinalized(ctx, loginTransactionID, time.Until(*snapshot.ExpiresAt))
	if err != nil {
		return nil, err
	}
	if !finalized {
		if hit, err := s.cache.GetCompletionResult(ctx, loginTransactionID, &cached); err == nil && hit {
			return &cached, nil
		}
		return nil, apperrors.Operation("登录完成流程正在处理中")
	}

	now := time.Now().UTC()
	authTime := now
	if command.AuthTime != nil && !command.AuthTime.IsZero() {
		authTime = command.AuthTime.UTC()
	}
	session := &domain.Session{
		SessionID:            s.newID("sso_sess"),
		UserID:               command.UserID,
		ClientID:             snapshot.ClientID,
		PlatformCode:         strings.TrimSpace(command.PlatformCode),
		DeviceID:             strings.TrimSpace(snapshot.DeviceID),
		LoginIP:              strings.TrimSpace(snapshot.LoginIP),
		UserAgent:            strings.TrimSpace(snapshot.UserAgent),
		ACR:                  acr,
		AMR:                  amr,
		LoginMethod:          loginMethod,
		ExternalProviderCode: externalProviderCode,
		ExternalIdentityID:   externalIdentityID,
		LoginAt:              authTime,
		ExpiresAt:            now.Add(time.Duration(maxInt(s.cfg.SessionIdleTimeoutSeconds, 1800)) * time.Second),
		Status:               domain.SessionStatusActive,
	}
	if err := s.repository.InsertSession(ctx, session); err != nil {
		_ = s.cache.ReleaseSessionFinalized(ctx, loginTransactionID)
		return nil, err
	}
	if client, err := s.requireClient(ctx, snapshot.ClientID); err == nil && client != nil {
		if len(snapshot.Scopes) == 0 {
			snapshot.Scopes = client.Scopes
		}
		if client.RequireConsent || client.TrustedFirstParty {
			_ = s.repository.UpsertConsentGrant(ctx, &domain.ConsentGrant{
				UserID:    command.UserID,
				ClientID:  client.ClientID,
				Scopes:    snapshot.Scopes,
				GrantedAt: now,
				Status:    domain.ConsentStatusActive,
			})
		}
		code := &domain.AuthorizationCode{
			Code:                s.newID("sso_code"),
			ClientID:            snapshot.ClientID,
			UserID:              command.UserID,
			SessionID:           session.SessionID,
			RedirectURI:         snapshot.RedirectURI,
			Scopes:              snapshot.Scopes,
			CodeChallenge:       snapshot.CodeChallenge,
			CodeChallengeMethod: snapshot.CodeChallengeMethod,
			Nonce:               snapshot.Nonce,
			ACR:                 session.ACR,
			AMR:                 session.AMR,
			ExpiresAt:           now.Add(5 * time.Minute),
			Status:              domain.CodeStatusActive,
		}
		if err := s.repository.InsertAuthorizationCode(ctx, code); err != nil {
			_ = s.cache.ReleaseSessionFinalized(ctx, loginTransactionID)
			return nil, err
		}
		result := &ssofacade.AuthenticationCompletionResult{
			Authenticated:            true,
			LoginTransactionID:       loginTransactionID,
			RedirectURL:              s.buildCodeRedirect(snapshot, code.Code),
			SessionCookieHeaderValue: ssohelper.BuildSessionCookie(s.cfg.SessionCookie, session.SessionID, session.ExpiresAt),
		}
		_ = s.cache.SaveCompletionResult(ctx, loginTransactionID, result, time.Until(*snapshot.ExpiresAt))
		_ = s.cache.RemoveSession(ctx, loginTransactionID)
		traceID := strings.TrimSpace(snapshot.TraceID)
		if traceID == "" && command.RequestContext != nil {
			traceID = strings.TrimSpace(command.RequestContext.TraceID)
		}
		s.insertSSOAuditLog(ctx, domain.AuditLog{
			EventType: "INTERACTIVE_LOGIN_COMPLETED",
			ClientID:  snapshot.ClientID,
			UserID:    &command.UserID,
			SessionID: session.SessionID,
			DeviceID:  session.DeviceID,
			TenantID:  snapshot.TenantID,
			LoginIP:   session.LoginIP,
			UserAgent: session.UserAgent,
			Result:    "SUCCESS",
			TraceID:   traceID,
		})
		return result, nil
	}
	_ = s.cache.ReleaseSessionFinalized(ctx, loginTransactionID)
	return nil, apperrors.Operation("客户端不存在或已禁用")
}

func (s *Service) BootstrapFirstPartySession(ctx context.Context, command ssofacade.BootstrapSessionCommand) (*ssofacade.BootstrapSessionResult, error) {
	if command.UserID <= 0 {
		return nil, apperrors.Params("userId不能为空")
	}
	clientID := strings.TrimSpace(command.ClientID)
	if clientID == "" {
		clientID = strings.TrimSpace(s.cfg.DefaultFirstPartyClientID)
	}
	client, err := s.requireClient(ctx, clientID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	loginMethod, externalProviderCode, externalIdentityID, acr, amr, sourceErr := normalizeSessionSource(
		command.LoginMethod,
		command.ExternalProviderCode,
		command.ExternalIdentityID,
		command.ACR,
		command.AMR,
	)
	if sourceErr != nil {
		return nil, sourceErr
	}
	session := &domain.Session{
		SessionID:            s.newID("sso_sess"),
		UserID:               command.UserID,
		ClientID:             client.ClientID,
		PlatformCode:         strings.TrimSpace(command.PlatformCode),
		LoginAt:              now,
		ExpiresAt:            now.Add(time.Duration(maxInt(s.cfg.SessionIdleTimeoutSeconds, 1800)) * time.Second),
		Status:               domain.SessionStatusActive,
		ACR:                  acr,
		AMR:                  amr,
		LoginMethod:          loginMethod,
		ExternalProviderCode: externalProviderCode,
		ExternalIdentityID:   externalIdentityID,
	}
	if command.RequestContext != nil {
		session.DeviceID = strings.TrimSpace(command.RequestContext.DeviceID)
		session.LoginIP = strings.TrimSpace(command.RequestContext.LoginIP)
		session.UserAgent = strings.TrimSpace(command.RequestContext.UserAgent)
	}
	if err := s.repository.InsertSession(ctx, session); err != nil {
		return nil, err
	}
	refreshToken := s.newID("sso_rt")
	refreshFamily := &domain.RefreshTokenFamily{
		FamilyID:         s.newID("sso_rtf"),
		SessionID:        session.SessionID,
		ClientID:         client.ClientID,
		UserID:           session.UserID,
		CurrentTokenHash: ssohelper.BuildTokenHash(refreshToken),
		ExpiresAt:        now.Add(time.Duration(maxInt(client.RefreshTokenTTLSec, 2592000)) * time.Second),
		Status:           domain.RefreshFamilyStatusActive,
		MetadataJSON:     metadataJSON(map[string]any{"scopes": []string{"openid", "profile", "email", "offline_access"}}),
	}
	if err := s.repository.InsertRefreshTokenFamily(ctx, refreshFamily); err != nil {
		return nil, err
	}
	accessToken, accessExpiresAt, err := s.issueAccessToken(ctx, client, session, []string{"openid", "profile", "email", "offline_access"})
	if err != nil {
		return nil, err
	}
	return &ssofacade.BootstrapSessionResult{
		AccessToken:              accessToken,
		TokenType:                "Bearer",
		AccessTTLSeconds:         int64(time.Until(accessExpiresAt).Seconds()),
		SessionCookieHeaderValue: ssohelper.BuildSessionCookie(s.cfg.SessionCookie, session.SessionID, session.ExpiresAt),
		RefreshCookieHeaderValue: ssohelper.BuildRefreshCookie(s.cfg.RefreshCookie, refreshToken, refreshFamily.ExpiresAt),
	}, nil
}

func (s *Service) ListSessionsByUserID(ctx context.Context, userID int64) ([]ssofacade.SessionRecord, error) {
	items, err := s.repository.ListSessionsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return toSessionRecords(items), nil
}

func (s *Service) ListSessionsByUserIDPage(ctx context.Context, userID int64, offset, limit int) ([]ssofacade.SessionRecord, error) {
	items, err := s.repository.ListSessionsByUserIDPage(ctx, userID, offset, limit)
	if err != nil {
		return nil, err
	}
	return toSessionRecords(items), nil
}

func (s *Service) CountSessionsByUserID(ctx context.Context, userID int64) (int64, error) {
	return s.repository.CountSessionsByUserID(ctx, userID)
}

func (s *Service) ListActiveSessions(ctx context.Context) ([]ssofacade.SessionRecord, error) {
	items, err := s.repository.ListActiveSessions(ctx)
	if err != nil {
		return nil, err
	}
	return toSessionRecords(items), nil
}

func (s *Service) CountActiveSessions(ctx context.Context) (int64, error) {
	return s.repository.CountActiveSessions(ctx)
}

func (s *Service) ListEventsSince(ctx context.Context, startTime time.Time) ([]ssofacade.AuditEventRecord, error) {
	items, err := s.repository.ListAuditEventsSince(ctx, startTime)
	if err != nil {
		return nil, err
	}
	result := make([]ssofacade.AuditEventRecord, 0, len(items))
	for _, item := range items {
		createdAt := item.CreatedAt
		result = append(result, ssofacade.AuditEventRecord{
			ID:         item.ID,
			EventType:  item.EventType,
			ClientID:   item.ClientID,
			UserID:     item.UserID,
			SessionID:  item.SessionID,
			DeviceID:   item.DeviceID,
			TenantID:   item.TenantID,
			LoginIP:    item.LoginIP,
			UserAgent:  item.UserAgent,
			Result:     item.Result,
			ReasonCode: item.ReasonCode,
			DetailJSON: item.DetailJSON,
			TraceID:    item.TraceID,
			CreatedAt:  &createdAt,
		})
	}
	return result, nil
}

func (s *Service) ListEnabledClients(ctx context.Context) ([]ssofacade.ClientRecord, error) {
	items, err := s.repository.ListEnabledClients(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]ssofacade.ClientRecord, 0, len(items))
	for _, item := range items {
		result = append(result, ssofacade.ClientRecord{
			ID:                 item.ID,
			ClientID:           item.ClientID,
			ClientName:         item.ClientName,
			ClientType:         item.ClientType,
			ClientAuthMethod:   item.ClientAuthMethod,
			GrantTypes:         append([]string(nil), item.GrantTypes...),
			Scopes:             append([]string(nil), item.Scopes...),
			RequirePKCE:        item.RequirePKCE,
			RequireConsent:     item.RequireConsent,
			TrustedFirstParty:  item.TrustedFirstParty,
			AccessTokenTTLSec:  item.AccessTokenTTLSec,
			RefreshTokenTTLSec: item.RefreshTokenTTLSec,
			Status:             item.Status,
			MetadataJSON:       item.MetadataJSON,
		})
	}
	return result, nil
}

func (s *Service) ClientCapabilities(ctx context.Context) map[string]any {
	return map[string]any{
		"clientTypes":       []string{"PUBLIC", "CONFIDENTIAL"},
		"clientAuthMethods": []string{"none", "client_secret_basic"},
		"grantTypes":        []string{"authorization_code", "refresh_token"},
		"scopes": []map[string]any{
			{"name": "openid", "required": true},
			{"name": "profile", "required": false},
			{"name": "email", "required": false},
			{"name": "offline_access", "required": false},
		},
		"codeChallengeMethods": []string{"S256"},
		"signingAlgorithms":    []string{"RS256"},
	}
}

func (s *Service) ListClients(ctx context.Context, query ssofacade.ClientAdminQuery) (*ssofacade.ClientAdminPage, error) {
	items, total, err := s.repository.ListClients(ctx, query)
	if err != nil {
		return nil, err
	}
	current, pageSize := normalizeClientAdminPage(query.Current, query.PageSize)
	return &ssofacade.ClientAdminPage{
		Records:  toClientAdminRecords(items),
		Total:    total,
		Current:  current,
		PageSize: pageSize,
	}, nil
}

func (s *Service) GetClient(ctx context.Context, clientID string) (*ssofacade.ClientAdminDetail, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return nil, apperrors.Params("clientId不能为空")
	}
	item, err := s.repository.FindClientDetail(ctx, clientID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, apperrors.NotFound("SSO客户端不存在")
	}
	records := toClientAdminRecords([]domain.Client{*item})
	if len(records) == 0 {
		return nil, apperrors.NotFound("SSO客户端不存在")
	}
	redirectItems, err := s.repository.ListClientRedirectURIs(ctx, clientID)
	if err != nil {
		return nil, err
	}
	secretItems, err := s.repository.ListClientSecrets(ctx, clientID)
	if err != nil {
		return nil, err
	}
	return &ssofacade.ClientAdminDetail{
		ClientAdminRecord: records[0],
		RedirectURIs:      toClientRedirectURIRecords(redirectItems),
		Secrets:           toClientSecretSummaryRecords(secretItems),
	}, nil
}

func (s *Service) ListClientRedirectURIs(ctx context.Context, clientID string) ([]ssofacade.ClientRedirectURIRecord, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return nil, apperrors.Params("clientId不能为空")
	}
	item, err := s.repository.FindClientDetail(ctx, clientID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, apperrors.NotFound("SSO客户端不存在")
	}
	redirects, err := s.repository.ListClientRedirectURIs(ctx, clientID)
	if err != nil {
		return nil, err
	}
	return toClientRedirectURIRecords(redirects), nil
}

func (s *Service) ListClientSecrets(ctx context.Context, clientID string) ([]ssofacade.ClientSecretSummaryRecord, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return nil, apperrors.Params("clientId不能为空")
	}
	item, err := s.repository.FindClientDetail(ctx, clientID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, apperrors.NotFound("SSO客户端不存在")
	}
	secrets, err := s.repository.ListClientSecrets(ctx, clientID)
	if err != nil {
		return nil, err
	}
	return toClientSecretSummaryRecords(secrets), nil
}

func (s *Service) CreateClient(ctx context.Context, actorID int64, request ssofacade.ClientAdminSaveRequest, proof stepup.ProofMetadata) (*ssofacade.ClientAdminDetail, error) {
	item, err := normalizeClientAdminSaveRequest("", request)
	if err != nil {
		return nil, err
	}
	if err := guardGenericClientMutation(item); err != nil {
		return nil, err
	}
	binding, err := BuildClientAdminCreateOperationBinding(request)
	if err != nil {
		return nil, err
	}
	if err := stepup.Require(proof, string(challengedomain.BusinessActionSSOClientCreate), binding); err != nil {
		return nil, err
	}
	item.Status = domain.ClientStatusActive
	if err := s.repository.InsertClient(ctx, item, actorID); err != nil {
		return nil, err
	}
	return s.GetClient(ctx, item.ClientID)
}

func (s *Service) UpdateClient(ctx context.Context, actorID int64, clientID string, request ssofacade.UpdateClientAdminRequest, proof stepup.ProofMetadata) (*ssofacade.ClientAdminDetail, error) {
	item, err := normalizeClientAdminUpdateRequest(clientID, request)
	if err != nil {
		return nil, err
	}
	binding, err := BuildClientAdminUpdateOperationBinding(clientID, request)
	if err != nil {
		return nil, err
	}
	if err := stepup.Require(proof, string(challengedomain.BusinessActionSSOClientUpdate), binding); err != nil {
		return nil, err
	}
	existing, err := s.repository.FindClientDetail(ctx, item.ClientID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, apperrors.NotFound("SSO客户端不存在")
	}
	if err := guardGenericClientMutation(existing); err != nil {
		return nil, err
	}
	item.Status = existing.Status
	if err := s.repository.UpdateClient(ctx, item, actorID); err != nil {
		return nil, err
	}
	return s.GetClient(ctx, item.ClientID)
}

func (s *Service) UpdateClientStatus(ctx context.Context, actorID int64, clientID string, request ssofacade.ClientStatusRequest, proof stepup.ProofMetadata) error {
	if s.validityInvalidations != nil && s.validityInvalidations.Enabled() && store.SQLXFromContext(ctx) == nil {
		return s.requireActiveSessionValidityTransaction(ctx, func(txCtx context.Context) error {
			return s.UpdateClientStatus(txCtx, actorID, clientID, request, proof)
		})
	}
	clientID = strings.TrimSpace(clientID)
	if err := validateClientID(clientID); err != nil {
		return err
	}
	if request.Status != domain.ClientStatusActive && request.Status != domain.ClientStatusDisabled {
		return apperrors.Params("客户端状态无效")
	}
	binding, err := BuildClientStatusOperationBinding(clientID, request)
	if err != nil {
		return err
	}
	if err := stepup.Require(proof, string(challengedomain.BusinessActionSSOClientStatusChange), binding); err != nil {
		return err
	}
	existing, err := s.repository.FindClientDetail(ctx, clientID)
	if err != nil {
		return err
	}
	if existing == nil {
		return apperrors.NotFound("SSO客户端不存在")
	}
	if err := guardGenericClientMutation(existing); err != nil {
		return err
	}
	// A disabled OAuth client must not leave an active-session candidate
	// usable. The former optional best-effort flag is incompatible with the
	// DG6.2 session-validity authority contract.
	revokeSessions := request.Status == domain.ClientStatusDisabled
	now := time.Now().UTC()
	if request.Status == domain.ClientStatusDisabled && revokeSessions {
		now, err = s.captureSessionRevocationCutoff(ctx)
		if err != nil {
			return err
		}
		if _, err := s.collectSessionRevocationEffects(ctx, func(afterID int64, limit int) ([]domain.Session, error) {
			return s.repository.ListActiveSessionsByClientIDPage(ctx, clientID, now, afterID, limit)
		}); err != nil {
			return err
		}
	}
	_, err = s.repository.UpdateClientStatus(ctx, clientID, request.Status, actorID, now)
	if err != nil {
		return err
	}
	// A repeated disable must still contain any active session left by a prior
	// interrupted/legacy operation. Session validity is not conditional on the
	// client-row transition reporting changed rows.
	if request.Status == domain.ClientStatusDisabled && revokeSessions {
		if _, err := s.repository.RevokeSessionsByClientID(ctx, clientID, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) UpdateClientRedirectURIs(ctx context.Context, actorID int64, clientID string, request ssofacade.ClientRedirectURIUpdateRequest, proof stepup.ProofMetadata) ([]ssofacade.ClientRedirectURIRecord, error) {
	clientID = strings.TrimSpace(clientID)
	if err := validateClientID(clientID); err != nil {
		return nil, err
	}
	binding, err := BuildClientRedirectURIsOperationBinding(clientID, request)
	if err != nil {
		return nil, err
	}
	if err := stepup.Require(proof, string(challengedomain.BusinessActionSSOClientRedirectEdit), binding); err != nil {
		return nil, err
	}
	existing, err := s.repository.FindClientDetail(ctx, clientID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, apperrors.NotFound("SSO客户端不存在")
	}
	if err := guardGenericClientMutation(existing); err != nil {
		return nil, err
	}
	redirects, err := normalizeClientRedirectURIsForAdmin(request, s.redirectValidationProfile())
	if err != nil {
		return nil, err
	}
	if s.transactor == nil || !s.transactor.Enabled() {
		return nil, apperrors.System("SSO客户端回调地址事务未配置")
	}
	now := time.Now().UTC()
	if err := s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return s.repository.ReplaceClientRedirectURIs(txCtx, clientID, redirects, actorID, now)
	}); err != nil {
		return nil, err
	}
	return s.ListClientRedirectURIs(ctx, clientID)
}

func (s *Service) GenerateClientSecret(ctx context.Context, actorID int64, clientID string, request ssofacade.ClientSecretGenerateRequest, proof stepup.ProofMetadata) (*ssofacade.ClientSecretGenerateResponse, error) {
	clientID = strings.TrimSpace(clientID)
	binding, err := BuildClientSecretGenerateOperationBinding(clientID, request)
	if err != nil {
		return nil, err
	}
	if err := stepup.Require(proof, string(challengedomain.BusinessActionSSOClientSecretGenerate), binding); err != nil {
		return nil, err
	}
	client, err := s.repository.FindClientDetail(ctx, clientID)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, apperrors.NotFound("SSO客户端不存在")
	}
	if err := guardGenericClientMutation(client); err != nil {
		return nil, err
	}
	if client.Status != domain.ClientStatusActive {
		return nil, apperrors.Unauthorized("SSO客户端不可用")
	}
	if strings.ToUpper(strings.TrimSpace(client.ClientType)) != "CONFIDENTIAL" || strings.TrimSpace(client.ClientAuthMethod) == "none" {
		return nil, apperrors.Params("仅机密客户端可生成密钥")
	}
	if s.password == nil {
		return nil, apperrors.System("密码哈希服务未配置")
	}
	secret, err := generateClientSecretPlaintext()
	if err != nil {
		return nil, err
	}
	hash, err := s.password.Hash(ctx, secret)
	if err != nil {
		return nil, apperrors.System("SSO客户端密钥哈希失败")
	}
	var expiresAt *time.Time
	if request.ExpiresInDays > 0 {
		value := time.Now().UTC().Add(time.Duration(request.ExpiresInDays) * 24 * time.Hour)
		expiresAt = &value
	}
	secretID := s.nextID()
	hint := clientSecretHint(secret)
	if err := s.repository.InsertClientSecret(ctx, &domain.ClientSecret{
		ID:         secretID,
		ClientID:   clientID,
		SecretHash: hash,
		SecretHint: hint,
		ExpiresAt:  expiresAt,
		Status:     domain.ClientStatusActive,
	}, actorID); err != nil {
		return nil, err
	}
	return &ssofacade.ClientSecretGenerateResponse{
		SecretID:     secretID,
		ClientSecret: secret,
		SecretHint:   hint,
		ExpiresAt:    expiresAt,
	}, nil
}

// UpsertManagedClient provisions the fixed confidential OIDC profile without fabricating interactive proof.
func (s *Service) UpsertManagedClient(ctx context.Context, command ssofacade.ManagedClientCommand) (*ssofacade.ManagedClientResult, error) {
	owner := strings.TrimSpace(command.OwnerNodeCode)
	if !managedClientOwnerPattern.MatchString(owner) {
		return nil, apperrors.Params("系统托管SSO客户端ownerNodeCode格式无效")
	}
	metadata, err := sonic.MarshalString(managedClientMetadata{ManagedBy: "hub_control", OwnerNodeCode: owner})
	if err != nil {
		return nil, apperrors.System("系统托管SSO客户端元数据生成失败")
	}
	request := ssofacade.ClientAdminSaveRequest{
		ClientID: command.ClientID, ClientName: command.ClientName,
		ClientType: "CONFIDENTIAL", ClientAuthMethod: "client_secret_basic",
		GrantTypes: []string{"authorization_code", "refresh_token"},
		Scopes:     []string{"openid", "profile", "email"}, RequirePKCE: true,
		AccessTokenTTLSec: 1800, RefreshTokenTTLSec: 2592000,
		MetadataJSON: metadata,
	}
	item, err := normalizeClientAdminSaveRequest("", request)
	if err != nil {
		return nil, err
	}
	redirects, err := normalizeClientRedirectURIsForAdmin(ssofacade.ClientRedirectURIUpdateRequest{RedirectURIs: []string{command.RedirectURI}}, s.redirectValidationProfile())
	if err != nil {
		return nil, err
	}
	if s.transactor == nil || !s.transactor.Enabled() {
		return nil, apperrors.System("SSO系统托管客户端事务未配置")
	}
	result := &ssofacade.ManagedClientResult{ClientID: item.ClientID}
	err = s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		existing, findErr := s.repository.FindClientDetailForUpdate(txCtx, item.ClientID)
		if findErr != nil {
			return findErr
		}
		if existing == nil {
			item.Status = domain.ClientStatusActive
			if insertErr := s.repository.InsertClient(txCtx, item, 0); insertErr != nil {
				return insertErr
			}
		} else {
			var ownership managedClientMetadata
			if parseErr := sonic.UnmarshalString(existing.MetadataJSON, &ownership); parseErr != nil || ownership.ManagedBy != "hub_control" || ownership.OwnerNodeCode != owner {
				return apperrors.ObjectState("SSO客户端已存在且不属于当前Node")
			}
			if updateErr := s.repository.UpdateClient(txCtx, item, 0); updateErr != nil {
				return updateErr
			}
			if existing.Status != domain.ClientStatusActive {
				if _, statusErr := s.repository.UpdateClientStatus(txCtx, item.ClientID, domain.ClientStatusActive, 0, time.Now().UTC()); statusErr != nil {
					return statusErr
				}
			}
		}
		if replaceErr := s.repository.ReplaceClientRedirectURIs(txCtx, item.ClientID, redirects, 0, time.Now().UTC()); replaceErr != nil {
			return replaceErr
		}
		activeCount, countErr := s.repository.CountActiveClientSecrets(txCtx, item.ClientID)
		if countErr != nil {
			return countErr
		}
		if activeCount > 0 && !command.RotateSecret {
			return nil
		}
		secret, generateErr := generateClientSecretPlaintext()
		if generateErr != nil {
			return generateErr
		}
		hash, hashErr := s.password.Hash(txCtx, secret)
		if hashErr != nil {
			return apperrors.System("SSO客户端密钥哈希失败")
		}
		secretID := s.nextID()
		if insertErr := s.repository.InsertClientSecret(txCtx, &domain.ClientSecret{
			ID: secretID, ClientID: item.ClientID, SecretHash: hash,
			SecretHint: clientSecretHint(secret), Status: domain.ClientStatusActive,
		}, 0); insertErr != nil {
			return insertErr
		}
		if command.RotateSecret {
			if _, disableErr := s.repository.DisableOtherActiveClientSecrets(txCtx, item.ClientID, secretID, 0, time.Now().UTC()); disableErr != nil {
				return disableErr
			}
		}
		result.ClientSecret = secret
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// SetManagedClientStatus changes an exact-owner managed client. Disabling the
// client revokes each active session in the same transaction; DG6.2 registers
// every target before the fact update and never relies on audit sampling.
func (s *Service) SetManagedClientStatus(ctx context.Context, command ssofacade.ManagedClientStatusCommand) error {
	clientID := strings.TrimSpace(command.ClientID)
	owner := strings.TrimSpace(command.OwnerNodeCode)
	if err := validateClientID(clientID); err != nil {
		return err
	}
	if !managedClientOwnerPattern.MatchString(owner) {
		return apperrors.Params("系统托管SSO客户端ownerNodeCode格式无效")
	}
	if command.Status != domain.ClientStatusActive && command.Status != domain.ClientStatusDisabled {
		return apperrors.Params("系统托管SSO客户端状态无效")
	}
	if s.transactor == nil || !s.transactor.Enabled() {
		return apperrors.System("SSO系统托管客户端事务未配置")
	}
	return s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		existing, err := s.repository.FindClientDetailForUpdate(txCtx, clientID)
		if err != nil || existing == nil {
			return err
		}
		var ownership managedClientMetadata
		if err := sonic.UnmarshalString(existing.MetadataJSON, &ownership); err != nil || ownership.ManagedBy != "hub_control" || ownership.OwnerNodeCode != owner {
			return apperrors.ObjectState("SSO客户端已存在且不属于当前Node")
		}
		now := time.Now().UTC()
		if command.Status == domain.ClientStatusDisabled && s.validityInvalidations != nil && s.validityInvalidations.Enabled() {
			cutoff, cutoffErr := s.captureSessionRevocationCutoff(txCtx)
			if cutoffErr != nil {
				return cutoffErr
			}
			now = cutoff
			if _, collectErr := s.collectSessionRevocationEffects(txCtx, func(afterID int64, limit int) ([]domain.Session, error) {
				return s.repository.ListActiveSessionsByClientIDPage(txCtx, clientID, now, afterID, limit)
			}); collectErr != nil {
				return collectErr
			}
		}
		if _, err = s.repository.UpdateClientStatus(txCtx, clientID, command.Status, 0, now); err != nil {
			return err
		}
		if command.Status == domain.ClientStatusDisabled {
			_, err = s.repository.RevokeSessionsByClientID(txCtx, clientID, now)
		}
		return err
	})
}

func (s *Service) DisableClientSecret(ctx context.Context, actorID int64, clientID string, secretID int64, request ssofacade.ClientSecretStatusRequest, proof stepup.ProofMetadata) error {
	clientID = strings.TrimSpace(clientID)
	binding, err := BuildClientSecretStatusOperationBinding(clientID, secretID, request)
	if err != nil {
		return err
	}
	if err := stepup.Require(proof, string(challengedomain.BusinessActionSSOClientSecretDisable), binding); err != nil {
		return err
	}
	client, err := s.repository.FindClientDetail(ctx, clientID)
	if err != nil {
		return err
	}
	if client == nil {
		return apperrors.NotFound("SSO客户端不存在")
	}
	if err := guardGenericClientMutation(client); err != nil {
		return err
	}
	secrets, err := s.repository.ListClientSecrets(ctx, clientID)
	if err != nil {
		return err
	}
	target, found := findClientSecretSummary(secrets, secretID)
	if !found {
		return apperrors.NotFound("SSO客户端密钥不存在")
	}
	if target.Status == domain.ClientStatusDisabled {
		return nil
	}
	if strings.ToUpper(strings.TrimSpace(client.ClientType)) == "CONFIDENTIAL" && client.Status == domain.ClientStatusActive && !request.AllowNoActiveSecret {
		activeCount, countErr := s.repository.CountActiveClientSecrets(ctx, clientID)
		if countErr != nil {
			return countErr
		}
		if activeCount <= 1 {
			return apperrors.Params("不能禁用最后一个有效客户端密钥")
		}
	}
	_, err = s.repository.UpdateClientSecretStatus(ctx, clientID, secretID, domain.ClientStatusDisabled, actorID, time.Now().UTC())
	return err
}

func guardGenericClientMutation(client *domain.Client) error {
	if client == nil {
		return nil
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(client.ClientID)), "hub-node-") {
		return apperrors.Forbidden("系统托管SSO客户端禁止后台修改")
	}
	var metadata managedClientMetadata
	if err := sonic.UnmarshalString(strings.TrimSpace(client.MetadataJSON), &metadata); err == nil && metadata.ManagedBy == "hub_control" {
		return apperrors.Forbidden("系统托管SSO客户端禁止后台修改")
	}
	return nil
}

func (s *Service) RevokeSession(ctx context.Context, sessionID string) (bool, error) {
	if s.validityInvalidations != nil && s.validityInvalidations.Enabled() && store.SQLXFromContext(ctx) == nil {
		var revoked bool
		err := s.requireActiveSessionValidityTransaction(ctx, func(txCtx context.Context) error {
			var callErr error
			revoked, callErr = s.RevokeSession(txCtx, sessionID)
			return callErr
		})
		return revoked, err
	}
	item, err := s.repository.FindSessionBySessionID(ctx, strings.TrimSpace(sessionID))
	if err != nil || item == nil {
		return false, err
	}
	revoked, err := s.revokeSessionAndRefreshFamiliesWithResult(ctx, item.SessionID)
	if err != nil {
		return false, err
	}
	s.auditSessionRevoked(ctx, item, revoked)
	return revoked, nil
}

func (s *Service) RevokeManagedSession(ctx context.Context, sessionID string) (bool, error) {
	if s.validityInvalidations == nil || !s.validityInvalidations.Enabled() {
		return s.revokeSessionAndRefreshFamiliesWithResult(ctx, strings.TrimSpace(sessionID))
	}
	return s.RevokeSession(ctx, sessionID)
}

// CaptureManagedSessionCutoff returns the MySQL acceptance boundary for managed commands.
func (s *Service) CaptureManagedSessionCutoff(ctx context.Context) (time.Time, error) {
	cutoff, err := s.repository.CaptureManagedSessionCutoff(ctx)
	if err != nil {
		return time.Time{}, err
	}
	if cutoff.IsZero() {
		return time.Time{}, apperrors.ServiceUnavailable("用户会话撤销截止条件不可用")
	}
	return cutoff.UTC(), nil
}

func (s *Service) RevokeSessionsByUserID(ctx context.Context, userID int64) (int64, error) {
	if s.validityInvalidations != nil && s.validityInvalidations.Enabled() && store.SQLXFromContext(ctx) == nil {
		var count int64
		err := s.requireActiveSessionValidityTransaction(ctx, func(txCtx context.Context) error {
			var callErr error
			count, callErr = s.RevokeSessionsByUserID(txCtx, userID)
			return callErr
		})
		return count, err
	}
	now, err := s.captureSessionRevocationCutoff(ctx)
	if err != nil {
		return 0, err
	}
	sessionsBefore, err := s.collectSessionRevocationEffects(ctx, func(afterID int64, limit int) ([]domain.Session, error) {
		return s.repository.ListActiveSessionsByUserIDPage(ctx, userID, now, afterID, limit)
	})
	if err != nil {
		return 0, err
	}
	if err := s.repository.RevokeRefreshFamiliesByUserID(ctx, userID, now); err != nil {
		return 0, err
	}
	revokedCount, err := s.repository.RevokeSessionsByUserID(ctx, userID, now)
	if err != nil {
		return 0, err
	}
	s.auditUserSessionsRevoked(ctx, userID, sessionsBefore, revokedCount)
	return revokedCount, s.cache.MarkUserRevoked(ctx, userID, now)
}

func (s *Service) RevokeSessionsByUserIDAtOrBefore(ctx context.Context, userID int64, cutoff time.Time) (int64, error) {
	if s.validityInvalidations != nil && s.validityInvalidations.Enabled() && store.SQLXFromContext(ctx) == nil {
		var count int64
		err := s.requireActiveSessionValidityTransaction(ctx, func(txCtx context.Context) error {
			var callErr error
			count, callErr = s.RevokeSessionsByUserIDAtOrBefore(txCtx, userID, cutoff)
			return callErr
		})
		return count, err
	}
	if userID <= 0 || cutoff.IsZero() {
		return 0, apperrors.Params("用户会话撤销截止条件无效")
	}
	cutoff = cutoff.UTC()
	if s.validityInvalidations != nil && s.validityInvalidations.Enabled() {
		if _, err := s.collectSessionRevocationEffects(ctx, func(afterID int64, limit int) ([]domain.Session, error) {
			return s.repository.ListActiveSessionsByUserIDPage(ctx, userID, cutoff, afterID, limit)
		}); err != nil {
			return 0, err
		}
	}
	if err := s.repository.RevokeRefreshFamiliesByUserIDAtOrBefore(ctx, userID, cutoff); err != nil {
		return 0, err
	}
	revokedCount, err := s.repository.RevokeSessionsByUserIDAtOrBefore(ctx, userID, cutoff)
	if err != nil {
		return 0, err
	}
	return revokedCount, s.cache.MarkUserRevoked(ctx, userID, cutoff)
}

func (s *Service) RevokeSessionsByPlatformCode(ctx context.Context, platformCode string) (int64, error) {
	if s.validityInvalidations != nil && s.validityInvalidations.Enabled() && store.SQLXFromContext(ctx) == nil {
		var count int64
		err := s.requireActiveSessionValidityTransaction(ctx, func(txCtx context.Context) error {
			var callErr error
			count, callErr = s.RevokeSessionsByPlatformCode(txCtx, platformCode)
			return callErr
		})
		return count, err
	}
	platformCode = strings.TrimSpace(platformCode)
	if platformCode == "" {
		return 0, apperrors.Params("platformCode不能为空")
	}
	now, err := s.captureSessionRevocationCutoff(ctx)
	if err != nil {
		return 0, err
	}
	sessionsBefore, err := s.collectSessionRevocationEffects(ctx, func(afterID int64, limit int) ([]domain.Session, error) {
		return s.repository.ListActiveSessionsByPlatformCodePage(ctx, platformCode, now, afterID, limit)
	})
	if err != nil {
		return 0, err
	}
	if err := s.repository.RevokeRefreshFamiliesByPlatformCode(ctx, platformCode, now); err != nil {
		return 0, err
	}
	revokedCount, err := s.repository.RevokeSessionsByPlatformCode(ctx, platformCode, now)
	if err != nil {
		return 0, err
	}
	s.auditPlatformSessionsRevoked(ctx, platformCode, sessionsBefore, revokedCount)
	return revokedCount, nil
}

func (s *Service) RevokeSessionsByPlatformLoginMethod(ctx context.Context, platformCode string, loginMethod string, externalProviderCode string) (int64, error) {
	if s.validityInvalidations != nil && s.validityInvalidations.Enabled() && store.SQLXFromContext(ctx) == nil {
		var count int64
		err := s.requireActiveSessionValidityTransaction(ctx, func(txCtx context.Context) error {
			var callErr error
			count, callErr = s.RevokeSessionsByPlatformLoginMethod(txCtx, platformCode, loginMethod, externalProviderCode)
			return callErr
		})
		return count, err
	}
	platformCode = strings.TrimSpace(platformCode)
	if platformCode == "" {
		return 0, apperrors.Params("platformCode不能为空")
	}
	loginMethod = strings.ToUpper(strings.TrimSpace(loginMethod))
	if loginMethod == "" {
		return 0, apperrors.Params("loginMethod不能为空")
	}
	providerCode := strings.TrimSpace(externalProviderCode)
	if providerCode != "" {
		canonicalProvider, err := canonicalExternalProviderCode(providerCode)
		if err != nil {
			return 0, err
		}
		providerCode = canonicalProvider
	}
	now, err := s.captureSessionRevocationCutoff(ctx)
	if err != nil {
		return 0, err
	}
	sessionsBefore, err := s.collectSessionRevocationEffects(ctx, func(afterID int64, limit int) ([]domain.Session, error) {
		return s.repository.ListActiveSessionsByPlatformLoginMethodPage(ctx, platformCode, loginMethod, providerCode, now, afterID, limit)
	})
	if err != nil {
		return 0, err
	}
	if err := s.repository.RevokeRefreshFamiliesByPlatformLoginMethod(ctx, platformCode, loginMethod, providerCode, now); err != nil {
		return 0, err
	}
	revokedCount, err := s.repository.RevokeSessionsByPlatformLoginMethod(ctx, platformCode, loginMethod, providerCode, now)
	if err != nil {
		return 0, err
	}
	s.auditPlatformLoginMethodSessionsRevoked(ctx, platformCode, loginMethod, providerCode, sessionsBefore, revokedCount)
	return revokedCount, nil
}

func (s *Service) RevokeSessionsByExternalProvider(ctx context.Context, providerCode string) (int64, error) {
	if s.validityInvalidations != nil && s.validityInvalidations.Enabled() && store.SQLXFromContext(ctx) == nil {
		var count int64
		err := s.requireActiveSessionValidityTransaction(ctx, func(txCtx context.Context) error {
			var callErr error
			count, callErr = s.RevokeSessionsByExternalProvider(txCtx, providerCode)
			return callErr
		})
		return count, err
	}
	providerCode, err := canonicalExternalProviderCode(providerCode)
	if err != nil {
		return 0, err
	}
	now, err := s.captureSessionRevocationCutoff(ctx)
	if err != nil {
		return 0, err
	}
	sessionsBefore, err := s.collectSessionRevocationEffects(ctx, func(afterID int64, limit int) ([]domain.Session, error) {
		return s.repository.ListActiveSessionsByExternalProviderPage(ctx, providerCode, now, afterID, limit)
	})
	if err != nil {
		return 0, err
	}
	if err := s.repository.RevokeRefreshFamiliesByExternalProvider(ctx, providerCode, now); err != nil {
		return 0, err
	}
	revokedCount, err := s.repository.RevokeSessionsByExternalProvider(ctx, providerCode, now)
	if err != nil {
		return 0, err
	}
	s.auditExternalProviderSessionsRevoked(ctx, providerCode, sessionsBefore, revokedCount)
	return revokedCount, nil
}

func (s *Service) RevokeSessionsByExternalIdentity(ctx context.Context, identityID int64) (int64, error) {
	if s.validityInvalidations != nil && s.validityInvalidations.Enabled() && store.SQLXFromContext(ctx) == nil {
		var count int64
		err := s.requireActiveSessionValidityTransaction(ctx, func(txCtx context.Context) error {
			var callErr error
			count, callErr = s.RevokeSessionsByExternalIdentity(txCtx, identityID)
			return callErr
		})
		return count, err
	}
	if identityID <= 0 {
		return 0, apperrors.Params("externalIdentityId不能为空")
	}
	now, err := s.captureSessionRevocationCutoff(ctx)
	if err != nil {
		return 0, err
	}
	sessionsBefore, err := s.collectSessionRevocationEffects(ctx, func(afterID int64, limit int) ([]domain.Session, error) {
		return s.repository.ListActiveSessionsByExternalIdentityPage(ctx, identityID, now, afterID, limit)
	})
	if err != nil {
		return 0, err
	}
	if err := s.repository.RevokeRefreshFamiliesByExternalIdentity(ctx, identityID, now); err != nil {
		return 0, err
	}
	revokedCount, err := s.repository.RevokeSessionsByExternalIdentity(ctx, identityID, now)
	if err != nil {
		return 0, err
	}
	s.auditExternalIdentitySessionsRevoked(ctx, identityID, sessionsBefore, revokedCount)
	return revokedCount, nil
}

func (s *Service) ValidateAccessToken(ctx context.Context, accessToken string) (*ssofacade.AccessTokenPrincipal, error) {
	if s.jwt == nil {
		return nil, apperrors.System("SSO jwt service未配置")
	}
	claims, err := s.jwt.Verify(ctx, strings.TrimSpace(accessToken))
	if err != nil {
		return nil, apperrors.Unauthorized("access token 非法或已失效")
	}
	if strings.TrimSpace(stringValue(firstClaim(claims, "token_type", "typ"))) != "access_token" {
		return nil, apperrors.Unauthorized("access token 非法或已失效")
	}
	if err := s.validateTokenIssuer(claims, "access token 非法或已失效"); err != nil {
		return nil, err
	}
	exp := unixTime(claims["exp"])
	if exp == nil || !exp.After(time.Now().UTC()) {
		return nil, apperrors.Unauthorized("access token 非法或已失效")
	}
	userID, subject, ok := tokenUserClaims(claims)
	clientID := strings.TrimSpace(stringValue(firstClaim(claims, "client_id", "cid")))
	if !ok || clientID == "" || !claimContainsString(claims["aud"], clientID) {
		return nil, apperrors.Unauthorized("access token 非法或已失效")
	}
	sessionID := strings.TrimSpace(stringValue(claims["sid"]))
	session, err := s.ResolveActiveSessionForCandidateUse(ctx, sessionID)
	if err != nil {
		// An unavailable authoritative session check is not evidence that a
		// cryptographically valid token is invalid. Preserve the failure so a
		// caller cannot turn it into an authorization or revocation no-op.
		return nil, err
	}
	if session == nil {
		return nil, apperrors.Unauthorized("access token 对应会话不存在或已失效")
	}
	if session.UserID != userID || session.ClientID != clientID {
		return nil, apperrors.Unauthorized("access token 与会话不匹配")
	}
	now := time.Now().UTC()
	if s.cache != nil {
		allow, err := s.cache.AllowTouch(ctx, session.SessionID, time.Duration(maxInt(s.cfg.SessionTouchThrottleSecond, 30))*time.Second)
		if err == nil && allow {
			_ = s.repository.TouchSession(ctx, session.SessionID, now)
		}
	}
	iat := unixTime(claims["iat"])
	return &ssofacade.AccessTokenPrincipal{
		TokenID:   strings.TrimSpace(stringValue(claims["jti"])),
		UserID:    userID,
		Subject:   subject,
		ClientID:  clientID,
		SessionID: sessionID,
		Scopes:    scopeSlice(firstClaim(claims, "scope", "scp")),
		ACR:       strings.TrimSpace(stringValue(claims["acr"])),
		AMR:       stringSlice(claims["amr"]),
		IssuedAt:  iat,
		ExpiresAt: exp,
	}, nil
}

func (s *Service) BuildDiscoveryDocument(ctx context.Context) (map[string]any, error) {
	_ = ctx
	baseURL := ssoBaseURL(s.cfg.BaseURL, s.cfg.Issuer)
	issuer := strings.TrimSpace(s.cfg.Issuer)
	if issuer == "" {
		issuer = baseURL
	}
	return map[string]any{
		"issuer":                                issuer,
		"authorization_endpoint":                baseURL + "/oauth2/authorize",
		"token_endpoint":                        baseURL + "/oauth2/token",
		"userinfo_endpoint":                     baseURL + "/oauth2/userinfo",
		"revocation_endpoint":                   baseURL + "/oauth2/revoke",
		"introspection_endpoint":                baseURL + "/oauth2/introspect",
		"jwks_uri":                              baseURL + "/.well-known/jwks.json",
		"scopes_supported":                      []string{"openid", "profile", "email", "offline_access"},
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"token_endpoint_auth_methods_supported": []string{"none", "client_secret_basic"},
		"code_challenge_methods_supported":      []string{"S256"},
	}, nil
}

func (s *Service) CanRedirectToClient(ctx context.Context, clientID, redirectURI string) bool {
	client, err := s.requireClient(ctx, clientID)
	if err != nil || client == nil {
		return false
	}
	return s.isRedirectAllowed(client, redirectURI)
}

func (s *Service) BuildJwksDocument(ctx context.Context) (map[string]any, error) {
	if s.jwt == nil {
		return nil, apperrors.System("SSO jwt service未配置")
	}
	raw, err := s.jwt.JWKS(ctx)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := sonic.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) ExchangeAuthorizationCode(ctx context.Context, clientID, clientSecret, code, redirectURI, codeVerifier string) (*tokenBundle, error) {
	if strings.TrimSpace(clientID) == "" {
		return nil, apperrors.Params("缺少 client_id")
	}
	if strings.TrimSpace(code) == "" || strings.TrimSpace(redirectURI) == "" {
		return nil, apperrors.Params("缺少 code 或 redirect_uri")
	}
	client, err := s.requireClient(ctx, clientID)
	if err != nil {
		s.auditOAuthTokenExchangeClientFailure(ctx, strings.TrimSpace(clientID), "unknown_client", err)
		return nil, err
	}
	if !clientAllowsGrant(client, "authorization_code") {
		err := apperrors.Unauthorized("当前客户端不允许 authorization_code 授权")
		s.auditOAuthTokenExchangeClientFailure(ctx, client.ClientID, "grant_not_allowed", err)
		return nil, err
	}
	if err := s.authenticateClient(client, clientSecret); err != nil {
		s.auditOAuthTokenExchangeClientFailure(ctx, client.ClientID, "invalid_client", err)
		return nil, err
	}
	item, err := s.repository.FindAuthorizationCode(ctx, strings.TrimSpace(code))
	if err != nil || item == nil {
		err := apperrors.Unauthorized("authorization code 无效")
		s.auditOAuthTokenExchangeClientFailure(ctx, client.ClientID, "invalid_code", err)
		return nil, err
	}
	if item.Status != domain.CodeStatusActive || item.ExpiresAt.Before(time.Now().UTC()) {
		err := apperrors.Unauthorized("authorization code 无效")
		s.auditOAuthTokenExchangeClientFailure(ctx, client.ClientID, "invalid_code", err)
		return nil, err
	}
	if item.ClientID != client.ClientID {
		err := apperrors.Unauthorized("authorization code 无效")
		s.auditOAuthTokenExchangeClientFailure(ctx, client.ClientID, "client_mismatch", err)
		return nil, err
	}
	if redirectURI != item.RedirectURI {
		err := apperrors.Unauthorized("redirect_uri 不匹配")
		s.auditOAuthTokenExchangeFailure(ctx, client.ClientID, item.UserID, item.SessionID, len(item.Scopes), "redirect_mismatch", err)
		return nil, err
	}
	if clientRequiresPKCE(client) && strings.TrimSpace(item.CodeChallenge) == "" {
		err := apperrors.Unauthorized("PKCE 校验失败")
		s.auditOAuthTokenExchangeFailure(ctx, client.ClientID, item.UserID, item.SessionID, len(item.Scopes), "pkce_required", err)
		return nil, err
	}
	if item.CodeChallenge != "" {
		if !strings.EqualFold(strings.TrimSpace(item.CodeChallengeMethod), "S256") ||
			strings.TrimSpace(codeVerifier) == "" ||
			!verifyPKCE(item.CodeChallenge, strings.TrimSpace(codeVerifier)) {
			err := apperrors.Unauthorized("PKCE 校验失败")
			s.auditOAuthTokenExchangeFailure(ctx, client.ClientID, item.UserID, item.SessionID, len(item.Scopes), "pkce_failed", err)
			return nil, err
		}
	}
	consumed, err := s.repository.ConsumeAuthorizationCode(ctx, item.Code, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if !consumed {
		err := apperrors.Unauthorized("authorization code 已被消费")
		s.auditOAuthTokenExchangeFailure(ctx, client.ClientID, item.UserID, item.SessionID, len(item.Scopes), "code_replay", err)
		return nil, err
	}
	session, err := s.repository.FindSessionBySessionID(ctx, item.SessionID)
	if err != nil || session == nil {
		return nil, apperrors.Unauthorized("登录会话不存在或已失效")
	}
	if session.Status != domain.SessionStatusActive || session.ExpiresAt.Before(time.Now().UTC()) {
		return nil, apperrors.Unauthorized("登录会话不存在或已失效")
	}
	if session.UserID != item.UserID || session.ClientID != client.ClientID {
		return nil, apperrors.Unauthorized("authorization code 与登录会话不匹配")
	}
	accessToken, accessExpiresAt, err := s.issueAccessToken(ctx, client, session, item.Scopes)
	if err != nil {
		return nil, err
	}
	idToken, err := s.issueIDToken(ctx, client, session, item.Scopes, item.Nonce)
	if err != nil {
		return nil, err
	}
	bundle := &tokenBundle{
		AccessToken:             accessToken,
		IDToken:                 idToken,
		TokenType:               "Bearer",
		ExpiresInSeconds:        int64(time.Until(accessExpiresAt).Seconds()),
		Scope:                   strings.Join(item.Scopes, " "),
		RefreshTokenBodyAllowed: !client.TrustedFirstParty,
	}
	if hasScope(item.Scopes, "offline_access") && clientAllowsGrant(client, "refresh_token") {
		refreshToken, refreshExpiresAt, err := s.issueRefreshToken(ctx, client, session, item.Scopes)
		if err != nil {
			return nil, err
		}
		family := &domain.RefreshTokenFamily{
			FamilyID:         s.newID("sso_rtf"),
			SessionID:        session.SessionID,
			ClientID:         session.ClientID,
			UserID:           session.UserID,
			CurrentTokenHash: ssohelper.BuildTokenHash(refreshToken),
			ExpiresAt:        refreshExpiresAt,
			Status:           domain.RefreshFamilyStatusActive,
			MetadataJSON:     metadataJSON(map[string]any{"scopes": item.Scopes}),
		}
		if err := s.repository.InsertRefreshTokenFamily(ctx, family); err != nil {
			return nil, err
		}
		bundle.RefreshToken = refreshToken
		bundle.RefreshTokenExpiresAt = &family.ExpiresAt
	}
	s.auditOAuthTokenExchange(ctx, client.ClientID, session.UserID, session.SessionID, len(item.Scopes), bundle.RefreshToken != "", nil)
	return bundle, nil
}

func (s *Service) ExchangeRefreshToken(ctx context.Context, clientID, clientSecret, refreshToken string) (*tokenBundle, error) {
	return s.exchangeRefreshToken(ctx, clientID, clientSecret, refreshToken, false)
}

func (s *Service) ExchangeRefreshTokenFromCookie(ctx context.Context, clientID, clientSecret, refreshToken string) (*tokenBundle, error) {
	return s.exchangeRefreshToken(ctx, clientID, clientSecret, refreshToken, true)
}

func (s *Service) exchangeRefreshToken(ctx context.Context, clientID, clientSecret, refreshToken string, fromCookie bool) (*tokenBundle, error) {
	if s.validityInvalidations != nil && s.validityInvalidations.Enabled() && store.SQLXFromContext(ctx) == nil {
		var result *tokenBundle
		var reuseDetected *refreshReuseCommitted
		err := s.requireActiveSessionValidityTransaction(ctx, func(txCtx context.Context) error {
			var callErr error
			result, callErr = s.exchangeRefreshToken(txCtx, clientID, clientSecret, refreshToken, fromCookie)
			if errors.As(callErr, &reuseDetected) {
				// The punitive fact mutation and its exact target event have both
				// succeeded. Returning nil commits them; the public Unauthorized is
				// deliberately produced after the transaction boundary below.
				return nil
			}
			return callErr
		})
		if reuseDetected != nil {
			return nil, apperrors.Unauthorized("refresh token reuse detected")
		}
		return result, err
	}
	if strings.TrimSpace(clientID) == "" {
		return nil, apperrors.Params("缺少 client_id")
	}
	if strings.TrimSpace(refreshToken) == "" {
		return nil, apperrors.Params("缺少 refresh_token")
	}
	client, err := s.requireClient(ctx, clientID)
	if err != nil {
		return nil, err
	}
	if fromCookie && !client.TrustedFirstParty {
		return nil, apperrors.Params("refresh token cookie fallback requires trusted first-party client")
	}
	if !clientAllowsGrant(client, "refresh_token") {
		return nil, apperrors.Unauthorized("当前客户端不允许 refresh_token 授权")
	}
	if err := s.authenticateClient(client, clientSecret); err != nil {
		return nil, err
	}
	refreshPrincipal, err := s.parseRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, err
	}
	if refreshPrincipal.ClientID != client.ClientID {
		return nil, apperrors.Unauthorized("refresh token 无效")
	}
	hash := ssohelper.BuildTokenHash(refreshToken)
	family, err := s.repository.FindRefreshFamilyByCurrentHash(ctx, hash)
	if err != nil || family == nil {
		prev, prevErr := s.repository.FindRefreshFamilyByPreviousHash(ctx, hash)
		if prevErr == nil && prev != nil {
			now := time.Now().UTC()
			if s.isRefreshRotationWithinReplaySkew(prev, refreshPrincipal, client.ClientID, now) {
				s.auditOAuthRefreshReuse(ctx, client.ClientID, refreshPrincipal.UserID, refreshPrincipal.SessionID, "rotation_skew_replay", false)
				return nil, apperrors.Unauthorized("refresh token 已轮换，请使用最新 refresh token")
			}
			if err := s.repository.MarkRefreshFamilyReuseDetected(ctx, prev.FamilyID, now); err != nil {
				return nil, err
			}
			if s.validityInvalidations == nil || !s.validityInvalidations.Enabled() {
				if err := s.repository.RevokeRefreshFamiliesBySessionID(ctx, prev.SessionID, now); err != nil {
					return nil, err
				}
			}
			if _, err := s.RevokeSession(ctx, prev.SessionID); err != nil {
				return nil, err
			}
			s.auditOAuthRefreshReuse(ctx, client.ClientID, refreshPrincipal.UserID, prev.SessionID, "reuse_detected", true)
			if s.validityInvalidations == nil || !s.validityInvalidations.Enabled() {
				return nil, apperrors.Unauthorized("refresh token reuse detected")
			}
			return nil, &refreshReuseCommitted{}
		}
		return nil, apperrors.Unauthorized("refresh token 无效")
	}
	now := time.Now().UTC()
	if family.Status != domain.RefreshFamilyStatusActive || family.ExpiresAt.Before(now) || (family.RevokedAt != nil && !family.RevokedAt.IsZero()) {
		return nil, apperrors.Unauthorized("refresh token 无效")
	}
	if refreshPrincipal.SessionID != family.SessionID ||
		refreshPrincipal.UserID != family.UserID ||
		refreshPrincipal.ClientID != family.ClientID ||
		family.ClientID != client.ClientID {
		return nil, apperrors.Unauthorized("refresh token 无效")
	}
	session, err := s.repository.FindSessionBySessionID(ctx, family.SessionID)
	if err != nil || session == nil || session.Status != domain.SessionStatusActive || session.ExpiresAt.Before(now) {
		return nil, apperrors.Unauthorized("登录会话不存在或已失效")
	}
	if session.UserID != family.UserID || session.ClientID != client.ClientID {
		return nil, apperrors.Unauthorized("refresh token 与登录会话不匹配")
	}
	refreshScopes := parseFamilyScopes(family.MetadataJSON)
	if len(refreshScopes) == 0 {
		refreshScopes = []string{"openid", "profile", "email", "offline_access"}
	}
	nextRefresh, nextRefreshExpiresAt, err := s.issueRefreshToken(ctx, client, session, refreshScopes)
	if err != nil {
		return nil, err
	}
	rotated, err := s.repository.RotateRefreshFamily(ctx, family.FamilyID, hash, ssohelper.BuildTokenHash(nextRefresh), now)
	if err != nil {
		return nil, err
	}
	if !rotated {
		s.auditOAuthRefresh(ctx, client.ClientID, session.UserID, session.SessionID, len(refreshScopes), "rotation_conflict", false, apperrors.Unauthorized("refresh token 无效"))
		return nil, apperrors.Unauthorized("refresh token 无效")
	}
	accessToken, accessExpiresAt, err := s.issueAccessToken(ctx, client, session, refreshScopes)
	if err != nil {
		return nil, err
	}
	idToken, err := s.issueIDToken(ctx, client, session, refreshScopes, "")
	if err != nil {
		return nil, err
	}
	bundle := &tokenBundle{
		AccessToken:             accessToken,
		IDToken:                 idToken,
		TokenType:               "Bearer",
		ExpiresInSeconds:        int64(time.Until(accessExpiresAt).Seconds()),
		Scope:                   strings.Join(refreshScopes, " "),
		RefreshToken:            nextRefresh,
		RefreshTokenExpiresAt:   &nextRefreshExpiresAt,
		RefreshTokenBodyAllowed: !client.TrustedFirstParty,
	}
	s.auditOAuthRefresh(ctx, client.ClientID, session.UserID, session.SessionID, len(refreshScopes), "refreshed", true, nil)
	return bundle, nil
}

func (s *Service) isRefreshRotationWithinReplaySkew(family *domain.RefreshTokenFamily, principal *refreshTokenPrincipal, clientID string, now time.Time) bool {
	if family == nil || principal == nil || family.RotatedAt == nil {
		return false
	}
	if family.Status != domain.RefreshFamilyStatusActive || family.ExpiresAt.Before(now) || (family.RevokedAt != nil && !family.RevokedAt.IsZero()) {
		return false
	}
	if principal.SessionID != family.SessionID || principal.UserID != family.UserID || principal.ClientID != family.ClientID || family.ClientID != strings.TrimSpace(clientID) {
		return false
	}
	skewSeconds := s.cfg.RefreshReplayClockSkewSec
	if skewSeconds <= 0 {
		return false
	}
	delta := now.Sub(family.RotatedAt.UTC())
	if delta < 0 {
		delta = -delta
	}
	return delta <= time.Duration(skewSeconds)*time.Second
}

func (s *Service) GetUserInfo(ctx context.Context, accessToken string) (map[string]any, error) {
	principal, err := s.ValidateAccessToken(ctx, accessToken)
	if err != nil {
		return nil, err
	}
	profile, err := s.profiles.GetProfileByUserID(ctx, principal.UserID)
	if err != nil || profile == nil {
		s.auditOAuthUserInfo(ctx, principal, "profile_missing", len(principal.Scopes), nil, err)
		return nil, apperrors.Unauthorized("当前用户不存在或已失效")
	}
	scopeSet := make(map[string]struct{})
	for _, item := range principal.Scopes {
		scopeSet[item] = struct{}{}
	}
	result := map[string]any{
		"sub": principal.Subject,
	}
	if _, ok := scopeSet["profile"]; ok {
		result["preferred_username"] = profile.AccountName
		result["name"] = profile.NickName
		result["picture"] = profile.Avatar
		result["profile"] = profile.Profile
	}
	if _, ok := scopeSet["email"]; ok {
		result["email"] = profile.Email
		result["email_verified"] = profile.EmailVerified
	}
	s.auditOAuthUserInfo(ctx, principal, "accessed", len(principal.Scopes), claimsReturned(result), nil)
	return result, nil
}

func (s *Service) RevokeToken(ctx context.Context, token string, tokenTypeHint string) error {
	if s.validityInvalidations != nil && s.validityInvalidations.Enabled() && store.SQLXFromContext(ctx) == nil {
		return s.requireActiveSessionValidityTransaction(ctx, func(txCtx context.Context) error { return s.RevokeToken(txCtx, token, tokenTypeHint) })
	}
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(tokenTypeHint), "access_token") {
		hash := ssohelper.BuildTokenHash(trimmed)
		family, err := s.repository.FindRefreshFamilyByCurrentHash(ctx, hash)
		if err != nil {
			return err
		}
		if family != nil {
			return s.revokeSessionAndRefreshFamilies(ctx, family.SessionID)
		}
		family, err = s.repository.FindRefreshFamilyByPreviousHash(ctx, hash)
		if err != nil {
			return err
		}
		if family != nil {
			return s.revokeSessionAndRefreshFamilies(ctx, family.SessionID)
		}
	}
	principal, err := s.ValidateAccessToken(ctx, trimmed)
	if err != nil {
		if isInvalidOAuthTokenError(err) {
			return nil
		}
		return err
	}
	if principal == nil {
		return nil
	}
	return s.revokeSessionAndRefreshFamilies(ctx, principal.SessionID)
}

func (s *Service) RevokeTokenForClient(ctx context.Context, clientID, clientSecret, token string, tokenTypeHint string) error {
	if s.validityInvalidations != nil && s.validityInvalidations.Enabled() && store.SQLXFromContext(ctx) == nil {
		return s.requireActiveSessionValidityTransaction(ctx, func(txCtx context.Context) error {
			return s.RevokeTokenForClient(txCtx, clientID, clientSecret, token, tokenTypeHint)
		})
	}
	auditClientID := strings.TrimSpace(clientID)
	hint := normalizeOAuthTokenTypeHint(tokenTypeHint)
	client, err := s.requireClient(ctx, clientID)
	if err != nil {
		s.auditOAuthTokenRevocation(ctx, auditClientID, nil, "", hint, "invalid_client", err)
		return err
	}
	auditClientID = client.ClientID
	if err := s.authenticateClient(client, clientSecret); err != nil {
		s.auditOAuthTokenRevocation(ctx, auditClientID, nil, "", hint, "invalid_client", err)
		return err
	}
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		s.auditOAuthTokenRevocation(ctx, auditClientID, nil, "", hint, "empty_token", nil)
		return nil
	}
	if hint != "access_token" {
		principal, parseErr := s.parseRefreshToken(ctx, trimmed)
		if parseErr == nil && principal != nil {
			if strings.TrimSpace(principal.ClientID) != strings.TrimSpace(client.ClientID) {
				s.auditOAuthTokenRevocation(ctx, auditClientID, nil, "", hint, "cross_client_noop", nil)
				return nil
			}
			sessionID, revokeErr := s.revokeRefreshTokenForClient(ctx, client.ClientID, trimmed)
			outcome := "revoked"
			if revokeErr == nil && strings.TrimSpace(sessionID) == "" {
				outcome = "invalid_token_noop"
			}
			s.auditOAuthTokenRevocation(ctx, auditClientID, &principal.UserID, firstNonBlank(sessionID, principal.SessionID), hint, outcome, revokeErr)
			return revokeErr
		}
		if parseErr != nil && !isInvalidOAuthTokenError(parseErr) {
			s.auditOAuthTokenRevocation(ctx, auditClientID, nil, "", hint, "authority_error", parseErr)
			return parseErr
		}
		if hint == "refresh_token" {
			s.auditOAuthTokenRevocation(ctx, auditClientID, nil, "", hint, "invalid_token_noop", nil)
			return nil
		}
	}
	principal, parseErr := s.parseAccessTokenForRevocation(ctx, trimmed)
	if parseErr != nil {
		if isInvalidOAuthTokenError(parseErr) {
			s.auditOAuthTokenRevocation(ctx, auditClientID, nil, "", hint, "invalid_token_noop", nil)
			return nil
		}
		s.auditOAuthTokenRevocation(ctx, auditClientID, nil, "", hint, "authority_error", parseErr)
		return parseErr
	}
	if principal == nil || strings.TrimSpace(principal.ClientID) != strings.TrimSpace(client.ClientID) {
		s.auditOAuthTokenRevocation(ctx, auditClientID, nil, "", hint, "invalid_token_noop", nil)
		return nil
	}
	active, validateErr := s.ValidateAccessToken(ctx, trimmed)
	if validateErr != nil {
		if isInvalidOAuthTokenError(validateErr) {
			s.auditOAuthTokenRevocation(ctx, auditClientID, &principal.UserID, principal.SessionID, hint, "inactive_token_noop", nil)
			return nil
		}
		s.auditOAuthTokenRevocation(ctx, auditClientID, &principal.UserID, principal.SessionID, hint, "authority_error", validateErr)
		return validateErr
	}
	if active == nil || strings.TrimSpace(active.ClientID) != strings.TrimSpace(client.ClientID) {
		s.auditOAuthTokenRevocation(ctx, auditClientID, &principal.UserID, principal.SessionID, hint, "inactive_token_noop", nil)
		return nil
	}
	revokeErr := s.revokeSessionAndRefreshFamilies(ctx, active.SessionID)
	s.auditOAuthTokenRevocation(ctx, auditClientID, &active.UserID, active.SessionID, hint, "revoked", revokeErr)
	return revokeErr
}

func (s *Service) IntrospectTokenForClient(ctx context.Context, clientID, clientSecret, token string, tokenTypeHint string) (map[string]any, error) {
	auditClientID := strings.TrimSpace(clientID)
	hint := normalizeOAuthTokenTypeHint(tokenTypeHint)
	client, err := s.requireClient(ctx, clientID)
	if err != nil {
		s.auditOAuthTokenIntrospection(ctx, auditClientID, hint, nil, err)
		return nil, err
	}
	auditClientID = client.ClientID
	if err := s.authenticateClient(client, clientSecret); err != nil {
		s.auditOAuthTokenIntrospection(ctx, auditClientID, hint, nil, err)
		return nil, err
	}
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		result := map[string]any{"active": false}
		s.auditOAuthTokenIntrospection(ctx, auditClientID, hint, result, nil)
		return result, nil
	}
	if hint == "refresh_token" {
		if result := s.introspectRefreshToken(ctx, client.ClientID, trimmed); result != nil {
			s.auditOAuthTokenIntrospection(ctx, auditClientID, hint, result, nil)
			return result, nil
		}
		result := map[string]any{"active": false}
		s.auditOAuthTokenIntrospection(ctx, auditClientID, hint, result, nil)
		return result, nil
	}
	if result := s.introspectAccessToken(ctx, client.ClientID, trimmed); result != nil {
		s.auditOAuthTokenIntrospection(ctx, auditClientID, hint, result, nil)
		return result, nil
	}
	if hint == "access_token" {
		result := map[string]any{"active": false}
		s.auditOAuthTokenIntrospection(ctx, auditClientID, hint, result, nil)
		return result, nil
	}
	if result := s.introspectRefreshToken(ctx, client.ClientID, trimmed); result != nil {
		s.auditOAuthTokenIntrospection(ctx, auditClientID, hint, result, nil)
		return result, nil
	}
	result := map[string]any{"active": false}
	s.auditOAuthTokenIntrospection(ctx, auditClientID, hint, result, nil)
	return result, nil
}

func (s *Service) auditOAuthTokenRevocation(ctx context.Context, clientID string, userID *int64, sessionID, tokenTypeHint, outcome string, err error) {
	result := ssoAuditResultSuccess
	reasonCode := strings.TrimSpace(outcome)
	if err != nil {
		result = ssoAuditResultFailure
		if reasonCode == "" {
			reasonCode = "operation_failed"
		}
	}
	s.insertSSOAuditLog(ctx, domain.AuditLog{
		EventType:  ssoAuditEventTokenRevoked,
		ClientID:   strings.TrimSpace(clientID),
		UserID:     userID,
		SessionID:  strings.TrimSpace(sessionID),
		Result:     result,
		ReasonCode: reasonCode,
		DetailJSON: metadataJSON(map[string]any{
			"tokenTypeHint": normalizeOAuthTokenTypeHint(tokenTypeHint),
			"outcome":       reasonCode,
		}),
	})
}

func (s *Service) auditSessionRevoked(ctx context.Context, session *domain.Session, revoked bool) {
	if session == nil {
		return
	}
	reasonCode := "revoked"
	revokedCount := 0
	if revoked {
		revokedCount = 1
	} else {
		reasonCode = "not_active"
	}
	s.insertSSOAuditLog(ctx, domain.AuditLog{
		EventType:  ssoAuditEventSessionRevoked,
		ClientID:   strings.TrimSpace(session.ClientID),
		UserID:     int64Ptr(session.UserID),
		SessionID:  strings.TrimSpace(session.SessionID),
		DeviceID:   strings.TrimSpace(session.DeviceID),
		LoginIP:    strings.TrimSpace(session.LoginIP),
		UserAgent:  strings.TrimSpace(session.UserAgent),
		Result:     ssoAuditResultSuccess,
		ReasonCode: reasonCode,
		DetailJSON: metadataJSON(map[string]any{
			"operation":    "revoke_session",
			"sessionId":    strings.TrimSpace(session.SessionID),
			"clientId":     strings.TrimSpace(session.ClientID),
			"deviceId":     strings.TrimSpace(session.DeviceID),
			"statusBefore": session.Status,
			"revoked":      revoked,
			"revokedCount": revokedCount,
		}),
	})
}

func (s *Service) auditUserSessionsRevoked(ctx context.Context, userID int64, sessionsBefore []domain.Session, revokedCount int64) {
	activeSnapshots := make([]map[string]any, 0, len(sessionsBefore))
	for _, session := range sessionsBefore {
		if session.Status != domain.SessionStatusActive || session.RevokedAt != nil {
			continue
		}
		activeSnapshots = append(activeSnapshots, map[string]any{
			"sessionId":    strings.TrimSpace(session.SessionID),
			"clientId":     strings.TrimSpace(session.ClientID),
			"deviceId":     strings.TrimSpace(session.DeviceID),
			"statusBefore": session.Status,
		})
	}
	reasonCode := "revoked"
	if revokedCount <= 0 {
		reasonCode = "no_active_session"
	}
	activeCountBefore := max(revokedCount, int64(len(activeSnapshots)))
	s.insertSSOAuditLog(ctx, domain.AuditLog{
		EventType:  ssoAuditEventUserSessionsRevoked,
		UserID:     int64Ptr(userID),
		Result:     ssoAuditResultSuccess,
		ReasonCode: reasonCode,
		DetailJSON: metadataJSON(map[string]any{
			"operation":                "revoke_user_sessions",
			"revokedCount":             revokedCount,
			"activeSessionCountBefore": activeCountBefore,
			"sessionSnapshotCount":     len(activeSnapshots),
			"sessionSnapshotsTruncated": activeCountBefore >
				int64(len(activeSnapshots)),
			"sessions": activeSnapshots,
		}),
	})
}

func (s *Service) auditExternalProviderSessionsRevoked(ctx context.Context, providerCode string, sessionsBefore []domain.Session, revokedCount int64) {
	reasonCode := "revoked"
	if revokedCount <= 0 {
		reasonCode = "no_active_session"
	}
	activeCountBefore := max(revokedCount, int64(len(sessionsBefore)))
	s.insertSSOAuditLog(ctx, domain.AuditLog{
		EventType:  ssoAuditEventExternalProviderSessionsRevoked,
		Result:     ssoAuditResultSuccess,
		ReasonCode: reasonCode,
		DetailJSON: metadataJSON(map[string]any{
			"operation":                "revoke_external_provider_sessions",
			"providerCode":             strings.TrimSpace(providerCode),
			"revokedCount":             revokedCount,
			"activeSessionCountBefore": activeCountBefore,
			"sessionSnapshotCount":     len(sessionsBefore),
			"sessionSnapshotsTruncated": activeCountBefore >
				int64(len(sessionsBefore)),
			"sessions": externalSessionAuditSnapshots(sessionsBefore),
		}),
	})
}

func (s *Service) auditPlatformSessionsRevoked(ctx context.Context, platformCode string, sessionsBefore []domain.Session, revokedCount int64) {
	reasonCode := "revoked"
	if revokedCount <= 0 {
		reasonCode = "no_active_session"
	}
	activeCountBefore := max(revokedCount, int64(len(sessionsBefore)))
	s.insertSSOAuditLog(ctx, domain.AuditLog{
		EventType:  ssoAuditEventPlatformSessionsRevoked,
		Result:     ssoAuditResultSuccess,
		ReasonCode: reasonCode,
		DetailJSON: metadataJSON(map[string]any{
			"operation":                "revoke_platform_sessions",
			"platformCode":             strings.TrimSpace(platformCode),
			"revokedCount":             revokedCount,
			"activeSessionCountBefore": activeCountBefore,
			"sessionSnapshotCount":     len(sessionsBefore),
			"sessionSnapshotsTruncated": activeCountBefore >
				int64(len(sessionsBefore)),
			"sessions": externalSessionAuditSnapshots(sessionsBefore),
		}),
	})
}

func (s *Service) auditPlatformLoginMethodSessionsRevoked(ctx context.Context, platformCode, loginMethod, providerCode string, sessionsBefore []domain.Session, revokedCount int64) {
	reasonCode := "revoked"
	if revokedCount <= 0 {
		reasonCode = "no_active_session"
	}
	activeCountBefore := max(revokedCount, int64(len(sessionsBefore)))
	s.insertSSOAuditLog(ctx, domain.AuditLog{
		EventType:  ssoAuditEventPlatformLoginMethodRevoked,
		Result:     ssoAuditResultSuccess,
		ReasonCode: reasonCode,
		DetailJSON: metadataJSON(map[string]any{
			"operation":                "revoke_platform_login_method_sessions",
			"platformCode":             strings.TrimSpace(platformCode),
			"loginMethod":              strings.TrimSpace(loginMethod),
			"externalProviderCode":     strings.TrimSpace(providerCode),
			"revokedCount":             revokedCount,
			"activeSessionCountBefore": activeCountBefore,
			"sessionSnapshotCount":     len(sessionsBefore),
			"sessionSnapshotsTruncated": activeCountBefore >
				int64(len(sessionsBefore)),
			"sessions": externalSessionAuditSnapshots(sessionsBefore),
		}),
	})
}

func (s *Service) auditExternalIdentitySessionsRevoked(ctx context.Context, identityID int64, sessionsBefore []domain.Session, revokedCount int64) {
	reasonCode := "revoked"
	if revokedCount <= 0 {
		reasonCode = "no_active_session"
	}
	activeCountBefore := max(revokedCount, int64(len(sessionsBefore)))
	s.insertSSOAuditLog(ctx, domain.AuditLog{
		EventType:  ssoAuditEventExternalIdentitySessionsRevoked,
		Result:     ssoAuditResultSuccess,
		ReasonCode: reasonCode,
		DetailJSON: metadataJSON(map[string]any{
			"operation":                "revoke_external_identity_sessions",
			"externalIdentityId":       identityID,
			"revokedCount":             revokedCount,
			"activeSessionCountBefore": activeCountBefore,
			"sessionSnapshotCount":     len(sessionsBefore),
			"sessionSnapshotsTruncated": activeCountBefore >
				int64(len(sessionsBefore)),
			"sessions": externalSessionAuditSnapshots(sessionsBefore),
		}),
	})
}

func (s *Service) clearSessionTouchCache(ctx context.Context, sessions []domain.Session) {
	if s.cache == nil {
		return
	}
	for _, session := range sessions {
		_ = s.cache.ClearSessionTouch(ctx, session.SessionID)
	}
}

type activeSessionPageLoader func(afterID int64, limit int) ([]domain.Session, error)

func (s *Service) captureSessionRevocationCutoff(ctx context.Context) (time.Time, error) {
	cutoff, err := s.repository.CaptureManagedSessionCutoff(ctx)
	if err != nil {
		return time.Time{}, err
	}
	if cutoff.IsZero() {
		return time.Time{}, apperrors.ServiceUnavailable("用户会话撤销截止条件不可用")
	}
	return cutoff.UTC(), nil
}

func (s *Service) collectSessionRevocationEffects(ctx context.Context, load activeSessionPageLoader) ([]domain.Session, error) {
	if load == nil {
		return nil, apperrors.System("SSO会话撤销分页器未配置")
	}
	var fence cachepolicy.TargetedMutationFence
	if s != nil && s.validityInvalidations != nil && s.validityInvalidations.Enabled() {
		var err error
		fence, err = s.beginActiveSessionValidityMutationFence(ctx)
		if err != nil {
			return nil, err
		}
	}
	snapshots := make([]domain.Session, 0, sessionRevocationAuditItemMax)
	var afterID int64
	for {
		page, err := load(afterID, sessionRevocationPageSize)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			return snapshots, nil
		}
		if err := s.registerActiveSessionValidityTargetsWithFence(ctx, page, fence); err != nil {
			return nil, err
		}
		s.clearSessionTouchCache(ctx, page)
		remaining := sessionRevocationAuditItemMax - len(snapshots)
		if remaining > 0 {
			if len(page) < remaining {
				remaining = len(page)
			}
			snapshots = append(snapshots, page[:remaining]...)
		}
		nextID := page[len(page)-1].ID
		if nextID <= afterID {
			return nil, apperrors.System("SSO会话撤销分页顺序无效")
		}
		afterID = nextID
		if len(page) < sessionRevocationPageSize {
			return snapshots, nil
		}
	}
}

func (s *Service) auditOAuthTokenExchange(ctx context.Context, clientID string, userID int64, sessionID string, scopeCount int, refreshIssued bool, err error) {
	result := ssoAuditResultSuccess
	reasonCode := "exchanged"
	if err != nil {
		result = ssoAuditResultFailure
		reasonCode = "exchange_failed"
	}
	s.insertOAuthTokenExchangeAudit(ctx, clientID, userID, sessionID, scopeCount, refreshIssued, result, reasonCode)
}

func (s *Service) auditOAuthTokenExchangeFailure(ctx context.Context, clientID string, userID int64, sessionID string, scopeCount int, reasonCode string, err error) {
	if err == nil {
		err = apperrors.Unauthorized("authorization code exchange failed")
	}
	if strings.TrimSpace(reasonCode) == "" {
		reasonCode = "exchange_failed"
	}
	s.insertOAuthTokenExchangeAudit(ctx, clientID, userID, sessionID, scopeCount, false, ssoAuditResultFailure, reasonCode)
}

func (s *Service) auditOAuthTokenExchangeClientFailure(ctx context.Context, clientID string, reasonCode string, err error) {
	if err == nil {
		err = apperrors.Unauthorized("authorization code exchange failed")
	}
	if strings.TrimSpace(reasonCode) == "" {
		reasonCode = "exchange_failed"
	}
	s.insertSSOAuditLog(ctx, domain.AuditLog{
		EventType:  ssoAuditEventTokenExchanged,
		ClientID:   strings.TrimSpace(clientID),
		Result:     ssoAuditResultFailure,
		ReasonCode: reasonCode,
		DetailJSON: metadataJSON(map[string]any{
			"grantType":     "authorization_code",
			"refreshIssued": false,
			"scopeCount":    0,
		}),
	})
}

func (s *Service) insertOAuthTokenExchangeAudit(ctx context.Context, clientID string, userID int64, sessionID string, scopeCount int, refreshIssued bool, result, reasonCode string) {
	s.insertSSOAuditLog(ctx, domain.AuditLog{
		EventType:  ssoAuditEventTokenExchanged,
		ClientID:   strings.TrimSpace(clientID),
		UserID:     int64Ptr(userID),
		SessionID:  strings.TrimSpace(sessionID),
		Result:     result,
		ReasonCode: reasonCode,
		DetailJSON: metadataJSON(map[string]any{
			"grantType":     "authorization_code",
			"refreshIssued": refreshIssued,
			"scopeCount":    scopeCount,
		}),
	})
}

func (s *Service) auditOAuthRefresh(ctx context.Context, clientID string, userID int64, sessionID string, scopeCount int, reasonCode string, rotated bool, err error) {
	result := ssoAuditResultSuccess
	if err != nil {
		result = ssoAuditResultFailure
	}
	if strings.TrimSpace(reasonCode) == "" {
		reasonCode = "refreshed"
	}
	s.insertSSOAuditLog(ctx, domain.AuditLog{
		EventType:  ssoAuditEventTokenRefreshed,
		ClientID:   strings.TrimSpace(clientID),
		UserID:     int64Ptr(userID),
		SessionID:  strings.TrimSpace(sessionID),
		Result:     result,
		ReasonCode: reasonCode,
		DetailJSON: metadataJSON(map[string]any{
			"grantType":  "refresh_token",
			"rotated":    rotated,
			"scopeCount": scopeCount,
		}),
	})
}

func (s *Service) auditOAuthRefreshReuse(ctx context.Context, clientID string, userID int64, sessionID string, reasonCode string, punished bool) {
	if strings.TrimSpace(reasonCode) == "" {
		reasonCode = "reuse_detected"
	}
	s.insertSSOAuditLog(ctx, domain.AuditLog{
		EventType:  ssoAuditEventRefreshReuse,
		ClientID:   strings.TrimSpace(clientID),
		UserID:     int64Ptr(userID),
		SessionID:  strings.TrimSpace(sessionID),
		Result:     ssoAuditResultFailure,
		ReasonCode: reasonCode,
		DetailJSON: metadataJSON(map[string]any{
			"grantType": "refresh_token",
			"punished":  punished,
		}),
	})
}

func (s *Service) auditOAuthUserInfo(ctx context.Context, principal *ssofacade.AccessTokenPrincipal, reasonCode string, scopeCount int, returnedClaims []string, err error) {
	if principal == nil {
		return
	}
	result := ssoAuditResultSuccess
	if err != nil || reasonCode != "accessed" {
		result = ssoAuditResultFailure
	}
	if strings.TrimSpace(reasonCode) == "" {
		reasonCode = "accessed"
	}
	detail := map[string]any{
		"scopeCount": scopeCount,
	}
	if len(returnedClaims) > 0 {
		detail["claimsReturned"] = returnedClaims
	}
	s.insertSSOAuditLog(ctx, domain.AuditLog{
		EventType:  ssoAuditEventUserInfoAccessed,
		ClientID:   strings.TrimSpace(principal.ClientID),
		UserID:     int64Ptr(principal.UserID),
		SessionID:  strings.TrimSpace(principal.SessionID),
		Result:     result,
		ReasonCode: reasonCode,
		DetailJSON: metadataJSON(detail),
	})
}

func (s *Service) auditOAuthTokenIntrospection(ctx context.Context, clientID, tokenTypeHint string, result map[string]any, err error) {
	auditResult := ssoAuditResultSuccess
	reasonCode := "inactive"
	if err != nil {
		auditResult = ssoAuditResultFailure
		reasonCode = "invalid_client"
	} else if boolValue(result["active"]) {
		reasonCode = "active"
	}
	var userID *int64
	if id := int64Value(firstClaim(result, "uid", "sub")); id > 0 {
		userID = &id
	}
	tokenType := strings.TrimSpace(stringValue(result["token_type"]))
	s.insertSSOAuditLog(ctx, domain.AuditLog{
		EventType:  ssoAuditEventTokenIntrospected,
		ClientID:   strings.TrimSpace(clientID),
		UserID:     userID,
		SessionID:  strings.TrimSpace(stringValue(result["sid"])),
		Result:     auditResult,
		ReasonCode: reasonCode,
		DetailJSON: metadataJSON(map[string]any{
			"tokenTypeHint": normalizeOAuthTokenTypeHint(tokenTypeHint),
			"tokenType":     tokenType,
			"active":        boolValue(result["active"]),
		}),
	})
}

func (s *Service) insertSSOAuditLog(ctx context.Context, item domain.AuditLog) {
	if s == nil || s.repository == nil {
		return
	}
	if strings.TrimSpace(item.TraceID) == "" {
		item.TraceID = xcontext.TraceIDFromContext(ctx)
	}
	_ = s.repository.InsertAuditLog(ctx, item)
}

func normalizeOAuthTokenTypeHint(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "access_token", "refresh_token":
		return normalized
	default:
		return "unspecified"
	}
}

func boolValue(value any) bool {
	typed, ok := value.(bool)
	return ok && typed
}

func int64Ptr(value int64) *int64 {
	return &value
}

func claimsReturned(result map[string]any) []string {
	if len(result) == 0 {
		return nil
	}
	claims := make([]string, 0, 3)
	if _, ok := result["sub"]; ok {
		claims = append(claims, "sub")
	}
	if _, ok := result["preferred_username"]; ok {
		claims = append(claims, "profile")
	}
	if _, ok := result["email"]; ok {
		claims = append(claims, "email")
	}
	return claims
}

func (s *Service) introspectAccessToken(ctx context.Context, clientID, token string) map[string]any {
	principal, err := s.parseAccessTokenForRevocation(ctx, token)
	if err != nil || principal == nil || strings.TrimSpace(principal.ClientID) != strings.TrimSpace(clientID) {
		return nil
	}
	active, err := s.ValidateAccessToken(ctx, token)
	if err != nil || active == nil || strings.TrimSpace(active.ClientID) != strings.TrimSpace(clientID) {
		return map[string]any{"active": false}
	}
	return accessTokenIntrospection(active)
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (s *Service) introspectRefreshToken(ctx context.Context, clientID, token string) map[string]any {
	principal, err := s.parseRefreshToken(ctx, token)
	if err != nil || principal == nil || strings.TrimSpace(principal.ClientID) != strings.TrimSpace(clientID) {
		return nil
	}
	hash := ssohelper.BuildTokenHash(token)
	family, err := s.repository.FindRefreshFamilyByCurrentHash(ctx, hash)
	if err != nil || family == nil || strings.TrimSpace(family.ClientID) != strings.TrimSpace(clientID) {
		return map[string]any{"active": false}
	}
	now := time.Now().UTC()
	if family.Status != domain.RefreshFamilyStatusActive || family.ExpiresAt.Before(now) || (family.RevokedAt != nil && !family.RevokedAt.IsZero()) {
		return map[string]any{"active": false}
	}
	session, err := s.ResolveActiveSession(ctx, family.SessionID)
	if err != nil || session == nil || session.UserID != principal.UserID || session.ClientID != principal.ClientID {
		return map[string]any{"active": false}
	}
	return refreshTokenIntrospection(principal)
}

func (s *Service) revokeRefreshTokenForClient(ctx context.Context, clientID, token string) (string, error) {
	hash := ssohelper.BuildTokenHash(token)
	family, err := s.repository.FindRefreshFamilyByCurrentHash(ctx, hash)
	if err != nil {
		return "", err
	}
	if family != nil {
		if strings.TrimSpace(family.ClientID) != strings.TrimSpace(clientID) {
			return "", nil
		}
		return family.SessionID, s.revokeSessionAndRefreshFamilies(ctx, family.SessionID)
	}
	family, err = s.repository.FindRefreshFamilyByPreviousHash(ctx, hash)
	if err != nil {
		return "", err
	}
	if family != nil {
		if strings.TrimSpace(family.ClientID) != strings.TrimSpace(clientID) {
			return "", nil
		}
		return family.SessionID, s.revokeSessionAndRefreshFamilies(ctx, family.SessionID)
	}
	return "", nil
}

func isInvalidOAuthTokenError(err error) bool {
	return err != nil && apperrors.From(err).Kind() == apperrors.KindAuth
}

func (s *Service) revokeSessionAndRefreshFamilies(ctx context.Context, sessionID string) error {
	_, err := s.revokeSessionAndRefreshFamiliesWithResult(ctx, sessionID)
	return err
}

func (s *Service) revokeSessionAndRefreshFamiliesWithResult(ctx context.Context, sessionID string) (bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if s.validityInvalidations != nil && s.validityInvalidations.Enabled() && store.SQLXFromContext(ctx) == nil {
		var revoked bool
		err := s.requireActiveSessionValidityTransaction(ctx, func(txCtx context.Context) error {
			var callErr error
			revoked, callErr = s.revokeSessionAndRefreshFamiliesWithResult(txCtx, sessionID)
			return callErr
		})
		return revoked, err
	}
	if s.validityInvalidations != nil && s.validityInvalidations.Enabled() {
		item, err := s.repository.FindSessionBySessionID(ctx, sessionID)
		if err != nil {
			return false, err
		}
		if item != nil {
			if err := s.registerActiveSessionValidityTargets(ctx, []domain.Session{*item}); err != nil {
				return false, err
			}
		}
	}
	now := time.Now().UTC()
	if err := s.repository.RevokeRefreshFamiliesBySessionID(ctx, sessionID, now); err != nil {
		return false, err
	}
	return s.repository.RevokeSession(ctx, sessionID, now)
}

// refreshReuseCommitted is an internal control signal, never returned to an
// OAuth caller. The outer transaction boundary recognizes it, commits the
// punishment plus v2 outbox event, then returns Unauthorized to the request.
type refreshReuseCommitted struct{}

func (*refreshReuseCommitted) Error() string { return "refresh token reuse punishment committed" }

type tokenBundle struct {
	AccessToken             string
	IDToken                 string
	RefreshToken            string
	TokenType               string
	ExpiresInSeconds        int64
	Scope                   string
	RefreshTokenExpiresAt   *time.Time
	RefreshTokenBodyAllowed bool
}

func (s *Service) buildCodeRedirect(snapshot *domain.AuthorizationSessionSnapshot, code string) string {
	u, err := url.Parse(snapshot.RedirectURI)
	if err != nil {
		return snapshot.RedirectURI
	}
	query := u.Query()
	query.Set("code", code)
	if strings.TrimSpace(snapshot.State) != "" {
		query.Set("state", snapshot.State)
	}
	u.RawQuery = query.Encode()
	return u.String()
}

func (s *Service) issueAccessToken(ctx context.Context, client *domain.Client, session *domain.Session, scopes []string) (string, time.Time, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(maxInt(client.AccessTokenTTLSec, 1800)) * time.Second)
	token, err := s.jwt.Sign(ctx, map[string]any{
		"iss":        s.resolveIssuer(),
		"sub":        fmt.Sprintf("%d", session.UserID),
		"aud":        []string{client.ClientID},
		"jti":        s.newID("atk"),
		"iat":        now.Unix(),
		"exp":        expiresAt.Unix(),
		"token_type": "access_token",
		"uid":        fmt.Sprintf("%d", session.UserID),
		"sid":        session.SessionID,
		"client_id":  client.ClientID,
		"scope":      strings.Join(scopes, " "),
		"acr":        session.ACR,
		"amr":        session.AMR,
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

func (s *Service) issueIDToken(ctx context.Context, client *domain.Client, session *domain.Session, scopes []string, nonce string) (string, error) {
	if !hasScope(scopes, "openid") {
		return "", nil
	}
	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(maxInt(client.AccessTokenTTLSec, 1800)) * time.Second)
	claims := map[string]any{
		"iss":       s.resolveIssuer(),
		"sub":       fmt.Sprintf("%d", session.UserID),
		"aud":       []string{client.ClientID},
		"jti":       s.newID("idt"),
		"iat":       now.Unix(),
		"exp":       expiresAt.Unix(),
		"typ":       "id_token",
		"sid":       session.SessionID,
		"acr":       session.ACR,
		"amr":       session.AMR,
		"auth_time": session.LoginAt.UTC().Unix(),
	}
	if strings.TrimSpace(nonce) != "" {
		claims["nonce"] = strings.TrimSpace(nonce)
	}
	if s.profiles != nil {
		if profile, err := s.profiles.GetProfileByUserID(ctx, session.UserID); err == nil && profile != nil {
			if strings.TrimSpace(profile.AccountName) != "" {
				claims["preferred_username"] = profile.AccountName
			}
			if strings.TrimSpace(profile.NickName) != "" {
				claims["name"] = profile.NickName
			}
			if hasScope(scopes, "email") && strings.TrimSpace(profile.Email) != "" {
				claims["email"] = profile.Email
				claims["email_verified"] = profile.EmailVerified
			}
		}
	}
	return s.jwt.Sign(ctx, claims)
}

func (s *Service) issueRefreshToken(ctx context.Context, client *domain.Client, session *domain.Session, scopes []string) (string, time.Time, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(maxInt(client.RefreshTokenTTLSec, 2592000)) * time.Second)
	token, err := s.jwt.Sign(ctx, map[string]any{
		"iss":        s.resolveIssuer(),
		"sub":        fmt.Sprintf("%d", session.UserID),
		"aud":        []string{client.ClientID},
		"jti":        s.newID("rft"),
		"iat":        now.Unix(),
		"exp":        expiresAt.Unix(),
		"token_type": "refresh_token",
		"uid":        fmt.Sprintf("%d", session.UserID),
		"sid":        session.SessionID,
		"client_id":  client.ClientID,
		"scope":      strings.Join(scopes, " "),
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

type refreshTokenPrincipal struct {
	TokenID   string
	UserID    int64
	Subject   string
	ClientID  string
	SessionID string
	Scopes    []string
	ExpiresAt *time.Time
}

func (s *Service) parseRefreshToken(ctx context.Context, refreshToken string) (*refreshTokenPrincipal, error) {
	if s.jwt == nil {
		return nil, apperrors.System("SSO jwt service未配置")
	}
	claims, err := s.jwt.Verify(ctx, strings.TrimSpace(refreshToken))
	if err != nil {
		return nil, apperrors.Unauthorized("refresh token 无效")
	}
	if strings.TrimSpace(stringValue(firstClaim(claims, "token_type", "typ"))) != "refresh_token" {
		return nil, apperrors.Unauthorized("refresh token 无效")
	}
	if err := s.validateTokenIssuer(claims, "refresh token 无效"); err != nil {
		return nil, err
	}
	userID, subject, ok := tokenUserClaims(claims)
	sessionID := strings.TrimSpace(stringValue(claims["sid"]))
	clientID := strings.TrimSpace(stringValue(firstClaim(claims, "client_id", "cid")))
	exp := unixTime(claims["exp"])
	if !ok || sessionID == "" || clientID == "" || exp == nil || !exp.After(time.Now().UTC()) {
		return nil, apperrors.Unauthorized("refresh token 无效")
	}
	if !claimContainsString(claims["aud"], clientID) {
		return nil, apperrors.Unauthorized("refresh token 无效")
	}
	return &refreshTokenPrincipal{
		TokenID:   strings.TrimSpace(stringValue(claims["jti"])),
		UserID:    userID,
		Subject:   subject,
		ClientID:  clientID,
		SessionID: sessionID,
		Scopes:    scopeSlice(firstClaim(claims, "scope", "scp")),
		ExpiresAt: exp,
	}, nil
}

func (s *Service) parseAccessTokenForRevocation(ctx context.Context, accessToken string) (*ssofacade.AccessTokenPrincipal, error) {
	if s.jwt == nil {
		return nil, apperrors.System("SSO jwt service未配置")
	}
	claims, err := s.jwt.Verify(ctx, strings.TrimSpace(accessToken))
	if err != nil {
		return nil, apperrors.Unauthorized("access token 非法或已失效")
	}
	if strings.TrimSpace(stringValue(firstClaim(claims, "token_type", "typ"))) != "access_token" {
		return nil, apperrors.Unauthorized("access token 非法或已失效")
	}
	if err := s.validateTokenIssuer(claims, "access token 非法或已失效"); err != nil {
		return nil, err
	}
	exp := unixTime(claims["exp"])
	userID, subject, ok := tokenUserClaims(claims)
	clientID := strings.TrimSpace(stringValue(firstClaim(claims, "client_id", "cid")))
	sessionID := strings.TrimSpace(stringValue(claims["sid"]))
	if !ok || clientID == "" || sessionID == "" || exp == nil || !exp.After(time.Now().UTC()) || !claimContainsString(claims["aud"], clientID) {
		return nil, apperrors.Unauthorized("access token 非法或已失效")
	}
	iat := unixTime(claims["iat"])
	return &ssofacade.AccessTokenPrincipal{
		TokenID:   strings.TrimSpace(stringValue(claims["jti"])),
		UserID:    userID,
		Subject:   subject,
		ClientID:  clientID,
		SessionID: sessionID,
		Scopes:    scopeSlice(firstClaim(claims, "scope", "scp")),
		ACR:       strings.TrimSpace(stringValue(claims["acr"])),
		AMR:       stringSlice(claims["amr"]),
		IssuedAt:  iat,
		ExpiresAt: exp,
	}, nil
}

func accessTokenIntrospection(principal *ssofacade.AccessTokenPrincipal) map[string]any {
	result := map[string]any{
		"active":     true,
		"token_type": "access_token",
		"client_id":  principal.ClientID,
		"sub":        principal.Subject,
		"uid":        principal.UserID,
		"sid":        principal.SessionID,
		"jti":        principal.TokenID,
	}
	if len(principal.Scopes) > 0 {
		result["scope"] = strings.Join(principal.Scopes, " ")
	}
	if principal.IssuedAt != nil && !principal.IssuedAt.IsZero() {
		result["iat"] = principal.IssuedAt.Unix()
	}
	if principal.ExpiresAt != nil && !principal.ExpiresAt.IsZero() {
		result["exp"] = principal.ExpiresAt.Unix()
	}
	if strings.TrimSpace(principal.ACR) != "" {
		result["acr"] = principal.ACR
	}
	if len(principal.AMR) > 0 {
		result["amr"] = principal.AMR
	}
	return result
}

func refreshTokenIntrospection(principal *refreshTokenPrincipal) map[string]any {
	result := map[string]any{
		"active":     true,
		"token_type": "refresh_token",
		"client_id":  principal.ClientID,
		"sub":        principal.Subject,
		"uid":        principal.UserID,
		"sid":        principal.SessionID,
		"jti":        principal.TokenID,
	}
	if len(principal.Scopes) > 0 {
		result["scope"] = strings.Join(principal.Scopes, " ")
	}
	if principal.ExpiresAt != nil && !principal.ExpiresAt.IsZero() {
		result["exp"] = principal.ExpiresAt.Unix()
	}
	return result
}

func (s *Service) requireClient(ctx context.Context, clientID string) (*domain.Client, error) {
	item, err := s.repository.FindClient(ctx, strings.TrimSpace(clientID))
	if err != nil {
		return nil, err
	}
	if item == nil || item.Status != domain.ClientStatusActive {
		return nil, apperrors.Unauthorized("客户端不存在或已禁用")
	}
	return item, nil
}

func (s *Service) isRedirectAllowed(client *domain.Client, redirectURI string) bool {
	redirectURI = strings.TrimSpace(redirectURI)
	for _, item := range client.RedirectURIs {
		if strings.TrimSpace(item) == redirectURI {
			return true
		}
	}
	return false
}

func (s *Service) normalizeScopes(client *domain.Client, requested []string) ([]string, error) {
	cleaned := make([]string, 0, len(requested))
	allow := make(map[string]struct{}, len(client.Scopes))
	for _, item := range client.Scopes {
		allow[strings.TrimSpace(item)] = struct{}{}
	}
	if len(requested) == 0 {
		return nil, apperrors.Params("scope 不能为空")
	}
	for _, item := range requested {
		scope := strings.TrimSpace(item)
		if scope == "" {
			continue
		}
		if _, ok := allow[scope]; !ok {
			return nil, apperrors.Params("scope 不被允许")
		}
		cleaned = append(cleaned, scope)
	}
	if len(cleaned) == 0 {
		return nil, apperrors.Params("scope 不能为空")
	}
	if !hasScope(cleaned, "openid") {
		return nil, apperrors.Params("OIDC 请求必须包含 openid")
	}
	sort.Strings(cleaned)
	return cleaned, nil
}

func (s *Service) authenticateClient(client *domain.Client, secret string) error {
	if err := validateClientAuthPolicy(client); err != nil {
		return err
	}
	method := strings.ToLower(strings.TrimSpace(client.ClientAuthMethod))
	switch method {
	case "", "none":
		if strings.TrimSpace(secret) != "" {
			return apperrors.Unauthorized("client 认证失败")
		}
		return nil
	case "client_secret_basic":
		if strings.TrimSpace(secret) == "" {
			return apperrors.Unauthorized("client 认证失败")
		}
		for _, hash := range client.SecretHashes {
			if s.password != nil && s.password.Verify(context.Background(), secret, hash) == nil {
				return nil
			}
		}
		return apperrors.Unauthorized("client 认证失败")
	default:
		return apperrors.Params("client 认证方式不支持")
	}
}

func validateClientAuthPolicy(client *domain.Client) error {
	if client == nil {
		return apperrors.Unauthorized("client 认证失败")
	}
	clientType := normalizeClientType(client.ClientType)
	method := strings.ToLower(strings.TrimSpace(client.ClientAuthMethod))
	if method == "" {
		method = "none"
	}
	switch clientType {
	case "":
		return nil
	case "public":
		if method != "none" {
			return apperrors.Unauthorized("client 认证失败")
		}
	case "confidential":
		if method == "none" {
			return apperrors.Unauthorized("client 认证失败")
		}
	default:
		return apperrors.Params("client 类型不支持")
	}
	return nil
}

func normalizeClientType(clientType string) string {
	switch strings.ToUpper(strings.TrimSpace(clientType)) {
	case "FIRST_PARTY_SPA", "THIRD_PARTY_SPA", "PUBLIC":
		return "public"
	case "CONFIDENTIAL_WEB", "CONFIDENTIAL":
		return "confidential"
	default:
		return strings.ToLower(strings.TrimSpace(clientType))
	}
}

func (s *Service) validateAuthorizeRequest(client *domain.Client, responseType, redirectURI string, scopes []string, codeChallenge, codeChallengeMethod, prompt string) error {
	if err := validateClientAuthPolicy(client); err != nil {
		return err
	}
	if strings.TrimSpace(responseType) == "" {
		return apperrors.Params("缺少 response_type")
	}
	if strings.TrimSpace(responseType) != "code" {
		return apperrors.Params("仅支持 response_type=code")
	}
	if !clientAllowsGrant(client, "authorization_code") {
		return apperrors.Params("当前客户端不允许 authorization_code 授权")
	}
	if !s.isRedirectAllowed(client, redirectURI) {
		return apperrors.Params("redirect_uri 不被允许")
	}
	if strings.TrimSpace(prompt) != "" {
		values := strings.Fields(strings.TrimSpace(prompt))
		hasNone := false
		for _, item := range values {
			if item == "none" {
				hasNone = true
				break
			}
		}
		if hasNone && len(values) > 1 {
			return apperrors.Params("prompt=none 不能与其他值组合")
		}
	}
	codeChallenge = strings.TrimSpace(codeChallenge)
	codeChallengeMethod = strings.TrimSpace(codeChallengeMethod)
	pkceRequired := clientRequiresPKCE(client)
	if pkceRequired && codeChallenge == "" {
		return apperrors.Params("当前客户端必须携带 PKCE")
	}
	if codeChallenge != "" && !isValidPKCEVerifier(codeChallenge) {
		return apperrors.Params("PKCE code_challenge 格式不合法")
	}
	if codeChallenge != "" && codeChallengeMethod == "" {
		return apperrors.Params("PKCE 必须显式使用 S256 code_challenge_method")
	}
	if codeChallengeMethod != "" && !strings.EqualFold(codeChallengeMethod, "S256") {
		return apperrors.Params("仅支持 S256 code_challenge_method")
	}
	return nil
}

func clientAllowsGrant(client *domain.Client, grant string) bool {
	if client == nil {
		return false
	}
	grant = strings.ToLower(strings.TrimSpace(grant))
	for _, item := range client.GrantTypes {
		if strings.ToLower(strings.TrimSpace(item)) == grant {
			return true
		}
	}
	return false
}

func clientRequiresPKCE(client *domain.Client) bool {
	if client == nil {
		return false
	}
	if client.RequirePKCE {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(client.ClientAuthMethod), "none")
}

func (s *Service) validateTokenIssuer(claims map[string]any, message string) error {
	expected := strings.TrimSpace(s.resolveIssuer())
	actual := strings.TrimSpace(stringValue(claims["iss"]))
	if expected == "" || actual == "" || actual != expected {
		return apperrors.Unauthorized(message)
	}
	return nil
}

func tokenUserClaims(claims map[string]any) (int64, string, bool) {
	uid := int64Value(claims["uid"])
	subject := strings.TrimSpace(stringValue(claims["sub"]))
	if uid <= 0 || subject == "" {
		return 0, "", false
	}
	subjectID := int64Value(subject)
	if subjectID <= 0 || subjectID != uid {
		return 0, "", false
	}
	return uid, subject, true
}

func claimContainsString(value any, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) == target
	case []string:
		for _, item := range typed {
			if strings.TrimSpace(item) == target {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if strings.TrimSpace(stringValue(item)) == target {
				return true
			}
		}
	default:
		return strings.TrimSpace(stringValue(value)) == target
	}
	return false
}

func (s *Service) resolveIssuer() string {
	if strings.TrimSpace(s.cfg.Issuer) != "" {
		return strings.TrimSpace(s.cfg.Issuer)
	}
	return strings.TrimRight(strings.TrimSpace(s.cfg.BaseURL), "/")
}

func (s *Service) newID(prefix string) string {
	if s.idGen == nil {
		return prefix + "_" + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
	}
	return fmt.Sprintf("%s_%d", prefix, s.idGen.NextID())
}

func (s *Service) nextID() int64 {
	if s.idGen == nil {
		return time.Now().UTC().UnixNano()
	}
	return s.idGen.NextID()
}

func toFacadeAuthSession(item *domain.AuthorizationSessionSnapshot) *ssofacade.AuthorizationSessionSnapshot {
	if item == nil {
		return nil
	}
	return &ssofacade.AuthorizationSessionSnapshot{
		LoginTransactionID:  item.LoginTransactionID,
		ClientID:            item.ClientID,
		RedirectURI:         item.RedirectURI,
		Scopes:              append([]string(nil), item.Scopes...),
		State:               item.State,
		Nonce:               item.Nonce,
		CodeChallenge:       item.CodeChallenge,
		CodeChallengeMethod: item.CodeChallengeMethod,
		DeviceID:            item.DeviceID,
		TenantID:            item.TenantID,
		LoginIP:             item.LoginIP,
		UserAgent:           item.UserAgent,
		TraceID:             item.TraceID,
		CreatedAt:           item.CreatedAt,
		ExpiresAt:           item.ExpiresAt,
	}
}

func toSessionRecords(items []domain.Session) []ssofacade.SessionRecord {
	result := make([]ssofacade.SessionRecord, 0, len(items))
	for _, item := range items {
		record := ssofacade.SessionRecord{
			SessionID:            item.SessionID,
			UserID:               item.UserID,
			ClientID:             item.ClientID,
			DeviceID:             item.DeviceID,
			LoginIP:              item.LoginIP,
			UserAgent:            item.UserAgent,
			ACR:                  item.ACR,
			AMR:                  append([]string(nil), item.AMR...),
			LoginMethod:          item.LoginMethod,
			ExternalProviderCode: item.ExternalProviderCode,
			ExternalIdentityID:   item.ExternalIdentityID,
			LoginAt:              &item.LoginAt,
			LastAccessAt:         item.LastAccessAt,
			ExpiresAt:            &item.ExpiresAt,
			RevokedAt:            item.RevokedAt,
			MetadataJSON:         item.MetadataJSON,
		}
		switch item.Status {
		case domain.SessionStatusRevoked:
			record.Status = "REVOKED"
		case domain.SessionStatusExpired:
			record.Status = "EXPIRED"
		default:
			record.Status = "ACTIVE"
		}
		result = append(result, record)
	}
	return result
}

func BuildClientAdminCreateOperationBinding(request ssofacade.ClientAdminSaveRequest) (string, error) {
	item, err := normalizeClientAdminSaveRequest("", request)
	if err != nil {
		return "", err
	}
	hash, err := clientAdminPayloadHash(item)
	if err != nil {
		return "", err
	}
	return clientAdminOperationBinding(item.ClientID, challengedomain.BusinessActionSSOClientCreate, hash), nil
}

func BuildClientAdminUpdateOperationBinding(clientID string, request ssofacade.UpdateClientAdminRequest) (string, error) {
	item, err := normalizeClientAdminUpdateRequest(clientID, request)
	if err != nil {
		return "", err
	}
	hash, err := clientAdminPayloadHash(item)
	if err != nil {
		return "", err
	}
	return clientAdminOperationBinding(item.ClientID, challengedomain.BusinessActionSSOClientUpdate, hash), nil
}

func BuildClientStatusOperationBinding(clientID string, request ssofacade.ClientStatusRequest) (string, error) {
	clientID = strings.TrimSpace(clientID)
	if err := validateClientID(clientID); err != nil {
		return "", err
	}
	if request.Status != domain.ClientStatusActive && request.Status != domain.ClientStatusDisabled {
		return "", apperrors.Params("客户端状态无效")
	}
	revokeActiveSessions := true
	if request.RevokeActiveSessions != nil {
		revokeActiveSessions = *request.RevokeActiveSessions
	}
	payload := clientStatusBindingPayload{
		ClientID:             clientID,
		Status:               request.Status,
		Reason:               strings.TrimSpace(request.Reason),
		RevokeActiveSessions: revokeActiveSessions,
	}
	raw, err := sonic.MarshalString(payload)
	if err != nil {
		return "", apperrors.System("SSO客户端状态payload序列化失败")
	}
	return clientAdminOperationBinding(clientID, challengedomain.BusinessActionSSOClientStatusChange, sha256Hex(raw)), nil
}

func BuildClientRedirectURIsOperationBinding(clientID string, request ssofacade.ClientRedirectURIUpdateRequest) (string, error) {
	clientID = strings.TrimSpace(clientID)
	if err := validateClientID(clientID); err != nil {
		return "", err
	}
	if err := validateClientRedirectURIInputCounts(request); err != nil {
		return "", err
	}
	redirects := normalizeRedirectBindingValues(request.RedirectURIs)
	postLogouts := normalizeRedirectBindingValues(request.PostLogoutRedirectURIs)
	if len(redirects) == 0 {
		return "", apperrors.Params("redirectUris不能为空")
	}
	payload := struct {
		ClientID               string   `json:"clientId"`
		RedirectURIs           []string `json:"redirectUris"`
		PostLogoutRedirectURIs []string `json:"postLogoutRedirectUris"`
	}{
		ClientID:               clientID,
		RedirectURIs:           redirects,
		PostLogoutRedirectURIs: postLogouts,
	}
	raw, err := sonic.MarshalString(payload)
	if err != nil {
		return "", apperrors.System("SSO客户端回调地址payload序列化失败")
	}
	return clientAdminOperationBinding(clientID, challengedomain.BusinessActionSSOClientRedirectEdit, sha256Hex(raw)), nil
}

func BuildClientSecretGenerateOperationBinding(clientID string, request ssofacade.ClientSecretGenerateRequest) (string, error) {
	clientID = strings.TrimSpace(clientID)
	if err := validateClientID(clientID); err != nil {
		return "", err
	}
	expiresInDays, err := validateSecretExpiresInDays(request.ExpiresInDays)
	if err != nil {
		return "", err
	}
	payload := clientSecretGenerateBindingPayload{
		ClientID:      clientID,
		ExpiresInDays: expiresInDays,
		Reason:        strings.TrimSpace(request.Reason),
	}
	raw, err := sonic.MarshalString(payload)
	if err != nil {
		return "", apperrors.System("SSO客户端密钥payload序列化失败")
	}
	return clientAdminOperationBinding(clientID, challengedomain.BusinessActionSSOClientSecretGenerate, sha256Hex(raw)), nil
}

func BuildClientSecretStatusOperationBinding(clientID string, secretID int64, request ssofacade.ClientSecretStatusRequest) (string, error) {
	clientID = strings.TrimSpace(clientID)
	if err := validateClientID(clientID); err != nil {
		return "", err
	}
	if secretID <= 0 {
		return "", apperrors.Params("secretId无效")
	}
	if request.Status != 0 && request.Status != domain.ClientStatusDisabled {
		return "", apperrors.Params("密钥状态无效")
	}
	payload := clientSecretStatusBindingPayload{
		ClientID:            clientID,
		SecretID:            secretID,
		Status:              domain.ClientStatusDisabled,
		Reason:              strings.TrimSpace(request.Reason),
		AllowNoActiveSecret: request.AllowNoActiveSecret,
	}
	raw, err := sonic.MarshalString(payload)
	if err != nil {
		return "", apperrors.System("SSO客户端密钥状态payload序列化失败")
	}
	return clientAdminOperationBinding(clientID, challengedomain.BusinessActionSSOClientSecretDisable, sha256Hex(raw)), nil
}

func normalizeClientAdminSaveRequest(pathClientID string, request ssofacade.ClientAdminSaveRequest) (*domain.Client, error) {
	clientID := strings.TrimSpace(pathClientID)
	if clientID == "" {
		clientID = strings.TrimSpace(request.ClientID)
	}
	if err := validateClientID(clientID); err != nil {
		return nil, err
	}
	clientType, err := normalizeClientTypeForAdmin(request.ClientType)
	if err != nil {
		return nil, err
	}
	authMethod, err := normalizeClientAuthMethodForAdmin(request.ClientAuthMethod)
	if err != nil {
		return nil, err
	}
	if clientType == "PUBLIC" && authMethod != "none" {
		return nil, apperrors.Params("公共客户端必须使用none认证方式")
	}
	if clientType == "CONFIDENTIAL" && authMethod != "client_secret_basic" {
		return nil, apperrors.Params("机密客户端必须使用client_secret_basic认证方式")
	}
	if clientType == "PUBLIC" && !request.RequirePKCE {
		return nil, apperrors.Params("公共客户端必须启用PKCE")
	}
	grantTypes, err := normalizeGrantTypesForAdmin(request.GrantTypes)
	if err != nil {
		return nil, err
	}
	scopes, err := normalizeScopesForAdmin(request.Scopes)
	if err != nil {
		return nil, err
	}
	accessTTL, refreshTTL, err := validateClientTTL(request.AccessTokenTTLSec, request.RefreshTokenTTLSec)
	if err != nil {
		return nil, err
	}
	clientName := strings.TrimSpace(request.ClientName)
	if clientName == "" {
		return nil, apperrors.Params("clientName不能为空")
	}
	return &domain.Client{
		ClientID:           clientID,
		ClientName:         clientName,
		ClientType:         clientType,
		ClientAuthMethod:   authMethod,
		GrantTypes:         grantTypes,
		Scopes:             scopes,
		RequirePKCE:        request.RequirePKCE,
		RequireConsent:     request.RequireConsent,
		TrustedFirstParty:  request.TrustedFirstParty,
		AccessTokenTTLSec:  accessTTL,
		RefreshTokenTTLSec: refreshTTL,
		MetadataJSON:       strings.TrimSpace(request.MetadataJSON),
	}, nil
}

func normalizeClientAdminUpdateRequest(clientID string, request ssofacade.UpdateClientAdminRequest) (*domain.Client, error) {
	return normalizeClientAdminSaveRequest(clientID, ssofacade.ClientAdminSaveRequest{
		ClientID:           clientID,
		ClientName:         request.ClientName,
		ClientType:         request.ClientType,
		ClientAuthMethod:   request.ClientAuthMethod,
		GrantTypes:         request.GrantTypes,
		Scopes:             request.Scopes,
		RequirePKCE:        request.RequirePKCE,
		RequireConsent:     request.RequireConsent,
		TrustedFirstParty:  request.TrustedFirstParty,
		AccessTokenTTLSec:  request.AccessTokenTTLSec,
		RefreshTokenTTLSec: request.RefreshTokenTTLSec,
		MetadataJSON:       request.MetadataJSON,
	})
}

func normalizeClientRedirectURIsForAdmin(request ssofacade.ClientRedirectURIUpdateRequest, profile string) ([]domain.ClientRedirectURI, error) {
	if err := validateClientRedirectURIInputCounts(request); err != nil {
		return nil, err
	}
	redirects := make([]string, 0, len(request.RedirectURIs))
	seenRedirects := make(map[string]struct{}, len(request.RedirectURIs))
	for _, item := range request.RedirectURIs {
		value := strings.TrimSpace(item)
		if err := validateRedirectURIForClient(value, profile); err != nil {
			return nil, err
		}
		if _, ok := seenRedirects[value]; ok {
			continue
		}
		seenRedirects[value] = struct{}{}
		redirects = append(redirects, value)
	}
	if len(redirects) == 0 {
		return nil, apperrors.Params("redirectUris不能为空")
	}
	postLogouts := make([]string, 0, len(request.PostLogoutRedirectURIs))
	seenPostLogouts := make(map[string]struct{}, len(request.PostLogoutRedirectURIs))
	for _, item := range request.PostLogoutRedirectURIs {
		value := strings.TrimSpace(item)
		if err := validatePostLogoutRedirectURIForClient(value, profile); err != nil {
			return nil, err
		}
		if _, ok := seenPostLogouts[value]; ok {
			continue
		}
		seenPostLogouts[value] = struct{}{}
		postLogouts = append(postLogouts, value)
	}
	sort.Strings(redirects)
	sort.Strings(postLogouts)
	if len(postLogouts) > len(redirects) {
		return nil, apperrors.Params("postLogoutRedirectUris数量不能超过redirectUris")
	}
	items := make([]domain.ClientRedirectURI, 0, len(redirects))
	for index, redirect := range redirects {
		item := domain.ClientRedirectURI{RedirectURI: redirect, Status: domain.ClientStatusActive}
		if index < len(postLogouts) {
			item.PostLogoutRedirectURI = postLogouts[index]
		}
		items = append(items, item)
	}
	return items, nil
}

func validateClientRedirectURIInputCounts(request ssofacade.ClientRedirectURIUpdateRequest) error {
	if len(request.RedirectURIs) > clientRedirectURIInputMax {
		return apperrors.Params("redirectUris数量超过限制")
	}
	if len(request.PostLogoutRedirectURIs) > clientRedirectURIInputMax {
		return apperrors.Params("postLogoutRedirectUris数量超过限制")
	}
	if len(request.RedirectURIs)+len(request.PostLogoutRedirectURIs) > clientRedirectURIInputMax {
		return apperrors.Params("回调地址总数量超过限制")
	}
	return nil
}

func normalizeRedirectBindingValues(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, item := range values {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func validateRedirectURIForClient(raw string, profile string) error {
	return validateClientManagedURI(raw, profile, false)
}

func validatePostLogoutRedirectURIForClient(raw string, profile string) error {
	return validateClientManagedURI(raw, profile, true)
}

func validateClientManagedURI(raw string, profile string, postLogout bool) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return apperrors.Params("redirect URI不能为空")
	}
	if strings.Contains(value, "*") {
		return apperrors.Params("redirect URI不允许通配符")
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return apperrors.Params("redirect URI不允许控制字符")
		}
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(value, "//") {
		return apperrors.Params("redirect URI必须包含安全scheme")
	}
	if strings.Contains(lower, "%0d") || strings.Contains(lower, "%0a") {
		return apperrors.Params("redirect URI不允许CRLF")
	}
	if hasEncodedSlashOrBackslashInAuthority(value) {
		return apperrors.Params("redirect URI host不允许编码绕过")
	}
	if strings.Contains(value, "\\") {
		return apperrors.Params("redirect URI不允许反斜杠")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return apperrors.Params("redirect URI格式不正确")
	}
	if parsed.Fragment != "" {
		return apperrors.Params("redirect URI不允许fragment")
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return apperrors.Params("redirect URI必须是绝对URI")
	}
	if parsed.User != nil {
		return apperrors.Params("redirect URI不允许userinfo")
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" || strings.Contains(host, "..") || strings.Contains(host, "/") || strings.Contains(host, "\\") {
		return apperrors.Params("redirect URI host不合法")
	}
	if strings.HasSuffix(host, ".") && !isLocalhostHost(host) {
		return apperrors.Params("redirect URI host不合法")
	}
	scheme := strings.ToLower(parsed.Scheme)
	switch scheme {
	case "https":
		return nil
	case "http":
		if isDevProfile(profile) && isLocalhostHost(host) && parsed.Port() != "" {
			return nil
		}
	}
	if postLogout {
		return apperrors.Params("post logout redirect URI必须使用HTTPS，dev仅允许带端口localhost")
	}
	return apperrors.Params("redirect URI必须使用HTTPS，dev仅允许带端口localhost")
}

func hasEncodedSlashOrBackslashInAuthority(value string) bool {
	authority := value
	if idx := strings.Index(authority, "://"); idx >= 0 {
		authority = authority[idx+3:]
	}
	if idx := strings.IndexAny(authority, "/?#"); idx >= 0 {
		authority = authority[:idx]
	}
	authority = strings.ToLower(authority)
	return strings.Contains(authority, "%2f") || strings.Contains(authority, "%5c") || strings.Contains(authority, "%252f") || strings.Contains(authority, "%255c")
}

func isDevProfile(profile string) bool {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "dev", "local", "test":
		return true
	default:
		return false
	}
}

func isLocalhostHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	return host == "localhost" || host == "127.0.0.1" || strings.HasSuffix(host, ".localhost")
}

func (s *Service) redirectValidationProfile() string {
	candidates := []string{s.cfg.BaseURL, s.cfg.Issuer}
	for _, candidate := range candidates {
		parsed, err := url.Parse(strings.TrimSpace(candidate))
		if err == nil && isLocalhostHost(parsed.Hostname()) {
			return "dev"
		}
	}
	return "prod"
}

func validateClientID(clientID string) error {
	if !clientIDPattern.MatchString(strings.TrimSpace(clientID)) {
		return apperrors.Params("clientId格式不正确")
	}
	return nil
}

func normalizeClientTypeForAdmin(value string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "PUBLIC":
		return "PUBLIC", nil
	case "CONFIDENTIAL":
		return "CONFIDENTIAL", nil
	default:
		return "", apperrors.Params("clientType不支持")
	}
}

func normalizeClientAuthMethodForAdmin(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "none":
		return "none", nil
	case "client_secret_basic":
		return "client_secret_basic", nil
	default:
		return "", apperrors.Params("clientAuthMethod不支持")
	}
}

func normalizeGrantTypesForAdmin(values []string) ([]string, error) {
	return normalizeStringSetForAdmin(values, map[string]struct{}{
		"authorization_code": {},
		"refresh_token":      {},
	}, "grantType不支持")
}

func normalizeScopesForAdmin(values []string) ([]string, error) {
	scopes, err := normalizeStringSetForAdmin(values, map[string]struct{}{
		"openid":         {},
		"profile":        {},
		"email":          {},
		"offline_access": {},
	}, "scope不支持")
	if err != nil {
		return nil, err
	}
	if !hasScope(scopes, "openid") {
		return nil, apperrors.Params("scope必须包含openid")
	}
	return scopes, nil
}

func normalizeStringSetForAdmin(values []string, allow map[string]struct{}, message string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, item := range values {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, ok := allow[value]; !ok {
			return nil, apperrors.Params(message + "：" + value)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil, apperrors.Params(message)
	}
	sort.Strings(result)
	return result, nil
}

func validateClientTTL(accessTTL, refreshTTL int) (int, int, error) {
	if accessTTL == 0 {
		accessTTL = 1800
	}
	if refreshTTL == 0 {
		refreshTTL = 2592000
	}
	if accessTTL < 300 || accessTTL > 7200 {
		return 0, 0, apperrors.Params("accessTokenTtlSec必须在300到7200之间")
	}
	if refreshTTL < 3600 || refreshTTL > 7776000 {
		return 0, 0, apperrors.Params("refreshTokenTtlSec必须在3600到7776000之间")
	}
	return accessTTL, refreshTTL, nil
}

func validateSecretExpiresInDays(value int) (int, error) {
	if value < 0 {
		return 0, apperrors.Params("expiresInDays不能为负数")
	}
	if value > 3650 {
		return 0, apperrors.Params("expiresInDays不能超过3650天")
	}
	return value, nil
}

func generateClientSecretPlaintext() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", apperrors.System("SSO客户端密钥生成失败")
	}
	return "sec_live_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

func clientSecretHint(secret string) string {
	secret = strings.TrimSpace(secret)
	if len(secret) <= 8 {
		return "sec_****"
	}
	return "sec_****" + secret[len(secret)-4:]
}

func findClientSecretSummary(items []domain.ClientSecretSummary, secretID int64) (domain.ClientSecretSummary, bool) {
	for _, item := range items {
		if item.ID == secretID {
			return item, true
		}
	}
	return domain.ClientSecretSummary{}, false
}

func clientAdminPayloadHash(item *domain.Client) (string, error) {
	payload := clientAdminBindingPayload{
		ClientID:           item.ClientID,
		ClientName:         item.ClientName,
		ClientType:         item.ClientType,
		ClientAuthMethod:   item.ClientAuthMethod,
		GrantTypes:         append([]string(nil), item.GrantTypes...),
		Scopes:             append([]string(nil), item.Scopes...),
		RequirePKCE:        item.RequirePKCE,
		RequireConsent:     item.RequireConsent,
		TrustedFirstParty:  item.TrustedFirstParty,
		AccessTokenTTLSec:  item.AccessTokenTTLSec,
		RefreshTokenTTLSec: item.RefreshTokenTTLSec,
		MetadataJSON:       item.MetadataJSON,
	}
	raw, err := sonic.MarshalString(payload)
	if err != nil {
		return "", apperrors.System("SSO客户端payload序列化失败")
	}
	return sha256Hex(raw), nil
}

func clientAdminOperationBinding(clientID string, action challengedomain.BusinessAction, payloadHash string) string {
	return "sso:client:" + strings.TrimSpace(clientID) + "|action:" + string(action) + "|payload:" + strings.TrimSpace(payloadHash)
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum[:])
}

func normalizeClientAdminPage(current, pageSize int) (int, int) {
	if current <= 0 {
		current = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return current, pageSize
}

func toClientAdminRecords(items []domain.Client) []ssofacade.ClientAdminRecord {
	result := make([]ssofacade.ClientAdminRecord, 0, len(items))
	for _, item := range items {
		result = append(result, ssofacade.ClientAdminRecord{
			ID:                     item.ID,
			ClientID:               item.ClientID,
			ClientName:             item.ClientName,
			ClientType:             item.ClientType,
			ClientAuthMethod:       item.ClientAuthMethod,
			GrantTypes:             append([]string(nil), item.GrantTypes...),
			Scopes:                 append([]string(nil), item.Scopes...),
			RequirePKCE:            item.RequirePKCE,
			RequireConsent:         item.RequireConsent,
			TrustedFirstParty:      item.TrustedFirstParty,
			AccessTokenTTLSec:      item.AccessTokenTTLSec,
			RefreshTokenTTLSec:     item.RefreshTokenTTLSec,
			Status:                 item.Status,
			MetadataJSON:           item.MetadataJSON,
			ActiveRedirectURICount: item.ActiveRedirectCount,
			ActiveSecretCount:      item.ActiveSecretCount,
			CreateTime:             item.CreateTime,
			UpdateTime:             item.UpdateTime,
		})
	}
	return result
}

func toClientRedirectURIRecords(items []domain.ClientRedirectURI) []ssofacade.ClientRedirectURIRecord {
	result := make([]ssofacade.ClientRedirectURIRecord, 0, len(items))
	for _, item := range items {
		result = append(result, ssofacade.ClientRedirectURIRecord{
			ID:                    item.ID,
			ClientID:              item.ClientID,
			RedirectURI:           item.RedirectURI,
			PostLogoutRedirectURI: item.PostLogoutRedirectURI,
			Status:                item.Status,
			CreateTime:            item.CreateTime,
			UpdateTime:            item.UpdateTime,
		})
	}
	return result
}

func toClientSecretSummaryRecords(items []domain.ClientSecretSummary) []ssofacade.ClientSecretSummaryRecord {
	result := make([]ssofacade.ClientSecretSummaryRecord, 0, len(items))
	for _, item := range items {
		result = append(result, ssofacade.ClientSecretSummaryRecord{
			SecretID:   item.ID,
			SecretHint: item.SecretHint,
			Status:     item.Status,
			ExpiresAt:  item.ExpiresAt,
			CreateTime: item.CreateTime,
		})
	}
	return result
}

func normalizeAMR(items []string) []string {
	result := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func normalizeSessionSource(loginMethod, providerCode string, identityID int64, acr string, amr []string) (string, string, int64, string, []string, error) {
	method := strings.ToUpper(strings.TrimSpace(loginMethod))
	provider := strings.TrimSpace(providerCode)
	normalizedACR := strings.TrimSpace(acr)
	external := method == "EXTERNAL_OAUTH" || provider != "" || identityID > 0
	if external {
		method = "EXTERNAL_OAUTH"
		canonicalProvider, err := canonicalExternalProviderCode(provider)
		if err != nil {
			return "", "", 0, "", nil, err
		}
		provider = canonicalProvider
		if normalizedACR == "" {
			normalizedACR = "LEVEL_1"
		}
		amr = withoutOAuthAMR(amr)
		amr = append(amr, "oauth")
		if provider != "" {
			amr = append(amr, "oauth:"+provider)
		}
	} else if method == "" {
		method = "LOCAL"
	}
	normalizedAMR := normalizeAMR(amr)
	if len(normalizedAMR) == 0 {
		normalizedAMR = []string{"pwd"}
	}
	return method, provider, identityID, normalizedACR, normalizedAMR, nil
}

func canonicalExternalProviderCode(providerCode string) (string, error) {
	canonical := strings.ToLower(strings.TrimSpace(providerCode))
	if canonical == "" {
		return "", apperrors.Params("externalProviderCode不能为空")
	}
	if strings.HasPrefix(canonical, "hub:") {
		if _, err := federation.CanonicalManagedOwner(strings.TrimPrefix(canonical, "hub:")); err != nil {
			return "", apperrors.Params("externalProviderCode格式无效")
		}
		return canonical, nil
	}
	if !externalProviderCodePattern.MatchString(canonical) {
		return "", apperrors.Params("externalProviderCode格式无效")
	}
	return canonical, nil
}

func withoutOAuthAMR(items []string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(item)
		lower := strings.ToLower(value)
		if value == "" || lower == "oauth" || strings.HasPrefix(lower, "oauth:") {
			continue
		}
		result = append(result, value)
	}
	return result
}

func externalSessionAuditSnapshots(sessions []domain.Session) []map[string]any {
	result := make([]map[string]any, 0, len(sessions))
	for _, session := range sessions {
		result = append(result, map[string]any{
			"sessionId":            strings.TrimSpace(session.SessionID),
			"clientId":             strings.TrimSpace(session.ClientID),
			"userId":               session.UserID,
			"deviceId":             strings.TrimSpace(session.DeviceID),
			"statusBefore":         session.Status,
			"loginMethod":          strings.TrimSpace(session.LoginMethod),
			"externalProviderCode": strings.TrimSpace(session.ExternalProviderCode),
			"externalIdentityId":   session.ExternalIdentityID,
		})
	}
	return result
}

func verifyPKCE(codeChallenge, codeVerifier string) bool {
	codeVerifier = strings.TrimSpace(codeVerifier)
	if !isValidPKCEVerifier(codeVerifier) {
		return false
	}
	sum := sha256Sum(codeVerifier)
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(codeChallenge)), []byte(sum)) == 1
}

func isValidPKCEVerifier(value string) bool {
	if len(value) < 43 || len(value) > 128 {
		return false
	}
	for _, item := range value {
		switch {
		case item >= 'a' && item <= 'z':
		case item >= 'A' && item <= 'Z':
		case item >= '0' && item <= '9':
		case item == '-' || item == '.' || item == '_' || item == '~':
		default:
			return false
		}
	}
	return true
}

func sha256Sum(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func hasScope(scopes []string, target string) bool {
	for _, item := range scopes {
		if strings.TrimSpace(item) == target {
			return true
		}
	}
	return false
}

func looksLikeJWT(value string) bool {
	return strings.Count(strings.TrimSpace(value), ".") == 2
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		raw, _ := sonic.MarshalString(value)
		return strings.Trim(raw, "\"")
	}
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(stringValue(item)); text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func scopeSlice(value any) []string {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return strings.Fields(strings.TrimSpace(typed))
	case []byte:
		if strings.TrimSpace(string(typed)) == "" {
			return nil
		}
		return strings.Fields(strings.TrimSpace(string(typed)))
	default:
		return stringSlice(value)
	}
}

func firstClaim(claims map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := claims[key]; ok && value != nil {
			return value
		}
	}
	return nil
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case int16:
		return int64(typed)
	case int8:
		return int64(typed)
	case uint64:
		return int64(typed)
	case uint32:
		return int64(typed)
	case uint16:
		return int64(typed)
	case uint8:
		return int64(typed)
	case float64:
		return int64(typed)
	case float32:
		return int64(typed)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil {
			return parsed
		}
		return 0
	case []byte:
		parsed, err := strconv.ParseInt(strings.TrimSpace(string(typed)), 10, 64)
		if err == nil {
			return parsed
		}
		return 0
	default:
		return 0
	}
}

func unixTime(value any) *time.Time {
	switch typed := value.(type) {
	case time.Time:
		item := typed.UTC()
		return &item
	case *time.Time:
		if typed == nil {
			return nil
		}
		item := typed.UTC()
		return &item
	}
	seconds := int64Value(value)
	if seconds <= 0 {
		return nil
	}
	item := time.Unix(seconds, 0).UTC()
	return &item
}

func maxInt(current, fallback int) int {
	if current > 0 {
		return current
	}
	return fallback
}

func ssoBaseURL(baseURL, issuer string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(strings.TrimSpace(issuer), "/")
	}
	if baseURL == "" {
		return ""
	}
	if strings.HasSuffix(baseURL, "/sso") {
		return baseURL
	}
	return baseURL + "/sso"
}

func metadataJSON(value map[string]any) string {
	raw, err := sonic.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}

func parseFamilyScopes(metadata string) []string {
	if strings.TrimSpace(metadata) == "" {
		return nil
	}
	var payload struct {
		Scopes []string `json:"scopes"`
	}
	if err := sonic.UnmarshalString(metadata, &payload); err != nil {
		return nil
	}
	return payload.Scopes
}
