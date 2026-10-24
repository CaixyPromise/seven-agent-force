package proxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewcontract"
)

const defaultMaxRequestBytes = 128 << 20

const executionKindReviewRun = "REVIEW_RUN"

// Config is deliberately free of Provider URLs and Provider credentials. The
// proxy is a fixed Gateway client; only Framework Server may resolve and use a
// Provider secret.
type Config struct {
	// ExecutionKind is empty for the legacy Agent Attempt contract. REVIEW_RUN
	// selects the mutually-exclusive ReviewRun identity below; it never falls
	// back to Attempt fields.
	ExecutionKind                    string `json:"executionKind,omitempty"`
	GatewayBaseURL                   string `json:"gatewayBaseUrl"`
	WorkerTokenFile                  string `json:"workerTokenFile"`
	WorkerID                         int64  `json:"workerId,string"`
	WorkerInstanceID                 string `json:"workerInstanceId"`
	HostBootID                       string `json:"hostBootId"`
	TaskID                           int64  `json:"taskId,string"`
	AttemptID                        int64  `json:"attemptId,string"`
	Generation                       int64  `json:"generation,string"`
	OwnerEpoch                       int64  `json:"ownerEpoch,string"`
	AttemptLeaseToken                string `json:"attemptLeaseToken"`
	ReviewRunID                      int64  `json:"reviewRunId,string,omitempty"`
	ReviewRunAttemptID               int64  `json:"reviewRunAttemptId,string,omitempty"`
	ReviewLeaseFile                  string `json:"reviewLeaseFile,omitempty"`
	ModelCredentialID                int64  `json:"modelCredentialId,string"`
	CredentialVersion                int64  `json:"credentialVersion,string"`
	ModelProtocol                    string `json:"modelProtocol"`
	ProviderStructuredOutputContract string `json:"structuredOutputContract,omitempty"`
	PrimaryModel                     string `json:"primaryModel"`
	ExecutionDeadline                string `json:"executionDeadline"`
	LeaseExpiresAt                   string `json:"leaseExpiresAt,omitempty"`
	ControlSocket                    string `json:"controlSocket"`
	MaxRequestBytes                  int64  `json:"maxRequestBytes"`
	RequestTimeoutMS                 int64  `json:"requestTimeoutMillis"`
}

type AuthorityState struct {
	WorkerInstanceID        string `json:"workerInstanceId"`
	OwnerEpoch              int64  `json:"ownerEpoch,string"`
	AttemptLeaseToken       string `json:"-"`
	LeaseExpiresAt          string `json:"leaseExpiresAt,omitempty"`
	ReattachPermitID        int64  `json:"reattachPermitId,string,omitempty"`
	ReattachPermitDigest    string `json:"reattachPermitDigest,omitempty"`
	ReattachPayloadDigest   string `json:"-"`
	ReattachPermitExpiresAt string `json:"reattachPermitExpiresAt,omitempty"`
}

type Handler struct {
	config      Config
	gateway     *url.URL
	client      *http.Client
	workerToken string
	authority   struct {
		sync.RWMutex
		AuthorityState
	}
	active struct {
		sync.Mutex
		cancel map[string]context.CancelFunc
	}
}

func NewHandler(config Config, client *http.Client) (*Handler, error) {
	config.GatewayBaseURL = strings.TrimRight(strings.TrimSpace(config.GatewayBaseURL), "/")
	config.WorkerTokenFile = strings.TrimSpace(config.WorkerTokenFile)
	config.WorkerInstanceID = strings.TrimSpace(config.WorkerInstanceID)
	config.HostBootID = strings.TrimSpace(config.HostBootID)
	config.AttemptLeaseToken = strings.TrimSpace(config.AttemptLeaseToken)
	config.ExecutionKind = strings.TrimSpace(config.ExecutionKind)
	config.ReviewLeaseFile = strings.TrimSpace(config.ReviewLeaseFile)
	config.PrimaryModel = strings.TrimSpace(config.PrimaryModel)
	config.ModelProtocol = strings.ToUpper(strings.TrimSpace(config.ModelProtocol))
	config.ProviderStructuredOutputContract = strings.TrimSpace(config.ProviderStructuredOutputContract)
	gateway, err := url.Parse(config.GatewayBaseURL)
	if err != nil || (gateway.Scheme != "https" && gateway.Scheme != "http") || gateway.Host == "" || gateway.User != nil || gateway.RawQuery != "" || gateway.Fragment != "" {
		return nil, errors.New("Gateway base URL must be an absolute HTTP(S) URL without user info, query, or fragment")
	}
	if config.WorkerID <= 0 || config.WorkerInstanceID == "" || config.HostBootID == "" || config.TaskID <= 0 || config.OwnerEpoch <= 0 || config.ModelCredentialID <= 0 || config.CredentialVersion <= 0 || config.PrimaryModel == "" || (config.ModelProtocol != "OPENAI" && config.ModelProtocol != "ANTHROPIC") {
		return nil, errors.New("provider Gateway execution identity is incomplete")
	}
	reviewRun := config.ExecutionKind == executionKindReviewRun
	if config.ExecutionKind != "" && !reviewRun {
		return nil, errors.New("provider Gateway execution kind is invalid")
	}
	leaseExpiresAt := ""
	if reviewRun {
		if config.AttemptID != 0 || config.Generation != 0 || config.AttemptLeaseToken != "" || config.ReviewRunID <= 0 || config.ReviewRunAttemptID <= 0 || config.ReviewLeaseFile == "" || config.ProviderStructuredOutputContract != reviewcontract.StructuredOutputContractV1 {
			return nil, errors.New("ReviewRun Provider execution identity is mixed or incomplete")
		}
		lease, err := readWorkerToken(config.ReviewLeaseFile)
		if err != nil {
			return nil, fmt.Errorf("read ReviewRun lease file: %w", err)
		}
		config.AttemptLeaseToken = lease
		validatedExpiry, err := validateReviewLeaseExpiry(config.LeaseExpiresAt)
		if err != nil {
			return nil, errors.New("ReviewRun lease expiry is invalid")
		}
		leaseExpiresAt = validatedExpiry.Format(time.RFC3339Nano)
	} else if config.AttemptID <= 0 || config.Generation <= 0 || config.AttemptLeaseToken == "" || config.ReviewRunID != 0 || config.ReviewRunAttemptID != 0 || config.ReviewLeaseFile != "" {
		return nil, errors.New("provider Gateway Attempt execution identity is mixed or incomplete")
	}
	token, err := readWorkerToken(config.WorkerTokenFile)
	if err != nil {
		return nil, err
	}
	if config.MaxRequestBytes <= 0 {
		config.MaxRequestBytes = defaultMaxRequestBytes
	}
	if client == nil {
		timeout := 5 * time.Minute
		if config.RequestTimeoutMS > 0 {
			timeout = time.Duration(config.RequestTimeoutMS) * time.Millisecond
		}
		client = &http.Client{Timeout: timeout}
	}
	h := &Handler{config: config, gateway: gateway, client: client, workerToken: token}
	h.authority.AuthorityState = AuthorityState{WorkerInstanceID: config.WorkerInstanceID, OwnerEpoch: config.OwnerEpoch, AttemptLeaseToken: config.AttemptLeaseToken, LeaseExpiresAt: leaseExpiresAt}
	h.active.cancel = make(map[string]context.CancelFunc)
	return h, nil
}

func readWorkerToken(path string) (string, error) {
	if path == "" {
		return "", errors.New("worker token file is required")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("inspect worker token file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("worker token file must be a private regular file")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read worker token file: %w", err)
	}
	token := strings.TrimSpace(string(body))
	if len(token) < 32 || strings.ContainsAny(token, "\r\n\x00") {
		return "", errors.New("worker token file is invalid")
	}
	return token, nil
}

func (h *Handler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeError(response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
		return
	}
	suffix, ok := responseSuffix(request.URL.Path)
	if !ok {
		writeError(response, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if (h.config.ModelProtocol == "OPENAI" && suffix != "/responses" && suffix != "/responses/compact") ||
		(h.config.ModelProtocol == "ANTHROPIC" && suffix != "/messages" && suffix != "/messages/count_tokens") {
		writeError(response, http.StatusNotFound, "MODEL_OPERATION_NOT_SUPPORTED")
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, h.config.MaxRequestBytes+1))
	if err != nil || int64(len(body)) > h.config.MaxRequestBytes {
		writeError(response, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE")
		return
	}
	var envelope struct {
		Model      string `json:"model"`
		Background bool   `json:"background"`
		Stream     bool   `json:"stream"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		writeError(response, http.StatusBadRequest, "INVALID_JSON")
		return
	}
	if strings.TrimSpace(envelope.Model) == "" || len(strings.TrimSpace(envelope.Model)) > 256 {
		writeError(response, http.StatusBadRequest, "MODEL_REQUEST_MODEL_INVALID")
		return
	}
	if envelope.Background {
		writeError(response, http.StatusBadRequest, "BACKGROUND_NOT_ALLOWED")
		return
	}
	authority := h.Authority()
	if authority.LeaseExpiresAt != "" {
		expiresAt, parseErr := time.Parse(time.RFC3339Nano, authority.LeaseExpiresAt)
		if parseErr != nil || !expiresAt.After(time.Now()) {
			writeError(response, http.StatusConflict, "AUTHORITY_EXPIRED")
			return
		}
	}
	invocationID, nonce, err := newInvocationIdentity()
	if err != nil {
		writeError(response, http.StatusServiceUnavailable, "INVOCATION_IDENTITY_UNAVAILABLE")
		return
	}
	bodyDigest := sha256.Sum256(body)
	upstreamURL := *h.gateway
	upstreamURL.Path = strings.TrimRight(h.gateway.Path, "/") + "/api/internal/workforce/v1/model-invocations" + suffix
	upstreamContext, cancel := context.WithCancel(request.Context())
	h.active.Lock()
	h.active.cancel[invocationID] = cancel
	h.active.Unlock()
	defer func() {
		cancel()
		h.active.Lock()
		delete(h.active.cancel, invocationID)
		h.active.Unlock()
	}()
	upstreamRequest, err := http.NewRequestWithContext(upstreamContext, http.MethodPost, upstreamURL.String(), bytes.NewReader(body))
	if err != nil {
		writeError(response, http.StatusBadGateway, "GATEWAY_REQUEST_FAILED")
		return
	}
	upstreamRequest.Header.Set("Authorization", "Bearer "+h.workerToken)
	upstreamRequest.Header.Set("Content-Type", "application/json")
	upstreamRequest.Header.Set("Accept", request.Header.Get("Accept"))
	upstreamRequest.Header.Set("anthropic-version", request.Header.Get("anthropic-version"))
	upstreamRequest.Header.Set("anthropic-beta", request.Header.Get("anthropic-beta"))
	upstreamRequest.Header.Set("X-Seven-Worker-ID", strconv.FormatInt(h.config.WorkerID, 10))
	upstreamRequest.Header.Set("X-Seven-Worker-Instance", authority.WorkerInstanceID)
	upstreamRequest.Header.Set("X-Seven-Host-Boot-ID", h.config.HostBootID)
	upstreamRequest.Header.Set("X-Seven-Task-ID", strconv.FormatInt(h.config.TaskID, 10))
	upstreamRequest.Header.Set("X-Seven-Owner-Epoch", strconv.FormatInt(authority.OwnerEpoch, 10))
	if h.config.ExecutionKind == executionKindReviewRun {
		upstreamRequest.Header.Set("X-Seven-Execution-Kind", executionKindReviewRun)
		upstreamRequest.Header.Set("X-Seven-Review-Run-ID", strconv.FormatInt(h.config.ReviewRunID, 10))
		upstreamRequest.Header.Set("X-Seven-Review-Run-Attempt-ID", strconv.FormatInt(h.config.ReviewRunAttemptID, 10))
		upstreamRequest.Header.Set("X-Seven-Review-Run-Lease", authority.AttemptLeaseToken)
		upstreamRequest.Header.Set("X-Seven-Review-Contract", h.config.ProviderStructuredOutputContract)
	} else {
		upstreamRequest.Header.Set("X-Seven-Attempt-ID", strconv.FormatInt(h.config.AttemptID, 10))
		upstreamRequest.Header.Set("X-Seven-Generation", strconv.FormatInt(h.config.Generation, 10))
		upstreamRequest.Header.Set("X-Seven-Attempt-Lease", authority.AttemptLeaseToken)
	}
	upstreamRequest.Header.Set("X-Seven-Invocation-ID", invocationID)
	upstreamRequest.Header.Set("X-Seven-Timestamp", time.Now().UTC().Format(time.RFC3339Nano))
	upstreamRequest.Header.Set("X-Seven-Nonce", nonce)
	upstreamRequest.Header.Set("X-Seven-Body-SHA256", "sha256:"+hex.EncodeToString(bodyDigest[:]))
	client := h.client
	var headerTimer *time.Timer
	if envelope.Stream {
		// http.Client.Timeout covers the entire response body, including SSE.
		// Bound the header wait while allowing an active stream to continue.
		streamClient := *h.client
		headerTimeout := streamClient.Timeout
		if headerTimeout <= 0 {
			headerTimeout = 5 * time.Minute
		}
		headerTimer = time.AfterFunc(headerTimeout, cancel)
		streamClient.Timeout = 0
		client = &streamClient
	}
	upstreamResponse, err := client.Do(upstreamRequest)
	if headerTimer != nil {
		headerTimer.Stop()
	}
	if err != nil {
		writeError(response, http.StatusBadGateway, "GATEWAY_UNAVAILABLE")
		return
	}
	defer upstreamResponse.Body.Close()
	copyResponseHeaders(response.Header(), upstreamResponse.Header)
	response.Header().Set("X-Seven-Invocation-ID", invocationID)
	response.WriteHeader(upstreamResponse.StatusCode)
	_, _ = io.Copy(response, upstreamResponse.Body)
}

func (h *Handler) Authority() AuthorityState {
	h.authority.RLock()
	defer h.authority.RUnlock()
	return h.authority.AuthorityState
}

func (h *Handler) MatchesIdentity(attemptID int64, generation int64) bool {
	return attemptID == h.config.AttemptID && generation == h.config.Generation
}

func (h *Handler) AdoptOwner(expectedEpoch, newEpoch int64, workerInstanceID string, leaseToken string, leaseExpiresAt string, permitID int64, permitToken string, permitExpiresAt string) error {
	workerInstanceID = strings.TrimSpace(workerInstanceID)
	leaseToken = strings.TrimSpace(leaseToken)
	permitToken = strings.TrimSpace(permitToken)
	leaseExpiry, err := validateAuthorityExpiry(leaseExpiresAt)
	if err != nil {
		return errors.New("LEASE_INVALID")
	}
	permitExpiry, err := validateAuthorityExpiry(permitExpiresAt)
	if err != nil {
		return errors.New("REATTACH_PERMIT_EXPIRED")
	}
	if workerInstanceID == "" || leaseToken == "" || permitID <= 0 || len(permitToken) < 32 || newEpoch != expectedEpoch+1 {
		return errors.New("OWNER_EPOCH_CONFLICT")
	}
	permitSum := sha256.Sum256([]byte(permitToken))
	permitDigest := hex.EncodeToString(permitSum[:])
	payloadSum := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%d\x00%s\x00%s\x00%s\x00%d\x00%s\x00%s", expectedEpoch, newEpoch, workerInstanceID, leaseToken, leaseExpiry.Format(time.RFC3339Nano), permitID, permitToken, permitExpiry.Format(time.RFC3339Nano))))
	payloadDigest := hex.EncodeToString(payloadSum[:])
	h.authority.Lock()
	defer h.authority.Unlock()
	if h.authority.OwnerEpoch == newEpoch && h.authority.WorkerInstanceID == workerInstanceID && h.authority.ReattachPermitID == permitID && h.authority.ReattachPermitDigest == permitDigest {
		if h.authority.ReattachPayloadDigest == payloadDigest {
			return nil
		}
		return errors.New("REATTACH_PAYLOAD_CONFLICT")
	}
	if h.authority.ReattachPermitID != 0 {
		return errors.New("REATTACH_PAYLOAD_CONFLICT")
	}
	if h.authority.OwnerEpoch != expectedEpoch {
		return errors.New("OWNER_EPOCH_CONFLICT")
	}
	h.authority.AuthorityState = AuthorityState{WorkerInstanceID: workerInstanceID, OwnerEpoch: newEpoch, AttemptLeaseToken: leaseToken, LeaseExpiresAt: leaseExpiry.Format(time.RFC3339Nano), ReattachPermitID: permitID, ReattachPermitDigest: permitDigest, ReattachPayloadDigest: payloadDigest, ReattachPermitExpiresAt: permitExpiry.Format(time.RFC3339Nano)}
	return nil
}

func (h *Handler) RenewAuthority(ownerEpoch int64, leaseToken, leaseExpiresAt string) error {
	leaseToken = strings.TrimSpace(leaseToken)
	validator := validateAuthorityExpiry
	if h.config.ExecutionKind == executionKindReviewRun {
		validator = validateReviewLeaseExpiry
	}
	expiresAt, err := validator(leaseExpiresAt)
	if leaseToken == "" || err != nil {
		return errors.New("LEASE_INVALID")
	}
	h.authority.Lock()
	defer h.authority.Unlock()
	if h.authority.OwnerEpoch != ownerEpoch {
		return errors.New("OWNER_EPOCH_CONFLICT")
	}
	if h.authority.LeaseExpiresAt != "" {
		current, parseErr := time.Parse(time.RFC3339Nano, h.authority.LeaseExpiresAt)
		if parseErr != nil {
			return errors.New("LEASE_INVALID")
		}
		if expiresAt.Equal(current) && leaseToken == h.authority.AttemptLeaseToken {
			return nil
		}
		if !expiresAt.After(current) {
			return errors.New("LEASE_NOT_MONOTONIC")
		}
	}
	h.authority.AttemptLeaseToken = leaseToken
	h.authority.LeaseExpiresAt = expiresAt.Format(time.RFC3339Nano)
	return nil
}

func validateAuthorityExpiry(value string) (time.Time, error) {
	return validateAuthorityExpiryWithin(value, 2*time.Minute)
}

func validateReviewLeaseExpiry(value string) (time.Time, error) {
	// ReviewRun leases are issued by the Server for five minutes, while the
	// reattach permit remains a shorter two-minute capability. Keep the wider
	// bound specific to the ReviewRun lease path instead of weakening permit
	// validation for legacy Agent proxy control.
	return validateAuthorityExpiryWithin(value, 10*time.Minute)
}

func validateAuthorityExpiryWithin(value string, maximum time.Duration) (time.Time, error) {
	expiry, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	now := time.Now().UTC()
	if err != nil || !expiry.After(now) || expiry.After(now.Add(maximum)) {
		return time.Time{}, errors.New("authority expiry is invalid")
	}
	return expiry.UTC(), nil
}

func (h *Handler) Interrupt() {
	h.active.Lock()
	defer h.active.Unlock()
	for _, cancel := range h.active.cancel {
		cancel()
	}
}

func newInvocationIdentity() (string, string, error) {
	invocation := make([]byte, 16)
	nonce := make([]byte, 32)
	if _, err := rand.Read(invocation); err != nil {
		return "", "", err
	}
	if _, err := rand.Read(nonce); err != nil {
		return "", "", err
	}
	return "pinv_" + hex.EncodeToString(invocation), hex.EncodeToString(nonce), nil
}

func responseSuffix(path string) (string, bool) {
	switch strings.TrimRight(path, "/") {
	case "/responses", "/v1/responses":
		return "/responses", true
	case "/responses/compact", "/v1/responses/compact":
		return "/responses/compact", true
	case "/messages", "/v1/messages":
		return "/messages", true
	case "/messages/count_tokens", "/v1/messages/count_tokens":
		return "/messages/count_tokens", true
	default:
		return "", false
	}
}

func copyResponseHeaders(target, source http.Header) {
	for key, values := range source {
		lower := strings.ToLower(key)
		if lower != "content-type" && lower != "x-request-id" && lower != "retry-after" && !strings.HasPrefix(lower, "x-ratelimit-") && !strings.HasPrefix(lower, "openai-") && !strings.HasPrefix(lower, "anthropic-") {
			continue
		}
		for _, value := range values {
			target.Add(key, value)
		}
	}
	target.Set("Cache-Control", "no-store")
}

func writeError(response http.ResponseWriter, status int, code string) {
	response.Header().Set("Content-Type", "application/json")
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(map[string]any{
		"error": map[string]any{"type": "seven_provider_proxy_error", "code": code, "message": http.StatusText(status)},
	})
}
