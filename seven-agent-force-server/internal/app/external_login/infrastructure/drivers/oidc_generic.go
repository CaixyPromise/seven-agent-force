package drivers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
	jwkjwx "github.com/lestrrat-go/jwx/v3/jwk"
	jwsjwx "github.com/lestrrat-go/jwx/v3/jws"
	jwtjwx "github.com/lestrrat-go/jwx/v3/jwt"
)

type OIDCOption func(*oidcBase)

func WithOIDCHTTPClient(client *http.Client) OIDCOption {
	return func(driver *oidcBase) {
		if client != nil {
			driver.httpClient = client
			if validator, ok := driver.validator.(*JWKSIDTokenValidator); ok {
				validator.httpClient = client
			}
		}
	}
}

func WithOIDCDevelopmentHTTP(allowed bool) OIDCOption {
	return func(driver *oidcBase) {
		driver.allowDevelopmentHTTP = allowed
	}
}

func WithOIDCIDTokenValidator(validator OIDCIDTokenValidator) OIDCOption {
	return func(driver *oidcBase) {
		if validator != nil {
			driver.validator = validator
		}
	}
}

type OIDCDriver struct {
	*oidcBase
}

type DiscoveryDocument struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

const maxOIDCDiscoveryDocumentBytes = 64 << 10

func NewOIDCDriver(options ...OIDCOption) *OIDCDriver {
	return &OIDCDriver{oidcBase: newOIDCBase("oidc", oidcDefaults{}, options...)}
}

func (d *OIDCDriver) Discover(ctx context.Context, expectedIssuer string) (DiscoveryDocument, error) {
	issuer := strings.TrimSpace(expectedIssuer)
	parsedIssuer, err := url.Parse(issuer)
	if err != nil || (parsedIssuer.Scheme != "https" && (!d.allowDevelopmentHTTP || parsedIssuer.Scheme != "http")) || parsedIssuer.Host == "" || parsedIssuer.User != nil || parsedIssuer.RawQuery != "" || parsedIssuer.Fragment != "" {
		return DiscoveryDocument{}, fmt.Errorf("OIDC discovery issuer must use an allowed absolute URL")
	}
	discoveryURL := *parsedIssuer
	discoveryURL.Path = "/.well-known/openid-configuration" + parsedIssuer.Path
	discoveryURL.RawPath = ""
	if parsedIssuer.RawPath != "" {
		discoveryURL.RawPath = "/.well-known/openid-configuration" + parsedIssuer.RawPath
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL.String(), nil)
	if err != nil {
		return DiscoveryDocument{}, fmt.Errorf("create OIDC discovery request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	client := d.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return DiscoveryDocument{}, fmt.Errorf("fetch OIDC discovery document: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return DiscoveryDocument{}, fmt.Errorf("OIDC discovery returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxOIDCDiscoveryDocumentBytes+1))
	if err != nil {
		return DiscoveryDocument{}, fmt.Errorf("read OIDC discovery document: %w", err)
	}
	if len(body) > maxOIDCDiscoveryDocumentBytes {
		return DiscoveryDocument{}, fmt.Errorf("OIDC discovery document exceeds %d bytes", maxOIDCDiscoveryDocumentBytes)
	}
	var document DiscoveryDocument
	if err := json.Unmarshal(body, &document); err != nil {
		return DiscoveryDocument{}, fmt.Errorf("decode OIDC discovery document: %w", err)
	}
	if document.Issuer != issuer {
		return DiscoveryDocument{}, fmt.Errorf("OIDC discovery issuer mismatch")
	}
	for name, endpoint := range map[string]string{
		"authorization_endpoint": document.AuthorizationEndpoint,
		"token_endpoint":         document.TokenEndpoint,
		"jwks_uri":               document.JWKSURI,
	} {
		if err := requireManagedOIDCEndpointOrigin(parsedIssuer, endpoint, d.allowDevelopmentHTTP); err != nil {
			return DiscoveryDocument{}, fmt.Errorf("invalid OIDC %s: %w", name, err)
		}
	}
	if document.UserinfoEndpoint != "" {
		if err := requireManagedOIDCEndpointOrigin(parsedIssuer, document.UserinfoEndpoint, d.allowDevelopmentHTTP); err != nil {
			return DiscoveryDocument{}, fmt.Errorf("invalid OIDC userinfo_endpoint: %w", err)
		}
	}
	return document, nil
}

func requireManagedOIDCEndpointOrigin(issuer *url.URL, raw string, allowDevelopmentHTTP bool) error {
	endpoint, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (endpoint.Scheme != "https" && (!allowDevelopmentHTTP || endpoint.Scheme != "http")) || endpoint.Host == "" || endpoint.User != nil || endpoint.Fragment != "" {
		return fmt.Errorf("endpoint must use an allowed absolute URL")
	}
	if !strings.EqualFold(endpoint.Scheme, issuer.Scheme) || !strings.EqualFold(endpoint.Host, issuer.Host) {
		return fmt.Errorf("endpoint origin must match issuer origin")
	}
	return nil
}

type oidcDefaults struct {
	issuer                string
	authorizationEndpoint string
	tokenEndpoint         string
	jwksURI               string
}

type oidcBase struct {
	code                 string
	defaults             oidcDefaults
	httpClient           *http.Client
	validator            OIDCIDTokenValidator
	now                  func() time.Time
	allowDevelopmentHTTP bool
}

func newOIDCBase(code string, defaults oidcDefaults, options ...OIDCOption) *oidcBase {
	validator := NewJWKSIDTokenValidator(http.DefaultClient)
	driver := &oidcBase{
		code:       code,
		defaults:   defaults,
		httpClient: http.DefaultClient,
		validator:  validator,
		now:        time.Now,
	}
	for _, option := range options {
		option(driver)
	}
	return driver
}

func (d *oidcBase) Code() string {
	return d.code
}

func (d *oidcBase) Capabilities() domain.ProviderCapability {
	return domain.BuiltInProviderCapabilities()[d.code]
}

func (d *oidcBase) BuildAuthorizationURL(_ context.Context, provider domain.Provider, request AuthorizationRequest) (string, error) {
	if strings.TrimSpace(provider.ClientID) == "" {
		return "", fmt.Errorf("%s client id is required", d.code)
	}
	if request.State == "" {
		return "", fmt.Errorf("%s authorization state is required", d.code)
	}
	if request.Nonce == "" {
		return "", fmt.Errorf("%s authorization nonce is required", d.code)
	}
	redirectURI := firstNonBlank(request.RedirectURI, provider.RedirectURI)
	if redirectURI == "" {
		return "", fmt.Errorf("%s redirect uri is required", d.code)
	}
	endpoint := firstNonBlank(provider.AuthorizationEndpoint, d.defaults.authorizationEndpoint)
	if endpoint == "" {
		return "", fmt.Errorf("%s authorization endpoint is required", d.code)
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse %s authorization endpoint: %w", d.code, err)
	}
	values := parsed.Query()
	values.Set("response_type", "code")
	values.Set("client_id", provider.ClientID)
	values.Set("redirect_uri", redirectURI)
	values.Set("scope", strings.Join(resolveScopes(request.Scopes, provider.Scopes, d.Capabilities().DefaultScopes), " "))
	values.Set("state", request.State)
	values.Set("nonce", request.Nonce)
	if request.CodeChallenge != "" {
		codeChallengeMethod := firstNonBlank(request.CodeChallengeMethod, "S256")
		if codeChallengeMethod != "S256" {
			return "", fmt.Errorf("%s PKCE code challenge method must be S256", d.code)
		}
		values.Set("code_challenge", request.CodeChallenge)
		values.Set("code_challenge_method", codeChallengeMethod)
	}
	if d.code == "google" && metadataBool(provider.MetadataJSON, "enableRefreshTokenStorage") {
		values.Set("access_type", "offline")
	}
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

func (d *oidcBase) ExchangeCode(ctx context.Context, provider domain.Provider, request TokenExchangeRequest) (*TokenExchangeResult, error) {
	if err := validateIssuerBinding(provider, request, d.defaults.issuer); err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.Code) == "" {
		return nil, fmt.Errorf("%s authorization code is required", d.code)
	}
	endpoint := firstNonBlank(provider.TokenEndpoint, d.defaults.tokenEndpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("%s token endpoint is required", d.code)
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", request.Code)
	form.Set("redirect_uri", firstNonBlank(request.RedirectURI, provider.RedirectURI))
	form.Set("code_verifier", request.CodeVerifier)
	useBasic := strings.TrimSpace(provider.TokenEndpointAuthMethod) == domain.TokenEndpointAuthMethodClientSecretBasic
	if !useBasic {
		form.Set("client_id", provider.ClientID)
		form.Set("client_secret", request.ClientSecret)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create %s token request: %w", d.code, err)
	}
	httpRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpRequest.Header.Set("Accept", "application/json")
	if useBasic {
		httpRequest.SetBasicAuth(provider.ClientID, request.ClientSecret)
	}

	body, err := doHTTP(d.httpClient, httpRequest)
	if err != nil {
		return nil, fmt.Errorf("exchange %s code: %w", d.code, err)
	}
	var response oauthTokenResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode %s token response: %w", d.code, err)
	}
	if response.Error != "" {
		return nil, fmt.Errorf("%s token exchange failed: %s", d.code, response.Error)
	}
	result := &TokenExchangeResult{
		TokenSet: domain.TokenSet{
			AccessToken:  response.AccessToken,
			RefreshToken: response.RefreshToken,
			IDToken:      response.IDToken,
			TokenType:    response.TokenType,
			Scopes:       parseScopeResponse(response.Scope, resolveScopes(request.Scopes, provider.Scopes, d.Capabilities().DefaultScopes)),
			ExpiresAt:    expiresAt(response.ExpiresIn),
		},
		RawTokenResponse: string(body),
		ExpectedIssuer:   firstNonBlank(request.ExpectedIssuer, provider.Issuer, d.defaults.issuer),
		CallbackIssuer:   request.CallbackIssuer,
		ExpectedNonce:    request.Nonce,
	}
	if result.TokenSet.IDToken != "" {
		if _, err := d.validateIDToken(ctx, provider, *result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (d *oidcBase) ResolveProfile(ctx context.Context, provider domain.Provider, tokens TokenExchangeResult) (*domain.ExternalProfile, error) {
	claims, err := d.validateIDToken(ctx, provider, tokens)
	if err != nil {
		return nil, err
	}
	email := ""
	if claims.EmailVerified {
		email = claims.Email
	}
	return &domain.ExternalProfile{
		Subject:       claims.Subject,
		Login:         claims.Subject,
		Email:         email,
		EmailVerified: claims.EmailVerified,
		DisplayName:   firstNonBlank(claims.Name, claims.Email, claims.Subject),
		AvatarURL:     claims.Picture,
		RawProfile:    claims.Raw,
	}, nil
}

func (d *oidcBase) RefreshToken(ctx context.Context, provider domain.Provider, tokenSet domain.TokenSet) (*domain.TokenSet, error) {
	if strings.TrimSpace(tokenSet.RefreshToken) == "" {
		return nil, fmt.Errorf("%s refresh token is required", d.code)
	}
	endpoint := firstNonBlank(provider.TokenEndpoint, d.defaults.tokenEndpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("%s token endpoint is required", d.code)
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", provider.ClientID)
	form.Set("refresh_token", tokenSet.RefreshToken)
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create %s refresh request: %w", d.code, err)
	}
	httpRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpRequest.Header.Set("Accept", "application/json")
	body, err := doHTTP(d.httpClient, httpRequest)
	if err != nil {
		return nil, fmt.Errorf("refresh %s token: %w", d.code, err)
	}
	var response oauthTokenResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode %s refresh response: %w", d.code, err)
	}
	if response.Error != "" {
		return nil, fmt.Errorf("%s token refresh failed: %s", d.code, response.Error)
	}
	return &domain.TokenSet{
		AccessToken:  response.AccessToken,
		RefreshToken: firstNonBlank(response.RefreshToken, tokenSet.RefreshToken),
		IDToken:      response.IDToken,
		TokenType:    response.TokenType,
		Scopes:       parseScopeResponse(response.Scope, tokenSet.Scopes),
		ExpiresAt:    expiresAt(response.ExpiresIn),
	}, nil
}

func (d *oidcBase) RevokeToken(ctx context.Context, provider domain.Provider, tokenSet domain.TokenSet) error {
	endpoint := metadataString(provider.MetadataJSON, "revocationEndpoint")
	if endpoint == "" {
		return nil
	}
	token := firstNonBlank(tokenSet.RefreshToken, tokenSet.AccessToken)
	if token == "" {
		return nil
	}
	form := url.Values{}
	form.Set("token", token)
	form.Set("client_id", provider.ClientID)
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create %s revoke request: %w", d.code, err)
	}
	httpRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, err = doHTTP(d.httpClient, httpRequest)
	return err
}

func (d *oidcBase) validateIDToken(ctx context.Context, provider domain.Provider, tokens TokenExchangeResult) (OIDCClaims, error) {
	if strings.TrimSpace(tokens.TokenSet.IDToken) == "" {
		return OIDCClaims{}, fmt.Errorf("%s id token is required", d.code)
	}
	expectedIssuer := firstNonBlank(tokens.ExpectedIssuer, provider.Issuer, d.defaults.issuer)
	claims, err := d.validator.ValidateIDToken(ctx, tokens.TokenSet.IDToken, OIDCValidationOptions{
		Issuer:   expectedIssuer,
		Audience: provider.ClientID,
		Nonce:    tokens.ExpectedNonce,
		JWKSURI:  firstNonBlank(provider.JWKSURI, d.defaults.jwksURI),
	})
	if err != nil {
		return OIDCClaims{}, err
	}
	if err := validateOIDCClaims(claims, OIDCValidationOptions{
		Issuer:   expectedIssuer,
		Audience: provider.ClientID,
		Nonce:    tokens.ExpectedNonce,
	}, d.now()); err != nil {
		return OIDCClaims{}, err
	}
	return claims, nil
}

func validateIssuerBinding(provider domain.Provider, request TokenExchangeRequest, defaultIssuer string) error {
	expectedIssuer := firstNonBlank(request.ExpectedIssuer, provider.Issuer, defaultIssuer)
	if provider.Issuer != "" && expectedIssuer != "" && provider.Issuer != expectedIssuer {
		return fmt.Errorf("oidc issuer binding mismatch: provider issuer %q does not match request issuer %q", provider.Issuer, expectedIssuer)
	}
	if request.CallbackIssuer != "" && expectedIssuer != "" && request.CallbackIssuer != expectedIssuer {
		return fmt.Errorf("oidc issuer mix-up rejected: callback issuer %q does not match expected issuer %q", request.CallbackIssuer, expectedIssuer)
	}
	return nil
}

func validateOIDCClaims(claims OIDCClaims, options OIDCValidationOptions, now time.Time) error {
	if options.Issuer == "" {
		return fmt.Errorf("oidc expected issuer is required")
	}
	if claims.Issuer != options.Issuer {
		return fmt.Errorf("oidc issuer mismatch: got %q want %q", claims.Issuer, options.Issuer)
	}
	if options.Audience == "" {
		return fmt.Errorf("oidc expected audience is required")
	}
	if !stringSliceContains(claims.Audience, options.Audience) {
		return fmt.Errorf("oidc audience mismatch")
	}
	if claims.ExpiresAt.IsZero() {
		return fmt.Errorf("oidc exp claim is required")
	}
	if !claims.ExpiresAt.After(now) {
		return fmt.Errorf("oidc id token is expired")
	}
	if claims.IssuedAt.IsZero() {
		return fmt.Errorf("oidc iat claim is required")
	}
	if claims.IssuedAt.After(now.Add(5 * time.Minute)) {
		return fmt.Errorf("oidc iat claim is in the future")
	}
	if strings.TrimSpace(options.Nonce) == "" {
		return fmt.Errorf("oidc expected nonce is required")
	}
	if claims.Nonce != options.Nonce {
		return fmt.Errorf("oidc nonce mismatch")
	}
	if strings.TrimSpace(claims.Subject) == "" {
		return fmt.Errorf("oidc sub claim is required")
	}
	return nil
}

func stringSliceContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

type JWKSIDTokenValidator struct {
	httpClient *http.Client
	cacheTTL   time.Duration
	now        func() time.Time
	mu         sync.Mutex
	cache      map[string]cachedJWKS
}

type cachedJWKS struct {
	set       jwkjwx.Set
	expiresAt time.Time
}

func NewJWKSIDTokenValidator(client *http.Client) *JWKSIDTokenValidator {
	if client == nil {
		client = http.DefaultClient
	}
	return &JWKSIDTokenValidator{
		httpClient: client,
		cacheTTL:   5 * time.Minute,
		now:        time.Now,
		cache:      make(map[string]cachedJWKS),
	}
}

func (v *JWKSIDTokenValidator) ValidateIDToken(ctx context.Context, raw string, options OIDCValidationOptions) (OIDCClaims, error) {
	if strings.TrimSpace(raw) == "" {
		return OIDCClaims{}, fmt.Errorf("oidc id token is required")
	}
	if strings.TrimSpace(options.JWKSURI) == "" {
		return OIDCClaims{}, fmt.Errorf("oidc jwks uri is required")
	}
	keySet, err := v.keySet(ctx, options.JWKSURI)
	if err != nil {
		return OIDCClaims{}, err
	}
	token, err := jwtjwx.ParseString(raw, jwtjwx.WithKeySet(keySet, jwsjwx.WithInferAlgorithmFromKey(true)), jwtjwx.WithValidate(false))
	if err != nil {
		return OIDCClaims{}, fmt.Errorf("verify oidc id token: %w", err)
	}
	return claimsFromJWT(token, raw)
}

func (v *JWKSIDTokenValidator) keySet(ctx context.Context, jwksURI string) (jwkjwx.Set, error) {
	now := v.now()
	v.mu.Lock()
	if cached, ok := v.cache[jwksURI]; ok && cached.expiresAt.After(now) {
		v.mu.Unlock()
		return cached.set, nil
	}
	v.mu.Unlock()

	fetchCtx := ctx
	cancel := func() {}
	if _, ok := ctx.Deadline(); !ok {
		fetchCtx, cancel = context.WithTimeout(ctx, 5*time.Second)
	}
	defer cancel()

	request, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, jwksURI, nil)
	if err != nil {
		return nil, fmt.Errorf("create oidc jwks request: %w", err)
	}
	body, err := doHTTP(v.httpClient, request)
	if err != nil {
		return nil, fmt.Errorf("fetch oidc jwks: %w", err)
	}
	keySet, err := jwkjwx.Parse(body)
	if err != nil {
		return nil, fmt.Errorf("parse oidc jwks: %w", err)
	}
	v.mu.Lock()
	v.cache[jwksURI] = cachedJWKS{set: keySet, expiresAt: now.Add(v.cacheTTL)}
	v.mu.Unlock()
	return keySet, nil
}

func claimsFromJWT(token jwtjwx.Token, _ string) (OIDCClaims, error) {
	issuer, _ := token.Issuer()
	subject, _ := token.Subject()
	audience, _ := token.Audience()
	expiresAt, _ := token.Expiration()
	issuedAt, _ := token.IssuedAt()
	var nonce string
	_ = token.Get("nonce", &nonce)
	var email string
	_ = token.Get("email", &email)
	var emailVerified bool
	_ = token.Get("email_verified", &emailVerified)
	var name string
	_ = token.Get("name", &name)
	var picture string
	_ = token.Get("picture", &picture)
	profileJSON, err := json.Marshal(map[string]any{
		"iss": issuer, "sub": subject, "aud": audience, "nonce": nonce,
		"email": email, "email_verified": emailVerified, "name": name, "picture": picture,
	})
	if err != nil {
		return OIDCClaims{}, fmt.Errorf("marshal verified OIDC claims: %w", err)
	}
	return OIDCClaims{
		Issuer:        issuer,
		Subject:       subject,
		Audience:      audience,
		ExpiresAt:     expiresAt,
		IssuedAt:      issuedAt,
		Nonce:         nonce,
		Email:         email,
		EmailVerified: emailVerified,
		Name:          name,
		Picture:       picture,
		Raw:           string(profileJSON),
	}, nil
}
