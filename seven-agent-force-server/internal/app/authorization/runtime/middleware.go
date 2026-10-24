package runtime

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net"
	neturl "net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
)

const (
	headerAuthorization      = "Authorization"
	headerGatewaySignature   = "X-Gateway-Signature"
	headerGatewayTimestamp   = "X-Gateway-Timestamp"
	headerGatewayNonce       = "X-Gateway-Nonce"
	headerGatewayVersion     = "X-Gateway-Signature-Version"
	headerInternalTimestamp  = "X-Internal-Timestamp"
	headerInternalNonce      = "X-Internal-Nonce"
	headerInternalSignature  = "X-Internal-Signature"
	headerUserID             = "X-User-Id"
	headerUsername           = "X-Username"
	headerNickname           = "X-Nickname"
	headerRoles              = "X-Roles"
	headerPermissions        = "X-Permissions"
	headerOrgID              = "X-Org-Id"
	headerOrgIDs             = "X-Org-Ids"
	headerDeptID             = "X-Dept-Id"
	headerDeptIDs            = "X-Dept-Ids"
	headerPostIDs            = "X-Post-Ids"
	headerPostCodes          = "X-Post-Codes"
	headerDataScopeDeptIDs   = "X-Data-Scope-Dept-Ids"
	headerDataScopeOrgIDs    = "X-Data-Scope-Org-Ids"
	headerDataScopeType      = "X-Data-Scope-Type"
	headerIsAdmin            = "X-Is-Admin"
	headerSessionID          = "X-Session-Id"
	headerAuthVersion        = "X-Auth-Version"
	headerSessionVersion     = "X-Session-Version"
	headerIssuedAtEpoch      = "X-Issued-At-Epoch"
	headerExpireAtEpoch      = "X-Expire-At-Epoch"
	cacheGatewayNoncePrefix  = "seven:auth:gateway:nonce:"
	cacheInternalNoncePrefix = "seven:auth:internal:nonce:"
)

type Middleware struct {
	cfg               config.AuthorizationConfig
	cache             cacheinfra.Manager
	service           ContextBuilder
	sessionCookieName string
	contextPath       string
}

type ContextBuilder interface {
	BuildContextFromAccessToken(ctx context.Context, accessToken string, source string) (*securitycontext.UserContext, error)
	BuildContextFromSession(ctx context.Context, sessionID string, source string) (*securitycontext.UserContext, error)
}

func NewMiddleware(cfg config.AuthorizationConfig, cache cacheinfra.Manager, service ContextBuilder, sessionCookieName string, contextPath ...string) *Middleware {
	path := ""
	if len(contextPath) > 0 {
		path = normalizeContextPath(contextPath[0])
	}
	return &Middleware{cfg: cfg, cache: cache, service: service, sessionCookieName: strings.TrimSpace(sessionCookieName), contextPath: path}
}

func (m *Middleware) Handler() app.HandlerFunc {
	return func(ctx context.Context, reqCtx *app.RequestContext) {
		defer securitycontext.Clear(reqCtx)

		path := m.normalizeRequestPath(string(reqCtx.Request.URI().Path()))
		rawPath := m.normalizeRequestPath(string(reqCtx.Request.URI().PathOriginal()))
		// Workforce Machine API requests authenticate at the owning deep-module
		// controller with a Worker Token plus instance/boot/generation fencing.
		// They intentionally do not use the Federation internal-service token.
		if m.isWorkforceMachinePath(path) {
			securitycontext.Set(reqCtx, securitycontext.Anonymous())
			reqCtx.Next(ctx)
			return
		}
		if m.isInternalPath(path) {
			if !m.authenticateInternal(ctx, reqCtx, path) {
				reqCtx.Abort()
				return
			}
			reqCtx.Next(ctx)
			return
		}

		if m.cfg.Mode == config.AuthorizationModeRemote && m.cfg.Remote.AcceptGatewayHeaders && m.hasGatewayHeaders(reqCtx) {
			if ok := m.authenticateGateway(ctx, reqCtx, path); !ok {
				reqCtx.Abort()
				return
			}
			reqCtx.Next(ctx)
			return
		}

		bearer := extractBearer(reqCtx)
		if bearer != "" {
			userContext, err := m.service.BuildContextFromAccessToken(ctx, bearer, string(m.cfg.Mode))
			if err != nil {
				if m.isAnonymousRequest(string(reqCtx.Method()), path, rawPath) {
					securitycontext.Set(reqCtx, securitycontext.Anonymous())
					reqCtx.Next(ctx)
					return
				}
				response.Error(reqCtx, err)
				reqCtx.Abort()
				return
			}
			securitycontext.Set(reqCtx, userContext)
			reqCtx.Next(ctx)
			return
		}

		if sessionID := readCookie(reqCtx, m.sessionCookieName); sessionID != "" {
			userContext, err := m.service.BuildContextFromSession(ctx, sessionID, "local-session")
			if err == nil && userContext != nil {
				securitycontext.Set(reqCtx, userContext)
				reqCtx.Next(ctx)
				return
			}
			if !m.isAnonymousRequest(string(reqCtx.Method()), path, rawPath) {
				response.Error(reqCtx, apperrors.Unauthorized("未登录或登录信息失效"))
				reqCtx.Abort()
				return
			}
		}

		if m.isAnonymousRequest(string(reqCtx.Method()), path, rawPath) {
			securitycontext.Set(reqCtx, securitycontext.Anonymous())
			reqCtx.Next(ctx)
			return
		}
		response.Error(reqCtx, apperrors.Unauthorized("未登录或登录信息失效"))
		reqCtx.Abort()
	}
}

func (m *Middleware) normalizeRequestPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if m.contextPath == "" {
		return path
	}
	if path == m.contextPath {
		return "/"
	}
	if strings.HasPrefix(path, m.contextPath+"/") {
		return strings.TrimPrefix(path, m.contextPath)
	}
	return path
}

func normalizeContextPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "/" {
		return ""
	}
	return "/" + strings.Trim(value, "/")
}

// isAnonymousRequest determines whether the global authorization middleware
// may pass a request to a handler that owns its remaining policy decision.
// Most historical anonymous entries are path-only.  Method-qualified entries
// deliberately exist for narrowly scoped handler-owned public resources such
// as the stable CONFIG_ASSET representation.
func (m *Middleware) isAnonymousRequest(method, path, rawPath string) bool {
	values := append([]string{}, m.cfg.AnonymousURLs...)
	if len(values) == 0 {
		values = []string{
			"/ping",
			"/healthz",
			"/ops/modules",
			"/sso/.well-known/**",
			"/sso/runtime/config",
			"/sso/oauth2/authorize",
			"/sso/oauth2/authorize/login",
			"/sso/oauth2/token",
			"/sso/oauth2/userinfo",
			"/sso/oauth2/revoke",
			"/sso/oauth2/introspect",
			"/login/**",
			"/v1/challenges/**",
			"/dict-client/**",
			"GET /config-assets/:id",
		}
	}
	for _, pattern := range values {
		if isStrictConfigAssetAnonymousPattern(pattern) && !isCanonicalConfigAssetPath(rawPath) {
			continue
		}
		if matchAnonymousRoutePattern(strings.TrimSpace(pattern), method, path) {
			return true
		}
	}
	return false
}

// isStrictConfigAssetAnonymousPattern identifies the one handler-owned public
// resource for which the middleware must validate both the normalized router
// path and the unescaped request path. Hertz may normalize an encoded slash
// before route matching; treating only the normalized value as authoritative
// would turn an encoded path variant into an anonymous capability.
func isStrictConfigAssetAnonymousPattern(pattern string) bool {
	return strings.EqualFold(strings.TrimSpace(pattern), "GET /config-assets/:id")
}

// isCanonicalConfigAssetPath intentionally accepts only the literal stable
// presentation form. It operates on PathOriginal, so percent-encoded slashes,
// digits, traversal markers, query-like data, and extra segments cannot be
// normalized into a valid anonymous route before this check runs.
func isCanonicalConfigAssetPath(path string) bool {
	const prefix = "/config-assets/"
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	idText := strings.TrimPrefix(path, prefix)
	if idText == "" || strings.ContainsAny(idText, "/?#%\\") {
		return false
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	return err == nil && id > 0 && strconv.FormatInt(id, 10) == idText
}

// matchAnonymousRoutePattern supports existing path-only policy entries and
// an opt-in "METHOD /path" form.  It does not infer methods from a path-only
// entry, preserving historical behavior while letting a new route avoid
// accidentally granting POST/PUT/DELETE access through an anonymous rule.
func matchAnonymousRoutePattern(pattern, method, path string) bool {
	pattern = strings.TrimSpace(pattern)
	method = strings.ToUpper(strings.TrimSpace(method))
	parts := strings.Fields(pattern)
	if len(parts) == 2 && isHTTPMethod(parts[0]) {
		return strings.EqualFold(parts[0], method) && matchPathPattern(parts[1], path)
	}
	return matchPathPattern(pattern, path)
}

func isHTTPMethod(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "TRACE", "CONNECT":
		return true
	default:
		return false
	}
}

func (m *Middleware) isInternalPath(path string) bool {
	return path == "/internal" || strings.HasPrefix(path, "/internal/")
}

func (m *Middleware) isWorkforceMachinePath(path string) bool {
	return path == "/internal/workforce/v1" || strings.HasPrefix(path, "/internal/workforce/v1/")
}

func (m *Middleware) hasGatewayHeaders(reqCtx *app.RequestContext) bool {
	return strings.TrimSpace(string(reqCtx.Request.Header.Peek(headerUserID))) != ""
}

func (m *Middleware) authenticateGateway(ctx context.Context, reqCtx *app.RequestContext, path string) bool {
	if !m.verifyTrustedProxy(reqCtx) {
		response.Error(reqCtx, apperrors.Unauthorized("网关来源不可信"))
		return false
	}
	if !m.verifyGatewaySignature(ctx, reqCtx, path) {
		response.Error(reqCtx, apperrors.Unauthorized("网关签名校验失败"))
		return false
	}
	securitycontext.Set(reqCtx, extractGatewayUserContext(reqCtx))
	return true
}

func (m *Middleware) authenticateInternal(ctx context.Context, reqCtx *app.RequestContext, path string) bool {
	if !m.cfg.Internal.Enabled {
		response.Error(reqCtx, apperrors.Unauthorized("内部服务鉴权未启用"))
		return false
	}
	headerName := strings.TrimSpace(m.cfg.Internal.HeaderName)
	if headerName == "" {
		headerName = "X-Internal-Token"
	}
	expectedToken := strings.TrimSpace(m.cfg.Internal.Token)
	actualToken := strings.TrimSpace(string(reqCtx.Request.Header.Peek(headerName)))
	if expectedToken == "" || actualToken == "" || !hmac.Equal([]byte(expectedToken), []byte(actualToken)) {
		response.Error(reqCtx, apperrors.Unauthorized("内部服务鉴权失败"))
		return false
	}
	if m.cfg.Internal.SignatureEnabled {
		if !m.verifyInternalSignature(ctx, reqCtx, path, actualToken) {
			response.Error(reqCtx, apperrors.Unauthorized("内部服务签名校验失败"))
			return false
		}
	}
	securitycontext.Set(reqCtx, &securitycontext.UserContext{
		Username:    "internal-service",
		Nickname:    "internal-service",
		RoleIDs:     []int64{},
		Roles:       []string{"ROLE_INTERNAL"},
		Permissions: []string{},
		Source:      "internal",
		IsAnonymous: false,
	})
	reqCtx.Set("__seven_auth_internal__", true)
	return true
}

func (m *Middleware) verifyTrustedProxy(reqCtx *app.RequestContext) bool {
	if reqCtx == nil {
		return false
	}
	clientIP := strings.TrimSpace(reqCtx.ClientIP())
	if clientIP == "" {
		return false
	}
	for _, item := range m.cfg.Network.TrustedProxies {
		if strings.TrimSpace(item) == clientIP {
			return true
		}
	}
	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}
	for _, cidr := range m.cfg.Network.TrustedCIDRs {
		_, block, err := net.ParseCIDR(strings.TrimSpace(cidr))
		if err == nil && block.Contains(ip) {
			return true
		}
	}
	return false
}

func (m *Middleware) verifyGatewaySignature(ctx context.Context, reqCtx *app.RequestContext, path string) bool {
	if !m.cfg.Gateway.SignatureEnabled {
		return false
	}
	signature := strings.TrimSpace(string(reqCtx.Request.Header.Peek(headerGatewaySignature)))
	timestamp := strings.TrimSpace(string(reqCtx.Request.Header.Peek(headerGatewayTimestamp)))
	nonce := strings.TrimSpace(string(reqCtx.Request.Header.Peek(headerGatewayNonce)))
	version := strings.TrimSpace(string(reqCtx.Request.Header.Peek(headerGatewayVersion)))
	if signature == "" || timestamp == "" || nonce == "" || version == "" {
		return false
	}
	if !contains(m.cfg.Gateway.AcceptedSignatureVersions, version) && !contains([]string{m.cfg.Gateway.SignatureVersion}, version) {
		return false
	}
	secret := strings.TrimSpace(m.cfg.Gateway.SecretsByVersion[version])
	if secret == "" && len(m.cfg.Gateway.SecretsByVersion) > 0 {
		return false
	}
	if secret == "" {
		secret = strings.TrimSpace(m.cfg.Gateway.Secret)
	}
	if secret == "" {
		return false
	}
	requestMillis, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if abs64(time.Now().UTC().UnixMilli()-requestMillis) > maxInt64(m.cfg.Gateway.TimestampToleranceSeconds, 300)*1000 {
		return false
	}
	payload := strings.Join([]string{
		string(reqCtx.Request.Method()),
		canonicalPathWithQuery(path, string(reqCtx.Request.URI().QueryString())),
		version,
		header(reqCtx, headerUserID),
		header(reqCtx, headerUsername),
		header(reqCtx, headerNickname),
		header(reqCtx, headerRoles),
		header(reqCtx, headerPermissions),
		header(reqCtx, headerOrgID),
		header(reqCtx, headerOrgIDs),
		header(reqCtx, headerDeptID),
		header(reqCtx, headerDeptIDs),
		header(reqCtx, headerPostIDs),
		header(reqCtx, headerPostCodes),
		header(reqCtx, headerDataScopeDeptIDs),
		header(reqCtx, headerDataScopeOrgIDs),
		header(reqCtx, headerDataScopeType),
		header(reqCtx, headerIsAdmin),
		header(reqCtx, headerSessionID),
		header(reqCtx, headerAuthVersion),
		header(reqCtx, headerSessionVersion),
		header(reqCtx, headerIssuedAtEpoch),
		header(reqCtx, headerExpireAtEpoch),
		timestamp,
		nonce,
	}, "|")
	expected := signBase64(payload, secret)
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return false
	}
	return m.reserveNonce(ctx, cacheGatewayNoncePrefix+hashHex(nonce), 5*time.Minute)
}

func (m *Middleware) verifyInternalSignature(ctx context.Context, reqCtx *app.RequestContext, path string, internalToken string) bool {
	secret := strings.TrimSpace(m.cfg.Internal.SignatureSecret)
	if secret == "" {
		return false
	}
	timestamp := strings.TrimSpace(string(reqCtx.Request.Header.Peek(headerInternalTimestamp)))
	nonce := strings.TrimSpace(string(reqCtx.Request.Header.Peek(headerInternalNonce)))
	signature := strings.TrimSpace(string(reqCtx.Request.Header.Peek(headerInternalSignature)))
	if timestamp == "" || nonce == "" || signature == "" {
		return false
	}
	minNonceLength, maxNonceLength := internalNonceLengthBounds(m.cfg.Internal)
	if !withinStringLength(nonce, minNonceLength, maxNonceLength) {
		return false
	}
	requestMillis, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if abs64(time.Now().UTC().UnixMilli()-requestMillis) > maxInt64(m.cfg.Internal.TimestampToleranceMs, 300000) {
		return false
	}
	authHash := hashHex(strings.TrimSpace(string(reqCtx.Request.Header.Peek(headerAuthorization))))
	payload := strings.Join([]string{
		string(reqCtx.Request.Method()),
		path,
		timestamp,
		nonce,
		authHash,
		internalToken,
	}, "\n")
	expected := signBase64(payload, secret)
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return false
	}
	return m.reserveNonce(ctx, cacheInternalNoncePrefix+hashHex(nonce), time.Duration(maxIntInt(m.cfg.Internal.NonceTTLSeconds, 300))*time.Second)
}

func (m *Middleware) reserveNonce(ctx context.Context, cacheKey string, ttl time.Duration) bool {
	if m.cache == nil {
		return false
	}
	ok, err := m.cache.SetNXString(ctx, cacheKey, "1", ttl)
	return err == nil && ok
}

func withinStringLength(value string, minLength int, maxLength int) bool {
	length := len(value)
	return length >= minLength && length <= maxLength
}

func internalNonceLengthBounds(cfg config.AuthorizationInternalConfig) (int, int) {
	minLength := cfg.NonceMinLength
	if minLength <= 0 {
		minLength = 8
	}
	maxLength := cfg.NonceMaxLength
	if maxLength <= 0 {
		maxLength = 64
	}
	if maxLength < minLength {
		maxLength = minLength
	}
	return minLength, maxLength
}

func extractGatewayUserContext(reqCtx *app.RequestContext) *securitycontext.UserContext {
	userID, _ := strconv.ParseInt(header(reqCtx, headerUserID), 10, 64)
	primaryOrgID, _ := strconv.ParseInt(header(reqCtx, headerOrgID), 10, 64)
	primaryDeptID, _ := strconv.ParseInt(header(reqCtx, headerDeptID), 10, 64)
	authVersion, _ := strconv.ParseInt(header(reqCtx, headerAuthVersion), 10, 64)
	sessionVersion, _ := strconv.ParseInt(header(reqCtx, headerSessionVersion), 10, 64)
	issuedAt, _ := strconv.ParseInt(header(reqCtx, headerIssuedAtEpoch), 10, 64)
	expireAt, _ := strconv.ParseInt(header(reqCtx, headerExpireAtEpoch), 10, 64)
	return &securitycontext.UserContext{
		UserID:           userID,
		Username:         header(reqCtx, headerUsername),
		Nickname:         header(reqCtx, headerNickname),
		RoleIDs:          []int64{},
		Roles:            splitCSV(header(reqCtx, headerRoles)),
		Permissions:      splitCSV(header(reqCtx, headerPermissions)),
		PrimaryOrgID:     primaryOrgID,
		OrgIDs:           splitInt64CSV(header(reqCtx, headerOrgIDs)),
		PrimaryDeptID:    primaryDeptID,
		DeptIDs:          splitInt64CSV(header(reqCtx, headerDeptIDs)),
		PostIDs:          splitInt64CSV(header(reqCtx, headerPostIDs)),
		PostCodes:        splitCSV(header(reqCtx, headerPostCodes)),
		DataScopeDeptIDs: splitInt64CSV(header(reqCtx, headerDataScopeDeptIDs)),
		DataScopeOrgIDs:  splitInt64CSV(header(reqCtx, headerDataScopeOrgIDs)),
		DataScopeType:    securitycontext.DataScopeType(header(reqCtx, headerDataScopeType)),
		SessionID:        header(reqCtx, headerSessionID),
		AuthVersion:      authVersion,
		SessionVersion:   sessionVersion,
		IssuedAtEpoch:    issuedAt,
		ExpireAtEpoch:    expireAt,
		Source:           "gateway",
		IsAdmin:          strings.EqualFold(header(reqCtx, headerIsAdmin), "true") || header(reqCtx, headerIsAdmin) == "1",
		IsAnonymous:      false,
	}
}

func extractBearer(reqCtx *app.RequestContext) string {
	headerValue := strings.TrimSpace(string(reqCtx.Request.Header.Peek(headerAuthorization)))
	if strings.HasPrefix(strings.ToLower(headerValue), "bearer ") {
		return strings.TrimSpace(headerValue[7:])
	}
	return ""
}

func header(reqCtx *app.RequestContext, key string) string {
	return strings.TrimSpace(string(reqCtx.Request.Header.Peek(key)))
}

func readCookie(reqCtx *app.RequestContext, name string) string {
	if reqCtx == nil || strings.TrimSpace(name) == "" {
		return ""
	}
	return strings.TrimSpace(string(reqCtx.Request.Header.Cookie(name)))
}

func matchPathPattern(pattern, value string) bool {
	pattern = strings.TrimSpace(pattern)
	value = strings.TrimSpace(value)
	if pattern == "" || value == "" {
		return false
	}
	if pattern == value {
		return true
	}
	if strings.Contains(pattern, "/:projectId/") {
		parts := strings.Split(pattern, "/:projectId/")
		if len(parts) != 2 {
			return false
		}
		prefix, suffix := parts[0]+"/", "/"+parts[1]
		if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, suffix) {
			return false
		}
		idText := strings.TrimSuffix(strings.TrimPrefix(value, prefix), suffix)
		id, err := strconv.ParseInt(idText, 10, 64)
		return err == nil && id > 0 && strconv.FormatInt(id, 10) == idText
	}
	if strings.HasSuffix(pattern, "/:id") {
		prefix := strings.TrimSuffix(pattern, ":id")
		if !strings.HasPrefix(value, prefix) {
			return false
		}
		idText := strings.TrimPrefix(value, prefix)
		if idText == "" || strings.Contains(idText, "/") {
			return false
		}
		id, err := strconv.ParseInt(idText, 10, 64)
		return err == nil && id > 0 && strconv.FormatInt(id, 10) == idText
	}
	if strings.HasSuffix(pattern, "/**") {
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "**"))
	}
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(value, prefix) && !strings.Contains(strings.TrimPrefix(value, prefix), "/")
	}
	return false
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, item := range parts {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	sort.Strings(result)
	return result
}

func splitInt64CSV(value string) []int64 {
	items := splitCSV(value)
	result := make([]int64, 0, len(items))
	for _, item := range items {
		parsed, err := strconv.ParseInt(item, 10, 64)
		if err == nil && parsed > 0 {
			result = append(result, parsed)
		}
	}
	return result
}

func canonicalPathWithQuery(path, rawQuery string) string {
	normalizedPath := strings.TrimSpace(path)
	if normalizedPath == "" {
		normalizedPath = "/"
	}
	if !strings.HasPrefix(normalizedPath, "/") {
		normalizedPath = "/" + normalizedPath
	}
	if strings.TrimSpace(rawQuery) == "" {
		return normalizedPath
	}
	parts := strings.Split(rawQuery, "&")
	pairs := make([][2]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		item := strings.SplitN(part, "=", 2)
		key, _ := neturl.QueryUnescape(item[0])
		value := ""
		if len(item) == 2 {
			value, _ = neturl.QueryUnescape(item[1])
		}
		pairs = append(pairs, [2]string{key, value})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i][0] == pairs[j][0] {
			return pairs[i][1] < pairs[j][1]
		}
		return pairs[i][0] < pairs[j][0]
	})
	encoded := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		encoded = append(encoded, rfc3986Encode(pair[0])+"="+rfc3986Encode(pair[1]))
	}
	return normalizedPath + "?" + strings.Join(encoded, "&")
}

func rfc3986Encode(value string) string {
	return strings.NewReplacer("+", "%20", "*", "%2A", "%7E", "~").Replace(neturl.QueryEscape(value))
}

func signBase64(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func hashHex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func contains(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, item := range values {
		if strings.TrimSpace(item) == target {
			return true
		}
	}
	return false
}

func abs64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func maxInt64(value, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}

func maxIntInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
