package facade

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// RestoredQuestionReader is a private continuation port. Caller must hold the
// authorized Task lock and validate the current machine before calling. It
// returns original safe question content, not permission to publish a request.
// The restore caller maps LocalRequestKey to interactionKey and replaces the
// source RoundKey with the trusted target input round.
type RestoredQuestionReader interface {
	RestoredQuestionTx(context.Context, pgx.Tx, int64, int64, int64, int64) (PublishRequest, error)
}
