package drivers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
)

const (
	githubAuthorizationEndpoint = "https://github.com/login/oauth/authorize"
	githubTokenEndpoint         = "https://github.com/login/oauth/access_token"
	githubProfileEndpoint       = "https://api.github.com/user"
	githubEmailsEndpoint        = "https://api.github.com/user/emails"
)

type GitHubDriver struct {
	httpClient *http.Client
}

type GitHubOption func(*GitHubDriver)

func NewGitHubDriver(options ...GitHubOption) *GitHubDriver {
	driver := &GitHubDriver{httpClient: http.DefaultClient}
	for _, option := range options {
		option(driver)
	}
	return driver
}

func WithGitHubHTTPClient(client *http.Client) GitHubOption {
	return func(driver *GitHubDriver) {
		if client != nil {
			driver.httpClient = client
		}
	}
}

func (d *GitHubDriver) Code() string {
	return "github"
}

func (d *GitHubDriver) Capabilities() domain.ProviderCapability {
	return domain.BuiltInProviderCapabilities()["github"]
}

func (d *GitHubDriver) BuildAuthorizationURL(_ context.Context, provider domain.Provider, request AuthorizationRequest) (string, error) {
	if strings.TrimSpace(provider.ClientID) == "" {
		return "", fmt.Errorf("github client id is required")
	}
	if request.State == "" {
		return "", fmt.Errorf("github authorization state is required")
	}
	if request.CodeChallenge == "" {
		return "", fmt.Errorf("github PKCE code challenge is required")
	}
	codeChallengeMethod := firstNonBlank(request.CodeChallengeMethod, "S256")
	if codeChallengeMethod != "S256" {
		return "", fmt.Errorf("github PKCE code challenge method must be S256")
	}
	redirectURI := firstNonBlank(request.RedirectURI, provider.RedirectURI)
	if redirectURI == "" {
		return "", fmt.Errorf("github redirect uri is required")
	}
	endpoint := firstNonBlank(provider.AuthorizationEndpoint, githubAuthorizationEndpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse github authorization endpoint: %w", err)
	}
	values := parsed.Query()
	values.Set("client_id", provider.ClientID)
	values.Set("redirect_uri", redirectURI)
	values.Set("scope", strings.Join(resolveScopes(request.Scopes, provider.Scopes, d.Capabilities().DefaultScopes), " "))
	values.Set("state", request.State)
	values.Set("code_challenge", request.CodeChallenge)
	values.Set("code_challenge_method", codeChallengeMethod)
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

func (d *GitHubDriver) ExchangeCode(ctx context.Context, provider domain.Provider, request TokenExchangeRequest) (*TokenExchangeResult, error) {
	if strings.TrimSpace(request.Code) == "" {
		return nil, fmt.Errorf("github authorization code is required")
	}
	endpoint := firstNonBlank(provider.TokenEndpoint, githubTokenEndpoint)
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", provider.ClientID)
	form.Set("client_secret", request.ClientSecret)
	form.Set("code", request.Code)
	form.Set("redirect_uri", firstNonBlank(request.RedirectURI, provider.RedirectURI))
	form.Set("code_verifier", request.CodeVerifier)

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create github token request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpRequest.Header.Set("Accept", "application/json")

	body, err := doHTTP(d.httpClient, httpRequest)
	if err != nil {
		return nil, fmt.Errorf("exchange github code: %w", err)
	}
	var response oauthTokenResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode github token response: %w", err)
	}
	if response.Error != "" {
		return nil, fmt.Errorf("github token exchange failed: %s", response.Error)
	}
	return &TokenExchangeResult{
		TokenSet: domain.TokenSet{
			AccessToken:  response.AccessToken,
			RefreshToken: response.RefreshToken,
			IDToken:      response.IDToken,
			TokenType:    response.TokenType,
			Scopes:       parseScopeResponse(response.Scope, resolveScopes(request.Scopes, provider.Scopes, d.Capabilities().DefaultScopes)),
			ExpiresAt:    expiresAt(response.ExpiresIn),
		},
		RawTokenResponse: string(body),
	}, nil
}

func (d *GitHubDriver) ResolveProfile(ctx context.Context, provider domain.Provider, tokens TokenExchangeResult) (*domain.ExternalProfile, error) {
	if strings.TrimSpace(tokens.TokenSet.AccessToken) == "" {
		return nil, fmt.Errorf("github access token is required")
	}
	profileEndpoint := metadataString(provider.MetadataJSON, "profileEndpoint")
	if profileEndpoint == "" {
		profileEndpoint = firstNonBlank(provider.UserinfoEndpoint, githubProfileEndpoint)
	}
	body, err := d.githubGET(ctx, provider, profileEndpoint, tokens.TokenSet.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("resolve github profile: %w", err)
	}
	var profile githubUserProfile
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&profile); err != nil {
		return nil, fmt.Errorf("decode github profile: %w", err)
	}
	subject := profile.Subject()
	if subject == "" {
		return nil, fmt.Errorf("github profile missing stable subject")
	}
	email := profile.Email
	emailVerified := false
	if provider.EmailAutoBindEnabled && scopesContain(append(provider.Scopes, tokens.TokenSet.Scopes...), "user:email") {
		resolvedEmail, err := d.resolveVerifiedEmail(ctx, provider, tokens.TokenSet.AccessToken)
		if err != nil {
			return nil, err
		}
		email = resolvedEmail
		emailVerified = resolvedEmail != ""
	}
	return &domain.ExternalProfile{
		Subject:       subject,
		Login:         profile.Login,
		Email:         email,
		EmailVerified: emailVerified,
		DisplayName:   firstNonBlank(profile.Name, profile.Login),
		AvatarURL:     profile.AvatarURL,
		RawProfile:    string(body),
	}, nil
}

func (d *GitHubDriver) RefreshToken(context.Context, domain.Provider, domain.TokenSet) (*domain.TokenSet, error) {
	return nil, fmt.Errorf("github oauth app token refresh is not supported")
}

func (d *GitHubDriver) RevokeToken(context.Context, domain.Provider, domain.TokenSet) error {
	return nil
}

func (d *GitHubDriver) resolveVerifiedEmail(ctx context.Context, provider domain.Provider, accessToken string) (string, error) {
	endpoint := metadataString(provider.MetadataJSON, "emailsEndpoint")
	if endpoint == "" {
		endpoint = githubEmailsEndpoint
	}
	body, err := d.githubGET(ctx, provider, endpoint, accessToken)
	if err != nil {
		return "", fmt.Errorf("resolve github emails: %w", err)
	}
	var emails []githubEmail
	if err := json.Unmarshal(body, &emails); err != nil {
		return "", fmt.Errorf("decode github emails: %w", err)
	}
	for _, item := range emails {
		if item.Verified && item.Primary && item.Email != "" {
			return item.Email, nil
		}
	}
	for _, item := range emails {
		if item.Verified && item.Email != "" {
			return item.Email, nil
		}
	}
	return "", nil
}

func (d *GitHubDriver) githubGET(ctx context.Context, provider domain.Provider, endpoint string, accessToken string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", "application/vnd.github+json")
	if apiVersion := firstNonBlank(metadataString(provider.MetadataJSON, "githubApiVersion"), metadataString(provider.MetadataJSON, "apiVersion")); apiVersion != "" {
		request.Header.Set("X-GitHub-Api-Version", apiVersion)
	}
	return doHTTP(d.httpClient, request)
}

type githubUserProfile struct {
	ID        json.Number `json:"id"`
	NodeID    string      `json:"node_id"`
	Login     string      `json:"login"`
	Name      string      `json:"name"`
	Email     string      `json:"email"`
	AvatarURL string      `json:"avatar_url"`
}

func (p githubUserProfile) Subject() string {
	if p.ID != "" {
		return p.ID.String()
	}
	return strings.TrimSpace(p.NodeID)
}

type githubEmail struct {
	Email    string `json:"email"`
	Verified bool   `json:"verified"`
	Primary  bool   `json:"primary"`
}

type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int64  `json:"expires_in"`
	Error        string `json:"error"`
}

func doHTTP(client *http.Client, request *http.Request) ([]byte, error) {
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if readErr != nil {
		return nil, readErr
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream returned HTTP %d", response.StatusCode)
	}
	return body, nil
}

func resolveScopes(requestScopes, providerScopes, defaultScopes []string) []string {
	for _, scopes := range [][]string{requestScopes, providerScopes, defaultScopes} {
		if len(scopes) > 0 {
			return compactStrings(scopes)
		}
	}
	return nil
}

func compactStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func parseScopeResponse(scope string, fallback []string) []string {
	if strings.TrimSpace(scope) == "" {
		return fallback
	}
	return strings.FieldsFunc(scope, func(r rune) bool {
		return r == ' ' || r == ','
	})
}

func expiresAt(seconds int64) *time.Time {
	if seconds <= 0 {
		return nil
	}
	value := time.Now().Add(time.Duration(seconds) * time.Second)
	return &value
}

func scopesContain(scopes []string, expected string) bool {
	for _, scope := range scopes {
		if strings.EqualFold(strings.TrimSpace(scope), expected) {
			return true
		}
	}
	return false
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func metadataString(raw string, keys ...string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return ""
	}
	for _, key := range keys {
		if value, ok := metadata[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func metadataBool(raw string, key string) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return false
	}
	switch value := metadata[key].(type) {
	case bool:
		return value
	case string:
		parsed, _ := strconv.ParseBool(value)
		return parsed
	default:
		return false
	}
}
