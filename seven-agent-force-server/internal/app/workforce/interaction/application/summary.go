package application

import (
	"context"
	continuation "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Service) WithSummaryAuthority(a facade.SummaryAuthority) *Service { s.summaries = a; return s }
func (s *Service) TaskSummaries(ctx context.Context, tenant, user int64, tasks []int64, canContinue bool) (out map[int64]facade.TaskSummary, err error) {
	if tenant <= 0 || user <= 0 || len(tasks) > 200 {
		return nil, domain.ErrInvalid
	}
	out = map[int64]facade.TaskSummary{}
	if len(tasks) == 0 {
		return out, nil
	}
	set := map[int64]bool{}
	for _, id := range tasks {
		if id <= 0 || set[id] {
			return nil, domain.ErrInvalid
		}
		set[id] = true
	}
	if s.summaries == nil {
		return nil, domain.ErrUnavailable
	}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		scope, e := s.summaries.AuthorizeSummary(ctx, tx, tenant, user, tasks)
		if e != nil {
			return e
		}
		for _, id := range scope.TaskIDs {
			if !set[id] {
				return domain.ErrFenced
			}
		}
		active := scope.ActiveTaskIDs
		if !canContinue {
			active = []int64{}
		}
		var e2 error
		out, e2 = s.repo.Summaries(ctx, tx, tenant, scope.PrincipalID, scope.TaskIDs, active)
		return e2
	})
	return
}

// AuthorizeInteractionSuspendTx is a narrow occurrence gate, called only after
// continuation owns the current Task lock. It never creates runtime authority.
func (s *Service) AuthorizeInteractionSuspendTx(ctx context.Context, tx pgx.Tx, r continuation.InteractionSuspendOccurrence) error {
	if tx == nil || r.TenantID <= 0 || r.TaskID <= 0 || r.InteractionID <= 0 || r.TaskTurnID <= 0 || r.AttemptID <= 0 || r.AdapterSessionGeneration <= 0 {
		return domain.ErrInvalid
	}
	i, e := s.repo.Get(ctx, tx, r.TenantID, r.TaskID, r.InteractionID)
	if e != nil {
		return safe(e)
	}
	if i.Kind != domain.QuestionKind || i.TaskTurnID != r.TaskTurnID || i.AttemptID != r.AttemptID || i.AdapterSessionGeneration != r.AdapterSessionGeneration {
		return domain.ErrFenced
	}
	if i.State == domain.Resolved && i.ResponseID > 0 {
		return continuation.ErrInteractionSuspendAnswerWon
	}
	if i.State != domain.Open || i.ResponseID != 0 || !time.Now().Before(i.ExpiresAt) {
		return domain.ErrFenced
	}
	return nil
}
