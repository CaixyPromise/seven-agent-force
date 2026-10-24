package infrastructure

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	jwtinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/jwt"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/keyring"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestProofTokenVerifyRejectsInvalidTokenAsForbidden(t *testing.T) {
	service := NewProofTokenService(newProofTokenTestJWT(t), &proofTokenTestSessionRepository{consumeResult: true}, time.Minute, 5*time.Minute)

	_, err := service.Verify(context.Background(), facade.ProofTokenVerifyRequest{
		ProofToken:          "not-a-jwt",
		AudienceServiceName: "auth",
		BusinessAction:      "ADMIN_FORCE_LOGOUT",
		FlowNonce:           "flow-1",
		SubjectIdentifier:   "user:1001",
		OperationBinding:    "user:1002|force-logout",
	})

	assertForbiddenProofTokenError(t, err)
}

func TestProofTokenVerifyRejectsBindingMismatchAsForbidden(t *testing.T) {
	service := NewProofTokenService(newProofTokenTestJWT(t), &proofTokenTestSessionRepository{consumeResult: true}, time.Minute, 5*time.Minute)
	_, token, err := service.Issue(context.Background(), proofTokenTestSession())
	if err != nil {
		t.Fatalf("issue proof token: %v", err)
	}

	_, err = service.Verify(context.Background(), facade.ProofTokenVerifyRequest{
		ProofToken:          token,
		AudienceServiceName: "auth",
		BusinessAction:      "ADMIN_FORCE_LOGOUT",
		FlowNonce:           "flow-1",
		SubjectIdentifier:   "user:1001",
		OperationBinding:    "user:9999|force-logout",
	})

	assertForbiddenProofTokenError(t, err)
}

func TestProofTokenVerifyRejectsClaimMismatchesAsForbidden(t *testing.T) {
	cases := []struct {
		name    string
		session *domain.ChallengeSession
		request facade.ProofTokenVerifyRequest
	}{
		{
			name:    "business action mismatch",
			session: proofTokenTestSession(),
			request: facade.ProofTokenVerifyRequest{
				AudienceServiceName: "auth",
				BusinessAction:      "ADMIN_RESET_PASSWORD",
				FlowNonce:           "flow-1",
				SubjectIdentifier:   "user:1001",
				OperationBinding:    "user:1002|force-logout",
			},
		},
		{
			name:    "flow nonce mismatch",
			session: proofTokenTestSession(),
			request: facade.ProofTokenVerifyRequest{
				AudienceServiceName: "auth",
				BusinessAction:      "ADMIN_FORCE_LOGOUT",
				FlowNonce:           "other-flow",
				SubjectIdentifier:   "user:1001",
				OperationBinding:    "user:1002|force-logout",
			},
		},
		{
			name:    "subject mismatch",
			session: proofTokenTestSession(),
			request: facade.ProofTokenVerifyRequest{
				AudienceServiceName: "auth",
				BusinessAction:      "ADMIN_FORCE_LOGOUT",
				FlowNonce:           "flow-1",
				SubjectIdentifier:   "user:9999",
				OperationBinding:    "user:1002|force-logout",
			},
		},
		{
			name:    "audience mismatch",
			session: proofTokenTestSession(),
			request: facade.ProofTokenVerifyRequest{
				AudienceServiceName: "other-service",
				BusinessAction:      "ADMIN_FORCE_LOGOUT",
				FlowNonce:           "flow-1",
				SubjectIdentifier:   "user:1001",
				OperationBinding:    "user:1002|force-logout",
			},
		},
		{
			name:    "expired proof",
			session: expiredProofTokenTestSession(),
			request: facade.ProofTokenVerifyRequest{
				AudienceServiceName: "auth",
				BusinessAction:      "ADMIN_FORCE_LOGOUT",
				FlowNonce:           "flow-1",
				SubjectIdentifier:   "user:1001",
				OperationBinding:    "user:1002|force-logout",
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			service := NewProofTokenService(newProofTokenTestJWT(t), &proofTokenTestSessionRepository{consumeResult: true}, time.Minute, 5*time.Minute)
			_, token, err := service.Issue(context.Background(), tt.session)
			if err != nil {
				t.Fatalf("issue proof token: %v", err)
			}
			tt.request.ProofToken = token

			_, err = service.Verify(context.Background(), tt.request)

			assertForbiddenProofTokenError(t, err)
		})
	}
}

func TestProofTokenVerifyRejectsConsumedTokenAsForbidden(t *testing.T) {
	service := NewProofTokenService(newProofTokenTestJWT(t), &proofTokenTestSessionRepository{consumeResult: false}, time.Minute, 5*time.Minute)
	_, token, err := service.Issue(context.Background(), proofTokenTestSession())
	if err != nil {
		t.Fatalf("issue proof token: %v", err)
	}

	_, err = service.Verify(context.Background(), facade.ProofTokenVerifyRequest{
		ProofToken:          token,
		AudienceServiceName: "auth",
		BusinessAction:      "ADMIN_FORCE_LOGOUT",
		FlowNonce:           "flow-1",
		SubjectIdentifier:   "user:1001",
		OperationBinding:    "user:1002|force-logout",
		ConsumeOnce:         true,
	})

	assertForbiddenProofTokenError(t, err)
}

func proofTokenTestSession() *domain.ChallengeSession {
	now := time.Now().UTC()
	expiresAt := now.Add(10 * time.Minute)
	return &domain.ChallengeSession{
		ChallengeIdentifier:       "challenge-proof",
		IssuingServiceName:        "challenge",
		AudienceServiceNames:      []string{"auth"},
		SubjectIdentifier:         "user:1001",
		FlowNonce:                 "flow-1",
		BusinessAction:            "ADMIN_FORCE_LOGOUT",
		AuthenticationMethodNames: []string{"TOTP"},
		CreatedAt:                 &now,
		ExpiresAt:                 &expiresAt,
		SessionContext:            map[string]any{"operationBinding": "user:1002|force-logout"},
	}
}

func expiredProofTokenTestSession() *domain.ChallengeSession {
	session := proofTokenTestSession()
	now := time.Now().UTC()
	expiresAt := now.Add(-time.Minute)
	session.ExpiresAt = &expiresAt
	return session
}

func assertForbiddenProofTokenError(t *testing.T, err error) {
	t.Helper()
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code() != apperrors.CodeForbidden {
		t.Fatalf("expected forbidden proof-token error, got %#v", appErr)
	}
}

func newProofTokenTestJWT(t *testing.T) *jwtinfra.Service {
	t.Helper()
	dir := t.TempDir()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	privatePath := filepath.Join(dir, "jwt.key")
	publicPath := filepath.Join(dir, "jwt.pub")
	writeProofTokenPEM(t, privatePath, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(privateKey))
	publicDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	writeProofTokenPEM(t, publicPath, "PUBLIC KEY", publicDER)

	keys, err := keyring.NewLocalProvider(config.KeysConfig{
		Provider: "local",
		JWT: config.JWTKeysConfig{
			Algorithm: "RS256",
			Active: config.JWTKeySourceConfig{
				KID:              "kid-active",
				PrivateKeySource: "file:" + privatePath,
				PublicKeySource:  "file:" + publicPath,
			},
		},
	})
	if err != nil {
		t.Fatalf("new local key provider: %v", err)
	}
	service, err := jwtinfra.New(keys, "RS256")
	if err != nil {
		t.Fatalf("new jwt service: %v", err)
	}
	return service
}

func writeProofTokenPEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	block := &pem.Block{Type: blockType, Bytes: der}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("write pem file: %v", err)
	}
}

type proofTokenTestSessionRepository struct {
	consumeResult bool
}

func (r *proofTokenTestSessionRepository) SaveSession(context.Context, *domain.ChallengeSession) error {
	return nil
}

func (r *proofTokenTestSessionRepository) GetSession(context.Context, string) (*domain.ChallengeSession, error) {
	return nil, apperrors.NotFound("challenge not found")
}

func (r *proofTokenTestSessionRepository) BindIdempotencyKey(context.Context, string, string, time.Duration) error {
	return nil
}

func (r *proofTokenTestSessionRepository) GetSessionByIdempotencyKey(context.Context, string) (string, bool, error) {
	return "", false, nil
}

func (r *proofTokenTestSessionRepository) AcquireSubmitLock(context.Context, string, time.Duration) (string, bool, error) {
	return "", true, nil
}

func (r *proofTokenTestSessionRepository) ReleaseSubmitLock(context.Context, string, string) error {
	return nil
}

func (r *proofTokenTestSessionRepository) MarkProofConsumed(context.Context, string, string, time.Duration) (bool, error) {
	return r.consumeResult, nil
}
