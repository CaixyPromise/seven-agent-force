// Package facade exposes the narrow structured-principal resolution contract
// shared by Workforce modules.  It deliberately carries numeric Framework
// identities and a caller-owned transaction; it never accepts usernames,
// display names, or synthetic actor strings as relationship identities.
package facade

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// IDGenerator is the Framework Snowflake seam used when a structured
// Workforce principal needs its first persistent identity.
type IDGenerator interface{ NextID() int64 }

// Resolver owns structured principal lookup/creation. Implementations may use
// infrastructure details internally, but consuming modules depend only on
// this stable facade instead of another module's private Store/helpers.
type Resolver interface {
	EnsureUser(context.Context, pgx.Tx, IDGenerator, int64) (int64, error)
	EnsureService(context.Context, pgx.Tx, IDGenerator, string) (int64, error)
}

// WorkerResolver is optional for consumers that record machine observations.
// Keeping it separate does not require user-only integrations to mint workers.
type WorkerResolver interface {
	EnsureWorker(context.Context, pgx.Tx, IDGenerator, int64) (int64, error)
}
