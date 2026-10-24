package domain

import "testing"

func TestWebhookProfileSecretAcceptsOnlyFixedProviderEndpoints(t *testing.T) {
	tests := []struct {
		name        string
		channelType string
		secret      WebhookProfileSecret
	}{
		{
			name:        "Feishu fixed hook with signing secret",
			channelType: ChannelTypeFeishuWebhook,
			secret: WebhookProfileSecret{
				EndpointURL:   "https://open.feishu.cn/open-apis/bot/v2/hook/hook-token",
				SigningSecret: "signing-secret",
			},
		},
		{
			name:        "WeCom fixed hook key",
			channelType: ChannelTypeWeComWebhook,
			secret: WebhookProfileSecret{
				EndpointURL: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=group-key",
			},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			normalized, err := NormalizeWebhookProfileSecret(testCase.channelType, testCase.secret)
			if err != nil {
				t.Fatalf("NormalizeWebhookProfileSecret() error = %v", err)
			}
			if normalized != testCase.secret {
				t.Fatalf("normalized secret = %#v, want %#v", normalized, testCase.secret)
			}
		})
	}
}

func TestWebhookProfileSecretRejectsCallerControlledEndpointShapes(t *testing.T) {
	tests := []struct {
		name        string
		channelType string
		secret      WebhookProfileSecret
	}{
		{
			name:        "non HTTPS endpoint",
			channelType: ChannelTypeFeishuWebhook,
			secret:      WebhookProfileSecret{EndpointURL: "http://open.feishu.cn/open-apis/bot/v2/hook/hook-token"},
		},
		{
			name:        "Feishu arbitrary host",
			channelType: ChannelTypeFeishuWebhook,
			secret:      WebhookProfileSecret{EndpointURL: "https://receiver.example/open-apis/bot/v2/hook/hook-token"},
		},
		{
			name:        "Feishu query override",
			channelType: ChannelTypeFeishuWebhook,
			secret:      WebhookProfileSecret{EndpointURL: "https://open.feishu.cn/open-apis/bot/v2/hook/hook-token?target=other-group"},
		},
		{
			name:        "WeCom missing key",
			channelType: ChannelTypeWeComWebhook,
			secret:      WebhookProfileSecret{EndpointURL: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send"},
		},
		{
			name:        "WeCom extra query override",
			channelType: ChannelTypeWeComWebhook,
			secret:      WebhookProfileSecret{EndpointURL: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=group-key&to=other-group"},
		},
		{
			name:        "WeCom signing secret",
			channelType: ChannelTypeWeComWebhook,
			secret: WebhookProfileSecret{
				EndpointURL:   "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=group-key",
				SigningSecret: "not-supported",
			},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := NormalizeWebhookProfileSecret(testCase.channelType, testCase.secret); err == nil {
				t.Fatalf("NormalizeWebhookProfileSecret(%q, %#v) unexpectedly succeeded", testCase.channelType, testCase.secret)
			}
		})
	}
}

func TestWebhookProfileConfigIsBoundedAndDoesNotAcceptUnknownFields(t *testing.T) {
	normalized, err := NormalizeWebhookProfileConfig(WebhookProfileConfig{})
	if err != nil {
		t.Fatalf("NormalizeWebhookProfileConfig() error = %v", err)
	}
	if normalized.TimeoutMilliseconds != 5000 || len(normalized.SuccessStatusCodes) != 0 {
		t.Fatalf("default profile config = %#v", normalized)
	}
	if _, err := NormalizeWebhookProfileConfig(WebhookProfileConfig{TimeoutMilliseconds: 999}); err == nil {
		t.Fatal("too-short profile timeout unexpectedly accepted")
	}
	if _, err := ParseWebhookProfileConfig(`{"timeoutMilliseconds":5000,"rawBody":"must-not-exist"}`); err == nil {
		t.Fatal("profile config accepted an unknown raw-body field")
	}
}
