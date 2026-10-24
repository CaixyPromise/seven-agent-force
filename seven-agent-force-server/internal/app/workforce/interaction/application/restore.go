package application

import (
	"context"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	"github.com/jackc/pgx/v5"
)

var _ facade.RestoredQuestionReader = (*Service)(nil)

func (s *Service) RestoredQuestionTx(ctx context.Context, tx pgx.Tx, tenant, task, targetTurn, responseID int64) (out facade.PublishRequest, err error) {
	if tx == nil || s.repo == nil {
		return out, domain.ErrUnavailable
	}
	if tenant <= 0 || task <= 0 || targetTurn <= 0 || responseID <= 0 {
		return out, domain.ErrInvalid
	}
	response, e := s.repo.Response(ctx, tx, tenant, responseID)
	if e != nil {
		return out, safe(e)
	}
	if response.TenantID != tenant || response.TaskID != task || response.ID != responseID || response.InteractionID <= 0 {
		return out, domain.ErrFenced
	}
	question, e := s.repo.Get(ctx, tx, tenant, task, response.InteractionID)
	if e != nil {
		return out, safe(e)
	}
	if question.TenantID != tenant || question.TaskID != task || question.ID != response.InteractionID || question.ResponseID != responseID || question.ResumeTaskTurnID != targetTurn || question.Kind != domain.QuestionKind || question.State != domain.Resolved || (question.DeliveryState != domain.Saved && question.DeliveryState != domain.Delivered) {
		return out, domain.ErrFenced
	}
	if domain.ValidateQuestions(question.Questions) != nil || domain.ValidateAnswers(question.Questions, response.Answers) != nil || domain.Digest(domain.NormalizeAnswers(question.Answers)) != domain.Digest(domain.NormalizeAnswers(response.Answers)) {
		return out, domain.ErrFenced
	}
	return facade.PublishRequest{RoundKey: question.RoundKey, LocalRequestKey: question.LocalRequestKey, Questions: domain.NormalizeQuestions(question.Questions)}, nil
}
