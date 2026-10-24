package facade

import (
	"context"
	"github.com/jackc/pgx/v5"
)

type AttemptLifecycle interface {
	CloseAttemptTx(context.Context, pgx.Tx, int64, int64, int64, string) error
}
