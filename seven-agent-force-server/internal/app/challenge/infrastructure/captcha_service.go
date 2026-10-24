package infrastructure

import (
	"context"
	"fmt"
	"strings"

	base64captcha "github.com/mojocn/base64Captcha"

	randominfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/random"
)

type CaptchaService struct {
	random *randominfra.Service
}

func NewCaptchaService(random *randominfra.Service) *CaptchaService {
	return &CaptchaService{random: random}
}

func (s *CaptchaService) Generate(ctx context.Context) (string, string, error) {
	if s == nil || s.random == nil {
		return "", "", fmt.Errorf("captcha service is not configured")
	}
	_ = ctx
	driver := base64captcha.NewDriverDigit(56, 170, 4, 0.36, 46)
	_, content, answer := driver.GenerateIdQuestionAnswer()
	item, err := driver.DrawCaptcha(content)
	if err != nil {
		return "", "", fmt.Errorf("generate image captcha: %w", err)
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return "", "", fmt.Errorf("generate image captcha: empty answer")
	}
	return answer, trimDataURI(item.EncodeB64string()), nil
}

func (s *CaptchaService) Verify(expected, input string) bool {
	expected = strings.TrimSpace(expected)
	input = strings.TrimSpace(input)
	return expected != "" && input != "" && strings.EqualFold(expected, input)
}

func trimDataURI(value string) string {
	value = strings.TrimSpace(value)
	if prefix := "data:image/png;base64,"; strings.HasPrefix(value, prefix) {
		return strings.TrimPrefix(value, prefix)
	}
	return value
}
