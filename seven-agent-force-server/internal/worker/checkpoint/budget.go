package checkpoint

import "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"

// ExecutionBudget is copied only from a trusted Worker ledger snapshot. The
// container's timers/counters never populate this protected checkpoint field.
type ExecutionBudget struct {
	Version                    int   `json:"version"`
	TaskID                     int64 `json:"taskId,string"`
	AttemptID                  int64 `json:"attemptId,string"`
	Generation                 int64 `json:"generation,string"`
	OwnerEpoch                 int64 `json:"ownerEpoch,string"`
	ActiveLimitMillis          int64 `json:"activeLimitMillis,string"`
	ActiveElapsedMillis        int64 `json:"activeElapsedMillis,string"`
	InteractionWaitingMillis   int64 `json:"interactionWaitingMillis,string"`
	AbsoluteDeadlineUnixMillis int64 `json:"absoluteDeadlineUnixMillis,string"`
}

func (b ExecutionBudget) Validate() error {
	if b.Version != 1 || b.TaskID <= 0 || b.AttemptID <= 0 || b.Generation <= 0 || b.OwnerEpoch <= 0 || b.ActiveLimitMillis < 0 || b.ActiveElapsedMillis < 0 || (b.ActiveLimitMillis > 0 && b.ActiveElapsedMillis > b.ActiveLimitMillis) || b.InteractionWaitingMillis < 0 || (b.ActiveLimitMillis == 0 && b.AbsoluteDeadlineUnixMillis != 0) || (b.ActiveLimitMillis > 0 && b.AbsoluteDeadlineUnixMillis <= 0) {
		return domain.ErrInteractionSuspend
	}
	return nil
}
