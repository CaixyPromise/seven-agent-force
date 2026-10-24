package provider

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	challengeinfra "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/random"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestWebAuthnRegistrationRejectsMalformedPublicKeyCose(t *testing.T) {
	store := &fakeWebAuthnSubjectStore{passkeys: map[string]domain.PasskeyRegistration{}}
	provider := NewWebAuthnPasskeyRegistrationStepProvider(
		challengeinfra.NewWebAuthnService(random.New(config.RandomConfig{NonceLength: 16})),
		store,
		"example.com",
		"Example",
		300,
		[]string{"https://example.com"},
	)
	now := time.Now().UTC()
	session := &domain.ChallengeSession{
		ChallengeIdentifier: "challenge-registration",
		SubjectIdentifier:   "user:1001",
		BusinessAction:      string(domain.BusinessActionMFAPasskeyBind),
		ChallengeState:      domain.ChallengeStatePending,
		SessionContext:      map[string]any{},
		CreatedAt:           &now,
		ExpiresAt:           timePointer(now.Add(time.Minute)),
	}
	step := &domain.ChallengeStep{
		StepIdentifier: "step-registration",
		ChallengeType:  domain.ChallengeTypeWebAuthnPasskeyRegistration,
		StepState:      domain.ChallengeStepStateInProgress,
	}
	if err := provider.Prepare(context.Background(), session, step); err != nil {
		t.Fatalf("prepare registration: %v", err)
	}
	clientDataJSON := mustRegistrationClientData(t, valueString(session.SessionContext[registrationChallengeKey(step)]))
	credentialIdentifier := base64.RawURLEncoding.EncodeToString([]byte("credential-1"))
	malformedPublicKeyCose := base64.RawURLEncoding.EncodeToString([]byte("not-a-cose-key"))

	verified, err := provider.Verify(context.Background(), session, step, map[string]any{
		"credentialIdentifier": credentialIdentifier,
		"clientDataJSON":       clientDataJSON,
		"attestationObject":    registrationAttestationObject(t, "none", "credential-1", malformedPublicKeyCose, 0),
		"userHandle":           valueString(session.SessionContext[registrationUserHandleKey(step)]),
	})
	if err != nil {
		t.Fatalf("verify registration: %v", err)
	}
	if verified {
		t.Fatal("expected malformed publicKeyCose registration to be rejected")
	}
	if _, ok := session.SessionContext["passkey.registration"]; ok {
		t.Fatalf("invalid registration must not write passkey.registration: %+v", session.SessionContext["passkey.registration"])
	}
}

func TestWebAuthnRegistrationStoresUserHandle(t *testing.T) {
	store := &fakeWebAuthnSubjectStore{passkeys: map[string]domain.PasskeyRegistration{}}
	provider := NewWebAuthnPasskeyRegistrationStepProvider(
		challengeinfra.NewWebAuthnService(random.New(config.RandomConfig{NonceLength: 16})),
		store,
		"example.com",
		"Example",
		300,
		[]string{"https://example.com"},
	)
	now := time.Now().UTC()
	session := &domain.ChallengeSession{
		ChallengeIdentifier: "challenge-registration",
		SubjectIdentifier:   "user:1001",
		BusinessAction:      string(domain.BusinessActionMFAPasskeyBind),
		ChallengeState:      domain.ChallengeStatePending,
		SessionContext:      map[string]any{},
		CreatedAt:           &now,
		ExpiresAt:           timePointer(now.Add(time.Minute)),
	}
	step := &domain.ChallengeStep{
		StepIdentifier: "step-registration",
		ChallengeType:  domain.ChallengeTypeWebAuthnPasskeyRegistration,
		StepState:      domain.ChallengeStepStateInProgress,
	}
	if err := provider.Prepare(context.Background(), session, step); err != nil {
		t.Fatalf("prepare registration: %v", err)
	}
	userHandle := valueString(session.SessionContext[registrationUserHandleKey(step)])
	if userHandle == "" {
		t.Fatal("expected registration prepare to store server userHandle")
	}
	if step.UserInterfaceHints["userHandle"] != userHandle {
		t.Fatalf("expected userHandle hint to match server value, got %#v", step.UserInterfaceHints["userHandle"])
	}

	credentialIdentifier := base64.RawURLEncoding.EncodeToString([]byte("credential-1"))
	publicKeyCose := encodeCOSEEC2PublicKey(t, mustECDSAKey(t))

	verified, err := provider.Verify(context.Background(), session, step, map[string]any{
		"credentialIdentifier": credentialIdentifier,
		"clientDataJSON":       mustRegistrationClientData(t, valueString(session.SessionContext[registrationChallengeKey(step)])),
		"attestationObject":    registrationAttestationObject(t, "none", "credential-1", publicKeyCose, 5),
		"userHandle":           userHandle,
	})
	if err != nil {
		t.Fatalf("verify registration: %v", err)
	}
	if !verified {
		t.Fatal("expected valid registration to pass")
	}
	registration, ok := session.SessionContext["passkey.registration"].(map[string]any)
	if !ok {
		t.Fatalf("expected passkey.registration to be stored: %+v", session.SessionContext["passkey.registration"])
	}
	if registration["userHandle"] != userHandle {
		t.Fatalf("expected userHandle to be stored, got %#v", registration["userHandle"])
	}
	if registration["credentialIdentifier"] != credentialIdentifier {
		t.Fatalf("expected attestation credential id to be stored, got %#v", registration["credentialIdentifier"])
	}
	if registration["publicKeyCose"] != publicKeyCose {
		t.Fatalf("expected attestation public key to be stored, got %#v", registration["publicKeyCose"])
	}
	if registration["signCount"] != int64(5) {
		t.Fatalf("expected attestation sign count to be stored, got %#v", registration["signCount"])
	}
	if registration["attestationFormat"] != "none" {
		t.Fatalf("expected attestation format to be stored, got %#v", registration["attestationFormat"])
	}
}

func TestWebAuthnRegistrationRefreshReplacesChallengeAndRejectsStaleClientData(t *testing.T) {
	store := &fakeWebAuthnSubjectStore{passkeys: map[string]domain.PasskeyRegistration{}}
	provider := NewWebAuthnPasskeyRegistrationStepProvider(
		challengeinfra.NewWebAuthnService(random.New(config.RandomConfig{NonceLength: 16})),
		store,
		"example.com",
		"Example",
		300,
		[]string{"https://example.com"},
	)
	now := time.Now().UTC()
	session := &domain.ChallengeSession{
		ChallengeIdentifier: "challenge-registration",
		SubjectIdentifier:   "user:1001",
		BusinessAction:      string(domain.BusinessActionMFAPasskeyBind),
		ChallengeState:      domain.ChallengeStatePending,
		SessionContext:      map[string]any{},
		CreatedAt:           &now,
		ExpiresAt:           timePointer(now.Add(time.Minute)),
	}
	step := &domain.ChallengeStep{
		StepIdentifier: "step-registration",
		ChallengeType:  domain.ChallengeTypeWebAuthnPasskeyRegistration,
		StepState:      domain.ChallengeStepStateInProgress,
	}
	if err := provider.Prepare(context.Background(), session, step); err != nil {
		t.Fatalf("prepare registration: %v", err)
	}
	staleChallenge := valueString(session.SessionContext[registrationChallengeKey(step)])
	if err := provider.Refresh(context.Background(), session, step); err != nil {
		t.Fatalf("refresh registration: %v", err)
	}
	freshChallenge := valueString(session.SessionContext[registrationChallengeKey(step)])
	if staleChallenge == "" || freshChallenge == "" {
		t.Fatalf("expected both stale and fresh challenges, got stale=%q fresh=%q", staleChallenge, freshChallenge)
	}
	if staleChallenge == freshChallenge {
		t.Fatal("expected refresh to replace registration challenge")
	}

	credentialIdentifier := base64.RawURLEncoding.EncodeToString([]byte("credential-1"))
	publicKeyCose := encodeCOSEEC2PublicKey(t, mustECDSAKey(t))
	stalePayload := map[string]any{
		"credentialIdentifier": credentialIdentifier,
		"clientDataJSON":       mustRegistrationClientData(t, staleChallenge),
		"attestationObject":    registrationAttestationObject(t, "none", "credential-1", publicKeyCose, 5),
		"userHandle":           valueString(session.SessionContext[registrationUserHandleKey(step)]),
	}
	verified, err := provider.Verify(context.Background(), session, step, stalePayload)
	if err != nil {
		t.Fatalf("verify stale registration: %v", err)
	}
	if verified {
		t.Fatal("expected stale registration challenge to be rejected after refresh")
	}
	if _, ok := session.SessionContext["passkey.registration"]; ok {
		t.Fatalf("stale registration must not write passkey.registration: %+v", session.SessionContext["passkey.registration"])
	}

	freshPayload := map[string]any{
		"credentialIdentifier": credentialIdentifier,
		"clientDataJSON":       mustRegistrationClientData(t, freshChallenge),
		"attestationObject":    registrationAttestationObject(t, "none", "credential-1", publicKeyCose, 5),
		"userHandle":           valueString(session.SessionContext[registrationUserHandleKey(step)]),
	}
	verified, err = provider.Verify(context.Background(), session, step, freshPayload)
	if err != nil {
		t.Fatalf("verify fresh registration: %v", err)
	}
	if !verified {
		t.Fatal("expected fresh registration challenge to pass")
	}
}

func TestWebAuthnRegistrationPrepareDoesNotExposeExistingCredentialSecrets(t *testing.T) {
	privateKey := mustECDSAKey(t)
	existing := domain.PasskeyRegistration{
		CredentialIdentifier: "credential-existing",
		PublicKeyCose:        encodeCOSEEC2PublicKey(t, privateKey),
		UserHandle:           encodedWebAuthnUserHandle("user:existing"),
		SignCount:            12,
	}
	store := &fakeWebAuthnSubjectStore{
		allowedCredentialIDs: []string{existing.CredentialIdentifier},
		passkeys:             map[string]domain.PasskeyRegistration{existing.CredentialIdentifier: existing},
	}
	provider := NewWebAuthnPasskeyRegistrationStepProvider(
		challengeinfra.NewWebAuthnService(random.New(config.RandomConfig{NonceLength: 16})),
		store,
		"example.com",
		"Example",
		300,
		[]string{"https://example.com"},
	)
	session := &domain.ChallengeSession{
		SubjectIdentifier: "user:1001",
		SessionContext:    map[string]any{},
	}
	step := &domain.ChallengeStep{
		StepIdentifier: "step-registration",
		ChallengeType:  domain.ChallengeTypeWebAuthnPasskeyRegistration,
	}
	if err := provider.Prepare(context.Background(), session, step); err != nil {
		t.Fatalf("prepare registration: %v", err)
	}
	excludes, ok := step.UserInterfaceHints["excludeCredentialIds"].([]string)
	if !ok {
		t.Fatalf("expected excludeCredentialIds hint, got %#v", step.UserInterfaceHints["excludeCredentialIds"])
	}
	if len(excludes) != 1 || excludes[0] != existing.CredentialIdentifier {
		t.Fatalf("unexpected excluded credential ids: %#v", excludes)
	}
	for _, secret := range []string{existing.PublicKeyCose, existing.UserHandle, privateKey.D.String()} {
		if mapContainsString(step.UserInterfaceHints, secret) {
			t.Fatalf("registration hints must not expose existing credential material %q: %#v", secret, step.UserInterfaceHints)
		}
		if mapContainsString(session.SessionContext, secret) {
			t.Fatalf("registration session must not expose existing credential material %q: %#v", secret, session.SessionContext)
		}
	}
}

func TestWebAuthnRegistrationRejectsExistingCredentialReplay(t *testing.T) {
	credentialIdentifier := base64.RawURLEncoding.EncodeToString([]byte("credential-1"))
	store := &fakeWebAuthnSubjectStore{
		passkeys: map[string]domain.PasskeyRegistration{
			credentialIdentifier: {
				CredentialIdentifier: credentialIdentifier,
				PublicKeyCose:        encodeCOSEEC2PublicKey(t, mustECDSAKey(t)),
			},
		},
	}
	provider := NewWebAuthnPasskeyRegistrationStepProvider(
		challengeinfra.NewWebAuthnService(random.New(config.RandomConfig{NonceLength: 16})),
		store,
		"example.com",
		"Example",
		300,
		[]string{"https://example.com"},
	)
	session := &domain.ChallengeSession{
		SubjectIdentifier: "user:1001",
		SessionContext:    map[string]any{},
	}
	step := &domain.ChallengeStep{
		StepIdentifier: "step-registration",
		ChallengeType:  domain.ChallengeTypeWebAuthnPasskeyRegistration,
	}
	if err := provider.Prepare(context.Background(), session, step); err != nil {
		t.Fatalf("prepare registration: %v", err)
	}
	publicKeyCose := encodeCOSEEC2PublicKey(t, mustECDSAKey(t))
	verified, err := provider.Verify(context.Background(), session, step, map[string]any{
		"credentialIdentifier": credentialIdentifier,
		"clientDataJSON":       mustRegistrationClientData(t, valueString(session.SessionContext[registrationChallengeKey(step)])),
		"attestationObject":    registrationAttestationObject(t, "none", "credential-1", publicKeyCose, 5),
		"userHandle":           valueString(session.SessionContext[registrationUserHandleKey(step)]),
	})
	if err != nil {
		t.Fatalf("verify duplicate registration: %v", err)
	}
	if verified {
		t.Fatal("expected duplicate credential registration to be rejected")
	}
	if _, ok := session.SessionContext["passkey.registration"]; ok {
		t.Fatalf("duplicate registration must not write passkey.registration: %+v", session.SessionContext["passkey.registration"])
	}
}

func TestWebAuthnRegistrationRejectsMissingOrMismatchedUserHandle(t *testing.T) {
	tests := []struct {
		name       string
		userHandle func(expected string) string
	}{
		{
			name: "missing user handle",
			userHandle: func(string) string {
				return ""
			},
		},
		{
			name: "mismatched user handle",
			userHandle: func(string) string {
				return encodedWebAuthnUserHandle("user:9999")
			},
		},
		{
			name: "non canonical user handle",
			userHandle: func(expected string) string {
				return expected + "="
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeWebAuthnSubjectStore{passkeys: map[string]domain.PasskeyRegistration{}}
			provider := NewWebAuthnPasskeyRegistrationStepProvider(
				challengeinfra.NewWebAuthnService(random.New(config.RandomConfig{NonceLength: 16})),
				store,
				"example.com",
				"Example",
				300,
			)
			now := time.Now().UTC()
			session := &domain.ChallengeSession{
				ChallengeIdentifier: "challenge-registration",
				SubjectIdentifier:   "user:1001",
				BusinessAction:      string(domain.BusinessActionMFAPasskeyBind),
				ChallengeState:      domain.ChallengeStatePending,
				SessionContext:      map[string]any{},
				CreatedAt:           &now,
				ExpiresAt:           timePointer(now.Add(time.Minute)),
			}
			step := &domain.ChallengeStep{
				StepIdentifier: "step-registration",
				ChallengeType:  domain.ChallengeTypeWebAuthnPasskeyRegistration,
				StepState:      domain.ChallengeStepStateInProgress,
			}
			if err := provider.Prepare(context.Background(), session, step); err != nil {
				t.Fatalf("prepare registration: %v", err)
			}
			expected := valueString(session.SessionContext[registrationUserHandleKey(step)])
			credentialIdentifier := base64.RawURLEncoding.EncodeToString([]byte("credential-1"))
			publicKeyCose := encodeCOSEEC2PublicKey(t, mustECDSAKey(t))

			payload := map[string]any{
				"credentialIdentifier": credentialIdentifier,
				"clientDataJSON":       mustRegistrationClientData(t, valueString(session.SessionContext[registrationChallengeKey(step)])),
				"attestationObject":    registrationAttestationObject(t, "none", "credential-1", publicKeyCose, 3),
			}
			if userHandle := tt.userHandle(expected); userHandle != "" {
				payload["userHandle"] = userHandle
			}
			verified, err := provider.Verify(context.Background(), session, step, payload)
			if err != nil {
				t.Fatalf("verify registration: %v", err)
			}
			if verified {
				t.Fatal("expected registration to be rejected")
			}
			if _, ok := session.SessionContext["passkey.registration"]; ok {
				t.Fatalf("invalid registration must not write passkey.registration: %+v", session.SessionContext["passkey.registration"])
			}
		})
	}
}

func TestWebAuthnRegistrationRejectsWhenAllowedOriginsAreMissing(t *testing.T) {
	store := &fakeWebAuthnSubjectStore{passkeys: map[string]domain.PasskeyRegistration{}}
	provider := NewWebAuthnPasskeyRegistrationStepProvider(
		challengeinfra.NewWebAuthnService(random.New(config.RandomConfig{NonceLength: 16})),
		store,
		"example.com",
		"Example",
		300,
	)
	now := time.Now().UTC()
	session := &domain.ChallengeSession{
		ChallengeIdentifier: "challenge-registration",
		SubjectIdentifier:   "user:1001",
		BusinessAction:      string(domain.BusinessActionMFAPasskeyBind),
		ChallengeState:      domain.ChallengeStatePending,
		SessionContext:      map[string]any{},
		CreatedAt:           &now,
		ExpiresAt:           timePointer(now.Add(time.Minute)),
	}
	step := &domain.ChallengeStep{
		StepIdentifier: "step-registration",
		ChallengeType:  domain.ChallengeTypeWebAuthnPasskeyRegistration,
		StepState:      domain.ChallengeStepStateInProgress,
	}
	if err := provider.Prepare(context.Background(), session, step); err != nil {
		t.Fatalf("prepare registration: %v", err)
	}
	publicKeyCose := encodeCOSEEC2PublicKey(t, mustECDSAKey(t))
	verified, err := provider.Verify(context.Background(), session, step, map[string]any{
		"credentialIdentifier": base64.RawURLEncoding.EncodeToString([]byte("credential-1")),
		"clientDataJSON":       mustRegistrationClientData(t, valueString(session.SessionContext[registrationChallengeKey(step)])),
		"attestationObject":    registrationAttestationObject(t, "none", "credential-1", publicKeyCose, 0),
		"userHandle":           valueString(session.SessionContext[registrationUserHandleKey(step)]),
	})
	if err != nil {
		t.Fatalf("verify registration: %v", err)
	}
	if verified {
		t.Fatal("expected registration to be rejected when allowed origins are missing")
	}
}

func TestWebAuthnRegistrationRejectsCredentialIDMismatchFromAttestation(t *testing.T) {
	store := &fakeWebAuthnSubjectStore{passkeys: map[string]domain.PasskeyRegistration{}}
	provider := NewWebAuthnPasskeyRegistrationStepProvider(
		challengeinfra.NewWebAuthnService(random.New(config.RandomConfig{NonceLength: 16})),
		store,
		"example.com",
		"Example",
		300,
	)
	now := time.Now().UTC()
	session := &domain.ChallengeSession{
		ChallengeIdentifier: "challenge-registration",
		SubjectIdentifier:   "user:1001",
		BusinessAction:      string(domain.BusinessActionMFAPasskeyBind),
		ChallengeState:      domain.ChallengeStatePending,
		SessionContext:      map[string]any{},
		CreatedAt:           &now,
		ExpiresAt:           timePointer(now.Add(time.Minute)),
	}
	step := &domain.ChallengeStep{
		StepIdentifier: "step-registration",
		ChallengeType:  domain.ChallengeTypeWebAuthnPasskeyRegistration,
		StepState:      domain.ChallengeStepStateInProgress,
	}
	if err := provider.Prepare(context.Background(), session, step); err != nil {
		t.Fatalf("prepare registration: %v", err)
	}
	publicKeyCose := encodeCOSEEC2PublicKey(t, mustECDSAKey(t))

	verified, err := provider.Verify(context.Background(), session, step, map[string]any{
		"credentialIdentifier": "payload-credential",
		"clientDataJSON":       mustRegistrationClientData(t, valueString(session.SessionContext[registrationChallengeKey(step)])),
		"attestationObject":    registrationAttestationObject(t, "none", "attestation-credential", publicKeyCose, 11),
		"publicKeyCose":        publicKeyCose,
		"userHandle":           valueString(session.SessionContext[registrationUserHandleKey(step)]),
	})
	if err != nil {
		t.Fatalf("verify registration: %v", err)
	}
	if verified {
		t.Fatal("expected credential id mismatch to be rejected")
	}
	if _, ok := session.SessionContext["passkey.registration"]; ok {
		t.Fatalf("invalid registration must not write passkey.registration: %+v", session.SessionContext["passkey.registration"])
	}
}

func TestWebAuthnRegistrationRejectsWrongOriginOrRpIDHash(t *testing.T) {
	tests := []struct {
		name       string
		origin     string
		rpID       string
		wantReject bool
	}{
		{name: "allowed origin and rp id", origin: "https://example.com", rpID: "example.com"},
		{name: "wrong origin", origin: "https://evil.example.com", rpID: "example.com", wantReject: true},
		{name: "wrong rp id hash", origin: "https://example.com", rpID: "evil.example.com", wantReject: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeWebAuthnSubjectStore{passkeys: map[string]domain.PasskeyRegistration{}}
			provider := NewWebAuthnPasskeyRegistrationStepProvider(
				challengeinfra.NewWebAuthnService(random.New(config.RandomConfig{NonceLength: 16})),
				store,
				"example.com",
				"Example",
				300,
				[]string{"https://example.com"},
			)
			now := time.Now().UTC()
			session := &domain.ChallengeSession{
				ChallengeIdentifier: "challenge-registration",
				SubjectIdentifier:   "user:1001",
				BusinessAction:      string(domain.BusinessActionMFAPasskeyBind),
				ChallengeState:      domain.ChallengeStatePending,
				SessionContext:      map[string]any{},
				CreatedAt:           &now,
				ExpiresAt:           timePointer(now.Add(time.Minute)),
			}
			step := &domain.ChallengeStep{
				StepIdentifier: "step-registration",
				ChallengeType:  domain.ChallengeTypeWebAuthnPasskeyRegistration,
				StepState:      domain.ChallengeStepStateInProgress,
			}
			if err := provider.Prepare(context.Background(), session, step); err != nil {
				t.Fatalf("prepare registration: %v", err)
			}
			publicKeyCose := encodeCOSEEC2PublicKey(t, mustECDSAKey(t))
			credentialIdentifier := base64.RawURLEncoding.EncodeToString([]byte("credential-1"))
			verified, err := provider.Verify(context.Background(), session, step, map[string]any{
				"credentialIdentifier": credentialIdentifier,
				"clientDataJSON":       mustRegistrationClientDataWithOrigin(t, valueString(session.SessionContext[registrationChallengeKey(step)]), tt.origin),
				"attestationObject":    registrationAttestationObjectForRP(t, "none", "credential-1", publicKeyCose, 0, tt.rpID),
				"userHandle":           valueString(session.SessionContext[registrationUserHandleKey(step)]),
			})
			if err != nil {
				t.Fatalf("verify registration: %v", err)
			}
			if tt.wantReject && verified {
				t.Fatal("expected registration to be rejected")
			}
			if !tt.wantReject && !verified {
				t.Fatal("expected registration to pass")
			}
		})
	}
}

func mustRegistrationClientData(t *testing.T, challenge string) string {
	t.Helper()
	return mustRegistrationClientDataWithOrigin(t, challenge, "https://example.com")
}

func mustRegistrationClientDataWithOrigin(t *testing.T, challenge, origin string) string {
	t.Helper()
	return encodeJSONForWebAuthn(t, map[string]any{
		"type":      "webauthn.create",
		"challenge": challenge,
		"origin":    origin,
	})
}

func registrationAttestationObject(t *testing.T, format, credentialID, publicKeyCose string, signCount uint32) string {
	t.Helper()
	return registrationAttestationObjectForRP(t, format, credentialID, publicKeyCose, signCount, "example.com")
}

func registrationAttestationObjectForRP(t *testing.T, format, credentialID, publicKeyCose string, signCount uint32, rpID string) string {
	t.Helper()
	authData := make([]byte, 37)
	rpIDHash := sha256.Sum256([]byte(rpID))
	copy(authData[:32], rpIDHash[:])
	authData[32] = 0x41
	binary.BigEndian.PutUint32(authData[33:37], signCount)
	authData = append(authData, make([]byte, 16)...)
	credentialIDBytes := []byte(credentialID)
	authData = append(authData, byte(len(credentialIDBytes)>>8), byte(len(credentialIDBytes)))
	authData = append(authData, credentialIDBytes...)
	authData = append(authData, mustDecodeBase64URLForRegistration(t, publicKeyCose)...)
	object := registrationCBORMap(map[string][]byte{
		"fmt":      registrationCBORText(format),
		"authData": registrationCBORBytes(authData),
		"attStmt":  []byte{0xa0},
	})
	return base64.RawURLEncoding.EncodeToString(object)
}

func mustDecodeBase64URLForRegistration(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("decode base64url: %v", err)
	}
	return decoded
}

func registrationCBORMap(values map[string][]byte) []byte {
	result := []byte{0xa0 | byte(len(values))}
	for _, key := range []string{"fmt", "authData", "attStmt"} {
		value, ok := values[key]
		if !ok {
			continue
		}
		result = append(result, registrationCBORText(key)...)
		result = append(result, value...)
	}
	return result
}

func registrationCBORText(value string) []byte {
	result := registrationCBORHeader(3, len(value))
	return append(result, []byte(value)...)
}

func registrationCBORBytes(value []byte) []byte {
	result := registrationCBORHeader(2, len(value))
	return append(result, value...)
}

func registrationCBORHeader(major byte, length int) []byte {
	switch {
	case length < 24:
		return []byte{major<<5 | byte(length)}
	case length <= 0xff:
		return []byte{major<<5 | 24, byte(length)}
	case length <= 0xffff:
		return []byte{major<<5 | 25, byte(length >> 8), byte(length)}
	default:
		return []byte{major<<5 | 26, byte(length >> 24), byte(length >> 16), byte(length >> 8), byte(length)}
	}
}
