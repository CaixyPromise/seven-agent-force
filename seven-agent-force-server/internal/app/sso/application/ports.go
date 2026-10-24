package application

import (
	"context"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/sso/domain"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
)

// SessionRepository is the application-owned persistence port. The concrete
// SQL repository is injected by the module; application code must not import
// the infrastructure implementation to orchestrate an SSO use case.
type SessionRepository interface {
	CaptureManagedSessionCutoff(context.Context) (time.Time, error)
	FindClient(context.Context, string) (*domain.Client, error)
	ListEnabledClients(context.Context) ([]domain.Client, error)
	ListClients(context.Context, ssofacade.ClientAdminQuery) ([]domain.Client, int64, error)
	FindClientDetail(context.Context, string) (*domain.Client, error)
	FindClientDetailForUpdate(context.Context, string) (*domain.Client, error)
	InsertClient(context.Context, *domain.Client, int64) error
	UpdateClient(context.Context, *domain.Client, int64) error
	UpdateClientStatus(context.Context, string, int, int64, time.Time) (bool, error)
	ReplaceClientRedirectURIs(context.Context, string, []domain.ClientRedirectURI, int64, time.Time) error
	ListClientRedirectURIs(context.Context, string) ([]domain.ClientRedirectURI, error)
	ListClientSecrets(context.Context, string) ([]domain.ClientSecretSummary, error)
	InsertClientSecret(context.Context, *domain.ClientSecret, int64) error
	UpdateClientSecretStatus(context.Context, string, int64, int, int64, time.Time) (bool, error)
	DisableOtherActiveClientSecrets(context.Context, string, int64, int64, time.Time) (int64, error)
	CountActiveClientSecrets(context.Context, string) (int64, error)
	InsertSession(context.Context, *domain.Session) error
	FindSessionBySessionID(context.Context, string) (*domain.Session, error)
	ListSessionsByUserID(context.Context, int64) ([]domain.Session, error)
	CountSessionsByUserID(context.Context, int64) (int64, error)
	ListSessionsByUserIDPage(context.Context, int64, int, int) ([]domain.Session, error)
	ListActiveSessions(context.Context) ([]domain.Session, error)
	ListActiveSessionsByExternalProviderPage(context.Context, string, time.Time, int64, int) ([]domain.Session, error)
	ListActiveSessionsByPlatformCodePage(context.Context, string, time.Time, int64, int) ([]domain.Session, error)
	ListActiveSessionsByPlatformLoginMethodPage(context.Context, string, string, string, time.Time, int64, int) ([]domain.Session, error)
	ListActiveSessionsByExternalIdentityPage(context.Context, int64, time.Time, int64, int) ([]domain.Session, error)
	ListActiveSessionsByUserIDPage(context.Context, int64, time.Time, int64, int) ([]domain.Session, error)
	ListActiveSessionsByClientIDPage(context.Context, string, time.Time, int64, int) ([]domain.Session, error)
	CountActiveSessions(context.Context) (int64, error)
	TouchSession(context.Context, string, time.Time) error
	RevokeSession(context.Context, string, time.Time) (bool, error)
	RevokeSessionsByUserID(context.Context, int64, time.Time) (int64, error)
	RevokeSessionsByUserIDAtOrBefore(context.Context, int64, time.Time) (int64, error)
	RevokeSessionsByClientID(context.Context, string, time.Time) (int64, error)
	RevokeSessionsByPlatformCode(context.Context, string, time.Time) (int64, error)
	RevokeSessionsByPlatformLoginMethod(context.Context, string, string, string, time.Time) (int64, error)
	RevokeSessionsByExternalProvider(context.Context, string, time.Time) (int64, error)
	RevokeSessionsByExternalIdentity(context.Context, int64, time.Time) (int64, error)
	InsertAuthorizationCode(context.Context, *domain.AuthorizationCode) error
	FindAuthorizationCode(context.Context, string) (*domain.AuthorizationCode, error)
	ConsumeAuthorizationCode(context.Context, string, time.Time) (bool, error)
	InsertRefreshTokenFamily(context.Context, *domain.RefreshTokenFamily) error
	FindRefreshFamilyByCurrentHash(context.Context, string) (*domain.RefreshTokenFamily, error)
	FindRefreshFamilyByPreviousHash(context.Context, string) (*domain.RefreshTokenFamily, error)
	RotateRefreshFamily(context.Context, string, string, string, time.Time) (bool, error)
	MarkRefreshFamilyReuseDetected(context.Context, string, time.Time) error
	RevokeRefreshFamiliesBySessionID(context.Context, string, time.Time) error
	RevokeRefreshFamiliesByExternalProvider(context.Context, string, time.Time) error
	RevokeRefreshFamiliesByPlatformCode(context.Context, string, time.Time) error
	RevokeRefreshFamiliesByPlatformLoginMethod(context.Context, string, string, string, time.Time) error
	RevokeRefreshFamiliesByExternalIdentity(context.Context, int64, time.Time) error
	RevokeRefreshFamiliesByUserID(context.Context, int64, time.Time) error
	RevokeRefreshFamiliesByUserIDAtOrBefore(context.Context, int64, time.Time) error
	UpsertConsentGrant(context.Context, *domain.ConsentGrant) error
	InsertAuditLog(context.Context, domain.AuditLog) error
	ListAuditEventsSince(context.Context, time.Time) ([]domain.AuditEvent, error)
}

// AuthorizationSessionCachePort keeps login transaction, touch throttling, and
// user-revocation-watermark state outside DG6.2's active-session projection.
type AuthorizationSessionCachePort interface {
	SaveSession(context.Context, *domain.AuthorizationSessionSnapshot, time.Duration) error
	GetSession(context.Context, string) (*domain.AuthorizationSessionSnapshot, error)
	RemoveSession(context.Context, string) error
	AcquireCompletionLock(context.Context, string) (bool, error)
	ReleaseCompletionLock(context.Context, string) error
	MarkSessionFinalized(context.Context, string, time.Duration) (bool, error)
	ReleaseSessionFinalized(context.Context, string) error
	SaveCompletionResult(context.Context, string, any, time.Duration) error
	GetCompletionResult(context.Context, string, any) (bool, error)
	MarkUserRevoked(context.Context, int64, time.Time) error
	UserRevokedAt(context.Context, int64) (*time.Time, error)
	AllowTouch(context.Context, string, time.Duration) (bool, error)
	ClearSessionTouch(context.Context, string) error
}
