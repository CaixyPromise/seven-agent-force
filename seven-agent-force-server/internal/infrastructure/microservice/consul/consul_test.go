package consul

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/microservice"
)

func TestClientDoesNotMutateInjectedHTTPClient(t *testing.T) {
	redirectErr := errors.New("caller redirect policy")
	injected := &http.Client{
		Timeout: 17 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return redirectErr
		},
	}

	const constructors = 32
	var wg sync.WaitGroup
	for range constructors {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client, err := NewClient(ClientOptions{Address: "http://127.0.0.1:8500", HTTPClient: injected, Timeout: 2 * time.Second})
			if err != nil {
				t.Error(err)
				return
			}
			if client.httpClient == injected {
				t.Error("Consul client retained caller-owned http.Client")
			}
			if client.httpClient.Timeout != 2*time.Second {
				t.Errorf("private timeout = %s, want 2s", client.httpClient.Timeout)
			}
		}()
	}
	wg.Wait()
	if injected.Timeout != 17*time.Second {
		t.Fatalf("injected timeout mutated to %s", injected.Timeout)
	}
	if err := injected.CheckRedirect(nil, nil); !errors.Is(err, redirectErr) {
		t.Fatalf("injected redirect policy mutated: %v", err)
	}
	client, err := NewClient(ClientOptions{Address: "http://127.0.0.1:8500"})
	if err != nil {
		t.Fatal(err)
	}
	if client.httpClient.Timeout <= 0 {
		t.Fatalf("default timeout = %s, want positive", client.httpClient.Timeout)
	}
}

func TestResolverUsesPassingHealthAPIAndACLHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/health/service/hub" || r.URL.Query().Get("passing") != "true" || r.URL.Query().Get("dc") != "dc1" {
			t.Fatalf("unexpected request URL: %s", r.URL.String())
		}
		if got := r.Header.Get("X-Consul-Token"); got != "secret" {
			t.Fatalf("token header = %q", got)
		}
		_, _ = w.Write([]byte(`[
          {"Node":{"Address":"10.0.0.1"},"Service":{"ID":"hub-a","Service":"hub","Address":"10.0.0.2","Port":9277,"Tags":["v1"],"Meta":{"protocol":"https"}},"Checks":[{"Status":"passing"}]},
          {"Node":{"Address":"10.0.0.3"},"Service":{"ID":"hub-b","Service":"hub","Port":9278},"Checks":[{"Status":"critical"}]}
        ]`))
	}))
	defer server.Close()
	client, err := NewClient(ClientOptions{Address: server.URL, Token: "secret", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	resolver := NewResolver(client, ResolverOptions{Datacenter: "dc1"})

	instances, err := resolver.Resolve(context.Background(), "hub")
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 1 || instances[0].ID != "hub-a" || instances[0].Host != "10.0.0.2" || instances[0].Scheme != "https" {
		t.Fatalf("Resolve() = %#v", instances)
	}
}

func TestResolverTreats200EmptyAsAuthoritativeAndDoesNotFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`[]`)) }))
	defer server.Close()
	client, _ := NewClient(ClientOptions{Address: server.URL, HTTPClient: server.Client()})
	fallback := &countingResolver{}
	resolver := NewResolver(client, ResolverOptions{Fallback: fallback})

	_, err := resolver.Resolve(context.Background(), "hub")
	if !errors.Is(err, microservice.ErrNoHealthyInstance) || fallback.calls.Load() != 0 {
		t.Fatalf("Resolve() error = %v, fallback calls = %d", err, fallback.calls.Load())
	}
}

func TestResolverFallsBackOnlyWhenConsulFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "down", http.StatusServiceUnavailable) }))
	defer server.Close()
	client, _ := NewClient(ClientOptions{Address: server.URL, HTTPClient: server.Client()})
	fallback := &countingResolver{instances: []microservice.ServiceInstance{{ID: "static", Healthy: true}}}
	resolver := NewResolver(client, ResolverOptions{Fallback: fallback})

	instances, err := resolver.Resolve(context.Background(), "hub")
	if err != nil || len(instances) != 1 || fallback.calls.Load() != 1 {
		t.Fatalf("Resolve() = %#v, %v, fallback calls = %d", instances, err, fallback.calls.Load())
	}
}

func TestRegistrarSendsHealthRegistrationAndEscapedDeregister(t *testing.T) {
	var registered atomic.Bool
	var deregistered atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/v1/agent/service/register":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			check := body["Check"].(map[string]any)
			if body["ID"] != "hub/a" || check["HTTP"] != "http://127.0.0.1:9277/internal/healthz" || check["Interval"] != "10s" || check["Timeout"] != "2s" {
				t.Fatalf("registration body = %#v", body)
			}
			registered.Store(true)
		case "/v1/agent/service/deregister/hub%2Fa":
			deregistered.Store(true)
		default:
			t.Fatalf("unexpected path: %s", r.URL.EscapedPath())
		}
	}))
	defer server.Close()
	client, _ := NewClient(ClientOptions{Address: server.URL, HTTPClient: server.Client()})
	registrar := NewRegistrar(client)
	registration := microservice.ServiceRegistration{
		ID: "hub/a", ServiceName: "hub", Port: 9277, Scheme: "http", HealthCheckPath: "/internal/healthz",
		HealthInterval: 10 * time.Second, HealthTimeout: 2 * time.Second,
	}
	if err := registrar.Register(context.Background(), registration); err != nil {
		t.Fatal(err)
	}
	if err := registrar.Deregister(context.Background(), registration.ID); err != nil {
		t.Fatal(err)
	}
	if !registered.Load() || !deregistered.Load() {
		t.Fatalf("registered=%v deregistered=%v", registered.Load(), deregistered.Load())
	}
}

func TestRegistrarBuildsBracketedIPv6HealthURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body registrationPayload
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Check.HTTP != "http://[2001:db8::1]:9277/internal/healthz" {
			t.Fatalf("health URL = %q", body.Check.HTTP)
		}
	}))
	defer server.Close()
	client, _ := NewClient(ClientOptions{Address: server.URL, HTTPClient: server.Client()})
	registrar := NewRegistrar(client)
	err := registrar.Register(context.Background(), microservice.ServiceRegistration{
		ID: "hub-v6", ServiceName: "hub", Address: "2001:db8::1", Port: 9277, Scheme: "http",
		HealthCheckPath: "/internal/healthz", HealthInterval: 10 * time.Second, HealthTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestConsulAdaptersReturnErrorsForNilDependenciesAndInvalidDurations(t *testing.T) {
	resolver := NewResolver(nil, ResolverOptions{})
	if _, err := resolver.Resolve(context.Background(), "hub"); !errors.Is(err, microservice.ErrInvalidDependency) {
		t.Fatalf("Resolve() error = %v", err)
	}
	registrar := NewRegistrar(nil)
	registration := microservice.ServiceRegistration{ID: "hub-a", ServiceName: "hub", Port: 9277}
	if err := registrar.Register(context.Background(), registration); !errors.Is(err, microservice.ErrInvalidDependency) {
		t.Fatalf("Register() error = %v", err)
	}
	if err := registrar.Deregister(context.Background(), "hub-a"); !errors.Is(err, microservice.ErrInvalidDependency) {
		t.Fatalf("Deregister() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("invalid registration must not issue HTTP request")
	}))
	defer server.Close()
	client, _ := NewClient(ClientOptions{Address: server.URL, HTTPClient: server.Client()})
	registrar = NewRegistrar(client)
	if err := registrar.Register(context.Background(), registration); err == nil {
		t.Fatal("Register() succeeded with zero health durations")
	}
}

type countingResolver struct {
	calls     atomic.Int32
	instances []microservice.ServiceInstance
}

func (r *countingResolver) Resolve(context.Context, string) ([]microservice.ServiceInstance, error) {
	r.calls.Add(1)
	return r.instances, nil
}
