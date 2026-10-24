package application

import (
	"context"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
)

// ReadEvaluation reissues only the exact persisted decision to a fenced Worker.
func (s *Service) ReadEvaluation(ctx context.Context, identity facade.MachineIdentity, attemptID int64, lease string, pending facade.EvaluationReplayIdentity) (facade.EvaluationReplay, error) {
	return s.store.ReadEvaluation(ctx, identity, attemptID, lease, pending)
}
