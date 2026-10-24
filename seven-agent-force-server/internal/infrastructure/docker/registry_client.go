package docker

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

const manifestAcceptHeader = "application/vnd.oci.image.manifest.v1+json, application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.v2+json, application/vnd.docker.distribution.manifest.list.v2+json"
const maxRegistryBodyBytes int64 = 16 << 20

type registryRuntime struct {
	ID                     int64
	Code                   string
	Name                   string
	Endpoint               string
	APIBaseURL             string
	AuthType               string
	Username               string
	Password               string
	TokenRealm             string
	TokenService           string
	TLSEnabled             bool
	InsecureSkipVerify     bool
	NamespaceWhitelistJSON string
}

type registryHTTPClient struct {
	timeout time.Duration
	normal  *http.Client
	skipTLS *http.Client
}

type registryResponse struct {
	resp      *http.Response
	body      []byte
	challenge bearerChallenge
}

func newRegistryHTTPClient(timeout time.Duration) *registryHTTPClient {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &registryHTTPClient{
		timeout: timeout,
		normal:  &http.Client{Timeout: timeout, Transport: http.DefaultTransport.(*http.Transport).Clone()},
		skipTLS: &http.Client{Timeout: timeout, Transport: insecureTransport()},
	}
}

func (c *registryHTTPClient) Test(ctx context.Context, rt registryRuntime) RegistryConnectionTestView {
	result, err := c.do(ctx, rt, http.MethodGet, "/_catalog?n=1", "", false)
	if err != nil {
		return RegistryConnectionTestView{Success: false, Message: "连接失败：" + err.Error()}
	}
	resp := result.resp
	body := result.body
	defer resp.Body.Close()
	challenge := result.challenge
	if challenge.realm == "" {
		challenge = parseBearerChallenge(resp.Header.Get("WWW-Authenticate"))
	}
	view := RegistryConnectionTestView{
		Success:         resp.StatusCode >= 200 && resp.StatusCode < 300,
		ServerHeader:    resp.Header.Get("Docker-Distribution-Api-Version"),
		RegistryVersion: resp.Header.Get("Server"),
		TokenRealm:      firstNonBlank(challenge.realm, rt.TokenRealm),
		TokenService:    firstNonBlank(challenge.service, rt.TokenService),
	}
	if view.Success {
		view.Message = "连接成功"
	} else {
		view.Message = fmt.Sprintf("连接失败，HTTP %d", resp.StatusCode)
		if len(body) > 0 {
			view.Message += "：" + truncate(strings.TrimSpace(string(body)), 256)
		}
	}
	return view
}

func (c *registryHTTPClient) ListRepositories(ctx context.Context, rt registryRuntime, current, size int64, keyword string, maxPages int64) (*PageResult[RemoteRepositoryView], error) {
	result := make([]RemoteRepositoryView, 0)
	if current <= 0 {
		current = 1
	}
	if size <= 0 {
		size = 20
	}
	if maxPages <= 0 {
		maxPages = 64
	}
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	targetStart := (current - 1) * size
	targetEnd := targetStart + size
	seenAllowed := int64(0)
	nextPath := fmt.Sprintf("/_catalog?n=%d", size)
	maybeMore := false
	for scannedPages := int64(0); nextPath != ""; scannedPages++ {
		if scannedPages >= maxPages {
			maybeMore = true
			break
		}
		var payload struct {
			Repositories []string `json:"repositories"`
		}
		response, err := c.do(ctx, rt, http.MethodGet, nextPath, "", false)
		if err != nil {
			return nil, err
		}
		resp := response.resp
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, apperrors.Operation(fmt.Sprintf("registry 请求失败，HTTP %d", resp.StatusCode))
		}
		if err := json.Unmarshal(response.body, &payload); err != nil {
			return nil, apperrors.Operation("解析 registry 响应失败：" + err.Error())
		}
		for _, repo := range payload.Repositories {
			if !allowRepository(rt, repo) {
				continue
			}
			if keyword != "" && !strings.Contains(strings.ToLower(repo), keyword) {
				continue
			}
			if seenAllowed >= targetStart && seenAllowed < targetEnd {
				result = append(result, RemoteRepositoryView{Repository: repo})
			}
			seenAllowed++
		}
		nextPath = registryNextPath(resp.Header.Get("Link"))
		if nextPath == "" || len(payload.Repositories) == 0 {
			break
		}
		if seenAllowed >= targetEnd {
			maybeMore = true
			break
		}
	}
	total := seenAllowed
	if maybeMore {
		total = targetEnd + 1
	}
	return &PageResult[RemoteRepositoryView]{Current: current, Size: size, Total: total, Records: result}, nil
}

func (c *registryHTTPClient) ListTags(ctx context.Context, rt registryRuntime, repository string) (*RemoteTagsView, error) {
	if !allowRepository(rt, repository) {
		return nil, apperrors.Forbidden("当前 registry 配置不允许访问该仓库")
	}
	var payload struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}
	if err := c.getJSON(ctx, rt, "/"+encodeRepository(repository)+"/tags/list", false, &payload); err != nil {
		return nil, err
	}
	return &RemoteTagsView{Repository: repository, Tags: payload.Tags}, nil
}

func (c *registryHTTPClient) GetManifest(ctx context.Context, rt registryRuntime, repository, reference string) (*RemoteManifestView, error) {
	if !allowRepository(rt, repository) {
		return nil, apperrors.Forbidden("当前 registry 配置不允许访问该仓库")
	}
	path := "/" + encodeRepository(repository) + "/manifests/" + url.PathEscape(reference)
	result, err := c.do(ctx, rt, http.MethodGet, path, manifestAcceptHeader, true)
	if err != nil {
		return nil, err
	}
	resp := result.resp
	body := result.body
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apperrors.Operation(fmt.Sprintf("获取 manifest 失败，HTTP %d", resp.StatusCode))
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, apperrors.Operation("解析 manifest 失败：" + err.Error())
	}
	view := &RemoteManifestView{
		Repository:    repository,
		Reference:     reference,
		Digest:        resp.Header.Get("Docker-Content-Digest"),
		MediaType:     firstNonBlank(resp.Header.Get("Content-Type"), stringValue(payload["mediaType"])),
		Size:          int64(len(body)),
		SchemaVersion: intValue(payload["schemaVersion"]),
		Payload:       payload,
	}
	fillManifestMetadata(view, payload)
	if cfg, ok := payload["config"].(map[string]any); ok {
		if digest := stringValue(cfg["digest"]); digest != "" {
			var blob map[string]any
			if err := c.getJSON(ctx, rt, "/"+encodeRepository(repository)+"/blobs/"+url.PathEscape(digest), false, &blob); err == nil {
				view.OS = firstNonBlank(view.OS, stringValue(blob["os"]))
				view.Architecture = firstNonBlank(view.Architecture, stringValue(blob["architecture"]))
				view.Variant = firstNonBlank(view.Variant, stringValue(blob["variant"]))
				view.Created = firstNonBlank(view.Created, stringValue(blob["created"]))
				if view.LayerCount == 0 {
					if rootfs, ok := blob["rootfs"].(map[string]any); ok {
						if diffIDs, ok := rootfs["diff_ids"].([]any); ok {
							view.LayerCount = len(diffIDs)
						}
					}
				}
			}
		}
	}
	return view, nil
}

func (c *registryHTTPClient) getJSON(ctx context.Context, rt registryRuntime, path string, manifest bool, target any) error {
	accept := ""
	if manifest {
		accept = manifestAcceptHeader
	}
	result, err := c.do(ctx, rt, http.MethodGet, path, accept, manifest)
	if err != nil {
		return err
	}
	resp := result.resp
	body := result.body
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return apperrors.Operation(fmt.Sprintf("registry 请求失败，HTTP %d", resp.StatusCode))
	}
	if err := json.Unmarshal(body, target); err != nil {
		return apperrors.Operation("解析 registry 响应失败：" + err.Error())
	}
	return nil
}

func (c *registryHTTPClient) do(ctx context.Context, rt registryRuntime, method, path, accept string, manifest bool) (registryResponse, error) {
	client := c.httpClient(rt)
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(rt.APIBaseURL, "/")+path, nil)
	if err != nil {
		return registryResponse{}, apperrors.Params("registry 请求地址错误")
	}
	decorateRegistryRequest(req, rt, accept)
	resp, err := client.Do(req)
	if err != nil {
		return registryResponse{}, apperrors.Operation("registry 请求失败：" + err.Error())
	}
	body, err := readLimited(resp.Body, maxRegistryBodyBytes)
	if err != nil {
		_ = resp.Body.Close()
		return registryResponse{}, registryReadError(err)
	}
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	if resp.StatusCode != http.StatusUnauthorized || strings.ToUpper(rt.AuthType) != "TOKEN_CHALLENGE" {
		return registryResponse{resp: resp, body: body}, nil
	}
	challenge := parseBearerChallenge(resp.Header.Get("WWW-Authenticate"))
	if challenge.realm == "" {
		return registryResponse{resp: resp, body: body}, nil
	}
	token, err := c.fetchBearerToken(ctx, rt, challenge)
	if err != nil {
		return registryResponse{}, err
	}
	retry, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(rt.APIBaseURL, "/")+path, nil)
	if err != nil {
		return registryResponse{}, apperrors.Params("registry 请求地址错误")
	}
	if accept != "" {
		retry.Header.Set("Accept", accept)
	}
	retry.Header.Set("Authorization", "Bearer "+token)
	retryResp, err := client.Do(retry)
	if err != nil {
		return registryResponse{}, apperrors.Operation("registry 请求失败：" + err.Error())
	}
	retryBody, err := readLimited(retryResp.Body, maxRegistryBodyBytes)
	if err != nil {
		_ = retryResp.Body.Close()
		return registryResponse{}, registryReadError(err)
	}
	_ = retryResp.Body.Close()
	retryResp.Body = io.NopCloser(bytes.NewReader(retryBody))
	_ = manifest
	return registryResponse{resp: retryResp, body: retryBody, challenge: challenge}, nil
}

func (c *registryHTTPClient) fetchBearerToken(ctx context.Context, rt registryRuntime, challenge bearerChallenge) (string, error) {
	if rt.Username == "" || rt.Password == "" {
		return "", apperrors.Operation("TOKEN_CHALLENGE 模式缺少 robot account")
	}
	realm := firstNonBlank(challenge.realm, rt.TokenRealm)
	service := firstNonBlank(challenge.service, rt.TokenService)
	if realm == "" || service == "" {
		return "", apperrors.Operation("registry Bearer challenge 缺少 realm 或 service")
	}
	token, err := c.fetchBearerTokenOnce(ctx, rt, challenge, realm, service)
	if err == nil {
		return token, nil
	}
	if fallback := fallbackLoopbackRealm(realm); fallback != "" && fallback != realm {
		return c.fetchBearerTokenOnce(ctx, rt, challenge, fallback, service)
	}
	return "", err
}

func (c *registryHTTPClient) fetchBearerTokenOnce(ctx context.Context, rt registryRuntime, challenge bearerChallenge, realm, service string) (string, error) {
	query := url.Values{}
	query.Set("service", service)
	query.Set("account", rt.Username)
	if challenge.scope != "" {
		query.Set("scope", challenge.scope)
	}
	tokenURL := realm
	if strings.Contains(tokenURL, "?") {
		tokenURL += "&" + query.Encode()
	} else {
		tokenURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	if err != nil {
		return "", apperrors.Operation("构建 Bearer token 请求失败：" + err.Error())
	}
	basic := base64.StdEncoding.EncodeToString([]byte(rt.Username + ":" + rt.Password))
	req.Header.Set("Authorization", "Basic "+basic)
	resp, err := c.httpClient(rt).Do(req)
	if err != nil {
		return "", apperrors.Operation("获取 Bearer token 失败：" + err.Error())
	}
	defer resp.Body.Close()
	body, err := readLimited(resp.Body, maxRegistryBodyBytes)
	if err != nil {
		return "", registryReadError(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", apperrors.Operation(fmt.Sprintf("获取 Bearer token 失败，HTTP %d", resp.StatusCode))
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", apperrors.Operation("解析 Bearer token 失败：" + err.Error())
	}
	token := firstNonBlank(stringValue(payload["token"]), stringValue(payload["access_token"]))
	if token == "" {
		return "", apperrors.Operation("Bearer token 响应缺少 token")
	}
	return token, nil
}

func (c *registryHTTPClient) httpClient(rt registryRuntime) *http.Client {
	if rt.InsecureSkipVerify {
		return c.skipTLS
	}
	return c.normal
}

func (c *registryHTTPClient) CloseIdleConnections() {
	if c == nil {
		return
	}
	for _, client := range []*http.Client{c.normal, c.skipTLS} {
		if client == nil {
			continue
		}
		if transport, ok := client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}
}

func insecureTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // v1 mirrors OTA behavior.
	return transport
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, apperrors.Operation("registry 响应过大")
	}
	return body, nil
}

func registryReadError(err error) error {
	if err == nil {
		return nil
	}
	if apperrors.From(err).Kind() == apperrors.KindOperation {
		return err
	}
	return apperrors.Operation("读取 registry 响应失败：" + err.Error())
}

func registryNextPath(link string) string {
	link = strings.TrimSpace(link)
	if link == "" {
		return ""
	}
	parts := strings.Split(link, ";")
	if len(parts) == 0 {
		return ""
	}
	raw := strings.Trim(strings.TrimSpace(parts[0]), "<>")
	if raw == "" {
		return ""
	}
	if parsed, err := url.Parse(raw); err == nil {
		if parsed.Path != "" {
			if parsed.RawQuery != "" {
				return parsed.Path + "?" + parsed.RawQuery
			}
			return parsed.Path
		}
	}
	if strings.HasPrefix(raw, "/") {
		return raw
	}
	return ""
}

func decorateRegistryRequest(req *http.Request, rt registryRuntime, accept string) {
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if strings.ToUpper(rt.AuthType) == "BASIC" && rt.Username != "" {
		basic := base64.StdEncoding.EncodeToString([]byte(rt.Username + ":" + rt.Password))
		req.Header.Set("Authorization", "Basic "+basic)
	}
}

type bearerChallenge struct {
	realm   string
	service string
	scope   string
}

func parseBearerChallenge(header string) bearerChallenge {
	header = strings.TrimSpace(header)
	if len(header) < 7 || !strings.EqualFold(header[:7], "Bearer ") {
		return bearerChallenge{}
	}
	raw := strings.TrimSpace(header[7:])
	values := map[string]string{}
	for _, part := range splitHeaderParts(raw) {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"`)
		values[strings.TrimSpace(key)] = value
	}
	return bearerChallenge{realm: values["realm"], service: values["service"], scope: values["scope"]}
}

func splitHeaderParts(raw string) []string {
	parts := []string{}
	var current strings.Builder
	inQuote := false
	for _, r := range raw {
		switch r {
		case '"':
			inQuote = !inQuote
			current.WriteRune(r)
		case ',':
			if inQuote {
				current.WriteRune(r)
			} else {
				parts = append(parts, strings.TrimSpace(current.String()))
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if s := strings.TrimSpace(current.String()); s != "" {
		parts = append(parts, s)
	}
	return parts
}
