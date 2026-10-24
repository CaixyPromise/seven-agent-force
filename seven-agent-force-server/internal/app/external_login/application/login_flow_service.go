package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/facade"
	platformfacade "github.com/CaixyPromise/seven-agent/force/internal/app/platform/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

func (s *Service) ListLoginMethods(ctx context.Context, _ facade.ListLoginMethodsRequest) ([]facade.LoginMethodRecord, error) {
	providers, err := s.repo.ListLoginMethods(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]facade.LoginMethodRecord, 0, len(providers))
	for _, provider := range providers {
		result = append(result, facade.LoginMethodRecord{
			ProviderCode: provider.ProviderCode,
			DisplayName:  provider.DisplayName,
			Icon:         provider.Icon,
			SortOrder:    provider.SortOrder,
			LoginURL:     s.loginURL(provider.ProviderCode),
		})
	}
	return result, nil
}

func (s *Service) StartExternalLogin(ctx context.Context, req facade.StartExternalLoginRequest) (*facade.StartExternalLoginResult, error) {
	provider, driver, err := s.loadLoginProviderAndDriver(ctx, req.ProviderCode)
	if err != nil {
		return nil, err
	}
	authority, err := s.resolvePlatformForStart(ctx, req, provider.ProviderCode)
	if err != nil {
		return nil, err
	}
	oauthState, err := s.randomToken(ctx)
	if err != nil {
		return nil, err
	}
	nonce, err := s.randomToken(ctx)
	if err != nil {
		return nil, err
	}
	codeVerifier, err := s.randomToken(ctx)
	if err != nil {
		return nil, err
	}
	encryptedVerifier, err := s.encryptLoginStateSecrets(ctx, loginStateSecrets{
		CodeVerifier: codeVerifier,
		Nonce:        nonce,
	})
	if err != nil {
		return nil, err
	}
	ttl := time.Duration(s.cfg.StateTTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	now := s.now()
	stateHash := hashString(oauthState)
	state := domain.LoginState{
		StateID:                 stateStorageIDFromHash(stateHash),
		ProviderCode:            provider.ProviderCode,
		PlatformCode:            authority.PlatformCode,
		ProvisioningAuthorityID: authority.ProvisioningAuthorityID,
		LoginTransactionID:      strings.TrimSpace(req.LoginTransactionID),
		RedirectAfterLogin:      strings.TrimSpace(req.RedirectAfterLogin),
		BindUserID:              req.BindUserID,
		StateHash:               stateHash,
		NonceHash:               hashString(nonce),
		CodeVerifierCiphertext:  encryptedVerifier.CiphertextB64,
		CodeVerifierEDEK:        encryptedVerifier.EDEKB64,
		CodeVerifierWrapKeyRef:  encryptedVerifier.WrapKeyRef,
		Issuer:                  provider.Issuer,
		ProviderConfigDigest:    managedProviderConfigDigest(provider),
		RedirectURI:             provider.RedirectURI,
		ExpiresAt:               now.Add(ttl),
		Status:                  domain.LoginStateStatusActive,
	}
	if req.RequestContext != nil {
		state.LoginIP = req.RequestContext.LoginIP
		state.UserAgent = req.RequestContext.UserAgent
		state.TraceID = req.RequestContext.TraceID
	}
	if err := s.repo.InsertLoginState(ctx, &state); err != nil {
		return nil, err
	}
	if s.stateCache != nil {
		if err := s.stateCache.Put(ctx, state, ttl); err != nil {
			return nil, err
		}
	}
	redirectURL, err := driver.BuildAuthorizationURL(ctx, *provider, AuthorizationRequest{
		State:               oauthState,
		Nonce:               nonce,
		CodeChallenge:       pkceChallenge(codeVerifier),
		CodeChallengeMethod: "S256",
		RedirectURI:         provider.RedirectURI,
		Scopes:              append([]string(nil), provider.Scopes...),
		Issuer:              provider.Issuer,
	})
	if err != nil {
		return nil, err
	}
	return &facade.StartExternalLoginResult{RedirectURL: redirectURL, StateID: oauthState}, nil
}

func (s *Service) CompleteExternalCallback(ctx context.Context, req facade.CompleteExternalCallbackRequest) (*facade.ExternalLoginResult, error) {
	provider, driver, err := s.loadLoginProviderAndDriver(ctx, req.ProviderCode)
	if err != nil {
		return nil, err
	}
	state, err := s.repo.ConsumeLoginState(ctx, hashString(req.State), s.now())
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, apperrors.ObjectState("外部登录state无效或已使用")
	}
	if state.ProviderCode != provider.ProviderCode {
		return nil, apperrors.ObjectState("外部登录provider不匹配")
	}
	if err := s.requirePlatformLoginMethod(ctx, state.PlatformCode, provider.ProviderCode); err != nil {
		return nil, err
	}
	if strings.TrimSpace(state.Issuer) != "" && strings.TrimSpace(req.Issuer) != "" && strings.TrimSpace(state.Issuer) != strings.TrimSpace(req.Issuer) {
		return nil, apperrors.ObjectState("外部登录issuer不匹配")
	}
	if s.stateCache != nil {
		_ = s.stateCache.Delete(ctx, state.StateID)
	}
	secrets, err := s.decryptLoginStateSecrets(ctx, *state, provider.ProtocolType)
	if err != nil {
		return nil, err
	}
	clientSecret, err := s.decryptClientSecret(ctx, *provider)
	if err != nil {
		return nil, err
	}
	tokenResult, err := driver.ExchangeCode(ctx, *provider, TokenExchangeRequest{
		Code:           strings.TrimSpace(req.Code),
		State:          strings.TrimSpace(req.State),
		CodeVerifier:   secrets.CodeVerifier,
		RedirectURI:    state.RedirectURI,
		ClientSecret:   clientSecret,
		Nonce:          secrets.Nonce,
		ExpectedIssuer: state.Issuer,
		CallbackIssuer: req.Issuer,
		Scopes:         append([]string(nil), provider.Scopes...),
	})
	if err != nil {
		return nil, providerOperationError(provider.ProviderCode, "授权码交换", err)
	}
	profile, err := driver.ResolveProfile(ctx, *provider, *tokenResult)
	if err != nil {
		return nil, providerOperationError(provider.ProviderCode, "账号资料获取", err)
	}
	if profile == nil || strings.TrimSpace(profile.Subject) == "" {
		return nil, apperrors.Operation("外部账号身份无效")
	}
	if state.BindUserID > 0 {
		return s.completeCurrentUserBinding(ctx, state, *provider, *profile, *tokenResult)
	}
	var identity *domain.ExternalIdentity
	err = s.withTransaction(ctx, func(txCtx context.Context) error {
		if isManagedProvider(provider) {
			locked, lockErr := s.repo.FindProviderForUpdate(txCtx, provider.ProviderCode)
			if lockErr != nil || locked == nil {
				return lockErr
			}
			if locked.Issuer != state.Issuer {
				return apperrors.ObjectState("外部登录issuer已变更，请重新发起登录")
			}
			if locked.Status != domain.ProviderStatusActive || !locked.LoginEnabled || managedProviderConfigDigest(locked) != state.ProviderConfigDigest {
				return apperrors.ObjectState("外部登录Provider配置已变更，请重新发起登录")
			}
			provider = locked
		}
		var txErr error
		identity, txErr = s.resolveOrAutoBindIdentity(txCtx, state, *provider, *profile)
		if txErr != nil || identity == nil {
			return txErr
		}
		if identity.Status != domain.IdentityStatusActive {
			return apperrors.ObjectState("外部账号已禁用")
		}
		subject, txErr := s.subjects.FindSubjectByID(txCtx, identity.UserID)
		if txErr != nil {
			return txErr
		}
		if subject == nil || !subject.Enabled || subject.LockStatus {
			return apperrors.ObjectState("本地账号不可用")
		}
		now := s.now()
		if txErr = s.repo.TouchIdentityLogin(txCtx, identity.ID, *profile, now); txErr != nil {
			return txErr
		}
		if txErr = s.syncLocalUserProfile(txCtx, provider.ProviderCode, identity.UserID, *profile); txErr != nil {
			return txErr
		}
		if !persistLoginTokens(*provider) {
			return nil
		}
		_, txErr = s.tokenVault.StoreTokenSet(txCtx, &domain.OAuthToken{
			ProviderCode: provider.ProviderCode,
			IdentityID:   identity.ID,
			UserID:       identity.UserID,
			TokenPurpose: domain.TokenPurposeLogin,
			Scopes:       append([]string(nil), tokenResult.TokenSet.Scopes...),
			ScopeHash:    scopeHash(tokenResult.TokenSet.Scopes),
			Status:       domain.TokenStatusActive,
			Version:      1,
		}, tokenResult.TokenSet)
		return txErr
	})
	if err != nil {
		return nil, err
	}
	if identity == nil {
		return nil, apperrors.Operation("外部账号未绑定")
	}
	if state.LoginTransactionID != "" {
		return s.completeSSOTransaction(ctx, state, provider.ProviderCode, identity.ID, identity.UserID, req.RequestContext)
	}
	return s.bootstrapFirstParty(ctx, provider.ProviderCode, state.PlatformCode, identity.ID, identity.UserID, req.RequestContext)
}

type startPlatformAuthority struct {
	PlatformCode            string
	ProvisioningAuthorityID string
}

func (s *Service) resolvePlatformForStart(ctx context.Context, req facade.StartExternalLoginRequest, providerCode string) (startPlatformAuthority, error) {
	if s == nil || s.platform == nil {
		if s != nil && s.cfg.FailClosed {
			return startPlatformAuthority{}, apperrors.System("平台登录策略未配置")
		}
		return startPlatformAuthority{}, nil
	}
	if req.BindUserID <= 0 && strings.TrimSpace(req.LoginContextID) == "" {
		return startPlatformAuthority{}, apperrors.Forbidden("登录上下文缺失，请重新登录")
	}
	platformReq := platformfacade.ResolvePlatformRequest{
		ClientID:           req.TrustedSource.ClientID,
		LoginTransactionID: req.LoginTransactionID,
		LoginContextID:     req.LoginContextID,
		RedirectURL:        req.RedirectAfterLogin,
		ExplicitCode:       req.PlatformCode,
		TrustedSource: platformfacade.TrustedSource{
			ClientID:    req.TrustedSource.ClientID,
			RedirectURL: req.TrustedSource.RedirectURL,
			Host:        req.TrustedSource.Host,
			Origin:      req.TrustedSource.Origin,
			Referer:     req.TrustedSource.Referer,
		},
	}
	var platformCode string
	if strings.TrimSpace(req.LoginContextID) != "" {
		validation, err := s.platform.ValidateLoginContext(ctx, req.LoginContextID, platformReq)
		if err != nil {
			return startPlatformAuthority{}, err
		}
		platformCode = validation.PlatformCode
	} else {
		code, err := s.platform.ResolvePlatformCode(ctx, platformReq)
		if err != nil {
			return startPlatformAuthority{}, err
		}
		platformCode = code
	}
	if err := s.platform.RequireLoginMethod(ctx, platformCode, externalOAuthLoginMethod, providerCode); err != nil {
		return startPlatformAuthority{}, err
	}
	result := startPlatformAuthority{PlatformCode: platformCode}
	if req.BindUserID <= 0 {
		authority, err := s.platform.IssueProvisioningAuthority(ctx, req.LoginContextID, platformReq)
		if err != nil {
			return startPlatformAuthority{}, err
		}
		if !strings.EqualFold(authority.PlatformCode, platformCode) {
			return startPlatformAuthority{}, apperrors.Forbidden("平台注册授权平台不匹配")
		}
		result.ProvisioningAuthorityID = authority.AuthorityID
	}
	return result, nil
}

func (s *Service) requirePlatformLoginMethod(ctx context.Context, platformCode, providerCode string) error {
	if s == nil || strings.TrimSpace(platformCode) == "" {
		return nil
	}
	if s.platform == nil {
		if s.cfg.FailClosed {
			return apperrors.System("平台登录策略未配置")
		}
		return nil
	}
	return s.platform.RequireLoginMethod(ctx, platformCode, externalOAuthLoginMethod, providerCode)
}

func (s *Service) completeCurrentUserBinding(ctx context.Context, state *domain.LoginState, provider domain.Provider, profile domain.ExternalProfile, tokenResult TokenExchangeResult) (*facade.ExternalLoginResult, error) {
	if !provider.BindEnabled {
		return nil, apperrors.ObjectState("外部账号绑定已禁用")
	}
	subject, err := s.subjects.FindSubjectByID(ctx, state.BindUserID)
	if err != nil {
		return nil, err
	}
	if subject == nil || !subject.Enabled || subject.LockStatus {
		return nil, apperrors.ObjectState("本地账号不可用")
	}
	var identity *domain.ExternalIdentity
	if err := s.withTransaction(ctx, func(txCtx context.Context) error {
		if isManagedProvider(&provider) {
			locked, lockErr := s.repo.FindProviderForUpdate(txCtx, provider.ProviderCode)
			if lockErr != nil || locked == nil {
				return lockErr
			}
			if locked.Issuer != state.Issuer {
				return apperrors.ObjectState("外部登录issuer已变更，请重新发起绑定")
			}
			if locked.Status != domain.ProviderStatusActive || !locked.BindEnabled || managedProviderConfigDigest(locked) != state.ProviderConfigDigest {
				return apperrors.ObjectState("外部登录Provider配置已变更，请重新发起绑定")
			}
			provider = *locked
		}
		var txErr error
		lookupSubject, txErr := s.systemSubjectLookup(txCtx, provider.ProviderCode, identityIssuer(provider), profile.Subject)
		if txErr != nil {
			return txErr
		}
		identity, txErr = s.repo.FindIdentityBySubject(txCtx, provider.ProviderCode, identityIssuer(provider), lookupSubject)
		if txErr != nil {
			return txErr
		}
		now := s.now()
		if identity != nil {
			if identity.UserID != state.BindUserID {
				return apperrors.Operation("该外部账号已绑定其他用户")
			}
			if identity.Status != domain.IdentityStatusActive {
				return apperrors.ObjectState("外部账号已禁用")
			}
			if txErr := s.repo.TouchIdentityLogin(txCtx, identity.ID, profile, now); txErr != nil {
				return txErr
			}
		} else {
			protected, protectErr := s.protectSystemSubject(txCtx, provider.ProviderCode, identityIssuer(provider), profile.Subject)
			if protectErr != nil {
				return protectErr
			}
			identity = &domain.ExternalIdentity{
				ID:                  s.nextID(),
				ProviderCode:        provider.ProviderCode,
				ExternalIssuer:      identityIssuer(provider),
				ExternalSubject:     protected.digest,
				SubjectCiphertext:   protected.encrypted.CiphertextB64,
				SubjectEDEK:         protected.encrypted.EDEKB64,
				SubjectWrapKeyRef:   protected.encrypted.WrapKeyRef,
				SubjectDigest:       protected.digest,
				SubjectDigestKeyRef: protected.digestKeyRef,
				UserID:              state.BindUserID,
				ExternalLogin:       profile.Login,
				ExternalEmail:       profile.Email,
				EmailVerified:       profile.EmailVerified,
				DisplayName:         profile.DisplayName,
				AvatarURL:           profile.AvatarURL,
				ProfileJSON:         profile.RawProfile,
				Status:              domain.IdentityStatusActive,
				FirstLinkedAt:       now,
				LastLoginAt:         &now,
				LastVerifiedAt:      &now,
			}
			if txErr := s.repo.InsertIdentity(txCtx, identity, state.BindUserID); txErr != nil {
				return txErr
			}
		}
		if txErr := s.syncLocalUserProfile(txCtx, provider.ProviderCode, state.BindUserID, profile); txErr != nil {
			return txErr
		}
		// A binding callback proves account ownership only. It must never create
		// a LOGIN token or an authenticated platform session. Enterprise login is
		// authorized separately by an explicit LOGIN grant.
		return nil
	}); err != nil {
		return nil, err
	}
	return &facade.ExternalLoginResult{
		Authenticated:      false,
		Outcome:            "BOUND",
		Purpose:            "IDENTITY_LINK",
		UserID:             state.BindUserID,
		ExternalIdentityID: identity.ID,
		ProviderCode:       provider.ProviderCode,
		RedirectURL:        firstNonBlankString(state.RedirectAfterLogin, "/account/settings"),
	}, nil
}

func (s *Service) syncLocalUserProfile(ctx context.Context, providerCode string, userID int64, profile domain.ExternalProfile) error {
	if s == nil || s.profiles == nil || userID <= 0 {
		return nil
	}
	return s.profiles.SyncExternalProfile(ctx, userfacade.SyncExternalProfileCommand{
		UserID:        userID,
		ProviderCode:  providerCode,
		ExternalLogin: profile.Login,
		NickName:      firstNonBlankString(profile.DisplayName, profile.Login),
		UserEmail:     profile.Email,
		EmailVerified: profile.EmailVerified,
		UserAvatar:    profile.AvatarURL,
		RawProfile:    profile.RawProfile,
	})
}

func (s *Service) loadLoginProviderAndDriver(ctx context.Context, providerCode string) (*domain.Provider, ProviderDriverPort, error) {
	code, err := domain.NormalizeProviderCode(providerCode)
	if err != nil {
		return nil, nil, err
	}
	provider, err := s.repo.FindProvider(ctx, code)
	if err != nil {
		return nil, nil, err
	}
	if provider == nil {
		return nil, nil, apperrors.NotFound("外部登录提供方不存在")
	}
	if provider.Status != domain.ProviderStatusActive || !provider.LoginEnabled {
		return nil, nil, apperrors.ObjectState("外部登录提供方已禁用")
	}
	driver, ok := s.drivers.Get(code)
	if (!ok || driver == nil) && provider.ProtocolType == domain.ProtocolTypeOIDC {
		driver, ok = s.drivers.Get("oidc")
	}
	if !ok || driver == nil {
		return nil, nil, apperrors.ObjectState("外部登录驱动未配置")
	}
	return provider, driver, nil
}

func (s *Service) resolveOrAutoBindIdentity(ctx context.Context, state *domain.LoginState, provider domain.Provider, profile domain.ExternalProfile) (*domain.ExternalIdentity, error) {
	issuer := identityIssuer(provider)
	lookupSubject, err := s.systemSubjectLookup(ctx, provider.ProviderCode, issuer, profile.Subject)
	if err != nil {
		return nil, err
	}
	identity, err := s.repo.FindIdentityBySubject(ctx, provider.ProviderCode, issuer, lookupSubject)
	if err != nil || identity != nil {
		return identity, err
	}
	managed := isManagedProvider(&provider)
	var subject *userfacade.SubjectRecord
	if !managed {
		if !provider.EmailAutoBindEnabled || !profile.EmailVerified || strings.TrimSpace(profile.Email) == "" {
			return nil, nil
		}
		subject, err = s.subjects.FindSubjectByEmail(ctx, strings.TrimSpace(profile.Email))
		if err != nil {
			return nil, err
		}
	} else if !provider.AccountAutoCreateEnabled {
		return nil, nil
	}
	var provisioning *platformfacade.ProvisioningPolicy
	if subject == nil && provider.AccountAutoCreateEnabled {
		provisioning, err = s.provisioningPolicyForState(ctx, state)
		if err != nil {
			return nil, err
		}
		if provisioning == nil || !provisioning.AllowAutoRegister {
			return nil, nil
		}
		subject, err = s.subjects.CreateExternalSubject(ctx, userfacade.CreateExternalSubjectCommand{
			AccountName:          externalAccountName(provider.ProviderCode, profile),
			NickName:             firstNonBlankString(profile.DisplayName, profile.Login, profile.Email),
			UserEmail:            profile.Email,
			UserAvatar:           profile.AvatarURL,
			RegisterPlatformCode: provisioning.PlatformCode,
			RegisterProviderCode: provider.ProviderCode,
			DefaultOrgID:         provisioning.DefaultOrgID,
			DefaultDeptID:        provisioning.DefaultDeptID,
			DefaultPostIDs:       append([]int64(nil), provisioning.DefaultPostIDs...),
			DefaultRoleIDs:       append([]int64(nil), provisioning.DefaultRoleIDs...),
			DisableEmailMerge:    managed,
		})
		if err != nil {
			return nil, err
		}
	}
	if subject == nil || !subject.Enabled || subject.LockStatus {
		return nil, nil
	}
	now := s.now()
	protected, err := s.protectSystemSubject(ctx, provider.ProviderCode, issuer, profile.Subject)
	if err != nil {
		return nil, err
	}
	identity = &domain.ExternalIdentity{
		ID:                  s.nextID(),
		ProviderCode:        provider.ProviderCode,
		ExternalIssuer:      issuer,
		ExternalSubject:     protected.digest,
		SubjectCiphertext:   protected.encrypted.CiphertextB64,
		SubjectEDEK:         protected.encrypted.EDEKB64,
		SubjectWrapKeyRef:   protected.encrypted.WrapKeyRef,
		SubjectDigest:       protected.digest,
		SubjectDigestKeyRef: protected.digestKeyRef,
		UserID:              subject.UserID,
		ExternalLogin:       profile.Login,
		ExternalEmail:       profile.Email,
		EmailVerified:       profile.EmailVerified,
		DisplayName:         profile.DisplayName,
		AvatarURL:           profile.AvatarURL,
		ProfileJSON:         profile.RawProfile,
		Status:              domain.IdentityStatusActive,
		FirstLinkedAt:       now,
		LastLoginAt:         &now,
		LastVerifiedAt:      &now,
	}
	if err := s.repo.InsertIdentity(ctx, identity, subject.UserID); err != nil {
		return nil, err
	}
	return identity, nil
}

type protectedSystemSubject struct {
	digest       string
	digestKeyRef string
	encrypted    EncryptedSecretValue
}

func (s *Service) systemSubjectLookup(ctx context.Context, providerCode, issuer, subject string) (string, error) {
	if s.identityDigester == nil {
		return strings.TrimSpace(subject), nil
	}
	digest, _, err := s.identityDigester.DigestSystemSubject(ctx, "", providerCode, issuer, strings.TrimSpace(subject))
	return digest, err
}

func (s *Service) protectSystemSubject(ctx context.Context, providerCode, issuer, subject string) (protectedSystemSubject, error) {
	subject = strings.TrimSpace(subject)
	if s.identityDigester == nil || s.secrets == nil {
		return protectedSystemSubject{digest: subject}, nil
	}
	digest, keyRef, err := s.identityDigester.DigestSystemSubject(ctx, "", providerCode, issuer, subject)
	if err != nil {
		return protectedSystemSubject{}, err
	}
	encrypted, err := s.secrets.EncryptString(ctx, subject)
	if err != nil {
		return protectedSystemSubject{}, err
	}
	return protectedSystemSubject{digest: digest, digestKeyRef: keyRef, encrypted: encrypted}, nil
}

func identityIssuer(provider domain.Provider) string {
	if provider.ProtocolType != domain.ProtocolTypeOIDC {
		return ""
	}
	return strings.TrimSpace(provider.Issuer)
}

func (s *Service) provisioningPolicyForState(ctx context.Context, state *domain.LoginState) (*platformfacade.ProvisioningPolicy, error) {
	if s == nil || s.platform == nil {
		if s != nil && s.cfg.FailClosed {
			return nil, apperrors.System("平台登录策略未配置")
		}
		return &platformfacade.ProvisioningPolicy{AllowAutoRegister: true}, nil
	}
	if state == nil || strings.TrimSpace(state.PlatformCode) == "" || strings.TrimSpace(state.ProvisioningAuthorityID) == "" {
		return nil, apperrors.Forbidden("平台注册上下文缺失")
	}
	return s.platform.GetProvisioningPolicy(ctx, platformfacade.ProvisioningAuthority{
		AuthorityID:  strings.TrimSpace(state.ProvisioningAuthorityID),
		PlatformCode: strings.TrimSpace(state.PlatformCode),
		Authority:    platformfacade.AuthorityProvisioning,
	})
}

func externalAccountName(providerCode string, profile domain.ExternalProfile) string {
	source := firstNonBlankString(profile.Email, profile.Login, profile.Subject)
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(providerCode + "|" + source))))
	return "u" + hex.EncodeToString(sum[:])[:15]
}

func firstNonBlankString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func providerOperationError(providerCode string, stage string, err error) error {
	name := strings.TrimSpace(providerCode)
	if strings.EqualFold(name, "github") {
		name = "GitHub"
	}
	if strings.EqualFold(name, "google") {
		name = "Google"
	}
	switch strings.TrimSpace(stage) {
	case "账号资料获取":
		return apperrors.Operation("获取" + name + "账号资料失败，请稍后重试")
	case "授权码交换":
		return apperrors.Operation(name + "授权码交换失败，请重新发起登录")
	default:
		return apperrors.Operation(name + "外部登录请求失败，请稍后重试")
	}
}

func (s *Service) completeSSOTransaction(ctx context.Context, state *domain.LoginState, providerCode string, identityID int64, userID int64, requestContext *facade.RequestContext) (*facade.ExternalLoginResult, error) {
	result, err := s.authenticationComplete.CompleteInteractiveAuthentication(ctx, ssofacade.CompleteInteractiveAuthenticationCommand{
		LoginTransactionID:   state.LoginTransactionID,
		UserID:               userID,
		AMR:                  []string{"oauth", "oauth:" + providerCode},
		LoginMethod:          externalOAuthLoginMethod,
		ExternalProviderCode: providerCode,
		ExternalIdentityID:   identityID,
		PlatformCode:         state.PlatformCode,
		AuthTime:             ptrTime(s.now()),
		RequestContext:       mapSSORequestContext(requestContext),
	})
	if err != nil {
		return nil, err
	}
	return &facade.ExternalLoginResult{
		Authenticated:            result.Authenticated,
		LoginTransactionID:       result.LoginTransactionID,
		UserID:                   userID,
		ExternalIdentityID:       identityID,
		ProviderCode:             providerCode,
		PlatformCode:             state.PlatformCode,
		RedirectURL:              result.RedirectURL,
		SessionCookieHeaderValue: result.SessionCookieHeaderValue,
	}, nil
}

func (s *Service) bootstrapFirstParty(ctx context.Context, providerCode string, platformCode string, identityID int64, userID int64, requestContext *facade.RequestContext) (*facade.ExternalLoginResult, error) {
	result, err := s.bootstrapSession.BootstrapFirstPartySession(ctx, ssofacade.BootstrapSessionCommand{
		UserID:               userID,
		AMR:                  []string{"oauth", "oauth:" + providerCode},
		LoginMethod:          externalOAuthLoginMethod,
		ExternalProviderCode: providerCode,
		ExternalIdentityID:   identityID,
		PlatformCode:         platformCode,
		RequestContext:       mapSSORequestContext(requestContext),
	})
	if err != nil {
		return nil, err
	}
	return &facade.ExternalLoginResult{
		Authenticated:            true,
		UserID:                   userID,
		ExternalIdentityID:       identityID,
		ProviderCode:             providerCode,
		PlatformCode:             platformCode,
		AccessToken:              result.AccessToken,
		TokenType:                result.TokenType,
		AccessTTLSeconds:         result.AccessTTLSeconds,
		SessionCookieHeaderValue: result.SessionCookieHeaderValue,
		RefreshCookieHeaderValue: result.RefreshCookieHeaderValue,
	}, nil
}

func (s *Service) decryptClientSecret(ctx context.Context, provider domain.Provider) (string, error) {
	if strings.TrimSpace(provider.ClientSecretCiphertext) == "" {
		return "", nil
	}
	return s.secrets.DecryptString(ctx, EncryptedSecretValue{
		CiphertextB64: provider.ClientSecretCiphertext,
		EDEKB64:       provider.ClientSecretEDEK,
		WrapKeyRef:    provider.ClientSecretWrapKeyRef,
	})
}

type loginStateSecrets struct {
	CodeVerifier string `json:"codeVerifier"`
	Nonce        string `json:"nonce"`
}

func (s *Service) encryptLoginStateSecrets(ctx context.Context, payload loginStateSecrets) (EncryptedSecretValue, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return EncryptedSecretValue{}, fmt.Errorf("marshal external login state secrets: %w", err)
	}
	encrypted, err := s.secrets.EncryptString(ctx, string(raw))
	if err != nil {
		return EncryptedSecretValue{}, fmt.Errorf("encrypt external login state secrets: %w", err)
	}
	return encrypted, nil
}

func (s *Service) decryptLoginStateSecrets(ctx context.Context, state domain.LoginState, protocolType string) (loginStateSecrets, error) {
	plain, err := s.secrets.DecryptString(ctx, EncryptedSecretValue{
		CiphertextB64: state.CodeVerifierCiphertext,
		EDEKB64:       state.CodeVerifierEDEK,
		WrapKeyRef:    state.CodeVerifierWrapKeyRef,
	})
	if err != nil {
		return loginStateSecrets{}, fmt.Errorf("decrypt external login state secrets: %w", err)
	}
	var payload loginStateSecrets
	if err := json.Unmarshal([]byte(plain), &payload); err != nil || strings.TrimSpace(payload.CodeVerifier) == "" {
		payload = loginStateSecrets{CodeVerifier: plain}
	}
	if strings.TrimSpace(payload.Nonce) == "" {
		if s.cfg.FailClosed || strings.EqualFold(strings.TrimSpace(protocolType), domain.ProtocolTypeOIDC) {
			return loginStateSecrets{}, apperrors.ObjectState("外部登录nonce缺失")
		}
		return payload, nil
	}
	if strings.TrimSpace(state.NonceHash) != "" && hashString(payload.Nonce) != state.NonceHash {
		return loginStateSecrets{}, apperrors.ObjectState("外部登录nonce校验失败")
	}
	return payload, nil
}

func (s *Service) loginURL(providerCode string) string {
	base := strings.TrimRight(strings.TrimSpace(s.cfg.CallbackBaseURL), "/")
	return base + "/login/external/" + providerCode + "/start"
}

func stateStorageIDFromHash(stateHash string) string {
	stateHash = strings.TrimSpace(stateHash)
	if len(stateHash) > 32 {
		return "st_" + stateHash[:32]
	}
	return "st_" + stateHash
}

func (s *Service) randomToken(ctx context.Context) (string, error) {
	if s.random != nil {
		return s.random.Token(ctx)
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (s *Service) nextID() int64 {
	if s.idGen == nil {
		return s.now().UnixNano()
	}
	return s.idGen.NextID()
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

func mapSSORequestContext(req *facade.RequestContext) *ssofacade.RequestContext {
	if req == nil {
		return nil
	}
	return &ssofacade.RequestContext{
		DeviceID:  req.DeviceID,
		TenantID:  req.TenantID,
		LoginIP:   req.LoginIP,
		UserAgent: req.UserAgent,
		TraceID:   req.TraceID,
	}
}
