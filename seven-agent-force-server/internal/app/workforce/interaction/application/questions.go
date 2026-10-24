package application

import (
	"context"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	"github.com/jackc/pgx/v5"
	"strconv"
)

func (s *Service) Questions(ctx context.Context, u facade.User, before int64, limit int) (out facade.QuestionPage, err error) {
	if before < 0 || limit < 0 || limit > 50 {
		return out, domain.ErrInvalid
	}
	if limit == 0 {
		limit = 20
	}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		if _, e := s.user(ctx, tx, u, facade.Read); e != nil {
			return e
		}
		if e := s.expire(ctx, tx, u.TenantID, u.TaskID); e != nil {
			return e
		}
		items, e := s.repo.QuestionHistory(ctx, tx, u.TenantID, u.TaskID, before, limit+1)
		if e != nil {
			return e
		}
		if len(items) > limit {
			items = items[:limit]
			cursor := strconv.FormatInt(items[len(items)-1].ID, 10)
			out.NextCursor = &cursor
		}
		out.Items = items
		return nil
	})
	return
}
