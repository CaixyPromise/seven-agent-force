package application

import (
	"math"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	wcp "github.com/CaixyPromise/seven-agent/force/internal/worker/checkpoint"
)

// ColdInteractionBudget issues a new container wall-clock bound, not a new task
// allowance. Source accumulated active/wait counters remain exact. Persist the
// returned value under the restore-run lock; retries must not call this again.
func ColdInteractionBudget(source wcp.ExecutionBudget, target MachineAuthority, now time.Time) (wcp.ExecutionBudget, error) {
	if source.Validate() != nil || (source.ActiveLimitMillis > 0 && source.ActiveElapsedMillis >= source.ActiveLimitMillis) || target.AttemptID <= 0 || target.Generation <= 0 || target.OwnerEpoch <= 0 {
		return wcp.ExecutionBudget{}, domain.ErrInteractionSuspend
	}
	if source.ActiveLimitMillis == 0 {
		result := source
		result.AttemptID, result.Generation, result.OwnerEpoch = target.AttemptID, target.Generation, target.OwnerEpoch
		return result, nil
	}
	remainingWait := max(int64(0), int64(10*time.Minute/time.Millisecond)-source.InteractionWaitingMillis)
	remainingActive := source.ActiveLimitMillis - source.ActiveElapsedMillis
	base := now.UnixMilli() + remainingWait + int64(2*time.Minute/time.Millisecond)
	if base <= 0 || remainingActive > math.MaxInt64-base {
		return wcp.ExecutionBudget{}, domain.ErrInteractionSuspend
	}
	result := source
	result.AttemptID, result.Generation, result.OwnerEpoch = target.AttemptID, target.Generation, target.OwnerEpoch
	result.AbsoluteDeadlineUnixMillis = base + remainingActive
	return result, nil
}
