package infrastructure

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/outboundurl"
)

const (
	staticHTTPResponseLimit = 64 << 10
	staticHTTPRequestLimit  = 64 << 10
)

// staticHTTPClientFactory creates a request client from a runtime-owned egress
// policy. Production wiring supplies OutboundURLGuard-backed clients; tests
// may inject one fake receiver without creating a real network request.
type staticHTTPClientFactory func(outboundurl.Policy) enterpriseHTTPDoer

// staticHTTPDriver implements the generic connector and the two compiled
// group profiles. It owns request construction, so business callers cannot
// select URLs, recipients, headers, authentication, or raw payload fragments.
type staticHTTPDriver struct {
	channelType string
	clients     staticHTTPClientFactory
	policies    outboundurl.PolicyResolver
	now         func() time.Time
}

func (d *staticHTTPDriver) Send(ctx context.Context, message domain.DriverMessage) error {
	result, err := d.SendResult(ctx, message)
	if err != nil {
		return err
	}
	if result.Status == domain.DriverResultProviderAccepted {
		return nil
	}
	return fmt.Errorf("static HTTP connector result: %s", result.Diagnostic)
}

func (d *staticHTTPDriver) SendResult(ctx context.Context, message domain.DriverMessage) (domain.DriverResult, error) {
	if d == nil || d.clients == nil {
		return providerFailure("DRIVER_UNAVAILABLE", "DRIVER_UNAVAILABLE", false), nil
	}
	channelType := strings.ToUpper(strings.TrimSpace(d.channelType))
	if channelType == "" {
		channelType = strings.ToUpper(strings.TrimSpace(message.Channel.ChannelType))
	}
	if !domain.IsStaticHTTPChannelType(channelType) {
		return providerFailure("CONFIGURATION", "UNSUPPORTED_PROVIDER", false), nil
	}
	if strings.TrimSpace(message.Target) != "" || strings.TrimSpace(message.IdentityKind) != "" || len(message.ProviderParams) != 0 {
		return providerFailure("CALLER_BOUNDARY", "STATIC_TARGET_OVERRIDE", false), nil
	}
	switch channelType {
	case domain.ChannelTypeHTTPConnector:
		return d.sendHTTPConnector(ctx, message)
	case domain.ChannelTypeFeishuWebhook, domain.ChannelTypeWeComWebhook:
		return d.sendWebhookProfile(ctx, channelType, message)
	default:
		return providerFailure("CONFIGURATION", "UNSUPPORTED_PROVIDER", false), nil
	}
}

func (d *staticHTTPDriver) sendHTTPConnector(ctx context.Context, message domain.DriverMessage) (domain.DriverResult, error) {
	config, err := domain.ParseHTTPConnectorConfig(message.Channel.ConfigJSON)
	if err != nil {
		return providerFailure("CONFIGURATION", "CONFIGURATION", false), nil
	}
	policy, err := resolveHTTPConnectorPolicy(config.EgressPolicyRef, d.policies)
	if err != nil {
		return providerFailure("CONFIGURATION", "EGRESS_POLICY", false), nil
	}
	body, err := buildHTTPConnectorBody(config, message)
	if err != nil {
		return providerFailure("CONFIGURATION", "PAYLOAD", false), nil
	}
	request, cancel, err := d.newJSONRequest(ctx, config.TimeoutMilliseconds, config.EndpointURL, body)
	if err != nil {
		return providerFailure("CONFIGURATION", "REQUEST", false), nil
	}
	defer cancel()
	applyHTTPConnectorHeaders(request, config, message)
	if err := applyHTTPConnectorAuthentication(request, config, message.SecretPlain, d.clock()); err != nil {
		return providerFailure("CONFIGURATION", "AUTHENTICATION", false), nil
	}
	return d.execute(request, d.clients(policy), config.SuccessStatusCodes, message.Probe, domain.ChannelTypeHTTPConnector)
}

func (d *staticHTTPDriver) sendWebhookProfile(ctx context.Context, channelType string, message domain.DriverMessage) (domain.DriverResult, error) {
	config, err := domain.ParseWebhookProfileConfig(message.Channel.ConfigJSON)
	if err != nil {
		return providerFailure("CONFIGURATION", "CONFIGURATION", false), nil
	}
	secret, err := domain.ParseWebhookProfileSecret(channelType, message.SecretPlain)
	if err != nil {
		return providerFailure("CONFIGURATION", "CONFIGURATION", false), nil
	}
	endpoint := secret.EndpointURL
	var body any
	switch channelType {
	case domain.ChannelTypeFeishuWebhook:
		endpoint, err = feishuWebhookEndpoint(endpoint, secret.SigningSecret, d.clock())
		if err != nil {
			return providerFailure("CONFIGURATION", "AUTHENTICATION", false), nil
		}
		if strings.EqualFold(message.Presentation, domain.DeliveryPresentationCard) {
			body = map[string]any{"msg_type": "interactive", "card": feishuInteractiveCard(message)}
		} else {
			body = map[string]any{"msg_type": "text", "content": map[string]string{"text": profileText(message)}}
		}
	case domain.ChannelTypeWeComWebhook:
		wecomText := profileText(message)
		if mention := weComMentionMarkdown(message.Mention); mention != "" {
			wecomText = strings.TrimSpace(wecomText + "\n\n" + mention)
		}
		if strings.EqualFold(message.Presentation, domain.DeliveryPresentationCard) {
			cardText := strings.TrimSpace(message.Text)
			if mention := weComMentionMarkdown(message.Mention); mention != "" {
				cardText = strings.TrimSpace(cardText + "\n\n" + mention)
			}
			card := map[string]any{"card_type": "text_notice", "source": map[string]any{"desc": "Seven Agent Force"}, "main_title": map[string]any{"title": message.Subject, "desc": cardText}}
			if message.PrimaryAction != nil && strings.TrimSpace(message.ExternalActionURL) != "" {
				card["card_action"] = map[string]any{"type": 1, "url": message.ExternalActionURL}
			}
			body = map[string]any{"msgtype": "template_card", "template_card": card}
		} else if strings.EqualFold(message.Presentation, domain.DeliveryPresentationMarkdown) {
			body = map[string]any{"msgtype": "markdown", "markdown": map[string]string{"content": wecomText}}
		} else {
			text := map[string]any{"content": profileText(message)}
			if message.Mention != nil && strings.EqualFold(message.Mention.Provider, "WECOM") && validMentionSubject(message.Mention.Subject) {
				text["mentioned_list"] = []string{strings.TrimSpace(message.Mention.Subject)}
			}
			body = map[string]any{"msgtype": "text", "text": text}
		}
	}
	request, cancel, err := d.newJSONRequest(ctx, config.TimeoutMilliseconds, endpoint, body)
	if err != nil {
		return providerFailure("CONFIGURATION", "REQUEST", false), nil
	}
	defer cancel()
	request.Header.Set(domain.HTTPConnectorIdempotencyHeader, strings.TrimSpace(message.DeliveryID))
	return d.execute(request, d.clients(outboundurl.Policy{Mode: outboundurl.ModePublic}), config.SuccessStatusCodes, message.Probe, channelType)
}

func weComMentionMarkdown(mention *domain.DriverMention) string {
	if mention == nil || !strings.EqualFold(mention.Provider, "WECOM") || !validMentionSubject(mention.Subject) {
		return ""
	}
	return "<@" + strings.TrimSpace(mention.Subject) + ">"
}

func validMentionSubject(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 || strings.ContainsAny(value, "<>\"'`\\\r\n\t ") {
		return false
	}
	return true
}

func (d *staticHTTPDriver) newJSONRequest(ctx context.Context, timeoutMilliseconds int, endpoint string, payload any) (*http.Request, context.CancelFunc, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	if len(encoded) > staticHTTPRequestLimit {
		return nil, nil, fmt.Errorf("notification payload is too large")
	}
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMilliseconds)*time.Millisecond)
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		cancel()
		return nil, nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	return request, cancel, nil
}

// execute decides a durable result as soon as valid response headers arrive.
// The delivery contract uses an allowed HTTP status as its receipt proof, not
// an arbitrary receiver response body. In particular, a malicious receiver
// must not turn an already received 202/4xx header into UNKNOWN by streaming a
// body forever. Only the non-persistent probe path reads a small, structured
// error envelope for operator diagnostics.
func (d *staticHTTPDriver) execute(request *http.Request, client enterpriseHTTPDoer, allowed []int, probe bool, provider string) (domain.DriverResult, error) {
	if client == nil {
		return providerFailure("DRIVER_UNAVAILABLE", "DRIVER_UNAVAILABLE", false), nil
	}
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(err, outboundurl.ErrRedirectBlocked) {
			// The first response was received and the guarded transport refused
			// to follow its redirect. This is a known policy failure, not a lost
			// response, and no second destination was contacted.
			return providerFailure("DESTINATION_DENIED", "REDIRECT_BLOCKED", false), nil
		}
		if errors.Is(err, outboundurl.ErrDNSResolutionFailed) {
			// Resolver failure occurs before a validated dial can begin. It is a
			// known transient failure rather than proof that the receiver accepted
			// a request. The delivery layer still applies its configured retry
			// policy and records only this stable diagnostic.
			return providerFailure("TRANSIENT", "DNS_RESOLUTION_FAILED", true), nil
		}
		if isStaticHTTPPreflightDenial(err) {
			// OutboundURLGuard rejected this request before it could reach the
			// network. This is a known, non-retryable delivery failure, not an
			// ambiguous response loss.
			return providerFailure("DESTINATION_DENIED", "DESTINATION_DENIED", false), nil
		}
		// A response may have been lost after the remote endpoint accepted the
		// request. Never automatically replay this ambiguous result.
		return providerUnknown("HTTP_RESPONSE_UNCONFIRMED"), nil
	}
	if response == nil {
		return providerUnknown("HTTP_RESPONSE_UNCONFIRMED"), nil
	}
	if response.Body != nil {
		defer response.Body.Close()
	}
	if isStaticHTTPSuccess(response.StatusCode, allowed) {
		return providerAccepted(""), nil
	}
	if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError {
		diagnostic := "HTTP_STATUS_" + strconv.Itoa(response.StatusCode)
		if probe {
			return providerFailureWithError("TRANSIENT", diagnostic, true, staticHTTPProbeProviderError(provider, response)), nil
		}
		return providerFailure("TRANSIENT", diagnostic, true), nil
	}
	if probe {
		return providerFailureWithError("PROVIDER_REJECTED", "HTTP_STATUS_"+strconv.Itoa(response.StatusCode), false, staticHTTPProbeProviderError(provider, response)), nil
	}
	return providerFailure("PROVIDER_REJECTED", "HTTP_STATUS_"+strconv.Itoa(response.StatusCode), false), nil
}

// staticHTTPProbeProviderError reads at most one small, known-shape envelope
// from a failed probe response. It never copies a raw response body into an
// API result; unsupported JSON and read errors collapse to controlled codes.
func staticHTTPProbeProviderError(provider string, response *http.Response) *domain.ProviderError {
	detail := &domain.ProviderError{
		Provider: provider,
	}
	if response == nil {
		return domain.SanitizeProviderError(detail)
	}
	detail.HTTPStatus = response.StatusCode
	if response.Body == nil {
		return domain.SanitizeProviderError(detail)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, staticHTTPResponseLimit+1))
	if err != nil {
		detail.Code = "HTTP_RESPONSE_READ_FAILED"
		return domain.SanitizeProviderError(detail)
	}
	if len(raw) > staticHTTPResponseLimit {
		detail.Code = "HTTP_RESPONSE_TOO_LARGE"
		return domain.SanitizeProviderError(detail)
	}
	var envelope struct {
		Code      json.RawMessage `json:"code"`
		Message   string          `json:"message"`
		RequestID string          `json:"requestId"`
		LogID     string          `json:"logId"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return domain.SanitizeProviderError(detail)
	}
	detail.Code = staticHTTPProbeCode(envelope.Code)
	detail.Message = envelope.Message
	detail.LogID = envelope.RequestID
	if strings.TrimSpace(detail.LogID) == "" {
		detail.LogID = envelope.LogID
	}
	return domain.SanitizeProviderError(detail)
}

func staticHTTPProbeCode(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err == nil {
		return number.String()
	}
	return ""
}

// isStaticHTTPPreflightDenial identifies errors emitted before an HTTP
// request can be sent. Redirect rejections deliberately do not belong here:
// the first request may already have reached the provider.
func isStaticHTTPPreflightDenial(err error) bool {
	return errors.Is(err, outboundurl.ErrDestinationDenied) ||
		errors.Is(err, outboundurl.ErrInvalidURL) ||
		errors.Is(err, outboundurl.ErrPolicyNotFound)
}

func buildHTTPConnectorBody(config domain.HTTPConnectorConfig, message domain.DriverMessage) (map[string]any, error) {
	body := make(map[string]any, len(config.FieldMappings))
	for _, mapping := range config.FieldMappings {
		value, ok := staticHTTPFieldValue(mapping.Source, message)
		if !ok {
			return nil, fmt.Errorf("unsupported field mapping")
		}
		body[mapping.Target] = value
	}
	return body, nil
}

func staticHTTPFieldValue(source string, message domain.DriverMessage) (any, bool) {
	switch source {
	case domain.HTTPConnectorFieldSubject:
		return message.Subject, true
	case domain.HTTPConnectorFieldText:
		return message.Text, true
	case domain.HTTPConnectorFieldEventKey:
		return message.EventKey, true
	case domain.HTTPConnectorFieldCategory:
		return message.Category, true
	case domain.HTTPConnectorFieldPriority:
		return message.Priority, true
	case domain.HTTPConnectorFieldTraceID:
		return message.TraceID, true
	case domain.HTTPConnectorFieldDeepLink:
		return message.DeepLink, true
	case domain.HTTPConnectorFieldActionURL:
		return message.ExternalActionURL, true
	default:
		return nil, false
	}
}

func applyHTTPConnectorHeaders(request *http.Request, config domain.HTTPConnectorConfig, message domain.DriverMessage) {
	request.Header.Set(config.IdempotencyHeader, strings.TrimSpace(message.DeliveryID))
	for _, header := range config.HeaderAllowlist {
		switch header {
		case "Accept":
			request.Header.Set("Accept", "application/json")
		case "X-Notification-Source":
			request.Header.Set(header, message.Channel.ChannelCode)
		case "X-Notification-Category":
			request.Header.Set(header, message.Category)
		case "X-Notification-Priority":
			request.Header.Set(header, message.Priority)
		}
	}
}

func applyHTTPConnectorAuthentication(request *http.Request, config domain.HTTPConnectorConfig, secret string, now time.Time) error {
	mode := config.Authentication.Mode
	switch mode {
	case domain.HTTPConnectorAuthNone:
		return nil
	case domain.HTTPConnectorAuthBearer:
		if strings.TrimSpace(secret) == "" {
			return fmt.Errorf("missing bearer secret")
		}
		request.Header.Set("Authorization", "Bearer "+secret)
		return nil
	case domain.HTTPConnectorAuthBasic:
		if strings.TrimSpace(secret) == "" {
			return fmt.Errorf("missing basic secret")
		}
		request.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(secret)))
		return nil
	case domain.HTTPConnectorAuthHMACSHA256:
		if strings.TrimSpace(secret) == "" {
			return fmt.Errorf("missing HMAC secret")
		}
		applyHMACSignature(request, secret, now)
		return nil
	default:
		return fmt.Errorf("unsupported authentication mode")
	}
}

func applyHMACSignature(request *http.Request, secret string, now time.Time) {
	body, _ := request.GetBody()
	var content []byte
	if body != nil {
		content, _ = io.ReadAll(body)
		_ = body.Close()
	}
	digest := sha256.Sum256(content)
	contentDigest := "sha-256=:" + base64.StdEncoding.EncodeToString(digest[:]) + ":"
	timestamp := strconv.FormatInt(now.Unix(), 10)
	nonce := sha256.Sum256([]byte(request.Header.Get(domain.HTTPConnectorIdempotencyHeader)))
	nonceValue := hexNonce(nonce[:])
	request.Header.Set("Content-Digest", contentDigest)
	request.Header.Set("X-Notification-Timestamp", timestamp)
	request.Header.Set("X-Notification-Nonce", nonceValue)
	components := "(\"@method\" \"@target-uri\" \"content-digest\" \"idempotency-key\" \"x-notification-timestamp\" \"x-notification-nonce\")"
	signatureInput := components + ";created=" + timestamp + ";keyid=\"connection\";alg=\"hmac-sha256\""
	base := strings.Join([]string{
		"\"@method\": post",
		"\"@target-uri\": " + request.URL.String(),
		"\"content-digest\": " + contentDigest,
		"\"idempotency-key\": " + request.Header.Get(domain.HTTPConnectorIdempotencyHeader),
		"\"x-notification-timestamp\": " + timestamp,
		"\"x-notification-nonce\": " + nonceValue,
		"\"@signature-params\": " + signatureInput,
	}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(base))
	request.Header.Set("Signature-Input", "sig1="+signatureInput)
	request.Header.Set("Signature", "sig1=:"+base64.StdEncoding.EncodeToString(mac.Sum(nil))+":")
}

func feishuWebhookEndpoint(endpoint, signingSecret string, now time.Time) (string, error) {
	if strings.TrimSpace(signingSecret) == "" {
		return endpoint, nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	timestamp := strconv.FormatInt(now.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(signingSecret))
	_, _ = mac.Write([]byte(timestamp + "\n" + signingSecret))
	query := parsed.Query()
	query.Set("timestamp", timestamp)
	query.Set("sign", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func profileText(message domain.DriverMessage) string {
	if strings.TrimSpace(message.Text) != "" {
		return externalTextWithAction(message)
	}
	return message.Subject
}

func isStaticHTTPSuccess(statusCode int, allowed []int) bool {
	if len(allowed) == 0 {
		return statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices
	}
	for _, candidate := range allowed {
		if statusCode == candidate {
			return true
		}
	}
	return false
}

func (d *staticHTTPDriver) clock() time.Time {
	if d != nil && d.now != nil {
		return d.now().UTC()
	}
	return time.Now().UTC()
}

func hexNonce(value []byte) string {
	return fmt.Sprintf("%x", value)
}
