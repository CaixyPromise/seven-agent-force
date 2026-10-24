//go:build integration

package consul

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/microservice"
)

func TestConsulRegistrationHealthCacheAndDeregistration(t *testing.T) {
	consulAddress := os.Getenv("CONSUL_HTTP_ADDR")
	if consulAddress == "" {
		t.Skip("CONSUL_HTTP_ADDR is required for the integration smoke")
	}
	listener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	var healthy atomic.Bool
	healthy.Store(true)
	healthServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !healthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})}
	go func() { _ = healthServer.Serve(listener) }()
	t.Cleanup(func() { _ = healthServer.Shutdown(context.Background()) })

	client, err := NewClient(ClientOptions{Address: consulAddress, Token: os.Getenv("CONSUL_HTTP_TOKEN"), Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	registrar := NewRegistrar(client)
	resolver := NewResolver(client, ResolverOptions{})
	instanceID := fmt.Sprintf("seven-runtime-smoke-%d", time.Now().UnixNano())
	advertisedHost := os.Getenv("CONSUL_TEST_ADVERTISED_HOST")
	if advertisedHost == "" {
		advertisedHost = "host.docker.internal"
	}
	port := listener.Addr().(*net.TCPAddr).Port
	registration := microservice.ServiceRegistration{
		ID: instanceID, ServiceName: instanceID, Address: advertisedHost, Port: port, Scheme: "http",
		HealthCheckPath: "/internal/healthz", HealthInterval: time.Second, HealthTimeout: 500 * time.Millisecond,
		Metadata: map[string]string{"protocol": "http", "smoke": strconv.FormatInt(time.Now().Unix(), 10)},
	}
	if err := registrar.Register(context.Background(), registration); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = registrar.Deregister(ctx, instanceID)
	})

	waitForResolution(t, 15*time.Second, func() error {
		instances, resolveErr := resolver.Resolve(context.Background(), instanceID)
		if resolveErr != nil {
			return resolveErr
		}
		if len(instances) != 1 || instances[0].ID != instanceID {
			return fmt.Errorf("unexpected instances: %#v", instances)
		}
		return nil
	})

	cached := microservice.NewCachedResolver(resolver, 250*time.Millisecond, 100*time.Millisecond)
	if _, err := cached.Resolve(context.Background(), instanceID); err != nil {
		t.Fatal(err)
	}
	healthy.Store(false)
	if _, err := cached.Resolve(context.Background(), instanceID); err != nil {
		t.Fatalf("unexpired snapshot should remain usable: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	waitForResolution(t, 15*time.Second, func() error {
		_, resolveErr := cached.Resolve(context.Background(), instanceID)
		if errors.Is(resolveErr, microservice.ErrNoHealthyInstance) {
			return nil
		}
		return fmt.Errorf("want authoritative empty result, got %v", resolveErr)
	})

	healthy.Store(true)
	waitForResolution(t, 15*time.Second, func() error {
		_, resolveErr := cached.Resolve(context.Background(), instanceID)
		return resolveErr
	})
	if err := registrar.Deregister(context.Background(), instanceID); err != nil {
		t.Fatal(err)
	}
	cached.Invalidate(instanceID)
	waitForResolution(t, 10*time.Second, func() error {
		_, resolveErr := cached.Resolve(context.Background(), instanceID)
		if errors.Is(resolveErr, microservice.ErrNoHealthyInstance) {
			return nil
		}
		return fmt.Errorf("want empty result after deregistration, got %v", resolveErr)
	})
}

func waitForResolution(t *testing.T, timeout time.Duration, check func() error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if lastErr = check(); lastErr == nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s: %v", timeout, lastErr)
}
