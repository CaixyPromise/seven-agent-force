package application

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/facade"
	platformfacade "github.com/CaixyPromise/seven-agent/force/internal/app/platform/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
)

const externalOAuthLoginMethod = "EXTERNAL_OAUTH"

type RepositoryPort interface {
	TokenVaultRepository
	ListLoginMethods(ctx context.Context) ([]domain.Provider, error)
	FindProvider(ctx context.Context, providerCode string) (*domain.Provider, error)
	FindProviderForUpdate(ctx context.Context, providerCode string) (*domain.Provider, error)
	InsertProvider(ctx context.Context, item *domain.Provider, actorID int64) error
	UpdateProvider(ctx context.Context, item *domain.Provider, actorID int64) error
	UpdateProviderStatus(ctx context.Context, providerCode string, status int, actorID int64, now time.Time) (bool, error)
	ListProviders(ctx context.Context, query domain.ProviderQuery) ([]domain.Provider, int64, error)
	ListProviderMethods(ctx context.Context, providerCode string) ([]domain.ProviderMethod, error)
	ReplaceProviderMethods(ctx context.Context, providerCode string, methods []domain.ProviderMethod) error
	InsertLoginState(ctx context.Context, item *domain.LoginState) error
	ConsumeLoginState(ctx context.Context, stateHash string, now time.Time) (*domain.LoginState, error)
	FindIdentityBySubject(ctx context.Context, providerCode, externalIssuer, externalSubject string) (*domain.ExternalIdentity, error)
	InsertIdentity(ctx context.Context, item *domain.ExternalIdentity, actorID int64) error
	CountIdentitiesByProvider(ctx context.Context, providerCode string) (int64, error)
	FindManagedProviderCommand(ctx context.Context, providerCode, connectionVersion string) (*domain.ManagedProviderCommand, error)
	InsertManagedProviderCommand(ctx context.Context, command *domain.ManagedProviderCommand) error
	ListIdentities(ctx context.Context, query domain.IdentityQuery) ([]domain.ExternalIdentity, int64, error)
	UpdateIdentityStatus(ctx context.Context, identityID int64, status int, actorID int64, now time.Time) (bool, error)
	TouchIdentityLogin(ctx context.Context, identityID int64, profile domain.ExternalProfile, now time.Time) error
	RevokeTokensByProvider(ctx context.Context, providerCode string, now time.Time, reason string) (int64, error)
	RevokeTokensByIdentity(ctx context.Context, identityID int64, now time.Time, reason string) (int64, error)
}

type StateCachePort interface {
	Put(ctx context.Context, item domain.LoginState, ttl time.Duration) error
	Get(ctx context.Context, stateID string) (*domain.LoginState, error)
	Delete(ctx context.Context, stateID string) error
}

type TransactorPort interface {
	Enabled() bool
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

type RandomTokenGenerator interface {
	Token(ctx context.Context) (string, error)
}

type DriverRegistryPort interface {
	Get(providerCode string) (ProviderDriverPort, bool)
	Capabilities() map[string]domain.ProviderCapability
}

type ProviderDriverPort interface {
	BuildAuthorizationURL(ctx context.Context, provider domain.Provider, request AuthorizationRequest) (string, error)
	ExchangeCode(ctx context.Context, provider domain.Provider, request TokenExchangeRequest) (*TokenExchangeResult, error)
	ResolveProfile(ctx context.Context, provider domain.Provider, tokens TokenExchangeResult) (*domain.ExternalProfile, error)
	RevokeToken(ctx context.Context, provider domain.Provider, tokenSet domain.TokenSet) error
}

type OIDCDiscoveryResult struct {
	Issuer                string
	AuthorizationEndpoint string
	TokenEndpoint         string
	UserinfoEndpoint      string
	JWKSURI               string
}

type OIDCDiscoveryPort interface {
	DiscoverOIDC(ctx context.Context, issuer string) (OIDCDiscoveryResult, error)
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

type ServiceDeps struct {
	Config                 config.ExternalLoginConfig
	Transactor             TransactorPort
	IDGen                  *xid.Generator
	Repository             RepositoryPort
	StateCache             StateCachePort
	Drivers                DriverRegistryPort
	Discovery              OIDCDiscoveryPort
	SecretValue            SecretValueService
	IdentityDigester       SystemIdentitySubjectDigester
	Random                 RandomTokenGenerator
	Subjects               userfacade.SubjectFacade
	Profiles               userfacade.ProfileFacade
	AuthorizationSessions  ssofacade.AuthorizationSessionFacade
	AuthenticationComplete ssofacade.AuthenticationCompletionFacade
	BootstrapSession       ssofacade.BootstrapSessionFacade
	Sessions               ssofacade.SessionFacade
	Platform               platformfacade.PublicFacade
}

// SystemIdentitySubjectDigester gives generic OAuth identities the same
// non-enumerable lookup property as organization-scoped enterprise identities.
// The raw provider subject never crosses the application/repository boundary.
type SystemIdentitySubjectDigester interface {
	DigestSystemSubject(ctx context.Context, keyRef, providerCode, issuer, subject string) (digest, resolvedKeyRef string, err error)
}

type Service struct {
	cfg                    config.ExternalLoginConfig
	transactor             TransactorPort
	idGen                  *xid.Generator
	repo                   RepositoryPort
	stateCache             StateCachePort
	drivers                DriverRegistryPort
	discovery              OIDCDiscoveryPort
	secrets                SecretValueService
	identityDigester       SystemIdentitySubjectDigester
	random                 RandomTokenGenerator
	subjects               userfacade.SubjectFacade
	profiles               userfacade.ProfileFacade
	authorizationSessions  ssofacade.AuthorizationSessionFacade
	authenticationComplete ssofacade.AuthenticationCompletionFacade
	bootstrapSession       ssofacade.BootstrapSessionFacade
	sessions               ssofacade.SessionFacade
	platform               platformfacade.PublicFacade
	tokenVault             *TokenVaultService
	now                    func() time.Time
}

func NewService(deps ServiceDeps) *Service {
	service := &Service{
		cfg:                    deps.Config,
		transactor:             deps.Transactor,
		idGen:                  deps.IDGen,
		repo:                   deps.Repository,
		stateCache:             deps.StateCache,
		drivers:                deps.Drivers,
		discovery:              deps.Discovery,
		secrets:                deps.SecretValue,
		identityDigester:       deps.IdentityDigester,
		random:                 deps.Random,
		subjects:               deps.Subjects,
		profiles:               deps.Profiles,
		authorizationSessions:  deps.AuthorizationSessions,
		authenticationComplete: deps.AuthenticationComplete,
		bootstrapSession:       deps.BootstrapSession,
		sessions:               deps.Sessions,
		platform:               deps.Platform,
		now:                    func() time.Time { return time.Now().UTC() },
	}
	service.tokenVault = NewTokenVaultService(deps.Repository, deps.SecretValue, nil)
	return service
}

func (s *Service) ProviderCapabilities(context.Context) facade.ProviderCapabilityCatalog {
	return mapCapabilityCatalog(s.driverCapabilities())
}

func (s *Service) ListProviderCapabilities(context.Context) facade.ProviderCapabilityCatalog {
	return mapCapabilityCatalog(s.driverCapabilities())
}

func (s *Service) ListProviderMethods(ctx context.Context, providerCode string) ([]facade.ProviderMethodDescriptor, error) {
	code, err := domain.NormalizeProviderCode(providerCode)
	if err != nil {
		return nil, err
	}
	methods, err := s.repo.ListProviderMethods(ctx, code)
	if err != nil {
		return nil, err
	}
	result := make([]facade.ProviderMethodDescriptor, 0, len(methods))
	for _, method := range methods {
		result = append(result, mapMethod(method))
	}
	return result, nil
}

func (s *Service) AcquireAccessToken(ctx context.Context, req facade.AcquireAccessTokenRequest) (*facade.AccessTokenLease, error) {
	return s.tokenVault.AcquireAccessToken(ctx, req)
}

func (s *Service) RefreshToken(context.Context, int64) error {
	return fmt.Errorf("external oauth token refresh is not implemented")
}

func (s *Service) ListTokens(ctx context.Context, query facade.TokenQuery) (*facade.TokenPage, error) {
	return s.tokenVault.ListTokenRecords(ctx, query)
}

func (s *Service) RevokeToken(ctx context.Context, actorID int64, tokenID int64, reason string, proof stepup.ProofMetadata) error {
	return s.tokenVault.RevokeToken(ctx, actorID, tokenID, reason, proof)
}

func (s *Service) RevokeTokensByProvider(ctx context.Context, providerCode string, reason string) (int64, error) {
	code, err := domain.NormalizeProviderCode(providerCode)
	if err != nil {
		return 0, err
	}
	return s.repo.RevokeTokensByProvider(ctx, code, s.now(), reason)
}

func (s *Service) RevokeTokensByIdentity(ctx context.Context, identityID int64, reason string) (int64, error) {
	return s.repo.RevokeTokensByIdentity(ctx, identityID, s.now(), reason)
}

func (s *Service) withTransaction(ctx context.Context, fn func(context.Context) error) error {
	if s != nil && s.transactor != nil && s.transactor.Enabled() {
		return s.transactor.WithinTransaction(ctx, fn)
	}
	return fn(ctx)
}

func (s *Service) driverCapabilities() map[string]domain.ProviderCapability {
	if s == nil || s.drivers == nil {
		return domain.BuiltInProviderCapabilities()
	}
	catalog := s.drivers.Capabilities()
	if len(catalog) == 0 {
		return domain.BuiltInProviderCapabilities()
	}
	return catalog
}

func mapCapabilityCatalog(input map[string]domain.ProviderCapability) facade.ProviderCapabilityCatalog {
	result := make(facade.ProviderCapabilityCatalog, len(input))
	for code, item := range input {
		result[code] = facade.ProviderCapabilityDescriptor{
			ProviderCode:  item.ProviderCode,
			DisplayName:   item.DisplayName,
			ProtocolType:  item.ProtocolType,
			Capabilities:  append([]string(nil), item.Capabilities...),
			DefaultScopes: append([]string(nil), item.DefaultScopes...),
		}
	}
	return result
}

func mapProviderDetail(item domain.Provider, methods []domain.ProviderMethod) facade.ProviderDetail {
	detail := facade.ProviderDetail{
		ID:                       item.ID,
		ProviderCode:             item.ProviderCode,
		ProviderName:             item.ProviderName,
		ProtocolType:             item.ProtocolType,
		Issuer:                   item.Issuer,
		AuthorizationEndpoint:    item.AuthorizationEndpoint,
		TokenEndpoint:            item.TokenEndpoint,
		UserinfoEndpoint:         item.UserinfoEndpoint,
		JWKSURI:                  item.JWKSURI,
		ClientID:                 item.ClientID,
		Scopes:                   append([]string(nil), item.Scopes...),
		RedirectURI:              item.RedirectURI,
		DisplayName:              item.DisplayName,
		Icon:                     item.Icon,
		SortOrder:                item.SortOrder,
		DisplayEnabled:           item.DisplayEnabled,
		LoginEnabled:             item.LoginEnabled,
		BindEnabled:              item.BindEnabled,
		EmailAutoBindEnabled:     item.EmailAutoBindEnabled,
		AccountAutoCreateEnabled: item.AccountAutoCreateEnabled,
		Status:                   item.Status,
		MetadataJSON:             item.MetadataJSON,
		CreateTime:               item.CreateTime,
		UpdateTime:               item.UpdateTime,
	}
	for _, method := range methods {
		detail.Methods = append(detail.Methods, mapMethod(method))
	}
	return detail
}

func mapMethod(item domain.ProviderMethod) facade.ProviderMethodDescriptor {
	return facade.ProviderMethodDescriptor{
		ID:             item.ID,
		ProviderCode:   item.ProviderCode,
		MethodKey:      item.MethodKey,
		CapabilityCode: item.CapabilityCode,
		RequiredScopes: append([]string(nil), item.RequiredScopes...),
		Status:         item.Status,
		MetadataJSON:   item.MetadataJSON,
	}
}

func mapIdentity(item domain.ExternalIdentity) facade.ExternalIdentityRecord {
	return facade.ExternalIdentityRecord{
		ID:             item.ID,
		ProviderCode:   item.ProviderCode,
		ExternalIssuer: item.ExternalIssuer,
		// Provider subjects are encrypted operational identities. They are never
		// returned through administration or current-user APIs.
		ExternalSubject: "",
		UserID:          item.UserID,
		ExternalLogin:   item.ExternalLogin,
		ExternalEmail:   item.ExternalEmail,
		EmailVerified:   item.EmailVerified,
		DisplayName:     item.DisplayName,
		AvatarURL:       item.AvatarURL,
		Status:          item.Status,
		FirstLinkedAt:   item.FirstLinkedAt,
		LastLoginAt:     item.LastLoginAt,
		LastVerifiedAt:  item.LastVerifiedAt,
		CreateTime:      item.CreateTime,
		UpdateTime:      item.UpdateTime,
	}
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func scopeHash(scopes []string) string {
	normalized := append([]string(nil), scopes...)
	for i := range normalized {
		normalized[i] = strings.TrimSpace(normalized[i])
	}
	return hashString(strings.Join(normalized, " "))
}
