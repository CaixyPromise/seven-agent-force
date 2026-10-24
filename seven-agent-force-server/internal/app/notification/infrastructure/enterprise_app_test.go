package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	notificationapp "github.com/CaixyPromise/seven-agent/force/internal/app/notification/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/outboundurl"
)

func TestEnterpriseApplicationDriversUseFixedHostsAndProviderOwnedPayloads(t *testing.T) {
	var mu sync.Mutex
	requests := make([]*http.Request, 0, 4)
	bodies := make([]map[string]any, 0, 4)
	client := enterpriseHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		var body map[string]any
		if request.Body != nil {
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				return nil, err
			}
		}
		mu.Lock()
		requests = append(requests, request.Clone(request.Context()))
		bodies = append(bodies, body)
		mu.Unlock()
		switch request.URL.Host + request.URL.Path {
		case "open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal":
			return enterpriseHTTPResponse(http.StatusOK, `{"code":0,"tenant_access_token":"tenant-token","expire":7200}`), nil
		case "open.feishu.cn/open-apis/im/v1/messages":
			return enterpriseHTTPResponse(http.StatusOK, `{"code":0,"data":{"message_id":"om_123"}}`), nil
		case "qyapi.weixin.qq.com/cgi-bin/gettoken":
			return enterpriseHTTPResponse(http.StatusOK, `{"errcode":0,"access_token":"wecom-token","expires_in":7200}`), nil
		case "qyapi.weixin.qq.com/cgi-bin/message/send":
			return enterpriseHTTPResponse(http.StatusOK, `{"errcode":0,"msgid":"msg_123"}`), nil
		default:
			return enterpriseHTTPResponse(http.StatusNotFound, `{}`), nil
		}
	})
	registry := NewDriverRegistryWithHTTPClient(nil, client)

	feishu, ok := registry.Driver(domain.ChannelTypeFeishuApp).(notificationapp.ResultChannelDriver)
	if !ok {
		t.Fatal("Feishu application driver must expose structured results")
	}
	feishuResult, err := feishu.SendResult(context.Background(), notificationapp.DriverMessage{
		Channel:     domain.Channel{ChannelType: domain.ChannelTypeFeishuApp, ScopeID: "local", ChannelCode: "feishu", ConfigJSON: `{"appId":"cli_test"}`},
		SecretPlain: "app-secret",
		Target:      "ou_target",
		Text:        "一条短文本",
		DeliveryID:  "ntf_ext_123",
	})
	if err != nil || feishuResult.Status != notificationapp.DriverResultProviderAccepted || feishuResult.ProviderReference != "om_123" {
		t.Fatalf("Feishu result=%#v err=%v", feishuResult, err)
	}

	wecom, ok := registry.Driver(domain.ChannelTypeWeComApp).(notificationapp.ResultChannelDriver)
	if !ok {
		t.Fatal("WeCom application driver must expose structured results")
	}
	wecomResult, err := wecom.SendResult(context.Background(), notificationapp.DriverMessage{
		Channel:        domain.Channel{ChannelType: domain.ChannelTypeWeComApp, ScopeID: "local", ChannelCode: "wecom", ConfigJSON: `{"corpId":"ww_test","agentId":"100001"}`},
		SecretPlain:    "corp-secret",
		Target:         "member-7",
		Text:           "一条短文本",
		ProviderParams: map[string]any{domain.ProviderParameterMentionedList: []string{"member-a", "member-b"}},
	})
	if err != nil || wecomResult.Status != notificationapp.DriverResultProviderAccepted || wecomResult.ProviderReference != "msg_123" {
		t.Fatalf("WeCom result=%#v err=%v", wecomResult, err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 4 || len(bodies) != 4 {
		t.Fatalf("requests=%d bodies=%d, want token+send per provider", len(requests), len(bodies))
	}
	if requests[0].URL.String() != feishuTokenURL || requests[1].URL.String() != feishuMessageURL {
		t.Fatalf("Feishu endpoints=%q %q, want fixed official endpoints", requests[0].URL, requests[1].URL)
	}
	if requests[1].Header.Get("Authorization") != "Bearer tenant-token" || bodies[1]["receive_id"] != "ou_target" || bodies[1]["msg_type"] != "text" || bodies[1]["uuid"] != "ntf_ext_123" {
		t.Fatalf("Feishu request headers=%v body=%#v", requests[1].Header, bodies[1])
	}
	content, ok := bodies[1]["content"].(string)
	if !ok || !strings.Contains(content, "一条短文本") {
		t.Fatalf("Feishu content=%#v", bodies[1]["content"])
	}
	if requests[2].URL.Host != "qyapi.weixin.qq.com" || requests[2].URL.Path != "/cgi-bin/gettoken" || requests[2].URL.Query().Get("corpid") != "ww_test" {
		t.Fatalf("WeCom token endpoint=%q", requests[2].URL)
	}
	if requests[3].URL.Host != "qyapi.weixin.qq.com" || requests[3].URL.Path != "/cgi-bin/message/send" || requests[3].URL.Query().Get("access_token") != "wecom-token" {
		t.Fatalf("WeCom message endpoint=%q", requests[3].URL)
	}
	text, ok := bodies[3]["text"].(map[string]any)
	if !ok || text["content"] != "一条短文本" {
		t.Fatalf("WeCom text body=%#v", bodies[3]["text"])
	}
	mentions, ok := text["mentioned_list"].([]any)
	if !ok || len(mentions) != 2 || mentions[0] != "member-a" {
		t.Fatalf("WeCom provider-owned mentions=%#v", text["mentioned_list"])
	}
	if _, exists := bodies[3]["uuid"]; exists {
		t.Fatalf("WeCom request must not invent Feishu UUID field: %#v", bodies[3])
	}
}

func TestEnterpriseApplicationDriversRenderOneFrozenPrimaryAction(t *testing.T) {
	var bodies []map[string]any
	client := enterpriseHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		var body map[string]any
		if request.Body != nil {
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				return nil, err
			}
		}
		bodies = append(bodies, body)
		switch request.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			return enterpriseHTTPResponse(http.StatusOK, `{"code":0,"tenant_access_token":"tenant-token","expire":7200}`), nil
		case "/open-apis/im/v1/messages":
			return enterpriseHTTPResponse(http.StatusOK, `{"code":0,"data":{"message_id":"om_card"}}`), nil
		case "/cgi-bin/gettoken":
			return enterpriseHTTPResponse(http.StatusOK, `{"errcode":0,"access_token":"wecom-token","expires_in":7200}`), nil
		case "/cgi-bin/message/send":
			return enterpriseHTTPResponse(http.StatusOK, `{"errcode":0,"msgid":"msg_card"}`), nil
		default:
			return enterpriseHTTPResponse(http.StatusNotFound, `{}`), nil
		}
	})
	registry := NewDriverRegistryWithHTTPClient(nil, client)
	action := &domain.NotificationAction{Type: domain.NotificationActionTypeOpenInternal, Label: "查看任务", Link: "/workforce/tasks/2065424359060983808?turn=2065424359060983812&focus=evaluation", Style: domain.NotificationActionStylePrimary}
	url := "https://workforce.example.com/console/workforce/tasks/2065424359060983808?turn=2065424359060983812&focus=evaluation"

	feishu := registry.Driver(domain.ChannelTypeFeishuApp).(notificationapp.ResultChannelDriver)
	result, err := feishu.SendResult(context.Background(), notificationapp.DriverMessage{
		Channel: domain.Channel{ChannelType: domain.ChannelTypeFeishuApp, ScopeID: "local", ChannelCode: "feishu", ConfigJSON: `{"appId":"cli_test"}`}, SecretPlain: "app-secret",
		IdentityKind: domain.ExternalIdentityFeishuOpenID, Target: "ou_target", Subject: "任务需要处理", Text: "请确认处理方式。", DeliveryID: "delivery-card", Presentation: domain.DeliveryPresentationCard, PrimaryAction: action, ExternalActionURL: url,
		Mention: &domain.DriverMention{Provider: "FEISHU", Subject: "ou_handler", Label: "相关处理人"},
	})
	if err != nil || result.Status != notificationapp.DriverResultProviderAccepted {
		t.Fatalf("Feishu card result=%#v err=%v", result, err)
	}
	feishuBody := bodies[1]
	if feishuBody["msg_type"] != "interactive" || feishuBody["uuid"] != "delivery-card" {
		t.Fatalf("Feishu card envelope=%#v", feishuBody)
	}
	var card map[string]any
	if err := json.Unmarshal([]byte(feishuBody["content"].(string)), &card); err != nil {
		t.Fatalf("decode Feishu card: %v", err)
	}
	encoded := jsonWithoutHTMLEscape(t, card)
	if strings.Count(encoded, url) != 1 || strings.Count(encoded, "查看任务") != 1 || strings.Count(encoded, `<at id=ou_handler></at>`) != 1 {
		t.Fatalf("Feishu card action=%s", encoded)
	}

	wecom := registry.Driver(domain.ChannelTypeWeComApp).(notificationapp.ResultChannelDriver)
	result, err = wecom.SendResult(context.Background(), notificationapp.DriverMessage{
		Channel: domain.Channel{ChannelType: domain.ChannelTypeWeComApp, ScopeID: "local", ChannelCode: "wecom", ConfigJSON: `{"corpId":"ww_test","agentId":"100001"}`}, SecretPlain: "corp-secret",
		IdentityKind: domain.ExternalIdentityWeComUserID, Target: "member-7", Subject: "任务需要处理", Text: "请确认处理方式。", Presentation: domain.DeliveryPresentationCard, PrimaryAction: action, ExternalActionURL: url,
	})
	if err != nil || result.Status != notificationapp.DriverResultProviderAccepted {
		t.Fatalf("WeCom card result=%#v err=%v", result, err)
	}
	wecomBody := bodies[3]
	if wecomBody["msgtype"] != "template_card" {
		t.Fatalf("WeCom card envelope=%#v", wecomBody)
	}
	encoded = jsonWithoutHTMLEscape(t, wecomBody)
	if strings.Count(encoded, url) != 1 {
		t.Fatalf("WeCom card action=%s", encoded)
	}
}

func TestEnterpriseApplicationDriversRetryConfirmedRateLimitAndServerResponses(t *testing.T) {
	feishuClient := enterpriseHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			return enterpriseHTTPResponse(http.StatusOK, `{"code":0,"tenant_access_token":"tenant-token","expire":7200}`), nil
		}
		return enterpriseHTTPResponse(http.StatusTooManyRequests, `{"code":99991663,"msg":"rate limited"}`), nil
	})
	feishu := NewDriverRegistryWithHTTPClient(nil, feishuClient).Driver(domain.ChannelTypeFeishuApp).(notificationapp.ResultChannelDriver)
	result, err := feishu.SendResult(context.Background(), notificationapp.DriverMessage{
		Channel: domain.Channel{ChannelType: domain.ChannelTypeFeishuApp, ScopeID: "local", ChannelCode: "feishu", ConfigJSON: `{"appId":"cli_test"}`}, SecretPlain: "app-secret",
		IdentityKind: domain.ExternalIdentityFeishuOpenID, Target: "ou_target", Text: "任务需要处理", DeliveryID: "delivery-feishu-429",
	})
	if err != nil || result.Status != notificationapp.DriverResultFailed || result.FailureClass != "THROTTLED" || !result.Retryable {
		t.Fatalf("Feishu 429 result=%#v err=%v", result, err)
	}

	weComClient := enterpriseHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/cgi-bin/gettoken" {
			return enterpriseHTTPResponse(http.StatusOK, `{"errcode":0,"access_token":"wecom-token","expires_in":7200}`), nil
		}
		return enterpriseHTTPResponse(http.StatusServiceUnavailable, `{"errcode":-1,"errmsg":"server busy"}`), nil
	})
	weCom := NewDriverRegistryWithHTTPClient(nil, weComClient).Driver(domain.ChannelTypeWeComApp).(notificationapp.ResultChannelDriver)
	result, err = weCom.SendResult(context.Background(), notificationapp.DriverMessage{
		Channel: domain.Channel{ChannelType: domain.ChannelTypeWeComApp, ScopeID: "local", ChannelCode: "wecom", ConfigJSON: `{"corpId":"ww_test","agentId":"100001"}`}, SecretPlain: "corp-secret",
		IdentityKind: domain.ExternalIdentityWeComUserID, Target: "member-7", Text: "任务需要处理", DeliveryID: "delivery-wecom-503",
	})
	if err != nil || result.Status != notificationapp.DriverResultFailed || result.FailureClass != "TRANSIENT" || result.Diagnostic != "WECOM_SERVER" || !result.Retryable {
		t.Fatalf("WeCom 503 result=%#v err=%v", result, err)
	}
}

func jsonWithoutHTMLEscape(t *testing.T, value any) string {
	t.Helper()
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		t.Fatalf("encode JSON: %v", err)
	}
	return strings.TrimSpace(buffer.String())
}

func TestEnterpriseApplicationDriverRejectsWeComAggregateTargetBeforeHTTP(t *testing.T) {
	calls := 0
	client := enterpriseHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return enterpriseHTTPResponse(http.StatusOK, `{}`), nil
	})
	driver := &enterpriseApplicationDriver{provider: domain.ChannelTypeWeComApp, client: client, tokens: newEnterpriseTokenCache()}
	result, err := driver.SendResult(context.Background(), notificationapp.DriverMessage{
		Channel:     domain.Channel{ChannelType: domain.ChannelTypeWeComApp, ScopeID: "local", ChannelCode: "wecom", ConfigJSON: `{"corpId":"ww_test","agentId":"100001"}`},
		SecretPlain: "corp-secret",
		Target:      "member-one|@all",
		Text:        "一条短文本",
	})
	if err != nil || result.Status != notificationapp.DriverResultFailed || result.FailureClass != "INVALID_TARGET" {
		t.Fatalf("aggregate target result=%#v err=%v, want invalid target failure", result, err)
	}
	if calls != 0 {
		t.Fatalf("aggregate target must not trigger token or send HTTP calls=%d", calls)
	}
}

func TestFeishuApplicationDriverSendsGroupWithChatIDReceiveType(t *testing.T) {
	requests := make([]*http.Request, 0, 2)
	bodies := make([]map[string]any, 0, 2)
	client := enterpriseHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		var body map[string]any
		if request.Body != nil {
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				return nil, err
			}
		}
		requests = append(requests, request.Clone(request.Context()))
		bodies = append(bodies, body)
		switch request.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			return enterpriseHTTPResponse(http.StatusOK, `{"code":0,"tenant_access_token":"tenant-token","expire":7200}`), nil
		case "/open-apis/im/v1/messages":
			return enterpriseHTTPResponse(http.StatusOK, `{"code":0,"data":{"message_id":"om_group"}}`), nil
		default:
			return enterpriseHTTPResponse(http.StatusNotFound, `{}`), nil
		}
	})
	driver := &enterpriseApplicationDriver{provider: domain.ChannelTypeFeishuApp, client: client, tokens: newEnterpriseTokenCache()}
	result, err := driver.SendResult(context.Background(), notificationapp.DriverMessage{
		Channel:      domain.Channel{ChannelType: domain.ChannelTypeFeishuApp, ScopeID: "local", ChannelCode: "feishu", ConfigJSON: `{"appId":"cli_test"}`},
		SecretPlain:  "app-secret",
		IdentityKind: domain.ExternalIdentityFeishuChatID,
		Target:       "oc_group_123",
		Text:         "一条短文本",
		DeliveryID:   "ntf_group_123",
	})
	if err != nil || result.Status != notificationapp.DriverResultProviderAccepted || result.ProviderReference != "om_group" {
		t.Fatalf("Feishu group result=%#v err=%v", result, err)
	}
	if len(requests) != 2 || requests[1].URL.Query().Get("receive_id_type") != "chat_id" || bodies[1]["receive_id"] != "oc_group_123" {
		t.Fatalf("Feishu group requests=%#v bodies=%#v, want chat_id request", requests, bodies)
	}
}

func TestFeishuApplicationDriverReturnsSanitizedSourceError(t *testing.T) {
	client := enterpriseHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			return enterpriseHTTPResponse(http.StatusOK, `{"code":0,"tenant_access_token":"tenant-token","expire":7200}`), nil
		case "/open-apis/im/v1/messages":
			return enterpriseHTTPResponse(http.StatusBadRequest, `{"code":230001,"msg":"The group oc_sensitive_target is unavailable; access_token=secret-value","error":{"log_id":"feishu-log-123"}}`), nil
		default:
			return enterpriseHTTPResponse(http.StatusNotFound, `{}`), nil
		}
	})
	driver := &enterpriseApplicationDriver{provider: domain.ChannelTypeFeishuApp, client: client, tokens: newEnterpriseTokenCache()}
	result, err := driver.SendResult(context.Background(), notificationapp.DriverMessage{
		Channel:      domain.Channel{ChannelType: domain.ChannelTypeFeishuApp, ScopeID: "local", ChannelCode: "feishu", ConfigJSON: `{"appId":"cli_test"}`},
		SecretPlain:  "app-secret",
		IdentityKind: domain.ExternalIdentityFeishuChatID,
		Target:       "oc_group_123",
		Text:         "一条短文本",
		DeliveryID:   "ntf_group_failure",
	})
	if err != nil || result.Status != notificationapp.DriverResultFailed || result.FailureClass != "AUTHENTICATION" || result.ProviderError == nil {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if result.ProviderError.Provider != domain.ChannelTypeFeishuApp || result.ProviderError.HTTPStatus != http.StatusBadRequest || result.ProviderError.Code != "230001" || result.ProviderError.LogID != "feishu-log-123" {
		t.Fatalf("provider error=%#v", result.ProviderError)
	}
	if result.ProviderError.Message != "The group [redacted] is unavailable; access_token=[redacted]" {
		t.Fatalf("provider message=%q", result.ProviderError.Message)
	}
}

func TestWeComApplicationDriverReturnsSourceError(t *testing.T) {
	client := enterpriseHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/cgi-bin/gettoken":
			return enterpriseHTTPResponse(http.StatusOK, `{"errcode":0,"access_token":"wecom-token","expires_in":7200}`), nil
		case "/cgi-bin/message/send":
			return enterpriseHTTPResponse(http.StatusOK, `{"errcode":81013,"errmsg":"invalid user: member-one"}`), nil
		default:
			return enterpriseHTTPResponse(http.StatusNotFound, `{}`), nil
		}
	})
	driver := &enterpriseApplicationDriver{provider: domain.ChannelTypeWeComApp, client: client, tokens: newEnterpriseTokenCache()}
	result, err := driver.SendResult(context.Background(), notificationapp.DriverMessage{
		Channel:     domain.Channel{ChannelType: domain.ChannelTypeWeComApp, ScopeID: "local", ChannelCode: "wecom", ConfigJSON: `{"corpId":"ww_test","agentId":"100001"}`},
		SecretPlain: "corp-secret",
		Target:      "member-one",
		Text:        "一条短文本",
	})
	if err != nil || result.Status != notificationapp.DriverResultFailed || result.FailureClass != "INVALID_TARGET" || result.ProviderError == nil {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if result.ProviderError.Provider != domain.ChannelTypeWeComApp || result.ProviderError.HTTPStatus != http.StatusOK || result.ProviderError.Code != "81013" || result.ProviderError.Message != "invalid user: [redacted]" {
		t.Fatalf("provider error=%#v", result.ProviderError)
	}
}

func TestWeComGenericRejectionIncludesOnlySanitizedProviderCode(t *testing.T) {
	client := enterpriseHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/cgi-bin/gettoken":
			return enterpriseHTTPResponse(http.StatusOK, `{"errcode":0,"access_token":"wecom-token","expires_in":7200}`), nil
		case "/cgi-bin/message/send":
			return enterpriseHTTPResponse(http.StatusOK, `{"errcode":40058,"errmsg":"invalid button url https://sensitive.example/path?token=secret"}`), nil
		default:
			return enterpriseHTTPResponse(http.StatusNotFound, `{}`), nil
		}
	})
	driver := &enterpriseApplicationDriver{provider: domain.ChannelTypeWeComApp, client: client, tokens: newEnterpriseTokenCache()}
	result, err := driver.SendResult(context.Background(), notificationapp.DriverMessage{
		Channel:     domain.Channel{ChannelType: domain.ChannelTypeWeComApp, ScopeID: "local", ChannelCode: "wecom", ConfigJSON: `{"corpId":"ww_test","agentId":"100001"}`},
		SecretPlain: "corp-secret",
		Target:      "member-one",
		Text:        "一条短文本",
	})
	if err != nil || result.Status != notificationapp.DriverResultFailed || result.FailureClass != "PROVIDER_REJECTED" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if result.Diagnostic != "WECOM_REJECTED_40058" {
		t.Fatalf("diagnostic=%q", result.Diagnostic)
	}
	if result.ProviderError == nil || result.ProviderError.Code != "40058" || strings.Contains(result.ProviderError.Message, "sensitive.example") || strings.Contains(result.ProviderError.Message, "secret") {
		t.Fatalf("provider error=%#v", result.ProviderError)
	}
}

func TestEnterpriseApplicationDriverDropsDisallowedWeComMention(t *testing.T) {
	var messageBody map[string]any
	client := enterpriseHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/cgi-bin/gettoken":
			return enterpriseHTTPResponse(http.StatusOK, `{"errcode":0,"access_token":"wecom-token","expires_in":7200}`), nil
		case "/cgi-bin/message/send":
			if err := json.NewDecoder(request.Body).Decode(&messageBody); err != nil {
				return nil, err
			}
			return enterpriseHTTPResponse(http.StatusOK, `{"errcode":0,"msgid":"msg_123"}`), nil
		default:
			return enterpriseHTTPResponse(http.StatusNotFound, `{}`), nil
		}
	})
	driver := &enterpriseApplicationDriver{provider: domain.ChannelTypeWeComApp, client: client, tokens: newEnterpriseTokenCache()}
	result, err := driver.SendResult(context.Background(), notificationapp.DriverMessage{
		Channel:        domain.Channel{ChannelType: domain.ChannelTypeWeComApp, ScopeID: "local", ChannelCode: "wecom", ConfigJSON: `{"corpId":"ww_test","agentId":"100001"}`},
		SecretPlain:    "corp-secret",
		Target:         "member-one",
		Text:           "一条短文本",
		ProviderParams: map[string]any{domain.ProviderParameterMentionedList: []string{"@all"}},
	})
	if err != nil || result.Status != notificationapp.DriverResultProviderAccepted {
		t.Fatalf("disallowed optional mention result=%#v err=%v", result, err)
	}
	text, ok := messageBody["text"].(map[string]any)
	if !ok {
		t.Fatalf("WeCom body text=%#v", messageBody["text"])
	}
	if _, exists := text["mentioned_list"]; exists {
		t.Fatalf("adapter must not forward disallowed mention: %#v", text)
	}
}

func TestEnterpriseTokenRefreshIsSingleFlightPerConnectionRevision(t *testing.T) {
	var mu sync.Mutex
	tokenCalls := 0
	client := enterpriseHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			mu.Lock()
			tokenCalls++
			mu.Unlock()
			time.Sleep(20 * time.Millisecond)
			return enterpriseHTTPResponse(http.StatusOK, `{"code":0,"tenant_access_token":"token","expire":7200}`), nil
		case "/open-apis/im/v1/messages":
			return enterpriseHTTPResponse(http.StatusOK, `{"code":0,"data":{"message_id":"om_ok"}}`), nil
		default:
			return enterpriseHTTPResponse(http.StatusNotFound, `{}`), nil
		}
	})
	driver := &enterpriseApplicationDriver{provider: domain.ChannelTypeFeishuApp, client: client, tokens: newEnterpriseTokenCache()}
	const sends = 12
	var wg sync.WaitGroup
	errs := make(chan error, sends)
	for index := 0; index < sends; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			result, err := driver.SendResult(context.Background(), notificationapp.DriverMessage{
				Channel:     domain.Channel{ChannelType: domain.ChannelTypeFeishuApp, ScopeID: "local", ChannelCode: "feishu", ConfigJSON: `{"appId":"cli_test"}`},
				SecretPlain: "secret",
				Target:      fmt.Sprintf("ou_%d", index),
				Text:        "test",
				DeliveryID:  fmt.Sprintf("ntf_%d", index),
			})
			if err != nil || result.Status != notificationapp.DriverResultProviderAccepted {
				errs <- fmt.Errorf("result=%#v err=%v", result, err)
			}
		}(index)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if tokenCalls != 1 {
		t.Fatalf("token calls=%d, want one single-flight refresh", tokenCalls)
	}
}

func TestEnterpriseRejectedTokenTriggersOneForcedRefresh(t *testing.T) {
	var mu sync.Mutex
	tokenCalls := 0
	client := enterpriseHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			mu.Lock()
			tokenCalls++
			call := tokenCalls
			mu.Unlock()
			if call == 1 {
				return enterpriseHTTPResponse(http.StatusOK, `{"code":0,"tenant_access_token":"stale-token","expire":7200}`), nil
			}
			return enterpriseHTTPResponse(http.StatusOK, `{"code":0,"tenant_access_token":"fresh-token","expire":7200}`), nil
		case "/open-apis/im/v1/messages":
			if request.Header.Get("Authorization") == "Bearer stale-token" {
				return enterpriseHTTPResponse(http.StatusOK, `{"code":99991663}`), nil
			}
			if request.Header.Get("Authorization") == "Bearer fresh-token" {
				return enterpriseHTTPResponse(http.StatusOK, `{"code":0,"data":{"message_id":"om_ok"}}`), nil
			}
		}
		return enterpriseHTTPResponse(http.StatusNotFound, `{}`), nil
	})
	driver := &enterpriseApplicationDriver{provider: domain.ChannelTypeFeishuApp, client: client, tokens: newEnterpriseTokenCache()}
	const sends = 12
	var wg sync.WaitGroup
	errs := make(chan error, sends)
	for index := 0; index < sends; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			result, err := driver.SendResult(context.Background(), notificationapp.DriverMessage{
				Channel:     domain.Channel{ChannelType: domain.ChannelTypeFeishuApp, ScopeID: "local", ChannelCode: "feishu", ConfigJSON: `{"appId":"cli_test"}`},
				SecretPlain: "secret",
				Target:      fmt.Sprintf("ou_%d", index),
				Text:        "test",
				DeliveryID:  fmt.Sprintf("ntf_%d", index),
			})
			if err != nil || result.Status != notificationapp.DriverResultProviderAccepted {
				errs <- fmt.Errorf("result=%#v err=%v", result, err)
			}
		}(index)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if tokenCalls != 2 {
		t.Fatalf("token calls=%d, want initial token plus one forced refresh", tokenCalls)
	}
}

func TestEnterpriseTokenExpiryUsesSkewAndBoundedJitter(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	cache := newEnterpriseTokenCache()
	cache.now = func() time.Time { return now }
	cache.jitter = func(max time.Duration) time.Duration {
		if max != tokenRefreshJitterMax {
			t.Fatalf("jitter max=%s, want %s", max, tokenRefreshJitterMax)
		}
		return 7 * time.Second
	}
	if got, want := cache.expiry(7200), now.Add(7200*time.Second-tokenRefreshSkew-7*time.Second); !got.Equal(want) {
		t.Fatalf("expiry=%s want=%s", got, want)
	}
}

func TestEnterpriseApplicationDriverRejectsFakeIPBeforeAnyDial(t *testing.T) {
	var dialed int
	guard := outboundurl.NewOutboundURLGuard(outboundurl.Options{
		Resolver: enterpriseStaticResolver{addresses: map[string][]net.IPAddr{
			"open.feishu.cn": {{IP: net.ParseIP("198.18.7.9")}},
		}},
		InterfaceAddrs: func() ([]net.Addr, error) { return nil, nil },
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			dialed++
			return nil, errors.New("must not dial fake IP")
		},
	})
	registry := NewDriverRegistryWithOutboundGuard(nil, guard)
	driver, ok := registry.Driver(domain.ChannelTypeFeishuApp).(notificationapp.ResultChannelDriver)
	if !ok {
		t.Fatal("guarded Feishu driver is missing")
	}
	result, err := driver.SendResult(context.Background(), notificationapp.DriverMessage{
		Channel:     domain.Channel{ChannelType: domain.ChannelTypeFeishuApp, ScopeID: "local", ChannelCode: "feishu", ConfigJSON: `{"appId":"cli_test"}`},
		SecretPlain: "secret",
		Target:      "ou_target",
		Text:        "test",
		DeliveryID:  "ntf_fake_ip",
	})
	if err != nil || result.Status != notificationapp.DriverResultFailed || result.Diagnostic != "TOKEN_TRANSPORT" || !result.Retryable {
		t.Fatalf("fake-IP result=%#v err=%v", result, err)
	}
	if dialed != 0 {
		t.Fatalf("official fixed host resolving to fake IP dialed %d times", dialed)
	}
}

type enterpriseHTTPDoerFunc func(*http.Request) (*http.Response, error)

func (f enterpriseHTTPDoerFunc) Do(request *http.Request) (*http.Response, error) { return f(request) }

func enterpriseHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type enterpriseStaticResolver struct {
	addresses map[string][]net.IPAddr
}

func (r enterpriseStaticResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	return append([]net.IPAddr(nil), r.addresses[host]...), nil
}
