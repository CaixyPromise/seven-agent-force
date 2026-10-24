package infrastructure

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
)

const (
	feishuTokenURL       = "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal"
	feishuMessageURL     = "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=open_id"
	feishuChatMessageURL = "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=chat_id"
	weComTokenURL        = "https://qyapi.weixin.qq.com/cgi-bin/gettoken"
	weComMessageURL      = "https://qyapi.weixin.qq.com/cgi-bin/message/send"

	providerResponseLimit = 64 << 10
	tokenRefreshSkew      = time.Minute
	tokenRefreshJitterMax = 15 * time.Second
)

type enterpriseHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// enterpriseApplicationDriver implements the two G4 direct application
// protocols. Its client is injected by the registry so production can only
// use an OutboundURLGuard-backed transport; it never creates an HTTP client.
type enterpriseApplicationDriver struct {
	provider string
	client   enterpriseHTTPDoer
	tokens   *enterpriseTokenCache
}

type enterpriseTokenCache struct {
	mu     sync.Mutex
	values map[string]enterpriseToken
	group  singleflight.Group
	now    func() time.Time
	jitter func(time.Duration) time.Duration
}

type enterpriseToken struct {
	value     string
	expiresAt time.Time
}

func newEnterpriseTokenCache() *enterpriseTokenCache {
	return &enterpriseTokenCache{
		values: make(map[string]enterpriseToken),
		now:    time.Now,
		jitter: func(max time.Duration) time.Duration {
			if max <= 0 {
				return 0
			}
			return time.Duration(rand.Int64N(int64(max) + 1))
		},
	}
}

func (d *enterpriseApplicationDriver) Send(ctx context.Context, message domain.DriverMessage) error {
	result, err := d.SendResult(ctx, message)
	if err != nil {
		return err
	}
	if result.Status == domain.DriverResultProviderAccepted {
		return nil
	}
	return fmt.Errorf("enterprise provider result: %s", result.Diagnostic)
}

func (d *enterpriseApplicationDriver) SendResult(ctx context.Context, message domain.DriverMessage) (domain.DriverResult, error) {
	if d == nil || d.client == nil || d.tokens == nil {
		return providerFailure("DRIVER_UNAVAILABLE", "DRIVER_UNAVAILABLE", false), nil
	}
	if strings.TrimSpace(message.Target) == "" || strings.TrimSpace(message.SecretPlain) == "" {
		return providerFailure("INVALID_TARGET", "INVALID_TARGET", false), nil
	}
	provider := strings.ToUpper(strings.TrimSpace(d.provider))
	identityKind := strings.ToUpper(strings.TrimSpace(message.IdentityKind))
	if identityKind == "" {
		// The legacy V1 driver path does not carry target identity. Preserve its
		// direct-member behavior; semantic G4 calls always supply the kind.
		identityKind = domain.ExpectedExternalIdentityKind(provider)
	}
	if !domain.SupportsEnterpriseApplicationIdentityKind(provider, identityKind) {
		return providerFailure("INVALID_TARGET", "INVALID_TARGET", false), nil
	}
	target, err := domain.NormalizeEnterpriseApplicationTarget(provider, identityKind, message.Target)
	if err != nil {
		return providerFailure("INVALID_TARGET", "INVALID_TARGET", false), nil
	}
	message.IdentityKind = identityKind
	message.Target = target
	message.ProviderParams = domain.SanitizeProviderParameterSnapshot(provider, message.ProviderParams)
	switch provider {
	case domain.ChannelTypeFeishuApp:
		return d.sendFeishu(ctx, message)
	case domain.ChannelTypeWeComApp:
		return d.sendWeCom(ctx, message)
	default:
		return providerFailure("CONFIGURATION", "UNSUPPORTED_PROVIDER", false), nil
	}
}

func (d *enterpriseApplicationDriver) sendFeishu(ctx context.Context, message domain.DriverMessage) (domain.DriverResult, error) {
	config, err := domain.ParseEnterpriseApplicationConfig(domain.ChannelTypeFeishuApp, message.Channel.ConfigJSON)
	if err != nil {
		return providerFailure("CONFIGURATION", "CONFIGURATION", false), nil
	}
	key := enterpriseTokenKey(message.Channel, domain.ChannelTypeFeishuApp, config, message.SecretPlain)
	token, tokenResult := d.getFeishuToken(ctx, key, config, message.SecretPlain)
	if tokenResult != nil {
		return *tokenResult, nil
	}
	result := d.sendFeishuWithToken(ctx, token, message)
	if !isFeishuTokenRejected(result) {
		return result, nil
	}
	d.tokens.invalidateRejected(key, token)
	token, refreshResult := d.getFeishuToken(ctx, key, config, message.SecretPlain)
	if refreshResult != nil {
		return *refreshResult, nil
	}
	result = d.sendFeishuWithToken(ctx, token, message)
	if isFeishuTokenRejected(result) {
		return providerFailureWithError("AUTHENTICATION", "AUTHENTICATION", false, result.ProviderError), nil
	}
	return result, nil
}

func (d *enterpriseApplicationDriver) getFeishuToken(ctx context.Context, key string, config domain.EnterpriseApplicationConfig, secret string) (string, *domain.DriverResult) {
	value, err, _ := d.tokens.get(ctx, key, func(ctx context.Context) (enterpriseToken, error) {
		response, requestErr := d.doJSON(ctx, http.MethodPost, feishuTokenURL, map[string]any{"app_id": config.AppID, "app_secret": secret}, nil)
		if requestErr != nil {
			return enterpriseToken{}, tokenFetchError{result: providerFailureWithError("TRANSIENT", "TOKEN_TRANSPORT", true, providerTransportError(domain.ChannelTypeFeishuApp, requestErr))}
		}
		payload, decoded := decodeFeishuResponse(response.body)
		providerError := feishuProviderError(response.statusCode, payload, decoded)
		if response.statusCode == http.StatusTooManyRequests {
			return enterpriseToken{}, tokenFetchError{result: providerFailureWithError("THROTTLED", "TOKEN_THROTTLED", true, providerError)}
		}
		if response.statusCode >= http.StatusInternalServerError {
			return enterpriseToken{}, tokenFetchError{result: providerFailureWithError("TRANSIENT", "TOKEN_SERVER", true, providerError)}
		}
		if response.statusCode < http.StatusOK || response.statusCode >= http.StatusMultipleChoices {
			return enterpriseToken{}, tokenFetchError{result: providerFailureWithError("AUTHENTICATION", "TOKEN_REJECTED", false, providerError)}
		}
		if !decoded || payload.Code != 0 || strings.TrimSpace(payload.TenantAccessToken) == "" {
			return enterpriseToken{}, tokenFetchError{result: providerFailureWithError("AUTHENTICATION", "TOKEN_REJECTED", false, providerError)}
		}
		return enterpriseToken{value: payload.TenantAccessToken, expiresAt: d.tokens.expiry(payload.Expire)}, nil
	})
	if err != nil {
		if typed, ok := err.(tokenFetchError); ok {
			return "", &typed.result
		}
		return "", pointerResult(providerFailure("TRANSIENT", "TOKEN_UNAVAILABLE", true))
	}
	return value.value, nil
}

func (d *enterpriseApplicationDriver) sendFeishuWithToken(ctx context.Context, token string, message domain.DriverMessage) domain.DriverResult {
	messageURL := feishuMessageURLForIdentityKind(message.IdentityKind)
	if messageURL == "" {
		return providerFailure("INVALID_TARGET", "INVALID_TARGET", false)
	}
	msgType := "text"
	contentValue := any(map[string]string{"text": externalTextWithAction(message)})
	if strings.EqualFold(message.Presentation, domain.DeliveryPresentationCard) {
		msgType = "interactive"
		contentValue = feishuInteractiveCard(message)
	}
	content, _ := json.Marshal(contentValue)
	response, err := d.doJSON(ctx, http.MethodPost, messageURL, map[string]any{
		"receive_id": message.Target,
		"msg_type":   msgType,
		"content":    string(content),
		"uuid":       providerUUID(message.DeliveryID),
	}, map[string]string{"Authorization": "Bearer " + token})
	if err != nil {
		// Feishu documents a bounded duplicate window for a reused UUID. The
		// same delivery ID supplies that UUID, so this is eligible for retry.
		return providerFailureWithError("TRANSIENT", "FEISHU_REQUEST_UNCONFIRMED", true, providerTransportError(domain.ChannelTypeFeishuApp, err))
	}
	payload, decoded := decodeFeishuResponse(response.body)
	providerError := feishuProviderError(response.statusCode, payload, decoded)
	if response.statusCode == http.StatusTooManyRequests {
		return providerFailureWithError("THROTTLED", "THROTTLED", true, providerError)
	}
	if response.statusCode >= http.StatusInternalServerError {
		return providerFailureWithError("TRANSIENT", "FEISHU_SERVER", true, providerError)
	}
	if response.statusCode < http.StatusOK || response.statusCode >= http.StatusMultipleChoices {
		return providerFailureWithError("AUTHENTICATION", "FEISHU_REJECTED", false, providerError)
	}
	if !decoded {
		return providerUnknownWithError("RESPONSE_INVALID", providerError)
	}
	if payload.Code == 0 {
		return providerAccepted(payload.Data.MessageID)
	}
	if isFeishuTokenCode(payload.Code) {
		return providerFailureWithError("AUTHENTICATION", "TOKEN_REJECTED", false, providerError)
	}
	if isFeishuInvalidTargetCode(payload.Code) {
		return providerFailureWithError("INVALID_TARGET", "INVALID_TARGET", false, providerError)
	}
	return providerFailureWithError("PROVIDER_REJECTED", "FEISHU_REJECTED", false, providerError)
}

func feishuInteractiveCard(message domain.DriverMessage) map[string]any {
	elements := []any{map[string]any{"tag": "markdown", "content": strings.TrimSpace(message.Text)}}
	if mention := feishuMentionMarkdown(message.Mention); mention != "" {
		elements = append(elements, map[string]any{"tag": "markdown", "content": mention})
	}
	if message.PrimaryAction != nil && strings.TrimSpace(message.ExternalActionURL) != "" {
		elements = append(elements, map[string]any{"tag": "action", "actions": []any{map[string]any{
			"tag": "button", "text": map[string]string{"tag": "plain_text", "content": message.PrimaryAction.Label},
			"type": "primary", "url": message.ExternalActionURL,
		}}})
	}
	return map[string]any{
		"config":   map[string]any{"wide_screen_mode": true},
		"header":   map[string]any{"template": "blue", "title": map[string]string{"tag": "plain_text", "content": strings.TrimSpace(message.Subject)}},
		"elements": elements,
	}
}

func feishuMentionMarkdown(mention *domain.DriverMention) string {
	if mention == nil || !strings.EqualFold(mention.Provider, "FEISHU") {
		return ""
	}
	subject := strings.TrimSpace(mention.Subject)
	if subject == "" || strings.ContainsAny(subject, "<>\"'`\\\r\n\t ") {
		return ""
	}
	return "<at id=" + subject + "></at>"
}

func feishuMessageURLForIdentityKind(identityKind string) string {
	switch strings.ToUpper(strings.TrimSpace(identityKind)) {
	case domain.ExternalIdentityFeishuOpenID:
		return feishuMessageURL
	case domain.ExternalIdentityFeishuChatID:
		return feishuChatMessageURL
	default:
		return ""
	}
}

func (d *enterpriseApplicationDriver) sendWeCom(ctx context.Context, message domain.DriverMessage) (domain.DriverResult, error) {
	config, err := domain.ParseEnterpriseApplicationConfig(domain.ChannelTypeWeComApp, message.Channel.ConfigJSON)
	if err != nil {
		return providerFailure("CONFIGURATION", "CONFIGURATION", false), nil
	}
	key := enterpriseTokenKey(message.Channel, domain.ChannelTypeWeComApp, config, message.SecretPlain)
	token, tokenResult := d.getWeComToken(ctx, key, config, message.SecretPlain)
	if tokenResult != nil {
		return *tokenResult, nil
	}
	result := d.sendWeComWithToken(ctx, token, config, message)
	if !isWeComTokenRejected(result) {
		return result, nil
	}
	d.tokens.invalidateRejected(key, token)
	token, refreshResult := d.getWeComToken(ctx, key, config, message.SecretPlain)
	if refreshResult != nil {
		return *refreshResult, nil
	}
	result = d.sendWeComWithToken(ctx, token, config, message)
	if isWeComTokenRejected(result) {
		return providerFailureWithError("AUTHENTICATION", "AUTHENTICATION", false, result.ProviderError), nil
	}
	return result, nil
}

func (d *enterpriseApplicationDriver) getWeComToken(ctx context.Context, key string, config domain.EnterpriseApplicationConfig, secret string) (string, *domain.DriverResult) {
	value, err, _ := d.tokens.get(ctx, key, func(ctx context.Context) (enterpriseToken, error) {
		endpoint, _ := url.Parse(weComTokenURL)
		query := endpoint.Query()
		query.Set("corpid", config.CorpID)
		query.Set("corpsecret", secret)
		endpoint.RawQuery = query.Encode()
		response, requestErr := d.doJSON(ctx, http.MethodGet, endpoint.String(), nil, nil)
		if requestErr != nil {
			return enterpriseToken{}, tokenFetchError{result: providerFailureWithError("TRANSIENT", "TOKEN_TRANSPORT", true, providerTransportError(domain.ChannelTypeWeComApp, requestErr))}
		}
		payload, decoded := decodeWeComResponse(response.body)
		providerError := weComProviderError(response.statusCode, payload, decoded)
		if response.statusCode == http.StatusTooManyRequests {
			return enterpriseToken{}, tokenFetchError{result: providerFailureWithError("THROTTLED", "TOKEN_THROTTLED", true, providerError)}
		}
		if response.statusCode >= http.StatusInternalServerError {
			return enterpriseToken{}, tokenFetchError{result: providerFailureWithError("TRANSIENT", "TOKEN_SERVER", true, providerError)}
		}
		if response.statusCode < http.StatusOK || response.statusCode >= http.StatusMultipleChoices {
			return enterpriseToken{}, tokenFetchError{result: providerFailureWithError("AUTHENTICATION", "TOKEN_REJECTED", false, providerError)}
		}
		if !decoded || payload.ErrCode != 0 || strings.TrimSpace(payload.AccessToken) == "" {
			return enterpriseToken{}, tokenFetchError{result: providerFailureWithError("AUTHENTICATION", "TOKEN_REJECTED", false, providerError)}
		}
		return enterpriseToken{value: payload.AccessToken, expiresAt: d.tokens.expiry(payload.ExpiresInSec)}, nil
	})
	if err != nil {
		if typed, ok := err.(tokenFetchError); ok {
			return "", &typed.result
		}
		return "", pointerResult(providerFailure("TRANSIENT", "TOKEN_UNAVAILABLE", true))
	}
	return value.value, nil
}

func (d *enterpriseApplicationDriver) sendWeComWithToken(ctx context.Context, token string, config domain.EnterpriseApplicationConfig, message domain.DriverMessage) domain.DriverResult {
	agentID, err := strconv.ParseInt(config.AgentID, 10, 64)
	if err != nil || agentID <= 0 {
		return providerFailure("CONFIGURATION", "CONFIGURATION", false)
	}
	endpoint, _ := url.Parse(weComMessageURL)
	query := endpoint.Query()
	query.Set("access_token", token)
	endpoint.RawQuery = query.Encode()
	text := map[string]any{"content": externalTextWithAction(message)}
	if mentioned, ok := message.ProviderParams[domain.ProviderParameterMentionedList]; ok {
		text["mentioned_list"] = mentioned
	}
	requestPayload := map[string]any{"touser": message.Target, "msgtype": "text", "agentid": agentID, "text": text}
	if strings.EqualFold(message.Presentation, domain.DeliveryPresentationCard) {
		card := map[string]any{
			"card_type":  "text_notice",
			"source":     map[string]any{"desc": "Seven Agent Force"},
			"main_title": map[string]any{"title": strings.TrimSpace(message.Subject), "desc": strings.TrimSpace(message.Text)},
		}
		if message.PrimaryAction != nil && strings.TrimSpace(message.ExternalActionURL) != "" {
			card["card_action"] = map[string]any{"type": 1, "url": message.ExternalActionURL}
		}
		requestPayload["msgtype"] = "template_card"
		requestPayload["template_card"] = card
		delete(requestPayload, "text")
	}
	response, err := d.doJSON(ctx, http.MethodPost, endpoint.String(), requestPayload, nil)
	if err != nil {
		// WeCom has no G4 request idempotency contract, so uncertain request
		// reachability must not be turned into an automatic resend.
		return providerUnknownWithError("WECOM_REQUEST_UNCONFIRMED", providerTransportError(domain.ChannelTypeWeComApp, err))
	}
	payload, decoded := decodeWeComResponse(response.body)
	providerError := weComProviderError(response.statusCode, payload, decoded)
	if response.statusCode == http.StatusTooManyRequests {
		return providerFailureWithError("THROTTLED", "THROTTLED", true, providerError)
	}
	if response.statusCode >= http.StatusInternalServerError {
		// A concrete 5xx response is different from transport response loss: the
		// provider rejected this request at its server boundary, so the same
		// frozen delivery may be retried. Missing/ambiguous responses remain
		// OUTCOME_UNKNOWN in the transport-error branch above.
		return providerFailureWithError("TRANSIENT", "WECOM_SERVER", true, providerError)
	}
	if response.statusCode < http.StatusOK || response.statusCode >= http.StatusMultipleChoices {
		return providerFailureWithError("AUTHENTICATION", "WECOM_REJECTED", false, providerError)
	}
	if !decoded {
		return providerUnknownWithError("RESPONSE_INVALID", providerError)
	}
	if payload.ErrCode == 0 && strings.TrimSpace(payload.InvalidUser) == "" && strings.TrimSpace(payload.UnlicensedUser) == "" {
		return providerAccepted(payload.MsgID)
	}
	if isWeComTokenCode(payload.ErrCode) {
		return providerFailureWithError("AUTHENTICATION", "TOKEN_REJECTED", false, providerError)
	}
	if payload.InvalidUser != "" || payload.UnlicensedUser != "" || isWeComInvalidTargetCode(payload.ErrCode) {
		return providerFailureWithError("INVALID_TARGET", "INVALID_TARGET", false, providerError)
	}
	if isWeComThrottleCode(payload.ErrCode) {
		return providerFailureWithError("THROTTLED", "THROTTLED", true, providerError)
	}
	if isWeComTransientCode(payload.ErrCode) {
		return providerFailureWithError("TRANSIENT", "WECOM_TRANSIENT", true, providerError)
	}
	return providerFailureWithError("PROVIDER_REJECTED", "WECOM_REJECTED", false, providerError)
}

type providerHTTPResponse struct {
	statusCode int
	body       []byte
}

type feishuResponsePayload struct {
	Code  int    `json:"code"`
	Msg   string `json:"msg"`
	Error struct {
		LogID string `json:"log_id"`
	} `json:"error"`
	Data struct {
		MessageID string `json:"message_id"`
	} `json:"data"`
	TenantAccessToken string `json:"tenant_access_token"`
	Expire            int    `json:"expire"`
}

type weComResponsePayload struct {
	ErrCode        int    `json:"errcode"`
	ErrMsg         string `json:"errmsg"`
	AccessToken    string `json:"access_token"`
	ExpiresInSec   int    `json:"expires_in"`
	MsgID          string `json:"msgid"`
	InvalidUser    string `json:"invaliduser"`
	UnlicensedUser string `json:"unlicenseduser"`
}

func decodeFeishuResponse(body []byte) (feishuResponsePayload, bool) {
	var payload feishuResponsePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return feishuResponsePayload{}, false
	}
	return payload, true
}

func decodeWeComResponse(body []byte) (weComResponsePayload, bool) {
	var payload weComResponsePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return weComResponsePayload{}, false
	}
	return payload, true
}

func feishuProviderError(status int, payload feishuResponsePayload, decoded bool) *domain.ProviderError {
	if decoded && payload.Code == 0 && status >= http.StatusOK && status < http.StatusMultipleChoices {
		return nil
	}
	detail := &domain.ProviderError{Provider: domain.ChannelTypeFeishuApp, HTTPStatus: status}
	if decoded {
		if payload.Code != 0 {
			detail.Code = strconv.Itoa(payload.Code)
		}
		detail.Message = payload.Msg
		detail.LogID = payload.Error.LogID
	}
	return detail
}

func weComProviderError(status int, payload weComResponsePayload, decoded bool) *domain.ProviderError {
	if decoded && payload.ErrCode == 0 && status >= http.StatusOK && status < http.StatusMultipleChoices {
		return nil
	}
	detail := &domain.ProviderError{Provider: domain.ChannelTypeWeComApp, HTTPStatus: status}
	if decoded {
		if payload.ErrCode != 0 {
			detail.Code = strconv.Itoa(payload.ErrCode)
		}
		detail.Message = payload.ErrMsg
	}
	return detail
}

func providerTransportError(provider string, err error) *domain.ProviderError {
	if err == nil {
		return nil
	}
	return &domain.ProviderError{Provider: provider, Message: err.Error()}
}

func (d *enterpriseApplicationDriver) doJSON(ctx context.Context, method, endpoint string, payload any, headers map[string]string) (providerHTTPResponse, error) {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return providerHTTPResponse{}, err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return providerHTTPResponse{}, err
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Accept", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := d.client.Do(request)
	if err != nil {
		return providerHTTPResponse{}, err
	}
	defer response.Body.Close()
	limited, readErr := io.ReadAll(io.LimitReader(response.Body, providerResponseLimit))
	if readErr != nil {
		return providerHTTPResponse{}, readErr
	}
	return providerHTTPResponse{statusCode: response.StatusCode, body: limited}, nil
}

func (c *enterpriseTokenCache) get(ctx context.Context, key string, load func(context.Context) (enterpriseToken, error)) (enterpriseToken, error, bool) {
	if c == nil {
		return enterpriseToken{}, fmt.Errorf("enterprise token cache is nil"), false
	}
	if current, ok := c.current(key); ok {
		return current, nil, false
	}
	value, err, shared := c.group.Do(key, func() (any, error) {
		if current, ok := c.current(key); ok {
			return current, nil
		}
		loaded, err := load(ctx)
		if err != nil {
			return enterpriseToken{}, err
		}
		if strings.TrimSpace(loaded.value) == "" || !loaded.expiresAt.After(c.clock()) {
			return enterpriseToken{}, fmt.Errorf("enterprise token result is invalid")
		}
		c.mu.Lock()
		c.values[key] = loaded
		c.mu.Unlock()
		return loaded, nil
	})
	if err != nil {
		return enterpriseToken{}, err, shared
	}
	token, ok := value.(enterpriseToken)
	if !ok {
		return enterpriseToken{}, fmt.Errorf("enterprise token cache result is invalid"), shared
	}
	return token, nil, shared
}

func (c *enterpriseTokenCache) current(key string) (enterpriseToken, bool) {
	if c == nil {
		return enterpriseToken{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.values[key]
	return value, ok && strings.TrimSpace(value.value) != "" && value.expiresAt.After(c.clock())
}

// invalidateRejected removes a token only when it is still the exact token
// rejected by the provider. Concurrent callers that receive the same rejection
// must join one forced refresh rather than deleting the fresh replacement.
func (c *enterpriseTokenCache) invalidateRejected(key, rejected string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if current, exists := c.values[key]; exists && current.value == rejected {
		delete(c.values, key)
	}
	c.mu.Unlock()
}

func (c *enterpriseTokenCache) expiry(seconds int) time.Time {
	if seconds <= 0 {
		seconds = 300
	}
	ttl := time.Duration(seconds) * time.Second
	early := tokenRefreshSkew
	if ttl <= early {
		early = ttl / 2
	}
	// A small per-process random component prevents similarly started nodes
	// from refreshing the same token in lockstep, while retaining a stable
	// safety margin before provider expiry.
	jitterMax := minDuration(tokenRefreshJitterMax, early/4)
	if c != nil && c.jitter != nil {
		jitter := c.jitter(jitterMax)
		if jitter < 0 {
			jitter = 0
		}
		if jitter > jitterMax {
			jitter = jitterMax
		}
		early += jitter
	}
	if early >= ttl {
		early = ttl / 2
	}
	return c.clock().Add(ttl - early)
}

func minDuration(left, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
}

func (c *enterpriseTokenCache) clock() time.Time {
	if c != nil && c.now != nil {
		return c.now()
	}
	return time.Now()
}

type tokenFetchError struct{ result domain.DriverResult }

func (e tokenFetchError) Error() string { return e.result.Diagnostic }

func providerAccepted(reference string) domain.DriverResult {
	return domain.DriverResult{Status: domain.DriverResultProviderAccepted, ProviderReference: reference, Diagnostic: "PROVIDER_ACCEPTED"}
}

func providerFailure(classification, diagnostic string, retryable bool) domain.DriverResult {
	return domain.DriverResult{Status: domain.DriverResultFailed, FailureClass: classification, Diagnostic: diagnostic, Retryable: retryable}
}

func providerFailureWithError(classification, diagnostic string, retryable bool, providerError *domain.ProviderError) domain.DriverResult {
	diagnostic = providerDiagnostic(diagnostic, providerError)
	result := providerFailure(classification, diagnostic, retryable)
	result.ProviderError = domain.SanitizeProviderError(providerError)
	return result
}

// providerDiagnostic carries only a provider's stable, sanitized numeric code
// into a terminal generic-rejection reason.  The delivery record deliberately
// does not retain provider prose, targets, request bodies, or log IDs, but an
// operator still needs the error class needed to follow the provider's setup
// guidance.  Specific classifications such as INVALID_TARGET and TOKEN_REJECTED
// already identify the remediation category and remain stable across releases.
func providerDiagnostic(diagnostic string, providerError *domain.ProviderError) string {
	if diagnostic != "WECOM_REJECTED" || providerError == nil {
		return diagnostic
	}
	code := strings.TrimSpace(providerError.Code)
	if code == "" {
		return diagnostic
	}
	for _, character := range code {
		if character < '0' || character > '9' {
			return diagnostic
		}
	}
	return diagnostic + "_" + code
}

func providerUnknown(diagnostic string) domain.DriverResult {
	return domain.DriverResult{Status: domain.DriverResultUnknown, FailureClass: "AMBIGUOUS", Diagnostic: diagnostic}
}

func providerUnknownWithError(diagnostic string, providerError *domain.ProviderError) domain.DriverResult {
	result := providerUnknown(diagnostic)
	result.ProviderError = domain.SanitizeProviderError(providerError)
	return result
}

func pointerResult(result domain.DriverResult) *domain.DriverResult { return &result }

func enterpriseTokenKey(channel domain.Channel, provider string, config domain.EnterpriseApplicationConfig, secret string) string {
	revision := sha256.Sum256([]byte(strings.Join([]string{channel.ScopeID, channel.ChannelCode, provider, config.AppID, config.CorpID, config.AgentID, channel.SecretWrapKeyRef, secret}, "\x00")))
	return provider + ":" + hex.EncodeToString(revision[:])
}

func providerUUID(deliveryID string) string {
	value := strings.TrimSpace(deliveryID)
	if len(value) > 50 {
		value = value[:50]
	}
	return value
}

func isFeishuTokenRejected(result domain.DriverResult) bool {
	return result.Diagnostic == "TOKEN_REJECTED"
}

func isWeComTokenRejected(result domain.DriverResult) bool {
	return result.Diagnostic == "TOKEN_REJECTED"
}

func isFeishuTokenCode(code int) bool {
	return code == 99991663 || code == 99991661
}

func isFeishuInvalidTargetCode(code int) bool {
	return code == 230001 || code == 230002 || code == 230003
}

func isWeComTokenCode(code int) bool {
	return code == 40014 || code == 42001 || code == 40082
}

func isWeComInvalidTargetCode(code int) bool {
	return code == 60111 || code == 60112 || code == 81013
}

func isWeComThrottleCode(code int) bool {
	return code == 45009 || code == 45011 || code == 45022
}

func isWeComTransientCode(code int) bool {
	return code == -1 || code == 45002
}
