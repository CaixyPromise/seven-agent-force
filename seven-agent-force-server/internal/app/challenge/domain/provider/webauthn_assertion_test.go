package provider

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	challengeinfra "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/random"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestWebAuthnAssertionVerifiesSignedAuthenticatorData(t *testing.T) {
	fixture := newWebAuthnAssertionFixture(t)

	verified, err := fixture.provider.Verify(context.Background(), fixture.session, fixture.step, fixture.payload(t, fixture.credentialID, "example.com", 0x05, 2, fixture.privateKey))
	if err != nil {
		t.Fatalf("verify valid assertion: %v", err)
	}
	if !verified {
		t.Fatal("expected valid signed assertion to pass")
	}
	if fixture.store.lastUsageCredential != fixture.credentialID || fixture.store.lastUsageSignCount != 2 {
		t.Fatalf("expected sign count update, got credential=%s signCount=%d", fixture.store.lastUsageCredential, fixture.store.lastUsageSignCount)
	}
}

func TestWebAuthnAssertionEligibleRequiresAllowedOriginsAndCredentials(t *testing.T) {
	privateKey := mustECDSAKey(t)
	store := &fakeWebAuthnSubjectStore{
		allowedCredentialIDs: []string{"credential-1"},
		passkeys: map[string]domain.PasskeyRegistration{
			"credential-1": {
				CredentialIdentifier: "credential-1",
				PublicKeyCose:        encodeCOSEEC2PublicKey(t, privateKey),
			},
		},
	}
	provider := NewWebAuthnPasskeyAssertionStepProvider(
		challengeinfra.NewWebAuthnService(random.New(config.RandomConfig{NonceLength: 16})),
		store,
		"example.com",
		[]string{"https://example.com"},
		300,
	)

	eligible, err := provider.Eligible(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "user:1001"}, &domain.ChallengeStep{})
	if err != nil {
		t.Fatalf("check passkey assertion eligibility: %v", err)
	}
	if !eligible {
		t.Fatal("expected assertion to be eligible when an allowed credential exists")
	}

	provider.origins = nil
	eligible, err = provider.Eligible(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "user:1001"}, &domain.ChallengeStep{})
	if err != nil {
		t.Fatalf("check missing origin eligibility: %v", err)
	}
	if eligible {
		t.Fatal("expected assertion to be ineligible without allowed origins")
	}

	provider.origins = []string{"https://example.com"}
	store.allowedCredentialIDs = nil
	eligible, err = provider.Eligible(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "user:1001"}, &domain.ChallengeStep{})
	if err != nil {
		t.Fatalf("check missing credential eligibility: %v", err)
	}
	if eligible {
		t.Fatal("expected assertion to be ineligible without allowed credentials")
	}

	expected := errors.New("passkey store unavailable")
	store.listErr = expected
	eligible, err = provider.Eligible(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "user:1001"}, &domain.ChallengeStep{})
	if !errors.Is(err, expected) {
		t.Fatalf("expected list passkey error, got %v", err)
	}
	if eligible {
		t.Fatal("expected errored passkey lookup to be ineligible")
	}
}

func TestWebAuthnAssertionPrepareStoresChallengeAndOnlyPublicHints(t *testing.T) {
	fixture := newWebAuthnAssertionFixture(t)
	challenge := valueString(fixture.session.SessionContext[assertionChallengeKey(fixture.step)])
	if challenge == "" {
		t.Fatal("expected assertion prepare to store server challenge")
	}
	if fixture.step.UserInterfaceHints["challenge"] != challenge {
		t.Fatalf("expected challenge hint to match stored challenge, got %#v", fixture.step.UserInterfaceHints["challenge"])
	}
	allowIDs, ok := fixture.step.UserInterfaceHints["allowCredentialIds"].([]string)
	if !ok {
		t.Fatalf("expected allowCredentialIds hint, got %#v", fixture.step.UserInterfaceHints["allowCredentialIds"])
	}
	if len(allowIDs) != 1 || allowIDs[0] != fixture.credentialID {
		t.Fatalf("unexpected allowed credential ids: %#v", allowIDs)
	}
	passkey := fixture.store.passkeys[fixture.credentialID]
	for _, secret := range []string{passkey.PublicKeyCose, passkey.UserHandle, fixture.privateKey.D.String()} {
		if mapContainsString(fixture.step.UserInterfaceHints, secret) {
			t.Fatalf("assertion hints must not expose private passkey material %q: %#v", secret, fixture.step.UserInterfaceHints)
		}
	}
}

func TestWebAuthnAssertionAcceptsFullUint32SignCount(t *testing.T) {
	fixture := newWebAuthnAssertionFixture(t)
	signCount := uint32(1 << 31)

	verified, err := fixture.provider.Verify(context.Background(), fixture.session, fixture.step, fixture.payload(t, fixture.credentialID, "example.com", 0x05, signCount, fixture.privateKey))
	if err != nil {
		t.Fatalf("verify valid assertion: %v", err)
	}
	if !verified {
		t.Fatal("expected valid assertion with high uint32 sign count to pass")
	}
	if fixture.store.lastUsageSignCount != int64(signCount) {
		t.Fatalf("expected full uint32 sign count to be stored, got %d", fixture.store.lastUsageSignCount)
	}
}

func TestWebAuthnAssertionRejectsMismatchedUserHandle(t *testing.T) {
	fixture := newWebAuthnAssertionFixture(t)
	payload := fixture.payload(t, fixture.credentialID, "example.com", 0x05, 2, fixture.privateKey)
	payload["userHandle"] = encodedWebAuthnUserHandle("user:9999")

	verified, err := fixture.provider.Verify(context.Background(), fixture.session, fixture.step, payload)
	if err != nil {
		t.Fatalf("verify assertion: %v", err)
	}
	if verified {
		t.Fatal("expected assertion with mismatched userHandle to be rejected")
	}
	if fixture.store.lastUsageCredential != "" {
		t.Fatalf("mismatched userHandle must not update passkey usage: %+v", fixture.store)
	}
}

func TestWebAuthnAssertionAllowsLegacyPasskeyWithoutStoredUserHandle(t *testing.T) {
	fixture := newWebAuthnAssertionFixture(t)
	passkey := fixture.store.passkeys[fixture.credentialID]
	passkey.UserHandle = ""
	fixture.store.passkeys[fixture.credentialID] = passkey
	payload := fixture.payload(t, fixture.credentialID, "example.com", 0x05, 2, fixture.privateKey)
	delete(payload, "userHandle")

	verified, err := fixture.provider.Verify(context.Background(), fixture.session, fixture.step, payload)
	if err != nil {
		t.Fatalf("verify assertion: %v", err)
	}
	if !verified {
		t.Fatal("expected legacy assertion without stored userHandle to pass")
	}
}

func TestWebAuthnAssertionRejectsWhenAllowedOriginsAreMissing(t *testing.T) {
	fixture := newWebAuthnAssertionFixture(t)
	fixture.provider.origins = nil

	verified, err := fixture.provider.Verify(context.Background(), fixture.session, fixture.step, fixture.payload(t, fixture.credentialID, "example.com", 0x05, 2, fixture.privateKey))
	if err != nil {
		t.Fatalf("verify assertion: %v", err)
	}
	if verified {
		t.Fatal("expected assertion to be rejected when allowed origins are missing")
	}
}

func TestWebAuthnAssertionRejectsForgedOrInvalidAuthenticatorData(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, fixture *webAuthnAssertionFixture) map[string]any
	}{
		{
			name: "forged signature",
			mutate: func(t *testing.T, fixture *webAuthnAssertionFixture) map[string]any {
				payload := fixture.payload(t, fixture.credentialID, "example.com", 0x05, 2, fixture.privateKey)
				payload["signature"] = base64.RawURLEncoding.EncodeToString([]byte("forged-signature"))
				return payload
			},
		},
		{
			name: "wrong origin",
			mutate: func(t *testing.T, fixture *webAuthnAssertionFixture) map[string]any {
				return fixture.payloadWithOrigin(t, fixture.credentialID, "example.com", "https://evil.example.com", 0x05, 2, fixture.privateKey)
			},
		},
		{
			name: "wrong rp id hash",
			mutate: func(t *testing.T, fixture *webAuthnAssertionFixture) map[string]any {
				return fixture.payload(t, fixture.credentialID, "evil.example.com", 0x05, 2, fixture.privateKey)
			},
		},
		{
			name: "missing user presence flag",
			mutate: func(t *testing.T, fixture *webAuthnAssertionFixture) map[string]any {
				return fixture.payload(t, fixture.credentialID, "example.com", 0x04, 2, fixture.privateKey)
			},
		},
		{
			name: "missing user verification flag",
			mutate: func(t *testing.T, fixture *webAuthnAssertionFixture) map[string]any {
				return fixture.payload(t, fixture.credentialID, "example.com", 0x01, 2, fixture.privateKey)
			},
		},
		{
			name: "sign counter regression",
			mutate: func(t *testing.T, fixture *webAuthnAssertionFixture) map[string]any {
				return fixture.payload(t, fixture.credentialID, "example.com", 0x05, 1, fixture.privateKey)
			},
		},
		{
			name: "missing user handle for bound credential",
			mutate: func(t *testing.T, fixture *webAuthnAssertionFixture) map[string]any {
				payload := fixture.payload(t, fixture.credentialID, "example.com", 0x05, 2, fixture.privateKey)
				delete(payload, "userHandle")
				return payload
			},
		},
		{
			name: "non canonical user handle for bound credential",
			mutate: func(t *testing.T, fixture *webAuthnAssertionFixture) map[string]any {
				payload := fixture.payload(t, fixture.credentialID, "example.com", 0x05, 2, fixture.privateKey)
				payload["userHandle"] = encodedWebAuthnUserHandle("user:1001") + "="
				return payload
			},
		},
		{
			name: "credential not allowed for current subject",
			mutate: func(t *testing.T, fixture *webAuthnAssertionFixture) map[string]any {
				attackerKey := mustECDSAKey(t)
				fixture.store.passkeys["attacker-credential"] = domain.PasskeyRegistration{
					CredentialIdentifier: "attacker-credential",
					PublicKeyCose:        encodeCOSEEC2PublicKey(t, attackerKey),
					SignCount:            1,
				}
				return fixture.payload(t, "attacker-credential", "example.com", 0x05, 2, attackerKey)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newWebAuthnAssertionFixture(t)
			verified, err := fixture.provider.Verify(context.Background(), fixture.session, fixture.step, tt.mutate(t, fixture))
			if err != nil {
				t.Fatalf("verify assertion: %v", err)
			}
			if verified {
				t.Fatalf("expected assertion to be rejected")
			}
			if fixture.store.lastUsageCredential != "" {
				t.Fatalf("invalid assertion must not update passkey usage: %+v", fixture.store)
			}
		})
	}
}

type webAuthnAssertionFixture struct {
	provider     *WebAuthnPasskeyAssertionStepProvider
	store        *fakeWebAuthnSubjectStore
	session      *domain.ChallengeSession
	step         *domain.ChallengeStep
	privateKey   *ecdsa.PrivateKey
	credentialID string
}

func newWebAuthnAssertionFixture(t *testing.T) *webAuthnAssertionFixture {
	t.Helper()
	privateKey := mustECDSAKey(t)
	credentialID := "credential-1"
	store := &fakeWebAuthnSubjectStore{
		allowedCredentialIDs: []string{credentialID},
		passkeys: map[string]domain.PasskeyRegistration{
			credentialID: {
				CredentialIdentifier: credentialID,
				PublicKeyCose:        encodeCOSEEC2PublicKey(t, privateKey),
				SignCount:            1,
				UserHandle:           encodedWebAuthnUserHandle("user:1001"),
			},
		},
	}
	provider := NewWebAuthnPasskeyAssertionStepProvider(
		challengeinfra.NewWebAuthnService(random.New(config.RandomConfig{NonceLength: 16})),
		store,
		"example.com",
		[]string{"https://example.com"},
		300,
	)
	now := time.Now().UTC()
	session := &domain.ChallengeSession{
		ChallengeIdentifier: "challenge-1",
		SubjectIdentifier:   "user:1001",
		BusinessAction:      string(domain.BusinessActionMFAPasskeyDelete),
		ChallengeState:      domain.ChallengeStatePending,
		SessionContext:      map[string]any{},
		CreatedAt:           &now,
		ExpiresAt:           timePointer(now.Add(time.Minute)),
	}
	step := &domain.ChallengeStep{
		StepIdentifier: "step-passkey",
		ChallengeType:  domain.ChallengeTypeWebAuthnPasskeyAssertion,
		StepState:      domain.ChallengeStepStateInProgress,
	}
	if err := provider.Prepare(context.Background(), session, step); err != nil {
		t.Fatalf("prepare assertion: %v", err)
	}
	return &webAuthnAssertionFixture{
		provider:     provider,
		store:        store,
		session:      session,
		step:         step,
		privateKey:   privateKey,
		credentialID: credentialID,
	}
}

func (f *webAuthnAssertionFixture) payload(t *testing.T, credentialID, rpID string, flags byte, signCount uint32, privateKey *ecdsa.PrivateKey) map[string]any {
	t.Helper()
	return f.payloadWithOrigin(t, credentialID, rpID, "https://example.com", flags, signCount, privateKey)
}

func (f *webAuthnAssertionFixture) payloadWithOrigin(t *testing.T, credentialID, rpID, origin string, flags byte, signCount uint32, privateKey *ecdsa.PrivateKey) map[string]any {
	t.Helper()
	challenge := valueString(f.session.SessionContext[assertionChallengeKey(f.step)])
	clientDataJSON := mustJSON(t, map[string]any{
		"type":      "webauthn.get",
		"challenge": challenge,
		"origin":    origin,
	})
	authenticatorData := authenticatorData(rpID, flags, signCount)
	clientHash := sha256.Sum256(clientDataJSON)
	signed := append(append([]byte(nil), authenticatorData...), clientHash[:]...)
	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, sha256Bytes(signed))
	if err != nil {
		t.Fatalf("sign assertion: %v", err)
	}
	return map[string]any{
		"credentialIdentifier": credentialID,
		"clientDataJSON":       base64.RawURLEncoding.EncodeToString(clientDataJSON),
		"authenticatorData":    base64.RawURLEncoding.EncodeToString(authenticatorData),
		"signature":            base64.RawURLEncoding.EncodeToString(signature),
		"userHandle":           encodedWebAuthnUserHandle("user:1001"),
	}
}

type fakeWebAuthnSubjectStore struct {
	allowedCredentialIDs []string
	passkeys             map[string]domain.PasskeyRegistration
	lastUsageCredential  string
	lastUsageSignCount   int64
	listErr              error
}

func (f *fakeWebAuthnSubjectStore) FindEnabledOtpBinding(context.Context, *domain.ChallengeSession) (*domain.OtpBindingRecord, error) {
	return nil, nil
}

func (f *fakeWebAuthnSubjectStore) FindEnabledOtpSecret(context.Context, *domain.ChallengeSession) (string, error) {
	return "", nil
}

func (f *fakeWebAuthnSubjectStore) FindPasswordCredential(context.Context, *domain.ChallengeSession) (string, error) {
	return "", nil
}

func (f *fakeWebAuthnSubjectStore) ListPasskeys(context.Context, *domain.ChallengeSession) ([]domain.PasskeyRegistration, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	result := make([]domain.PasskeyRegistration, 0, len(f.allowedCredentialIDs))
	for _, credentialID := range f.allowedCredentialIDs {
		if item, ok := f.passkeys[credentialID]; ok {
			result = append(result, item)
		}
	}
	return result, nil
}

func (f *fakeWebAuthnSubjectStore) FindPasskey(_ context.Context, credentialKey string) (*domain.PasskeyRegistration, error) {
	item, ok := f.passkeys[credentialKey]
	if !ok {
		return nil, nil
	}
	copy := item
	return &copy, nil
}

func (f *fakeWebAuthnSubjectStore) UpdatePasskeyUsage(_ context.Context, credentialKey string, signCount int64, _ time.Time) error {
	f.lastUsageCredential = credentialKey
	f.lastUsageSignCount = signCount
	return nil
}

func (f *fakeWebAuthnSubjectStore) ConsumeRecoveryCode(context.Context, *domain.ChallengeSession, string, time.Time) (bool, error) {
	return false, nil
}

func (f *fakeWebAuthnSubjectStore) CompleteTotpBinding(context.Context, *domain.ChallengeSession, string, time.Time, int) error {
	return nil
}

func (f *fakeWebAuthnSubjectStore) CompletePasskeyBinding(context.Context, *domain.ChallengeSession, domain.PasskeyRegistration, bool, time.Time, int) error {
	return nil
}

func (f *fakeWebAuthnSubjectStore) ResolveAccountName(context.Context, *domain.ChallengeSession) (string, error) {
	return "", nil
}

func (f *fakeWebAuthnSubjectStore) ResolveTargetEmail(context.Context, *domain.ChallengeSession) (string, error) {
	return "", nil
}

func mustECDSAKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ecdsa key: %v", err)
	}
	return privateKey
}

func encodeCOSEEC2PublicKey(t *testing.T, privateKey *ecdsa.PrivateKey) string {
	t.Helper()
	x := privateKey.PublicKey.X.Bytes()
	y := privateKey.PublicKey.Y.Bytes()
	if len(x) > 32 || len(y) > 32 {
		t.Fatalf("unexpected P-256 coordinate length x=%d y=%d", len(x), len(y))
	}
	paddedX := append(make([]byte, 32-len(x)), x...)
	paddedY := append(make([]byte, 32-len(y)), y...)
	cose := []byte{
		0xa5,
		0x01, 0x02,
		0x03, 0x26,
		0x20, 0x01,
		0x21, 0x58, 0x20,
	}
	cose = append(cose, paddedX...)
	cose = append(cose, 0x22, 0x58, 0x20)
	cose = append(cose, paddedY...)
	return base64.RawURLEncoding.EncodeToString(cose)
}

func authenticatorData(rpID string, flags byte, signCount uint32) []byte {
	rpHash := sha256.Sum256([]byte(rpID))
	result := make([]byte, 37)
	copy(result[:32], rpHash[:])
	result[32] = flags
	binary.BigEndian.PutUint32(result[33:37], signCount)
	return result
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return payload
}

func encodeJSONForWebAuthn(t *testing.T, value any) string {
	t.Helper()
	return base64.RawURLEncoding.EncodeToString(mustJSON(t, value))
}

func encodedWebAuthnUserHandle(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func sha256Bytes(value []byte) []byte {
	sum := sha256.Sum256(value)
	return sum[:]
}

func timePointer(value time.Time) *time.Time {
	return &value
}
