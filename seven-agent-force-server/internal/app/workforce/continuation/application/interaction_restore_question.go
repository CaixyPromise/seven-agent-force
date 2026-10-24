package application

import (
	"encoding/json"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	interaction "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	"strconv"
)

// ProjectInteractionRestoreQuestion accepts only the relationship-checked
// projection returned by interaction.RestoredQuestionReader. It never uses
// archived native state. The old round is deliberately excluded: the CLI binds
// this question to the current resumed round without emitting a new request.
func ProjectInteractionRestoreQuestion(question interaction.PublishRequest, response json.RawMessage, responseID int64) (json.RawMessage, error) {
	var answer struct {
		InteractionKey string `json:"interactionKey"`
		ResponseID     string `json:"responseId"`
	}
	if responseID <= 0 || len(response) > 16<<10 || json.Unmarshal(response, &answer) != nil || answer.ResponseID != strconv.FormatInt(responseID, 10) || question.LocalRequestKey == "" || answer.InteractionKey != question.LocalRequestKey || len(question.Questions) == 0 {
		return nil, domain.ErrInteractionSuspend
	}
	body, err := json.Marshal(map[string]any{"interactionKey": question.LocalRequestKey, "questions": question.Questions})
	if err != nil || len(body) > 64<<10 {
		return nil, domain.ErrInteractionSuspend
	}
	return body, nil
}
