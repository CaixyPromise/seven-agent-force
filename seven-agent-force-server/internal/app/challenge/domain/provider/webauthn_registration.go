package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	challengeinfra "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/infrastructure"
	"github.com/bytedance/sonic"
)

type WebAuthnPasskeyRegistrationStepProvider struct {
	webauthn *challengeinfra.WebAuthnService
	store    SubjectCredentialStore
	rpID     string
	rpName   string
	origins  []string
	timeout  int
}

func NewWebAuthnPasskeyRegistrationStepProvider(webauthn *challengeinfra.WebAuthnService, store SubjectCredentialStore, rpID, rpName string, timeout int, origins ...[]string) *WebAuthnPasskeyRegistrationStepProvider {
	var allowedOrigins []string
	if len(origins) > 0 {
		allowedOrigins = append([]string(nil), origins[0]...)
	}
	return &WebAuthnPasskeyRegistrationStepProvider{webauthn: webauthn, store: store, rpID: rpID, rpName: rpName, origins: allowedOrigins, timeout: timeout}
}

func (p *WebAuthnPasskeyRegistrationStepProvider) Type() domain.ChallengeType {
	return domain.ChallengeTypeWebAuthnPasskeyRegistration
}

func (p *WebAuthnPasskeyRegistrationStepProvider) Prepare(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) error {
	if p == nil || p.webauthn == nil || p.store == nil {
		return fmt.Errorf("webauthn registration provider is not fully configured")
	}
	challenge, err := p.webauthn.GenerateChallenge(ctx)
	if err != nil {
		return err
	}
	session.EnsureSessionContext()[registrationChallengeKey(step)] = challenge
	passkeys, err := p.store.ListPasskeys(ctx, session)
	if err != nil {
		return err
	}
	excludes := make([]string, 0, len(passkeys))
	for _, item := range passkeys {
		excludes = append(excludes, item.CredentialIdentifier)
	}
	hints := step.EnsureUserInterfaceHints()
	hints["challenge"] = challenge
	hints["rpId"] = p.rpID
	hints["rpName"] = p.rpName
	hints["timeoutSeconds"] = p.timeout
	hints["excludeCredentialIds"] = excludes
	userHandle := canonicalWebAuthnUserHandleForSession(session)
	if userHandle == "" {
		return fmt.Errorf("webauthn registration user handle is unavailable")
	}
	session.EnsureSessionContext()[registrationUserHandleKey(step)] = userHandle
	hints["userHandle"] = userHandle
	return nil
}

func (p *WebAuthnPasskeyRegistrationStepProvider) Refresh(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) error {
	return p.Prepare(ctx, session, step)
}

func (p *WebAuthnPasskeyRegistrationStepProvider) Verify(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep, payload map[string]any) (bool, error) {
	if p == nil || p.webauthn == nil || p.store == nil {
		return false, fmt.Errorf("webauthn registration provider is not fully configured")
	}
	expected := valueString(session.EnsureSessionContext()[registrationChallengeKey(step)])
	clientDataJSON := payloadString(payload, "clientDataJSON")
	attestationObject := payloadString(payload, "attestationObject")
	credentialIdentifier := payloadString(payload, "credentialIdentifier")
	displayName := payloadString(payload, "displayName")
	if displayName == "" {
		displayName = "Passkey-" + shortCredentialIdentifier(credentialIdentifier)
	}
	if expected == "" || clientDataJSON == "" || attestationObject == "" || credentialIdentifier == "" {
		return false, nil
	}
	expectedUserHandle := valueString(session.EnsureSessionContext()[registrationUserHandleKey(step)])
	if !webauthnCanonicalUserHandleMatches(expectedUserHandle, payloadString(payload, "userHandle")) {
		return false, nil
	}
	if !p.webauthn.ValidateRegistrationClientData(clientDataJSON, expected) {
		return false, nil
	}
	if !hasAllowedOrigins(p.origins) || !p.webauthn.ValidateClientOrigin(clientDataJSON, p.origins) {
		return false, nil
	}
	parsed, err := p.webauthn.ParseRegistrationAttestation(attestationObject)
	if err != nil {
		return false, nil
	}
	if parsed == nil || strings.TrimSpace(parsed.CredentialIdentifier) == "" {
		return false, nil
	}
	if !p.webauthn.ValidateRegistrationRpIDHash(parsed, p.rpID) {
		return false, nil
	}
	if strings.TrimSpace(credentialIdentifier) != strings.TrimSpace(parsed.CredentialIdentifier) {
		return false, nil
	}
	existing, err := p.store.FindPasskey(ctx, credentialIdentifier)
	if err != nil {
		return false, err
	}
	if existing != nil {
		return false, nil
	}
	publicKeyCose := strings.TrimSpace(parsed.PublicKeyCose)
	signCount := parsed.SignCount
	aaguid := strings.TrimSpace(parsed.AAGUID)
	attestationFormat := strings.TrimSpace(parsed.AttestationFormat)
	if publicKeyCose == "" {
		return false, nil
	}
	if !p.webauthn.ValidatePublicKeyCose(publicKeyCose) {
		return false, nil
	}
	registration := map[string]any{
		"credentialIdentifier": parsed.CredentialIdentifier,
		"publicKeyCose":        publicKeyCose,
		"signCount":            signCount,
		"userHandle":           expectedUserHandle,
		"aaguid":               aaguid,
		"transports":           transportsJSON(payload["transports"]),
		"attestationFormat":    attestationFormat,
		"displayName":          displayName,
	}
	session.EnsureSessionContext()["passkey.registration"] = registration
	return true, nil
}

func registrationChallengeKey(step *domain.ChallengeStep) string {
	return "passkey.registration.challenge." + step.StepIdentifier
}

func registrationUserHandleKey(step *domain.ChallengeStep) string {
	return "passkey.registration.userHandle." + step.StepIdentifier
}

func shortCredentialIdentifier(credentialIdentifier string) string {
	value := strings.TrimSpace(credentialIdentifier)
	if len(value) <= 8 {
		return value
	}
	return value[len(value)-8:]
}

func transportsJSON(value any) string {
	if value == nil {
		return ""
	}
	raw, err := sonic.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	default:
		text := strings.TrimSpace(fmt.Sprintf("%v", value))
		if text == "" {
			return 0
		}
		var parsed int64
		_, _ = fmt.Sscan(text, &parsed)
		return parsed
	}
}
