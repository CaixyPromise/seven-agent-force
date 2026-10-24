package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain/provider"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

type CompletionHandler struct {
	store             provider.SubjectCredentialStore
	recoveryBatchSize int
}

func NewCompletionHandler(store provider.SubjectCredentialStore, recoveryBatchSize int) *CompletionHandler {
	return &CompletionHandler{store: store, recoveryBatchSize: recoveryBatchSize}
}

func (h *CompletionHandler) OnPassed(ctx context.Context, session *domain.ChallengeSession) error {
	if h == nil || h.store == nil {
		return apperrors.System("challenge completion handler未配置")
	}
	if session == nil {
		return apperrors.Params("challenge session不能为空")
	}
	switch domain.BusinessAction(strings.TrimSpace(session.BusinessAction)) {
	case domain.BusinessActionMFAOTPBind, domain.BusinessActionMFAOTPSwitch:
		plainSecret, _ := session.EnsureSessionContext()["otp.newSecretEncrypted"].(string)
		plainSecret = strings.TrimSpace(plainSecret)
		if plainSecret == "" {
			plainSecret, _ = session.EnsureSessionContext()["otp.pendingSecretPlain"].(string)
		}
		plainSecret = strings.TrimSpace(plainSecret)
		if plainSecret == "" {
			return apperrors.Params("challenge otp pending secret不能为空")
		}
		batchSize, err := h.requiredRecoveryBatchSize()
		if err != nil {
			return err
		}
		return h.store.CompleteTotpBinding(ctx, session, plainSecret, time.Now(), batchSize)
	case domain.BusinessActionMFAPasskeyBind, domain.BusinessActionMFAPasskeySwitch:
		registration, err := parsePasskeyRegistration(session)
		if err != nil {
			return err
		}
		batchSize, err := h.requiredRecoveryBatchSize()
		if err != nil {
			return err
		}
		return h.store.CompletePasskeyBinding(
			ctx,
			session,
			registration,
			domain.BusinessAction(strings.TrimSpace(session.BusinessAction)) == domain.BusinessActionMFAPasskeySwitch,
			time.Now(),
			batchSize,
		)
	default:
		return nil
	}
}

func (h *CompletionHandler) requiredRecoveryBatchSize() (int, error) {
	if h.recoveryBatchSize <= 0 {
		return 0, apperrors.System("challenge recovery batch size未配置")
	}
	return h.recoveryBatchSize, nil
}

func parsePasskeyRegistration(session *domain.ChallengeSession) (domain.PasskeyRegistration, error) {
	if session == nil || session.SessionContext == nil {
		return domain.PasskeyRegistration{}, apperrors.Params("challenge passkey registration不能为空")
	}
	candidate := session.SessionContext["passkey.registration"]
	mapValue, ok := candidate.(map[string]any)
	if !ok || len(mapValue) == 0 {
		return domain.PasskeyRegistration{}, apperrors.Params("challenge passkey registration不能为空")
	}
	credentialIdentifier, ok := normalizeRequiredString(mapValue["credentialIdentifier"])
	if !ok {
		return domain.PasskeyRegistration{}, apperrors.Params("challenge passkey registration字段不完整")
	}
	publicKeyCose, ok := normalizeRequiredString(mapValue["publicKeyCose"])
	if !ok {
		return domain.PasskeyRegistration{}, apperrors.Params("challenge passkey registration字段不完整")
	}
	registration := domain.PasskeyRegistration{
		CredentialIdentifier: credentialIdentifier,
		PublicKeyCose:        publicKeyCose,
		SignCount:            parseInt64(mapValue["signCount"]),
		UserHandle:           normalizeOptionalValue(mapValue["userHandle"]),
		AAGUID:               normalizeOptionalValue(mapValue["aaguid"]),
		Transports:           normalizeOptionalValue(mapValue["transports"]),
		AttestationFormat:    normalizeOptionalValue(mapValue["attestationFormat"]),
		DisplayName:          normalizeOptionalValue(mapValue["displayName"]),
	}
	if registration.CredentialIdentifier == "" || registration.PublicKeyCose == "" {
		return domain.PasskeyRegistration{}, apperrors.Params("challenge passkey registration字段不完整")
	}
	return registration, nil
}

func parseInt64(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int32:
		return int64(typed)
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	case float32:
		return int64(typed)
	case string:
		typed = strings.TrimSpace(typed)
		if typed == "" {
			return 0
		}
		var result int64
		fmt.Sscan(typed, &result)
		return result
	default:
		return 0
	}
}

func normalizeOptionalValue(value any) string {
	text := strings.TrimSpace(fmt.Sprintf("%v", value))
	if strings.EqualFold(text, "<nil>") || strings.EqualFold(text, "null") {
		return ""
	}
	return text
}

func normalizeRequiredString(value any) (string, bool) {
	typed, ok := value.(string)
	if !ok {
		return "", false
	}
	typed = strings.TrimSpace(typed)
	if typed == "" {
		return "", false
	}
	return typed, true
}
