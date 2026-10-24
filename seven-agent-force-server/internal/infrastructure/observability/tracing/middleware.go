package tracing

import (
	"context"
	"crypto/rand"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/logger"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type carrier struct {
	c *app.RequestContext
}

func (c carrier) Get(key string) string {
	return string(c.c.Request.Header.Peek(key))
}

func (c carrier) Set(key, value string) {
	c.c.Request.Header.Set(key, value)
}

func (c carrier) Keys() []string {
	keys := make([]string, 0, 16)
	c.c.Request.Header.VisitAll(func(key, value []byte) {
		keys = append(keys, string(key))
	})
	return keys
}

func Middleware(tracer oteltrace.Tracer, propagator propagation.TextMapPropagator, log *zap.Logger) app.HandlerFunc {
	if tracer == nil {
		tracer = oteltrace.NewNoopTracerProvider().Tracer("seven-framework-server")
	}
	if propagator == nil {
		propagator = propagation.TraceContext{}
	}
	if log == nil {
		log = zap.NewNop()
	}
	return func(ctx context.Context, c *app.RequestContext) {
		if ctx == nil {
			ctx = context.Background()
		}
		failures := &traceFailureReporter{log: log}
		parent := extractServerParent(ctx, c, propagator, failures)
		parentSpanContext, parentContextOK := spanContextFromContext(parent, failures, "server_parent_context")
		if !parentContextOK {
			parent = ctx
			parentSpanContext = oteltrace.SpanContext{}
		}
		legacyTraceID := strings.TrimSpace(string(c.Request.Header.Peek(xcontext.TraceIDHeader)))
		if !parentSpanContext.IsValid() && xcontext.IsCanonicalTraceID(legacyTraceID) {
			if bridged, ok := legacyRemoteParent(ctx, legacyTraceID, failures); ok {
				parent = bridged
				parentSpanContext, _ = spanContextFromContext(parent, failures, "server_legacy_parent_context")
			}
		}
		routeName := string(c.Path())
		if routeName == "" {
			routeName = c.FullPath()
		}
		if routeName == "" {
			routeName = string(c.Request.URI().Path())
		}
		span := startServerSpan(tracer, parent, routeName, failures)
		spanContext, spanOK := spanContextFromSpan(span, failures, "server_span_context")
		spanCtx := parent
		if spanOK {
			spanCtx = oteltrace.ContextWithSpan(parent, span)
		}

		traceID := ""
		if spanOK {
			traceID = spanContext.TraceID().String()
		} else if parentSpanContext.IsValid() {
			traceID = parentSpanContext.TraceID().String()
		}
		if traceID == "" {
			traceID = xcontext.EnsureTraceID(c)
		}
		c.Set(xcontext.TraceIDKey, traceID)
		c.Header(xcontext.TraceIDHeader, traceID)
		spanCtx = xcontext.WithTraceID(spanCtx, traceID)
		defer func() {
			endServerSpan(span, failures)
			failures.warn(spanCtx)
		}()
		if xcontext.IsCanonicalTraceID(legacyTraceID) && legacyTraceID != traceID {
			warnInboundTraceMismatch(spanCtx, log, legacyTraceID)
		}

		startedAt := time.Now()
		c.Next(spanCtx)
		c.Set(xcontext.TraceIDKey, traceID)
		c.Header(xcontext.TraceIDHeader, traceID)

		setServerSpanAttributes(span, failures,
			attribute.String("http.method", string(c.Method())),
			attribute.String("http.route", c.FullPath()),
			attribute.Int("http.status_code", c.Response.StatusCode()),
			attribute.Int64("http.server_duration_ms", time.Since(startedAt).Milliseconds()),
		)
	}
}

type traceFailureReporter struct {
	log       *zap.Logger
	operation string
}

func (r *traceFailureReporter) record(operation string) {
	if r == nil || r.operation != "" {
		return
	}
	r.operation = operation
}

func (r *traceFailureReporter) warn(ctx context.Context) {
	if r == nil || r.operation == "" {
		return
	}
	operation := r.operation
	defer func() {
		_ = recover()
	}()
	logger.WithContext(ctx, r.log).Warn("trace_operation_failed", zap.String("operation", operation))
}

func extractServerParent(ctx context.Context, c *app.RequestContext, propagator propagation.TextMapPropagator, failures *traceFailureReporter) (parent context.Context) {
	parent = ctx
	defer func() {
		if recover() != nil {
			failures.record("server_context_extract")
			parent = ctx
		}
	}()
	extracted := propagator.Extract(ctx, carrier{c: c})
	if extracted == nil {
		failures.record("server_context_extract")
		return ctx
	}
	spanContext, ok := spanContextFromContext(extracted, failures, "server_parent_context")
	if !ok || !spanContext.IsValid() {
		if !ok {
			return ctx
		}
		return ctx
	}
	return oteltrace.ContextWithRemoteSpanContext(ctx, spanContext)
}

func spanContextFromContext(ctx context.Context, failures *traceFailureReporter, operation string) (spanContext oteltrace.SpanContext, ok bool) {
	ok = true
	defer func() {
		if recover() != nil {
			failures.record(operation)
			spanContext = oteltrace.SpanContext{}
			ok = false
		}
	}()
	return oteltrace.SpanContextFromContext(ctx), true
}

func startServerSpan(tracer oteltrace.Tracer, parent context.Context, routeName string, failures *traceFailureReporter) (span oteltrace.Span) {
	defer func() {
		if recover() != nil {
			failures.record("server_span_start")
			span = nil
		}
	}()
	_, span = tracer.Start(parent, routeName, oteltrace.WithSpanKind(oteltrace.SpanKindServer))
	if span == nil {
		failures.record("server_span_start")
	}
	return span
}

func spanContextFromSpan(span oteltrace.Span, failures *traceFailureReporter, operation string) (spanContext oteltrace.SpanContext, ok bool) {
	defer func() {
		if recover() != nil {
			failures.record(operation)
			spanContext = oteltrace.SpanContext{}
			ok = false
		}
	}()
	if span == nil {
		failures.record(operation)
		return oteltrace.SpanContext{}, false
	}
	spanContext = span.SpanContext()
	if !spanContext.IsValid() {
		return spanContext, false
	}
	return spanContext, true
}

func setServerSpanAttributes(span oteltrace.Span, failures *traceFailureReporter, attributes ...attribute.KeyValue) {
	if span == nil {
		return
	}
	defer func() {
		if recover() != nil {
			failures.record("server_span_complete")
		}
	}()
	span.SetAttributes(attributes...)
}

func endServerSpan(span oteltrace.Span, failures *traceFailureReporter) {
	if span == nil {
		return
	}
	defer func() {
		if recover() != nil {
			failures.record("server_span_end")
		}
	}()
	span.End()
}

func warnInboundTraceMismatch(ctx context.Context, log *zap.Logger, legacyTraceID string) {
	defer func() {
		_ = recover()
	}()
	logger.WithContext(ctx, log).Warn("inbound_trace_id_mismatch",
		zap.String("inbound_trace_id", legacyTraceID),
	)
}

func legacyRemoteParent(ctx context.Context, value string, failures *traceFailureReporter) (context.Context, bool) {
	traceID, err := oteltrace.TraceIDFromHex(value)
	if err != nil || !traceID.IsValid() {
		return ctx, false
	}
	var spanID oteltrace.SpanID
	for !spanID.IsValid() {
		if _, err := rand.Read(spanID[:]); err != nil {
			failures.record("server_legacy_parent_bridge")
			candidate := xcontext.NewTraceID()
			spanID, _ = oteltrace.SpanIDFromHex(candidate[:16])
		}
	}
	spanContext := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
		Remote:  true,
	})
	return oteltrace.ContextWithRemoteSpanContext(ctx, spanContext), true
}
