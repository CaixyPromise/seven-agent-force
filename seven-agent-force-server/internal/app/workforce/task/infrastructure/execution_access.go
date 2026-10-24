package infrastructure

import (
	"context"
	"time"

	executionaccess "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/facade"
	"github.com/jackc/pgx/v5"
)

// SetExecutionAccessLifecycle binds the operation/grant owner into the Task's
// existing cancellation transaction. It does not give task infrastructure
// access to executionaccess persistence.
func (s *Store) SetExecutionAccessLifecycle(lifecycle executionaccess.Lifecycle) {
	s.executionAccess = lifecycle
}

// closeExecutionAccessTx invalidates task-scoped network authority on every
// terminal path, not only user cancellation. It keeps this store unaware of
// executionaccess tables and lets that module fence its own active attempts.
func (s *Store) closeExecutionAccessTx(ctx context.Context, tx pgx.Tx, taskID int64) error {
	if s.executionAccess == nil {
		return nil
	}
	var tenantID int64
	if err := tx.QueryRow(ctx, `SELECT tenant_id FROM workforce_tasks WHERE id=$1`, taskID).Scan(&tenantID); err != nil {
		return err
	}
	return s.executionAccess.CancelTaskTx(ctx, tx, tenantID, taskID, time.Now().UTC())
}
