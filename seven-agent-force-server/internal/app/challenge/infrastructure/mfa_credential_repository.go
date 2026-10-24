package infrastructure

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	credentialfacade "github.com/CaixyPromise/seven-agent/force/internal/app/credential/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

type MfaCredentialStore struct {
	credentials credentialfacade.UserCredentialFacade
	subjects    *SubjectResolver
}

type MfaCredentialRepository struct {
	credentials credentialfacade.UserCredentialFacade
}

func NewMfaCredentialRepository(credentials credentialfacade.UserCredentialFacade) *MfaCredentialRepository {
	return &MfaCredentialRepository{credentials: credentials}
}

func NewMfaCredentialStore(credentials credentialfacade.UserCredentialFacade, subjects *SubjectResolver) *MfaCredentialStore {
	return &MfaCredentialStore{
		credentials: credentials,
		subjects:    subjects,
	}
}

func (r *MfaCredentialRepository) FindEnabledOtpBinding(ctx context.Context, userID int64) (*domain.OtpBindingRecord, error) {
	record, err := r.credentials.FindActiveTotpByUserID(ctx, userID)
	if err != nil || record == nil {
		return nil, err
	}
	return &domain.OtpBindingRecord{
		UserID:          record.UserID,
		SecretEncrypted: record.SecretCiphertext,
		VerifiedAt:      record.VerifiedAt,
		LastUsedAt:      record.LastUsedAt,
	}, nil
}

func (r *MfaCredentialRepository) ConsumeRecoveryCode(ctx context.Context, userID int64, recoveryCode string, usedAt time.Time) (bool, error) {
	return r.credentials.ConsumeRecoveryCode(ctx, userID, recoveryCode, usedAt)
}

func (r *MfaCredentialRepository) MarkTotpUsed(ctx context.Context, userID int64, usedAt time.Time) error {
	return r.credentials.MarkTotpUsed(ctx, userID, usedAt)
}

func (r *MfaCredentialStore) FindEnabledOtpBinding(ctx context.Context, session *domain.ChallengeSession) (*domain.OtpBindingRecord, error) {
	subject, err := r.resolveSubject(ctx, session)
	if err != nil || subject == nil || subject.UserID <= 0 {
		return nil, err
	}
	record, err := r.credentials.FindActiveTotpByUserID(ctx, subject.UserID)
	if err != nil || record == nil {
		return nil, err
	}
	return &domain.OtpBindingRecord{
		UserID:          record.UserID,
		SecretEncrypted: record.SecretCiphertext,
		VerifiedAt:      record.VerifiedAt,
		LastUsedAt:      record.LastUsedAt,
	}, nil
}

func (r *MfaCredentialStore) FindEnabledOtpSecret(ctx context.Context, session *domain.ChallengeSession) (string, error) {
	subject, err := r.resolveSubject(ctx, session)
	if err != nil || subject == nil || subject.UserID <= 0 {
		return "", err
	}
	record, err := r.credentials.FindActiveTotpSecretByUserID(ctx, subject.UserID)
	if err != nil || record == nil {
		return "", err
	}
	return strings.TrimSpace(record.Secret), nil
}

func (r *MfaCredentialStore) FindPasswordCredential(ctx context.Context, session *domain.ChallengeSession) (string, error) {
	subject, err := r.resolveSubject(ctx, session)
	if err != nil || subject == nil || subject.UserID <= 0 {
		return "", err
	}
	record, err := r.credentials.FindActivePasswordByUserID(ctx, subject.UserID)
	if err != nil || record == nil {
		return "", err
	}
	return strings.TrimSpace(record.PasswordHash), nil
}

func (r *MfaCredentialStore) FindPasskey(ctx context.Context, credentialKey string) (*domain.PasskeyRegistration, error) {
	item, err := r.credentials.FindActivePasskeyByCredentialKey(ctx, strings.TrimSpace(credentialKey))
	if err != nil || item == nil {
		return nil, err
	}
	return &domain.PasskeyRegistration{
		CredentialIdentifier: strings.TrimSpace(item.CredentialKey),
		PublicKeyCose:        strings.TrimSpace(item.PublicKeyCose),
		SignCount:            item.SignCount,
		UserHandle:           strings.TrimSpace(item.UserHandle),
		AAGUID:               strings.TrimSpace(item.AAGUID),
		Transports:           strings.TrimSpace(item.Transports),
		AttestationFormat:    strings.TrimSpace(item.AttestationFormat),
		DisplayName:          strings.TrimSpace(item.DisplayName),
	}, nil
}

func (r *MfaCredentialStore) ListPasskeys(ctx context.Context, session *domain.ChallengeSession) ([]domain.PasskeyRegistration, error) {
	subject, err := r.resolveSubject(ctx, session)
	if err != nil || subject == nil || subject.UserID <= 0 {
		return nil, err
	}
	items, err := r.credentials.ListActivePasskeys(ctx, subject.UserID)
	if err != nil || len(items) == 0 {
		return nil, err
	}
	result := make([]domain.PasskeyRegistration, 0, len(items))
	for _, item := range items {
		result = append(result, domain.PasskeyRegistration{
			CredentialIdentifier: strings.TrimSpace(item.CredentialKey),
			PublicKeyCose:        strings.TrimSpace(item.PublicKeyCose),
			SignCount:            item.SignCount,
			UserHandle:           strings.TrimSpace(item.UserHandle),
			AAGUID:               strings.TrimSpace(item.AAGUID),
			Transports:           strings.TrimSpace(item.Transports),
			AttestationFormat:    strings.TrimSpace(item.AttestationFormat),
			DisplayName:          strings.TrimSpace(item.DisplayName),
		})
	}
	return result, nil
}

func (r *MfaCredentialStore) UpdatePasskeyUsage(ctx context.Context, credentialKey string, signCount int64, usedAt time.Time) error {
	return r.credentials.UpdatePasskeyUsage(ctx, strings.TrimSpace(credentialKey), signCount, usedAt)
}

func (r *MfaCredentialStore) ConsumeRecoveryCode(ctx context.Context, session *domain.ChallengeSession, recoveryCode string, usedAt time.Time) (bool, error) {
	subject, err := r.resolveSubject(ctx, session)
	if err != nil || subject == nil || subject.UserID <= 0 {
		return false, err
	}
	return r.credentials.ConsumeRecoveryCode(ctx, subject.UserID, recoveryCode, usedAt)
}

func (r *MfaCredentialStore) CountAvailableRecoveryCodes(ctx context.Context, session *domain.ChallengeSession) (int, error) {
	subject, err := r.resolveSubject(ctx, session)
	if err != nil || subject == nil || subject.UserID <= 0 {
		return 0, err
	}
	return r.credentials.CountAvailableRecoveryCodes(ctx, subject.UserID)
}

func (r *MfaCredentialStore) CompleteTotpBinding(ctx context.Context, session *domain.ChallengeSession, plainSecret string, verifiedAt time.Time, recoveryBatchSize int) error {
	subject, err := r.resolveRequiredSubject(ctx, session)
	if err != nil {
		return err
	}
	return r.credentials.CompleteTotpBinding(ctx, credentialfacade.CompleteTotpBindingCommand{
		UserID:            subject.UserID,
		PlainSecret:       strings.TrimSpace(plainSecret),
		VerifiedAt:        timePointer(verifiedAt),
		RecoveryBatchSize: recoveryBatchSize,
	})
}

func (r *MfaCredentialStore) CompletePasskeyBinding(ctx context.Context, session *domain.ChallengeSession, registration domain.PasskeyRegistration, disableExisting bool, verifiedAt time.Time, recoveryBatchSize int) error {
	subject, err := r.resolveRequiredSubject(ctx, session)
	if err != nil {
		return err
	}
	return r.credentials.CompletePasskeyBinding(ctx, credentialfacade.CompletePasskeyBindingCommand{
		UserID:            subject.UserID,
		CredentialKey:     strings.TrimSpace(registration.CredentialIdentifier),
		PublicKeyCose:     strings.TrimSpace(registration.PublicKeyCose),
		SignCount:         registration.SignCount,
		UserHandle:        strings.TrimSpace(registration.UserHandle),
		AAGUID:            strings.TrimSpace(registration.AAGUID),
		Transports:        strings.TrimSpace(registration.Transports),
		AttestationFormat: strings.TrimSpace(registration.AttestationFormat),
		DisplayName:       strings.TrimSpace(registration.DisplayName),
		DisableExisting:   disableExisting,
		VerifiedAt:        timePointer(verifiedAt),
		RecoveryBatchSize: recoveryBatchSize,
	})
}

func (r *MfaCredentialStore) ResolveAccountName(ctx context.Context, session *domain.ChallengeSession) (string, error) {
	subject, err := r.resolveSubject(ctx, session)
	if err != nil {
		return "user", err
	}
	if subject != nil {
		if value := strings.TrimSpace(subject.AccountName); value != "" {
			return value, nil
		}
	}
	if session == nil {
		return "user", nil
	}
	raw := strings.TrimSpace(session.SubjectIdentifier)
	if strings.HasPrefix(raw, "login:") {
		if value := strings.TrimSpace(strings.TrimPrefix(raw, "login:")); value != "" {
			return value, nil
		}
	}
	return "user", nil
}

func (r *MfaCredentialStore) ResolveTargetEmail(ctx context.Context, session *domain.ChallengeSession) (string, error) {
	if session == nil {
		return "", nil
	}
	if extension, ok := session.EnsureSessionContext()["extensionContext"].(map[string]any); ok {
		if userAccount := contextString(extension["userAccount"]); userAccount != "" && r.subjects != nil {
			subject, err := r.subjects.findByAccount(ctx, userAccount)
			if err != nil {
				return "", err
			}
			if subject != nil {
				return strings.TrimSpace(subject.Email), nil
			}
		}
	}
	if r.subjects == nil {
		return "", nil
	}
	raw := strings.TrimSpace(session.SubjectIdentifier)
	switch {
	case strings.HasPrefix(raw, "user:"):
		id, err := parseSubjectUserID(raw)
		if err != nil {
			return "", nil
		}
		subject, err := r.subjects.findByID(ctx, id)
		if err != nil {
			return "", err
		}
		if subject != nil {
			return strings.TrimSpace(subject.Email), nil
		}
	case strings.HasPrefix(raw, "login:"):
		subject, err := r.subjects.findByAccount(ctx, strings.TrimSpace(strings.TrimPrefix(raw, "login:")))
		if err != nil {
			return "", err
		}
		if subject != nil {
			return strings.TrimSpace(subject.Email), nil
		}
	}
	return "", nil
}

func (r *MfaCredentialStore) resolveRequiredSubject(ctx context.Context, session *domain.ChallengeSession) (*ResolvedSubject, error) {
	subject, err := r.resolveSubject(ctx, session)
	if err != nil {
		return nil, err
	}
	if subject == nil || subject.UserID <= 0 {
		return nil, apperrors.Params("challenge session userId不能为空")
	}
	return subject, nil
}

func (r *MfaCredentialStore) resolveSubject(ctx context.Context, session *domain.ChallengeSession) (*ResolvedSubject, error) {
	if session == nil {
		return nil, nil
	}
	if r == nil || r.credentials == nil {
		return nil, apperrors.System("challenge credential store未配置")
	}
	if r.subjects == nil {
		if strings.TrimSpace(session.SubjectIdentifier) == "" {
			return nil, nil
		}
		return nil, apperrors.System("challenge subject resolver未配置")
	}
	subject, err := r.subjects.Resolve(ctx, session)
	if err != nil || subject == nil {
		return subject, err
	}
	if value := strings.TrimSpace(subject.Email); value != "" {
		session.EnsureSessionContext()["subject.email"] = value
	}
	return subject, nil
}

func contextString(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}

func parseSubjectUserID(raw string) (int64, error) {
	value := strings.TrimSpace(strings.TrimPrefix(raw, "user:"))
	if value == "" {
		return 0, fmt.Errorf("empty user id")
	}
	return strconv.ParseInt(value, 10, 64)
}

func timePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	utc := value.UTC()
	return &utc
}
