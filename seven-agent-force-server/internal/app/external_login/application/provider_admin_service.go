package application

import (
	"context"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
)

func (s *Service) ListProviders(ctx context.Context, query facade.ProviderQuery) (*facade.ProviderPage, error) {
	domainQuery := domain.ProviderQuery{
		Keyword:      query.Keyword,
		ProviderCode: query.ProviderCode,
		ProtocolType: query.ProtocolType,
		Status:       query.Status,
		Current:      query.Current,
		PageSize:     query.PageSize,
	}
	items, total, err := s.repo.ListProviders(ctx, domainQuery)
	if err != nil {
		return nil, err
	}
	current, pageSize := normalizePage(query.Current, query.PageSize)
	records := make([]facade.ProviderDetail, 0, len(items))
	for _, item := range items {
		records = append(records, mapProviderDetail(item, nil))
	}
	return &facade.ProviderPage{Records: records, Total: total, Current: current, PageSize: pageSize}, nil
}

func (s *Service) GetProvider(ctx context.Context, providerCode string) (*facade.ProviderDetail, error) {
	code, err := domain.NormalizeProviderCode(providerCode)
	if err != nil {
		return nil, err
	}
	provider, err := s.repo.FindProvider(ctx, code)
	if err != nil || provider == nil {
		return nil, err
	}
	methods, err := s.repo.ListProviderMethods(ctx, code)
	if err != nil {
		return nil, err
	}
	detail := mapProviderDetail(*provider, methods)
	return &detail, nil
}

func (s *Service) CreateProvider(ctx context.Context, actorID int64, req facade.ProviderSaveRequest, proof stepup.ProofMetadata) (*facade.ProviderDetail, error) {
	code, err := domain.NormalizeProviderCode(req.ProviderCode)
	if err != nil {
		return nil, err
	}
	if domain.IsManagedProviderCode(code) {
		return nil, apperrors.Forbidden("hub:命名空间由系统托管")
	}
	if err := stepup.Require(proof, StepUpActionExternalLoginProviderCreate, BuildProviderCreateOperationBinding(code)); err != nil {
		return nil, err
	}
	provider := providerFromSaveRequest(req, code)
	if err := domain.ValidateProvider(provider); err != nil {
		return nil, err
	}
	if err := s.repo.InsertProvider(ctx, &provider, actorID); err != nil {
		return nil, err
	}
	return s.GetProvider(ctx, code)
}

func (s *Service) UpdateProvider(ctx context.Context, actorID int64, providerCode string, req facade.ProviderUpdateRequest, proof stepup.ProofMetadata) (*facade.ProviderDetail, error) {
	code, err := domain.NormalizeProviderCode(providerCode)
	if err != nil {
		return nil, err
	}
	if err := stepup.Require(proof, StepUpActionExternalLoginProviderUpdate, BuildProviderUpdateOperationBinding(code)); err != nil {
		return nil, err
	}
	existing, err := s.repo.FindProvider(ctx, code)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, apperrors.NotFound("外部登录提供方不存在")
	}
	if isManagedProvider(existing) {
		return nil, apperrors.Forbidden("系统托管Provider禁止后台修改")
	}
	provider := providerFromUpdateRequest(req, code)
	provider.Status = existing.Status
	if err := domain.ValidateProvider(provider); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateProvider(ctx, &provider, actorID); err != nil {
		return nil, err
	}
	return s.GetProvider(ctx, code)
}

func (s *Service) UpdateProviderStatus(ctx context.Context, actorID int64, providerCode string, req facade.ProviderStatusRequest, proof stepup.ProofMetadata) error {
	code, err := domain.NormalizeProviderCode(providerCode)
	if err != nil {
		return err
	}
	status, err := domain.NormalizeProviderStatus(req.Status)
	if err != nil {
		return err
	}
	if err := stepup.Require(proof, StepUpActionExternalLoginProviderStatusChange, BuildProviderStatusOperationBinding(code, status)); err != nil {
		return err
	}
	existing, err := s.repo.FindProvider(ctx, code)
	if err != nil {
		return err
	}
	if isManagedProvider(existing) {
		return apperrors.Forbidden("系统托管Provider禁止后台修改")
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		return apperrors.Params("reason不能为空")
	}
	return s.withTransaction(ctx, func(txCtx context.Context) error {
		affected, err := s.repo.UpdateProviderStatus(txCtx, code, status, actorID, s.now())
		if err != nil {
			return err
		}
		if !affected {
			return apperrors.ObjectState("外部登录提供方状态未变更")
		}
		if status == domain.ProviderStatusDisabled {
			if s.sessions == nil {
				return apperrors.System("SSO会话撤销能力未配置")
			}
			if _, err := s.sessions.RevokeSessionsByExternalProvider(txCtx, code); err != nil {
				return err
			}
			if _, err := s.repo.RevokeTokensByProvider(txCtx, code, s.now(), reason); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) RotateClientSecret(ctx context.Context, actorID int64, providerCode string, req facade.RotateClientSecretRequest, proof stepup.ProofMetadata) error {
	code, err := domain.NormalizeProviderCode(providerCode)
	if err != nil {
		return err
	}
	if err := stepup.Require(proof, StepUpActionExternalLoginProviderSecretRotate, BuildProviderSecretRotateOperationBinding(code)); err != nil {
		return err
	}
	provider, err := s.repo.FindProvider(ctx, code)
	if err != nil || provider == nil {
		return err
	}
	if isManagedProvider(provider) {
		return apperrors.Forbidden("系统托管Provider禁止后台修改")
	}
	encrypted, err := s.secrets.EncryptString(ctx, strings.TrimSpace(req.ClientSecret))
	if err != nil {
		return err
	}
	provider.ClientSecretCiphertext = encrypted.CiphertextB64
	provider.ClientSecretEDEK = encrypted.EDEKB64
	provider.ClientSecretWrapKeyRef = encrypted.WrapKeyRef
	return s.repo.UpdateProvider(ctx, provider, actorID)
}

func (s *Service) ListIdentities(ctx context.Context, query facade.IdentityQuery) (*facade.IdentityPage, error) {
	items, total, err := s.repo.ListIdentities(ctx, domain.IdentityQuery{
		ProviderCode: strings.TrimSpace(query.ProviderCode),
		UserID:       query.UserID,
		Status:       query.Status,
		Keyword:      strings.TrimSpace(query.Keyword),
		Current:      query.Current,
		PageSize:     query.PageSize,
	})
	if err != nil {
		return nil, err
	}
	current, pageSize := normalizePage(query.Current, query.PageSize)
	records := make([]facade.ExternalIdentityRecord, 0, len(items))
	for _, item := range items {
		records = append(records, mapIdentity(item))
	}
	return &facade.IdentityPage{Records: records, Total: total, Current: current, PageSize: pageSize}, nil
}

func (s *Service) ListCurrentUserBindings(ctx context.Context, userID int64) ([]facade.CurrentUserBinding, error) {
	if userID <= 0 {
		return nil, apperrors.Unauthorized("未登录")
	}
	active := domain.ProviderStatusActive
	providers, _, err := s.repo.ListProviders(ctx, domain.ProviderQuery{Status: &active, Current: 1, PageSize: 200})
	if err != nil {
		return nil, err
	}
	userIDValue := userID
	identities, _, err := s.repo.ListIdentities(ctx, domain.IdentityQuery{UserID: &userIDValue, Current: 1, PageSize: 200})
	if err != nil {
		return nil, err
	}
	identityByProvider := make(map[string]domain.ExternalIdentity, len(identities))
	for _, identity := range identities {
		identityByProvider[identity.ProviderCode] = identity
	}
	result := make([]facade.CurrentUserBinding, 0, len(providers))
	for _, provider := range providers {
		if !provider.DisplayEnabled || !provider.BindEnabled {
			continue
		}
		item := facade.CurrentUserBinding{
			ProviderCode: provider.ProviderCode,
			DisplayName:  provider.DisplayName,
			Icon:         provider.Icon,
			BindEnabled:  provider.BindEnabled && provider.Status == domain.ProviderStatusActive,
			BindURL:      "/external-login/me/" + provider.ProviderCode + "/start",
			SortOrder:    provider.SortOrder,
		}
		if identity, ok := identityByProvider[provider.ProviderCode]; ok {
			item.Bound = identity.Status == domain.IdentityStatusActive
			item.IdentityID = identity.ID
			item.ExternalLogin = identity.ExternalLogin
			item.ExternalEmail = identity.ExternalEmail
			item.EmailVerified = identity.EmailVerified
			item.AvatarURL = identity.AvatarURL
			item.Status = identity.Status
			item.LastLoginAt = identity.LastLoginAt
			item.LastVerifiedAt = identity.LastVerifiedAt
		}
		result = append(result, item)
	}
	return result, nil
}

func (s *Service) ResolveIdentity(ctx context.Context, providerCode, externalSubject string) (*facade.ExternalIdentityRecord, error) {
	code, err := domain.NormalizeProviderCode(providerCode)
	if err != nil {
		return nil, err
	}
	provider, err := s.repo.FindProvider(ctx, code)
	if err != nil || provider == nil {
		return nil, err
	}
	lookupSubject, err := s.systemSubjectLookup(ctx, code, identityIssuer(*provider), externalSubject)
	if err != nil {
		return nil, err
	}
	identity, err := s.repo.FindIdentityBySubject(ctx, code, identityIssuer(*provider), lookupSubject)
	if err != nil || identity == nil {
		return nil, err
	}
	record := mapIdentity(*identity)
	return &record, nil
}

func (s *Service) UpdateIdentityStatus(ctx context.Context, actorID int64, identityID int64, req facade.IdentityStatusRequest, proof stepup.ProofMetadata) error {
	status, err := domain.NormalizeIdentityStatus(req.Status)
	if err != nil {
		return err
	}
	if err := stepup.Require(proof, StepUpActionExternalLoginIdentityStatusChange, BuildIdentityStatusOperationBinding(identityID, status)); err != nil {
		return err
	}
	return s.withTransaction(ctx, func(txCtx context.Context) error {
		affected, err := s.repo.UpdateIdentityStatus(txCtx, identityID, status, actorID, s.now())
		if err != nil {
			return err
		}
		if !affected {
			return apperrors.ObjectState("外部身份状态未变更")
		}
		if status == domain.IdentityStatusDisabled || status == domain.IdentityStatusUnlinked {
			if s.sessions != nil {
				if _, err := s.sessions.RevokeSessionsByExternalIdentity(txCtx, identityID); err != nil {
					return err
				}
			}
			if _, err := s.repo.RevokeTokensByIdentity(txCtx, identityID, s.now(), req.Reason); err != nil {
				return err
			}
		}
		return nil
	})
}

func providerFromSaveRequest(req facade.ProviderSaveRequest, code string) domain.Provider {
	return domain.Provider{
		ProviderCode:             code,
		ProviderName:             strings.TrimSpace(req.ProviderName),
		ProtocolType:             strings.TrimSpace(req.ProtocolType),
		Issuer:                   strings.TrimSpace(req.Issuer),
		AuthorizationEndpoint:    strings.TrimSpace(req.AuthorizationEndpoint),
		TokenEndpoint:            strings.TrimSpace(req.TokenEndpoint),
		UserinfoEndpoint:         strings.TrimSpace(req.UserinfoEndpoint),
		JWKSURI:                  strings.TrimSpace(req.JWKSURI),
		ClientID:                 strings.TrimSpace(req.ClientID),
		Scopes:                   append([]string(nil), req.Scopes...),
		RedirectURI:              strings.TrimSpace(req.RedirectURI),
		DisplayName:              strings.TrimSpace(req.DisplayName),
		Icon:                     strings.TrimSpace(req.Icon),
		SortOrder:                req.SortOrder,
		DisplayEnabled:           req.DisplayEnabled,
		LoginEnabled:             req.LoginEnabled,
		BindEnabled:              req.BindEnabled,
		EmailAutoBindEnabled:     req.EmailAutoBindEnabled,
		AccountAutoCreateEnabled: req.AccountAutoCreateEnabled,
		Status:                   domain.ProviderStatusActive,
		MetadataJSON:             strings.TrimSpace(req.MetadataJSON),
	}
}

func providerFromUpdateRequest(req facade.ProviderUpdateRequest, code string) domain.Provider {
	return domain.Provider{
		ProviderCode:             code,
		ProviderName:             strings.TrimSpace(req.ProviderName),
		ProtocolType:             strings.TrimSpace(req.ProtocolType),
		Issuer:                   strings.TrimSpace(req.Issuer),
		AuthorizationEndpoint:    strings.TrimSpace(req.AuthorizationEndpoint),
		TokenEndpoint:            strings.TrimSpace(req.TokenEndpoint),
		UserinfoEndpoint:         strings.TrimSpace(req.UserinfoEndpoint),
		JWKSURI:                  strings.TrimSpace(req.JWKSURI),
		ClientID:                 strings.TrimSpace(req.ClientID),
		Scopes:                   append([]string(nil), req.Scopes...),
		RedirectURI:              strings.TrimSpace(req.RedirectURI),
		DisplayName:              strings.TrimSpace(req.DisplayName),
		Icon:                     strings.TrimSpace(req.Icon),
		SortOrder:                req.SortOrder,
		DisplayEnabled:           req.DisplayEnabled,
		LoginEnabled:             req.LoginEnabled,
		BindEnabled:              req.BindEnabled,
		EmailAutoBindEnabled:     req.EmailAutoBindEnabled,
		AccountAutoCreateEnabled: req.AccountAutoCreateEnabled,
		Status:                   domain.ProviderStatusActive,
		MetadataJSON:             strings.TrimSpace(req.MetadataJSON),
	}
}
