package application

import (
	"context"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/domain"
)

func (s *Service) hydrateHistoricalRun(ctx context.Context, tenantID int64, item domain.RunView) (domain.RunView, error) {
	if !domain.ValidCompletionCheckDigest(item.CompletionCheckDigest) {
		return domain.RunView{}, domain.ErrInvalidReviewInput
	}
	// A historical Run without this identity must never acquire criteria from a
	// later TaskTurn. Its original evidence remains readable without hydration.
	item.CompletionCheckSummary = nil
	if item.CompletionCheckDigest == "" {
		return item, nil
	}
	if s.taskInputs == nil {
		return domain.RunView{}, domain.ErrInvalidReviewInput
	}
	input, err := s.taskInputs.IndependentReviewInput(ctx, tenantID, item.TaskID, item.TaskTurnID)
	if err != nil || input.TenantID != tenantID || input.TaskID != item.TaskID || input.TaskTurnID != item.TaskTurnID || input.CompletionCheckDigest != item.CompletionCheckDigest {
		return domain.RunView{}, domain.ErrInvalidReviewInput
	}
	if _, err := reviewRequirements(input); err != nil {
		return domain.RunView{}, domain.ErrInvalidReviewInput
	}
	item.CompletionCheckSummary = &domain.CompletionCheckSummary{Instructions: input.Instructions, ReviewPrompt: input.ReviewPrompt}
	return item, nil
}
