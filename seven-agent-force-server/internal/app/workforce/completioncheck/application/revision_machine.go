package application

import (
	"context"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	"github.com/jackc/pgx/v5"
)

var _ facade.RevisionMachineControl = (*RevisionService)(nil)

func (s *RevisionService) FindSafePointRevisionTx(ctx context.Context, tx pgx.Tx, tenant, task, turn, attempt int64) (*facade.Revision, error) {
	if tx == nil || s.repo == nil {
		return nil, facade.ErrRevisionUnavailable
	}
	if tenant <= 0 || task <= 0 || turn <= 0 || attempt <= 0 {
		return nil, facade.ErrInvalid
	}
	r, e := s.repo.FindSafePointRevision(ctx, tx, tenant, task, turn, attempt)
	if e != nil {
		return nil, revisionError(e)
	}
	return r, nil
}

// ApplyPendingAtSafePoint owns the transaction. The Worker supplies only its
// authenticated identity and attempt/cycle; authority derives all business IDs.
func (s *RevisionService) ApplyPendingAtSafePoint(ctx context.Context, m facade.MachineAuthority, attempt int64, lease string, cycle int64) (out facade.ApplyPendingResult, err error) {
	if m.WorkerID <= 0 || m.InstanceID == "" || m.HostBootID == "" || m.Generation <= 0 || m.OwnerEpoch <= 0 || attempt <= 0 || cycle <= 0 || lease == "" {
		return out, facade.ErrInvalid
	}
	resolver, ok := s.authority.(facade.RevisionSafePointResolver)
	if !ok || s.pool == nil || s.repo == nil || s.turns == nil {
		return out, facade.ErrRevisionUnavailable
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return out, facade.ErrRevisionUnavailable
	}
	defer tx.Rollback(ctx)
	point, e := resolver.ResolveRevisionSafePoint(ctx, tx, m, attempt, lease, cycle)
	if e != nil {
		return out, revisionError(e)
	}
	if point != nil {
		// A resolver cannot silently replace the authenticated machine or source.
		if point.Authority != m || point.AttemptID != attempt || point.ExecutionLease != lease {
			return out, facade.ErrStale
		}
		revision, e := s.ApplyRevisionTx(ctx, tx, *point)
		if e != nil {
			return out, e
		}
		out = facade.ApplyPendingResult{Applied: true, TaskTurnID: revision.AppliedTurnID}
	}
	if e = tx.Commit(ctx); e != nil {
		return facade.ApplyPendingResult{}, facade.ErrRevisionUnavailable
	}
	return out, nil
}
