package drivers

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
)

type Driver interface {
	Code() string
	Capabilities() domain.ProviderCapability
	BuildAuthorizationURL(ctx context.Context, provider domain.Provider, request AuthorizationRequest) (string, error)
	ExchangeCode(ctx context.Context, provider domain.Provider, request TokenExchangeRequest) (*TokenExchangeResult, error)
	ResolveProfile(ctx context.Context, provider domain.Provider, tokens TokenExchangeResult) (*domain.ExternalProfile, error)
	RefreshToken(ctx context.Context, provider domain.Provider, tokenSet domain.TokenSet) (*domain.TokenSet, error)
	RevokeToken(ctx context.Context, provider domain.Provider, tokenSet domain.TokenSet) error
}

type AuthorizationRequest struct {
	State               string
	Nonce               string
	CodeChallenge       string
	CodeChallengeMethod string
	RedirectURI         string
	Scopes              []string
	Issuer              string
}

type TokenExchangeRequest struct {
	Code           string
	State          string
	CodeVerifier   string
	RedirectURI    string
	ClientSecret   string
	Nonce          string
	ExpectedIssuer string
	CallbackIssuer string
	Scopes         []string
}

type TokenExchangeResult struct {
	TokenSet         domain.TokenSet
	RawTokenResponse string
	ExpectedIssuer   string
	CallbackIssuer   string
	ExpectedNonce    string
}

type OIDCValidationOptions struct {
	Issuer   string
	Audience string
	Nonce    string
	JWKSURI  string
}

type OIDCClaims struct {
	Issuer        string
	Subject       string
	Audience      []string
	ExpiresAt     time.Time
	IssuedAt      time.Time
	Nonce         string
	Email         string
	EmailVerified bool
	Name          string
	Picture       string
	Raw           string
}

type OIDCIDTokenValidator interface {
	ValidateIDToken(ctx context.Context, raw string, options OIDCValidationOptions) (OIDCClaims, error)
}

type Registry struct {
	mu      sync.RWMutex
	drivers map[string]Driver
}

func NewRegistry(drivers ...Driver) *Registry {
	registry := &Registry{drivers: make(map[string]Driver, len(drivers))}
	for _, driver := range drivers {
		_ = registry.Register(driver)
	}
	return registry
}

func (r *Registry) Register(driver Driver) error {
	if driver == nil {
		return fmt.Errorf("external login provider driver is required")
	}
	code := normalizeProviderCode(driver.Code())
	if code == "" {
		return fmt.Errorf("external login provider driver code is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.drivers[code]; exists {
		return fmt.Errorf("external login provider driver %q is already registered", code)
	}
	r.drivers[code] = driver
	return nil
}

func (r *Registry) Get(providerCode string) (Driver, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	driver, ok := r.drivers[normalizeProviderCode(providerCode)]
	return driver, ok
}

func (r *Registry) Capabilities() map[string]domain.ProviderCapability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]domain.ProviderCapability, len(r.drivers))
	for code, driver := range r.drivers {
		result[code] = driver.Capabilities()
	}
	return result
}

func normalizeProviderCode(code string) string {
	return strings.ToLower(strings.TrimSpace(code))
}
