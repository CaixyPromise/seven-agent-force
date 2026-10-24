package domain

import (
	"context"
	"testing"
	"time"
)

type fakeRepo struct {
	binding        *OtpBindingRecord
	consumed       bool
	markedUserID   int64
	markedUsedAt   time.Time
	consumedCode   string
	consumedUsedAt time.Time
}

func (f *fakeRepo) FindEnabledOtpBinding(ctx context.Context, userID int64) (*OtpBindingRecord, error) {
	return f.binding, nil
}

func (f *fakeRepo) ConsumeRecoveryCode(ctx context.Context, userID int64, recoveryCode string, usedAt time.Time) (bool, error) {
	f.consumedCode = recoveryCode
	f.consumedUsedAt = usedAt
	return f.consumed, nil
}

func (f *fakeRepo) MarkTotpUsed(ctx context.Context, userID int64, usedAt time.Time) error {
	f.markedUserID = userID
	f.markedUsedAt = usedAt
	return nil
}

func TestFindEnabledOtpBinding(t *testing.T) {
	repo := &fakeRepo{binding: &OtpBindingRecord{UserID: 1, SecretEncrypted: "cipher"}}
	service := NewService(repo)
	record, err := service.FindEnabledOtpBinding(context.Background(), 1)
	if err != nil {
		t.Fatalf("find enabled otp binding: %v", err)
	}
	if record == nil || record.SecretEncrypted != "cipher" {
		t.Fatalf("unexpected record: %#v", record)
	}
}

func TestConsumeRecoveryCode(t *testing.T) {
	repo := &fakeRepo{consumed: true}
	service := NewService(repo)
	ok, err := service.ConsumeRecoveryCode(context.Background(), 1, "ABCD-EFGH-IJKL", time.Time{})
	if err != nil {
		t.Fatalf("consume recovery code: %v", err)
	}
	if !ok || repo.consumedCode != "ABCD-EFGH-IJKL" || repo.consumedUsedAt.IsZero() {
		t.Fatalf("unexpected consume state: ok=%v code=%s usedAt=%v", ok, repo.consumedCode, repo.consumedUsedAt)
	}
}

func TestMarkTotpUsed(t *testing.T) {
	repo := &fakeRepo{}
	service := NewService(repo)
	if err := service.MarkTotpUsed(context.Background(), 7, time.Time{}); err != nil {
		t.Fatalf("mark totp used: %v", err)
	}
	if repo.markedUserID != 7 || repo.markedUsedAt.IsZero() {
		t.Fatalf("unexpected mark state: userID=%d usedAt=%v", repo.markedUserID, repo.markedUsedAt)
	}
}
