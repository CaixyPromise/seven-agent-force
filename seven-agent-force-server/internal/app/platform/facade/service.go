package facade

import (
	"context"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
)

type PublicFacade interface {
	ResolveLoginOptions(ctx context.Context, req ResolvePlatformRequest) (*LoginOptionResult, error)
	ResolvePlatformCode(ctx context.Context, req ResolvePlatformRequest) (string, error)
	ValidateLoginContext(ctx context.Context, loginContextID string, req ResolvePlatformRequest) (*LoginContextValidation, error)
	IssueProvisioningAuthority(ctx context.Context, loginContextID string, req ResolvePlatformRequest) (*ProvisioningAuthority, error)
	GetProvisioningPolicy(ctx context.Context, authority ProvisioningAuthority) (*ProvisioningPolicy, error)
	GetFormRegistrationPolicy(ctx context.Context, platformCode string) (*ProvisioningPolicy, error)
	RequireLoginMethod(ctx context.Context, platformCode string, methodType string, providerCode string) error
}

type AdminFacade interface {
	ListPlatforms(ctx context.Context, query PlatformQuery) (*PlatformPage, error)
	GetPlatform(ctx context.Context, platformCode string) (*PlatformDetail, error)
	CreatePlatform(ctx context.Context, actorID int64, req PlatformSaveRequest, proof stepup.ProofMetadata) (*PlatformDetail, error)
	UpdatePlatform(ctx context.Context, actorID int64, platformCode string, req PlatformSaveRequest, proof stepup.ProofMetadata) (*PlatformDetail, error)
	UpdatePlatformStatus(ctx context.Context, actorID int64, platformCode string, req PlatformStatusRequest, proof stepup.ProofMetadata) error
	ReplaceLoginMethods(ctx context.Context, actorID int64, platformCode string, methods []LoginMethodSaveRequest, proof stepup.ProofMetadata) error
	ReplaceSourceRules(ctx context.Context, actorID int64, platformCode string, rules []SourceRuleSaveRequest, proof stepup.ProofMetadata) error
	ReplaceDefaultRoles(ctx context.Context, actorID int64, platformCode string, roles []DefaultRoleSaveRequest, proof stepup.ProofMetadata) error
}

type ProviderStatusFacade interface {
	ListLoginEnabledProviders(ctx context.Context) (map[string]bool, error)
	IsProviderLoginEnabled(ctx context.Context, providerCode string) (bool, error)
}

// ManagedLoginMethod excludes local IDs and metadata JSON.
type ManagedLoginMethod struct {
	MethodType     string
	ProviderCode   string
	DisplayName    string
	Icon           string
	SortOrder      int
	DisplayEnabled bool
	LoginEnabled   bool
}

// ManagedSourceRule excludes local IDs and metadata JSON.
type ManagedSourceRule struct {
	MatchType  string
	MatchValue string
	Priority   int
	Status     int
}

// ManagedLoginPolicy is the complete safe remotely manageable snapshot.
type ManagedLoginPolicy struct {
	PlatformCode      string
	Status            int
	AllowAutoRegister bool
	AllowFormRegister bool
	LoginMethods      []ManagedLoginMethod
	SourceRules       []ManagedSourceRule
}

// ApplyManagedLoginPolicyCommand applies one absolute safe snapshot.
type ApplyManagedLoginPolicyCommand struct {
	ManagedLoginPolicy
}

// ManagedLoginPolicyFacade reuses local Platform validation and persistence.
type ManagedLoginPolicyFacade interface {
	GetManagedLoginPolicy(ctx context.Context) (*ManagedLoginPolicy, error)
	ApplyManagedLoginPolicy(ctx context.Context, command ApplyManagedLoginPolicyCommand) (int64, error)
}
