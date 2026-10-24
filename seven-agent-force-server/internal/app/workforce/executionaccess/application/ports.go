package application

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/domain"
)

// Repository is persistence only. Authorization and state transitions remain
// in Service/domain and all methods participate in the caller-owned tx.
type Repository interface {
	FindOccurrence(context.Context, pgx.Tx, int64, int64, string) (*domain.Operation, error)
	GetOperation(context.Context, pgx.Tx, int64, int64, int64, bool) (domain.Operation, error)
	NextReadyOperation(context.Context, pgx.Tx, int64, int64, int64, int64) (*domain.Operation, error)
	RecoverableClaim(context.Context, pgx.Tx, int64, int64, int64, int64, int64, int64, string, string, string) (*domain.Operation, error)
	InsertOperation(context.Context, pgx.Tx, domain.Operation) error
	UpdateOperation(context.Context, pgx.Tx, domain.Operation) error
	PendingSuspendOperations(context.Context, pgx.Tx, int64, int64, int64, int64) ([]domain.Operation, error)

	FindDecision(context.Context, pgx.Tx, int64, int64, int64, string) (*domain.Decision, error)
	InsertDecision(context.Context, pgx.Tx, domain.Decision) error

	ActiveGrants(context.Context, pgx.Tx, int64, int64, bool) ([]domain.Grant, error)
	GrantAudits(context.Context, pgx.Tx, int64, int64) ([]domain.GrantAudit, error)
	GetGrant(context.Context, pgx.Tx, int64, int64, int64, bool) (domain.Grant, error)
	InsertGrant(context.Context, pgx.Tx, domain.Grant) error
	UpdateGrant(context.Context, pgx.Tx, domain.Grant) error

	InsertAttempt(context.Context, pgx.Tx, domain.Attempt) error
	AttemptForOperation(context.Context, pgx.Tx, int64, bool) (*domain.Attempt, error)
	GetAttempt(context.Context, pgx.Tx, int64, int64, bool) (domain.Attempt, error)
	UpdateAttempt(context.Context, pgx.Tx, domain.Attempt) error
	PendingResults(context.Context, pgx.Tx, int64, int64, int64, int64, int) ([]domain.Operation, error)
	CancellableOperations(context.Context, pgx.Tx, int64, int64) ([]domain.Operation, error)
	TaskGrants(context.Context, pgx.Tx, int64, int64) ([]domain.Grant, error)
	CancelActiveAttempts(context.Context, pgx.Tx, int64, int64, time.Time) error
	ExpiredOperations(context.Context, pgx.Tx, int, time.Time) ([]domain.Operation, error)
	ExpiredClaimOperations(context.Context, pgx.Tx, int, time.Time) ([]domain.Operation, error)
	ExpiredGrants(context.Context, pgx.Tx, int, time.Time) ([]domain.Grant, error)
	OperationsForGrant(context.Context, pgx.Tx, int64, int64, int64) ([]domain.Operation, error)
	CancelOperationAttempts(context.Context, pgx.Tx, int64, time.Time) error
}
