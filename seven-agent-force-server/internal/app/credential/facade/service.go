package facade

import (
	"context"
	"time"
)

type UserCredentialFacade interface {
	FindActivePasswordByUserID(ctx context.Context, userID int64) (*PasswordCredential, error)
	UpsertPasswordCredential(ctx context.Context, command UpsertPasswordCredentialCommand) error
	MarkPasswordUsed(ctx context.Context, userID int64, usedAt time.Time) error

	FindActiveTotpByUserID(ctx context.Context, userID int64) (*TotpCredential, error)
	FindActiveTotpSecretByUserID(ctx context.Context, userID int64) (*TotpSecret, error)
	UpsertTotpCredential(ctx context.Context, command UpsertTotpCredentialCommand) error
	CompleteTotpBinding(ctx context.Context, command CompleteTotpBindingCommand) error
	DisableTotpCredential(ctx context.Context, userID int64) (bool, error)
	MarkTotpUsed(ctx context.Context, userID int64, usedAt time.Time) error

	ListActivePasskeys(ctx context.Context, userID int64) ([]PasskeyCredential, error)
	FindActivePasskeyByCredentialKey(ctx context.Context, credentialKey string) (*PasskeyCredential, error)
	SavePasskeyCredential(ctx context.Context, command SavePasskeyCredentialCommand) error
	CompletePasskeyBinding(ctx context.Context, command CompletePasskeyBindingCommand) error
	DisablePasskeyCredential(ctx context.Context, userID int64, credentialKey string) (bool, error)
	UpdatePasskeyUsage(ctx context.Context, credentialKey string, signCount int64, usedAt time.Time) error

	CountAvailableRecoveryCodes(ctx context.Context, userID int64) (int, error)
	RegenerateRecoveryCodes(ctx context.Context, userID int64, batchSize int) (*RegeneratedRecoveryCodes, error)
	ConsumeRecoveryCode(ctx context.Context, userID int64, recoveryCode string, usedAt time.Time) (bool, error)
}
