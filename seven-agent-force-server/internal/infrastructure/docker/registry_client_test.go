package docker

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

func TestRegistryClientConnectionErrorIsOperationError(t *testing.T) {
	client := newRegistryHTTPClient(0)
	_, err := client.ListRepositories(context.Background(), registryRuntime{
		Endpoint:   "http://127.0.0.1:1",
		APIBaseURL: "http://127.0.0.1:1/v2",
		AuthType:   "ANONYMOUS",
	}, 1, 20, "", 64)
	if err == nil {
		t.Fatal("expected connection error")
	}
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code() != apperrors.CodeOperateError {
		t.Fatalf("expected operation error, got %#v", appErr)
	}
	if !strings.Contains(appErr.Message(), "registry 请求失败") {
		t.Fatalf("expected registry failure message, got %q", appErr.Message())
	}
}

func TestRegistryClientBearerChallengeCatalog(t *testing.T) {
	tokenHits := 0
	catalogHits := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			tokenHits++
			if r.URL.Query().Get("service") != "registry.local" {
				t.Fatalf("unexpected token service: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "registry-token"})
		case "/v2/_catalog":
			catalogHits++
			if got := r.Header.Get("Authorization"); got != "Bearer registry-token" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="`+server.URL+`/token",service="registry.local",scope="registry:catalog:*"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"repositories": []string{"library/nginx", "internal/app"}})
		default:
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+server.URL+`/token",service="registry.local"`)
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer server.Close()

	client := newRegistryHTTPClient(0)
	page, err := client.ListRepositories(context.Background(), registryRuntime{
		Endpoint:   server.URL,
		APIBaseURL: server.URL + "/v2",
		AuthType:   "TOKEN_CHALLENGE",
		Username:   "robot",
		Password:   "secret",
	}, 1, 20, "", 64)
	if err != nil {
		t.Fatalf("ListRepositories returned error: %v", err)
	}
	if tokenHits != 1 || catalogHits != 2 {
		t.Fatalf("expected one token request and one challenged catalog retry, got token=%d catalog=%d", tokenHits, catalogHits)
	}
	rows := page.Records
	if len(rows) != 2 || rows[0].Repository != "library/nginx" || rows[1].Repository != "internal/app" {
		t.Fatalf("unexpected repositories: %#v", rows)
	}
}

func TestRegistryClientBearerChallengeTestReturnsDiscovery(t *testing.T) {
	tokenHits := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			tokenHits++
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "registry-token"})
		case "/v2/_catalog":
			if got := r.Header.Get("Authorization"); got != "Bearer registry-token" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="`+server.URL+`/token",service="registry.local",scope="registry:catalog:*"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"repositories": []string{}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newRegistryHTTPClient(0)
	result := client.Test(context.Background(), registryRuntime{
		Endpoint:   server.URL,
		APIBaseURL: server.URL + "/v2",
		AuthType:   "TOKEN_CHALLENGE",
		Username:   "robot",
		Password:   "secret",
	})
	if !result.Success || tokenHits != 1 {
		t.Fatalf("expected successful challenge test, got %#v tokenHits=%d", result, tokenHits)
	}
	if result.TokenRealm != server.URL+"/token" || result.TokenService != "registry.local" {
		t.Fatalf("expected discovered realm/service, got %#v", result)
	}
}

func TestRegistryClientCatalogStopsAfterRequestedPage(t *testing.T) {
	pageHits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageHits++
		if pageHits == 1 {
			w.Header().Set("Link", `</v2/_catalog?n=2&last=b>; rel="next"`)
			_ = json.NewEncoder(w).Encode(map[string]any{"repositories": []string{"a", "b"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"repositories": []string{"c", "d"}})
	}))
	defer server.Close()

	client := newRegistryHTTPClient(0)
	page, err := client.ListRepositories(context.Background(), registryRuntime{
		Endpoint:   server.URL,
		APIBaseURL: server.URL + "/v2",
		AuthType:   "ANONYMOUS",
	}, 1, 2, "", 64)
	if err != nil {
		t.Fatalf("ListRepositories returned error: %v", err)
	}
	if pageHits != 1 {
		t.Fatalf("expected first page only, got pageHits=%d", pageHits)
	}
	if len(page.Records) != 2 || page.Records[0].Repository != "a" || page.Total != 3 {
		t.Fatalf("unexpected page: %#v", page)
	}
}

func TestRegistryNamespaceWhitelist(t *testing.T) {
	rt := registryRuntime{NamespaceWhitelistJSON: `["library/", "team/app"]`}
	if !allowRepository(rt, "library/nginx") {
		t.Fatalf("expected library namespace to be allowed")
	}
	if !allowRepository(rt, "team/app-api") {
		t.Fatalf("expected prefix namespace to be allowed")
	}
	if allowRepository(rt, "evil/nginx") {
		t.Fatalf("expected disallowed namespace to be rejected")
	}
}

func TestRegistrySecretNotReturned(t *testing.T) {
	svc := &service{}
	view := svc.toRegistryView(RegistryRecord{
		ID:               1,
		Name:             "Local",
		Code:             "local",
		RegistryType:     "REGISTRY",
		Endpoint:         "http://127.0.0.1:5000",
		AuthType:         "BASIC",
		SecretCiphertext: validString("ciphertext"),
		SecretEDEK:       validString("edek"),
		WrapKeyRef:       validString("key"),
	})
	payload, _ := json.Marshal(view)
	if strings.Contains(string(payload), "ciphertext") || strings.Contains(string(payload), "edek") || strings.Contains(string(payload), "key") {
		t.Fatalf("secret material leaked in view: %s", payload)
	}
	if !view.SecretConfigured {
		t.Fatalf("expected secretConfigured=true")
	}
}

func TestQualifyRegistryRepository(t *testing.T) {
	rt := registryRuntime{Endpoint: "http://127.0.0.1:15000"}
	if got := qualifyRegistryRepository(rt, "library/nginx"); got != "127.0.0.1:15000/library/nginx" {
		t.Fatalf("unexpected qualified repository: %s", got)
	}
	if got := qualifyRegistryRepository(rt, "registry.local/team/app"); got != "registry.local/team/app" {
		t.Fatalf("repository with explicit registry host should remain unchanged: %s", got)
	}
	if got := qualifyRegistryRepository(rt, "localhost/app"); got != "localhost/app" {
		t.Fatalf("localhost repository should remain unchanged: %s", got)
	}
}

func TestComposeOutputMasking(t *testing.T) {
	output := sanitizeOutput("password: plain\napiToken: abc\nTOKEN=xyz\nnormal: ok", 0)
	if strings.Contains(output, "plain") || strings.Contains(output, "abc") || strings.Contains(output, "xyz") {
		t.Fatalf("sensitive compose output not masked: %s", output)
	}
	if !strings.Contains(output, "normal: ok") {
		t.Fatalf("non-sensitive output should remain visible: %s", output)
	}
}

func TestNormalizeInspectMasksSecrets(t *testing.T) {
	inspect := normalizeInspect(map[string]any{
		"Config": map[string]any{
			"Env":    []string{"NORMAL=ok", "PASSWORD=secret", "API_TOKEN=token"},
			"Labels": map[string]string{"normal": "ok", "secret.token": "hidden"},
		},
	})
	payload, _ := json.Marshal(inspect)
	text := string(payload)
	for _, leaked := range []string{"hidden", "PASSWORD=secret", "API_TOKEN=token"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("inspect leaked sensitive value %q in %s", leaked, text)
		}
	}
	if !strings.Contains(text, "NORMAL=ok") {
		t.Fatalf("non-sensitive env should remain visible: %s", text)
	}
}

func TestSafeLabelsMasksSecrets(t *testing.T) {
	labels := safeLabels(map[string]string{
		"normal":       "ok",
		"secret.token": "hidden",
	})
	if labels["normal"] != "ok" {
		t.Fatalf("normal label should remain visible: %#v", labels)
	}
	if labels["secret.token"] != "******" {
		t.Fatalf("sensitive label should be masked: %#v", labels)
	}
}

func validString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: true}
}
