package observability

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	jobruntime "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/job/runner"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/observability/metrics"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/observability/profiling"
	obsTracing "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/observability/tracing"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type HealthSnapshot struct {
	Enabled    bool `json:"enabled"`
	Prometheus struct {
		Enabled bool   `json:"enabled"`
		Path    string `json:"path"`
	} `json:"prometheus"`
	Tracing struct {
		Enabled        bool   `json:"enabled"`
		ServiceName    string `json:"serviceName"`
		OTLPConfigured bool   `json:"otlpConfigured"`
	} `json:"tracing"`
	Pprof struct {
		Enabled bool   `json:"enabled"`
		Prefix  string `json:"prefix"`
	} `json:"pprof"`
}

type RuntimeSnapshot = metrics.Snapshot

type Manager interface {
	jobruntime.Metrics
	Start(ctx context.Context) error
	Close(ctx context.Context) error
	Middlewares() []app.HandlerFunc
	Attach(server *server.Hertz)
	Health() HealthSnapshot
	Snapshot(ctx context.Context) RuntimeSnapshot
}

type manager struct {
	cfg           config.ObservabilityConfig
	logger        *zap.Logger
	recorder      *metrics.Recorder
	traceProvider *sdktrace.TracerProvider
	meterProvider *sdkmetric.MeterProvider
	promHandler   http.Handler
	tracer        oteltrace.Tracer
	propagator    propagation.TextMapPropagator
}

func New(cfg config.ObservabilityConfig, log *zap.Logger, datasourceDB *sql.DB, cacheMgr cacheinfra.Manager) (Manager, error) {
	mgr := &manager{
		cfg:        cfg,
		logger:     log,
		propagator: propagation.TraceContext{},
	}
	if cfg.Enabled {
		recorder := metrics.NewRecorder(datasourceDB, cacheMgr)
		mgr.recorder = recorder

		promExporter, err := otelprom.New(otelprom.WithRegisterer(recorder.Registry()))
		if err != nil {
			return nil, err
		}
		mgr.meterProvider = sdkmetric.NewMeterProvider(sdkmetric.WithReader(promExporter))
		mgr.promHandler = promhttp.HandlerFor(recorder.Registry(), promhttp.HandlerOpts{})
	}

	resourceAttrs := resource.NewSchemaless(attribute.String("service.name", cfg.Tracing.ServiceName))
	traceOptions := []sdktrace.TracerProviderOption{sdktrace.WithResource(resourceAttrs)}
	traceSampler := sdktrace.Sampler(sdktrace.NeverSample())
	if cfg.Enabled && cfg.Tracing.Enabled && cfg.Tracing.OTLPEndpoint != "" {
		exporterOptions := []otlptracehttp.Option{
			otlptracehttp.WithEndpointURL(cfg.Tracing.OTLPEndpoint),
			otlptracehttp.WithTimeout(cfg.Tracing.ExportTimeout),
		}
		if cfg.Tracing.Insecure {
			exporterOptions = append(exporterOptions, otlptracehttp.WithInsecure())
		}
		traceExporter, exportErr := otlptracehttp.New(context.Background(), exporterOptions...)
		if exportErr == nil {
			traceOptions = append(traceOptions, sdktrace.WithBatcher(traceExporter))
			traceSampler = sdktrace.ParentBased(sdktrace.AlwaysSample())
		} else if log != nil {
			log.Warn("initialize OTLP trace exporter failed; local trace correlation remains enabled",
				zap.Error(exportErr),
			)
		}
	}
	traceOptions = append(traceOptions, sdktrace.WithSampler(traceSampler))
	mgr.traceProvider = sdktrace.NewTracerProvider(traceOptions...)
	mgr.tracer = mgr.traceProvider.Tracer("seven-framework-server/http")
	return mgr, nil
}

func (m *manager) Start(ctx context.Context) error {
	_ = ctx
	if m == nil {
		return nil
	}
	if m.traceProvider != nil {
		otel.SetTracerProvider(m.traceProvider)
	}
	if m.meterProvider != nil {
		otel.SetMeterProvider(m.meterProvider)
	}
	otel.SetTextMapPropagator(m.propagator)
	return nil
}

func (m *manager) Close(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if m.traceProvider != nil {
		_ = m.traceProvider.Shutdown(ctx)
	}
	if m.meterProvider != nil {
		_ = m.meterProvider.Shutdown(ctx)
	}
	return nil
}

func (m *manager) Middlewares() []app.HandlerFunc {
	if m == nil {
		return nil
	}
	middlewares := []app.HandlerFunc{
		obsTracing.Middleware(m.tracer, m.propagator, m.logger),
	}
	if m.cfg.Enabled && m.recorder != nil {
		middlewares = append(middlewares, m.httpMetricsMiddleware())
	}
	return middlewares
}

func (m *manager) Attach(server *server.Hertz) {
	if m == nil || !m.cfg.Enabled || server == nil {
		return
	}
	if m.cfg.Prometheus.Enabled && m.promHandler != nil {
		server.GET(m.cfg.Prometheus.Path, m.prometheusHandler())
	}
	if m.cfg.Pprof.Enabled {
		group := server.Group("/ops")
		profiling.Attach(group, m.cfg.Pprof.Prefix)
	}
}

func (m *manager) prometheusHandler() app.HandlerFunc {
	handler := adaptHandler(m.promHandler)
	return func(ctx context.Context, c *app.RequestContext) {
		if !m.isPrometheusAuthorized(c) {
			c.Header("WWW-Authenticate", `Bearer realm="seven-system-ops"`)
			c.AbortWithStatus(401)
			return
		}
		handler(ctx, c)
	}
}

func (m *manager) isPrometheusAuthorized(c *app.RequestContext) bool {
	expected := strings.TrimSpace(m.cfg.Prometheus.AccessToken)
	if expected == "" {
		return false
	}
	if token := strings.TrimSpace(string(c.Request.Header.Peek("X-Ops-Token"))); token == expected {
		return true
	}
	auth := strings.TrimSpace(string(c.Request.Header.Peek("Authorization")))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[len("Bearer "):]) == expected
	}
	return false
}

func (m *manager) RecordSchedulerRun(name string, duration time.Duration, err error) {
	if m == nil || m.recorder == nil {
		return
	}
	m.recorder.RecordSchedulerRun(name, duration, err)
}

func (m *manager) Health() HealthSnapshot {
	snapshot := HealthSnapshot{Enabled: m != nil && m.cfg.Enabled}
	if m == nil {
		return snapshot
	}
	snapshot.Prometheus.Enabled = m.cfg.Prometheus.Enabled
	snapshot.Prometheus.Path = m.cfg.Prometheus.Path
	snapshot.Tracing.Enabled = m.cfg.Tracing.Enabled
	snapshot.Tracing.ServiceName = m.cfg.Tracing.ServiceName
	snapshot.Tracing.OTLPConfigured = strings.TrimSpace(m.cfg.Tracing.OTLPEndpoint) != ""
	snapshot.Pprof.Enabled = m.cfg.Pprof.Enabled
	snapshot.Pprof.Prefix = m.cfg.Pprof.Prefix
	return snapshot
}

func (m *manager) Snapshot(ctx context.Context) RuntimeSnapshot {
	if m == nil || m.recorder == nil {
		return RuntimeSnapshot{CapturedAt: time.Now().UTC()}
	}
	return m.recorder.Snapshot(ctx)
}

func (m *manager) httpMetricsMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		startedAt := time.Now()
		c.Next(ctx)
		if m.recorder != nil {
			path := c.FullPath()
			if path == "" {
				path = string(c.Path())
			}
			m.recorder.RecordHTTPRequest(string(c.Method()), path, c.Response.StatusCode(), time.Since(startedAt))
		}
		if traceID := xcontext.TraceID(c); traceID != "" {
			c.Header(xcontext.TraceIDHeader, traceID)
		}
	}
}

func adaptHandler(handler http.Handler) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		request := httptest.NewRequest(string(c.Method()), c.Request.URI().String(), bytes.NewReader(c.Request.Body()))
		c.Request.Header.VisitAll(func(key, value []byte) {
			request.Header.Add(string(key), string(value))
		})
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		for key, values := range recorder.Result().Header {
			for _, value := range values {
				c.Header(key, value)
			}
		}
		c.Status(recorder.Code)
		_, _ = c.Write(recorder.Body.Bytes())
	}
}
