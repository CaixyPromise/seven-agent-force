package application

import (
	"context"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	"github.com/jackc/pgx/v5"
)

func (s *Service) PublishReceipt(ctx context.Context, m facade.Machine, r facade.PublishReceiptRequest) (out facade.Receipt, err error) {
	if !domain.ValidKey(r.LocalRequestKey) || !domain.ValidKey(r.RoundKey) || (r.Kind != domain.QuestionKind && r.Kind != domain.MessageKind) || (r.Status != "RECEIVED" && r.Status != "REJECTED" && r.Status != "UNKNOWN") {
		return out, domain.ErrInvalid
	}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.machine(ctx, tx, m)
		if e != nil {
			return e
		}
		if (r.Kind == domain.QuestionKind && !capable(t, "interaction.received")) || (r.Kind == domain.MessageKind && !capable(t, "message.received")) {
			return domain.ErrNotAllowed
		}
		i, e := s.repo.FindOccurrence(ctx, tx, t.AttemptID, t.AdapterSessionGeneration, r.LocalRequestKey)
		if e != nil {
			return e
		}
		if r.Kind == domain.QuestionKind && r.ResponseID > 0 && (i == nil || i.ResponseID != r.ResponseID) {
			cold, e := s.repo.ColdQuestion(ctx, tx, t.TenantID, t.TaskID, t.TaskTurnID, r.ResponseID)
			if e != nil {
				return e
			}
			if cold != nil {
				if cold.ResumeTaskTurnID != t.TaskTurnID || cold.LocalRequestKey != r.LocalRequestKey || cold.State != domain.Resolved || cold.DeliveryState == domain.NoDelivery {
					return domain.ErrFenced
				}
				out = receipt(*cold, true)
				return nil
			}
		}
		if i == nil {
			return domain.ErrNotFound
		}
		if i.TenantID != t.TenantID || i.TaskID != t.TaskID || i.TaskTurnID != t.TaskTurnID || i.Kind != r.Kind || i.State != domain.Resolved || i.ResponseID != r.ResponseID || i.DeliveryState == domain.NoDelivery || (i.RoundKey != "" && i.RoundKey != r.RoundKey) || i.ResumeTaskTurnID != 0 {
			return domain.ErrFenced
		}
		out = receipt(*i, true)
		return nil
	})
	return
}
