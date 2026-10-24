package control

import (
	"context"
	"errors"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	"net/http"
)

func (c *Client) ReadEvaluation(ctx context.Context, attempt taskdomain.Attempt, pending facade.EvaluationReplayIdentity) (facade.EvaluationReplay, error) {
	var result struct {
		Found     *bool                      `json:"found"`
		Directive facade.EvaluationDirective `json:"directive"`
	}
	body := merge(c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch), map[string]any{"attemptLeaseToken": attempt.LeaseToken, "pending": pending})
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/result-evaluations/read"), body, "", nil, &result)
	if err != nil {
		return facade.EvaluationReplay{}, err
	}
	if result.Found == nil {
		return facade.EvaluationReplay{}, errors.New("EVALUATION_REPLAY_RESPONSE_INVALID")
	}
	if *result.Found && result.Directive.CycleID != pending.CycleID {
		return facade.EvaluationReplay{}, errors.New("EVALUATION_REPLAY_IDENTITY_INVALID")
	}
	return facade.EvaluationReplay{Found: *result.Found, Directive: result.Directive}, nil
}
