package provider

import (
	"context"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	challengeinfra "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/infrastructure"
	randominfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/random"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestImageCaptchaPrepareGeneratesRefreshableImageAndSessionCode(t *testing.T) {
	provider := NewImageCaptchaChallengeStepProvider(challengeinfra.NewCaptchaService(randominfra.New(config.RandomConfig{CodeLength: 4})))
	session := &domain.ChallengeSession{}
	step := &domain.ChallengeStep{StepIdentifier: "step-captcha", ChallengeType: domain.ChallengeTypeImageCaptcha}

	if err := provider.Prepare(context.Background(), session, step); err != nil {
		t.Fatalf("prepare image captcha: %v", err)
	}
	code, _ := session.SessionContext[captchaContextKey(step)].(string)
	if len(code) != 4 {
		t.Fatalf("expected generated 4 digit session code, got %q", code)
	}
	if step.UserInterfaceHints["imageType"] != "base64" {
		t.Fatalf("expected base64 image type hint, got %#v", step.UserInterfaceHints["imageType"])
	}
	if step.UserInterfaceHints["refreshable"] != true {
		t.Fatalf("expected refreshable hint, got %#v", step.UserInterfaceHints["refreshable"])
	}
	if image, _ := step.UserInterfaceHints["codeImage"].(string); image == "" {
		t.Fatalf("expected captcha image hint")
	}
	if _, ok := step.UserInterfaceHints["captchaCode"]; ok {
		t.Fatalf("captcha code must not be exposed as a separate UI hint: %#v", step.UserInterfaceHints)
	}
}

func TestImageCaptchaVerifyConsumesCodeOnce(t *testing.T) {
	provider := NewImageCaptchaChallengeStepProvider(challengeinfra.NewCaptchaService(randominfra.New(config.RandomConfig{CodeLength: 4})))
	session := &domain.ChallengeSession{}
	step := &domain.ChallengeStep{StepIdentifier: "step-captcha", ChallengeType: domain.ChallengeTypeImageCaptcha}

	if err := provider.Prepare(context.Background(), session, step); err != nil {
		t.Fatalf("prepare image captcha: %v", err)
	}
	code, _ := session.SessionContext[captchaContextKey(step)].(string)

	ok, err := provider.Verify(context.Background(), session, step, map[string]any{"captchaCode": " " + code + " "})
	if err != nil {
		t.Fatalf("verify image captcha: %v", err)
	}
	if !ok {
		t.Fatal("expected generated captcha code to verify")
	}
	if _, exists := session.SessionContext[captchaContextKey(step)]; exists {
		t.Fatalf("expected successful captcha verification to consume stored code")
	}

	ok, err = provider.Verify(context.Background(), session, step, map[string]any{"captchaCode": code})
	if err != nil {
		t.Fatalf("verify replayed image captcha: %v", err)
	}
	if ok {
		t.Fatal("expected consumed captcha code replay to fail")
	}
}

func TestImageCaptchaVerifyRejectsMissingExpectedCode(t *testing.T) {
	provider := NewImageCaptchaChallengeStepProvider(&challengeinfra.CaptchaService{})
	session := &domain.ChallengeSession{SessionContext: map[string]any{}}
	step := &domain.ChallengeStep{StepIdentifier: "step-captcha", ChallengeType: domain.ChallengeTypeImageCaptcha}

	ok, err := provider.Verify(context.Background(), session, step, map[string]any{"captchaCode": ""})
	if err != nil {
		t.Fatalf("verify image captcha: %v", err)
	}
	if ok {
		t.Fatal("expected missing captcha code in session context to fail verification")
	}
}

func TestImageCaptchaRefreshOverwritesSessionCodeAndKeepsPlainCodeOutOfHints(t *testing.T) {
	provider := NewImageCaptchaChallengeStepProvider(challengeinfra.NewCaptchaService(randominfra.New(config.RandomConfig{CodeLength: 4})))
	session := &domain.ChallengeSession{SessionContext: map[string]any{}}
	step := &domain.ChallengeStep{StepIdentifier: "step-captcha", ChallengeType: domain.ChallengeTypeImageCaptcha}
	oldCode := "sentinel-code-that-random-service-will-not-generate"
	session.SessionContext[captchaContextKey(step)] = oldCode

	if err := provider.Refresh(context.Background(), session, step); err != nil {
		t.Fatalf("refresh image captcha: %v", err)
	}
	newCode, _ := session.SessionContext[captchaContextKey(step)].(string)
	if len(newCode) != 4 || newCode == oldCode {
		t.Fatalf("expected refresh to overwrite session captcha code, got %q", newCode)
	}
	if _, ok := step.UserInterfaceHints["captchaCode"]; ok {
		t.Fatalf("captcha refresh must not expose captchaCode hint: %#v", step.UserInterfaceHints)
	}
	if _, ok := step.UserInterfaceHints["code"]; ok {
		t.Fatalf("captcha refresh must not expose plain code hint: %#v", step.UserInterfaceHints)
	}
	ok, err := provider.Verify(context.Background(), session, step, map[string]any{"captchaCode": oldCode})
	if err != nil {
		t.Fatalf("verify stale captcha code after refresh: %v", err)
	}
	if ok {
		t.Fatal("expected stale captcha code from before refresh to fail")
	}
	ok, err = provider.Verify(context.Background(), session, step, map[string]any{"captchaCode": newCode})
	if err != nil {
		t.Fatalf("verify refreshed captcha code: %v", err)
	}
	if !ok {
		t.Fatal("expected refreshed captcha code to verify")
	}
}
