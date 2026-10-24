package application

import (
	"context"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/facade"
)

type Service struct {
	domain *domain.Service
}

func NewService(domainService *domain.Service) *Service {
	return &Service{domain: domainService}
}

func (s *Service) FindEnabledOtpBinding(ctx context.Context, userID int64) (*facade.OtpBindingRecord, error) {
	record, err := s.domain.FindEnabledOtpBinding(ctx, userID)
	if err != nil || record == nil {
		return nil, err
	}
	return &facade.OtpBindingRecord{
		UserID:          record.UserID,
		SecretEncrypted: record.SecretEncrypted,
		VerifiedAt:      record.VerifiedAt,
		LastUsedAt:      record.LastUsedAt,
	}, nil
}

func (s *Service) ConsumeRecoveryCode(ctx context.Context, userID int64, recoveryCode string, usedAt time.Time) (bool, error) {
	return s.domain.ConsumeRecoveryCode(ctx, userID, recoveryCode, usedAt)
}

func (s *Service) MarkTotpUsed(ctx context.Context, userID int64, usedAt time.Time) error {
	return s.domain.MarkTotpUsed(ctx, userID, usedAt)
}
