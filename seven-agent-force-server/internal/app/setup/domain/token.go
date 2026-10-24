package domain

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/google/uuid"
)

const minTokenSecretLength = 32

type TokenPayload struct {
	Exp          int64  `json:"exp"`
	StartupEpoch int64  `json:"startupEpoch"`
	Nonce        string `json:"nonce"`
}

type TokenService struct {
	secret       []byte
	ttl          time.Duration
	startupEpoch int64
}

func NewTokenService(tokenSecret string, ttlSeconds int64, startupEpoch int64) (*TokenService, error) {
	secret := strings.TrimSpace(tokenSecret)
	if secret != "" && len(secret) < minTokenSecretLength {
		return nil, fmt.Errorf("setup.tokenSecret length must not be less than %d characters", minTokenSecretLength)
	}
	if secret == "" {
		secret = randomSecret()
	}
	if ttlSeconds <= 0 {
		ttlSeconds = 300
	}
	return &TokenService{
		secret:       []byte(secret),
		ttl:          time.Duration(ttlSeconds) * time.Second,
		startupEpoch: startupEpoch,
	}, nil
}

func (s *TokenService) Generate(now time.Time) (string, error) {
	if s == nil || len(s.secret) == 0 {
		return "", apperrors.System("setup token service未配置")
	}
	payload := TokenPayload{
		Exp:          now.UTC().Add(s.ttl).Unix(),
		StartupEpoch: s.startupEpoch,
		Nonce:        uuid.NewString(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal setup token payload: %w", err)
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(raw)
	return encodedPayload + "." + s.sign(encodedPayload), nil
}

func (s *TokenService) Validate(token string, now time.Time) (*TokenPayload, error) {
	if s == nil || len(s.secret) == 0 {
		return nil, apperrors.System("setup token service未配置")
	}
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return nil, invalidSetupToken("初始化校验缺失，请刷新页面后重试")
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) != 2 {
		return nil, invalidSetupToken("")
	}
	expected := s.sign(parts[0])
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return nil, invalidSetupToken("")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, invalidSetupToken("")
	}
	var payload TokenPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, invalidSetupToken("")
	}
	if strings.TrimSpace(payload.Nonce) == "" || payload.StartupEpoch != s.startupEpoch || payload.Exp <= now.UTC().Unix() {
		return nil, invalidSetupToken("")
	}
	return &payload, nil
}

func (s *TokenService) sign(encodedPayload string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(encodedPayload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func invalidSetupToken(message string) error {
	if strings.TrimSpace(message) == "" {
		message = "初始化校验无效，请刷新页面后重试"
	}
	return apperrors.New(apperrors.CodeNoAuth, apperrors.KindForbidden, message)
}

func randomSecret() string {
	buf := make([]byte, 48)
	if _, err := rand.Read(buf); err != nil {
		return strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", "")
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
