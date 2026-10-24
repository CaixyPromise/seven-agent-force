package domain

import (
	"context"
	"fmt"
	"strings"
	"time"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xtime"
	"github.com/google/uuid"
)

type Service struct {
	repo         UserCredentialRepository
	idGen        IDGenerator
	recovery     RecoveryCodeService
	payloadCodec RecoveryPayloadCodec
}

func NewService(repo UserCredentialRepository, idGen IDGenerator, recovery RecoveryCodeService, payloadCodec RecoveryPayloadCodec) *Service {
	return &Service{
		repo:         repo,
		idGen:        idGen,
		recovery:     recovery,
		payloadCodec: payloadCodec,
	}
}

func (s *Service) FindActivePasswordByUserID(ctx context.Context, userID int64) (*PasswordCredential, error) {
	if err := validateUserID(userID); err != nil {
		return nil, err
	}
	record, err := s.repo.FindSingleActive(ctx, userID, CredentialTypePassword, PrimaryCredentialKey)
	if err != nil || record == nil {
		return nil, err
	}
	return toPasswordCredential(record), nil
}

func (s *Service) UpsertPasswordCredential(ctx context.Context, input UpsertPasswordInput) error {
	if err := validateUserID(input.UserID); err != nil {
		return err
	}
	if strings.TrimSpace(input.PasswordHash) == "" {
		return apperrors.Params("passwordHash不能为空")
	}
	existing, err := s.repo.FindSingleAny(ctx, input.UserID, CredentialTypePassword, PrimaryCredentialKey)
	if err != nil {
		return err
	}
	now := chooseTime(input.PasswordChangedAt)
	if existing == nil {
		return s.repo.Insert(ctx, &CredentialRecord{
			ID:                 s.idGen.NextID(),
			UserID:             input.UserID,
			CredentialType:     CredentialTypePassword,
			CredentialKey:      PrimaryCredentialKey,
			SecretHash:         input.PasswordHash,
			Status:             CredentialStatusActive,
			MustChangePassword: input.MustChangePassword,
			PasswordChangedAt:  &now,
			VerifiedAt:         &now,
			CreatorID:          input.CreatorID,
			UpdaterID:          input.UpdaterID,
			CreateTime:         &now,
			UpdateTime:         &now,
			IsDeleted:          0,
		})
	}
	existing.SecretHash = input.PasswordHash
	existing.Status = CredentialStatusActive
	existing.InvalidatedAt = nil
	existing.MustChangePassword = input.MustChangePassword
	existing.PasswordChangedAt = &now
	existing.VerifiedAt = &now
	existing.UpdaterID = input.UpdaterID
	existing.UpdateTime = &now
	return s.repo.Update(ctx, existing)
}

func (s *Service) MarkPasswordUsed(ctx context.Context, userID int64, usedAt time.Time) error {
	if err := validateUserID(userID); err != nil {
		return err
	}
	return s.repo.UpdateLastUsedByScope(ctx, userID, CredentialTypePassword, PrimaryCredentialKey, CredentialStatusActive, chooseInstant(usedAt))
}

func (s *Service) FindActiveTotpByUserID(ctx context.Context, userID int64) (*TotpCredential, error) {
	if err := validateUserID(userID); err != nil {
		return nil, err
	}
	record, err := s.repo.FindSingleActive(ctx, userID, CredentialTypeTotp, PrimaryCredentialKey)
	if err != nil || record == nil {
		return nil, err
	}
	return toTotpCredential(record), nil
}

func (s *Service) UpsertTotpCredential(ctx context.Context, input UpsertTotpInput) error {
	if err := validateUserID(input.UserID); err != nil {
		return err
	}
	if strings.TrimSpace(input.SecretCiphertext) == "" {
		return apperrors.Params("secretCiphertext不能为空")
	}
	existing, err := s.repo.FindSingleAny(ctx, input.UserID, CredentialTypeTotp, PrimaryCredentialKey)
	if err != nil {
		return err
	}
	verifiedAt := chooseTime(input.VerifiedAt)
	if existing == nil {
		return s.repo.Insert(ctx, &CredentialRecord{
			ID:               s.idGen.NextID(),
			UserID:           input.UserID,
			CredentialType:   CredentialTypeTotp,
			CredentialKey:    PrimaryCredentialKey,
			SecretCiphertext: input.SecretCiphertext,
			Status:           CredentialStatusActive,
			VerifiedAt:       &verifiedAt,
			CreatorID:        input.CreatorID,
			UpdaterID:        input.UpdaterID,
			CreateTime:       &verifiedAt,
			UpdateTime:       &verifiedAt,
			IsDeleted:        0,
		})
	}
	existing.SecretCiphertext = input.SecretCiphertext
	existing.Status = CredentialStatusActive
	existing.InvalidatedAt = nil
	existing.VerifiedAt = &verifiedAt
	existing.UpdaterID = input.UpdaterID
	existing.UpdateTime = &verifiedAt
	return s.repo.Update(ctx, existing)
}

func (s *Service) DisableTotpCredential(ctx context.Context, userID int64) (bool, error) {
	if err := validateUserID(userID); err != nil {
		return false, err
	}
	invalidatedAt := xtime.Now()
	affected, err := s.repo.UpdateStatusByScope(ctx, userID, CredentialTypeTotp, PrimaryCredentialKey, CredentialStatusActive, CredentialStatusDisabled, invalidatedAt)
	if err != nil {
		return false, err
	}
	if affected > 0 {
		return true, nil
	}
	record, err := s.repo.FindSingleAny(ctx, userID, CredentialTypeTotp, PrimaryCredentialKey)
	if err != nil || record == nil {
		return false, err
	}
	return record.Status == CredentialStatusDisabled, nil
}

func (s *Service) MarkTotpUsed(ctx context.Context, userID int64, usedAt time.Time) error {
	if err := validateUserID(userID); err != nil {
		return err
	}
	return s.repo.UpdateLastUsedByScope(ctx, userID, CredentialTypeTotp, PrimaryCredentialKey, CredentialStatusActive, chooseInstant(usedAt))
}

func (s *Service) ListActivePasskeys(ctx context.Context, userID int64) ([]PasskeyCredential, error) {
	if err := validateUserID(userID); err != nil {
		return nil, err
	}
	items, err := s.repo.ListActiveByUserAndType(ctx, userID, CredentialTypePasskey)
	if err != nil {
		return nil, err
	}
	result := make([]PasskeyCredential, 0, len(items))
	for _, item := range items {
		passkey, err := s.toPasskeyCredential(item)
		if err != nil {
			continue
		}
		result = append(result, passkey)
	}
	return result, nil
}

func (s *Service) FindActivePasskeyByCredentialKey(ctx context.Context, credentialKey string) (*PasskeyCredential, error) {
	credentialKey = strings.TrimSpace(credentialKey)
	if credentialKey == "" {
		return nil, apperrors.Params("credentialKey不能为空")
	}
	record, err := s.repo.FindActiveByTypeAndKey(ctx, CredentialTypePasskey, credentialKey)
	if err != nil || record == nil {
		return nil, err
	}
	passkey, err := s.toPasskeyCredential(*record)
	if err != nil {
		return nil, err
	}
	return &passkey, nil
}

func (s *Service) SavePasskeyCredential(ctx context.Context, input SavePasskeyInput) error {
	if err := validateUserID(input.UserID); err != nil {
		return err
	}
	if strings.TrimSpace(input.CredentialKey) == "" {
		return apperrors.Params("credentialKey不能为空")
	}
	if strings.TrimSpace(input.PublicKeyCose) == "" {
		return apperrors.Params("publicKeyCose不能为空")
	}
	existedActive, err := s.repo.FindActiveByTypeAndKey(ctx, CredentialTypePasskey, input.CredentialKey)
	if err != nil {
		return err
	}
	if existedActive != nil && existedActive.UserID != input.UserID {
		return apperrors.Params("credentialKey已被其他用户占用")
	}
	verifiedAt := chooseTime(input.VerifiedAt)
	if input.DisableExisting {
		if _, err := s.repo.UpdateStatusByUserAndType(ctx, input.UserID, CredentialTypePasskey, CredentialStatusActive, CredentialStatusDisabled, verifiedAt); err != nil {
			return err
		}
	}
	payloadJSON, err := s.payloadCodec.EncodePasskey(PasskeyPayload{
		PublicKeyCose:     input.PublicKeyCose,
		SignCount:         input.SignCount,
		UserHandle:        strings.TrimSpace(input.UserHandle),
		AAGUID:            strings.TrimSpace(input.AAGUID),
		Transports:        strings.TrimSpace(input.Transports),
		AttestationFormat: strings.TrimSpace(input.AttestationFormat),
		DisplayName:       strings.TrimSpace(input.DisplayName),
	})
	if err != nil {
		return err
	}
	existing, err := s.repo.FindAnyByTypeAndKey(ctx, CredentialTypePasskey, input.CredentialKey)
	if err != nil {
		return err
	}
	if existing == nil {
		return s.repo.Insert(ctx, &CredentialRecord{
			ID:                    s.idGen.NextID(),
			UserID:                input.UserID,
			CredentialType:        CredentialTypePasskey,
			CredentialKey:         input.CredentialKey,
			CredentialPayloadJSON: payloadJSON,
			Status:                CredentialStatusActive,
			VerifiedAt:            &verifiedAt,
			LastUsedAt:            &verifiedAt,
			CreatorID:             input.CreatorID,
			UpdaterID:             input.UpdaterID,
			CreateTime:            &verifiedAt,
			UpdateTime:            &verifiedAt,
			IsDeleted:             0,
		})
	}
	existing.UserID = input.UserID
	existing.CredentialPayloadJSON = payloadJSON
	existing.Status = CredentialStatusActive
	existing.InvalidatedAt = nil
	existing.VerifiedAt = &verifiedAt
	existing.LastUsedAt = &verifiedAt
	existing.UpdaterID = input.UpdaterID
	existing.UpdateTime = &verifiedAt
	return s.repo.Update(ctx, existing)
}

func (s *Service) DisablePasskeyCredential(ctx context.Context, userID int64, credentialKey string) (bool, error) {
	if err := validateUserID(userID); err != nil {
		return false, err
	}
	credentialKey = strings.TrimSpace(credentialKey)
	if credentialKey == "" {
		return false, apperrors.Params("credentialKey不能为空")
	}
	invalidatedAt := xtime.Now()
	affected, err := s.repo.UpdateStatusByScope(ctx, userID, CredentialTypePasskey, credentialKey, CredentialStatusActive, CredentialStatusDisabled, invalidatedAt)
	if err != nil {
		return false, err
	}
	if affected > 0 {
		return true, nil
	}
	record, err := s.repo.FindAnyByTypeAndKey(ctx, CredentialTypePasskey, credentialKey)
	if err != nil || record == nil {
		return false, err
	}
	return record.UserID == userID && record.Status == CredentialStatusDisabled, nil
}

func (s *Service) UpdatePasskeyUsage(ctx context.Context, credentialKey string, signCount int64, usedAt time.Time) error {
	credentialKey = strings.TrimSpace(credentialKey)
	if credentialKey == "" {
		return nil
	}
	record, err := s.repo.FindActiveByTypeAndKey(ctx, CredentialTypePasskey, credentialKey)
	if err != nil || record == nil {
		return err
	}
	payload, err := s.payloadCodec.DecodePasskey(record.CredentialPayloadJSON)
	if err != nil {
		return err
	}
	payload.SignCount = signCount
	record.CredentialPayloadJSON, err = s.payloadCodec.EncodePasskey(payload)
	if err != nil {
		return err
	}
	used := chooseInstant(usedAt)
	record.LastUsedAt = &used
	record.UpdateTime = &used
	return s.repo.Update(ctx, record)
}

func (s *Service) CountAvailableRecoveryCodes(ctx context.Context, userID int64) (int, error) {
	if err := validateUserID(userID); err != nil {
		return 0, err
	}
	return s.repo.CountActiveByUserAndType(ctx, userID, CredentialTypeRecoveryCode)
}

func (s *Service) RegenerateRecoveryCodes(ctx context.Context, userID int64, batchSize int) (*RegeneratedRecoveryCodes, error) {
	if err := validateUserID(userID); err != nil {
		return nil, err
	}
	if batchSize <= 0 {
		return nil, apperrors.Params("batchSize必须大于0")
	}
	now := xtime.Now()
	if _, err := s.repo.InvalidateActiveRecoveryCodes(ctx, userID, now); err != nil {
		return nil, err
	}
	batchIdentifier := "batch_" + compactUUID()
	plainCodes, err := s.recovery.GenerateCodes(batchSize)
	if err != nil {
		return nil, fmt.Errorf("generate recovery codes: %w", err)
	}
	for _, code := range plainCodes {
		salt, err := s.recovery.GenerateSalt()
		if err != nil {
			return nil, fmt.Errorf("generate recovery code salt: %w", err)
		}
		hash, err := s.recovery.HashCode(code, salt, DefaultRecoveryHashIteration)
		if err != nil {
			return nil, fmt.Errorf("hash recovery code: %w", err)
		}
		payloadJSON, err := s.payloadCodec.EncodeRecoveryCode(RecoveryCodePayload{
			Salt:            salt,
			HashAlgorithm:   s.recovery.HashAlgorithm(),
			IterationCount:  DefaultRecoveryHashIteration,
			BatchIdentifier: batchIdentifier,
		})
		if err != nil {
			return nil, err
		}
		record := &CredentialRecord{
			ID:                    s.idGen.NextID(),
			UserID:                userID,
			CredentialType:        CredentialTypeRecoveryCode,
			CredentialKey:         "rc_" + compactUUID(),
			SecretHash:            hash,
			CredentialPayloadJSON: payloadJSON,
			Status:                CredentialStatusActive,
			VerifiedAt:            &now,
			CreateTime:            &now,
			UpdateTime:            &now,
			IsDeleted:             0,
		}
		if err := s.repo.Insert(ctx, record); err != nil {
			return nil, err
		}
	}
	return &RegeneratedRecoveryCodes{
		BatchIdentifier: batchIdentifier,
		PlainCodes:      plainCodes,
		GeneratedAt:     &now,
	}, nil
}

func (s *Service) ConsumeRecoveryCode(ctx context.Context, userID int64, recoveryCode string, usedAt time.Time) (bool, error) {
	if err := validateUserID(userID); err != nil {
		return false, err
	}
	if strings.TrimSpace(recoveryCode) == "" {
		return false, nil
	}
	candidates, err := s.repo.ListActiveRecoveryCodes(ctx, userID)
	if err != nil {
		return false, err
	}
	used := chooseInstant(usedAt)
	for _, candidate := range candidates {
		payload, err := s.payloadCodec.DecodeRecoveryCode(candidate.CredentialPayloadJSON)
		if err != nil {
			continue
		}
		if !isValidRecoveryCodePayload(payload) {
			continue
		}
		if !s.recovery.VerifyCode(recoveryCode, payload.Salt, payload.IterationCount, candidate.SecretHash) {
			continue
		}
		ok, err := s.repo.ConsumeRecoveryCodeByID(ctx, candidate.ID, used)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func validateUserID(userID int64) error {
	if userID <= 0 {
		return apperrors.Params("userId不能为空")
	}
	return nil
}

func chooseTime(value *time.Time) time.Time {
	if value == nil || value.IsZero() {
		return xtime.Now()
	}
	return value.UTC()
}

func chooseInstant(value time.Time) time.Time {
	if value.IsZero() {
		return xtime.Now()
	}
	return value.UTC()
}

func compactUUID() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}

func isValidRecoveryCodePayload(payload RecoveryCodePayload) bool {
	return strings.TrimSpace(payload.Salt) != "" &&
		strings.TrimSpace(payload.HashAlgorithm) != "" &&
		payload.IterationCount > 0 &&
		strings.TrimSpace(payload.BatchIdentifier) != ""
}

func toPasswordCredential(record *CredentialRecord) *PasswordCredential {
	return &PasswordCredential{
		UserID:             record.UserID,
		CredentialKey:      record.CredentialKey,
		PasswordHash:       record.SecretHash,
		MustChangePassword: record.MustChangePassword,
		PasswordChangedAt:  record.PasswordChangedAt,
		LastUsedAt:         record.LastUsedAt,
		VerifiedAt:         record.VerifiedAt,
	}
}

func toTotpCredential(record *CredentialRecord) *TotpCredential {
	return &TotpCredential{
		UserID:           record.UserID,
		CredentialKey:    record.CredentialKey,
		SecretCiphertext: record.SecretCiphertext,
		VerifiedAt:       record.VerifiedAt,
		LastUsedAt:       record.LastUsedAt,
	}
}

func (s *Service) toPasskeyCredential(record CredentialRecord) (PasskeyCredential, error) {
	payload, err := s.payloadCodec.DecodePasskey(record.CredentialPayloadJSON)
	if err != nil {
		return PasskeyCredential{}, err
	}
	return PasskeyCredential{
		UserID:            record.UserID,
		CredentialKey:     record.CredentialKey,
		PublicKeyCose:     payload.PublicKeyCose,
		SignCount:         payload.SignCount,
		UserHandle:        payload.UserHandle,
		AAGUID:            payload.AAGUID,
		Transports:        payload.Transports,
		AttestationFormat: payload.AttestationFormat,
		DisplayName:       payload.DisplayName,
		VerifiedAt:        record.VerifiedAt,
		CreateTime:        record.CreateTime,
		LastUsedAt:        record.LastUsedAt,
	}, nil
}
