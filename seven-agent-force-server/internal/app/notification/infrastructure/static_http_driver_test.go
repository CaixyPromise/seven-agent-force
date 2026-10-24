package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	notificationapp "github.com/CaixyPromise/seven-agent/force/internal/app/notification/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/outboundurl"
)

func TestHTTPConnectorDriverBuildsFixedRequestForBearerBasicAndHMAC(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		mode     string
		secret   string
		wantAuth string
		wantHMAC bool
	}{
		{name: "bearer", mode: domain.HTTPConnectorAuthBearer, secret: "bearer-secret", wantAuth: "Bearer bearer-secret"},
		{name: "basic", mode: domain.HTTPConnectorAuthBasic, secret: "user:password", wantAuth: "Basic dXNlcjpwYXNzd29yZA=="},
		{name: "hmac", mode: domain.HTTPConnectorAuthHMACSHA256, secret: "hmac-secret", wantHMAC: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var observedRequest *http.Request
			var observedBody map[string]any
			var observedRawBody []byte
			client := enterpriseHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
				observedRequest = request.Clone(request.Context())
				var err error
				observedRawBody, err = io.ReadAll(request.Body)
				if err != nil {
					return nil, err
				}
				if err := json.Unmarshal(observedRawBody, &observedBody); err != nil {
					return nil, err
				}
				return enterpriseHTTPResponse(http.StatusAccepted, "{}"), nil
			})
			driver := staticDriverFor(t, domain.ChannelTypeHTTPConnector, client)
			result, err := driver.SendResult(context.Background(), notificationapp.DriverMessage{
				Channel:     staticHTTPDriverChannel(t, testCase.mode),
				SecretPlain: testCase.secret,
				Subject:     "发布完成",
				Text:        "请到系统查看详情。",
				EventKey:    "release.ready",
				Category:    "RELEASE",
				Priority:    "HIGH",
				TraceID:     "trace-42",
				DeepLink:    "/system/release/42",
				DeliveryID:  "delivery-http-42",
			})
			if err != nil || result.Status != notificationapp.DriverResultProviderAccepted {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			if observedRequest == nil || observedRequest.Method != http.MethodPost || observedRequest.URL.String() != "https://receiver.example/notifications" {
				t.Fatalf("fixed request=%#v", observedRequest)
			}
			if observedRequest.Header.Get(domain.HTTPConnectorIdempotencyHeader) != "delivery-http-42" || observedRequest.Header.Get("X-Notification-Category") != "RELEASE" || observedRequest.Header.Get("X-Notification-Priority") != "HIGH" {
				t.Fatalf("protected headers=%#v", observedRequest.Header)
			}
			if observedBody["title"] != "发布完成" || observedBody["content"] != "请到系统查看详情。" || observedBody["event"] != "release.ready" || len(observedBody) != 3 {
				t.Fatalf("mapped body=%#v", observedBody)
			}
			if testCase.wantAuth != "" && observedRequest.Header.Get("Authorization") != testCase.wantAuth {
				t.Fatalf("authorization=%q, want %q", observedRequest.Header.Get("Authorization"), testCase.wantAuth)
			}
			if testCase.wantHMAC {
				digest := sha256.Sum256(observedRawBody)
				wantDigest := "sha-256=:" + base64.StdEncoding.EncodeToString(digest[:]) + ":"
				if observedRequest.Header.Get("Content-Digest") != wantDigest || !strings.HasPrefix(observedRequest.Header.Get("Signature-Input"), "sig1=(") || !strings.HasPrefix(observedRequest.Header.Get("Signature"), "sig1=:") || observedRequest.Header.Get("X-Notification-Timestamp") == "" || observedRequest.Header.Get("X-Notification-Nonce") == "" {
					t.Fatalf("HMAC headers=%#v", observedRequest.Header)
				}
			}
		})
	}
}

func TestHTTPConnectorDriverReturnsUnknownForLostResponse(t *testing.T) {
	client := enterpriseHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection closed after write")
	})
	driver := staticDriverFor(t, domain.ChannelTypeHTTPConnector, client)
	result, err := driver.SendResult(context.Background(), notificationapp.DriverMessage{
		Channel:     staticHTTPDriverChannel(t, domain.HTTPConnectorAuthBearer),
		SecretPlain: "bearer-secret",
		Subject:     "发布完成",
		Text:        "详情",
		DeliveryID:  "delivery-lost-response",
	})
	if err != nil || result.Status != notificationapp.DriverResultUnknown || result.Diagnostic != "HTTP_RESPONSE_UNCONFIRMED" || result.Retryable {
		t.Fatalf("lost response result=%#v err=%v", result, err)
	}
}

func TestStaticHTTPDriversRetryOnlyExplicitRateLimitAndServerFailures(t *testing.T) {
	for _, statusCode := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway} {
		t.Run(strconv.Itoa(statusCode), func(t *testing.T) {
			driver := staticDriverFor(t, domain.ChannelTypeWeComWebhook, enterpriseHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
				return enterpriseHTTPResponse(statusCode, `{}`), nil
			}))
			secret, err := domain.EncodeWebhookProfileSecret(domain.ChannelTypeWeComWebhook, domain.WebhookProfileSecret{EndpointURL: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=retry-fixture"})
			if err != nil {
				t.Fatal(err)
			}
			config, err := domain.EncodeWebhookProfileConfig(domain.WebhookProfileConfig{TimeoutMilliseconds: 5000})
			if err != nil {
				t.Fatal(err)
			}
			result, err := driver.SendResult(context.Background(), notificationapp.DriverMessage{
				Channel: domain.Channel{ChannelType: domain.ChannelTypeWeComWebhook, ChannelCode: "wecom-retry", ConfigJSON: config}, SecretPlain: secret,
				Text: "任务需要处理", DeliveryID: "delivery-retry-" + strconv.Itoa(statusCode),
			})
			if err != nil || result.Status != notificationapp.DriverResultFailed || result.FailureClass != "TRANSIENT" || result.Diagnostic != "HTTP_STATUS_"+strconv.Itoa(statusCode) || !result.Retryable {
				t.Fatalf("status %d result=%#v err=%v", statusCode, result, err)
			}
		})
	}

	driver := staticDriverFor(t, domain.ChannelTypeWeComWebhook, enterpriseHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
		return enterpriseHTTPResponse(http.StatusBadRequest, `{}`), nil
	}))
	secret, err := domain.EncodeWebhookProfileSecret(domain.ChannelTypeWeComWebhook, domain.WebhookProfileSecret{EndpointURL: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=reject-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	config, err := domain.EncodeWebhookProfileConfig(domain.WebhookProfileConfig{TimeoutMilliseconds: 5000})
	if err != nil {
		t.Fatal(err)
	}
	result, err := driver.SendResult(context.Background(), notificationapp.DriverMessage{
		Channel: domain.Channel{ChannelType: domain.ChannelTypeWeComWebhook, ChannelCode: "wecom-reject", ConfigJSON: config}, SecretPlain: secret,
		Text: "任务需要处理", DeliveryID: "delivery-reject-400",
	})
	if err != nil || result.Status != notificationapp.DriverResultFailed || result.FailureClass != "PROVIDER_REJECTED" || result.Retryable {
		t.Fatalf("400 result=%#v err=%v", result, err)
	}
}

func TestHTTPConnectorDriverDecidesFromHeadersWithoutReadingSlowBody(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		statusCode int
		wantStatus string
	}{
		{name: "accepted headers", statusCode: http.StatusAccepted, wantStatus: notificationapp.DriverResultProviderAccepted},
		{name: "rejected headers", statusCode: http.StatusBadRequest, wantStatus: notificationapp.DriverResultFailed},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			body := &staticUnexpectedReadBody{}
			driver := staticDriverFor(t, domain.ChannelTypeHTTPConnector, enterpriseHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: testCase.statusCode, Body: body, Header: make(http.Header)}, nil
			}))
			result, err := driver.SendResult(context.Background(), notificationapp.DriverMessage{
				Channel:     staticHTTPDriverChannel(t, domain.HTTPConnectorAuthBearer),
				SecretPlain: "bearer-secret",
				Subject:     "status is enough",
				Text:        "the body must not delay durable delivery",
				DeliveryID:  "delivery-slow-body-" + strconv.Itoa(testCase.statusCode),
			})
			if err != nil || result.Status != testCase.wantStatus {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			if body.reads != 0 || !body.closed {
				t.Fatalf("response body reads=%d closed=%t, want no read and close", body.reads, body.closed)
			}
		})
	}
}

func TestHTTPConnectorProbeReturnsOnlySanitizedBoundedErrorDetails(t *testing.T) {
	t.Run("known response envelope", func(t *testing.T) {
		driver := staticDriverFor(t, domain.ChannelTypeHTTPConnector, enterpriseHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
			return enterpriseHTTPResponse(http.StatusBadRequest, `{"code":230001,"message":"denied bearer top-secret from ip: 203.0.113.9 at https://receiver.example/error","requestId":"request-42"}`), nil
		}))
		result, err := driver.SendResult(context.Background(), notificationapp.DriverMessage{
			Channel:     staticHTTPDriverChannel(t, domain.HTTPConnectorAuthBearer),
			SecretPlain: "bearer-secret",
			Subject:     "probe",
			Text:        "probe text",
			DeliveryID:  "probe-connector-error",
			Probe:       true,
		})
		if err != nil || result.Status != notificationapp.DriverResultFailed || result.ProviderError == nil {
			t.Fatalf("result=%#v err=%v", result, err)
		}
		detail := result.ProviderError
		if detail.HTTPStatus != http.StatusBadRequest || detail.Code != "230001" || detail.LogID != "request-42" || strings.Contains(detail.Message, "top-secret") || strings.Contains(detail.Message, "203.0.113.9") || strings.Contains(detail.Message, "receiver.example") {
			t.Fatalf("sanitized probe error=%#v", detail)
		}
	})

	t.Run("oversized response is not exposed", func(t *testing.T) {
		body := strings.Repeat("x", staticHTTPResponseLimit+1)
		driver := staticDriverFor(t, domain.ChannelTypeHTTPConnector, enterpriseHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
			return enterpriseHTTPResponse(http.StatusBadRequest, body), nil
		}))
		result, err := driver.SendResult(context.Background(), notificationapp.DriverMessage{
			Channel:     staticHTTPDriverChannel(t, domain.HTTPConnectorAuthBearer),
			SecretPlain: "bearer-secret",
			Subject:     "probe",
			Text:        "probe text",
			DeliveryID:  "probe-connector-oversized",
			Probe:       true,
		})
		if err != nil || result.ProviderError == nil || result.ProviderError.Code != "HTTP_RESPONSE_TOO_LARGE" || result.ProviderError.Message != "" {
			t.Fatalf("oversized result=%#v err=%v", result, err)
		}
	})
}

func TestHTTPConnectorDriverMarksGuardDenialAsFailedWithoutDial(t *testing.T) {
	dials := 0
	guard := outboundurl.NewOutboundURLGuard(outboundurl.Options{
		Resolver: staticHTTPResolver{addresses: map[string][]net.IPAddr{
			"receiver.example": {{IP: net.ParseIP("198.18.7.9")}},
		}},
		InterfaceAddrs: func() ([]net.Addr, error) { return nil, nil },
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			dials++
			return nil, errors.New("guard denial must not dial")
		},
	})
	registry := NewDriverRegistryWithOutboundGuard(nil, guard)
	driver, ok := registry.Driver(domain.ChannelTypeHTTPConnector).(*staticHTTPDriver)
	if !ok || driver == nil {
		t.Fatal("guarded static HTTP driver is unavailable")
	}

	result, err := driver.SendResult(context.Background(), notificationapp.DriverMessage{
		Channel:     staticHTTPDriverChannel(t, domain.HTTPConnectorAuthBearer),
		SecretPlain: "bearer-secret",
		Subject:     "发布完成",
		Text:        "详情",
		DeliveryID:  "delivery-guard-denied",
	})
	if err != nil || result.Status != notificationapp.DriverResultFailed || result.FailureClass != "DESTINATION_DENIED" || result.Diagnostic != "DESTINATION_DENIED" || result.Retryable {
		t.Fatalf("guard denial result=%#v err=%v", result, err)
	}
	if dials != 0 {
		t.Fatalf("guard denial dialed=%d, want 0", dials)
	}
}

func TestHTTPConnectorDriverClassifiesKnownPreDialAndRedirectFailures(t *testing.T) {
	t.Run("redirect response is a permanent policy failure", func(t *testing.T) {
		driver := staticDriverFor(t, domain.ChannelTypeHTTPConnector, enterpriseHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
			return nil, outboundurl.ErrRedirectBlocked
		}))
		result, err := driver.SendResult(context.Background(), notificationapp.DriverMessage{
			Channel:     staticHTTPDriverChannel(t, domain.HTTPConnectorAuthBearer),
			SecretPlain: "bearer-secret",
			Subject:     "redirect must not follow",
			Text:        "details",
			DeliveryID:  "delivery-redirect-blocked",
		})
		if err != nil || result.Status != notificationapp.DriverResultFailed || result.FailureClass != "DESTINATION_DENIED" || result.Diagnostic != "REDIRECT_BLOCKED" || result.Retryable {
			t.Fatalf("redirect result=%#v err=%v", result, err)
		}
	})

	t.Run("DNS failure is transient and never dials", func(t *testing.T) {
		dials := 0
		guard := outboundurl.NewOutboundURLGuard(outboundurl.Options{
			Resolver:       staticHTTPResolver{err: errors.New("resolver unavailable")},
			InterfaceAddrs: func() ([]net.Addr, error) { return nil, nil },
			DialContext: func(context.Context, string, string) (net.Conn, error) {
				dials++
				return nil, errors.New("DNS failure must not dial")
			},
		})
		registry := NewDriverRegistryWithOutboundGuard(nil, guard)
		driver, ok := registry.Driver(domain.ChannelTypeHTTPConnector).(*staticHTTPDriver)
		if !ok || driver == nil {
			t.Fatal("guarded static HTTP driver is unavailable")
		}

		result, err := driver.SendResult(context.Background(), notificationapp.DriverMessage{
			Channel:     staticHTTPDriverChannel(t, domain.HTTPConnectorAuthBearer),
			SecretPlain: "bearer-secret",
			Subject:     "DNS must not send",
			Text:        "details",
			DeliveryID:  "delivery-dns-failure",
		})
		if err != nil || result.Status != notificationapp.DriverResultFailed || result.FailureClass != "TRANSIENT" || result.Diagnostic != "DNS_RESOLUTION_FAILED" || !result.Retryable {
			t.Fatalf("DNS failure result=%#v err=%v", result, err)
		}
		if dials != 0 {
			t.Fatalf("DNS failure dialed=%d, want 0", dials)
		}
	})
}

func TestFixedWebhookProfilesUseCompiledProviderShapes(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		channelType string
		endpoint    string
		signing     string
		assertBody  func(*testing.T, map[string]any)
	}{
		{
			name:        "feishu",
			channelType: domain.ChannelTypeFeishuWebhook,
			endpoint:    "https://open.feishu.cn/open-apis/bot/v2/hook/token-42",
			signing:     "feishu-signing-secret",
			assertBody: func(t *testing.T, body map[string]any) {
				t.Helper()
				if body["msg_type"] != "text" {
					t.Fatalf("Feishu body=%#v", body)
				}
				content, ok := body["content"].(map[string]any)
				if !ok || content["text"] != "群提醒正文" || len(body) != 2 {
					t.Fatalf("Feishu body=%#v", body)
				}
			},
		},
		{
			name:        "wecom",
			channelType: domain.ChannelTypeWeComWebhook,
			endpoint:    "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=token-42",
			assertBody: func(t *testing.T, body map[string]any) {
				t.Helper()
				if body["msgtype"] != "text" {
					t.Fatalf("WeCom body=%#v", body)
				}
				text, ok := body["text"].(map[string]any)
				if !ok || text["content"] != "群提醒正文" || len(body) != 2 {
					t.Fatalf("WeCom body=%#v", body)
				}
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var observedRequest *http.Request
			var observedBody map[string]any
			client := enterpriseHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
				observedRequest = request.Clone(request.Context())
				if err := json.NewDecoder(request.Body).Decode(&observedBody); err != nil {
					return nil, err
				}
				return enterpriseHTTPResponse(http.StatusOK, "{}"), nil
			})
			driver := staticDriverFor(t, testCase.channelType, client)
			secret, err := domain.EncodeWebhookProfileSecret(testCase.channelType, domain.WebhookProfileSecret{EndpointURL: testCase.endpoint, SigningSecret: testCase.signing})
			if err != nil {
				t.Fatalf("encode profile secret: %v", err)
			}
			config, err := domain.EncodeWebhookProfileConfig(domain.WebhookProfileConfig{TimeoutMilliseconds: 5000})
			if err != nil {
				t.Fatalf("encode profile config: %v", err)
			}
			result, err := driver.SendResult(context.Background(), notificationapp.DriverMessage{
				Channel:     domain.Channel{ChannelType: testCase.channelType, ChannelCode: testCase.name + "-group", ConfigJSON: config},
				SecretPlain: secret,
				Text:        "群提醒正文",
				DeliveryID:  "delivery-" + testCase.name,
			})
			if err != nil || result.Status != notificationapp.DriverResultProviderAccepted {
				t.Fatalf("profile result=%#v err=%v", result, err)
			}
			if observedRequest == nil || observedRequest.Method != http.MethodPost || observedRequest.Header.Get(domain.HTTPConnectorIdempotencyHeader) != "delivery-"+testCase.name {
				t.Fatalf("profile request=%#v", observedRequest)
			}
			if observedRequest.URL.Host != strings.Split(strings.TrimPrefix(testCase.endpoint, "https://"), "/")[0] {
				t.Fatalf("profile host=%q endpoint=%q", observedRequest.URL.Host, testCase.endpoint)
			}
			if testCase.channelType == domain.ChannelTypeFeishuWebhook && (observedRequest.URL.Query().Get("timestamp") == "" || observedRequest.URL.Query().Get("sign") == "") {
				t.Fatalf("Feishu signing query=%q", observedRequest.URL.RawQuery)
			}
			testCase.assertBody(t, observedBody)
		})
	}
}

func TestFixedWebhookProfilesRenderFrozenActionCards(t *testing.T) {
	action := &domain.NotificationAction{Type: domain.NotificationActionTypeOpenInternal, Label: "查看任务", Link: "/workforce/tasks/42", Style: domain.NotificationActionStylePrimary}
	actionURL := "https://workforce.example.com/console/workforce/tasks/42"
	for _, testCase := range []struct {
		channelType string
		endpoint    string
		wantType    string
	}{
		{channelType: domain.ChannelTypeFeishuWebhook, endpoint: "https://open.feishu.cn/open-apis/bot/v2/hook/token-card", wantType: "interactive"},
		{channelType: domain.ChannelTypeWeComWebhook, endpoint: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=token-card", wantType: "template_card"},
	} {
		t.Run(testCase.channelType, func(t *testing.T) {
			var observed map[string]any
			driver := staticDriverFor(t, testCase.channelType, enterpriseHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
				if err := json.NewDecoder(request.Body).Decode(&observed); err != nil {
					return nil, err
				}
				return enterpriseHTTPResponse(http.StatusOK, `{}`), nil
			}))
			secret, err := domain.EncodeWebhookProfileSecret(testCase.channelType, domain.WebhookProfileSecret{EndpointURL: testCase.endpoint})
			if err != nil {
				t.Fatal(err)
			}
			config, err := domain.EncodeWebhookProfileConfig(domain.WebhookProfileConfig{TimeoutMilliseconds: 5000})
			if err != nil {
				t.Fatal(err)
			}
			mention := &domain.DriverMention{Provider: "FEISHU", Subject: "ou_handler", Label: "相关处理人"}
			if testCase.channelType == domain.ChannelTypeWeComWebhook {
				mention = &domain.DriverMention{Provider: "WECOM", Subject: "zhangwei", Label: "相关处理人"}
			}
			result, err := driver.SendResult(context.Background(), notificationapp.DriverMessage{
				Channel: domain.Channel{ChannelType: testCase.channelType, ChannelCode: "group", ConfigJSON: config}, SecretPlain: secret,
				Subject: "任务需要处理", Text: "请确认。", DeliveryID: "delivery-card", Presentation: domain.DeliveryPresentationCard, PrimaryAction: action, ExternalActionURL: actionURL, Mention: mention,
			})
			if err != nil || result.Status != notificationapp.DriverResultProviderAccepted {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			encoded, _ := json.Marshal(observed)
			mentionSubject := "ou_handler"
			if testCase.channelType == domain.ChannelTypeWeComWebhook {
				mentionSubject = "zhangwei"
			}
			if (observed["msg_type"] != testCase.wantType && observed["msgtype"] != testCase.wantType) || strings.Count(string(encoded), actionURL) != 1 || strings.Count(string(encoded), mentionSubject) != 1 {
				t.Fatalf("actionable webhook payload=%s", encoded)
			}
		})
	}
}

func TestFixedWebhookCompletedCardOmitsMention(t *testing.T) {
	var observed map[string]any
	driver := staticDriverFor(t, domain.ChannelTypeFeishuWebhook, enterpriseHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(request.Body).Decode(&observed); err != nil {
			return nil, err
		}
		return enterpriseHTTPResponse(http.StatusOK, `{}`), nil
	}))
	secret, err := domain.EncodeWebhookProfileSecret(domain.ChannelTypeFeishuWebhook, domain.WebhookProfileSecret{EndpointURL: "https://open.feishu.cn/open-apis/bot/v2/hook/completed-no-mention"})
	if err != nil {
		t.Fatal(err)
	}
	config, err := domain.EncodeWebhookProfileConfig(domain.WebhookProfileConfig{TimeoutMilliseconds: 5000})
	if err != nil {
		t.Fatal(err)
	}
	result, err := driver.SendResult(context.Background(), notificationapp.DriverMessage{
		Channel: domain.Channel{ChannelType: domain.ChannelTypeFeishuWebhook, ChannelCode: "completed", ConfigJSON: config}, SecretPlain: secret,
		Subject: "当前轮任务已完成", Text: "结果已准备好。", DeliveryID: "delivery-completed", Presentation: domain.DeliveryPresentationCard,
	})
	if err != nil || result.Status != notificationapp.DriverResultProviderAccepted {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	encoded, _ := json.Marshal(observed)
	if strings.Contains(string(encoded), "<at") || strings.Contains(string(encoded), "mentioned_list") {
		t.Fatalf("completed payload unexpectedly contains mention: %s", encoded)
	}
}

func TestHTTPConnectorMapsOnlyPlatformGeneratedActionURL(t *testing.T) {
	config := domain.HTTPConnectorConfig{FieldMappings: []domain.HTTPConnectorFieldMapping{{Source: domain.HTTPConnectorFieldDeepLink, Target: "relative"}, {Source: domain.HTTPConnectorFieldActionURL, Target: "absolute"}}}
	body, err := buildHTTPConnectorBody(config, notificationapp.DriverMessage{DeepLink: "/workforce/tasks/42", ExternalActionURL: "https://workforce.example.com/workforce/tasks/42"})
	if err != nil || body["relative"] != "/workforce/tasks/42" || body["absolute"] != "https://workforce.example.com/workforce/tasks/42" {
		t.Fatalf("mapped action values=%#v err=%v", body, err)
	}
}

func TestFixedWebhookProfileRejectsDynamicTargetBeforeHTTP(t *testing.T) {
	calls := 0
	client := enterpriseHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return enterpriseHTTPResponse(http.StatusOK, "{}"), nil
	})
	driver := staticDriverFor(t, domain.ChannelTypeFeishuWebhook, client)
	config, err := domain.EncodeWebhookProfileConfig(domain.WebhookProfileConfig{TimeoutMilliseconds: 5000})
	if err != nil {
		t.Fatalf("encode profile config: %v", err)
	}
	result, err := driver.SendResult(context.Background(), notificationapp.DriverMessage{
		Channel: domain.Channel{ChannelType: domain.ChannelTypeFeishuWebhook, ChannelCode: "feishu-group", ConfigJSON: config},
		Target:  "oc_caller_supplied_group",
		Text:    "must not send",
	})
	if err != nil || result.Status != notificationapp.DriverResultFailed || result.FailureClass != "CALLER_BOUNDARY" || result.Diagnostic != "STATIC_TARGET_OVERRIDE" {
		t.Fatalf("dynamic profile target result=%#v err=%v", result, err)
	}
	if calls != 0 {
		t.Fatalf("dynamic profile target reached HTTP calls=%d", calls)
	}
}

func staticDriverFor(t *testing.T, channelType string, client enterpriseHTTPDoer) *staticHTTPDriver {
	t.Helper()
	registry := NewDriverRegistryWithHTTPClient(nil, client)
	driver, ok := registry.Driver(channelType).(*staticHTTPDriver)
	if !ok || driver == nil {
		t.Fatalf("static driver %s is unavailable", channelType)
	}
	driver.now = func() time.Time { return time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC) }
	return driver
}

func staticHTTPDriverChannel(t *testing.T, mode string) domain.Channel {
	t.Helper()
	config, err := domain.EncodeHTTPConnectorConfig(domain.HTTPConnectorConfig{
		EndpointURL: "https://receiver.example/notifications",
		Method:      domain.HTTPConnectorMethodPOST,
		Authentication: domain.HTTPConnectorAuthentication{
			Mode:      mode,
			SecretRef: domain.HTTPConnectorSecretRefConnection,
		},
		FieldMappings: []domain.HTTPConnectorFieldMapping{
			{Source: domain.HTTPConnectorFieldSubject, Target: "title"},
			{Source: domain.HTTPConnectorFieldText, Target: "content"},
			{Source: domain.HTTPConnectorFieldEventKey, Target: "event"},
		},
		HeaderAllowlist:     []string{"X-Notification-Category", "X-Notification-Priority"},
		IdempotencyHeader:   domain.HTTPConnectorIdempotencyHeader,
		TimeoutMilliseconds: 5000,
	})
	if err != nil {
		t.Fatalf("encode connector config: %v", err)
	}
	return domain.Channel{ChannelType: domain.ChannelTypeHTTPConnector, ChannelCode: "audit-http", ConfigJSON: config}
}

type staticHTTPResolver struct {
	addresses map[string][]net.IPAddr
	err       error
}

type staticUnexpectedReadBody struct {
	reads  int
	closed bool
}

func (b *staticUnexpectedReadBody) Read([]byte) (int, error) {
	b.reads++
	return 0, errors.New("response body must not be read")
}

func (b *staticUnexpectedReadBody) Close() error {
	b.closed = true
	return nil
}

func (r staticHTTPResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	if r.err != nil {
		return nil, r.err
	}
	return append([]net.IPAddr(nil), r.addresses[host]...), nil
}
