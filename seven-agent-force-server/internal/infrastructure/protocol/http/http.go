package http

import (
	"context"
	"strings"
	"time"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/httpx"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/logger"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/route"
	"go.uber.org/zap"
)

const defaultMaxRequestBodyBytes = 8 * 1024 * 1024

func NewServer(cfg config.Config, log *zap.Logger, middlewares ...app.HandlerFunc) *server.Hertz {
	hlog.SetLogger(logger.NewHertzAdapter(log))
	maxRequestBodyBytes := cfg.Server.MaxRequestBodyBytes
	if maxRequestBodyBytes <= 0 {
		maxRequestBodyBytes = defaultMaxRequestBodyBytes
	}

	engine := server.New(
		server.WithBindConfig(httpx.NewBindConfig()),
		server.WithHostPorts(cfg.Address()),
		server.WithReadTimeout(cfg.Server.ReadTimeout),
		server.WithWriteTimeout(cfg.Server.WriteTimeout),
		server.WithIdleTimeout(cfg.Server.IdleTimeout),
		server.WithMaxRequestBodySize(maxRequestBodyBytes),
		server.WithHandleMethodNotAllowed(true),
		server.WithTracer(responseWriteFailureTracer{log: log}),
	)

	engine.Use(StandardMiddlewareChain(cfg, log, middlewares...)...)
	engine.NoRoute(func(ctx context.Context, c *app.RequestContext) {
		writeAppError(c, apperrors.NotFound("请求路径不存在"))
	})
	engine.NoMethod(func(ctx context.Context, c *app.RequestContext) {
		writeAppError(c, apperrors.MethodNotAllowed("请求方法不被允许"))
	})
	return engine
}

// responseWriteFailureTracer observes Hertz errors that happen after handlers
// and middleware have returned, including socket failures while BodyStream is
// being written. It deliberately logs no raw error because storage adapters
// can include private object keys and network errors can include peer details.
type responseWriteFailureTracer struct{ log *zap.Logger }

func (t responseWriteFailureTracer) Start(ctx context.Context, _ *app.RequestContext) context.Context {
	return ctx
}

func (t responseWriteFailureTracer) Finish(ctx context.Context, c *app.RequestContext) {
	if c == nil || !wasStreamingResponse(c) || c.GetTraceInfo() == nil || c.GetTraceInfo().Stats() == nil {
		return
	}
	err := c.GetTraceInfo().Stats().Error()
	if err == nil {
		return
	}
	category := "stream_write_error"
	if isClientDisconnect(err) {
		category = "client_disconnect"
	}
	log := t.log
	if log == nil {
		log = zap.NewNop()
	}
	logger.WithContext(ctx, log).Warn("response_stream_write_aborted",
		zap.String("category", category),
		zap.String("method", string(c.Method())),
		zap.String("path", string(c.Path())),
		zap.String("trace_id", xcontext.TraceID(c)),
	)
}

func wasStreamingResponse(c *app.RequestContext) bool {
	if c == nil {
		return false
	}
	contentType := strings.ToLower(strings.TrimSpace(string(c.Response.Header.ContentType())))
	return c.Response.IsBodyStream() ||
		strings.TrimSpace(string(c.Response.Header.Peek("Content-Disposition"))) != "" ||
		strings.Contains(contentType, "text/event-stream")
}

// StandardMiddlewareChain builds the shared public and internal Hertz middleware order.
func StandardMiddlewareChain(cfg config.Config, log *zap.Logger, middlewares ...app.HandlerFunc) []app.HandlerFunc {
	handlers := make([]app.HandlerFunc, 0, len(middlewares)+3)
	handlers = append(handlers, middlewares...)
	handlers = append(handlers, traceMiddleware(), requestLogMiddleware(log, cfg.Logging.Request), recoveryMiddleware(log))
	return handlers
}

func traceMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		traceID := xcontext.EnsureTraceID(c)
		ctx = xcontext.WithTraceID(ctx, traceID)
		c.Next(ctx)
	}
}

func requestLogMiddleware(log *zap.Logger, cfg config.RequestLoggingConfig) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		startedAt := time.Now()
		meta := xcontext.BuildRequestMeta(c)
		requestLog := logger.WithContext(ctx, log)
		startedEvent, finishedEvent := requestLogEventNames(meta.Path)
		sensitiveDictMutation := isSensitiveDictMutationRequest(c)

		if cfg.Enabled {
			requestLog.Info(startedEvent,
				zap.String("method", meta.Method),
				zap.String("path", meta.Path),
				zap.String("raw_query", maskRawQuery(meta.RawQuery, cfg)),
				zap.String("client_ip", meta.ClientIP),
				zap.String("user_agent", meta.UserAgent),
				zap.String("content_type", meta.ContentType),
				zap.Int("content_length", meta.ContentLength),
				zap.Any("payload", buildRequestPayload(c, cfg)),
			)
		}

		c.Next(ctx)

		if cfg.Enabled {
			fields := []zap.Field{
				zap.String("method", meta.Method),
				zap.String("path", meta.Path),
				zap.Int("status", c.Response.StatusCode()),
				zap.Int64("latency_ms", time.Since(startedAt).Milliseconds()),
				zap.Int("response_size", responseSize(c)),
			}
			if code, message := xcontext.ResponseError(c); code != 0 {
				if sensitiveDictMutation {
					message = "sensitive dictionary mutation rejected"
				}
				fields = append(fields,
					zap.Int("error_code", code),
					zap.String("error_message", message),
				)
				if cause := xcontext.ResponseErrorCause(c); cause != "" && !sensitiveDictMutation {
					fields = append(fields, zap.String("error_cause", cause))
				}
			}
			requestLog.Info(finishedEvent, fields...)
		}
	}
}

func responseSize(c *app.RequestContext) int {
	if c.Response.IsBodyStream() {
		// Body() materializes and consumes Hertz streams. Streaming handlers set
		// their verified size as Content-Length, so logging must remain header-only.
		return c.Response.Header.ContentLength()
	}
	return len(c.Response.Body())
}

func requestLogEventNames(path string) (string, string) {
	if strings.HasPrefix(path, "/internal/node/v1") {
		return "internal_request_started", "internal_request_finished"
	}
	return "request_started", "request_finished"
}

func recoveryMiddleware(log *zap.Logger) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		defer func() {
			if recovered := recover(); recovered != nil {
				handleRecovered(logger.WithContext(ctx, log), c, recovered)
				c.Abort()
			}
		}()
		c.Next(ctx)
	}
}

func Routes(engine *server.Hertz) *route.Engine {
	return engine.Engine
}
