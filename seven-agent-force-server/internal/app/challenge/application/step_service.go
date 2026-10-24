package application

import (
	"context"
	"fmt"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain/provider"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

type StepService struct {
	providers *provider.Registry
}

func NewStepService(providers *provider.Registry) *StepService {
	return &StepService{providers: providers}
}

func (s *StepService) PrepareStep(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) error {
	if session == nil {
		return apperrors.Params("challenge session不能为空")
	}
	item, err := s.providerFor(step)
	if err != nil {
		return err
	}
	return item.Prepare(ctx, session, step)
}

func (s *StepService) IsStepEligible(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) (bool, error) {
	if session == nil {
		return false, apperrors.Params("challenge session不能为空")
	}
	item, err := s.providerFor(step)
	if err != nil {
		return false, err
	}
	eligible, ok := item.(provider.ChallengeStepEligibilityProvider)
	if !ok {
		return true, nil
	}
	return eligible.Eligible(ctx, session, step)
}

func (s *StepService) VerifyStep(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep, payload map[string]any) (bool, error) {
	if session == nil {
		return false, apperrors.Params("challenge session不能为空")
	}
	item, err := s.providerFor(step)
	if err != nil {
		return false, err
	}
	return item.Verify(ctx, session, step, payload)
}

func (s *StepService) RefreshStep(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) error {
	item, err := s.providerFor(step)
	if err != nil {
		return err
	}
	refreshable, ok := item.(provider.RefreshableChallengeStepProvider)
	if !ok {
		return apperrors.Operation("当前步骤不支持刷新")
	}
	return refreshable.Refresh(ctx, session, step)
}

func (s *StepService) providerFor(step *domain.ChallengeStep) (provider.ChallengeStepProvider, error) {
	if s == nil || s.providers == nil {
		return nil, fmt.Errorf("challenge step providers are not configured")
	}
	if step == nil {
		return nil, apperrors.Params("challenge step不能为空")
	}
	item, ok := s.providers.Provider(step.ChallengeType)
	if !ok || item == nil {
		return nil, apperrors.Operation("challenge step provider不存在")
	}
	return item, nil
}
