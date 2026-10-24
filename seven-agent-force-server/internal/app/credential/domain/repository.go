package domain

import (
	"context"
	"time"
)

type UserCredentialRepository interface {
	FindSingleActive(ctx context.Context, userID int64, credentialType CredentialType, credentialKey string) (*CredentialRecord, error)
	FindSingleAny(ctx context.Context, userID int64, credentialType CredentialType, credentialKey string) (*CredentialRecord, error)
	FindActiveByTypeAndKey(ctx context.Context, credentialType CredentialType, credentialKey string) (*CredentialRecord, error)
	FindAnyByTypeAndKey(ctx context.Context, credentialType CredentialType, credentialKey string) (*CredentialRecord, error)
	ListActiveByUserAndType(ctx context.Context, userID int64, credentialType CredentialType) ([]CredentialRecord, error)
	CountActiveByUserAndType(ctx context.Context, userID int64, credentialType CredentialType) (int, error)
	Insert(ctx context.Context, record *CredentialRecord) error
	Update(ctx context.Context, record *CredentialRecord) error
	UpdateStatusByUserAndType(ctx context.Context, userID int64, credentialType CredentialType, fromStatus CredentialStatus, toStatus CredentialStatus, invalidatedAt time.Time) (int64, error)
	UpdateStatusByScope(ctx context.Context, userID int64, credentialType CredentialType, credentialKey string, fromStatus CredentialStatus, toStatus CredentialStatus, invalidatedAt time.Time) (int64, error)
	UpdateLastUsedByScope(ctx context.Context, userID int64, credentialType CredentialType, credentialKey string, status CredentialStatus, usedAt time.Time) error
	ListActiveRecoveryCodes(ctx context.Context, userID int64) ([]CredentialRecord, error)
	InvalidateActiveRecoveryCodes(ctx context.Context, userID int64, invalidatedAt time.Time) (int64, error)
	ConsumeRecoveryCodeByID(ctx context.Context, id int64, usedAt time.Time) (bool, error)
}

type IDGenerator interface {
	NextID() int64
}

type RecoveryCodeService interface {
	GenerateCodes(batchSize int) ([]string, error)
	GenerateSalt() (string, error)
	HashCode(code, saltB64 string, iterationCount int) (string, error)
	VerifyCode(code, saltB64 string, iterationCount int, expectedHashB64 string) bool
	HashAlgorithm() string
}

type RecoveryPayloadCodec interface {
	EncodePasskey(payload PasskeyPayload) (string, error)
	DecodePasskey(payload string) (PasskeyPayload, error)
	EncodeRecoveryCode(payload RecoveryCodePayload) (string, error)
	DecodeRecoveryCode(payload string) (RecoveryCodePayload, error)
}
