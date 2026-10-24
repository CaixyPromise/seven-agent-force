package microservice

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"go.opentelemetry.io/otel/baggage"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/embedded"
)

func TestHTTPServiceClientPropagatesOneTraceAcrossRetriesWithoutBaggage(t *testing.T) {
	const traceIDText = "33333333333333333333333333333333"
	traceID, _ := oteltrace.TraceIDFromHex(traceIDText)
	spanID, _ := oteltrace.SpanIDFromHex("3333333333333333")
	traceState, _ := oteltrace.ParseTraceState("vendor=value")
	ctx := oteltrace.ContextWithSpanContext(context.Background(), oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID: traceID, SpanID: spanID, TraceState: traceState, TraceFlags: oteltrace.FlagsSampled,
	}))
	ctx = xcontext.WithTraceID(ctx, traceIDText)
	member, _ := baggage.NewMember("private", "must-not-propagate")
	bag, _ := baggage.New(member)
	ctx = baggage.ContextWithBaggage(ctx, bag)

	type captured struct{ traceparent, tracestate, traceID, baggage string }
	requests := make(chan captured, 2)
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- captured{r.Header.Get("traceparent"), r.Header.Get("tracestate"), r.Header.Get(xcontext.TraceIDHeader), r.Header.Get("baggage")}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- captured{r.Header.Get("traceparent"), r.Header.Get("tracestate"), r.Header.Get(xcontext.TraceIDHeader), r.Header.Get("baggage")}
		_, _ = w.Write([]byte("ok"))
	}))
	defer second.Close()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	client := newTestServiceClientWithOptions(t, []string{first.URL, second.URL}, HTTPClientOptions{
		RequestTimeout: time.Second, Tracer: provider.Tracer("test"),
	})

	response, err := client.Do(ctx, ServiceRequest{ServiceName: "node-a", Method: http.MethodGet, Path: "/internal/node/v1/descriptor", TracePropagation: true})
	if err != nil {
		t.Fatal(err)
	}
	firstRequest, secondRequest := <-requests, <-requests
	if firstRequest != secondRequest {
		t.Fatalf("retry changed trace headers: first=%+v second=%+v", firstRequest, secondRequest)
	}
	if firstRequest.traceID != traceIDText || firstRequest.traceparent == "" || firstRequest.tracestate != "vendor=value" || firstRequest.baggage != "" {
		t.Fatalf("propagated headers=%+v", firstRequest)
	}
	if response.InstanceID == "" {
		t.Fatalf("selected instance missing from response: %+v", response)
	}
}

func TestHTTPServiceClientKeepsContextOnlyTraceWhenCreatingClientSpan(t *testing.T) {
	const traceID = "cccccccccccccccccccccccccccccccc"
	var gotTraceparent, gotTraceID string
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		gotTraceparent = request.Header.Get("traceparent")
		gotTraceID = request.Header.Get(xcontext.TraceIDHeader)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody}, nil
	})}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	instance := ServiceInstance{ID: "node-a-1", ServiceName: "node-a", Host: "node.invalid", Port: 443, Scheme: "https", Healthy: true}
	client := NewHTTPServiceClient(nil, NewRoundRobin(), HTTPClientOptions{
		HTTPClient: httpClient, RequestTimeout: time.Second, Tracer: provider.Tracer("test"),
	})
	ctx := xcontext.WithTraceID(context.Background(), traceID)

	if _, err := client.Do(ctx, ServiceRequest{ServiceName: "node-a", ResolvedInstances: []ServiceInstance{instance}, Method: http.MethodGet, Path: "/internal/node/v1/descriptor", TracePropagation: true}); err != nil {
		t.Fatal(err)
	}
	if gotTraceID != traceID || !strings.Contains(gotTraceparent, "-"+traceID+"-") {
		t.Fatalf("traceparent=%q X-Trace-Id=%q", gotTraceparent, gotTraceID)
	}
}

func TestHTTPServiceClientKeepsExternalRequestsFreeOfTraceAndBaggage(t *testing.T) {
	const traceIDText = "dededededededededededededededede"
	traceID, _ := oteltrace.TraceIDFromHex(traceIDText)
	spanID, _ := oteltrace.SpanIDFromHex("dededededededede")
	ctx := oteltrace.ContextWithSpanContext(context.Background(), oteltrace.NewSpanContext(oteltrace.SpanContextConfig{TraceID: traceID, SpanID: spanID}))
	ctx = xcontext.WithTraceID(ctx, traceIDText)
	member, _ := baggage.NewMember("private", "must-not-propagate")
	bag, _ := baggage.New(member)
	ctx = baggage.ContextWithBaggage(ctx, bag)

	var headers http.Header
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		headers = request.Header.Clone()
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody}, nil
	})}
	instance := ServiceInstance{ID: "external-oidc", ServiceName: "external-oidc", Host: "issuer.example", Port: 443, Scheme: "https", Healthy: true}
	client := NewHTTPServiceClient(nil, NewRoundRobin(), HTTPClientOptions{HTTPClient: httpClient, RequestTimeout: time.Second})

	if _, err := client.Do(ctx, ServiceRequest{ServiceName: "external-oidc", ResolvedInstances: []ServiceInstance{instance}, Method: http.MethodGet, Path: "/userinfo"}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"traceparent", "tracestate", xcontext.TraceIDHeader, "baggage"} {
		if value := headers.Get(key); value != "" {
			t.Fatalf("external request leaked %s=%q", key, value)
		}
	}
}

func TestHTTPServiceClientFailsOpenWhenClientTraceStartPanics(t *testing.T) {
	const traceID = "fefefefefefefefefefefefefefefefe"
	var received http.Header
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		received = request.Header.Clone()
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody}, nil
	})}
	instance := ServiceInstance{ID: "node-a-1", ServiceName: "node-a", Host: "node.invalid", Port: 443, Scheme: "https", Healthy: true}
	client := NewHTTPServiceClient(nil, NewRoundRobin(), HTTPClientOptions{
		HTTPClient: httpClient, RequestTimeout: time.Second, Tracer: panicStartTracer{},
	})
	var warnings []string
	response, err := client.Do(xcontext.WithTraceID(context.Background(), traceID), ServiceRequest{
		ServiceName:       "node-a",
		ResolvedInstances: []ServiceInstance{instance},
		Method:            http.MethodGet,
		Path:              "/internal/node/v1/descriptor",
		Header:            http.Header{xcontext.TraceIDHeader: []string{traceID}},
		TracePropagation:  true,
		TraceWarning: func(operation string) {
			warnings = append(warnings, operation)
			panic("trace warning must not interrupt business")
		},
	})
	if err != nil || response == nil || response.StatusCode != http.StatusOK {
		t.Fatalf("business request response=%#v err=%v", response, err)
	}
	if received.Get(xcontext.TraceIDHeader) != traceID || received.Get("traceparent") != "" {
		t.Fatalf("fallback headers=%#v", received)
	}
	if len(warnings) != 1 || warnings[0] != "client_span_start" {
		t.Fatalf("trace warnings=%#v", warnings)
	}
}

func TestHTTPServiceClientFailsOpenWhenClientTraceEndPanics(t *testing.T) {
	const traceID = "fafafafafafafafafafafafafafafafa"
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody}, nil
	})}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	instance := ServiceInstance{ID: "node-a-1", ServiceName: "node-a", Host: "node.invalid", Port: 443, Scheme: "https", Healthy: true}
	client := NewHTTPServiceClient(nil, NewRoundRobin(), HTTPClientOptions{
		HTTPClient: httpClient, RequestTimeout: time.Second, Tracer: panicEndTracer{base: provider.Tracer("test")},
	})
	var warnings []string
	response, err := client.Do(xcontext.WithTraceID(context.Background(), traceID), ServiceRequest{
		ServiceName:       "node-a",
		ResolvedInstances: []ServiceInstance{instance},
		Method:            http.MethodGet,
		Path:              "/internal/node/v1/descriptor",
		TracePropagation:  true,
		TraceWarning: func(operation string) {
			warnings = append(warnings, operation)
		},
	})
	if err != nil || response == nil || response.StatusCode != http.StatusOK {
		t.Fatalf("business request response=%#v err=%v", response, err)
	}
	if len(warnings) != 1 || warnings[0] != "client_span_end" {
		t.Fatalf("trace warnings=%#v", warnings)
	}
}

type panicStartTracer struct{ embedded.Tracer }

func (panicStartTracer) Start(context.Context, string, ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	panic("trace start must not interrupt business")
}

type panicEndTracer struct {
	embedded.Tracer
	base oteltrace.Tracer
}

func (t panicEndTracer) Start(ctx context.Context, name string, options ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	next, span := t.base.Start(ctx, name, options...)
	return next, panicEndSpan{Span: span}
}

type panicEndSpan struct{ oteltrace.Span }

func (panicEndSpan) End(...oteltrace.SpanEndOption) {
	panic("trace end must not interrupt business")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestCachedResolverHonorsConfiguredResolveTimeout(t *testing.T) {
	resolver := NewCachedResolverWithOptions(timeoutTestResolver{}, CachedResolverOptions{
		TTL: time.Minute, EmptyTTL: time.Second, ResolveTimeout: 37 * time.Millisecond,
	})
	if resolver.refreshTimeout != 37*time.Millisecond {
		t.Fatalf("refresh timeout=%s want 37ms", resolver.refreshTimeout)
	}
}

type timeoutTestResolver struct{}

func (timeoutTestResolver) Resolve(context.Context, string) ([]ServiceInstance, error) {
	return nil, ErrNoHealthyInstance
}

type resolverFunc func(context.Context, string) ([]ServiceInstance, error)

func (f resolverFunc) Resolve(ctx context.Context, serviceName string) ([]ServiceInstance, error) {
	return f(ctx, serviceName)
}

type panicResolver struct{}

func (*panicResolver) Resolve(context.Context, string) ([]ServiceInstance, error) {
	panic("typed-nil resolver invoked")
}

func TestInstanceCacheCoalescesConcurrentRefreshes(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	resolver := resolverFunc(func(context.Context, string) ([]ServiceInstance, error) {
		calls.Add(1)
		<-release
		return []ServiceInstance{{ID: "a", ServiceName: "hub", Host: "127.0.0.1", Port: 80, Scheme: "http", Healthy: true}}, nil
	})
	cache := NewCachedResolver(resolver, time.Minute, time.Second)

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			instances, err := cache.Resolve(context.Background(), "hub")
			if err != nil || len(instances) != 1 {
				t.Errorf("Resolve() = %v, %v", instances, err)
			}
		}()
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
}

func TestInstanceCacheShortCachesAuthoritativeEmptyResult(t *testing.T) {
	var calls atomic.Int32
	cache := NewCachedResolver(resolverFunc(func(context.Context, string) ([]ServiceInstance, error) {
		calls.Add(1)
		return nil, ErrNoHealthyInstance
	}), time.Minute, time.Minute)

	for range 2 {
		_, err := cache.Resolve(context.Background(), "hub")
		if !errors.Is(err, ErrNoHealthyInstance) {
			t.Fatalf("Resolve() error = %v", err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
}

func TestInstanceCacheReturnsIsolatedSnapshots(t *testing.T) {
	cache := NewCachedResolver(resolverFunc(func(context.Context, string) ([]ServiceInstance, error) {
		return []ServiceInstance{{
			ID: "a", Healthy: true, Tags: []string{"v1"}, Metadata: map[string]string{"protocol": "https"},
		}}, nil
	}), time.Minute, time.Second)

	first, err := cache.Resolve(context.Background(), "hub")
	if err != nil {
		t.Fatal(err)
	}
	first[0].Tags[0] = "corrupted"
	first[0].Metadata["protocol"] = "http"
	second, err := cache.Resolve(context.Background(), "hub")
	if err != nil {
		t.Fatal(err)
	}
	if second[0].Tags[0] != "v1" || second[0].Metadata["protocol"] != "https" {
		t.Fatalf("cached snapshot was mutated: %#v", second[0])
	}
}

func TestInstanceCacheWaiterCancellationDoesNotPoisonSharedRefresh(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	resolver := resolverFunc(func(ctx context.Context, _ string) ([]ServiceInstance, error) {
		close(started)
		select {
		case <-release:
			return []ServiceInstance{{ID: "a", Healthy: true}}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	cache := NewCachedResolver(resolver, time.Minute, time.Second)
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstErr := make(chan error, 1)
	go func() {
		_, err := cache.Resolve(firstCtx, "hub")
		firstErr <- err
	}()
	<-started
	secondResult := make(chan error, 1)
	go func() {
		instances, err := cache.Resolve(context.Background(), "hub")
		if err == nil && len(instances) != 1 {
			err = errors.New("second waiter received no instance")
		}
		secondResult <- err
	}()
	time.Sleep(10 * time.Millisecond)
	cancelFirst()
	if err := <-firstErr; !errors.Is(err, context.Canceled) {
		t.Fatalf("first Resolve() error = %v, want context.Canceled", err)
	}
	close(release)
	if err := <-secondResult; err != nil {
		t.Fatalf("second Resolve() error = %v", err)
	}
}

func TestInstanceCacheRefreshHasOwnedDeadline(t *testing.T) {
	cache := NewCachedResolver(resolverFunc(func(ctx context.Context, _ string) ([]ServiceInstance, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}), time.Minute, time.Second)
	cache.refreshTimeout = 10 * time.Millisecond

	started := time.Now()
	_, err := cache.Resolve(context.Background(), "hub")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Resolve() error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Resolve() took %s", elapsed)
	}
}

func TestInstanceCacheInvalidateWinsAgainstInFlightRefresh(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	cache := NewCachedResolver(resolverFunc(func(context.Context, string) ([]ServiceInstance, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
			return []ServiceInstance{{ID: "old", Healthy: true}}, nil
		}
		return []ServiceInstance{{ID: "new", Healthy: true}}, nil
	}), time.Minute, time.Second)

	firstDone := make(chan error, 1)
	go func() {
		_, err := cache.Resolve(context.Background(), "hub")
		firstDone <- err
	}()
	<-started
	cache.Invalidate("hub")
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	instances, err := cache.Resolve(context.Background(), "hub")
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || len(instances) != 1 || instances[0].ID != "new" {
		t.Fatalf("Resolve() = %#v, calls=%d", instances, calls.Load())
	}
}

func TestInstanceCacheNormalizesDurationsAndRejectsNilResolver(t *testing.T) {
	cache := NewCachedResolver(nil, 0, -time.Second)
	if cache.ttl <= 0 || cache.emptyTTL <= 0 || cache.refreshTimeout <= 0 {
		t.Fatalf("unsafe cache durations: ttl=%s emptyTTL=%s refreshTimeout=%s", cache.ttl, cache.emptyTTL, cache.refreshTimeout)
	}
	if _, err := cache.Resolve(context.Background(), "hub"); !errors.Is(err, ErrInvalidDependency) {
		t.Fatalf("Resolve() error = %v, want ErrInvalidDependency", err)
	}
	var nilCache *CachedResolver
	if _, err := nilCache.Resolve(context.Background(), "hub"); !errors.Is(err, ErrInvalidDependency) {
		t.Fatalf("nil Resolve() error = %v, want ErrInvalidDependency", err)
	}
	nilCache.Invalidate("hub")
}

func TestRoundRobinSelectsHealthyInstancesAndHonorsExclusions(t *testing.T) {
	lb := NewRoundRobin()
	instances := []ServiceInstance{
		{ID: "bad", Healthy: false},
		{ID: "a", Healthy: true},
		{ID: "b", Healthy: true},
	}

	first, err := lb.Select("hub", instances, nil)
	if err != nil || first.ID != "a" {
		t.Fatalf("first Select() = %q, %v", first.ID, err)
	}
	second, err := lb.Select("hub", instances, nil)
	if err != nil || second.ID != "b" {
		t.Fatalf("second Select() = %q, %v", second.ID, err)
	}
	selected, err := lb.Select("hub", instances, map[string]struct{}{first.ID: {}})
	if err != nil || selected.ID != "b" {
		t.Fatalf("excluded Select() = %q, %v", selected.ID, err)
	}
}

func TestNilRoundRobinReturnsDependencyError(t *testing.T) {
	var balancer *RoundRobin
	if _, err := balancer.Select("hub", nil, nil); !errors.Is(err, ErrInvalidDependency) {
		t.Fatalf("Select() error = %v, want ErrInvalidDependency", err)
	}
}

func TestHTTPServiceClientRetriesGETOnceOnAlternateInstance(t *testing.T) {
	var firstCalls atomic.Int32
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		firstCalls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("first unavailable"))
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Upstream", "second")
		_, _ = w.Write([]byte("ok"))
	}))
	defer second.Close()

	client := newTestServiceClient(t, first.URL, second.URL)
	response, err := client.Do(context.Background(), ServiceRequest{ServiceName: "hub", Method: http.MethodGet, Path: "/resource"})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || string(response.Body) != "ok" || response.Header.Get("X-Upstream") != "second" {
		t.Fatalf("Do() response = %#v", response)
	}
	if got := firstCalls.Load(); got != 1 {
		t.Fatalf("first calls = %d", got)
	}
}

func TestHTTPServiceClientDoesNotMutateInjectedHTTPClient(t *testing.T) {
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
			client := NewHTTPServiceClient(nil, nil, HTTPClientOptions{HTTPClient: injected, RequestTimeout: 2 * time.Second})
			if client.client == injected {
				t.Error("service client retained caller-owned http.Client")
			}
			if client.client.Timeout != 2*time.Second {
				t.Errorf("private timeout = %s, want 2s", client.client.Timeout)
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
}

func TestHTTPServiceClientRetriesDuplicateIDOnDistinctEndpoint(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("second"))
	}))
	defer second.Close()

	instances := make([]ServiceInstance, 0, 2)
	for _, rawURL := range []string{first.URL, second.URL} {
		instance, err := ParseServiceURL("hub", rawURL)
		if err != nil {
			t.Fatal(err)
		}
		instance.ID = "shared-consul-service-id"
		instances = append(instances, instance)
	}
	client := NewHTTPServiceClient(resolverFunc(func(context.Context, string) ([]ServiceInstance, error) {
		return instances, nil
	}), NewRoundRobin(), HTTPClientOptions{RequestTimeout: time.Second})

	response, err := client.Do(context.Background(), ServiceRequest{ServiceName: "hub", Method: http.MethodGet, Path: "/resource"})
	if err != nil || string(response.Body) != "second" {
		t.Fatalf("Do() = %#v, %v", response, err)
	}
}

func TestHTTPServiceClientReturnsErrorForNilDependencies(t *testing.T) {
	client := NewHTTPServiceClient(nil, nil, HTTPClientOptions{})
	_, err := client.Do(context.Background(), ServiceRequest{ServiceName: "hub", Method: http.MethodGet, Path: "/resource"})
	if !errors.Is(err, ErrInvalidDependency) {
		t.Fatalf("Do() error = %v, want ErrInvalidDependency", err)
	}
	var nilClient *HTTPServiceClient
	if _, err := nilClient.Do(context.Background(), ServiceRequest{}); !errors.Is(err, ErrInvalidDependency) {
		t.Fatalf("nil Do() error = %v, want ErrInvalidDependency", err)
	}
}

func TestRootComponentsRejectTypedNilDependencies(t *testing.T) {
	var resolver *panicResolver
	client := NewHTTPServiceClient(resolver, NewRoundRobin(), HTTPClientOptions{})
	if _, err := client.Do(context.Background(), ServiceRequest{ServiceName: "hub", Method: http.MethodGet, Path: "/"}); !errors.Is(err, ErrInvalidDependency) {
		t.Fatalf("Do() error = %v, want ErrInvalidDependency", err)
	}
	cache := NewCachedResolver(resolver, time.Second, time.Second)
	if _, err := cache.Resolve(context.Background(), "hub"); !errors.Is(err, ErrInvalidDependency) {
		t.Fatalf("cache Resolve() error = %v, want ErrInvalidDependency", err)
	}
}

func TestHTTPServiceClientDoesNotRetryUnsafeWrite(t *testing.T) {
	var secondCalls atomic.Int32
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { secondCalls.Add(1) }))
	defer second.Close()

	client := newTestServiceClient(t, first.URL, second.URL)
	response, err := client.Do(context.Background(), ServiceRequest{ServiceName: "hub", Method: http.MethodPost, Path: "/command", Body: []byte("payload")})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadGateway || secondCalls.Load() != 0 {
		t.Fatalf("unsafe write retried: response=%#v secondCalls=%d", response, secondCalls.Load())
	}
}

func TestHTTPServiceClientPreservesRetryableResponseWhenNoAlternateExists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("temporarily unavailable"))
	}))
	defer server.Close()

	client := newTestServiceClient(t, server.URL)
	response, err := client.Do(context.Background(), ServiceRequest{ServiceName: "hub", Method: http.MethodGet, Path: "/resource"})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusServiceUnavailable || string(response.Body) != "temporarily unavailable" {
		t.Fatalf("Do() response = %#v", response)
	}
}

func TestHTTPServiceClientDoesNotRetryNonFailoverStatus(t *testing.T) {
	var secondCalls atomic.Int32
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { secondCalls.Add(1) }))
	defer second.Close()

	client := newTestServiceClient(t, first.URL, second.URL)
	response, err := client.Do(context.Background(), ServiceRequest{ServiceName: "hub", Method: http.MethodGet, Path: "/resource"})
	if err != nil || response.StatusCode != http.StatusUnauthorized || secondCalls.Load() != 0 {
		t.Fatalf("Do() = %#v, %v, second calls = %d", response, err, secondCalls.Load())
	}
}

func TestHTTPServiceClientRejectsOversizedResponseWithoutRetry(t *testing.T) {
	var secondCalls atomic.Int32
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("too large")) }))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { secondCalls.Add(1) }))
	defer second.Close()

	client := newTestServiceClient(t, first.URL, second.URL)
	_, err := client.Do(context.Background(), ServiceRequest{ServiceName: "hub", Method: http.MethodGet, Path: "/resource", MaxResponseBytes: 3})
	if !errors.Is(err, ErrResponseTooLarge) || secondCalls.Load() != 0 {
		t.Fatalf("Do() error = %v, second calls = %d", err, secondCalls.Load())
	}
}

func TestHTTPServiceClientDefaultsToBoundedRequestBody(t *testing.T) {
	client := NewHTTPServiceClient(resolverFunc(func(context.Context, string) ([]ServiceInstance, error) {
		t.Fatal("resolver must not be called for an oversized request")
		return nil, nil
	}), NewRoundRobin(), HTTPClientOptions{})

	_, err := client.Do(context.Background(), ServiceRequest{
		ServiceName: "hub",
		Method:      http.MethodPost,
		Path:        "/command",
		Body:        make([]byte, defaultMaxRequestBytes+1),
	})
	if !errors.Is(err, ErrRequestTooLarge) {
		t.Fatalf("Do() error = %v, want ErrRequestTooLarge", err)
	}
}

func TestHTTPServiceClientRetriesExplicitReplaySafeWriteWithBody(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusGatewayTimeout)
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_, _ = w.Write(body)
	}))
	defer second.Close()

	client := newTestServiceClient(t, first.URL, second.URL)
	response, err := client.Do(context.Background(), ServiceRequest{
		ServiceName: "hub", Method: http.MethodPost, Path: "/command", Body: []byte("payload"), ReplaySafe: true,
	})
	if err != nil || string(response.Body) != "payload" {
		t.Fatalf("Do() = %#v, %v", response, err)
	}
}

func TestHTTPServiceClientRejectsEscapingTargetsAndRedirects(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, target.URL, http.StatusFound)
	}))
	defer redirect.Close()
	client := newTestServiceClient(t, redirect.URL)

	for _, path := range []string{"https://attacker.invalid/x", "//attacker.invalid/x", "resource"} {
		if _, err := client.Do(context.Background(), ServiceRequest{ServiceName: "hub", Method: http.MethodGet, Path: path}); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("path %q error = %v", path, err)
		}
	}
	response, err := client.Do(context.Background(), ServiceRequest{ServiceName: "hub", Method: http.MethodGet, Path: "/redirect"})
	if err != nil || response.StatusCode != http.StatusFound {
		t.Fatalf("redirect response = %#v, %v", response, err)
	}
}

func newTestServiceClient(t *testing.T, rawURLs ...string) ServiceClient {
	t.Helper()
	instances := make([]ServiceInstance, 0, len(rawURLs))
	for i, rawURL := range rawURLs {
		instance, err := ParseServiceURL("hub", rawURL)
		if err != nil {
			t.Fatal(err)
		}
		instance.ID = string(rune('a' + i))
		instances = append(instances, instance)
	}
	return NewHTTPServiceClient(resolverFunc(func(context.Context, string) ([]ServiceInstance, error) {
		return instances, nil
	}), NewRoundRobin(), HTTPClientOptions{RequestTimeout: time.Second, MaxResponseBytes: 1 << 20})
}

func newTestServiceClientWithOptions(t *testing.T, rawURLs []string, options HTTPClientOptions) ServiceClient {
	t.Helper()
	instances := make([]ServiceInstance, 0, len(rawURLs))
	for i, rawURL := range rawURLs {
		instance, err := ParseServiceURL("hub", rawURL)
		if err != nil {
			t.Fatal(err)
		}
		instance.ID = string(rune('a' + i))
		instances = append(instances, instance)
	}
	if options.MaxResponseBytes == 0 {
		options.MaxResponseBytes = 1 << 20
	}
	return NewHTTPServiceClient(resolverFunc(func(context.Context, string) ([]ServiceInstance, error) {
		return instances, nil
	}), NewRoundRobin(), options)
}
