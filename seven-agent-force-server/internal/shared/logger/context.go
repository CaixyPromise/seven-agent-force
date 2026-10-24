package logger

import (
	"context"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// RuntimeIdentity contains stable fields identifying the process that emitted a log entry.
type RuntimeIdentity struct {
	ServiceName       string
	PlatformMode      string
	NodeCode          string
	Profile           string
	ServiceInstanceID string
}

// WithRuntimeIdentity adds stable source fields to every entry emitted by base.
func WithRuntimeIdentity(base *zap.Logger, identity RuntimeIdentity) *zap.Logger {
	if base == nil {
		base = zap.NewNop()
	}
	return base.With(
		zap.String("service_name", identity.ServiceName),
		zap.String("platform_mode", identity.PlatformMode),
		zap.String("node_code", identity.NodeCode),
		zap.String("profile", identity.Profile),
		zap.String("service_instance_id", identity.ServiceInstanceID),
	)
}

// WithContext adds the canonical trace and span identifiers available in ctx.
func WithContext(ctx context.Context, base *zap.Logger) *zap.Logger {
	if base == nil {
		base = zap.NewNop()
	}
	if ctx == nil {
		return base
	}
	fields := make([]zap.Field, 0, 2)
	spanContext := oteltrace.SpanContextFromContext(ctx)
	traceID := xcontext.TraceIDFromContext(ctx)
	if spanContext.IsValid() {
		traceID = spanContext.TraceID().String()
		fields = append(fields, zap.String("span_id", spanContext.SpanID().String()))
	}
	if traceID != "" {
		fields = append(fields, zap.String("trace_id", traceID))
	}
	if len(fields) == 0 {
		return base
	}
	return base.With(fields...)
}
