package facade

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// TaskLifecycle fences every not-yet-terminal delivery fact when its Task is
// cancelled. Task owns the surrounding transaction; Delivery owns its state.
type TaskLifecycle interface {
	CancelForTask(context.Context, pgx.Tx, int64, string) error
}
