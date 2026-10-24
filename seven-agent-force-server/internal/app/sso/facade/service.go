package facade

import (
	"context"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
)

type AuthorizationSessionFacade interface {
	CreateAuthorizationSession(ctx context.Context, request CreateAuthorizationSessionRequest) (*AuthorizationSessionSnapshot, error)
	GetAuthorizationSession(ctx context.Context, loginTransactionID string) (*AuthorizationSessionSnapshot, error)
	RemoveAuthorizationSession(ctx context.Context, loginTransactionID string) error
	AcquireCompletionLock(ctx context.Context, loginTransactionID string) (bool, error)
	ReleaseCompletionLock(ctx context.Context, loginTransactionID string) error
	MarkSessionFinalized(ctx context.Context, loginTransactionID string) (bool, error)
	ReleaseSessionFinalized(ctx context.Context, loginTransactionID string) error
}

type AuthenticationCompletionFacade interface {
	CompleteInteractiveAuthentication(ctx context.Context, command CompleteInteractiveAuthenticationCommand) (*AuthenticationCompletionResult, error)
}

type BootstrapSessionFacade interface {
	BootstrapFirstPartySession(ctx context.Context, command BootstrapSessionCommand) (*BootstrapSessionResult, error)
}

type SessionFacade interface {
	ListSessionsByUserID(ctx context.Context, userID int64) ([]SessionRecord, error)
	ListActiveSessions(ctx context.Context) ([]SessionRecord, error)
	CountActiveSessions(ctx context.Context) (int64, error)
	RevokeSession(ctx context.Context, sessionID string) (bool, error)
	RevokeSessionsByUserID(ctx context.Context, userID int64) (int64, error)
	RevokeSessionsByPlatformCode(ctx context.Context, platformCode string) (int64, error)
	RevokeSessionsByPlatformLoginMethod(ctx context.Context, platformCode string, loginMethod string, externalProviderCode string) (int64, error)
	RevokeSessionsByExternalProvider(ctx context.Context, providerCode string) (int64, error)
	RevokeSessionsByExternalIdentity(ctx context.Context, identityID int64) (int64, error)
	ResolveActiveSessionRecord(ctx context.Context, sessionID string) (*SessionRecord, error)
}

type ManagedSessionFacade interface {
	ListSessionsByUserIDPage(ctx context.Context, userID int64, offset, limit int) ([]SessionRecord, error)
	CountSessionsByUserID(ctx context.Context, userID int64) (int64, error)
	CaptureManagedSessionCutoff(ctx context.Context) (time.Time, error)
	RevokeManagedSession(ctx context.Context, sessionID string) (bool, error)
	RevokeSessionsByUserIDAtOrBefore(ctx context.Context, userID int64, cutoff time.Time) (int64, error)
}

type SessionValidator interface {
	ResolveActiveSessionRecord(ctx context.Context, sessionID string) (*SessionRecord, error)
}

type TokenFacade interface {
	ValidateAccessToken(ctx context.Context, accessToken string) (*AccessTokenPrincipal, error)
	BuildDiscoveryDocument(ctx context.Context) (map[string]any, error)
	BuildJwksDocument(ctx context.Context) (map[string]any, error)
}

type AuditEventQueryFacade interface {
	ListEventsSince(ctx context.Context, startTime time.Time) ([]AuditEventRecord, error)
}

type ClientQueryFacade interface {
	ListEnabledClients(ctx context.Context) ([]ClientRecord, error)
}

type ClientAdminFacade interface {
	ClientCapabilities(ctx context.Context) map[string]any
	ListClients(ctx context.Context, query ClientAdminQuery) (*ClientAdminPage, error)
	GetClient(ctx context.Context, clientID string) (*ClientAdminDetail, error)
	ListClientRedirectURIs(ctx context.Context, clientID string) ([]ClientRedirectURIRecord, error)
	ListClientSecrets(ctx context.Context, clientID string) ([]ClientSecretSummaryRecord, error)
	CreateClient(ctx context.Context, actorID int64, request ClientAdminSaveRequest, proof stepup.ProofMetadata) (*ClientAdminDetail, error)
	UpdateClient(ctx context.Context, actorID int64, clientID string, request UpdateClientAdminRequest, proof stepup.ProofMetadata) (*ClientAdminDetail, error)
	UpdateClientStatus(ctx context.Context, actorID int64, clientID string, request ClientStatusRequest, proof stepup.ProofMetadata) error
	UpdateClientRedirectURIs(ctx context.Context, actorID int64, clientID string, request ClientRedirectURIUpdateRequest, proof stepup.ProofMetadata) ([]ClientRedirectURIRecord, error)
	GenerateClientSecret(ctx context.Context, actorID int64, clientID string, request ClientSecretGenerateRequest, proof stepup.ProofMetadata) (*ClientSecretGenerateResponse, error)
	DisableClientSecret(ctx context.Context, actorID int64, clientID string, secretID int64, request ClientSecretStatusRequest, proof stepup.ProofMetadata) error
}

// ManagedClientFacade exposes only system-owned confidential OIDC client provisioning.
type ManagedClientFacade interface {
	UpsertManagedClient(ctx context.Context, command ManagedClientCommand) (*ManagedClientResult, error)
	SetManagedClientStatus(ctx context.Context, command ManagedClientStatusCommand) error
}
