package microservice

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	defaultMaxRequestBytes  int64 = 1 << 20
	defaultMaxResponseBytes int64 = 4 << 20
	defaultConnectTimeout         = time.Second
	defaultRequestTimeout         = 3 * time.Second
	defaultIdleConnTimeout        = 90 * time.Second
)

type HTTPClientOptions struct {
	ConnectTimeout      time.Duration
	RequestTimeout      time.Duration
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
	MaxRequestBytes     int64
	MaxResponseBytes    int64
	HTTPClient          *http.Client
	OutboundPolicy      *OutboundTrustPolicy
	DialContext         func(context.Context, string, string) (net.Conn, error)
	Tracer              oteltrace.Tracer
}

type HTTPServiceClient struct {
	resolver         ServiceResolver
	balancer         LoadBalancer
	client           *http.Client
	maxRequestBytes  int64
	maxResponseBytes int64
	outboundPolicy   *OutboundTrustPolicy
	tracer           oteltrace.Tracer
}

func NewHTTPServiceClient(resolver ServiceResolver, balancer LoadBalancer, options HTTPClientOptions) *HTTPServiceClient {
	connectTimeout := options.ConnectTimeout
	if connectTimeout <= 0 {
		connectTimeout = defaultConnectTimeout
	}
	requestTimeout := options.RequestTimeout
	if requestTimeout <= 0 {
		requestTimeout = defaultRequestTimeout
	}
	idleConnTimeout := options.IdleConnTimeout
	if idleConnTimeout <= 0 {
		idleConnTimeout = defaultIdleConnTimeout
	}
	maxIdleConns := options.MaxIdleConns
	if maxIdleConns <= 0 {
		maxIdleConns = 100
	}
	maxIdleConnsPerHost := options.MaxIdleConnsPerHost
	if maxIdleConnsPerHost <= 0 {
		maxIdleConnsPerHost = 20
	}
	var client *http.Client
	if options.HTTPClient == nil {
		baseDial := options.DialContext
		if baseDial == nil {
			baseDial = (&net.Dialer{Timeout: connectTimeout, KeepAlive: 30 * time.Second}).DialContext
		}
		transport := &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				if pinned, ok := pinnedDialTargetFromContext(ctx); ok {
					_, port, splitErr := net.SplitHostPort(address)
					if splitErr != nil {
						return nil, splitErr
					}
					address = net.JoinHostPort(pinned, port)
				}
				return baseDial(ctx, network, address)
			},
			MaxIdleConns:        maxIdleConns,
			MaxIdleConnsPerHost: maxIdleConnsPerHost,
			IdleConnTimeout:     idleConnTimeout,
		}
		client = &http.Client{Transport: transport, Timeout: requestTimeout}
	} else {
		clientCopy := *options.HTTPClient
		clientCopy.Timeout = requestTimeout
		client = &clientCopy
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	maxResponseBytes := options.MaxResponseBytes
	if maxResponseBytes <= 0 {
		maxResponseBytes = defaultMaxResponseBytes
	}
	maxRequestBytes := options.MaxRequestBytes
	if maxRequestBytes <= 0 {
		maxRequestBytes = defaultMaxRequestBytes
	}
	tracer := options.Tracer
	if tracer == nil {
		tracer = otel.Tracer("seven-framework-server/microservice")
	}
	return &HTTPServiceClient{
		resolver: resolver, balancer: balancer, client: client,
		maxRequestBytes: maxRequestBytes, maxResponseBytes: maxResponseBytes, outboundPolicy: options.OutboundPolicy,
		tracer: tracer,
	}
}

func (c *HTTPServiceClient) Do(ctx context.Context, request ServiceRequest) (*ServiceResponse, error) {
	if c == nil || isNilDependency(c.balancer) || c.client == nil {
		return nil, ErrInvalidDependency
	}
	if ctx == nil {
		return nil, ErrInvalidContext
	}
	if err := c.validateRequest(request); err != nil {
		return nil, err
	}
	var (
		span     oteltrace.Span
		failures *traceFailureReporter
	)
	if request.TracePropagation {
		failures = &traceFailureReporter{handler: request.TraceWarning}
		ctx, span = c.prepareTracePropagation(ctx, &request, failures)
		defer endClientSpan(span, failures)
	}
	instances := append([]ServiceInstance(nil), request.ResolvedInstances...)
	if len(instances) == 0 {
		if isNilDependency(c.resolver) {
			return nil, ErrInvalidDependency
		}
		var err error
		instances, err = c.resolver.Resolve(ctx, request.ServiceName)
		if err != nil {
			return nil, err
		}
	}
	if c.outboundPolicy != nil {
		for index := range instances {
			addresses, err := c.outboundPolicy.resolveAndValidate(ctx, instances[index].Host, request.TrustScope)
			if err != nil {
				return nil, err
			}
			instances[index].dialIP = addresses[0].String()
		}
	}
	canRetry := request.Method == http.MethodGet || request.ReplaySafe
	attempts := 1
	if canRetry {
		attempts = 2
	}
	excluded := make(map[string]struct{}, attempts)
	var lastResponse *ServiceResponse
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		instance, selectErr := c.balancer.Select(request.ServiceName, instances, excluded)
		if selectErr != nil {
			if attempt > 0 {
				return lastResponse, lastErr
			}
			return nil, selectErr
		}
		excluded[instance.IdentityKey()] = struct{}{}
		response, retry, requestErr := c.doOnce(ctx, instance, request)
		lastResponse, lastErr = response, requestErr
		if !retry || attempt+1 == attempts || ctx.Err() != nil {
			completeClientSpan(span, response, requestErr, failures)
			return response, requestErr
		}
	}
	return nil, ErrNoHealthyInstance
}

type traceFailureReporter struct {
	handler TraceFailureHandler
	warned  bool
}

func (r *traceFailureReporter) warn(operation string) {
	if r == nil || r.warned {
		return
	}
	r.warned = true
	defer func() {
		_ = recover()
	}()
	if r.handler != nil {
		r.handler(operation)
	}
}

func (c *HTTPServiceClient) prepareTracePropagation(ctx context.Context, request *ServiceRequest, failures *traceFailureReporter) (next context.Context, span oteltrace.Span) {
	// Keep a complete, safe fallback before touching OTel. A trace failure must
	// neither leak caller-provided propagation headers nor interrupt the Node call.
	fallbackContext, fallbackTraceID := xcontext.EnsureContextTraceID(ctx)
	fallbackHeader := sanitizedTraceHeaders(request.Header, fallbackTraceID)
	next = fallbackContext
	request.Header = fallbackHeader

	operation := "client_trace_prepare"
	defer func() {
		if recover() == nil {
			return
		}
		failures.warn(operation)
		endClientSpan(span, failures)
		next = fallbackContext
		span = nil
		request.Header = fallbackHeader
	}()

	operation = "client_parent_context"
	if traceID := traceIDFromContext(ctx, failures, operation); traceID != "" {
		fallbackContext = xcontext.WithTraceID(ctx, traceID)
		fallbackTraceID = traceID
		fallbackHeader.Set(xcontext.TraceIDHeader, fallbackTraceID)
		next = fallbackContext
	}
	parent := ensureClientParentContext(fallbackContext)

	operation = "client_span_start"
	_, span = c.tracer.Start(parent, request.Method+" "+request.ServiceName, oteltrace.WithSpanKind(oteltrace.SpanKindClient))
	if span == nil {
		failures.warn(operation)
		return fallbackContext, nil
	}

	operation = "client_span_context"
	spanContext, ok := spanContextFromSpan(span, failures, operation)
	if !ok {
		failures.warn(operation)
		endClientSpan(span, failures)
		return fallbackContext, nil
	}
	traceID := spanContext.TraceID().String()
	next = xcontext.WithTraceID(oteltrace.ContextWithSpan(parent, span), traceID)

	candidateHeader := fallbackHeader.Clone()
	operation = "client_trace_inject"
	// Federation only accepts W3C Trace Context. In particular, never allow
	// a global/custom propagator to re-inject baggage across a node boundary.
	propagation.TraceContext{}.Inject(next, propagation.HeaderCarrier(candidateHeader))
	candidateHeader.Set(xcontext.TraceIDHeader, traceID)

	route := request.Path
	if parsed, parseErr := url.Parse(request.Path); parseErr == nil {
		route = parsed.Path
	}
	operation = "client_span_attributes"
	span.SetAttributes(
		attribute.String("http.request.method", request.Method),
		attribute.String("http.route", route),
		attribute.String("server.address", request.ServiceName),
	)
	request.Header = candidateHeader
	return next, span
}

func sanitizedTraceHeaders(header http.Header, traceID string) http.Header {
	sanitized := header.Clone()
	if sanitized == nil {
		sanitized = make(http.Header)
	}
	sanitized.Del("traceparent")
	sanitized.Del("tracestate")
	sanitized.Del("baggage")
	sanitized.Set(xcontext.TraceIDHeader, traceID)
	return sanitized
}

func traceIDFromContext(ctx context.Context, failures *traceFailureReporter, operation string) (traceID string) {
	defer func() {
		if recover() != nil {
			failures.warn(operation)
			traceID = ""
		}
	}()
	spanContext := oteltrace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		return ""
	}
	return spanContext.TraceID().String()
}

func spanContextFromSpan(span oteltrace.Span, failures *traceFailureReporter, operation string) (spanContext oteltrace.SpanContext, ok bool) {
	defer func() {
		if recover() != nil {
			failures.warn(operation)
			spanContext = oteltrace.SpanContext{}
			ok = false
		}
	}()
	if span == nil {
		failures.warn(operation)
		return oteltrace.SpanContext{}, false
	}
	spanContext = span.SpanContext()
	return spanContext, spanContext.IsValid()
}

func completeClientSpan(span oteltrace.Span, response *ServiceResponse, requestErr error, failures *traceFailureReporter) {
	if span == nil {
		return
	}
	defer func() {
		if recover() != nil {
			failures.warn("client_span_complete")
		}
	}()
	if requestErr != nil {
		span.RecordError(requestErr)
		span.SetStatus(codes.Error, "transport error")
		return
	}
	if response == nil {
		return
	}
	span.SetAttributes(attribute.Int("http.response.status_code", response.StatusCode))
	if response.StatusCode >= http.StatusInternalServerError {
		span.SetStatus(codes.Error, "remote server error")
	}
}

func endClientSpan(span oteltrace.Span, failures *traceFailureReporter) {
	if span == nil {
		return
	}
	defer func() {
		if recover() != nil {
			failures.warn("client_span_end")
		}
	}()
	span.End()
}

func (c *HTTPServiceClient) CloseIdleConnections() {
	if c == nil || c.client == nil {
		return
	}
	c.client.CloseIdleConnections()
}

func (c *HTTPServiceClient) validateRequest(request ServiceRequest) error {
	if request.ServiceName == "" || request.Method == "" || request.Path == "" || !strings.HasPrefix(request.Path, "/") || strings.HasPrefix(request.Path, "//") {
		return ErrInvalidRequest
	}
	u, err := url.Parse(request.Path)
	if err != nil || u.IsAbs() || u.Host != "" || u.Fragment != "" {
		return ErrInvalidRequest
	}
	if int64(len(request.Body)) > c.maxRequestBytes {
		return ErrRequestTooLarge
	}
	return nil
}

func (c *HTTPServiceClient) doOnce(ctx context.Context, instance ServiceInstance, request ServiceRequest) (*ServiceResponse, bool, error) {
	target := instance.BaseURL() + request.Path
	if instance.dialIP != "" {
		ctx = context.WithValue(ctx, pinnedDialTargetKey{}, instance.dialIP)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, request.Method, target, bytes.NewReader(request.Body))
	if err != nil {
		return nil, false, fmt.Errorf("build service request: %w", err)
	}
	httpRequest.Header = request.Header.Clone()
	response, err := c.client.Do(httpRequest)
	if err != nil {
		return nil, retryableNetworkError(err), err
	}
	limit := request.MaxResponseBytes
	if limit <= 0 || limit > c.maxResponseBytes {
		limit = c.maxResponseBytes
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if readErr != nil {
		_, _ = io.Copy(io.Discard, response.Body)
		response.Body.Close()
		return nil, false, fmt.Errorf("read service response: %w", readErr)
	}
	if int64(len(body)) > limit {
		_, _ = io.Copy(io.Discard, response.Body)
		response.Body.Close()
		return nil, false, ErrResponseTooLarge
	}
	retry := response.StatusCode == http.StatusBadGateway || response.StatusCode == http.StatusServiceUnavailable || response.StatusCode == http.StatusGatewayTimeout
	if retry {
		_, _ = io.Copy(io.Discard, response.Body)
	}
	response.Body.Close()
	return &ServiceResponse{StatusCode: response.StatusCode, Header: response.Header.Clone(), Body: body, InstanceID: instance.ID}, retry, nil
}

func ensureClientParentContext(ctx context.Context) context.Context {
	if oteltrace.SpanContextFromContext(ctx).IsValid() {
		return ctx
	}
	ctx, traceID := xcontext.EnsureContextTraceID(ctx)
	parsedTraceID, _ := oteltrace.TraceIDFromHex(traceID)
	var spanID oteltrace.SpanID
	for !spanID.IsValid() {
		candidate := xcontext.NewTraceID()
		spanID, _ = oteltrace.SpanIDFromHex(candidate[:16])
	}
	return oteltrace.ContextWithSpanContext(ctx, oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID: parsedTraceID,
		SpanID:  spanID,
	}))
}

type pinnedDialTargetKey struct{}

func pinnedDialTargetFromContext(ctx context.Context) (string, bool) {
	value, ok := ctx.Value(pinnedDialTargetKey{}).(string)
	return value, ok && value != ""
}

func retryableNetworkError(err error) bool {
	var networkErr net.Error
	return errors.As(err, &networkErr)
}
