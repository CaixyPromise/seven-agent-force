package application

import (
	"context"
	"fmt"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

type FlowService struct {
	steps      *StepService
	completion *CompletionHandler
}

func NewFlowService(steps *StepService, completion *CompletionHandler) *FlowService {
	return &FlowService{
		steps:      steps,
		completion: completion,
	}
}

func (s *FlowService) PrepareStep(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) error {
	if s == nil || s.steps == nil {
		return fmt.Errorf("challenge flow step service is not configured")
	}
	return s.steps.PrepareStep(ctx, session, step)
}

func (s *FlowService) VerifyStep(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep, payload map[string]any) (bool, error) {
	if s == nil || s.steps == nil {
		return false, fmt.Errorf("challenge flow step service is not configured")
	}
	return s.steps.VerifyStep(ctx, session, step, payload)
}

func (s *FlowService) OnPassed(ctx context.Context, session *domain.ChallengeSession) error {
	if s == nil || s.completion == nil {
		return fmt.Errorf("challenge completion handler is not configured")
	}
	if session == nil {
		return apperrors.Params("challenge session不能为空")
	}
	return s.completion.OnPassed(ctx, session)
}
