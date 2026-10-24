package external_login

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
)

type enterpriseRoundTrip func(*http.Request) (*http.Response, error)

func (f enterpriseRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestFeishuEnterpriseAdapterUsesAuthorizationCodeWithoutClaimingPKCEOrNonce(t *testing.T) {
	adapter := &feishuEnterpriseAdapter{}
	capabilities := adapter.Capabilities()
	if !capabilities.SupportsAuthorizationCode || !capabilities.SupportsUserInfo || capabilities.SupportsPKCE || capabilities.SupportsOIDCNonce {
		t.Fatalf("unexpected capabilities: %#v", capabilities)
	}
	redirect, err := adapter.BuildAuthorizationURL(domain.EnterpriseConnection{AppID: "cli_test"}, "state-1", "nonce-ignored", "challenge-ignored", "https://console.example/external-identities/oauth/callback/feishu")
	if err != nil || !strings.Contains(redirect, "app_id=cli_test") || !strings.Contains(redirect, "state=state-1") || strings.Contains(redirect, "code_challenge") {
		t.Fatalf("authorization URL=%q err=%v", redirect, err)
	}
}

func TestFeishuEnterpriseAdapterClassifiesGroupDiscoveryScopeRejectionWithoutLeakingProviderBody(t *testing.T) {
	client := &http.Client{Transport: enterpriseRoundTrip(func(req *http.Request) (*http.Response, error) {
		body := `{"code":0,"tenant_access_token":"tenant-token"}`
		status := http.StatusOK
		if strings.Contains(req.URL.Path, "/im/v1/chats") {
			status = http.StatusForbidden
			body = `{"code":99991672,"msg":"Access denied. One of the following scopes is required: [im:chat:readonly, im:chat], sensitive-provider-detail"}`
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	adapter := &feishuEnterpriseAdapter{client: client}
	_, err := adapter.DiscoverGroups(context.Background(), domain.EnterpriseConnection{AppID: "cli_test"}, "secret")
	if err == nil {
		t.Fatal("group discovery without scope succeeded")
	}
	var rejected interface{ GroupDiscoveryRejection() (string, string) }
	if !errors.As(err, &rejected) {
		t.Fatalf("error=%T, want classified group discovery rejection", err)
	}
	reason, message := rejected.GroupDiscoveryRejection()
	if reason != "ENTERPRISE_GROUP_DISCOVERY_FEISHU_SCOPE_REQUIRED" || !strings.Contains(message, "im:chat:readonly") {
		t.Fatalf("reason=%q message=%q", reason, message)
	}
	if strings.Contains(err.Error(), "99991672") || strings.Contains(err.Error(), "sensitive-provider-detail") || strings.Contains(err.Error(), "tenant-token") {
		t.Fatalf("provider response escaped: %v", err)
	}
}

func TestWeComEnterpriseAdapterResolvesOnlyUserID(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: enterpriseRoundTrip(func(req *http.Request) (*http.Response, error) {
		requests++
		body := `{"errcode":0,"access_token":"app-token"}`
		if strings.Contains(req.URL.Path, "getuserinfo") {
			body = `{"errcode":0,"UserId":"member-42"}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	adapter := &weComEnterpriseAdapter{client: client}
	resolved, err := adapter.ExchangeAndResolve(context.Background(), domain.EnterpriseConnection{CorpID: "ww-test"}, "secret", "code", "", "")
	if err != nil || resolved.Subject != "member-42" || resolved.SubjectKind != "WECOM_USERID" || requests != 2 {
		t.Fatalf("subject=%q kind=%q requests=%d err=%v", resolved.Subject, resolved.SubjectKind, requests, err)
	}
}

func TestWeComEnterpriseAdapterClassifiesRejectedAuthorizationWithoutLeakingProviderBody(t *testing.T) {
	client := &http.Client{Transport: enterpriseRoundTrip(func(req *http.Request) (*http.Response, error) {
		body := `{"errcode":0,"access_token":"app-token"}`
		if strings.Contains(req.URL.Path, "getuserinfo") {
			body = `{"errcode":40029,"errmsg":"invalid code with sensitive provider detail"}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	adapter := &weComEnterpriseAdapter{client: client}
	_, err := adapter.ExchangeAndResolve(context.Background(), domain.EnterpriseConnection{CorpID: "ww-test"}, "secret", "single-use-code", "", "")
	if err == nil {
		t.Fatal("rejected enterprise authorization succeeded")
	}
	var rejected interface{ AuthorizationRejection() (string, string) }
	if !errors.As(err, &rejected) {
		t.Fatalf("error=%T, want classified authorization rejection", err)
	}
	reason, message := rejected.AuthorizationRejection()
	if reason != "ENTERPRISE_AUTHORIZATION_PROVIDER_REJECTED" || !strings.Contains(message, "当前成员") {
		t.Fatalf("reason=%q message=%q", reason, message)
	}
	if strings.Contains(err.Error(), "40029") || strings.Contains(err.Error(), "sensitive provider detail") || strings.Contains(err.Error(), "single-use-code") {
		t.Fatalf("provider response escaped: %v", err)
	}
}

func TestWeComEnterpriseAdapterClassifiesTrustedIPRejection(t *testing.T) {
	client := &http.Client{Transport: enterpriseRoundTrip(func(req *http.Request) (*http.Response, error) {
		body := `{"errcode":0,"access_token":"app-token"}`
		if strings.Contains(req.URL.Path, "getuserinfo") {
			body = `{"errcode":60020,"errmsg":"not allow to access from your ip: 192.0.2.42"}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	adapter := &weComEnterpriseAdapter{client: client}
	_, err := adapter.ExchangeAndResolve(context.Background(), domain.EnterpriseConnection{CorpID: "ww-test"}, "secret", "single-use-code", "", "")
	var rejected interface{ AuthorizationRejection() (string, string) }
	if err == nil || !errors.As(err, &rejected) {
		t.Fatalf("error=%v, want trusted IP rejection", err)
	}
	reason, message := rejected.AuthorizationRejection()
	if reason != "ENTERPRISE_AUTHORIZATION_WECOM_TRUSTED_IP_REQUIRED" || !strings.Contains(message, "企业可信 IP") {
		t.Fatalf("reason=%q message=%q", reason, message)
	}
	if strings.Contains(err.Error(), "192.0.2.42") {
		t.Fatalf("provider response escaped: %v", err)
	}
}

func TestWeComEnterpriseAdapterProbeValidatesAgentAndTrustedIP(t *testing.T) {
	tests := []struct {
		name       string
		agentBody  string
		wantReason string
	}{
		{name: "success", agentBody: `{"errcode":0,"agentid":1000003}`},
		{name: "trusted ip", agentBody: `{"errcode":60020,"errmsg":"not allow to access from your ip, from ip: 192.0.2.42"}`, wantReason: "ENTERPRISE_CONNECTION_WECOM_TRUSTED_IP_REQUIRED"},
		{name: "agent mismatch", agentBody: `{"errcode":0,"agentid":1000002}`, wantReason: "ENTERPRISE_CONNECTION_WECOM_AGENT_MISMATCH"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: enterpriseRoundTrip(func(req *http.Request) (*http.Response, error) {
				body := `{"errcode":0,"access_token":"app-token"}`
				if strings.Contains(req.URL.Path, "/agent/get") {
					body = tc.agentBody
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			err := (&weComEnterpriseAdapter{client: client}).Probe(context.Background(), domain.EnterpriseConnection{CorpID: "ww-test", AgentID: "1000003"}, "secret")
			if tc.wantReason == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var rejected interface{ ConnectionRejection() (string, string) }
			if err == nil || !errors.As(err, &rejected) {
				t.Fatalf("error=%v, want classified connection rejection", err)
			}
			reason, message := rejected.ConnectionRejection()
			if reason != tc.wantReason {
				t.Fatalf("reason=%q", reason)
			}
			if tc.name == "trusted ip" && !strings.Contains(message, "192.0.2.42") {
				t.Fatalf("trusted IP guidance=%q", message)
			}
		})
	}
}

func TestWeComRejectedIPOnlyReturnsValidIPv4(t *testing.T) {
	if got := weComRejectedIP("not allow, from ip: 27.36.105.32, hint 123"); got != "27.36.105.32" {
		t.Fatalf("ip=%q", got)
	}
	if got := weComRejectedIP("not allow, from ip: 999.999.999.999"); got != "" {
		t.Fatalf("invalid ip escaped=%q", got)
	}
}

func TestWeComEnterpriseAdapterUsesBrowserQRCodeAuthorization(t *testing.T) {
	adapter := &weComEnterpriseAdapter{}
	redirect, err := adapter.BuildAuthorizationURL(
		domain.EnterpriseConnection{CorpID: "ww-corp", AgentID: "1000002"},
		"state-1",
		"nonce-ignored",
		"challenge-ignored",
		"https://console.example/external-identities/oauth/callback/wecom",
	)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(redirect)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "https" || parsed.Host != "open.work.weixin.qq.com" || parsed.Path != "/wwopen/sso/qrConnect" {
		t.Fatalf("authorization endpoint=%q", redirect)
	}
	query := parsed.Query()
	if query.Get("appid") != "ww-corp" || query.Get("agentid") != "1000002" || query.Get("state") != "state-1" || query.Get("redirect_uri") != "https://console.example/external-identities/oauth/callback/wecom" {
		t.Fatalf("authorization query=%v", query)
	}
	if query.Has("scope") || query.Has("response_type") || parsed.Fragment != "" {
		t.Fatalf("browser QR authorization contains in-client OAuth parameters: %q", redirect)
	}
}

func TestWeComEnterpriseAdapterRequiresAgentIDForBrowserQRCodeAuthorization(t *testing.T) {
	adapter := &weComEnterpriseAdapter{}
	if _, err := adapter.BuildAuthorizationURL(domain.EnterpriseConnection{CorpID: "ww-corp"}, "state-1", "", "", "https://console.example/callback"); err == nil {
		t.Fatal("missing agent id was accepted")
	}
}

func TestEnterpriseProviderTransportLossIsOutcomeUnknownWithoutLeakingBody(t *testing.T) {
	client := &http.Client{Transport: enterpriseRoundTrip(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("access_token=must-not-escape")
	})}
	adapter := &feishuEnterpriseAdapter{client: client}
	_, err := adapter.ExchangeAndResolve(context.Background(), domain.EnterpriseConnection{AppID: "app"}, "secret", "single-use-code", "", "https://console.example/callback")
	if err == nil {
		t.Fatal("transport loss succeeded")
	}
	var uncertain interface{ OutcomeUnknown() bool }
	if !errors.As(err, &uncertain) || !uncertain.OutcomeUnknown() {
		t.Fatalf("error=%v, want outcome unknown", err)
	}
	if strings.Contains(err.Error(), "access_token") || strings.Contains(err.Error(), "single-use-code") {
		t.Fatalf("sensitive transport error escaped: %v", err)
	}
}

func TestEnterpriseProviderRejectsOversizedAndRedirectResponses(t *testing.T) {
	redirectClient := &http.Client{Transport: enterpriseRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"https://evil.example"}}, Body: io.NopCloser(strings.NewReader("redirect"))}, nil
	})}
	if err := (&feishuEnterpriseAdapter{client: redirectClient}).Probe(context.Background(), domain.EnterpriseConnection{AppID: "app"}, "secret"); err == nil {
		t.Fatal("provider redirect was accepted")
	}
	oversized := strings.Repeat("x", enterpriseResponseLimit+1)
	largeClient := &http.Client{Transport: enterpriseRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(oversized))}, nil
	})}
	if err := (&feishuEnterpriseAdapter{client: largeClient}).Probe(context.Background(), domain.EnterpriseConnection{AppID: "app"}, "secret"); err == nil {
		t.Fatal("oversized provider response was accepted")
	}
}

func TestLocalFakeEnterpriseAdapterExercisesOAuthAndMessageContracts(t *testing.T) {
	for _, provider := range []domain.EnterpriseProvider{domain.EnterpriseProviderFeishu, domain.EnterpriseProviderWeCom} {
		t.Run(string(provider), func(t *testing.T) {
			var sawExchange, sawMessage bool
			expectedKind := domain.SubjectKindForProvider(provider)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch req.URL.Path {
				case "/probe":
					_, _ = io.WriteString(w, `{"ok":true}`)
				case "/exchange":
					sawExchange = true
					_, _ = io.WriteString(w, `{"subject":"member-42","subjectKind":"`+expectedKind+`","nonce":"nonce-1","pkceVerified":true}`)
				case "/message":
					sawMessage = true
					_, _ = io.WriteString(w, `{"accepted":true}`)
				default:
					http.NotFound(w, req)
				}
			}))
			defer server.Close()
			adapter := &fakeEnterpriseAdapter{client: server.Client(), provider: provider, baseURL: server.URL}
			if err := adapter.Probe(context.Background(), domain.EnterpriseConnection{AppID: "app"}, "secret"); err != nil {
				t.Fatal(err)
			}
			resolved, err := adapter.ExchangeAndResolve(context.Background(), domain.EnterpriseConnection{AppID: "app"}, "secret", "code", "verifier", "http://127.0.0.1/callback")
			if err != nil || resolved.Subject != "member-42" || resolved.SubjectKind != expectedKind || resolved.Nonce != "nonce-1" {
				t.Fatalf("resolved=%#v err=%v", resolved, err)
			}
			if err = adapter.SendPersonalTest(context.Background(), domain.EnterpriseConnection{AppID: "app"}, "secret", resolved.Subject); err != nil {
				t.Fatal(err)
			}
			if !sawExchange || !sawMessage {
				t.Fatalf("exchange=%t message=%t", sawExchange, sawMessage)
			}
		})
	}
}

func TestLocalFakeEnterpriseAdapterRejectsProviderPKCEMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"subject":"member-42","subjectKind":"FEISHU_OPEN_ID","nonce":"nonce-1","pkceVerified":false}`)
	}))
	defer server.Close()
	adapter := &fakeEnterpriseAdapter{client: server.Client(), provider: domain.EnterpriseProviderFeishu, baseURL: server.URL}
	if _, err := adapter.ExchangeAndResolve(context.Background(), domain.EnterpriseConnection{AppID: "app"}, "secret", "code", "wrong-verifier", "http://127.0.0.1/callback"); err == nil {
		t.Fatal("provider PKCE mismatch was accepted")
	}
}
