package application

import (
	"context"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
)

// ActiveSessionValidityCachePort is the only application-visible cache
// surface for DG6.2. It intentionally carries the neutral shared projection,
// not domain.Session or an infrastructure implementation type. Eligibility is
// supplied by the application loader and cache adapters cannot add business
// rules or reverse-import this package.
type ActiveSessionValidityCachePort interface {
	Resolve(ctx context.Context, sessionID string, loader func(context.Context) (*cachepolicy.ActiveSessionValiditySnapshot, bool, error)) (*cachepolicy.ActiveSessionValiditySnapshot, error)
}
