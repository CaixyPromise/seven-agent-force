package provider

import (
	"context"
	"fmt"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	challengeinfra "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/infrastructure"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

type ImageCaptchaChallengeStepProvider struct {
	captcha *challengeinfra.CaptchaService
}

func NewImageCaptchaChallengeStepProvider(captcha *challengeinfra.CaptchaService) *ImageCaptchaChallengeStepProvider {
	return &ImageCaptchaChallengeStepProvider{captcha: captcha}
}

func (p *ImageCaptchaChallengeStepProvider) Type() domain.ChallengeType {
	return domain.ChallengeTypeImageCaptcha
}

func (p *ImageCaptchaChallengeStepProvider) Prepare(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) error {
	return p.Refresh(ctx, session, step)
}

func (p *ImageCaptchaChallengeStepProvider) Refresh(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) error {
	if p == nil || p.captcha == nil {
		return fmt.Errorf("image captcha provider is not fully configured")
	}
	if session == nil || step == nil {
		return apperrors.Params("challenge session或step不能为空")
	}
	code, image, err := p.captcha.Generate(ctx)
	if err != nil {
		return err
	}
	session.EnsureSessionContext()[captchaContextKey(step)] = code
	hints := step.EnsureUserInterfaceHints()
	hints["codeImage"] = image
	hints["imageType"] = "base64"
	hints["refreshable"] = true
	return nil
}

func (p *ImageCaptchaChallengeStepProvider) Verify(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep, payload map[string]any) (bool, error) {
	_ = ctx
	if p == nil || p.captcha == nil {
		return false, fmt.Errorf("image captcha provider is not fully configured")
	}
	if session == nil || step == nil {
		return false, apperrors.Params("challenge session或step不能为空")
	}
	input := payloadString(payload, "captchaCode")
	expected := valueString(session.EnsureSessionContext()[captchaContextKey(step)])
	if p.captcha.Verify(expected, input) {
		delete(session.EnsureSessionContext(), captchaContextKey(step))
		return true, nil
	}
	return false, nil
}

func captchaContextKey(step *domain.ChallengeStep) string {
	return "captcha.code." + step.StepIdentifier
}
