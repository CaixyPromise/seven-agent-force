package infrastructure

import (
	"context"
	"fmt"
	"strings"
	"time"

	emailinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/email"
	randominfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/random"
)

type EmailOTPService struct {
	random *randominfra.Service
	sender emailinfra.Sender
}

func NewEmailOTPService(random *randominfra.Service, sender emailinfra.Sender) *EmailOTPService {
	return &EmailOTPService{random: random, sender: sender}
}

func (s *EmailOTPService) Generate(ctx context.Context, email string, scene string, ttl time.Duration) (string, error) {
	if s == nil || s.random == nil {
		return "", fmt.Errorf("email otp service is not configured")
	}
	if strings.TrimSpace(email) == "" {
		return "", nil
	}
	code, err := s.random.Code(ctx)
	if err != nil {
		return "", err
	}
	if s.sender == nil {
		return "", fmt.Errorf("email sender is not configured")
	}
	if err := s.sender.SendChallengeOTP(ctx, emailinfra.ChallengeOTPRequest{
		ToEmail: email,
		Code:    code,
		Scene:   scene,
		TTL:     ttl,
	}); err != nil {
		return "", err
	}
	return code, nil
}

func (s *EmailOTPService) Verify(expected, input string) bool {
	return strings.TrimSpace(expected) != "" && strings.TrimSpace(expected) == strings.TrimSpace(input)
}

func MaskEmail(email string) string {
	email = strings.TrimSpace(email)
	at := strings.Index(email, "@")
	if at <= 1 {
		return "***"
	}
	return email[:1] + "***" + email[at-1:]
}
