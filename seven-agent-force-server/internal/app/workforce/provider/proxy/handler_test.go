package proxy

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewcontract"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func testConfig(t *testing.T) Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "worker.token")
	if err := os.WriteFile(path, []byte(strings.Repeat("a", 64)), 0o600); err != nil {
		t.Fatal(err)
	}
	return Config{
		GatewayBaseURL: "https://framework.internal", WorkerTokenFile: path,
		WorkerID: 1688799477, WorkerInstanceID: "wki_1", HostBootID: "boot_1",
		TaskID: 2610240832, AttemptID: 3107414342, Generation: 1, OwnerEpoch: 1,
		AttemptLeaseToken: strings.Repeat("l", 64), ModelCredentialID: 3260443846,
		CredentialVersion: 3, ModelProtocol: "OPENAI", PrimaryModel: "gpt-test",
	}
}

func TestHandlerOnlyAllowsResponses(t *testing.T) {
	handler, err := NewHandler(testConfig(t), &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("Gateway must not be called")
		return nil, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		method, path string
		want         int
	}{
		{http.MethodPost, "/v1/models", http.StatusNotFound},
		{http.MethodGet, "/v1/responses", http.StatusMethodNotAllowed},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(test.method, test.path, strings.NewReader(`{"model":"gpt-test"}`)))
		if response.Code != test.want {
			t.Fatalf("%s %s status=%d want=%d", test.method, test.path, response.Code, test.want)
		}
	}
}

func TestHandlerUsesWorkerAuthorityAndPreservesBytes(t *testing.T) {
	requestBytes := []byte("{ \"model\" : \"gpt-test\", \"stream\":true, \"input\" : [1,2] }")
	streamBytes := []byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\ndata: {\"type\":\"response.completed\"}\n\n")
	var observed *http.Request
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		observed = request.Clone(context.Background())
		body, _ := io.ReadAll(request.Body)
		observed.Body = io.NopCloser(bytes.NewReader(body))
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(bytes.NewReader(streamBytes))}, nil
	})}
	handler, err := NewHandler(testConfig(t), client)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(requestBytes))
	request.Header.Set("Authorization", "Bearer hostile-placeholder")
	request.Header.Set("X-Seven-Worker-ID", "forged")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), streamBytes) {
		t.Fatalf("response status=%d body=%q", response.Code, response.Body.Bytes())
	}
	if observed == nil || observed.URL.Path != "/api/internal/workforce/v1/model-invocations/responses" {
		t.Fatalf("Gateway request=%v", observed)
	}
	gotBody, _ := io.ReadAll(observed.Body)
	if !bytes.Equal(gotBody, requestBytes) {
		t.Fatalf("request bytes changed: got=%q want=%q", gotBody, requestBytes)
	}
	if observed.Header.Get("Authorization") != "Bearer "+strings.Repeat("a", 64) || observed.Header.Get("X-Seven-Worker-ID") != "1688799477" {
		t.Fatalf("Gateway identity headers=%v", observed.Header)
	}
	for _, required := range []string{"X-Seven-Invocation-ID", "X-Seven-Nonce", "X-Seven-Body-Sha256", "X-Seven-Attempt-Lease"} {
		if observed.Header.Get(required) == "" {
			t.Fatalf("missing %s", required)
		}
	}
}

func TestStreamingBodyCanOutliveHeaderTimeout(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: first\n\n"))
		w.(http.Flusher).Flush()
		time.Sleep(60 * time.Millisecond)
		_, _ = w.Write([]byte("data: second\n\n"))
	}))
	defer upstream.Close()
	config := testConfig(t)
	config.GatewayBaseURL = upstream.URL
	handler, err := NewHandler(config, &http.Client{Timeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-test","stream":true}`)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "data: second") {
		t.Fatalf("stream was truncated: status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestHandlerAllowsAuxiliaryModelAndRejectsBackground(t *testing.T) {
	called := false
	handler, err := NewHandler(testConfig(t), &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"ok"}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"helper-model"}`)))
	if response.Code != http.StatusOK || !called {
		t.Fatalf("auxiliary model status=%d called=%v", response.Code, called)
	}
	called = false
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-test","background":true}`)))
	if response.Code != http.StatusBadRequest || called {
		t.Fatalf("background status=%d called=%v", response.Code, called)
	}
}

func TestHandlerPinsProtocolButNotModelCatalog(t *testing.T) {
	config := testConfig(t)
	config.ModelProtocol = "ANTHROPIC"
	called := 0
	handler, err := NewHandler(config, &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		called++
		if request.URL.Path != "/api/internal/workforce/v1/model-invocations/messages" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"type":"message"}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-helper"}`)))
	if response.Code != http.StatusOK || called != 1 {
		t.Fatalf("Anthropic helper call status=%d called=%d", response.Code, called)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-helper"}`)))
	if response.Code != http.StatusNotFound || called != 1 {
		t.Fatalf("cross-protocol call status=%d called=%d", response.Code, called)
	}
}

func TestCopyResponseHeadersAllowsBoundedAnthropicMetadata(t *testing.T) {
	target := make(http.Header)
	copyResponseHeaders(target, http.Header{
		"Anthropic-Ratelimit-Requests-Remaining": []string{"7"},
		"Set-Cookie":                             []string{"secret=blocked"},
	})
	if target.Get("Anthropic-Ratelimit-Requests-Remaining") != "7" || target.Get("Set-Cookie") != "" {
		t.Fatalf("headers=%v", target)
	}
}

func TestAuthorityAdoptionFencesOldEpoch(t *testing.T) {
	handler, err := NewHandler(testConfig(t), &http.Client{})
	if err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)
	if err := handler.AdoptOwner(1, 2, "wki_2", strings.Repeat("n", 64), expiry, 1001, strings.Repeat("p", 64), expiry); err != nil {
		t.Fatal(err)
	}
	if state := handler.Authority(); state.OwnerEpoch != 2 || state.WorkerInstanceID != "wki_2" {
		t.Fatalf("authority=%+v", state)
	}
	if err := handler.AdoptOwner(1, 3, "wki_3", strings.Repeat("x", 64), "", 1002, strings.Repeat("q", 64), expiry); err == nil {
		t.Fatal("stale owner adoption must fail")
	}
}

func TestAuthorityRejectsExpiredPermitAndNonMonotonicRenewal(t *testing.T) {
	handler, err := NewHandler(testConfig(t), &http.Client{})
	if err != nil {
		t.Fatal(err)
	}
	expired := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	future := time.Now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano)
	if err = handler.AdoptOwner(1, 2, "wki_2", strings.Repeat("n", 64), future, 1001, strings.Repeat("p", 64), expired); err == nil || err.Error() != "REATTACH_PERMIT_EXPIRED" {
		t.Fatalf("expired permit error=%v", err)
	}
	if err = handler.RenewAuthority(1, strings.Repeat("l", 64), future); err != nil {
		t.Fatal(err)
	}
	older := time.Now().UTC().Add(20 * time.Second).Format(time.RFC3339Nano)
	if err = handler.RenewAuthority(1, strings.Repeat("m", 64), older); err == nil || err.Error() != "LEASE_NOT_MONOTONIC" {
		t.Fatalf("non-monotonic renewal error=%v", err)
	}
}

func TestReviewHandlerStartsWithAndRenewsFiveMinuteLease(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	workerToken := filepath.Join(root, "worker.token")
	reviewLease := filepath.Join(root, "review.lease")
	if err := os.WriteFile(workerToken, []byte(strings.Repeat("w", 64)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reviewLease, []byte(strings.Repeat("l", 64)), 0o600); err != nil {
		t.Fatal(err)
	}
	expires := time.Now().UTC().Add(5 * time.Minute).Format(time.RFC3339Nano)
	config := Config{
		ExecutionKind:                    executionKindReviewRun,
		GatewayBaseURL:                   "https://framework.internal",
		WorkerTokenFile:                  workerToken,
		WorkerID:                         1001,
		WorkerInstanceID:                 "review-worker",
		HostBootID:                       "review-boot",
		TaskID:                           1002,
		OwnerEpoch:                       3,
		ReviewRunID:                      1003,
		ReviewRunAttemptID:               1004,
		ReviewLeaseFile:                  reviewLease,
		LeaseExpiresAt:                   expires,
		ModelCredentialID:                1005,
		CredentialVersion:                2,
		ModelProtocol:                    "OPENAI",
		ProviderStructuredOutputContract: reviewcontract.StructuredOutputContractV1,
		PrimaryModel:                     "review-model",
		ControlSocket:                    filepath.Join(root, "control.sock"),
	}
	handler, err := NewHandler(config, &http.Client{})
	if err != nil {
		t.Fatal(err)
	}
	if got := handler.Authority().LeaseExpiresAt; got == "" {
		t.Fatal("ReviewRun handler did not initialize local lease authority")
	}
	renewed := time.Now().UTC().Add(6 * time.Minute).Format(time.RFC3339Nano)
	if err = handler.RenewAuthority(config.OwnerEpoch, strings.Repeat("n", 64), renewed); err != nil {
		t.Fatalf("five-minute ReviewRun lease renewal was rejected: %v", err)
	}
}

func TestHandlerFailsClosedAfterLocalAuthorityExpires(t *testing.T) {
	handler, err := NewHandler(testConfig(t), &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("Gateway must not be called with expired local authority")
		return nil, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	handler.authority.Lock()
	handler.authority.LeaseExpiresAt = time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano)
	handler.authority.Unlock()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-test"}`)))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "AUTHORITY_EXPIRED") {
		t.Fatalf("expired authority response=%d body=%s", response.Code, response.Body.String())
	}
}
