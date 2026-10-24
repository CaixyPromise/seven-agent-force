package application

import (
	"context"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain/provider"
)

func TestCompletionHandlerPersistsOtpAndRegeneratesRecoveryCodes(t *testing.T) {
	store := &fakeCompletionStore{}
	handler := NewCompletionHandler(store, 8)
	session := &domain.ChallengeSession{
		SubjectIdentifier: "login:alice",
		BusinessAction:    string(domain.BusinessActionMFAOTPBind),
		SessionContext: map[string]any{
			"otp.pendingSecretPlain": "JBSWY3DPEHPK3PXP",
		},
	}

	if err := handler.OnPassed(context.Background(), session); err != nil {
		t.Fatalf("completion handler on passed: %v", err)
	}
	if store.plainSecret != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("unexpected secret persisted: %s", store.plainSecret)
	}
	if store.batchSize != 8 {
		t.Fatalf("unexpected recovery batch size: %d", store.batchSize)
	}
}

func TestCompletionHandlerIgnoresOtherBusinessActions(t *testing.T) {
	store := &fakeCompletionStore{}
	handler := NewCompletionHandler(store, 8)
	session := &domain.ChallengeSession{
		SubjectIdentifier: "login:alice",
		BusinessAction:    "PASSWORD_LOGIN",
		SessionContext: map[string]any{
			"otp.pendingSecretPlain": "JBSWY3DPEHPK3PXP",
		},
	}

	if err := handler.OnPassed(context.Background(), session); err != nil {
		t.Fatalf("completion handler on passed: %v", err)
	}
	if store.plainSecret != "" {
		t.Fatalf("expected no otp binding side effect, got %s", store.plainSecret)
	}
	if store.batchSize != 0 {
		t.Fatalf("expected no recovery regeneration, got %d", store.batchSize)
	}
}

func TestCompletionHandlerRejectsMissingRecoveryBatchSize(t *testing.T) {
	store := &fakeCompletionStore{}
	handler := NewCompletionHandler(store, 0)
	session := &domain.ChallengeSession{
		SubjectIdentifier: "login:alice",
		BusinessAction:    string(domain.BusinessActionMFAOTPSwitch),
		SessionContext: map[string]any{
			"otp.pendingSecretPlain": "JBSWY3DPEHPK3PXP",
		},
	}

	if err := handler.OnPassed(context.Background(), session); err == nil {
		t.Fatal("expected missing recovery batch size to fail")
	}
}

func TestCompletionHandlerRejectsMissingOtpSecret(t *testing.T) {
	store := &fakeCompletionStore{}
	handler := NewCompletionHandler(store, 8)
	session := &domain.ChallengeSession{
		SubjectIdentifier: "login:alice",
		BusinessAction:    string(domain.BusinessActionMFAOTPSwitch),
		SessionContext:    map[string]any{},
	}

	if err := handler.OnPassed(context.Background(), session); err == nil {
		t.Fatal("expected missing otp secret to fail")
	}
}

func TestCompletionHandlerCompletesPasskeyBinding(t *testing.T) {
	store := &fakeCompletionStore{}
	handler := NewCompletionHandler(store, 8)
	session := &domain.ChallengeSession{
		SubjectIdentifier: "login:alice",
		BusinessAction:    string(domain.BusinessActionMFAPasskeySwitch),
		SessionContext: map[string]any{
			"passkey.registration": map[string]any{
				"credentialIdentifier": "cred-1",
				"publicKeyCose":        "pk-cose",
				"signCount":            7,
				"userHandle":           "dXNlcjoxMDAx",
				"displayName":          "Alice Key",
			},
		},
	}

	if err := handler.OnPassed(context.Background(), session); err != nil {
		t.Fatalf("completion handler on passed: %v", err)
	}
	if store.passkey.CredentialIdentifier != "cred-1" || !store.disableExisting {
		t.Fatalf("unexpected passkey completion: %#v disable=%v", store.passkey, store.disableExisting)
	}
	if store.passkey.UserHandle != "dXNlcjoxMDAx" {
		t.Fatalf("expected passkey userHandle to be propagated, got %q", store.passkey.UserHandle)
	}
	if store.batchSize != 8 {
		t.Fatalf("unexpected recovery batch size: %d", store.batchSize)
	}
}

func TestCompletionHandlerRejectsPasskeyBindingWhenRequiredFieldMissing(t *testing.T) {
	store := &fakeCompletionStore{}
	handler := NewCompletionHandler(store, 8)
	session := &domain.ChallengeSession{
		SubjectIdentifier: "login:alice",
		BusinessAction:    string(domain.BusinessActionMFAPasskeyBind),
		SessionContext: map[string]any{
			"passkey.registration": map[string]any{
				"credentialIdentifier": "cred-1",
				"publicKeyCose":        nil,
			},
		},
	}

	if err := handler.OnPassed(context.Background(), session); err == nil {
		t.Fatal("expected missing passkey required field to fail")
	}
	if store.passkey.CredentialIdentifier != "" {
		t.Fatalf("expected no passkey completion side effect, got %#v", store.passkey)
	}
}

func TestCompletionHandlerRejectsPasskeyBindingWhenRequiredFieldIsNonString(t *testing.T) {
	store := &fakeCompletionStore{}
	handler := NewCompletionHandler(store, 8)
	session := &domain.ChallengeSession{
		SubjectIdentifier: "login:alice",
		BusinessAction:    string(domain.BusinessActionMFAPasskeyBind),
		SessionContext: map[string]any{
			"passkey.registration": map[string]any{
				"credentialIdentifier": 12345,
				"publicKeyCose":        "pk-cose",
			},
		},
	}

	if err := handler.OnPassed(context.Background(), session); err == nil {
		t.Fatal("expected non-string passkey required field to fail")
	}
	if store.passkey.CredentialIdentifier != "" {
		t.Fatalf("expected no passkey completion side effect, got %#v", store.passkey)
	}
}

type fakeCompletionStore struct {
	plainSecret     string
	batchSize       int
	totpCalls       int
	passkey         domain.PasskeyRegistration
	disableExisting bool
}

func (f *fakeCompletionStore) FindEnabledOtpBinding(ctx context.Context, session *domain.ChallengeSession) (*domain.OtpBindingRecord, error) {
	return nil, nil
}
func (f *fakeCompletionStore) FindEnabledOtpSecret(ctx context.Context, session *domain.ChallengeSession) (string, error) {
	return "", nil
}
func (f *fakeCompletionStore) FindPasswordCredential(ctx context.Context, session *domain.ChallengeSession) (string, error) {
	return "", nil
}
func (f *fakeCompletionStore) ListPasskeys(ctx context.Context, session *domain.ChallengeSession) ([]domain.PasskeyRegistration, error) {
	return nil, nil
}
func (f *fakeCompletionStore) FindPasskey(ctx context.Context, credentialKey string) (*domain.PasskeyRegistration, error) {
	return nil, nil
}
func (f *fakeCompletionStore) UpdatePasskeyUsage(ctx context.Context, credentialKey string, signCount int64, usedAt time.Time) error {
	return nil
}
func (f *fakeCompletionStore) ConsumeRecoveryCode(ctx context.Context, session *domain.ChallengeSession, recoveryCode string, usedAt time.Time) (bool, error) {
	return false, nil
}
func (f *fakeCompletionStore) CompleteTotpBinding(ctx context.Context, session *domain.ChallengeSession, plainSecret string, verifiedAt time.Time, recoveryBatchSize int) error {
	f.plainSecret = plainSecret
	f.totpCalls++
	f.batchSize = recoveryBatchSize
	return nil
}
func (f *fakeCompletionStore) CompletePasskeyBinding(ctx context.Context, session *domain.ChallengeSession, registration domain.PasskeyRegistration, disableExisting bool, verifiedAt time.Time, recoveryBatchSize int) error {
	f.passkey = registration
	f.disableExisting = disableExisting
	f.batchSize = recoveryBatchSize
	return nil
}
func (f *fakeCompletionStore) ResolveAccountName(ctx context.Context, session *domain.ChallengeSession) (string, error) {
	return "alice", nil
}
func (f *fakeCompletionStore) ResolveTargetEmail(ctx context.Context, session *domain.ChallengeSession) (string, error) {
	return "", nil
}

var _ provider.SubjectCredentialStore = (*fakeCompletionStore)(nil)
