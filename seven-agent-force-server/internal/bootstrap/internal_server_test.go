package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	protocolhttp "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/protocol/http"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/network/standard"
	"github.com/cloudwego/hertz/pkg/route"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestInternalServerUsesSharedStandardMiddlewareChain(t *testing.T) {
	const traceID = "ffffffffffffffffffffffffffffffff"
	listener := listenLoopback(t)
	core, logs := observer.New(zap.DebugLevel)
	cfg := config.Config{Logging: config.LoggingConfig{Request: config.RequestLoggingConfig{Enabled: true}}}
	var observabilityCalls atomic.Int32
	middlewares := protocolhttp.StandardMiddlewareChain(cfg, zap.New(core), func(ctx context.Context, reqCtx *app.RequestContext) {
		observabilityCalls.Add(1)
		reqCtx.Next(ctx)
	})
	internal, err := newInternalServerWithMiddleware(listener, middlewares, internalRouteMounterFunc(func(router route.IRouter) {
		router.GET("/internal/node/v1/descriptor", func(ctx context.Context, reqCtx *app.RequestContext) {
			if got := xcontext.TraceIDFromContext(ctx); got != traceID {
				t.Errorf("Go context trace=%q, want %q", got, traceID)
			}
			reqCtx.Status(http.StatusNoContent)
		})
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := internal.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = internal.Shutdown(context.Background()) })
	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/internal/node/v1/descriptor", listener.Addr()), nil)
	request.Header.Set(xcontext.TraceIDHeader, traceID)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent || response.Header.Get(xcontext.TraceIDHeader) != traceID {
		t.Fatalf("status/trace=%d/%q", response.StatusCode, response.Header.Get(xcontext.TraceIDHeader))
	}
	if logs.FilterMessage("internal_request_started").Len() != 1 || logs.FilterMessage("internal_request_finished").Len() != 1 {
		t.Fatalf("internal request logs missing: %#v", logs.All())
	}
	if observabilityCalls.Load() != 1 {
		t.Fatalf("shared observability middleware calls=%d, want 1", observabilityCalls.Load())
	}
}

func TestInternalServerIsolatesNodeRoutesAndShutsDown(t *testing.T) {
	primaryListener := listenLoopback(t)
	primary := server.New(
		server.WithListener(primaryListener),
		server.WithTransport(standard.NewTransporter),
	)
	primary.GET("/api/healthz", func(ctx context.Context, reqCtx *app.RequestContext) {
		reqCtx.Status(http.StatusNoContent)
	})
	startHertz(t, primary)

	internalListener := listenLoopback(t)
	internal, err := newInternalServer(internalListener, internalRouteMounterFunc(func(router route.IRouter) {
		router.GET("/internal/node/v1/descriptor", func(ctx context.Context, reqCtx *app.RequestContext) {
			reqCtx.Status(http.StatusNoContent)
		})
	}))
	if err != nil {
		t.Fatalf("create internal server: %v", err)
	}
	if err := internal.Start(); err != nil {
		t.Fatalf("start internal server: %v", err)
	}

	internalURL := fmt.Sprintf("http://%s", internalListener.Addr())
	primaryURL := fmt.Sprintf("http://%s", primaryListener.Addr())
	waitForHTTPStatus(t, internalURL+"/internal/node/v1/descriptor", http.StatusNoContent)
	waitForHTTPStatus(t, primaryURL+"/api/healthz", http.StatusNoContent)
	assertHTTPStatus(t, primaryURL+"/internal/node/v1/descriptor", http.StatusNotFound)
	assertHTTPStatus(t, internalURL+"/api/healthz", http.StatusNotFound)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := internal.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown internal server: %v", err)
	}
	waitForConnectionRefused(t, internalListener.Addr().String())
}

func TestInternalServerReturnsStartFailureForClosedListener(t *testing.T) {
	listener := listenLoopback(t)
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	internal, err := newInternalServer(listener, internalRouteMounterFunc(func(route.IRouter) {}))
	if err != nil {
		t.Fatalf("create internal server: %v", err)
	}
	if err := internal.Start(); err == nil {
		t.Fatal("starting with a closed listener must fail readiness")
	}
}

func TestInternalServerRejectsEmptyMounterSet(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mounters []bootstrapruntime.InternalRouteMounter
	}{
		{name: "empty"},
		{name: "nil", mounters: []bootstrapruntime.InternalRouteMounter{internalRouteMounterFunc(nil)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listener := listenLoopback(t)
			internal, err := newInternalServer(listener, tc.mounters...)
			if err == nil || internal != nil {
				t.Fatalf("empty internal mounter set must be rejected: internal=%v err=%v", internal, err)
			}
		})
	}
}

func TestInternalServerStartTimeoutClosesListenerBeforeLateRunCanOutliveIt(t *testing.T) {
	listener := listenLoopback(t)
	address := listener.Addr().String()
	lateRun := make(chan struct{})
	internal := &internalServer{
		listener:     listener,
		startTimeout: 20 * time.Millisecond,
		run: func() error {
			<-lateRun
			return nil
		},
		isRunning: func() bool { return false },
	}

	if err := internal.Start(); err == nil {
		t.Fatal("start timeout must be returned when the internal server never becomes ready")
	}
	waitForConnectionRefused(t, address)
	close(lateRun)
}

func TestInternalServerCompletionReportsPostReadinessFailure(t *testing.T) {
	listener := listenLoopback(t)
	release := make(chan struct{})
	internal := &internalServer{
		listener:     listener,
		startTimeout: time.Second,
		run: func() error {
			<-release
			return errors.New("node listener exited")
		},
		isRunning: func() bool { return true },
	}
	if err := internal.Start(); err != nil {
		t.Fatalf("start internal server: %v", err)
	}
	close(release)
	select {
	case err := <-internal.Completion():
		if err == nil || err.Error() != "node listener exited" {
			t.Fatalf("completion error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("post-readiness internal completion was not reported")
	}
}

func TestInternalServerShutdownClosesUnstartedListenerIdempotently(t *testing.T) {
	listener := listenLoopback(t)
	internal, err := newInternalServer(listener, internalRouteMounterFunc(func(route.IRouter) {}))
	if err != nil {
		t.Fatalf("create internal server: %v", err)
	}

	var wait sync.WaitGroup
	errs := make(chan error, 4)
	for range 4 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errs <- internal.Shutdown(context.Background())
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent shutdown: %v", err)
		}
	}
	waitForConnectionRefused(t, listener.Addr().String())
}

type internalRouteMounterFunc func(route.IRouter)

func (f internalRouteMounterFunc) MountInternal(router route.IRouter) {
	f(router)
}

var _ bootstrapruntime.InternalRouteMounter = internalRouteMounterFunc(nil)

func listenLoopback(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func startHertz(t *testing.T, instance *server.Hertz) {
	t.Helper()
	errs := make(chan error, 1)
	go func() { errs <- instance.Run() }()
	t.Cleanup(func() {
		if instance.IsRunning() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = instance.Shutdown(shutdownCtx)
		}
		select {
		case err := <-errs:
			if err != nil {
				t.Errorf("primary server exited: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("primary server did not stop")
		}
	})
}

func waitForHTTPStatus(t *testing.T, url string, want int) {
	t.Helper()
	client := &http.Client{
		Timeout: 100 * time.Millisecond,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(url)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == want {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s did not return status %d", url, want)
}

func assertHTTPStatus(t *testing.T, url string, want int) {
	t.Helper()
	response, err := (&http.Client{
		Timeout: time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}).Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer response.Body.Close()
	if response.StatusCode != want {
		t.Fatalf("GET %s status=%d want=%d", url, response.StatusCode, want)
	}
}

func waitForConnectionRefused(t *testing.T, address string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err != nil {
			return
		}
		_ = connection.Close()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("internal listener %s remained reachable after shutdown", address)
}
