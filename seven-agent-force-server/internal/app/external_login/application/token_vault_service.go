package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/bytedance/sonic"
)

const tokenRefreshLockTTL = 30 * time.Second

type TokenVaultRepository interface {
	InsertToken(ctx context.Context, item *domain.OAuthToken) error
	FindActiveToken(ctx context.Context, providerCode string, identityID int64, userID int64, tokenPurpose string, scopeHash string) (*domain.OAuthToken, error)
	UpdateTokenSet(ctx context.Context, item *domain.OAuthToken, expectedVersion int) (bool, error)
	RevokeToken(ctx context.Context, tokenID int64, now time.Time, reason string) (bool, error)
	ListTokens(ctx context.Context, query domain.TokenQuery) ([]domain.OAuthToken, int64, error)
}

type EncryptedSecretValue struct {
	CiphertextB64 string
	EDEKB64       string
	WrapKeyRef    string
}

type SecretValueService interface {
	EncryptString(ctx context.Context, plain string) (EncryptedSecretValue, error)
	DecryptString(ctx context.Context, value EncryptedSecretValue) (string, error)
	EncryptBytes(ctx context.Context, plain []byte) (EncryptedSecretValue, error)
	DecryptBytes(ctx context.Context, value EncryptedSecretValue) ([]byte, error)
}

type RefreshLockCache interface {
	SetNXString(ctx context.Context, cacheKey string, value string, ttl time.Duration) (bool, error)
	Delete(ctx context.Context, cacheKey string) error
}

type TokenVaultService struct {
	repo    TokenVaultRepository
	secrets SecretValueService
	cache   RefreshLockCache
	now     func() time.Time
}

func NewTokenVaultService(repo TokenVaultRepository, secrets SecretValueService, cacheManager RefreshLockCache) *TokenVaultService {
	return &TokenVaultService{
		repo:    repo,
		secrets: secrets,
		cache:   cacheManager,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

// StoreTokenSet encrypts a token set and stores the corresponding vault record.
func (s *TokenVaultService) StoreTokenSet(ctx context.Context, item *domain.OAuthToken, tokenSet domain.TokenSet) (*domain.OAuthToken, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("external oauth token vault repository is not configured")
	}
	if item == nil {
		return nil, fmt.Errorf("external oauth token is required")
	}
	copy := *item
	if copy.AccessExpiresAt == nil && tokenSet.ExpiresAt != nil {
		copy.AccessExpiresAt = tokenSet.ExpiresAt
	}
	if len(copy.Scopes) == 0 && len(tokenSet.Scopes) > 0 {
		copy.Scopes = append([]string(nil), tokenSet.Scopes...)
	}
	if strings.TrimSpace(copy.ScopeHash) == "" && len(copy.Scopes) > 0 {
		copy.ScopeHash = scopeHash(copy.Scopes)
	}
	existing, err := s.repo.FindActiveToken(ctx, copy.ProviderCode, copy.IdentityID, copy.UserID, copy.TokenPurpose, copy.ScopeHash)
	if err != nil {
		return nil, err
	}
	if err := s.encryptTokenSet(ctx, &copy, tokenSet); err != nil {
		return nil, err
	}
	if existing != nil {
		now := s.now()
		copy.ID = existing.ID
		copy.Version = existing.Version + 1
		copy.LastRefreshAt = &now
		updated, err := s.repo.UpdateTokenSet(ctx, &copy, existing.Version)
		if err != nil {
			return nil, err
		}
		if !updated {
			return nil, fmt.Errorf("external oauth token was changed, please retry")
		}
		return &copy, nil
	}
	if err := s.repo.InsertToken(ctx, &copy); err != nil {
		return nil, err
	}
	return &copy, nil
}

// AcquireAccessToken returns a decrypted access token lease for an active vault record.
func (s *TokenVaultService) AcquireAccessToken(ctx context.Context, req facade.AcquireAccessTokenRequest) (*facade.AccessTokenLease, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("external oauth token vault repository is not configured")
	}
	status := domain.TokenStatusActive
	identityID := req.IdentityID
	userID := req.UserID
	items, _, err := s.repo.ListTokens(ctx, domain.TokenQuery{
		ProviderCode: strings.TrimSpace(req.ProviderCode),
		IdentityID:   &identityID,
		UserID:       &userID,
		TokenPurpose: strings.TrimSpace(req.TokenPurpose),
		Status:       &status,
		Current:      1,
		PageSize:     1,
	})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("active external oauth token is not available")
	}
	item := items[0]
	if item.Status != domain.TokenStatusActive || item.RevokedAt != nil {
		return nil, fmt.Errorf("external oauth token is not active")
	}
	tokenSet, err := s.decryptTokenSet(ctx, item)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(tokenSet.AccessToken) == "" {
		return nil, fmt.Errorf("external oauth access token is empty")
	}
	return &facade.AccessTokenLease{
		TokenID:      item.ID,
		ProviderCode: item.ProviderCode,
		IdentityID:   item.IdentityID,
		UserID:       item.UserID,
		AccessToken:  tokenSet.AccessToken,
		TokenType:    tokenSet.TokenType,
		Scopes:       append([]string(nil), tokenSet.Scopes...),
		ExpiresAt:    tokenSet.ExpiresAt,
	}, nil
}

// ListTokenRecords returns admin-facing token metadata without decrypting token values.
func (s *TokenVaultService) ListTokenRecords(ctx context.Context, query facade.TokenQuery) (*facade.TokenPage, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("external oauth token vault repository is not configured")
	}
	domainQuery := domain.TokenQuery{
		ProviderCode: strings.TrimSpace(query.ProviderCode),
		IdentityID:   query.IdentityID,
		UserID:       query.UserID,
		TokenPurpose: strings.TrimSpace(query.TokenPurpose),
		Status:       query.Status,
		Current:      query.Current,
		PageSize:     query.PageSize,
	}
	items, total, err := s.repo.ListTokens(ctx, domainQuery)
	if err != nil {
		return nil, err
	}
	current, pageSize := normalizePage(query.Current, query.PageSize)
	records := make([]facade.OAuthTokenRecord, 0, len(items))
	for _, item := range items {
		records = append(records, mapTokenRecord(item))
	}
	return &facade.TokenPage{
		Records:  records,
		Total:    total,
		Current:  current,
		PageSize: pageSize,
	}, nil
}

// RefreshTokenSet encrypts a refreshed token set and updates it with an optimistic version guard.
func (s *TokenVaultService) RefreshTokenSet(ctx context.Context, item *domain.OAuthToken, tokenSet domain.TokenSet, expectedVersion int) (bool, error) {
	if s == nil || s.repo == nil {
		return false, fmt.Errorf("external oauth token vault repository is not configured")
	}
	if item == nil {
		return false, fmt.Errorf("external oauth token is required")
	}
	release, err := s.acquireRefreshLock(ctx, item.ID)
	if err != nil {
		return false, err
	}
	if release != nil {
		defer release()
	}
	copy := *item
	if err := s.encryptTokenSet(ctx, &copy, tokenSet); err != nil {
		return false, err
	}
	now := s.now()
	copy.LastRefreshAt = &now
	if tokenSet.ExpiresAt != nil {
		copy.AccessExpiresAt = tokenSet.ExpiresAt
	}
	copy.Version = expectedVersion + 1
	return s.repo.UpdateTokenSet(ctx, &copy, expectedVersion)
}

// RevokeToken requires privileged step-up proof before revoking a single token record.
func (s *TokenVaultService) RevokeToken(ctx context.Context, actorID int64, tokenID int64, reason string, proof stepup.ProofMetadata) error {
	if err := stepup.Require(proof, StepUpActionExternalOAuthTokenRevoke, BuildTokenRevokeOperationBinding(tokenID)); err != nil {
		return err
	}
	if s == nil || s.repo == nil {
		return fmt.Errorf("external oauth token vault repository is not configured")
	}
	_ = actorID
	_, err := s.repo.RevokeToken(ctx, tokenID, s.now(), reason)
	return err
}

func (s *TokenVaultService) encryptTokenSet(ctx context.Context, item *domain.OAuthToken, tokenSet domain.TokenSet) error {
	if s == nil || s.secrets == nil {
		return fmt.Errorf("external oauth token vault secret service is not configured")
	}
	raw, err := sonic.Marshal(tokenSet)
	if err != nil {
		return fmt.Errorf("marshal external oauth token set: %w", err)
	}
	encrypted, err := s.secrets.EncryptBytes(ctx, raw)
	if err != nil {
		return fmt.Errorf("encrypt external oauth token set: %w", err)
	}
	item.TokenSetCiphertext = encrypted.CiphertextB64
	item.TokenSetEDEK = encrypted.EDEKB64
	item.TokenSetWrapKeyRef = encrypted.WrapKeyRef
	if item.Status == 0 {
		item.Status = domain.TokenStatusActive
	}
	return nil
}

func (s *TokenVaultService) decryptTokenSet(ctx context.Context, item domain.OAuthToken) (domain.TokenSet, error) {
	if s == nil || s.secrets == nil {
		return domain.TokenSet{}, fmt.Errorf("external oauth token vault secret service is not configured")
	}
	raw, err := s.secrets.DecryptBytes(ctx, EncryptedSecretValue{
		CiphertextB64: item.TokenSetCiphertext,
		EDEKB64:       item.TokenSetEDEK,
		WrapKeyRef:    item.TokenSetWrapKeyRef,
	})
	if err != nil {
		return domain.TokenSet{}, fmt.Errorf("decrypt external oauth token set: %w", err)
	}
	var tokenSet domain.TokenSet
	if err := sonic.Unmarshal(raw, &tokenSet); err != nil {
		return domain.TokenSet{}, fmt.Errorf("unmarshal external oauth token set: %w", err)
	}
	return tokenSet, nil
}

func (s *TokenVaultService) acquireRefreshLock(ctx context.Context, tokenID int64) (func(), error) {
	if s == nil || s.cache == nil {
		return nil, nil
	}
	key := fmt.Sprintf("external-login:token-refresh:%d", tokenID)
	ok, err := s.cache.SetNXString(ctx, key, "1", tokenRefreshLockTTL)
	if err != nil {
		return nil, fmt.Errorf("acquire external oauth token refresh lock: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("external oauth token refresh is already in progress")
	}
	return func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.cache.Delete(cleanupCtx, key)
	}, nil
}

func mapTokenRecord(item domain.OAuthToken) facade.OAuthTokenRecord {
	return facade.OAuthTokenRecord{
		ID:               item.ID,
		ProviderCode:     item.ProviderCode,
		IdentityID:       item.IdentityID,
		UserID:           item.UserID,
		TokenPurpose:     item.TokenPurpose,
		Scopes:           append([]string(nil), item.Scopes...),
		ScopeHash:        item.ScopeHash,
		AccessExpiresAt:  item.AccessExpiresAt,
		RefreshExpiresAt: item.RefreshExpiresAt,
		LastRefreshAt:    item.LastRefreshAt,
		RevokedAt:        item.RevokedAt,
		Status:           item.Status,
		Version:          item.Version,
		MetadataJSON:     item.MetadataJSON,
		CreateTime:       item.CreateTime,
		UpdateTime:       item.UpdateTime,
	}
}

func normalizePage(current, pageSize int) (int, int) {
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
