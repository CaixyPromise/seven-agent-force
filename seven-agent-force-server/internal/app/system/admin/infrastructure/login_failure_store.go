package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
)

const (
	loginFailureKeyPrefix   = "login_failure:"
	accountLockKeyPrefix    = "account_lock:"
	captchaFailureKeyPrefix = "captcha_failure:"
	contextFailureKeyPrefix = "login_context_failure:"
	failureExpireTTL        = 24 * time.Hour
)

type LoginFailureStateStore struct {
	cache cache.Manager
}

func NewLoginFailureStateStore(cacheManager cache.Manager) *LoginFailureStateStore {
	return &LoginFailureStateStore{cache: cacheManager}
}

func (s *LoginFailureStateStore) GetFailureCount(ctx context.Context, userAccount string) (int, error) {
	return s.getInt(ctx, s.failureKeys(userAccount)...)
}

func (s *LoginFailureStateStore) SaveFailureCount(ctx context.Context, userAccount string, count int) error {
	if s == nil || s.cache == nil || strings.TrimSpace(userAccount) == "" {
		return nil
	}
	return s.cache.SetString(ctx, s.failureKey(userAccount), strconv.Itoa(count), failureExpireTTL)
}

func (s *LoginFailureStateStore) DeleteFailureCount(ctx context.Context, userAccount string) error {
	return s.deleteMany(ctx, s.failureKeys(userAccount)...)
}

func (s *LoginFailureStateStore) GetContextFailureCount(ctx context.Context, scope, value string) (int, error) {
	return s.getInt(ctx, s.contextFailureKey(scope, value))
}

func (s *LoginFailureStateStore) SaveContextFailureCount(ctx context.Context, scope, value string, count int) error {
	if s == nil || s.cache == nil || strings.TrimSpace(scope) == "" || strings.TrimSpace(value) == "" {
		return nil
	}
	return s.cache.SetString(ctx, s.contextFailureKey(scope, value), strconv.Itoa(count), failureExpireTTL)
}

func (s *LoginFailureStateStore) GetCaptchaFailureCount(ctx context.Context, userAccount string) (int, error) {
	return s.getInt(ctx, s.captchaFailureKeys(userAccount)...)
}

func (s *LoginFailureStateStore) SaveCaptchaFailureCount(ctx context.Context, userAccount string, count int) error {
	if s == nil || s.cache == nil || strings.TrimSpace(userAccount) == "" {
		return nil
	}
	return s.cache.SetString(ctx, s.captchaFailureKey(userAccount), strconv.Itoa(count), failureExpireTTL)
}

func (s *LoginFailureStateStore) DeleteCaptchaFailureCount(ctx context.Context, userAccount string) error {
	return s.deleteMany(ctx, s.captchaFailureKeys(userAccount)...)
}

func (s *LoginFailureStateStore) GetLockUntil(ctx context.Context, userAccount string) (*int64, error) {
	if s == nil || s.cache == nil || strings.TrimSpace(userAccount) == "" {
		return nil, nil
	}
	for _, key := range s.lockKeys(userAccount) {
		raw, found, err := s.cache.GetString(ctx, key)
		if err != nil {
			return nil, err
		}
		if !found || strings.TrimSpace(raw) == "" {
			continue
		}
		value, parseErr := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if parseErr != nil {
			continue
		}
		return &value, nil
	}
	return nil, nil
}

func (s *LoginFailureStateStore) SaveLockUntil(ctx context.Context, userAccount string, unlockTime int64, ttlHours int) error {
	if s == nil || s.cache == nil || strings.TrimSpace(userAccount) == "" {
		return nil
	}
	ttl := time.Duration(ttlHours) * time.Hour
	if ttlHours <= 0 {
		ttl = 0
	}
	return s.cache.SetString(ctx, s.lockKey(userAccount), strconv.FormatInt(unlockTime, 10), ttl)
}

func (s *LoginFailureStateStore) DeleteLock(ctx context.Context, userAccount string) error {
	return s.deleteMany(ctx, s.lockKeys(userAccount)...)
}

func (s *LoginFailureStateStore) getInt(ctx context.Context, keys ...string) (int, error) {
	if s == nil || s.cache == nil {
		return 0, nil
	}
	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			continue
		}
		raw, found, err := s.cache.GetString(ctx, key)
		if err != nil {
			return 0, err
		}
		if !found || strings.TrimSpace(raw) == "" {
			continue
		}
		value, parseErr := strconv.Atoi(strings.TrimSpace(raw))
		if parseErr != nil {
			continue
		}
		return value, nil
	}
	return 0, nil
}

func (s *LoginFailureStateStore) deleteMany(ctx context.Context, keys ...string) error {
	if s == nil || s.cache == nil {
		return nil
	}
	filtered := make([]string, 0, len(keys))
	for _, key := range keys {
		if strings.TrimSpace(key) != "" {
			filtered = append(filtered, key)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return s.cache.DeleteMany(ctx, filtered...)
}

func (s *LoginFailureStateStore) failureKey(userAccount string) string {
	return loginFailureKeyPrefix + hashedAccountSegment(userAccount)
}

func (s *LoginFailureStateStore) failureKeys(userAccount string) []string {
	keys := []string{s.failureKey(userAccount), loginFailureKeyPrefix + strings.TrimSpace(userAccount)}
	if s != nil && s.cache != nil {
		keys = append(keys, s.cache.Builder().Build("login", "failure", strings.TrimSpace(userAccount)))
	}
	return keys
}

func (s *LoginFailureStateStore) lockKey(userAccount string) string {
	return accountLockKeyPrefix + hashedAccountSegment(userAccount)
}

func (s *LoginFailureStateStore) lockKeys(userAccount string) []string {
	keys := []string{s.lockKey(userAccount), accountLockKeyPrefix + strings.TrimSpace(userAccount)}
	if s != nil && s.cache != nil {
		keys = append(keys, s.cache.Builder().Build("login", "lock", strings.TrimSpace(userAccount)))
	}
	return keys
}

func (s *LoginFailureStateStore) captchaFailureKey(userAccount string) string {
	return captchaFailureKeyPrefix + hashedAccountSegment(userAccount)
}

func (s *LoginFailureStateStore) captchaFailureKeys(userAccount string) []string {
	keys := []string{s.captchaFailureKey(userAccount), captchaFailureKeyPrefix + strings.TrimSpace(userAccount)}
	if s != nil && s.cache != nil {
		keys = append(keys, s.cache.Builder().Build("login", "captcha-failure", strings.TrimSpace(userAccount)))
	}
	return keys
}

func (s *LoginFailureStateStore) contextFailureKey(scope, value string) string {
	scope = normalizeContextScope(scope)
	if scope == "" || strings.TrimSpace(value) == "" {
		return ""
	}
	return contextFailureKeyPrefix + scope + ":" + hashedAccountSegment(value)
}

func normalizeContextScope(scope string) string {
	switch strings.TrimSpace(scope) {
	case "ip":
		return "ip"
	case "device":
		return "device"
	case "ip_device":
		return "ip_device"
	default:
		return ""
	}
}

func hashedAccountSegment(userAccount string) string {
	trimmed := strings.TrimSpace(userAccount)
	if trimmed == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(trimmed))
	return "sha256:" + hex.EncodeToString(digest[:])
}
