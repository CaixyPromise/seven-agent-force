package domain

import (
	"context"
	"strings"
	"time"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xtime"
)

type Service struct {
	repo MfaCredentialRepository
}

func NewService(repo MfaCredentialRepository) *Service {
	return &Service{repo: repo}
}

func (s *Service) FindEnabledOtpBinding(ctx context.Context, userID int64) (*OtpBindingRecord, error) {
	if err := validateUserID(userID); err != nil {
		return nil, err
	}
	return s.repo.FindEnabledOtpBinding(ctx, userID)
}

func (s *Service) ConsumeRecoveryCode(ctx context.Context, userID int64, recoveryCode string, usedAt time.Time) (bool, error) {
	if err := validateUserID(userID); err != nil {
		return false, err
	}
	if strings.TrimSpace(recoveryCode) == "" {
		return false, nil
	}
	return s.repo.ConsumeRecoveryCode(ctx, userID, recoveryCode, chooseInstant(usedAt))
}

func (s *Service) MarkTotpUsed(ctx context.Context, userID int64, usedAt time.Time) error {
	if err := validateUserID(userID); err != nil {
		return err
	}
	return s.repo.MarkTotpUsed(ctx, userID, chooseInstant(usedAt))
}

func validateUserID(userID int64) error {
	if userID <= 0 {
		return apperrors.Params("userId不能为空")
	}
	return nil
}

func chooseInstant(value time.Time) time.Time {
	if value.IsZero() {
		return xtime.Now()
	}
	return value.UTC()
}
