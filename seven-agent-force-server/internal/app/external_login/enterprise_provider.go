package external_login

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	externalapp "github.com/CaixyPromise/seven-agent/force/internal/app/external_login/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
)

const enterpriseResponseLimit = 1 << 20

type enterpriseOAuthError struct {
	uncertain bool
	cause     error
}

func (e enterpriseOAuthError) Error() string        { return e.cause.Error() }
func (e enterpriseOAuthError) Unwrap() error        { return e.cause }
func (e enterpriseOAuthError) OutcomeUnknown() bool { return e.uncertain }

// enterpriseAuthorizationRejection classifies a provider response that is
// safe for the product layer to explain without exposing the upstream body,
// authorization code, token, or external subject.
type enterpriseAuthorizationRejection struct {
	reason  string
	message string
}

type enterpriseConnectionRejection struct {
	reason  string
	message string
}

type enterpriseGroupDiscoveryRejection struct {
	reason  string
	message string
}

func (e enterpriseConnectionRejection) Error() string { return "enterprise connection rejected" }
func (e enterpriseConnectionRejection) ConnectionRejection() (string, string) {
	return e.reason, e.message
}

func (e enterpriseGroupDiscoveryRejection) Error() string {
	return "enterprise group discovery rejected"
}
func (e enterpriseGroupDiscoveryRejection) GroupDiscoveryRejection() (string, string) {
	return e.reason, e.message
}

func (e enterpriseAuthorizationRejection) Error() string { return "enterprise authorization rejected" }
func (e enterpriseAuthorizationRejection) AuthorizationRejection() (string, string) {
	return e.reason, e.message
}

type enterpriseProviderRegistry struct {
	items map[domain.EnterpriseProvider]externalapp.EnterpriseProviderAdapter
}

func newEnterpriseProviderRegistry(client *http.Client, fakeBaseURL string) *enterpriseProviderRegistry {
	registry := &enterpriseProviderRegistry{items: map[domain.EnterpriseProvider]externalapp.EnterpriseProviderAdapter{}}
	fakeBaseURL = strings.TrimRight(strings.TrimSpace(fakeBaseURL), "/")
	if fakeBaseURL != "" {
		for _, provider := range []domain.EnterpriseProvider{domain.EnterpriseProviderFeishu, domain.EnterpriseProviderWeCom} {
			registry.items[provider] = &fakeEnterpriseAdapter{client: client, provider: provider, baseURL: fakeBaseURL}
		}
		return registry
	}
	for _, adapter := range []externalapp.EnterpriseProviderAdapter{
		&feishuEnterpriseAdapter{client: client}, &weComEnterpriseAdapter{client: client},
	} {
		registry.items[adapter.Provider()] = adapter
	}
	return registry
}

// fakeEnterpriseAdapter is only installed when the local/test-only config is
// explicitly present. It exercises the same service transactions while a
// controlled local upstream provides deterministic OAuth and message replies.
type fakeEnterpriseAdapter struct {
	client   *http.Client
	provider domain.EnterpriseProvider
	baseURL  string
}

func (a *fakeEnterpriseAdapter) Provider() domain.EnterpriseProvider { return a.provider }
func (*fakeEnterpriseAdapter) Capabilities() domain.EnterpriseProtocolCapabilities {
	return domain.EnterpriseProtocolCapabilities{SupportsAuthorizationCode: true, SupportsPKCE: true, SupportsOIDCNonce: true, SupportsUserInfo: true}
}
func (a *fakeEnterpriseAdapter) BuildAuthorizationURL(_ domain.EnterpriseConnection, state, nonce, challenge, redirectURI string) (string, error) {
	values := url.Values{"provider": {string(a.provider)}, "state": {state}, "nonce": {nonce}, "code_challenge": {challenge}, "redirect_uri": {redirectURI}}
	return a.baseURL + "/authorize?" + values.Encode(), nil
}
func (a *fakeEnterpriseAdapter) Probe(ctx context.Context, connection domain.EnterpriseConnection, secret string) error {
	var result struct {
		OK bool `json:"ok"`
	}
	return enterpriseJSON(ctx, a.client, http.MethodPost, a.baseURL+"/probe", map[string]string{"provider": string(a.provider), "app_id": connection.AppID, "corp_id": connection.CorpID, "secret": secret}, "", &result, func() error {
		if !result.OK {
			return fmt.Errorf("fake enterprise credential rejected")
		}
		return nil
	})
}
func (a *fakeEnterpriseAdapter) ExchangeAndResolve(ctx context.Context, connection domain.EnterpriseConnection, secret, code, verifier, redirectURI string) (externalapp.EnterpriseResolvedSubject, error) {
	var result struct {
		Subject      string `json:"subject"`
		SubjectKind  string `json:"subjectKind"`
		Nonce        string `json:"nonce"`
		PKCEVerified bool   `json:"pkceVerified"`
	}
	err := enterpriseJSON(ctx, a.client, http.MethodPost, a.baseURL+"/exchange", map[string]string{"provider": string(a.provider), "app_id": connection.AppID, "corp_id": connection.CorpID, "secret": secret, "code": code, "verifier": verifier, "redirect_uri": redirectURI}, "", &result, func() error {
		if strings.TrimSpace(result.Subject) == "" || result.SubjectKind != domain.SubjectKindForProvider(a.provider) || strings.TrimSpace(result.Nonce) == "" || !result.PKCEVerified {
			return fmt.Errorf("fake enterprise identity rejected")
		}
		return nil
	})
	return externalapp.EnterpriseResolvedSubject{Subject: result.Subject, SubjectKind: result.SubjectKind, Nonce: result.Nonce}, err
}
func (a *fakeEnterpriseAdapter) SendPersonalTest(ctx context.Context, connection domain.EnterpriseConnection, secret, subject string) error {
	var result struct {
		Accepted bool `json:"accepted"`
	}
	return enterpriseJSON(ctx, a.client, http.MethodPost, a.baseURL+"/message", map[string]string{"provider": string(a.provider), "app_id": connection.AppID, "corp_id": connection.CorpID, "agent_id": connection.AgentID, "secret": secret, "subject": subject}, "", &result, func() error {
		if !result.Accepted {
			return fmt.Errorf("fake enterprise message rejected")
		}
		return nil
	})
}
func (a *fakeEnterpriseAdapter) DiscoverGroups(ctx context.Context, connection domain.EnterpriseConnection, secret string) ([]externalapp.EnterpriseDiscoveredGroup, error) {
	if a.provider != domain.EnterpriseProviderFeishu {
		return nil, fmt.Errorf("group discovery unsupported")
	}
	var result struct {
		Groups []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			ChatID      string `json:"chatId"`
		} `json:"groups"`
	}
	err := enterpriseJSON(ctx, a.client, http.MethodPost, a.baseURL+"/groups", map[string]string{"provider": string(a.provider), "app_id": connection.AppID, "secret": secret}, "", &result, func() error { return nil })
	items := make([]externalapp.EnterpriseDiscoveredGroup, 0, len(result.Groups))
	for _, group := range result.Groups {
		items = append(items, externalapp.EnterpriseDiscoveredGroup{DisplayName: group.Name, Description: group.Description, Target: group.ChatID})
	}
	return items, err
}

func (r *enterpriseProviderRegistry) GetEnterprise(provider domain.EnterpriseProvider) (externalapp.EnterpriseProviderAdapter, bool) {
	if r == nil {
		return nil, false
	}
	item, ok := r.items[provider]
	return item, ok
}

type feishuEnterpriseAdapter struct{ client *http.Client }

func (*feishuEnterpriseAdapter) Provider() domain.EnterpriseProvider {
	return domain.EnterpriseProviderFeishu
}
func (*feishuEnterpriseAdapter) Capabilities() domain.EnterpriseProtocolCapabilities {
	return domain.EnterpriseProtocolCapabilities{SupportsAuthorizationCode: true, SupportsUserInfo: true}
}
func (*feishuEnterpriseAdapter) BuildAuthorizationURL(connection domain.EnterpriseConnection, state, _, _, redirectURI string) (string, error) {
	if strings.TrimSpace(connection.AppID) == "" {
		return "", fmt.Errorf("feishu app id is missing")
	}
	values := url.Values{"app_id": {connection.AppID}, "redirect_uri": {redirectURI}, "state": {state}}
	return "https://accounts.feishu.cn/open-apis/authen/v1/authorize?" + values.Encode(), nil
}
func (a *feishuEnterpriseAdapter) Probe(ctx context.Context, connection domain.EnterpriseConnection, secret string) error {
	var response struct {
		Code int `json:"code"`
	}
	return enterpriseJSON(ctx, a.client, http.MethodPost, "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal", map[string]string{"app_id": connection.AppID, "app_secret": secret}, "", &response, func() error {
		if response.Code != 0 {
			return fmt.Errorf("feishu credential rejected")
		}
		return nil
	})
}
func (a *feishuEnterpriseAdapter) SendPersonalTest(ctx context.Context, connection domain.EnterpriseConnection, secret, subject string) error {
	var token struct {
		Code              int    `json:"code"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	if err := enterpriseJSON(ctx, a.client, http.MethodPost, "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal", map[string]string{"app_id": connection.AppID, "app_secret": secret}, "", &token, func() error {
		if token.Code != 0 || token.TenantAccessToken == "" {
			return fmt.Errorf("feishu credential rejected")
		}
		return nil
	}); err != nil {
		return err
	}
	var result struct {
		Code int `json:"code"`
	}
	body := map[string]any{"receive_id": subject, "msg_type": "text", "content": `{"text":"Seven Agent Force 测试消息：你的个人通知已连接。"}`}
	endpoint := "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=open_id"
	return enterpriseJSON(ctx, a.client, http.MethodPost, endpoint, body, "Bearer "+token.TenantAccessToken, &result, func() error {
		if result.Code != 0 {
			return fmt.Errorf("feishu message rejected")
		}
		return nil
	})
}
func (a *feishuEnterpriseAdapter) DiscoverGroups(ctx context.Context, connection domain.EnterpriseConnection, secret string) ([]externalapp.EnterpriseDiscoveredGroup, error) {
	var token struct {
		Code              int    `json:"code"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	if err := enterpriseJSON(ctx, a.client, http.MethodPost, "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal", map[string]string{"app_id": connection.AppID, "app_secret": secret}, "", &token, func() error {
		if token.Code != 0 || token.TenantAccessToken == "" {
			return fmt.Errorf("feishu credential rejected")
		}
		return nil
	}); err != nil {
		return nil, err
	}
	var response struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Items []struct {
				ChatID      string `json:"chat_id"`
				Name        string `json:"name"`
				Description string `json:"description"`
				UserCount   int    `json:"user_count"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := feishuGroupDiscoveryJSON(ctx, a.client, "https://open.feishu.cn/open-apis/im/v1/chats?page_size=100", token.TenantAccessToken, &response); err != nil {
		return nil, err
	}
	items := make([]externalapp.EnterpriseDiscoveredGroup, 0, len(response.Data.Items))
	for _, group := range response.Data.Items {
		description := strings.TrimSpace(group.Description)
		if description == "" && group.UserCount > 0 {
			description = fmt.Sprintf("%d 名成员", group.UserCount)
		}
		items = append(items, externalapp.EnterpriseDiscoveredGroup{DisplayName: group.Name, Description: description, Target: group.ChatID})
	}
	return items, nil
}

func feishuGroupDiscoveryJSON(ctx context.Context, client *http.Client, endpoint, token string, out any) error {
	if client == nil {
		return enterpriseOAuthError{cause: fmt.Errorf("enterprise oauth client is not configured")}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return enterpriseOAuthError{uncertain: true, cause: fmt.Errorf("enterprise provider request uncertain")}
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, enterpriseResponseLimit+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return enterpriseOAuthError{uncertain: true, cause: fmt.Errorf("enterprise provider response unreadable")}
	}
	if len(raw) > enterpriseResponseLimit {
		return enterpriseOAuthError{cause: fmt.Errorf("enterprise provider response exceeds limit")}
	}
	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return enterpriseOAuthError{cause: fmt.Errorf("invalid enterprise provider response")}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || envelope.Code != 0 {
		if envelope.Code == 99991672 || envelope.Code == 99991679 || strings.Contains(strings.ToLower(envelope.Msg), "im:chat") {
			return enterpriseGroupDiscoveryRejection{
				reason:  "ENTERPRISE_GROUP_DISCOVERY_FEISHU_SCOPE_REQUIRED",
				message: "飞书应用缺少群信息读取权限，请在飞书开发者后台开通 im:chat:readonly，并发布新版本后重试",
			}
		}
		return enterpriseGroupDiscoveryRejection{
			reason:  "ENTERPRISE_GROUP_DISCOVERY_PROVIDER_REJECTED",
			message: "飞书拒绝读取应用可见群，请检查应用权限、发布状态以及机器人是否可用后重试",
		}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return enterpriseOAuthError{cause: fmt.Errorf("invalid enterprise provider response")}
	}
	return nil
}
func (a *feishuEnterpriseAdapter) ExchangeAndResolve(ctx context.Context, connection domain.EnterpriseConnection, secret, code, _, redirectURI string) (externalapp.EnterpriseResolvedSubject, error) {
	var token struct {
		Code        int    `json:"code"`
		AccessToken string `json:"access_token"`
	}
	err := enterpriseJSON(ctx, a.client, http.MethodPost, "https://open.feishu.cn/open-apis/authen/v2/oauth/token", map[string]string{"grant_type": "authorization_code", "client_id": connection.AppID, "client_secret": secret, "code": code, "redirect_uri": redirectURI}, "", &token, func() error {
		if token.Code != 0 || strings.TrimSpace(token.AccessToken) == "" {
			return fmt.Errorf("feishu authorization rejected")
		}
		return nil
	})
	if err != nil {
		return externalapp.EnterpriseResolvedSubject{}, err
	}
	var profile struct {
		Code int `json:"code"`
		Data struct {
			OpenID string `json:"open_id"`
		} `json:"data"`
		OpenID string `json:"open_id"`
	}
	err = enterpriseJSON(ctx, a.client, http.MethodGet, "https://open.feishu.cn/open-apis/authen/v1/user_info", nil, "Bearer "+token.AccessToken, &profile, func() error {
		if profile.Code != 0 {
			return fmt.Errorf("feishu user identity rejected")
		}
		return nil
	})
	if err != nil {
		return externalapp.EnterpriseResolvedSubject{}, err
	}
	subject := profile.Data.OpenID
	if subject == "" {
		subject = profile.OpenID
	}
	if strings.TrimSpace(subject) == "" {
		return externalapp.EnterpriseResolvedSubject{}, fmt.Errorf("feishu user identity missing")
	}
	return externalapp.EnterpriseResolvedSubject{Subject: subject, SubjectKind: "FEISHU_OPEN_ID"}, nil
}

type weComEnterpriseAdapter struct{ client *http.Client }

var weComIPv4Pattern = regexp.MustCompile(`(?:\d{1,3}\.){3}\d{1,3}`)

func weComRejectedIP(message string) string {
	for _, candidate := range weComIPv4Pattern.FindAllString(message, -1) {
		if parsed := net.ParseIP(candidate); parsed != nil && parsed.To4() != nil {
			return candidate
		}
	}
	return ""
}

func (*weComEnterpriseAdapter) Provider() domain.EnterpriseProvider {
	return domain.EnterpriseProviderWeCom
}
func (*weComEnterpriseAdapter) Capabilities() domain.EnterpriseProtocolCapabilities {
	return domain.EnterpriseProtocolCapabilities{SupportsAuthorizationCode: true, SupportsUserInfo: true}
}
func (*weComEnterpriseAdapter) BuildAuthorizationURL(connection domain.EnterpriseConnection, state, _, _, redirectURI string) (string, error) {
	if strings.TrimSpace(connection.CorpID) == "" {
		return "", fmt.Errorf("wecom corp id is missing")
	}
	if strings.TrimSpace(connection.AgentID) == "" {
		return "", fmt.Errorf("wecom agent id is missing")
	}
	values := url.Values{
		"appid":        {strings.TrimSpace(connection.CorpID)},
		"agentid":      {strings.TrimSpace(connection.AgentID)},
		"redirect_uri": {redirectURI},
		"state":        {state},
	}
	return "https://open.work.weixin.qq.com/wwopen/sso/qrConnect?" + values.Encode(), nil
}
func (a *weComEnterpriseAdapter) applicationToken(ctx context.Context, connection domain.EnterpriseConnection, secret string) (string, error) {
	values := url.Values{"corpid": {connection.CorpID}, "corpsecret": {secret}}
	var result struct {
		ErrCode     int    `json:"errcode"`
		AccessToken string `json:"access_token"`
	}
	if err := enterpriseJSON(ctx, a.client, http.MethodGet, "https://qyapi.weixin.qq.com/cgi-bin/gettoken?"+values.Encode(), nil, "", &result, func() error {
		if result.ErrCode != 0 || strings.TrimSpace(result.AccessToken) == "" {
			return fmt.Errorf("wecom credential rejected")
		}
		return nil
	}); err != nil {
		return "", err
	}
	return result.AccessToken, nil
}
func (a *weComEnterpriseAdapter) Probe(ctx context.Context, connection domain.EnterpriseConnection, secret string) error {
	token, err := a.applicationToken(ctx, connection, secret)
	if err != nil {
		return err
	}
	values := url.Values{"access_token": {token}, "agentid": {strings.TrimSpace(connection.AgentID)}}
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		AgentID int64  `json:"agentid"`
	}
	return enterpriseJSON(ctx, a.client, http.MethodGet, "https://qyapi.weixin.qq.com/cgi-bin/agent/get?"+values.Encode(), nil, "", &result, func() error {
		if result.ErrCode == 60020 {
			message := "企业微信拒绝了当前服务端出口 IP，请在该自建应用的“企业可信 IP”中放行 Seven 服务端公网出口 IP"
			if rejectedIP := weComRejectedIP(result.ErrMsg); rejectedIP != "" {
				message = "企业微信拒绝了当前服务端出口 IP " + rejectedIP + "，请在该自建应用的“企业可信 IP”中放行此地址"
			}
			return enterpriseConnectionRejection{
				reason:  "ENTERPRISE_CONNECTION_WECOM_TRUSTED_IP_REQUIRED",
				message: message,
			}
		}
		if result.ErrCode != 0 || result.AgentID <= 0 || strconv.FormatInt(result.AgentID, 10) != strings.TrimSpace(connection.AgentID) {
			return enterpriseConnectionRejection{
				reason:  "ENTERPRISE_CONNECTION_WECOM_AGENT_MISMATCH",
				message: "企业微信应用检查未通过，请确认 Agent ID 和应用密钥来自同一个自建应用",
			}
		}
		return nil
	})
}
func (a *weComEnterpriseAdapter) SendPersonalTest(ctx context.Context, connection domain.EnterpriseConnection, secret, subject string) error {
	token, err := a.applicationToken(ctx, connection, secret)
	if err != nil {
		return err
	}
	var result struct {
		ErrCode int `json:"errcode"`
	}
	body := map[string]any{"touser": subject, "msgtype": "text", "agentid": connection.AgentID, "text": map[string]string{"content": "Seven Agent Force 测试消息：你的个人通知已连接。"}, "safe": 0}
	return enterpriseJSON(ctx, a.client, http.MethodPost, "https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token="+url.QueryEscape(token), body, "", &result, func() error {
		if result.ErrCode != 0 {
			return fmt.Errorf("wecom message rejected")
		}
		return nil
	})
}
func (a *weComEnterpriseAdapter) ExchangeAndResolve(ctx context.Context, connection domain.EnterpriseConnection, secret, code, _, _ string) (externalapp.EnterpriseResolvedSubject, error) {
	token, err := a.applicationToken(ctx, connection, secret)
	if err != nil {
		return externalapp.EnterpriseResolvedSubject{}, err
	}
	values := url.Values{"access_token": {token}, "code": {code}}
	var result struct {
		ErrCode int    `json:"errcode"`
		UserID  string `json:"UserId"`
	}
	err = enterpriseJSON(ctx, a.client, http.MethodGet, "https://qyapi.weixin.qq.com/cgi-bin/auth/getuserinfo?"+values.Encode(), nil, "", &result, func() error {
		if result.ErrCode == 60020 {
			return enterpriseAuthorizationRejection{
				reason:  "ENTERPRISE_AUTHORIZATION_WECOM_TRUSTED_IP_REQUIRED",
				message: "企业微信拒绝了当前服务端出口 IP，请联系管理员在该自建应用中配置企业可信 IP",
			}
		}
		if result.ErrCode == 50001 {
			return enterpriseAuthorizationRejection{
				reason:  "ENTERPRISE_AUTHORIZATION_WECOM_TRUSTED_DOMAIN_REQUIRED",
				message: "企业微信拒绝了当前回调域名，请联系管理员检查该自建应用的授权回调域和可信域名",
			}
		}
		if result.ErrCode != 0 || strings.TrimSpace(result.UserID) == "" {
			return enterpriseAuthorizationRejection{
				reason:  "ENTERPRISE_AUTHORIZATION_PROVIDER_REJECTED",
				message: "企业微信没有接受本次授权，请确认当前成员属于该企业并重新绑定",
			}
		}
		return nil
	})
	if err != nil {
		return externalapp.EnterpriseResolvedSubject{}, err
	}
	return externalapp.EnterpriseResolvedSubject{Subject: result.UserID, SubjectKind: "WECOM_USERID"}, nil
}

func enterpriseJSON(ctx context.Context, client *http.Client, method, endpoint string, body any, bearer string, out any, validate func() error) error {
	if client == nil {
		return fmt.Errorf("enterprise oauth client is not configured")
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", bearer)
	}
	resp, err := client.Do(req)
	if err != nil {
		return enterpriseOAuthError{uncertain: true, cause: fmt.Errorf("enterprise provider request uncertain")}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return enterpriseOAuthError{cause: fmt.Errorf("enterprise provider rejected request")}
	}
	limited := io.LimitReader(resp.Body, enterpriseResponseLimit+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return enterpriseOAuthError{uncertain: true, cause: fmt.Errorf("enterprise provider response unreadable")}
	}
	if len(raw) > enterpriseResponseLimit {
		return enterpriseOAuthError{cause: fmt.Errorf("enterprise provider response exceeds limit")}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return enterpriseOAuthError{cause: fmt.Errorf("invalid enterprise provider response")}
	}
	if err := validate(); err != nil {
		return enterpriseOAuthError{cause: err}
	}
	return nil
}
