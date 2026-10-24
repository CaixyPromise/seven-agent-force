package domain

import "context"

// Repository defines platform policy persistence required by the domain-facing application layer.
type Repository interface {
	ListActivePlatforms(ctx context.Context) ([]Platform, error)
	ListPlatforms(ctx context.Context, query PlatformQuery) ([]Platform, int64, error)
	FindPlatform(ctx context.Context, platformCode string) (*Platform, error)
	ListActiveSSOClientBindings(ctx context.Context) ([]SSOClientBinding, error)
	ListActiveSourceRules(ctx context.Context) ([]SourceRule, error)
	ListLoginMethods(ctx context.Context, platformCode string) ([]LoginMethod, error)
	ListSourceRules(ctx context.Context, platformCode string) ([]SourceRule, error)
	ListDefaultRoleRecords(ctx context.Context, platformCode string) ([]DefaultRole, error)
	ListDefaultRoles(ctx context.Context, platformCode string, maxCount int) ([]DefaultRole, error)
	FindDefaultPlatform(ctx context.Context) (*Platform, error)
	FindManagedDefaultPlatform(ctx context.Context) (*Platform, error)
	FindManagedDefaultPlatformForUpdate(ctx context.Context) (*Platform, error)
	ListManagedLoginMethods(ctx context.Context, platformCode string) ([]LoginMethod, error)
	ListManagedLoginMethodsForUpdate(ctx context.Context, platformCode string) ([]LoginMethod, error)
	ListManagedSourceRules(ctx context.Context, platformCode string) ([]SourceRule, error)
	ListManagedSourceRulesForUpdate(ctx context.Context, platformCode string) ([]SourceRule, error)
	InsertPlatform(ctx context.Context, platform Platform, actorID int64) error
	UpdatePlatform(ctx context.Context, platform Platform, actorID int64) error
	UpdatePlatformStatus(ctx context.Context, platformCode string, status int, actorID int64) error
	ReplaceLoginMethods(ctx context.Context, platformCode string, methods []LoginMethod, actorID int64) error
	ReplaceSourceRules(ctx context.Context, platformCode string, rules []SourceRule, actorID int64) error
	ReplaceDefaultRoles(ctx context.Context, platformCode string, roles []DefaultRole, actorID int64) error
	ListAvailableExternalProviderCodes(ctx context.Context, providerCodes []string) ([]string, error)
	ListManagedExternalProviderCodes(ctx context.Context, providerCodes []string) ([]string, error)
	ValidateDefaultRoles(ctx context.Context, roleIDs []int64) ([]RoleSafety, error)
}

type PlatformQuery struct {
	Keyword      string
	PlatformCode string
	PlatformType string
	Status       *int
	Current      int
	PageSize     int
}

type RoleSafety struct {
	RoleID          int64
	Exists          bool
	Active          bool
	AutoAssignable  bool
	PermissionCodes []string
	MenuPermissions []string
}
