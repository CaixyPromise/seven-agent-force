// Package facade exposes Provider facts that another Workforce application
// module may freeze into an immutable execution policy.  It never exposes a
// credential, base URL, secret material, or Provider-invocation internals.
package facade

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var ErrIndependentReviewProviderUnavailable = errors.New("INDEPENDENT_REVIEW_PROVIDER_UNAVAILABLE")

// IndependentReviewProvider is the non-secret, already-validated Provider
// identity selected for one independent-review attempt. All internal IDs
// remain int64 until the receiving HTTP/RabbitMQ boundary encodes them.
type IndependentReviewProvider struct {
	ID            int64
	ConfigDigest  string
	Protocol      string
	Model         string
	Capabilities  []byte
	TimeoutMillis int64
}

// IndependentReviewResolver is transaction-aware so the review attempt freezes
// the exact Provider revision it validates. The Provider module owns this
// validation; callers never inspect Provider tables.
type IndependentReviewResolver interface {
	ResolveIndependentReviewProvider(context.Context, pgx.Tx, int64, int64, string) (IndependentReviewProvider, error)
}
