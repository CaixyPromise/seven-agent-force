package application

import (
	"context"
	"testing"
	"time"

	challengedomain "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	credentialfacade "github.com/CaixyPromise/seven-agent/force/internal/app/credential/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
)

func TestMfaManagementServiceCurrentUserMutationsRequireActionSpecificProof(t *testing.T) {
	t.Run("regenerate recovery codes requires recovery-code proof", func(t *testing.T) {
		credentials := &fakeMfaCredentialFacade{}
		service := NewMfaManagementService(credentials, nil, nil, nil, 10, 300)

		if _, err := service.RegenerateRecoveryCodesByUserID(context.Background(), 1001, validMfaProof(
			challengedomain.BusinessActionMFARecoveryCodesRegenerate,
			"",
			challengedomain.ChallengeTypeTimeBasedOneTimePassword,
		)); err == nil {
			t.Fatal("expected TOTP proof to be rejected for recovery-code regeneration")
		}
		if credentials.regenerateRecoveryCodesCalls != 0 {
			t.Fatal("weak proof regenerated recovery codes")
		}

		if _, err := service.RegenerateRecoveryCodesByUserID(context.Background(), 1001, validMfaProof(
			challengedomain.BusinessActionMFARecoveryCodesRegenerate,
			"",
			challengedomain.ChallengeTypeRecoveryCodeVerification,
		)); err != nil {
			t.Fatalf("expected recovery-code proof to pass: %v", err)
		}
		if credentials.regenerateRecoveryCodesCalls != 1 {
			t.Fatalf("expected one regeneration, got %d", credentials.regenerateRecoveryCodesCalls)
		}
	})

	t.Run("delete otp requires otp proof", func(t *testing.T) {
		credentials := &fakeMfaCredentialFacade{}
		service := NewMfaManagementService(credentials, nil, nil, nil, 10, 300)

		if _, err := service.DeleteOtpBindingByUserID(context.Background(), 1001, validMfaProof(
			challengedomain.BusinessActionMFAOTPDelete,
			"",
			challengedomain.ChallengeTypeEmailOneTimePassword,
		)); err == nil {
			t.Fatal("expected email OTP proof to be rejected for OTP deletion")
		}
		if credentials.disableTotpCalls != 0 {
			t.Fatal("weak proof disabled OTP")
		}

		if _, err := service.DeleteOtpBindingByUserID(context.Background(), 1001, validMfaProof(
			challengedomain.BusinessActionMFAOTPDelete,
			"",
			challengedomain.ChallengeTypeTimeBasedOneTimePassword,
		)); err != nil {
			t.Fatalf("expected TOTP proof to pass: %v", err)
		}
		if credentials.disableTotpCalls != 1 {
			t.Fatalf("expected one OTP disable, got %d", credentials.disableTotpCalls)
		}
	})

	t.Run("delete passkey requires passkey proof and credential binding", func(t *testing.T) {
		credentials := &fakeMfaCredentialFacade{}
		service := NewMfaManagementService(credentials, nil, nil, nil, 10, 300)

		if _, err := service.DeletePasskeyByUserID(context.Background(), 1001, "passkey-1", validMfaProof(
			challengedomain.BusinessActionMFAPasskeyDelete,
			"passkey:other",
			challengedomain.ChallengeTypeWebAuthnPasskeyAssertion,
		)); err == nil {
			t.Fatal("expected wrong passkey binding to be rejected")
		}
		if _, err := service.DeletePasskeyByUserID(context.Background(), 1001, "passkey-1", validMfaProof(
			challengedomain.BusinessActionMFAPasskeyDelete,
			"passkey:passkey-1",
			challengedomain.ChallengeTypeTimeBasedOneTimePassword,
		)); err == nil {
			t.Fatal("expected TOTP proof to be rejected for passkey deletion")
		}
		if credentials.disablePasskeyCalls != 0 {
			t.Fatal("weak proof disabled passkey")
		}

		if _, err := service.DeletePasskeyByUserID(context.Background(), 1001, "passkey-1", validMfaProof(
			challengedomain.BusinessActionMFAPasskeyDelete,
			"passkey:passkey-1",
			challengedomain.ChallengeTypeWebAuthnPasskeyAssertion,
		)); err != nil {
			t.Fatalf("expected passkey proof to pass: %v", err)
		}
		if credentials.disablePasskeyCalls != 1 {
			t.Fatalf("expected one passkey disable, got %d", credentials.disablePasskeyCalls)
		}
	})
}

func validMfaProof(action challengedomain.BusinessAction, binding string, method challengedomain.ChallengeType) stepup.ProofMetadata {
	return stepup.ProofMetadata{
		BusinessAction:        string(action),
		OperationBinding:      binding,
		ProofIdentifier:       "proof-jti",
		ChallengeIdentifier:   "challenge-id",
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: []string{string(method)},
	}
}

type fakeMfaCredentialFacade struct {
	regenerateRecoveryCodesCalls int
	disableTotpCalls             int
	disablePasskeyCalls          int
}

func (f *fakeMfaCredentialFacade) FindActivePasswordByUserID(context.Context, int64) (*credentialfacade.PasswordCredential, error) {
	return nil, nil
}

func (f *fakeMfaCredentialFacade) UpsertPasswordCredential(context.Context, credentialfacade.UpsertPasswordCredentialCommand) error {
	return nil
}

func (f *fakeMfaCredentialFacade) MarkPasswordUsed(context.Context, int64, time.Time) error {
	return nil
}

func (f *fakeMfaCredentialFacade) FindActiveTotpByUserID(context.Context, int64) (*credentialfacade.TotpCredential, error) {
	return nil, nil
}

func (f *fakeMfaCredentialFacade) FindActiveTotpSecretByUserID(context.Context, int64) (*credentialfacade.TotpSecret, error) {
	return nil, nil
}

func (f *fakeMfaCredentialFacade) UpsertTotpCredential(context.Context, credentialfacade.UpsertTotpCredentialCommand) error {
	return nil
}

func (f *fakeMfaCredentialFacade) CompleteTotpBinding(context.Context, credentialfacade.CompleteTotpBindingCommand) error {
	return nil
}

func (f *fakeMfaCredentialFacade) DisableTotpCredential(context.Context, int64) (bool, error) {
	f.disableTotpCalls++
	return true, nil
}

func (f *fakeMfaCredentialFacade) MarkTotpUsed(context.Context, int64, time.Time) error {
	return nil
}

func (f *fakeMfaCredentialFacade) ListActivePasskeys(context.Context, int64) ([]credentialfacade.PasskeyCredential, error) {
	return nil, nil
}

func (f *fakeMfaCredentialFacade) FindActivePasskeyByCredentialKey(context.Context, string) (*credentialfacade.PasskeyCredential, error) {
	return nil, nil
}

func (f *fakeMfaCredentialFacade) SavePasskeyCredential(context.Context, credentialfacade.SavePasskeyCredentialCommand) error {
	return nil
}

func (f *fakeMfaCredentialFacade) CompletePasskeyBinding(context.Context, credentialfacade.CompletePasskeyBindingCommand) error {
	return nil
}

func (f *fakeMfaCredentialFacade) DisablePasskeyCredential(context.Context, int64, string) (bool, error) {
	f.disablePasskeyCalls++
	return true, nil
}

func (f *fakeMfaCredentialFacade) UpdatePasskeyUsage(context.Context, string, int64, time.Time) error {
	return nil
}

func (f *fakeMfaCredentialFacade) CountAvailableRecoveryCodes(context.Context, int64) (int, error) {
	return 0, nil
}

func (f *fakeMfaCredentialFacade) RegenerateRecoveryCodes(context.Context, int64, int) (*credentialfacade.RegeneratedRecoveryCodes, error) {
	f.regenerateRecoveryCodesCalls++
	now := time.Now().UTC()
	return &credentialfacade.RegeneratedRecoveryCodes{
		BatchIdentifier: "batch-1",
		PlainCodes:      []string{"AAAA-BBBB"},
		GeneratedAt:     &now,
	}, nil
}

func (f *fakeMfaCredentialFacade) ConsumeRecoveryCode(context.Context, int64, string, time.Time) (bool, error) {
	return false, nil
}
