package control

import (
	"context"
	completion "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	task "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	"net/http"
	"strconv"
)

func (c *Client) ApplyCompletionRevision(ctx context.Context, attempt task.Attempt, cycleID int64) (completion.ApplyPendingResult, error) {
	var result completion.ApplyPendingResult
	body := merge(c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch), map[string]any{"attemptLeaseToken": attempt.LeaseToken, "cycleId": strconv.FormatInt(cycleID, 10)})
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/completion-check-revisions/apply"), body, "", nil, &result)
	return result, err
}
