package application

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
)

func TestTokenVaultEncryptsAndDecryptsTokenSet(t *testing.T) {
	repo := newFakeTokenVaultRepository()
	vault := NewTokenVaultService(repo, fakeSecretValueService{}, nil)
	expiresAt := time.Date(2026, 6, 21, 13, 0, 0, 0, time.UTC)

	token, err := vault.StoreTokenSet(context.Background(), &domain.OAuthToken{
		ProviderCode: "github",
		IdentityID:   10,
		UserID:       20,
		TokenPurpose: domain.TokenPurposeLogin,
		ScopeHash:    "scope-hash",
		Status:       domain.TokenStatusActive,
	}, domain.TokenSet{
		AccessToken:  "access-live",
		RefreshToken: "refresh-live",
		TokenType:    "Bearer",
		Scopes:       []string{"read:user"},
		ExpiresAt:    &expiresAt,
	})
	if err != nil {
		t.Fatalf("store token set: %v", err)
	}
	if token.TokenSetCiphertext == "" || strings.Contains(token.TokenSetCiphertext, "access-live") {
		t.Fatalf("token set was not encrypted: %#v", token)
	}

	lease, err := vault.AcquireAccessToken(context.Background(), facade.AcquireAccessTokenRequest{
		ProviderCode: "github",
		IdentityID:   10,
		UserID:       20,
		TokenPurpose: domain.TokenPurposeLogin,
	})
	if err != nil {
		t.Fatalf("acquire access token: %v", err)
	}
	if lease.AccessToken != "access-live" || lease.TokenType != "Bearer" || lease.ExpiresAt == nil || !lease.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("unexpected lease: %#v", lease)
	}
}

func TestTokenVaultListRecordsNeverExposeTokenPlaintext(t *testing.T) {
	repo := newFakeTokenVaultRepository()
	vault := NewTokenVaultService(repo, fakeSecretValueService{}, nil)
	if _, err := vault.StoreTokenSet(context.Background(), &domain.OAuthToken{
		ProviderCode: "google",
		IdentityID:   11,
		UserID:       21,
		TokenPurpose: domain.TokenPurposeAPI,
		ScopeHash:    "scope-hash",
		Status:       domain.TokenStatusActive,
	}, domain.TokenSet{AccessToken: "plain-access", RefreshToken: "plain-refresh"}); err != nil {
		t.Fatalf("store token set: %v", err)
	}

	page, err := vault.ListTokenRecords(context.Background(), facade.TokenQuery{ProviderCode: "google"})
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	rendered := fmt.Sprintf("%#v", page)
	if strings.Contains(rendered, "plain-access") || strings.Contains(rendered, "plain-refresh") {
		t.Fatalf("token plaintext leaked through list records: %s", rendered)
	}
	if len(page.Records) != 1 || page.Records[0].ProviderCode != "google" {
		t.Fatalf("unexpected page: %#v", page)
	}
}

func TestTokenVaultStoreTokenSetUpdatesExistingActiveToken(t *testing.T) {
	repo := newFakeTokenVaultRepository()
	vault := NewTokenVaultService(repo, fakeSecretValueService{}, nil)

	first, err := vault.StoreTokenSet(context.Background(), &domain.OAuthToken{
		ProviderCode: "github",
		IdentityID:   10,
		UserID:       20,
		TokenPurpose: domain.TokenPurposeLogin,
		Scopes:       []string{"read:user"},
		ScopeHash:    "scope-hash",
		Status:       domain.TokenStatusActive,
		Version:      1,
	}, domain.TokenSet{AccessToken: "first-access", Scopes: []string{"read:user"}})
	if err != nil {
		t.Fatalf("store first token set: %v", err)
	}

	second, err := vault.StoreTokenSet(context.Background(), &domain.OAuthToken{
		ProviderCode: "github",
		IdentityID:   10,
		UserID:       20,
		TokenPurpose: domain.TokenPurposeLogin,
		Scopes:       []string{"read:user"},
		ScopeHash:    "scope-hash",
		Status:       domain.TokenStatusActive,
		Version:      1,
	}, domain.TokenSet{AccessToken: "second-access", Scopes: []string{"read:user"}})
	if err != nil {
		t.Fatalf("store second token set: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected existing token to be updated, got first id %d second id %d", first.ID, second.ID)
	}
	if len(repo.tokens) != 1 {
		t.Fatalf("expected one token record after repeat store, got %d", len(repo.tokens))
	}

	lease, err := vault.AcquireAccessToken(context.Background(), facade.AcquireAccessTokenRequest{
		ProviderCode: "github",
		IdentityID:   10,
		UserID:       20,
		TokenPurpose: domain.TokenPurposeLogin,
	})
	if err != nil {
		t.Fatalf("acquire access token: %v", err)
	}
	if lease.AccessToken != "second-access" {
		t.Fatalf("expected updated access token, got %q", lease.AccessToken)
	}
}

func TestTokenVaultAcquireAccessTokenRejectsRevokedToken(t *testing.T) {
	repo := newFakeTokenVaultRepository()
	vault := NewTokenVaultService(repo, fakeSecretValueService{}, nil)
	if _, err := vault.StoreTokenSet(context.Background(), &domain.OAuthToken{
		ProviderCode: "github",
		IdentityID:   12,
		UserID:       22,
		TokenPurpose: domain.TokenPurposeLogin,
		ScopeHash:    "scope-hash",
		Status:       domain.TokenStatusRevoked,
	}, domain.TokenSet{AccessToken: "revoked-access"}); err != nil {
		t.Fatalf("store token set: %v", err)
	}

	_, err := vault.AcquireAccessToken(context.Background(), facade.AcquireAccessTokenRequest{
		ProviderCode: "github",
		IdentityID:   12,
		UserID:       22,
		TokenPurpose: domain.TokenPurposeLogin,
	})
	if err == nil {
		t.Fatal("expected revoked token to be rejected")
	}
}

func TestTokenVaultRefreshUsesVersionGuard(t *testing.T) {
	repo := newFakeTokenVaultRepository()
	vault := NewTokenVaultService(repo, fakeSecretValueService{}, nil)
	token, err := vault.StoreTokenSet(context.Background(), &domain.OAuthToken{
		ProviderCode: "github",
		IdentityID:   13,
		UserID:       23,
		TokenPurpose: domain.TokenPurposeLogin,
		ScopeHash:    "scope-hash",
		Status:       domain.TokenStatusActive,
		Version:      3,
	}, domain.TokenSet{AccessToken: "old-access"})
	if err != nil {
		t.Fatalf("store token set: %v", err)
	}

	updated, err := vault.RefreshTokenSet(context.Background(), token, domain.TokenSet{AccessToken: "new-access"}, 2)
	if err != nil {
		t.Fatalf("refresh stale token set: %v", err)
	}
	if updated {
		t.Fatal("expected stale expectedVersion to fail optimistic guard")
	}
	if repo.lastExpectedVersion != 2 {
		t.Fatalf("expected repository guard version 2, got %d", repo.lastExpectedVersion)
	}

	updated, err = vault.RefreshTokenSet(context.Background(), token, domain.TokenSet{AccessToken: "new-access"}, 3)
	if err != nil {
		t.Fatalf("refresh token set: %v", err)
	}
	if !updated {
		t.Fatal("expected matching expectedVersion to update")
	}
}

func TestTokenVaultRefreshAfterRevokeDoesNotReactivateToken(t *testing.T) {
	repo := newFakeTokenVaultRepository()
	vault := NewTokenVaultService(repo, fakeSecretValueService{}, nil)
	token, err := vault.StoreTokenSet(context.Background(), &domain.OAuthToken{
		ProviderCode: "github",
		IdentityID:   15,
		UserID:       25,
		TokenPurpose: domain.TokenPurposeLogin,
		ScopeHash:    "scope-hash",
		Status:       domain.TokenStatusActive,
		Version:      4,
	}, domain.TokenSet{AccessToken: "old-access"})
	if err != nil {
		t.Fatalf("store token set: %v", err)
	}
	if _, err := repo.RevokeToken(context.Background(), token.ID, time.Date(2026, 6, 21, 14, 0, 0, 0, time.UTC), "manual revoke"); err != nil {
		t.Fatalf("revoke token: %v", err)
	}

	updated, err := vault.RefreshTokenSet(context.Background(), token, domain.TokenSet{AccessToken: "new-access"}, 4)
	if err != nil {
		t.Fatalf("refresh after revoke: %v", err)
	}
	if updated {
		t.Fatal("expected refresh after revoke to be rejected")
	}
	current := repo.tokens[token.ID]
	if current.Status != domain.TokenStatusRevoked || current.RevokedAt == nil {
		t.Fatalf("refresh reactivated revoked token: %#v", current)
	}
}

func TestTokenVaultRevokeTokenRequiresStepUpProof(t *testing.T) {
	repo := newFakeTokenVaultRepository()
	vault := NewTokenVaultService(repo, fakeSecretValueService{}, nil)
	if _, err := vault.StoreTokenSet(context.Background(), &domain.OAuthToken{
		ProviderCode: "github",
		IdentityID:   14,
		UserID:       24,
		TokenPurpose: domain.TokenPurposeLogin,
		ScopeHash:    "scope-hash",
		Status:       domain.TokenStatusActive,
	}, domain.TokenSet{AccessToken: "access"}); err != nil {
		t.Fatalf("store token set: %v", err)
	}

	if err := vault.RevokeToken(context.Background(), 100, 1, "manual revoke", stepup.ProofMetadata{}); err == nil {
		t.Fatal("expected missing step-up proof to reject revoke")
	}
	if repo.revokeCount != 0 {
		t.Fatalf("repository revoke called without proof: %d", repo.revokeCount)
	}

	proof := stepup.ProofMetadata{
		BusinessAction:        StepUpActionExternalOAuthTokenRevoke,
		OperationBinding:      BuildTokenRevokeOperationBinding(1),
		ProofIdentifier:       "proof-1",
		ChallengeIdentifier:   "challenge-1",
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: []string{"TOTP"},
	}
	if err := vault.RevokeToken(context.Background(), 100, 1, "manual revoke", proof); err != nil {
		t.Fatalf("revoke with proof: %v", err)
	}
	if repo.revokeCount != 1 {
		t.Fatalf("expected repository revoke after proof, got %d", repo.revokeCount)
	}
}

type fakeSecretValueService struct{}

func (fakeSecretValueService) EncryptString(ctx context.Context, plain string) (EncryptedSecretValue, error) {
	return fakeSecretValueService{}.EncryptBytes(ctx, []byte(plain))
}

func (fakeSecretValueService) DecryptString(ctx context.Context, value EncryptedSecretValue) (string, error) {
	plain, err := fakeSecretValueService{}.DecryptBytes(ctx, value)
	return string(plain), err
}

func (fakeSecretValueService) EncryptBytes(_ context.Context, plain []byte) (EncryptedSecretValue, error) {
	return EncryptedSecretValue{
		CiphertextB64: "cipher:" + base64.StdEncoding.EncodeToString(plain),
		EDEKB64:       "edek",
		WrapKeyRef:    "test-key",
	}, nil
}

func (fakeSecretValueService) DecryptBytes(_ context.Context, value EncryptedSecretValue) ([]byte, error) {
	return base64.StdEncoding.DecodeString(strings.TrimPrefix(value.CiphertextB64, "cipher:"))
}

type fakeTokenVaultRepository struct {
	nextID              int64
	tokens              map[int64]domain.OAuthToken
	lastExpectedVersion int
	revokeCount         int
}

func newFakeTokenVaultRepository() *fakeTokenVaultRepository {
	return &fakeTokenVaultRepository{
		nextID: 1,
		tokens: make(map[int64]domain.OAuthToken),
	}
}

func (f *fakeTokenVaultRepository) InsertToken(_ context.Context, item *domain.OAuthToken) error {
	item.ID = f.nextID
	f.nextID++
	f.tokens[item.ID] = *item
	return nil
}

func (f *fakeTokenVaultRepository) FindActiveToken(_ context.Context, providerCode string, identityID int64, userID int64, tokenPurpose string, scopeHash string) (*domain.OAuthToken, error) {
	for _, item := range f.tokens {
		if item.ProviderCode == providerCode &&
			item.IdentityID == identityID &&
			item.UserID == userID &&
			item.TokenPurpose == tokenPurpose &&
			item.ScopeHash == scopeHash &&
			item.Status == domain.TokenStatusActive &&
			item.RevokedAt == nil {
			copy := item
			return &copy, nil
		}
	}
	return nil, nil
}

func (f *fakeTokenVaultRepository) UpdateTokenSet(_ context.Context, item *domain.OAuthToken, expectedVersion int) (bool, error) {
	f.lastExpectedVersion = expectedVersion
	current, ok := f.tokens[item.ID]
	if !ok || current.Version != expectedVersion || current.Status != domain.TokenStatusActive || current.RevokedAt != nil {
		return false, nil
	}
	item.Version = expectedVersion + 1
	f.tokens[item.ID] = *item
	return true, nil
}

func (f *fakeTokenVaultRepository) RevokeToken(_ context.Context, tokenID int64, now time.Time, reason string) (bool, error) {
	item, ok := f.tokens[tokenID]
	if !ok || item.Status != domain.TokenStatusActive {
		return false, nil
	}
	item.Status = domain.TokenStatusRevoked
	item.RevokedAt = &now
	item.Version++
	item.MetadataJSON = reason
	f.tokens[tokenID] = item
	f.revokeCount++
	return true, nil
}

func (f *fakeTokenVaultRepository) ListTokens(_ context.Context, query domain.TokenQuery) ([]domain.OAuthToken, int64, error) {
	items := make([]domain.OAuthToken, 0, len(f.tokens))
	for _, item := range f.tokens {
		if query.ProviderCode != "" && item.ProviderCode != query.ProviderCode {
			continue
		}
		if query.IdentityID != nil && item.IdentityID != *query.IdentityID {
			continue
		}
		if query.UserID != nil && item.UserID != *query.UserID {
			continue
		}
		if query.TokenPurpose != "" && item.TokenPurpose != query.TokenPurpose {
			continue
		}
		if query.Status != nil && item.Status != *query.Status {
			continue
		}
		items = append(items, item)
	}
	return items, int64(len(items)), nil
}
