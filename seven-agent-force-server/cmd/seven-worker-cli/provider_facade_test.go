package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type facadeRoundTripFunc func(*http.Request) (*http.Response, error)

func (f facadeRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestProviderFacadePreservesBytesAndDropsAuthorizationAndInternalHeaders(t *testing.T) {
	wantRequest := []byte("{ \"model\":\"gpt-test\", \"input\" : [1,2] }")
	wantResponse := []byte("data: {\"type\":\"response.completed\"}\n\n")
	var observed *http.Request
	client := &http.Client{Transport: facadeRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		observed = request.Clone(context.Background())
		body, _ := io.ReadAll(request.Body)
		observed.Body = io.NopCloser(bytes.NewReader(body))
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(bytes.NewReader(wantResponse))}, nil
	})}
	handler := newProviderFacadeHandler(providerFacadeSpec{Model: "gpt-test", Protocol: "OPENAI", ProxyBaseURL: "http://seven-provider-proxy:8080"}, client)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(wantRequest))
	request.Header.Set("Authorization", "Bearer hostile")
	request.Header.Set("X-Seven-Worker-ID", "forged")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), wantResponse) {
		t.Fatalf("response status=%d body=%q", response.Code, response.Body.Bytes())
	}
	if observed == nil || observed.Header.Get("Authorization") != "" || observed.Header.Get("X-Seven-Worker-ID") != "" {
		t.Fatalf("Facade leaked inbound authority: %+v", observed)
	}
	body, _ := io.ReadAll(observed.Body)
	if !bytes.Equal(body, wantRequest) {
		t.Fatalf("Facade changed request bytes: %q", body)
	}
}

func TestProviderFacadeRejectsUnsupportedRoutesAndInvalidModel(t *testing.T) {
	handler := newProviderFacadeHandler(providerFacadeSpec{Model: "gpt-test", Protocol: "OPENAI", ProxyBaseURL: "http://seven-provider-proxy:8080"}, &http.Client{Transport: facadeRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("Proxy must not be called")
		return nil, nil
	})})
	for _, test := range []struct {
		method, path, body string
		want               int
	}{
		{http.MethodGet, "/v1/responses", `{}`, http.StatusMethodNotAllowed},
		{http.MethodPost, "/v1/models", `{}`, http.StatusNotFound},
		{http.MethodPost, "/v1/messages", `{"model":"gpt-test"}`, http.StatusNotFound},
		{http.MethodPost, "/v1/responses", `{"model":""}`, http.StatusBadRequest},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(test.method, test.path, strings.NewReader(test.body)))
		if response.Code != test.want {
			t.Fatalf("%s %s status=%d want=%d", test.method, test.path, response.Code, test.want)
		}
	}
}

func TestProviderFacadeAllowsCatalogExternalHelperModel(t *testing.T) {
	var gotBody []byte
	handler := newProviderFacadeHandler(providerFacadeSpec{Model: "gpt-test", Protocol: "OPENAI", ProxyBaseURL: "http://seven-provider-proxy:8080"}, &http.Client{Transport: facadeRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		gotBody, _ = io.ReadAll(request.Body)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"resp_helper"}`))}, nil
	})})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"codex-autoreview"}`)))
	if response.Code != http.StatusOK || !bytes.Contains(gotBody, []byte(`"codex-autoreview"`)) {
		t.Fatalf("helper model was not forwarded unchanged: status=%d body=%s", response.Code, gotBody)
	}
}

func TestProviderFacadeForwardsAnthropicMessagesWithoutRuntimeCredential(t *testing.T) {
	var paths []string
	handler := newProviderFacadeHandler(providerFacadeSpec{Model: "claude-primary", Protocol: "ANTHROPIC", ProxyBaseURL: "http://seven-provider-proxy:8080"}, &http.Client{Transport: facadeRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		paths = append(paths, request.URL.Path)
		if request.Header.Get("Authorization") != "" || request.Header.Get("x-api-key") != "" {
			t.Fatalf("Facade leaked runtime credentials: %v", request.Header)
		}
		if request.Header.Get("anthropic-version") != "2023-06-01" || request.Header.Get("anthropic-beta") != "tools-2025-04-14" {
			t.Fatalf("Anthropic headers were not preserved: %v", request.Header)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Anthropic-Ratelimit-Requests-Remaining": []string{"9"}}, Body: io.NopCloser(strings.NewReader(`{"type":"message","model":"claude-helper"}`))}, nil
	})})
	for _, path := range []string{"/v1/messages", "/v1/messages/count_tokens"} {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"claude-helper"}`))
		request.Header.Set("Authorization", "Bearer hostile")
		request.Header.Set("x-api-key", "hostile")
		request.Header.Set("anthropic-version", "2023-06-01")
		request.Header.Set("anthropic-beta", "tools-2025-04-14")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status=%d", path, response.Code)
		}
		if response.Header().Get("Anthropic-Ratelimit-Requests-Remaining") != "9" {
			t.Fatalf("%s lost bounded Anthropic response headers: %v", path, response.Header())
		}
	}
	if strings.Join(paths, ",") != "/v1/messages,/v1/messages/count_tokens" {
		t.Fatalf("paths=%v", paths)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"claude-helper"}`)))
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross-protocol status=%d", response.Code)
	}
}

func TestProviderFacadeWaitsForServerRunningConfirmation(t *testing.T) {
	var upstreamCalls atomic.Int64
	client := &http.Client{Transport: facadeRoundTripFunc(func(*http.Request) (*http.Response, error) {
		upstreamCalls.Add(1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_gate"}`)),
		}, nil
	})}
	confirmed := make(chan struct{})
	handler := newProviderFacadeHandler(providerFacadeSpec{Model: "gpt-test", Protocol: "OPENAI", ProxyBaseURL: "http://seven-provider-proxy:8080"}, client, confirmed)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-test"}`)))
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("Provider request escaped before Server RUNNING confirmation")
	case <-time.After(30 * time.Millisecond):
	}
	if upstreamCalls.Load() != 0 {
		t.Fatalf("upstream calls before confirmation=%d", upstreamCalls.Load())
	}
	close(confirmed)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Provider request did not continue after confirmation")
	}
	if upstreamCalls.Load() != 1 || response.Code != http.StatusOK {
		t.Fatalf("upstream calls=%d status=%d", upstreamCalls.Load(), response.Code)
	}
}
