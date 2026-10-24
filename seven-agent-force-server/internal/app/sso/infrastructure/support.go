package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/sso/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/jwt"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/keyring"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

const revokedMarkerTTL = 30 * 24 * time.Hour

func BuildTokenHash(raw string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func BuildSessionCookie(cfg config.SSOCookieConfig, sessionID string, expiresAt time.Time) string {
	cookie := &http.Cookie{
		Name:     cfg.Name,
		Value:    sessionID,
		Path:     choosePath(cfg.Path),
		HttpOnly: true,
		Secure:   cfg.Secure,
		SameSite: resolveSameSite(cfg.SameSite),
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
	}
	return cookie.String()
}

func BuildExpiredSessionCookie(cfg config.SSOCookieConfig) string {
	cookie := &http.Cookie{
		Name:     cfg.Name,
		Value:    "",
		Path:     choosePath(cfg.Path),
		HttpOnly: true,
		Secure:   cfg.Secure,
		SameSite: resolveSameSite(cfg.SameSite),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
	}
	return cookie.String()
}

func BuildRefreshCookie(cfg config.SSORefreshCookieConfig, token string, expiresAt time.Time) string {
	cookie := &http.Cookie{
		Name:     resolveRefreshCookieWriteName(cfg),
		Value:    token,
		Path:     choosePath(cfg.Path),
		HttpOnly: cfg.HTTPOnly,
		Secure:   cfg.Secure,
		SameSite: resolveSameSite(cfg.SameSite),
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
	}
	return cookie.String()
}

func BuildExpiredRefreshCookies(cfg config.SSORefreshCookieConfig) []string {
	result := make([]string, 0, 2)
	for _, name := range refreshCookieCandidateNames(cfg) {
		cookie := &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     choosePath(cfg.Path),
			HttpOnly: cfg.HTTPOnly,
			Secure:   cfg.Secure,
			SameSite: resolveSameSite(cfg.SameSite),
			MaxAge:   -1,
			Expires:  time.Unix(0, 0).UTC(),
		}
		result = append(result, cookie.String())
	}
	return result
}

func ResolveSessionCookie(req *http.Request, cfg config.SSOCookieConfig) string {
	if req == nil || strings.TrimSpace(cfg.Name) == "" {
		return ""
	}
	cookie, err := req.Cookie(cfg.Name)
	if err != nil || cookie == nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func resolveRefreshCookieWriteName(cfg config.SSORefreshCookieConfig) string {
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return ""
	}
	if !cfg.Secure && strings.HasPrefix(name, "__Host-") {
		return strings.TrimPrefix(name, "__Host-")
	}
	return name
}

func refreshCookieCandidateNames(cfg config.SSORefreshCookieConfig) []string {
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return nil
	}
	result := make([]string, 0, 2)
	writeName := resolveRefreshCookieWriteName(cfg)
	if writeName != "" {
		result = append(result, writeName)
	}
	if name != "" && name != writeName {
		result = append(result, name)
	}
	return result
}

type AuthSessionCache struct {
	cache cache.Manager
}

func NewAuthSessionCache(cacheMgr cache.Manager) *AuthSessionCache {
	return &AuthSessionCache{cache: cacheMgr}
}

func (c *AuthSessionCache) SaveSession(ctx context.Context, snapshot *domain.AuthorizationSessionSnapshot, ttl time.Duration) error {
	if c == nil || c.cache == nil || snapshot == nil || strings.TrimSpace(snapshot.LoginTransactionID) == "" {
		return nil
	}
	return c.cache.Set(ctx, c.sessionKey(snapshot.LoginTransactionID), snapshot, ttl)
}

func (c *AuthSessionCache) GetSession(ctx context.Context, loginTransactionID string) (*domain.AuthorizationSessionSnapshot, error) {
	if c == nil || c.cache == nil || strings.TrimSpace(loginTransactionID) == "" {
		return nil, nil
	}
	var snapshot domain.AuthorizationSessionSnapshot
	hit, err := c.cache.Get(ctx, c.sessionKey(loginTransactionID), &snapshot)
	if err != nil || !hit {
		return nil, err
	}
	return &snapshot, nil
}

func (c *AuthSessionCache) RemoveSession(ctx context.Context, loginTransactionID string) error {
	if c == nil || c.cache == nil || strings.TrimSpace(loginTransactionID) == "" {
		return nil
	}
	return c.cache.Delete(ctx, c.sessionKey(loginTransactionID))
}

func (c *AuthSessionCache) AcquireCompletionLock(ctx context.Context, loginTransactionID string) (bool, error) {
	if c == nil || c.cache == nil {
		return true, nil
	}
	return c.cache.SetNXString(ctx, c.lockKey(loginTransactionID), "1", 15*time.Second)
}

func (c *AuthSessionCache) ReleaseCompletionLock(ctx context.Context, loginTransactionID string) error {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.Delete(ctx, c.lockKey(loginTransactionID))
}

func (c *AuthSessionCache) MarkSessionFinalized(ctx context.Context, loginTransactionID string, ttl time.Duration) (bool, error) {
	if c == nil || c.cache == nil {
		return true, nil
	}
	return c.cache.SetNXString(ctx, c.finalizedKey(loginTransactionID), "1", ttl)
}

func (c *AuthSessionCache) ReleaseSessionFinalized(ctx context.Context, loginTransactionID string) error {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.Delete(ctx, c.finalizedKey(loginTransactionID))
}

func (c *AuthSessionCache) SaveCompletionResult(ctx context.Context, loginTransactionID string, result any, ttl time.Duration) error {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.Set(ctx, c.resultKey(loginTransactionID), result, ttl)
}

func (c *AuthSessionCache) GetCompletionResult(ctx context.Context, loginTransactionID string, dest any) (bool, error) {
	if c == nil || c.cache == nil {
		return false, nil
	}
	return c.cache.Get(ctx, c.resultKey(loginTransactionID), dest)
}

func (c *AuthSessionCache) MarkUserRevoked(ctx context.Context, userID int64, revokedAt time.Time) error {
	if c == nil || c.cache == nil || userID <= 0 {
		return nil
	}
	_, err := c.cache.SetMaxTimestamp(ctx, c.revokedMarkerKey(userID), revokedAt.UTC(), revokedMarkerTTL)
	return err
}

func (c *AuthSessionCache) UserRevokedAt(ctx context.Context, userID int64) (*time.Time, error) {
	if c == nil || c.cache == nil || userID <= 0 {
		return nil, nil
	}
	raw, hit, err := c.cache.GetString(ctx, c.revokedMarkerKey(userID))
	if err != nil || !hit || strings.TrimSpace(raw) == "" {
		return nil, err
	}
	parsed, parseErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw))
	if parseErr != nil {
		return nil, nil
	}
	return &parsed, nil
}

func (c *AuthSessionCache) AllowTouch(ctx context.Context, sessionID string, throttle time.Duration) (bool, error) {
	if c == nil || c.cache == nil || strings.TrimSpace(sessionID) == "" || throttle <= 0 {
		return true, nil
	}
	return c.cache.SetNXString(ctx, c.touchKey(sessionID), "1", throttle)
}

func (c *AuthSessionCache) ClearSessionTouch(ctx context.Context, sessionID string) error {
	if c == nil || c.cache == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	return c.cache.Delete(ctx, c.touchKey(sessionID))
}

func (c *AuthSessionCache) sessionKey(loginTransactionID string) string {
	return c.cache.Builder().Build("sso", "auth-session", loginTransactionID)
}

func (c *AuthSessionCache) lockKey(loginTransactionID string) string {
	return c.cache.Builder().Build("sso", "auth-session-lock", loginTransactionID)
}

func (c *AuthSessionCache) finalizedKey(loginTransactionID string) string {
	return c.cache.Builder().Build("sso", "auth-session-finalized", loginTransactionID)
}

func (c *AuthSessionCache) resultKey(loginTransactionID string) string {
	return c.cache.Builder().Build("sso", "auth-session-result", loginTransactionID)
}

func (c *AuthSessionCache) revokedMarkerKey(userID int64) string {
	return c.cache.Builder().Build("sso", "session", "revoked", "user", userID)
}

func (c *AuthSessionCache) touchKey(sessionID string) string {
	return c.cache.Builder().Build("sso", "session-touch", sessionID)
}

func choosePath(value string) string {
	if strings.TrimSpace(value) == "" {
		return "/"
	}
	return value
}

func resolveSameSite(value string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func strconvTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func BuildSigningProvider(cfg config.SSOConfig) (keyring.SigningKeyProvider, error) {
	keysCfg := config.KeysConfig{
		JWT: config.JWTKeysConfig{
			Algorithm: "RS256",
		},
	}
	for kid, status := range cfg.JWT.KeyStatusByKID {
		privateSource := strings.TrimSpace(cfg.JWT.PrivateKeysByKID[kid])
		publicSource := strings.TrimSpace(cfg.JWT.PublicKeysByKID[kid])
		if publicSource == "" {
			continue
		}
		item := config.JWTKeySourceConfig{
			KID:              strings.TrimSpace(kid),
			PrivateKeySource: privateSource,
			PublicKeySource:  publicSource,
		}
		switch strings.ToUpper(strings.TrimSpace(status)) {
		case "ACTIVE":
			keysCfg.JWT.Active = item
		case "NEXT":
			keysCfg.JWT.Next = item
		default:
			keysCfg.JWT.Retired = append(keysCfg.JWT.Retired, item)
		}
	}
	if keysCfg.JWT.Active.KID == "" && strings.TrimSpace(cfg.JWT.CurrentKID) != "" {
		kid := strings.TrimSpace(cfg.JWT.CurrentKID)
		keysCfg.JWT.Active = config.JWTKeySourceConfig{
			KID:              kid,
			PrivateKeySource: strings.TrimSpace(cfg.JWT.PrivateKeysByKID[kid]),
			PublicKeySource:  strings.TrimSpace(cfg.JWT.PublicKeysByKID[kid]),
		}
	}
	if keysCfg.JWT.Next.KID == "" && strings.TrimSpace(cfg.JWT.NextKID) != "" {
		kid := strings.TrimSpace(cfg.JWT.NextKID)
		keysCfg.JWT.Next = config.JWTKeySourceConfig{
			KID:              kid,
			PrivateKeySource: strings.TrimSpace(cfg.JWT.PrivateKeysByKID[kid]),
			PublicKeySource:  strings.TrimSpace(cfg.JWT.PublicKeysByKID[kid]),
		}
	}
	provider, err := keyring.NewLocalProvider(keysCfg)
	if err != nil {
		return nil, err
	}
	return provider, nil
}

func BuildJWTService(cfg config.SSOConfig) (*jwt.Service, error) {
	provider, err := BuildSigningProvider(cfg)
	if err != nil {
		return nil, err
	}
	return jwt.New(provider, "RS256")
}
