package application

import (
	"context"
	"fmt"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/credential/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/credential/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	credentialcryptoinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/credentialcrypto"
	envelopeinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/envelope"
)

type Service struct {
	transactor store.Transactor
	domain     *domain.Service
	envelope   envelopeinfra.Service
	codec      credentialcryptoinfra.Codec
}

func NewService(
	transactor store.Transactor,
	domainService *domain.Service,
	envelope envelopeinfra.Service,
	codec credentialcryptoinfra.Codec,
) *Service {
	return &Service{
		transactor: transactor,
		domain:     domainService,
		envelope:   envelope,
		codec:      codec,
	}
}

func (s *Service) FindActivePasswordByUserID(ctx context.Context, userID int64) (*facade.PasswordCredential, error) {
	result, err := s.domain.FindActivePasswordByUserID(ctx, userID)
	if err != nil || result == nil {
		return nil, err
	}
	return &facade.PasswordCredential{
		UserID:             result.UserID,
		CredentialKey:      result.CredentialKey,
		PasswordHash:       result.PasswordHash,
		MustChangePassword: result.MustChangePassword,
		PasswordChangedAt:  result.PasswordChangedAt,
		LastUsedAt:         result.LastUsedAt,
		VerifiedAt:         result.VerifiedAt,
	}, nil
}

func (s *Service) UpsertPasswordCredential(ctx context.Context, command facade.UpsertPasswordCredentialCommand) error {
	return s.withTx(ctx, func(txCtx context.Context) error {
		return s.domain.UpsertPasswordCredential(txCtx, domain.UpsertPasswordInput{
			UserID:             command.UserID,
			PasswordHash:       command.PasswordHash,
			MustChangePassword: command.MustChangePassword,
			PasswordChangedAt:  command.PasswordChangedAt,
			CreatorID:          command.CreatorID,
			UpdaterID:          command.UpdaterID,
		})
	})
}

func (s *Service) MarkPasswordUsed(ctx context.Context, userID int64, usedAt time.Time) error {
	return s.withTx(ctx, func(txCtx context.Context) error {
		return s.domain.MarkPasswordUsed(txCtx, userID, usedAt)
	})
}

func (s *Service) FindActiveTotpByUserID(ctx context.Context, userID int64) (*facade.TotpCredential, error) {
	result, err := s.domain.FindActiveTotpByUserID(ctx, userID)
	if err != nil || result == nil {
		return nil, err
	}
	return &facade.TotpCredential{
		UserID:           result.UserID,
		CredentialKey:    result.CredentialKey,
		SecretCiphertext: result.SecretCiphertext,
		VerifiedAt:       result.VerifiedAt,
		LastUsedAt:       result.LastUsedAt,
	}, nil
}

func (s *Service) FindActiveTotpSecretByUserID(ctx context.Context, userID int64) (*facade.TotpSecret, error) {
	if s.envelope == nil || s.codec == nil {
		return nil, fmt.Errorf("credential totp secret service is not configured")
	}
	result, err := s.domain.FindActiveTotpByUserID(ctx, userID)
	if err != nil || result == nil {
		return nil, err
	}
	payload, err := s.codec.Decode(result.SecretCiphertext)
	if err != nil {
		return nil, apperrors.ObjectState("TOTP凭证不可用，请重新绑定")
	}
	plainSecret, err := s.envelope.DecryptString(ctx, payload)
	if err != nil {
		return nil, apperrors.ObjectState("TOTP凭证不可用，请重新绑定")
	}
	return &facade.TotpSecret{
		UserID:        result.UserID,
		CredentialKey: result.CredentialKey,
		Secret:        plainSecret,
		VerifiedAt:    result.VerifiedAt,
		LastUsedAt:    result.LastUsedAt,
	}, nil
}

func (s *Service) UpsertTotpCredential(ctx context.Context, command facade.UpsertTotpCredentialCommand) error {
	return s.withTx(ctx, func(txCtx context.Context) error {
		return s.domain.UpsertTotpCredential(txCtx, domain.UpsertTotpInput{
			UserID:           command.UserID,
			SecretCiphertext: command.SecretCiphertext,
			VerifiedAt:       command.VerifiedAt,
			CreatorID:        command.CreatorID,
			UpdaterID:        command.UpdaterID,
		})
	})
}

func (s *Service) CompleteTotpBinding(ctx context.Context, command facade.CompleteTotpBindingCommand) error {
	if s.envelope == nil || s.codec == nil {
		return fmt.Errorf("credential totp binding service is not configured")
	}
	return s.withTx(ctx, func(txCtx context.Context) error {
		secretEnvelope, err := s.envelope.EncryptString(txCtx, command.PlainSecret)
		if err != nil {
			return err
		}
		secretCiphertext, err := s.codec.Encode(secretEnvelope)
		if err != nil {
			return err
		}
		if err := s.domain.UpsertTotpCredential(txCtx, domain.UpsertTotpInput{
			UserID:           command.UserID,
			SecretCiphertext: secretCiphertext,
			VerifiedAt:       command.VerifiedAt,
		}); err != nil {
			return err
		}
		_, err = s.domain.RegenerateRecoveryCodes(txCtx, command.UserID, command.RecoveryBatchSize)
		return err
	})
}

func (s *Service) DisableTotpCredential(ctx context.Context, userID int64) (bool, error) {
	var (
		disabled bool
		err      error
	)
	err = s.withTx(ctx, func(txCtx context.Context) error {
		disabled, err = s.domain.DisableTotpCredential(txCtx, userID)
		return err
	})
	return disabled, err
}

func (s *Service) MarkTotpUsed(ctx context.Context, userID int64, usedAt time.Time) error {
	return s.withTx(ctx, func(txCtx context.Context) error {
		return s.domain.MarkTotpUsed(txCtx, userID, usedAt)
	})
}

func (s *Service) ListActivePasskeys(ctx context.Context, userID int64) ([]facade.PasskeyCredential, error) {
	items, err := s.domain.ListActivePasskeys(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]facade.PasskeyCredential, 0, len(items))
	for _, item := range items {
		result = append(result, toFacadePasskey(item))
	}
	return result, nil
}

func (s *Service) FindActivePasskeyByCredentialKey(ctx context.Context, credentialKey string) (*facade.PasskeyCredential, error) {
	item, err := s.domain.FindActivePasskeyByCredentialKey(ctx, credentialKey)
	if err != nil || item == nil {
		return nil, err
	}
	result := toFacadePasskey(*item)
	return &result, nil
}

func (s *Service) SavePasskeyCredential(ctx context.Context, command facade.SavePasskeyCredentialCommand) error {
	return s.withTx(ctx, func(txCtx context.Context) error {
		return s.domain.SavePasskeyCredential(txCtx, domain.SavePasskeyInput{
			UserID:            command.UserID,
			CredentialKey:     command.CredentialKey,
			PublicKeyCose:     command.PublicKeyCose,
			SignCount:         command.SignCount,
			UserHandle:        command.UserHandle,
			AAGUID:            command.AAGUID,
			Transports:        command.Transports,
			AttestationFormat: command.AttestationFormat,
			DisplayName:       command.DisplayName,
			DisableExisting:   command.DisableExisting,
			VerifiedAt:        command.VerifiedAt,
			CreatorID:         command.CreatorID,
			UpdaterID:         command.UpdaterID,
		})
	})
}

func (s *Service) CompletePasskeyBinding(ctx context.Context, command facade.CompletePasskeyBindingCommand) error {
	return s.withTx(ctx, func(txCtx context.Context) error {
		if err := s.domain.SavePasskeyCredential(txCtx, domain.SavePasskeyInput{
			UserID:            command.UserID,
			CredentialKey:     command.CredentialKey,
			PublicKeyCose:     command.PublicKeyCose,
			SignCount:         command.SignCount,
			UserHandle:        command.UserHandle,
			AAGUID:            command.AAGUID,
			Transports:        command.Transports,
			AttestationFormat: command.AttestationFormat,
			DisplayName:       command.DisplayName,
			DisableExisting:   command.DisableExisting,
			VerifiedAt:        command.VerifiedAt,
		}); err != nil {
			return err
		}
		_, err := s.domain.RegenerateRecoveryCodes(txCtx, command.UserID, command.RecoveryBatchSize)
		return err
	})
}

func (s *Service) DisablePasskeyCredential(ctx context.Context, userID int64, credentialKey string) (bool, error) {
	var (
		disabled bool
		err      error
	)
	err = s.withTx(ctx, func(txCtx context.Context) error {
		disabled, err = s.domain.DisablePasskeyCredential(txCtx, userID, credentialKey)
		return err
	})
	return disabled, err
}

func (s *Service) UpdatePasskeyUsage(ctx context.Context, credentialKey string, signCount int64, usedAt time.Time) error {
	return s.withTx(ctx, func(txCtx context.Context) error {
		return s.domain.UpdatePasskeyUsage(txCtx, credentialKey, signCount, usedAt)
	})
}

func (s *Service) CountAvailableRecoveryCodes(ctx context.Context, userID int64) (int, error) {
	return s.domain.CountAvailableRecoveryCodes(ctx, userID)
}

func (s *Service) RegenerateRecoveryCodes(ctx context.Context, userID int64, batchSize int) (*facade.RegeneratedRecoveryCodes, error) {
	var (
		result *domain.RegeneratedRecoveryCodes
		err    error
	)
	err = s.withTx(ctx, func(txCtx context.Context) error {
		result, err = s.domain.RegenerateRecoveryCodes(txCtx, userID, batchSize)
		return err
	})
	if err != nil || result == nil {
		return nil, err
	}
	return &facade.RegeneratedRecoveryCodes{
		BatchIdentifier: result.BatchIdentifier,
		PlainCodes:      result.PlainCodes,
		GeneratedAt:     result.GeneratedAt,
	}, nil
}

func (s *Service) ConsumeRecoveryCode(ctx context.Context, userID int64, recoveryCode string, usedAt time.Time) (bool, error) {
	var (
		consumed bool
		err      error
	)
	err = s.withTx(ctx, func(txCtx context.Context) error {
		consumed, err = s.domain.ConsumeRecoveryCode(txCtx, userID, recoveryCode, usedAt)
		return err
	})
	return consumed, err
}

func (s *Service) withTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.transactor == nil || !s.transactor.Enabled() {
		return fn(ctx)
	}
	return s.transactor.WithinTransaction(ctx, fn)
}

func toFacadePasskey(item domain.PasskeyCredential) facade.PasskeyCredential {
	return facade.PasskeyCredential{
		UserID:            item.UserID,
		CredentialKey:     item.CredentialKey,
		PublicKeyCose:     item.PublicKeyCose,
		SignCount:         item.SignCount,
		UserHandle:        item.UserHandle,
		AAGUID:            item.AAGUID,
		Transports:        item.Transports,
		AttestationFormat: item.AttestationFormat,
		DisplayName:       item.DisplayName,
		VerifiedAt:        item.VerifiedAt,
		CreateTime:        item.CreateTime,
		LastUsedAt:        item.LastUsedAt,
	}
}
