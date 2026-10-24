package application

import (
	"context"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
)

// RevisionView reads input, optimistic version and request eligibility in one
// Task-locked transaction. It does not imply a checkpoint or applied revision.
func (s *RevisionService) RevisionView(ctx context.Context, u facade.RevisionUser) (out facade.RevisionView, err error) {
	out.AllowedActions = []string{}
	if u.TenantID <= 0 || u.TaskID <= 0 || u.UserID <= 0 {
		return out, facade.ErrInvalid
	}
	if s.pool == nil || s.repo == nil {
		return out, facade.ErrRevisionUnavailable
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return out, facade.ErrRevisionUnavailable
	}
	defer tx.Rollback(ctx)
	source, err := s.repo.LoadRevisionSource(ctx, tx, u.TenantID, u.TaskID)
	if err != nil {
		return out, revisionError(err)
	}
	pending, err := s.repo.FindPendingRevision(ctx, tx, u.TenantID, u.TaskID)
	if err != nil {
		return out, revisionError(err)
	}
	out.ExpectedVersion, out.CompletionCheck, out.Pending = source.Version, source.Input, pending
	if u.CanContinue && source.Status == "RUNNING" && source.AttemptID > 0 && pending == nil && s.authority != nil && s.turns != nil {
		out.AllowedActions = append(out.AllowedActions, facade.RequestRevision)
	}
	if err = tx.Commit(ctx); err != nil {
		return facade.RevisionView{}, revisionError(err)
	}
	return out, nil
}
