package facade

import (
	"context"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
)

type LoginMethodFacade interface {
	ListLoginMethods(ctx context.Context, req ListLoginMethodsRequest) ([]LoginMethodRecord, error)
}

type ExternalLoginFlowFacade interface {
	StartExternalLogin(ctx context.Context, req StartExternalLoginRequest) (*StartExternalLoginResult, error)
	CompleteExternalCallback(ctx context.Context, req CompleteExternalCallbackRequest) (*ExternalLoginResult, error)
}

type ProviderAdminFacade interface {
	ProviderCapabilities(ctx context.Context) ProviderCapabilityCatalog
	ListProviders(ctx context.Context, query ProviderQuery) (*ProviderPage, error)
	GetProvider(ctx context.Context, providerCode string) (*ProviderDetail, error)
	CreateProvider(ctx context.Context, actorID int64, req ProviderSaveRequest, proof stepup.ProofMetadata) (*ProviderDetail, error)
	UpdateProvider(ctx context.Context, actorID int64, providerCode string, req ProviderUpdateRequest, proof stepup.ProofMetadata) (*ProviderDetail, error)
	UpdateProviderStatus(ctx context.Context, actorID int64, providerCode string, req ProviderStatusRequest, proof stepup.ProofMetadata) error
	RotateClientSecret(ctx context.Context, actorID int64, providerCode string, req RotateClientSecretRequest, proof stepup.ProofMetadata) error
}

type ManagedOIDCProviderFacade interface {
	ApplyManagedOIDCProvider(ctx context.Context, command ManagedOIDCProviderCommand) error
	DisableManagedOIDCProvider(ctx context.Context, ownerNodeCode, connectionVersion string, targetRevision int64) error
}

type IdentityBindingFacade interface {
	ListIdentities(ctx context.Context, query IdentityQuery) (*IdentityPage, error)
	ListCurrentUserBindings(ctx context.Context, userID int64) ([]CurrentUserBinding, error)
	UpdateIdentityStatus(ctx context.Context, actorID int64, identityID int64, req IdentityStatusRequest, proof stepup.ProofMetadata) error
	ResolveIdentity(ctx context.Context, providerCode, externalSubject string) (*ExternalIdentityRecord, error)
}

type ExternalOAuthTokenFacade interface {
	ListTokens(ctx context.Context, query TokenQuery) (*TokenPage, error)
	AcquireAccessToken(ctx context.Context, req AcquireAccessTokenRequest) (*AccessTokenLease, error)
	RefreshToken(ctx context.Context, tokenID int64) error
	RevokeToken(ctx context.Context, actorID int64, tokenID int64, reason string, proof stepup.ProofMetadata) error
	RevokeTokensByProvider(ctx context.Context, providerCode string, reason string) (int64, error)
	RevokeTokensByIdentity(ctx context.Context, identityID int64, reason string) (int64, error)
}

type CapabilityIndexFacade interface {
	ListProviderCapabilities(ctx context.Context) ProviderCapabilityCatalog
	ListProviderMethods(ctx context.Context, providerCode string) ([]ProviderMethodDescriptor, error)
}
