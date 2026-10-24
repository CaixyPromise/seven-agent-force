//go:build integration

package application

import (
	"context"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
)

// RelaySelectedOutboxToBrokerForIntegration keeps the strict exact-selection
// boundary while exercising the real broker path in the G4.3 isolated recovery
// probe. This symbol is compiled only with the integration build tag and is
// not part of the production notification facade or HTTP surface.
func (s *Service) RelaySelectedOutboxToBrokerForIntegration(ctx context.Context, selections []domain.OutboxEventSelection) error {
	return s.relaySelectedOutbox(ctx, selections, false)
}
