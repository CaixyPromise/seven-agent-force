package drivers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
	jwtjwx "github.com/lestrrat-go/jwx/v3/jwt"
)

func TestClaimsFromJWTStoresVerifiedClaimsJSONNotCompactToken(t *testing.T) {
	token := jwtjwx.New()
	for key, value := range map[string]any{"iss": "https://hub.example.com", "sub": "owner", "aud": []string{"node-a"}, "email": "owner@example.com", "email_verified": true} {
		if err := token.Set(key, value); err != nil {
			t.Fatalf("set %s: %v", key, err)
		}
	}
	claims, err := claimsFromJWT(token, "header.payload.signature")
	if err != nil {
		t.Fatalf("claimsFromJWT: %v", err)
	}
	var stored map[string]any
	if err := json.Unmarshal([]byte(claims.Raw), &stored); err != nil {
		t.Fatalf("profile is not JSON: %v", err)
	}
	if strings.Contains(claims.Raw, "header.payload.signature") || stored["email"] != "owner@example.com" || stored["email_verified"] != true {
		t.Fatalf("unexpected stored claims: %s", claims.Raw)
	}
}

func TestRegistryRejectsDuplicateProviderDriver(t *testing.T) {
	registry := NewRegistry()

	if err := registry.Register(fakeDriver{code: "github"}); err != nil {
		t.Fatalf("register first driver: %v", err)
	}
	if err := registry.Register(fakeDriver{code: "github"}); err == nil {
		t.Fatal("expected duplicate provider driver to be rejected")
	}
}

func TestGitHubDriverBuildsAuthorizationURLWithStateAndPKCE(t *testing.T) {
	driver := NewGitHubDriver()
	provider := domain.Provider{
		ProviderCode: "github",
		ClientID:     "github-client",
		RedirectURI:  "https://seven.example/login/external/github/callback",
		Scopes:       []string{"read:user", "user:email"},
	}

	authURL, err := driver.BuildAuthorizationURL(context.Background(), provider, AuthorizationRequest{
		State:               "state-123",
		CodeChallenge:       "pkce-challenge",
		CodeChallengeMethod: "S256",
	})
	if err != nil {
		t.Fatalf("build authorization url: %v", err)
	}

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse authorization url: %v", err)
	}
	values := parsed.Query()
	if parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.Path != "/login/oauth/authorize" {
		t.Fatalf("unexpected GitHub authorization endpoint: %s", authURL)
	}
	if got := values.Get("client_id"); got != "github-client" {
		t.Fatalf("client_id = %q", got)
	}
	if got := values.Get("redirect_uri"); got != provider.RedirectURI {
		t.Fatalf("redirect_uri = %q", got)
	}
	if got := values.Get("scope"); got != "read:user user:email" {
		t.Fatalf("scope = %q", got)
	}
	if got := values.Get("state"); got != "state-123" {
		t.Fatalf("state = %q", got)
	}
	if got := values.Get("code_challenge"); got != "pkce-challenge" {
		t.Fatalf("code_challenge = %q", got)
	}
	if got := values.Get("code_challenge_method"); got != "S256" {
		t.Fatalf("code_challenge_method = %q", got)
	}
}

func TestGoogleDriverRejectsIDTokenWithWrongAudience(t *testing.T) {
	driver := NewGoogleDriver(WithOIDCIDTokenValidator(fakeOIDCValidator{
		claims: OIDCClaims{
			Issuer:        "https://accounts.google.com",
			Subject:       "google-subject",
			Audience:      []string{"wrong-client"},
			ExpiresAt:     time.Now().Add(time.Hour),
			IssuedAt:      time.Now().Add(-time.Minute),
			Nonce:         "nonce-123",
			Email:         "person@example.com",
			EmailVerified: true,
		},
	}))

	_, err := driver.ResolveProfile(context.Background(), domain.Provider{
		ProviderCode: "google",
		Issuer:       "https://accounts.google.com",
		ClientID:     "expected-client",
	}, TokenExchangeResult{
		TokenSet:       domain.TokenSet{IDToken: "signed-id-token"},
		ExpectedIssuer: "https://accounts.google.com",
		ExpectedNonce:  "nonce-123",
	})
	if err == nil {
		t.Fatal("expected wrong audience to be rejected")
	}
	if !strings.Contains(err.Error(), "audience") {
		t.Fatalf("expected audience error, got %v", err)
	}
}

func TestGoogleDriverRejectsIDTokenWithoutExpectedNonce(t *testing.T) {
	driver := NewGoogleDriver(WithOIDCIDTokenValidator(fakeOIDCValidator{
		claims: OIDCClaims{
			Issuer:        "https://accounts.google.com",
			Subject:       "google-subject",
			Audience:      []string{"expected-client"},
			ExpiresAt:     time.Now().Add(time.Hour),
			IssuedAt:      time.Now().Add(-time.Minute),
			Nonce:         "nonce-from-token",
			Email:         "person@example.com",
			EmailVerified: true,
		},
	}))

	_, err := driver.ResolveProfile(context.Background(), domain.Provider{
		ProviderCode: "google",
		Issuer:       "https://accounts.google.com",
		ClientID:     "expected-client",
	}, TokenExchangeResult{
		TokenSet:       domain.TokenSet{IDToken: "signed-id-token"},
		ExpectedIssuer: "https://accounts.google.com",
	})
	if err == nil {
		t.Fatal("expected missing expected nonce to be rejected")
	}
	if !strings.Contains(err.Error(), "nonce") {
		t.Fatalf("expected nonce error, got %v", err)
	}
}

func TestOIDCDriverRejectsIssuerMixUp(t *testing.T) {
	driver := NewOIDCDriver(WithOIDCIDTokenValidator(fakeOIDCValidator{}))

	_, err := driver.ExchangeCode(context.Background(), domain.Provider{
		ProviderCode:  "oidc",
		Issuer:        "https://issuer.example",
		ClientID:      "client-id",
		TokenEndpoint: "https://issuer.example/oauth/token",
	}, TokenExchangeRequest{
		Code:           "code-123",
		ExpectedIssuer: "https://issuer.example",
		CallbackIssuer: "https://attacker.example",
	})
	if err == nil {
		t.Fatal("expected issuer mix-up to be rejected")
	}
	if !strings.Contains(err.Error(), "issuer") {
		t.Fatalf("expected issuer error, got %v", err)
	}
}

func TestOIDCExchangeUsesOnlyConfiguredTokenEndpointAuthentication(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		authMethod string
		wantBasic  bool
	}{
		{name: "managed-basic", authMethod: domain.TokenEndpointAuthMethodClientSecretBasic, wantBasic: true},
		{name: "ordinary-compatible-body", wantBasic: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var authorization string
			var form url.Values
			driver := NewOIDCDriver(WithOIDCHTTPClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				authorization = request.Header.Get("Authorization")
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Fatalf("read token request: %v", err)
				}
				form, err = url.ParseQuery(string(body))
				if err != nil {
					t.Fatalf("parse token request: %v", err)
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"access_token":"access","token_type":"Bearer","expires_in":300}`)), Request: request}, nil
			})}))
			provider := domain.Provider{
				ProviderCode: "oidc", ProtocolType: domain.ProtocolTypeOIDC, Issuer: "https://hub.example.com",
				TokenEndpoint: "https://hub.example.com/sso/oauth2/token", ClientID: "client-id", RedirectURI: "https://node.example.com/callback",
				TokenEndpointAuthMethod: testCase.authMethod,
			}
			if _, err := driver.ExchangeCode(context.Background(), provider, TokenExchangeRequest{
				Code: "code", CodeVerifier: "verifier", RedirectURI: provider.RedirectURI, ClientSecret: "client-secret", ExpectedIssuer: provider.Issuer,
			}); err != nil {
				t.Fatalf("exchange code: %v", err)
			}
			if got := strings.HasPrefix(authorization, "Basic "); got != testCase.wantBasic {
				t.Fatalf("Authorization=%q basic=%v want=%v", authorization, got, testCase.wantBasic)
			}
			if testCase.wantBasic {
				if form.Get("client_id") != "" || form.Get("client_secret") != "" {
					t.Fatalf("Basic request leaked client credentials into form: %v", form)
				}
			} else if form.Get("client_id") != provider.ClientID || form.Get("client_secret") != "client-secret" {
				t.Fatalf("ordinary request credentials=%v", form)
			}
		})
	}
}

func TestOIDCDiscoveryRequiresExactIssuerAndRejectsRedirects(t *testing.T) {
	redirected := false
	driver := NewOIDCDriver(WithOIDCHTTPClient(&http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			redirected = true
			return http.ErrUseLastResponse
		},
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Set("Location", "https://other.example/.well-known/openid-configuration")
			return &http.Response{StatusCode: http.StatusFound, Body: io.NopCloser(strings.NewReader("")), Header: header, Request: r}, nil
		}),
	}))
	if _, err := driver.Discover(context.Background(), "https://hub.example"); err == nil {
		t.Fatal("expected discovery redirect rejection")
	}
	if !redirected {
		t.Fatal("discovery redirect policy was not consulted")
	}
}

func TestOIDCDiscoveryBuildsWellKnownURLForPathIssuers(t *testing.T) {
	tests := map[string]string{
		"https://hub.example":              "https://hub.example/.well-known/openid-configuration",
		"https://hub.example/":             "https://hub.example/.well-known/openid-configuration/",
		"https://hub.example/tenant":       "https://hub.example/.well-known/openid-configuration/tenant",
		"https://hub.example/a/b":          "https://hub.example/.well-known/openid-configuration/a/b",
		"https://hub.example/a%2Fb/tenant": "https://hub.example/.well-known/openid-configuration/a%2Fb/tenant",
	}
	for issuer, want := range tests {
		t.Run(issuer, func(t *testing.T) {
			var got string
			driver := NewOIDCDriver(WithOIDCHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				got = r.URL.String()
				doc := fmt.Sprintf(`{"issuer":%q,"authorization_endpoint":"https://hub.example/authorize","token_endpoint":"https://hub.example/token","jwks_uri":"https://hub.example/jwks"}`, issuer)
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(doc)), Header: make(http.Header), Request: r}, nil
			})}))
			if _, err := driver.Discover(context.Background(), issuer); err != nil {
				t.Fatalf("discover: %v", err)
			}
			if got != want {
				t.Fatalf("discovery URL=%q want=%q", got, want)
			}
		})
	}
}

func TestOIDCDiscoveryRejectsIssuerURLComponents(t *testing.T) {
	for _, issuer := range []string{"https://user@hub.example", "https://hub.example/path?x=1", "https://hub.example/path#fragment"} {
		if _, err := NewOIDCDriver().Discover(context.Background(), issuer); err == nil {
			t.Fatalf("accepted unsafe issuer %q", issuer)
		}
	}
}

func TestOIDCDiscoveryRejectsIssuerMismatchMalformedAndProductionHTTP(t *testing.T) {
	for name, testCase := range map[string]struct{ issuer, document string }{
		"issuer-mismatch": {"https://hub.example", `{"issuer":"https://other.example","authorization_endpoint":"https://hub.example/authorize","token_endpoint":"https://hub.example/token","jwks_uri":"https://hub.example/jwks"}`},
		"malformed":       {"https://hub.example", `{"issuer":`},
		"production-http": {"http://hub.example", `{}`},
	} {
		t.Run(name, func(t *testing.T) {
			driver := NewOIDCDriver(WithOIDCHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(testCase.document)), Header: make(http.Header), Request: r}, nil
			})}))
			if _, err := driver.Discover(context.Background(), testCase.issuer); err == nil {
				t.Fatal("expected discovery rejection")
			}
		})
	}
}

func TestOIDCDiscoveryRejectsOversizeAndCrossOriginEndpoints(t *testing.T) {
	for name, document := range map[string]string{
		"oversize":     strings.Repeat("x", (64<<10)+1),
		"cross-origin": `{"issuer":"https://hub.example","authorization_endpoint":"https://evil.example/authorize","token_endpoint":"https://hub.example/token","jwks_uri":"https://hub.example/jwks"}`,
	} {
		t.Run(name, func(t *testing.T) {
			driver := NewOIDCDriver(WithOIDCHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(document)), Header: make(http.Header), Request: r}, nil
			})}))
			if _, err := driver.Discover(context.Background(), "https://hub.example"); err == nil {
				t.Fatal("expected unsafe discovery document rejection")
			}
		})
	}
}

func TestGitHubDriverDoesNotLeakUpstreamErrorBody(t *testing.T) {
	driver := NewGitHubDriver(WithGitHubHTTPClient(&http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Status:     "400 Bad Request",
				Body:       io.NopCloser(strings.NewReader(`{"access_token":"gho_fake_secret_token","error":"bad_verification_code"}`)),
				Header:     make(http.Header),
				Request:    r,
			}, nil
		}),
	}))
	_, err := driver.ExchangeCode(context.Background(), domain.Provider{
		ProviderCode:  "github",
		ClientID:      "github-client",
		TokenEndpoint: "https://github.example/token",
		RedirectURI:   "https://seven.example/login/external/github/callback",
	}, TokenExchangeRequest{
		Code:         "code-123",
		CodeVerifier: "verifier-123",
	})
	if err == nil {
		t.Fatal("expected upstream token endpoint failure")
	}
	message := err.Error()
	if strings.Contains(message, "gho_fake_secret_token") || strings.Contains(message, "bad_verification_code") || strings.Contains(message, "access_token") {
		t.Fatalf("upstream error leaked raw body: %v", err)
	}
	if !strings.Contains(message, "HTTP 400") {
		t.Fatalf("expected sanitized status code, got %v", err)
	}
}

type fakeDriver struct {
	code string
}

func (f fakeDriver) Code() string {
	return f.code
}

func (f fakeDriver) Capabilities() domain.ProviderCapability {
	return domain.ProviderCapability{ProviderCode: f.code}
}

func (f fakeDriver) BuildAuthorizationURL(context.Context, domain.Provider, AuthorizationRequest) (string, error) {
	return "", nil
}

func (f fakeDriver) ExchangeCode(context.Context, domain.Provider, TokenExchangeRequest) (*TokenExchangeResult, error) {
	return nil, nil
}

func (f fakeDriver) ResolveProfile(context.Context, domain.Provider, TokenExchangeResult) (*domain.ExternalProfile, error) {
	return nil, nil
}

func (f fakeDriver) RefreshToken(context.Context, domain.Provider, domain.TokenSet) (*domain.TokenSet, error) {
	return nil, nil
}

func (f fakeDriver) RevokeToken(context.Context, domain.Provider, domain.TokenSet) error {
	return nil
}

type fakeOIDCValidator struct {
	claims OIDCClaims
	err    error
}

func (f fakeOIDCValidator) ValidateIDToken(context.Context, string, OIDCValidationOptions) (OIDCClaims, error) {
	if f.err != nil {
		return OIDCClaims{}, f.err
	}
	return f.claims, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
