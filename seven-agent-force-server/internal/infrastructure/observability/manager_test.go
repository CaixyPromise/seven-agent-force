package observability

import (
	"context"
	"testing"

	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

func TestManagerAttachesPrometheusAndPprof(t *testing.T) {
	manager, err := New(config.ObservabilityConfig{
		Enabled: true,
		Prometheus: config.ObservabilityPrometheusConfig{
			Enabled:     true,
			Path:        "/ops/prometheus",
			AccessToken: "test-ops-token",
		},
		Tracing: config.ObservabilityTracingConfig{
			Enabled:     true,
			ServiceName: "test-service",
		},
		Pprof: config.ObservabilityPprofConfig{
			Enabled: true,
			Prefix:  "/ops/debug/pprof",
		},
	}, zap.NewNop(), nil, cacheinfra.NewManager("test", nil))
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close(context.Background()) })
	engine := server.New()
	for _, middleware := range manager.Middlewares() {
		engine.Use(middleware)
	}
	manager.Attach(engine)

	resp := ut.PerformRequest(engine.Engine, "GET", "/ops/prometheus", nil)
	if resp.Code != 401 {
		t.Fatalf("expected unauthenticated prometheus status 401, got %d", resp.Code)
	}
	resp = ut.PerformRequest(engine.Engine, "GET", "/ops/prometheus", nil, ut.Header{Key: "Authorization", Value: "Bearer test-ops-token"})
	if resp.Code != 200 {
		t.Fatalf("unexpected authenticated prometheus status: %d", resp.Code)
	}
	pprofResp := ut.PerformRequest(engine.Engine, "GET", "/ops/debug/pprof/", nil)
	if pprofResp.Code != 200 {
		t.Fatalf("unexpected pprof status: %d", pprofResp.Code)
	}
}

func TestManagerKeepsNonRecordingTraceCorrelationWithoutExporter(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.ObservabilityConfig
	}{
		{name: "observability disabled"},
		{name: "tracing disabled", cfg: config.ObservabilityConfig{Enabled: true}},
		{name: "otlp endpoint absent", cfg: config.ObservabilityConfig{Enabled: true, Tracing: config.ObservabilityTracingConfig{Enabled: true}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manager, err := New(tc.cfg, zap.NewNop(), nil, nil)
			if err != nil {
				t.Fatalf("new manager: %v", err)
			}
			if err := manager.Start(context.Background()); err != nil {
				t.Fatalf("start manager: %v", err)
			}
			t.Cleanup(func() { _ = manager.Close(context.Background()) })
			engine := server.New()
			for _, middleware := range manager.Middlewares() {
				engine.Use(middleware)
			}
			engine.GET("/trace", func(ctx context.Context, c *app.RequestContext) {
				spanContext := oteltrace.SpanContextFromContext(ctx)
				if !spanContext.IsValid() || oteltrace.SpanFromContext(ctx).IsRecording() {
					t.Errorf("span context = %#v, want valid non-recording", spanContext)
				}
				if xcontext.TraceIDFromContext(ctx) != spanContext.TraceID().String() {
					t.Errorf("context trace = %q, span trace = %s", xcontext.TraceIDFromContext(ctx), spanContext.TraceID())
				}
				c.String(200, "ok")
			})
			resp := ut.PerformRequest(engine.Engine, "GET", "/trace", nil)
			if got := resp.Header().Get(xcontext.TraceIDHeader); len(got) != 32 {
				t.Fatalf("response trace = %q", got)
			}
		})
	}
}
