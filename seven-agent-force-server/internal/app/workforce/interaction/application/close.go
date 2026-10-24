package application

import (
	"context"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	"github.com/jackc/pgx/v5"
)

// Close records that a native request can no longer accept an answer. It does
// not claim that a previously saved answer was consumed by the native agent.
func (s *Service) Close(ctx context.Context, m facade.Machine, r facade.CloseRequest) (out facade.Receipt, err error) {
	if !domain.ValidKey(r.RoundKey) || !domain.ValidKey(r.LocalRequestKey) ||
		(r.Reason != "NATIVE_REQUEST_CLEARED" && r.Reason != "TURN_COMPLETED" && r.Reason != "TURN_INTERRUPTED") {
		return out, domain.ErrInvalid
	}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.machine(ctx, tx, m)
		if e != nil {
			return e
		}
		if !capable(t, "interaction.closed") {
			return domain.ErrNotAllowed
		}
		i, e := s.repo.FindOccurrence(ctx, tx, t.AttemptID, t.AdapterSessionGeneration, r.LocalRequestKey)
		if e != nil {
			return e
		}
		if i == nil {
			return domain.ErrNotFound
		}
		if i.TenantID != t.TenantID || i.TaskID != t.TaskID || i.TaskTurnID != t.TaskTurnID || i.RoundKey != r.RoundKey || i.Kind != domain.QuestionKind {
			return domain.ErrFenced
		}
		if i.State != domain.Open {
			out = receipt(*i, true)
			return nil
		}
		state := domain.Superseded
		if r.Reason == "TURN_INTERRUPTED" {
			state = domain.Cancelled
		}
		if e = i.Close(state); e != nil {
			return e
		}
		if e = s.persist(ctx, tx, *i, false); e != nil {
			return e
		}
		out = receipt(*i, false)
		return nil
	})
	return
}
