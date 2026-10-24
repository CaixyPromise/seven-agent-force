package application

import (
	"context"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	"github.com/jackc/pgx/v5"
	"time"
)

// OpenQuestions provides Server-owned occurrence identities to the trusted
// Worker suspend path. Container-supplied entity IDs never enter this lookup.
func (s *Service) OpenQuestions(ctx context.Context, m facade.Machine) (out []facade.PendingQuestion, err error) {
	out = []facade.PendingQuestion{}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.machine(ctx, tx, m)
		if e != nil {
			return e
		}
		if !capable(t, "interaction.request") {
			return domain.ErrNotAllowed
		}
		items, e := s.repo.OpenQuestions(ctx, tx, t.TenantID, t.TaskID)
		if e != nil {
			return e
		}
		now := time.Now()
		for _, i := range items {
			if i.Kind != domain.QuestionKind || i.State != domain.Open || i.TaskTurnID != t.TaskTurnID || i.AttemptID != t.AttemptID || i.AdapterSessionGeneration != t.AdapterSessionGeneration || !now.Before(i.ExpiresAt) {
				continue
			}
			out = append(out, facade.PendingQuestion{InteractionID: i.ID, LocalRequestKey: i.LocalRequestKey, RoundKey: i.RoundKey, IsBlocking: i.IsBlocking, ExpiresAt: i.ExpiresAt})
		}
		return nil
	})
	return
}
