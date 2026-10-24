package infrastructure

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/domain"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestOpenAIDiscoveryUsesBearerAndReturnsIDs(t *testing.T) {
	discovery := NewModelDiscoveryWithClient(&http.Client{Transport: roundTrip(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1/models" || request.Header.Get("Authorization") != "Bearer secret" || request.Header.Get("x-api-key") != "" {
			t.Fatalf("request=%s headers=%v", request.URL, request.Header)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"gpt-b"},{"id":"gpt-a"}]}`)), Header: make(http.Header)}, nil
	})})
	models, err := discovery.Discover(context.Background(), domain.RuntimeCredential{Protocol: domain.ProtocolOpenAI, BaseURL: "https://models.example/v1", APIKey: "secret"})
	if err != nil || len(models) != 2 || models[0] != "gpt-b" {
		t.Fatalf("models=%v err=%v", models, err)
	}
}

func TestAnthropicDiscoveryReadsAllPages(t *testing.T) {
	calls := 0
	discovery := NewModelDiscoveryWithClient(&http.Client{Transport: roundTrip(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.Header.Get("x-api-key") != "secret" || request.Header.Get("anthropic-version") != "2023-06-01" {
			t.Fatalf("headers=%v", request.Header)
		}
		body := `{"data":[{"id":"claude-a"}],"has_more":true,"last_id":"a"}`
		if calls == 2 {
			if request.URL.Query().Get("after_id") != "a" {
				t.Fatalf("query=%v", request.URL.Query())
			}
			body = `{"data":[{"id":"claude-b"}],"has_more":false}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})})
	models, err := discovery.Discover(context.Background(), domain.RuntimeCredential{Protocol: domain.ProtocolAnthropic, BaseURL: "https://anthropic.example/v1", APIKey: "secret"})
	if err != nil || calls != 2 || len(models) != 2 || models[1] != "claude-b" {
		t.Fatalf("models=%v calls=%d err=%v", models, calls, err)
	}
}
