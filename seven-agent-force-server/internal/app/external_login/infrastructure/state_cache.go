package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
)

const loginStateCacheKeyPrefix = "external-login:state:"

type StateCache struct {
	cache cache.Manager
}

func NewStateCache(cacheManager cache.Manager) *StateCache {
	return &StateCache{cache: cacheManager}
}

func (c *StateCache) Put(ctx context.Context, item domain.LoginState, ttl time.Duration) error {
	if c == nil || c.cache == nil {
		return nil
	}
	payload := loginStateCachePayload{
		ProviderCode:            item.ProviderCode,
		PlatformCode:            item.PlatformCode,
		ProvisioningAuthorityID: item.ProvisioningAuthorityID,
		StateHash:               item.StateHash,
		NonceHash:               item.NonceHash,
		CodeVerifierCiphertext:  item.CodeVerifierCiphertext,
		CodeVerifierEDEK:        item.CodeVerifierEDEK,
		CodeVerifierWrapKeyRef:  item.CodeVerifierWrapKeyRef,
		Issuer:                  item.Issuer,
		RedirectURI:             item.RedirectURI,
		LoginTransactionID:      item.LoginTransactionID,
		RedirectAfterLogin:      item.RedirectAfterLogin,
		Expiration:              item.ExpiresAt,
	}
	if err := c.cache.Set(ctx, loginStateCacheKey(item.StateID), payload, ttl); err != nil {
		if errors.Is(err, cache.ErrCacheLayerUnsupported) {
			return nil
		}
		return fmt.Errorf("put external login state cache: %w", err)
	}
	return nil
}

func (c *StateCache) Get(ctx context.Context, stateID string) (*domain.LoginState, error) {
	if c == nil || c.cache == nil {
		return nil, nil
	}
	var payload loginStateCachePayload
	hit, err := c.cache.Get(ctx, loginStateCacheKey(stateID), &payload)
	if err != nil {
		if errors.Is(err, cache.ErrCacheLayerUnsupported) {
			return nil, nil
		}
		return nil, fmt.Errorf("get external login state cache: %w", err)
	}
	if !hit {
		return nil, nil
	}
	return &domain.LoginState{
		StateID:                 stateID,
		ProviderCode:            payload.ProviderCode,
		PlatformCode:            payload.PlatformCode,
		ProvisioningAuthorityID: payload.ProvisioningAuthorityID,
		LoginTransactionID:      payload.LoginTransactionID,
		RedirectAfterLogin:      payload.RedirectAfterLogin,
		StateHash:               payload.StateHash,
		NonceHash:               payload.NonceHash,
		CodeVerifierCiphertext:  payload.CodeVerifierCiphertext,
		CodeVerifierEDEK:        payload.CodeVerifierEDEK,
		CodeVerifierWrapKeyRef:  payload.CodeVerifierWrapKeyRef,
		Issuer:                  payload.Issuer,
		RedirectURI:             payload.RedirectURI,
		ExpiresAt:               payload.Expiration,
	}, nil
}

func (c *StateCache) Delete(ctx context.Context, stateID string) error {
	if c == nil || c.cache == nil {
		return nil
	}
	if err := c.cache.Delete(ctx, loginStateCacheKey(stateID)); err != nil {
		if errors.Is(err, cache.ErrCacheLayerUnsupported) {
			return nil
		}
		return fmt.Errorf("delete external login state cache: %w", err)
	}
	return nil
}

type loginStateCachePayload struct {
	ProviderCode            string    `json:"providerCode"`
	PlatformCode            string    `json:"platformCode,omitempty"`
	ProvisioningAuthorityID string    `json:"provisioningAuthorityId,omitempty"`
	StateHash               string    `json:"stateHash"`
	NonceHash               string    `json:"nonceHash,omitempty"`
	CodeVerifierCiphertext  string    `json:"codeVerifierCiphertext,omitempty"`
	CodeVerifierEDEK        string    `json:"codeVerifierEdek,omitempty"`
	CodeVerifierWrapKeyRef  string    `json:"codeVerifierWrapKeyRef,omitempty"`
	Issuer                  string    `json:"issuer,omitempty"`
	RedirectURI             string    `json:"redirectUri"`
	LoginTransactionID      string    `json:"loginTransactionId,omitempty"`
	RedirectAfterLogin      string    `json:"redirectAfterLogin,omitempty"`
	Expiration              time.Time `json:"expiration"`
}

func loginStateCacheKey(stateID string) string {
	return loginStateCacheKeyPrefix + stateID
}
