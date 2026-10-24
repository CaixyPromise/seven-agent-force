package application

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	providerdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/domain"
	invocationdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/providerinvocation/domain"
)

const (
	MaxRequestBytes      = 128 << 20
	MaxNonStreamingBytes = 128 << 20
	MaxSSEEventBytes     = 1 << 20
	TimestampSkew        = 30 * time.Second
	streamIdleTimeout    = 5 * time.Minute
)

var (
	errProviderResponseTooLarge = errors.New("provider response exceeds the configured limit")
	errProviderSSEEventTooLarge = errors.New("provider SSE event exceeds the configured limit")
	errProviderStreamIdle       = errors.New("provider stream idle timeout")
)

type Repository interface {
	Reserve(context.Context, int64, invocationdomain.RequestIdentity, invocationdomain.Operation, []byte, time.Time) (invocationdomain.Reservation, error)
	SetCredentialVersion(context.Context, int64, int64) error
	MarkStreaming(context.Context, int64, int, string, time.Time) error
	Finish(context.Context, invocationdomain.Finish) error
	Get(context.Context, int64, int64) (invocationdomain.View, error)
	ListAttempt(context.Context, int64, int64, int) ([]invocationdomain.View, error)
	ListTask(context.Context, int64, int64, int) ([]invocationdomain.View, error)
	ListCredential(context.Context, int64, int64, int) ([]invocationdomain.View, error)
	CancelActive(context.Context, int64, int64, string, time.Time) error
	CancelCredentialActive(context.Context, int64, int64, string, time.Time) error
}

// ReviewRunRepository is the optional extension needed by ReviewRun callers.
// Keeping it separate preserves source compatibility for existing Agent
// Attempt fakes and lets the composition root bind this seam when ReviewRun
// runtime wiring is ready.
type ReviewRunRepository interface {
	Repository
	ListReviewRun(context.Context, int64, int64, int64, int) ([]invocationdomain.View, error)
	CancelReviewRunActive(context.Context, int64, int64, int64, string, time.Time) error
}

type ReviewRunTaskRepository interface {
	CancelReviewRunActiveForTask(context.Context, int64, int64, string, time.Time) error
}

type AnalyticsRepository interface {
	GetAnalytics(context.Context, int64, int, time.Time) (invocationdomain.Analytics, error)
}

type CredentialResolver interface {
	ResolveInvocationCredential(context.Context, int64, int64, int64, string) (providerdomain.InvocationCredential, error)
}

type Adapter interface {
	Invoke(context.Context, providerdomain.InvocationCredential, invocationdomain.Operation, string, []byte, http.Header, bool) (ProviderResponse, error)
}

type IDGenerator interface{ NextID() int64 }

type ProviderResponse struct {
	StatusCode        int
	Header            http.Header
	Body              io.ReadCloser
	UpstreamRequestID string
}

type InvokeRequest struct {
	Identity  invocationdomain.RequestIdentity
	Operation invocationdomain.Operation
	Body      []byte
	Header    http.Header
	Accept    string
}

type InvokeResponse struct {
	ProtocolInvocationKey string
	StatusCode            int
	Header                http.Header
	Body                  io.ReadCloser
}

type Service struct {
	repository  Repository
	credentials CredentialResolver
	adapter     Adapter
	idGen       IDGenerator
	now         func() time.Time
	mu          sync.Mutex
	active      map[int64]activeInvocation
}

type activeInvocation struct {
	tenantID           int64
	executionKind      invocationdomain.ExecutionKind
	taskID             int64
	attemptID          int64
	reviewRunID        int64
	reviewRunAttemptID int64
	credentialID       int64
	cancel             context.CancelFunc
}

func NewService(repository Repository, credentials CredentialResolver, adapter Adapter, idGen IDGenerator) *Service {
	return &Service{repository: repository, credentials: credentials, adapter: adapter, idGen: idGen, now: time.Now, active: make(map[int64]activeInvocation)}
}

func (s *Service) InvokeProvider(ctx context.Context, request InvokeRequest) (InvokeResponse, error) {
	if s == nil || s.repository == nil || s.credentials == nil || s.adapter == nil || s.idGen == nil {
		return InvokeResponse{}, invocationError("PROVIDER_GATEWAY_NOT_CONFIGURED", http.StatusServiceUnavailable, "Provider Gateway is not configured", nil)
	}
	if !request.Operation.Valid() {
		return InvokeResponse{}, invocationError("PROVIDER_OPERATION_NOT_SUPPORTED", http.StatusNotFound, "Provider operation is not supported", nil)
	}
	if err := request.Identity.Validate(); err != nil {
		return InvokeResponse{}, err
	}
	if len(request.Body) == 0 || len(request.Body) > MaxRequestBytes {
		return InvokeResponse{}, invocationError("PROVIDER_REQUEST_TOO_LARGE", http.StatusRequestEntityTooLarge, "Provider request body exceeds the configured limit", nil)
	}
	now := s.now().UTC()
	if request.Identity.Timestamp.IsZero() || request.Identity.Timestamp.Before(now.Add(-TimestampSkew)) || request.Identity.Timestamp.After(now.Add(TimestampSkew)) {
		return InvokeResponse{}, invocationError("PROVIDER_TIMESTAMP_INVALID", http.StatusConflict, "Provider invocation timestamp is outside the accepted window", nil)
	}
	digest := sha256.Sum256(request.Body)
	canonicalBodyDigest, digestErr := normalizeSHA256(request.Identity.BodySHA256)
	if digestErr != nil || !strings.EqualFold(hex.EncodeToString(digest[:]), canonicalBodyDigest) {
		return InvokeResponse{}, invocationError("PROVIDER_BODY_DIGEST_MISMATCH", http.StatusConflict, "Provider request body digest does not match", nil)
	}
	request.Identity.BodySHA256 = canonicalBodyDigest
	policy, err := parseRequestPolicy(request.Body)
	if err != nil {
		return InvokeResponse{}, err
	}
	invocationID := s.idGen.NextID()
	if invocationID <= 0 {
		return InvokeResponse{}, invocationError("MODEL_INVOCATION_ID_UNAVAILABLE", http.StatusServiceUnavailable, "model invocation identity is unavailable", nil)
	}
	reservation, err := s.repository.Reserve(ctx, invocationID, request.Identity, request.Operation, request.Body, now)
	if err != nil {
		return InvokeResponse{}, err
	}
	if policy.Background {
		_ = s.repository.Finish(context.Background(), invocationdomain.Finish{ID: reservation.ID, Status: invocationdomain.StatusFailed, FinishedAt: now, ErrorCode: "PROVIDER_BACKGROUND_NOT_ALLOWED"})
		return InvokeResponse{}, invocationError("PROVIDER_BACKGROUND_NOT_ALLOWED", http.StatusBadRequest, "background Responses are not supported inside an Attempt", nil)
	}
	credential, err := s.credentials.ResolveInvocationCredential(ctx, reservation.Frozen.TenantID, reservation.Frozen.CredentialID, reservation.Frozen.CredentialVersion, reservation.Frozen.PrimaryModel)
	if err != nil {
		_ = s.repository.Finish(context.Background(), invocationdomain.Finish{ID: reservation.ID, Status: invocationdomain.StatusFailed, FinishedAt: s.now().UTC(), ErrorCode: "PROVIDER_CONFIG_UNAVAILABLE"})
		return InvokeResponse{}, invocationError("PROVIDER_CONFIG_UNAVAILABLE", http.StatusConflict, "Frozen Provider configuration is unavailable", err)
	}
	// A key rotation takes effect immediately. Persist the exact secret version
	// that was decrypted for this call, including a rotation racing reservation.
	if credential.CredentialVersion != reservation.Frozen.CredentialVersion {
		if err := s.repository.SetCredentialVersion(ctx, reservation.ID, credential.CredentialVersion); err != nil {
			_ = s.repository.Finish(context.Background(), invocationdomain.Finish{ID: reservation.ID, Status: invocationdomain.StatusFailed, FinishedAt: s.now().UTC(), ErrorCode: "MODEL_CREDENTIAL_VERSION_UPDATE_FAILED"})
			return InvokeResponse{}, err
		}
	}
	if (credential.Protocol == providerdomain.ProtocolOpenAI && request.Operation != invocationdomain.OperationResponsesCreate && request.Operation != invocationdomain.OperationResponsesCompact) ||
		(credential.Protocol == providerdomain.ProtocolAnthropic && request.Operation != invocationdomain.OperationMessagesCreate && request.Operation != invocationdomain.OperationMessagesCountTokens) {
		_ = s.repository.Finish(context.Background(), invocationdomain.Finish{ID: reservation.ID, Status: invocationdomain.StatusFailed, FinishedAt: s.now().UTC(), ErrorCode: "MODEL_OPERATION_NOT_SUPPORTED"})
		return InvokeResponse{}, invocationError("MODEL_OPERATION_NOT_SUPPORTED", http.StatusBadRequest, "model operation does not match the credential protocol", nil)
	}
	deadline := reservation.Frozen.ExecutionDeadline
	requestTimeout := 5 * time.Minute
	if remaining := time.Until(deadline); !deadline.IsZero() && remaining < requestTimeout {
		requestTimeout = remaining
	}
	if requestTimeout <= 0 {
		_ = s.repository.Finish(context.Background(), invocationdomain.Finish{ID: reservation.ID, Status: invocationdomain.StatusFailed, FinishedAt: s.now().UTC(), ErrorCode: "PROVIDER_DEADLINE_EXCEEDED"})
		return InvokeResponse{}, invocationError("PROVIDER_DEADLINE_EXCEEDED", http.StatusGatewayTimeout, "Attempt execution deadline has expired", nil)
	}
	// Streaming calls can legitimately outlive the response-header timeout.
	// The transport bounds header wait; the Attempt context remains the total
	// execution boundary, including explicit cancellation and credential disable.
	invocationCtx := ctx
	var cancel context.CancelFunc
	if policy.Stream {
		if !deadline.IsZero() {
			invocationCtx, cancel = context.WithDeadline(ctx, deadline)
		} else {
			invocationCtx, cancel = context.WithCancel(ctx)
		}
	} else {
		invocationCtx, cancel = context.WithTimeout(ctx, requestTimeout)
	}
	s.track(reservation.ID, reservation.Frozen.TenantID, request.Identity, reservation.Frozen.CredentialID, cancel)
	providerResponse, err := s.adapter.Invoke(invocationCtx, credential, request.Operation, reservation.ProtocolInvocationKey, request.Body, request.Header, policy.Stream)
	if err != nil {
		s.untrack(reservation.ID)
		cancel()
		_ = s.repository.Finish(context.Background(), invocationdomain.Finish{ID: reservation.ID, Status: invocationdomain.StatusFailed, FinishedAt: s.now().UTC(), ErrorCode: classifyAdapterError(err)})
		return InvokeResponse{}, err
	}
	headersAt := s.now().UTC()
	if err := s.repository.MarkStreaming(ctx, reservation.ID, providerResponse.StatusCode, providerResponse.UpstreamRequestID, headersAt); err != nil {
		providerResponse.Body.Close()
		s.untrack(reservation.ID)
		cancel()
		return InvokeResponse{}, invocationError("PROVIDER_AUDIT_PERSIST_FAILED", http.StatusInternalServerError, "Provider invocation facts could not be persisted", err)
	}
	upstreamBody := providerResponse.Body
	if policy.Stream {
		upstreamBody = newIdleTimeoutBody(upstreamBody, streamIdleTimeout)
	}
	body := newObservedBody(upstreamBody, policy.Stream, providerResponse.StatusCode, providerResponse.UpstreamRequestID, reservation.ID, s.repository, s.now, func() {
		s.untrack(reservation.ID)
		cancel()
	})
	return InvokeResponse{ProtocolInvocationKey: reservation.ProtocolInvocationKey, StatusCode: providerResponse.StatusCode, Header: providerResponse.Header, Body: body}, nil
}

// idleTimeoutBody bounds silence between streaming bytes without imposing a
// total duration on a stream that continues to make progress.
type idleTimeoutBody struct {
	source   io.ReadCloser
	timer    *time.Timer
	timeout  time.Duration
	timedOut atomic.Bool
}

func newIdleTimeoutBody(source io.ReadCloser, timeout time.Duration) io.ReadCloser {
	b := &idleTimeoutBody{source: source, timeout: timeout}
	b.timer = time.AfterFunc(timeout, func() {
		b.timedOut.Store(true)
		_ = source.Close()
	})
	return b
}

func (b *idleTimeoutBody) Read(p []byte) (int, error) {
	n, err := b.source.Read(p)
	if n > 0 {
		b.timer.Reset(b.timeout)
	}
	if err != nil && b.timedOut.Load() {
		return n, errProviderStreamIdle
	}
	return n, err
}

func (b *idleTimeoutBody) Close() error {
	b.timer.Stop()
	return b.source.Close()
}

func normalizeSHA256(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "sha256:")
	if len(value) != sha256.Size*2 {
		return "", errors.New("SHA-256 digest length is invalid")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return "", errors.New("SHA-256 digest is invalid")
	}
	return value, nil
}

func (s *Service) CancelAttemptInvocations(ctx context.Context, tenantID int64, attemptID int64, reasonCode string) error {
	s.mu.Lock()
	for _, invocation := range s.active {
		if invocation.executionKind == invocationdomain.ExecutionKindAgentAttempt && invocation.tenantID == tenantID && invocation.attemptID == attemptID {
			invocation.cancel()
		}
	}
	s.mu.Unlock()
	return s.repository.CancelActive(ctx, tenantID, attemptID, reasonCode, s.now().UTC())
}

func (s *Service) CancelReviewRunInvocations(ctx context.Context, tenantID, reviewRunID, reviewRunAttemptID int64, reasonCode string) error {
	repository, ok := s.repository.(ReviewRunRepository)
	if !ok {
		return invocationError("REVIEW_RUN_REPOSITORY_NOT_CONFIGURED", http.StatusServiceUnavailable, "ReviewRun Provider invocation repository is not configured", nil)
	}
	s.mu.Lock()
	for _, invocation := range s.active {
		if invocation.executionKind == invocationdomain.ExecutionKindReviewRun && invocation.tenantID == tenantID && invocation.reviewRunID == reviewRunID && invocation.reviewRunAttemptID == reviewRunAttemptID {
			invocation.cancel()
		}
	}
	s.mu.Unlock()
	return repository.CancelReviewRunActive(ctx, tenantID, reviewRunID, reviewRunAttemptID, reasonCode, s.now().UTC())
}

// CancelReviewRunInvocationsForTask is used after the Task cancellation
// transaction commits. ReviewRun rows are already fenced in that transaction;
// this second, narrow operation stops any in-process Provider stream and
// closes active durable invocation facts without touching Agent Attempt
// invocations for the same task.
func (s *Service) CancelReviewRunInvocationsForTask(ctx context.Context, tenantID, taskID int64, reasonCode string) error {
	repository, ok := s.repository.(ReviewRunTaskRepository)
	if !ok {
		return invocationError("REVIEW_RUN_REPOSITORY_NOT_CONFIGURED", http.StatusServiceUnavailable, "ReviewRun Provider invocation repository is not configured", nil)
	}
	s.mu.Lock()
	for _, invocation := range s.active {
		if invocation.executionKind == invocationdomain.ExecutionKindReviewRun && invocation.tenantID == tenantID && invocation.taskID == taskID {
			invocation.cancel()
		}
	}
	s.mu.Unlock()
	return repository.CancelReviewRunActiveForTask(ctx, tenantID, taskID, reasonCode, s.now().UTC())
}

func (s *Service) CancelModelInvocations(ctx context.Context, tenantID int64, credentialID int64, reasonCode string) error {
	s.mu.Lock()
	for _, invocation := range s.active {
		if invocation.tenantID == tenantID && invocation.credentialID == credentialID {
			invocation.cancel()
		}
	}
	s.mu.Unlock()
	return s.repository.CancelCredentialActive(ctx, tenantID, credentialID, reasonCode, s.now().UTC())
}

func (s *Service) ResolveInvocationView(ctx context.Context, tenantID, invocationID int64) (invocationdomain.View, error) {
	return s.repository.Get(ctx, tenantID, invocationID)
}

func (s *Service) ListAttemptInvocations(ctx context.Context, tenantID int64, attemptID int64, limit int) ([]invocationdomain.View, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	return s.repository.ListAttempt(ctx, tenantID, attemptID, limit)
}

func (s *Service) ListTaskInvocations(ctx context.Context, tenantID int64, taskID int64, limit int) ([]invocationdomain.TaskView, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	items, err := s.repository.ListTask(ctx, tenantID, taskID, limit)
	if err != nil {
		return nil, err
	}
	projected := make([]invocationdomain.TaskView, 0, len(items))
	for _, item := range items {
		projected = append(projected, item.TaskProjection())
	}
	return projected, nil
}

func (s *Service) ListCredentialInvocations(ctx context.Context, tenantID int64, credentialID int64, limit int) ([]invocationdomain.View, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	return s.repository.ListCredential(ctx, tenantID, credentialID, limit)
}

func (s *Service) GetAnalytics(ctx context.Context, tenantID int64, days int) (invocationdomain.Analytics, error) {
	if tenantID < 1 {
		return invocationdomain.Analytics{}, invocationError("MODEL_ANALYTICS_SCOPE_INVALID", http.StatusForbidden, "organization scope is required", nil)
	}
	if days != 7 && days != 30 && days != 90 {
		return invocationdomain.Analytics{}, invocationError("MODEL_ANALYTICS_RANGE_INVALID", http.StatusBadRequest, "analytics range must be 7, 30, or 90 days", nil)
	}
	repository, ok := s.repository.(AnalyticsRepository)
	if !ok {
		return invocationdomain.Analytics{}, invocationError("MODEL_ANALYTICS_UNAVAILABLE", http.StatusServiceUnavailable, "model analytics is unavailable", nil)
	}
	return repository.GetAnalytics(ctx, tenantID, days, s.now().UTC())
}

func (s *Service) ListReviewRunInvocations(ctx context.Context, tenantID, reviewRunID, reviewRunAttemptID, limit int64) ([]invocationdomain.View, error) {
	repository, ok := s.repository.(ReviewRunRepository)
	if !ok {
		return nil, invocationError("REVIEW_RUN_REPOSITORY_NOT_CONFIGURED", http.StatusServiceUnavailable, "ReviewRun Provider invocation repository is not configured", nil)
	}
	bounded := int(limit)
	if bounded < 1 || bounded > 200 {
		bounded = 100
	}
	return repository.ListReviewRun(ctx, tenantID, reviewRunID, reviewRunAttemptID, bounded)
}

func (s *Service) track(id int64, tenantID int64, identity invocationdomain.RequestIdentity, credentialID int64, cancel context.CancelFunc) {
	s.mu.Lock()
	s.active[id] = activeInvocation{tenantID: tenantID, executionKind: identity.ExecutionKind.Normalized(), taskID: identity.TaskID, attemptID: identity.AttemptID, reviewRunID: identity.ReviewRunID, reviewRunAttemptID: identity.ReviewRunAttemptID, credentialID: credentialID, cancel: cancel}
	s.mu.Unlock()
}

func (s *Service) untrack(id int64) {
	s.mu.Lock()
	delete(s.active, id)
	s.mu.Unlock()
}

type requestPolicy struct {
	Model      string `json:"model"`
	Stream     bool   `json:"stream"`
	Background bool   `json:"background"`
}

func parseRequestPolicy(body []byte) (requestPolicy, error) {
	var policy requestPolicy
	if err := json.Unmarshal(body, &policy); err != nil {
		return policy, invocationError("PROVIDER_REQUEST_INVALID", http.StatusBadRequest, "Provider request must be a valid JSON object", err)
	}
	if strings.TrimSpace(policy.Model) == "" {
		return policy, invocationError("MODEL_REQUEST_MODEL_INVALID", http.StatusBadRequest, "model request model is required", nil)
	}
	policy.Model = strings.TrimSpace(policy.Model)
	if len(policy.Model) > 256 || strings.IndexFunc(policy.Model, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
		return policy, invocationError("MODEL_REQUEST_MODEL_INVALID", http.StatusBadRequest, "model request model is invalid", nil)
	}
	return policy, nil
}

func invocationError(code string, status int, message string, cause error) error {
	return &invocationdomain.Error{ReasonCode: code, HTTPStatus: status, Message: message, Cause: cause}
}

func classifyAdapterError(err error) string {
	if errors.Is(err, errProviderStreamIdle) {
		return "PROVIDER_STREAM_IDLE_TIMEOUT"
	}
	if errors.Is(err, errProviderResponseTooLarge) {
		return "PROVIDER_RESPONSE_TOO_LARGE"
	}
	if errors.Is(err, errProviderSSEEventTooLarge) {
		return "PROVIDER_SSE_EVENT_TOO_LARGE"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "PROVIDER_REQUEST_TIMEOUT"
	}
	if errors.Is(err, context.Canceled) {
		return "PROVIDER_REQUEST_CANCELLED"
	}
	return invocationdomain.Reason(err)
}

type observedBody struct {
	source       io.ReadCloser
	tee          io.Reader
	pipeWriter   *io.PipeWriter
	result       <-chan observedResult
	stream       bool
	statusCode   int
	requestID    string
	invocationID int64
	repository   Repository
	now          func() time.Time
	done         func()
	once         sync.Once
	firstByte    *time.Time
}

func newObservedBody(source io.ReadCloser, stream bool, statusCode int, requestID string, invocationID int64, repository Repository, now func() time.Time, done func()) io.ReadCloser {
	bounded := newBoundedProviderBody(source, stream, MaxNonStreamingBytes, MaxSSEEventBytes)
	reader, writer := io.Pipe()
	result := make(chan observedResult, 1)
	go func() {
		result <- parseResult(reader, stream)
		close(result)
	}()
	return &observedBody{source: bounded, tee: io.TeeReader(bounded, writer), pipeWriter: writer, result: result, stream: stream, statusCode: statusCode, requestID: requestID, invocationID: invocationID, repository: repository, now: now, done: done}
}

func (b *observedBody) Read(p []byte) (int, error) {
	n, err := b.tee.Read(p)
	if n > 0 && b.firstByte == nil {
		now := b.now().UTC()
		b.firstByte = &now
	}
	if err != nil {
		_ = b.pipeWriter.CloseWithError(err)
		b.finish(err)
	}
	return n, err
}

func (b *observedBody) Close() error {
	err := b.source.Close()
	_ = b.pipeWriter.CloseWithError(context.Canceled)
	b.finish(context.Canceled)
	return err
}

func (b *observedBody) finish(readErr error) {
	b.once.Do(func() {
		observed := <-b.result
		status := invocationdomain.StatusSucceeded
		errorCode := ""
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			if errors.Is(readErr, errProviderResponseTooLarge) || errors.Is(readErr, errProviderSSEEventTooLarge) {
				status = invocationdomain.StatusFailed
			} else {
				// Headers or partial body may already have reached the caller.
				// The upstream result is not safely retryable after that point.
				status = invocationdomain.StatusOutcomeUnknown
			}
			errorCode = classifyAdapterError(readErr)
		} else if b.statusCode < 200 || b.statusCode >= 300 {
			status = invocationdomain.StatusFailed
			errorCode = "PROVIDER_UPSTREAM_REJECTED"
		}
		statusCode := b.statusCode
		_ = b.repository.Finish(context.Background(), invocationdomain.Finish{
			ID: b.invocationID, Status: status, StatusCode: &statusCode, UpstreamRequestID: b.requestID,
			FirstByteAt: b.firstByte, FinishedAt: b.now().UTC(), ResponseModel: observed.Model, Usage: observed.Usage, ErrorCode: errorCode,
		})
		b.done()
	})
}

// boundedProviderBody enforces the wire limits without parsing and
// re-serializing Provider JSON or SSE bytes.
type boundedProviderBody struct {
	source           io.ReadCloser
	stream           bool
	maxTotal         int64
	maxEvent         int64
	total            int64
	eventBytes       int64
	lineContentBytes int64
	pendingErr       error
}

func newBoundedProviderBody(source io.ReadCloser, stream bool, maxTotal, maxEvent int64) io.ReadCloser {
	return &boundedProviderBody{source: source, stream: stream, maxTotal: maxTotal, maxEvent: maxEvent}
}

func (b *boundedProviderBody) Read(p []byte) (int, error) {
	if b.pendingErr != nil {
		err := b.pendingErr
		b.pendingErr = nil
		return 0, err
	}
	n, err := b.source.Read(p)
	if n == 0 {
		return n, err
	}
	if !b.stream {
		remaining := b.maxTotal - b.total
		if remaining <= 0 {
			return 0, errProviderResponseTooLarge
		}
		if int64(n) > remaining {
			n = int(remaining)
			b.pendingErr = errProviderResponseTooLarge
			err = nil
		}
		b.total += int64(n)
		return n, err
	}
	for index := 0; index < n; index++ {
		value := p[index]
		b.eventBytes++
		switch value {
		case '\n':
			if b.lineContentBytes == 0 {
				b.eventBytes = 0
			}
			b.lineContentBytes = 0
		case '\r':
			// CR belongs to the CRLF delimiter, not the line content.
		default:
			b.lineContentBytes++
		}
		if b.eventBytes > b.maxEvent {
			if index == 0 {
				return 0, errProviderSSEEventTooLarge
			}
			b.pendingErr = errProviderSSEEventTooLarge
			return index, nil
		}
	}
	return n, err
}

func (b *boundedProviderBody) Close() error { return b.source.Close() }

type observedResult struct {
	Model string
	Usage invocationdomain.Usage
}

func parseResult(reader io.Reader, stream bool) observedResult {
	if stream {
		return parseSSEResult(reader)
	}
	var envelope struct {
		Model string          `json:"model"`
		Usage json.RawMessage `json:"usage"`
	}
	if json.NewDecoder(io.LimitReader(reader, MaxNonStreamingBytes+1)).Decode(&envelope) != nil {
		return observedResult{}
	}
	return observedResult{Model: strings.TrimSpace(envelope.Model), Usage: decodeUsage(envelope.Usage)}
}

func parseSSEResult(reader io.Reader) observedResult {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), MaxSSEEventBytes)
	var result observedResult
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		var event struct {
			Model    string          `json:"model"`
			Usage    json.RawMessage `json:"usage"`
			Response struct {
				Model string          `json:"model"`
				Usage json.RawMessage `json:"usage"`
			} `json:"response"`
			Message struct {
				Model string          `json:"model"`
				Usage json.RawMessage `json:"usage"`
			} `json:"message"`
			Delta struct {
				Usage json.RawMessage `json:"usage"`
			} `json:"delta"`
		}
		if json.Unmarshal(payload, &event) != nil {
			continue
		}
		candidate := event.Usage
		if len(candidate) == 0 {
			candidate = event.Response.Usage
		}
		if len(candidate) == 0 {
			candidate = event.Message.Usage
		}
		if len(candidate) == 0 {
			candidate = event.Delta.Usage
		}
		result.Usage = mergeUsage(result.Usage, decodeUsage(candidate))
		if event.Model != "" {
			result.Model = event.Model
		} else if event.Response.Model != "" {
			result.Model = event.Response.Model
		} else if event.Message.Model != "" {
			result.Model = event.Message.Model
		}
	}
	return result
}

func decodeUsage(raw json.RawMessage) invocationdomain.Usage {
	if len(raw) == 0 {
		return invocationdomain.Usage{}
	}
	var value struct {
		InputTokens              *int64 `json:"input_tokens"`
		CacheReadInputTokens     *int64 `json:"cache_read_input_tokens"`
		CacheCreationInputTokens *int64 `json:"cache_creation_input_tokens"`
		CacheWriteTokens         *int64 `json:"cache_write_tokens"`
		OutputTokens             *int64 `json:"output_tokens"`
		TotalTokens              *int64 `json:"total_tokens"`
		InputDetails             struct {
			CachedTokens *int64 `json:"cached_tokens"`
		} `json:"input_tokens_details"`
		OutputDetails struct {
			ReasoningTokens *int64 `json:"reasoning_tokens"`
		} `json:"output_tokens_details"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return invocationdomain.Usage{}
	}
	cached := value.CacheReadInputTokens
	if cached == nil {
		cached = value.InputDetails.CachedTokens
	}
	created := value.CacheCreationInputTokens
	if created == nil {
		created = value.CacheWriteTokens
	}
	filtered, _ := json.Marshal(value)
	return invocationdomain.Usage{InputTokens: value.InputTokens, CachedInputTokens: cached, CacheCreationInputTokens: created, OutputTokens: value.OutputTokens, ReasoningOutputTokens: value.OutputDetails.ReasoningTokens, TotalTokens: value.TotalTokens, Raw: filtered}
}

func mergeUsage(current, next invocationdomain.Usage) invocationdomain.Usage {
	if next.InputTokens != nil {
		current.InputTokens = next.InputTokens
	}
	if next.CachedInputTokens != nil {
		current.CachedInputTokens = next.CachedInputTokens
	}
	if next.CacheCreationInputTokens != nil {
		current.CacheCreationInputTokens = next.CacheCreationInputTokens
	}
	if next.OutputTokens != nil {
		current.OutputTokens = next.OutputTokens
	}
	if next.ReasoningOutputTokens != nil {
		current.ReasoningOutputTokens = next.ReasoningOutputTokens
	}
	if next.TotalTokens != nil {
		current.TotalTokens = next.TotalTokens
	}
	if len(next.Raw) > 0 {
		current.Raw = next.Raw
	}
	return current
}
