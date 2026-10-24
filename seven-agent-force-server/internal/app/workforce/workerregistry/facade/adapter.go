package facade

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// AdapterCapabilities returns capabilities of the precise admitted image on
// the current Worker session, not a container's self-declared feature list.
type AdapterCapabilityReader interface {
	AdapterCapabilities(context.Context, pgx.Tx, int64, string, string) ([]string, error)
}
