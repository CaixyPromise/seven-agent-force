package tracing

import (
	"context"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestMiddlewareUsesW3CTraceAsCanonicalAndAlignsContexts(t *testing.T) {
	const (
		traceID     = "0123456789abcdef0123456789abcdef"
		legacyID    = "11111111111111111111111111111111"
		parentSpan  = "0123456789abcdef"
		traceparent = "00-" + traceID + "-" + parentSpan + "-01"
	)
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.NeverSample()))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	core, logs := observer.New(zap.WarnLevel)
	engine := server.New()
	engine.Use(Middleware(provider.Tracer("test"), propagation.TraceContext{}, zap.New(core)))
	engine.GET("/trace", func(ctx context.Context, c *app.RequestContext) {
		spanContext := oteltrace.SpanContextFromContext(ctx)
		if spanContext.TraceID().String() != traceID {
			t.Errorf("span trace = %s, want %s", spanContext.TraceID(), traceID)
		}
		if xcontext.TraceIDFromContext(ctx) != traceID || xcontext.TraceID(c) != traceID {
			t.Errorf("contexts not aligned: go=%q hertz=%q", xcontext.TraceIDFromContext(ctx), xcontext.TraceID(c))
		}
		c.String(200, "ok")
	})

	resp := ut.PerformRequest(engine.Engine, "GET", "/trace", nil,
		ut.Header{Key: "traceparent", Value: traceparent},
		ut.Header{Key: xcontext.TraceIDHeader, Value: legacyID},
	)
	if got := resp.Header().Get(xcontext.TraceIDHeader); got != traceID {
		t.Fatalf("response trace = %q, want %q", got, traceID)
	}
	if logs.FilterMessage("inbound_trace_id_mismatch").Len() != 1 {
		t.Fatalf("mismatch logs = %d, want 1", logs.Len())
	}
}

func TestMiddlewareBridgesLegacyTraceWithoutControllingSampling(t *testing.T) {
	const legacyID = "22222222222222222222222222222222"
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	engine := server.New()
	engine.Use(Middleware(provider.Tracer("test"), propagation.TraceContext{}, zap.NewNop()))
	engine.GET("/trace", func(ctx context.Context, c *app.RequestContext) {
		spanContext := oteltrace.SpanContextFromContext(ctx)
		if spanContext.TraceID().String() != legacyID {
			t.Errorf("span trace = %s, want %s", spanContext.TraceID(), legacyID)
		}
		if !spanContext.SpanID().IsValid() {
			t.Error("span ID must be valid")
		}
		if spanContext.IsSampled() || oteltrace.SpanFromContext(ctx).IsRecording() {
			t.Errorf("legacy bridge controlled sampling: %#v", spanContext)
		}
		c.String(200, "ok")
	})

	resp := ut.PerformRequest(engine.Engine, "GET", "/trace", nil, ut.Header{Key: xcontext.TraceIDHeader, Value: legacyID})
	if got := resp.Header().Get(xcontext.TraceIDHeader); got != legacyID {
		t.Fatalf("response trace = %q, want %q", got, legacyID)
	}
}

func TestMiddlewareReplacesMalformedAndZeroLegacyTrace(t *testing.T) {
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.NeverSample()))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	for _, input := range []string{"trace-fixed", "00000000000000000000000000000000"} {
		t.Run(input, func(t *testing.T) {
			engine := server.New()
			engine.Use(Middleware(provider.Tracer("test"), propagation.TraceContext{}, zap.NewNop()))
			engine.GET("/trace", func(ctx context.Context, c *app.RequestContext) { c.String(200, "ok") })
			resp := ut.PerformRequest(engine.Engine, "GET", "/trace", nil, ut.Header{Key: xcontext.TraceIDHeader, Value: input})
			got := resp.Header().Get(xcontext.TraceIDHeader)
			parsed, err := oteltrace.TraceIDFromHex(got)
			if err != nil || !parsed.IsValid() || got == input {
				t.Fatalf("response trace = %q, input=%q err=%v", got, input, err)
			}
		})
	}
}

func TestMiddlewareFailsOpenWhenTraceExtractionPanics(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	engine := server.New()
	engine.Use(Middleware(oteltrace.NewNoopTracerProvider().Tracer("test"), panickingPropagator{}, zap.New(core)))
	engine.GET("/business", func(ctx context.Context, c *app.RequestContext) {
		if xcontext.TraceIDFromContext(ctx) == "" {
			t.Error("business handler lost fallback trace context")
		}
		c.String(200, "business-ok")
	})

	resp := ut.PerformRequest(engine.Engine, "GET", "/business", nil)
	if resp.Code != 200 || resp.Body.String() != "business-ok" {
		t.Fatalf("business response=%d %q", resp.Code, resp.Body.String())
	}
	if !xcontext.IsCanonicalTraceID(resp.Header().Get(xcontext.TraceIDHeader)) {
		t.Fatalf("fallback trace header=%q", resp.Header().Get(xcontext.TraceIDHeader))
	}
	warnings := logs.FilterMessage("trace_operation_failed").All()
	if len(warnings) != 1 || warnings[0].ContextMap()["operation"] != "server_context_extract" {
		t.Fatalf("trace degradation warning=%#v", warnings)
	}
}

type panickingPropagator struct{}

func (panickingPropagator) Inject(context.Context, propagation.TextMapCarrier) {}

func (panickingPropagator) Extract(context.Context, propagation.TextMapCarrier) context.Context {
	panic("trace extraction must not interrupt business")
}

func (panickingPropagator) Fields() []string { return nil }
