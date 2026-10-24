package infrastructure

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

type DownloadTokenService struct {
	secret  []byte
	cache   cacheinfra.Manager
	cfg     config.FileDistributionConfig
	nowFunc func() time.Time
}

func NewDownloadTokenService(cfg config.FileDistributionConfig, cache cacheinfra.Manager) (*DownloadTokenService, error) {
	secret := strings.TrimSpace(cfg.SignedURLSecret)
	if secret == "" {
		random := make([]byte, 32)
		if _, err := rand.Read(random); err != nil {
			return nil, err
		}
		secret = base64.RawURLEncoding.EncodeToString(random)
	}
	if len(secret) < 32 {
		return nil, fmt.Errorf("file distribution signed URL secret must be at least 32 characters")
	}
	if cfg.SignedURLTTLSeconds <= 0 {
		cfg.SignedURLTTLSeconds = 300
	}
	return &DownloadTokenService{
		secret:  []byte(secret),
		cache:   cache,
		cfg:     cfg,
		nowFunc: time.Now,
	}, nil
}

func (s *DownloadTokenService) Issue(ctx context.Context, fileID, userID int64, scopeID, ip string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("download token service is not configured")
	}
	claims := domain.DownloadTokenClaims{
		FileID:  fileID,
		UserID:  userID,
		ScopeID: strings.TrimSpace(scopeID),
		Exp:     s.nowFunc().Add(time.Duration(s.cfg.SignedURLTTLSeconds) * time.Second).Unix(),
	}
	if s.cfg.AllowIPBind {
		claims.IP = strings.TrimSpace(ip)
	}
	if s.cfg.OneTimeToken {
		random := make([]byte, 16)
		if _, err := rand.Read(random); err != nil {
			return "", err
		}
		claims.JTI = base64.RawURLEncoding.EncodeToString(random)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signature := s.sign(encodedPayload)
	token := encodedPayload + "." + signature
	if claims.JTI != "" && s.cache != nil {
		key := "file:download:token:" + claims.JTI
		if err := s.cache.SetString(ctx, key, token, time.Until(time.Unix(claims.Exp, 0))); err != nil {
			return "", err
		}
	} else if claims.JTI != "" {
		return "", fmt.Errorf("one-time download token requires cache manager")
	}
	return token, nil
}

func (s *DownloadTokenService) Verify(ctx context.Context, token, ip string) (*domain.DownloadTokenClaims, error) {
	if s == nil {
		return nil, fmt.Errorf("download token service is not configured")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid download token")
	}
	expected := s.sign(parts[0])
	if subtle.ConstantTimeCompare([]byte(expected), []byte(parts[1])) != 1 {
		return nil, fmt.Errorf("invalid download token signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, err
	}
	var claims domain.DownloadTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}
	if claims.FileID <= 0 || claims.Exp <= s.nowFunc().Unix() {
		return nil, fmt.Errorf("download token expired")
	}
	if claims.IP != "" && strings.TrimSpace(ip) != claims.IP {
		return nil, fmt.Errorf("download token ip mismatch")
	}
	if claims.JTI != "" && s.cache != nil {
		key := "file:download:token:" + claims.JTI
		stored, ok, err := s.cache.GetDelString(ctx, key)
		if err != nil {
			return nil, err
		}
		if !ok || stored != token {
			return nil, fmt.Errorf("download token already consumed")
		}
	} else if claims.JTI != "" {
		return nil, fmt.Errorf("one-time download token requires cache manager")
	}
	return &claims, nil
}

func (s *DownloadTokenService) sign(payload string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
