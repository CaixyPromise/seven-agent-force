package control

import (
	"context"
	completion "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	task "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	"net/http"
)

func (c *Client) RecordCompletionCheck(ctx context.Context, attempt task.Attempt, e completion.Evidence) (completion.Evidence, error) {
	var result completion.Evidence
	body := merge(c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch), map[string]any{"attemptLeaseToken": attempt.LeaseToken, "evidence": e})
	key := c.attemptIdempotencyKey("completion-check", attempt, e.Phase, idText(e.EvaluationCycleID), e.CheckDigest)
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/completion-check-results"), body, key, nil, &result)
	return result, err
}
func (c *Client) ReadCompletionCheck(ctx context.Context, attempt task.Attempt, phase string, cycle int64, digest string) (*completion.Evidence, error) {
	var result *completion.Evidence
	body := merge(c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch), map[string]any{"attemptLeaseToken": attempt.LeaseToken, "phase": phase, "cycleId": idText(cycle), "digest": digest})
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/completion-check-results/read"), body, "", nil, &result)
	return result, err
}
