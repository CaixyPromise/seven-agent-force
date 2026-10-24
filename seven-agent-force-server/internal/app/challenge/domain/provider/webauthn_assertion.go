package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	challengeinfra "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/infrastructure"
)

type WebAuthnPasskeyAssertionStepProvider struct {
	webauthn *challengeinfra.WebAuthnService
	store    SubjectCredentialStore
	rpID     string
	origins  []string
	timeout  int
}

func NewWebAuthnPasskeyAssertionStepProvider(webauthn *challengeinfra.WebAuthnService, store SubjectCredentialStore, rpID string, origins []string, timeout int) *WebAuthnPasskeyAssertionStepProvider {
	return &WebAuthnPasskeyAssertionStepProvider{webauthn: webauthn, store: store, rpID: rpID, origins: origins, timeout: timeout}
}

func (p *WebAuthnPasskeyAssertionStepProvider) Type() domain.ChallengeType {
	return domain.ChallengeTypeWebAuthnPasskeyAssertion
}

func (p *WebAuthnPasskeyAssertionStepProvider) Eligible(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) (bool, error) {
	_ = step
	if p == nil || p.store == nil || !hasAllowedOrigins(p.origins) {
		return false, nil
	}
	passkeys, err := p.store.ListPasskeys(ctx, session)
	if err != nil {
		return false, err
	}
	for _, item := range passkeys {
		if strings.TrimSpace(item.CredentialIdentifier) != "" {
			return true, nil
		}
	}
	return false, nil
}

func (p *WebAuthnPasskeyAssertionStepProvider) Prepare(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep) error {
	if p == nil || p.webauthn == nil || p.store == nil {
		return fmt.Errorf("webauthn assertion provider is not fully configured")
	}
	challenge, err := p.webauthn.GenerateChallenge(ctx)
	if err != nil {
		return err
	}
	session.EnsureSessionContext()[assertionChallengeKey(step)] = challenge
	passkeys, err := p.store.ListPasskeys(ctx, session)
	if err != nil {
		return err
	}
	allowIDs := make([]string, 0, len(passkeys))
	for _, item := range passkeys {
		allowIDs = append(allowIDs, item.CredentialIdentifier)
	}
	hints := step.EnsureUserInterfaceHints()
	hints["challenge"] = challenge
	hints["rpId"] = p.rpID
	hints["timeoutSeconds"] = p.timeout
	hints["allowCredentialIds"] = allowIDs
	return nil
}

func (p *WebAuthnPasskeyAssertionStepProvider) Verify(ctx context.Context, session *domain.ChallengeSession, step *domain.ChallengeStep, payload map[string]any) (bool, error) {
	if p == nil || p.webauthn == nil || p.store == nil {
		return false, fmt.Errorf("webauthn assertion provider is not fully configured")
	}
	credentialIdentifier := payloadString(payload, "credentialIdentifier")
	clientDataJSON := payloadString(payload, "clientDataJSON")
	authenticatorData := payloadString(payload, "authenticatorData")
	signature := payloadString(payload, "signature")
	if credentialIdentifier == "" || clientDataJSON == "" || authenticatorData == "" || signature == "" {
		return false, nil
	}
	expected := valueString(session.EnsureSessionContext()[assertionChallengeKey(step)])
	if !p.webauthn.ValidateAssertionClientData(clientDataJSON, expected) {
		return false, nil
	}
	if !hasAllowedOrigins(p.origins) || !p.webauthn.ValidateClientOrigin(clientDataJSON, p.origins) || !p.webauthn.ValidateRpIDHash(authenticatorData, p.rpID) || !p.webauthn.ValidateUserPresence(authenticatorData) {
		return false, nil
	}
	allowed, err := p.store.ListPasskeys(ctx, session)
	if err != nil {
		return false, err
	}
	if !credentialAllowed(credentialIdentifier, allowed) {
		return false, nil
	}
	passkey, err := p.store.FindPasskey(ctx, credentialIdentifier)
	if err != nil || passkey == nil {
		return false, err
	}
	if !p.webauthn.VerifyAssertionSignature(passkey.PublicKeyCose, authenticatorData, clientDataJSON, signature) {
		return false, nil
	}
	if !webauthnUserHandleMatches(passkey.UserHandle, payloadString(payload, "userHandle")) {
		return false, nil
	}
	signCount := p.webauthn.ParseSignCount(authenticatorData, passkey.SignCount+1)
	if signCount == 0 && passkey.SignCount == 0 {
		return p.store.UpdatePasskeyUsage(ctx, credentialIdentifier, passkey.SignCount, time.Now().UTC()) == nil, nil
	}
	if signCount <= passkey.SignCount {
		return false, nil
	}
	if err := p.store.UpdatePasskeyUsage(ctx, credentialIdentifier, signCount, time.Now().UTC()); err != nil {
		return false, err
	}
	return true, nil
}

func assertionChallengeKey(step *domain.ChallengeStep) string {
	return "passkey.assertion.challenge." + step.StepIdentifier
}

func credentialAllowed(credentialIdentifier string, passkeys []domain.PasskeyRegistration) bool {
	for _, item := range passkeys {
		if item.CredentialIdentifier == credentialIdentifier {
			return true
		}
	}
	return false
}

func hasAllowedOrigins(origins []string) bool {
	for _, origin := range origins {
		if strings.TrimSpace(origin) != "" {
			return true
		}
	}
	return false
}
