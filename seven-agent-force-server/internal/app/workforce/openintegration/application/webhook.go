package application

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	openinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/infrastructure"
	secretvalue "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
	"go.uber.org/zap"
)

type WebhookHTTPClientFactory interface {
	HTTPClient(string) (*http.Client, error)
}

type WebhookWorker struct {
	store     *openinfra.Store
	secrets   secretvalue.Service
	clients   WebhookHTTPClientFactory
	logger    *zap.Logger
	batchSize int
	lease     time.Duration
	timeout   time.Duration
	interval  time.Duration
	now       func() time.Time
	owner     string
}

func NewWebhookWorker(store *openinfra.Store, secrets secretvalue.Service, clients WebhookHTTPClientFactory, logger *zap.Logger, batchSize int, lease, timeout, interval time.Duration) *WebhookWorker {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &WebhookWorker{store: store, secrets: secrets, clients: clients, logger: logger, batchSize: batchSize, lease: lease, timeout: timeout, interval: interval, now: time.Now, owner: newLeaseOwner()}
}

func newLeaseOwner() string {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return fmt.Sprintf("webhook-process-%d", time.Now().UnixNano())
	}
	return "webhook-process-" + hex.EncodeToString(random[:])
}

func (w *WebhookWorker) Run(ctx context.Context) {
	if w.interval <= 0 {
		w.interval = time.Second
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		if _, err := w.ProcessOnce(ctx); err != nil && ctx.Err() == nil {
			w.logger.Warn("process Open Task webhook deliveries", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *WebhookWorker) ProcessOnce(ctx context.Context) (int, error) {
	claims, err := w.store.ClaimDeliveries(ctx, w.owner, w.batchSize, w.lease)
	if err != nil {
		return 0, err
	}
	groups := groupClaimsByApplication(claims)
	if err := processClaimGroups(ctx, groups, w.processClaim); err != nil {
		return len(claims), err
	}
	return len(claims), nil
}

func processClaimGroups(ctx context.Context, groups [][]openinfra.DeliveryClaim, process func(context.Context, openinfra.DeliveryClaim) error) error {
	totalClaims := 0
	for _, group := range groups {
		totalClaims += len(group)
	}
	var wg sync.WaitGroup
	errorsCh := make(chan error, totalClaims)
	for _, group := range groups {
		group := group
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, claim := range group {
				if processErr := process(ctx, claim); processErr != nil {
					errorsCh <- processErr
				}
			}
		}()
	}
	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		return err
	}
	return nil
}

func groupClaimsByApplication(claims []openinfra.DeliveryClaim) [][]openinfra.DeliveryClaim {
	groups := make([][]openinfra.DeliveryClaim, 0, len(claims))
	indexes := make(map[int64]int, len(claims))
	for _, claim := range claims {
		index, exists := indexes[claim.Delivery.ApplicationID]
		if !exists {
			index = len(groups)
			indexes[claim.Delivery.ApplicationID] = index
			groups = append(groups, nil)
		}
		groups[index] = append(groups[index], claim)
	}
	return groups
}

func (w *WebhookWorker) processClaim(ctx context.Context, claim openinfra.DeliveryClaim) (resultErr error) {
	credentialLease, err := w.store.AcquireSigningCredentialLease(ctx, claim.Delivery.TenantID, claim.Delivery.ApplicationID)
	if err != nil {
		return err
	}
	defer func() {
		if releaseErr := credentialLease.Release(); releaseErr != nil {
			resultErr = errors.Join(resultErr, releaseErr)
		}
	}()
	claim.CurrentSigningCredential = credentialLease.Current
	claim.GraceSigningCredential = credentialLease.Grace
	claim.Delivery.SigningCredentialID = credentialLease.Current.ID
	claim.Delivery.GraceSigningCredentialID = nil
	if credentialLease.Grace != nil {
		graceID := credentialLease.Grace.ID
		claim.Delivery.GraceSigningCredentialID = &graceID
	}
	if err := w.store.RenewDeliveryLease(ctx, credentialLease, claim.Delivery, w.owner, w.lease); err != nil {
		return err
	}
	outcome := w.deliver(ctx, claim)
	return w.store.FinishDeliveryWithLease(ctx, credentialLease, claim.Delivery, w.owner, outcome)
}

func (w *WebhookWorker) deliver(parent context.Context, claim openinfra.DeliveryClaim) openinfra.DeliveryOutcome {
	started := w.now().UTC()
	signingCredentialsUsed := false
	fail := func(class string, status *int, retryAfter time.Duration) openinfra.DeliveryOutcome {
		outcome := DeliveryFailureOutcome(claim.Delivery.AttemptCount, class, status, w.now().UTC().Sub(started), started, retryAfter)
		outcome.SigningCredentialsUsed = signingCredentialsUsed
		return outcome
	}
	if len(claim.Delivery.Payload) == 0 || len(claim.Delivery.Payload) > 64<<10 {
		return fail("PAYLOAD_INVALID", nil, 0)
	}
	if claim.CurrentSigningCredential.ID <= 0 {
		return fail("SIGNING_SECRET_UNAVAILABLE", nil, 0)
	}
	currentSecret, err := w.secrets.DecryptString(parent, claim.CurrentSigningCredential.Secret)
	if err != nil || currentSecret == "" {
		return fail("SIGNING_SECRET_UNAVAILABLE", nil, 0)
	}
	current := DecryptedSigningCredential{ID: claim.CurrentSigningCredential.ID, Secret: currentSecret}
	var grace *DecryptedSigningCredential
	if claim.GraceSigningCredential != nil {
		if claim.GraceSigningCredential.ID <= 0 || claim.GraceSigningCredential.ID == current.ID {
			return fail("SIGNING_SECRET_UNAVAILABLE", nil, 0)
		}
		graceSecret, decryptErr := w.secrets.DecryptString(parent, claim.GraceSigningCredential.Secret)
		if decryptErr != nil || graceSecret == "" {
			return fail("SIGNING_SECRET_UNAVAILABLE", nil, 0)
		}
		grace = &DecryptedSigningCredential{ID: claim.GraceSigningCredential.ID, Secret: graceSecret}
	}
	client, err := w.clients.HTTPClient(claim.Delivery.WebhookURL)
	if err != nil {
		return fail("DESTINATION_DENIED", nil, 0)
	}
	timeout := w.timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	timestamp := strconv.FormatInt(w.now().UTC().Unix(), 10)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, claim.Delivery.WebhookURL, bytes.NewReader(claim.Delivery.Payload))
	if err != nil {
		return fail("REQUEST_INVALID", nil, 0)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Seven-Event-ID", strconv.FormatInt(claim.Delivery.EventID, 10))
	request.Header.Set("X-Seven-Delivery-ID", strconv.FormatInt(claim.Delivery.ID, 10))
	request.Header.Set("X-Seven-Timestamp", timestamp)
	request.Header.Set("X-Seven-Signature", SignatureHeader(current, grace, timestamp, claim.Delivery.Payload))
	signingCredentialsUsed = true
	response, err := client.Do(request)
	if err != nil {
		return fail("NETWORK_ERROR", nil, 0)
	}
	defer response.Body.Close()
	_, _ = io.CopyN(io.Discard, response.Body, 1024)
	status := response.StatusCode
	if status >= 200 && status < 300 {
		return openinfra.DeliveryOutcome{Status: "DELIVERED", HTTPStatus: &status, Duration: w.now().UTC().Sub(started), SigningCredentialsUsed: true}
	}
	retryAfter := boundedRetryAfter(response.Header.Get("Retry-After"), w.now().UTC())
	if status == http.StatusRequestTimeout || status == http.StatusTooEarly || status == http.StatusTooManyRequests || status >= 500 {
		return fail("HTTP_RETRYABLE", &status, retryAfter)
	}
	return openinfra.DeliveryOutcome{Status: "EXHAUSTED", FailureClass: "HTTP_PERMANENT", HTTPStatus: &status, Duration: w.now().UTC().Sub(started), SigningCredentialsUsed: true}
}

type DecryptedSigningCredential struct {
	ID     int64
	Secret string
}

func SignatureV2(credentialID int64, secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return "v2=" + strconv.FormatInt(credentialID, 10) + "." + hex.EncodeToString(mac.Sum(nil))
}

func SignatureHeader(current DecryptedSigningCredential, grace *DecryptedSigningCredential, timestamp string, body []byte) string {
	header := SignatureV2(current.ID, current.Secret, timestamp, body)
	if grace != nil {
		header += "," + SignatureV2(grace.ID, grace.Secret, timestamp, body)
	}
	return header
}

func DeliveryFailureOutcome(attempt int, class string, httpStatus *int, duration time.Duration, now time.Time, retryAfter time.Duration) openinfra.DeliveryOutcome {
	if attempt >= 6 {
		return openinfra.DeliveryOutcome{Status: "EXHAUSTED", FailureClass: class, HTTPStatus: httpStatus, Duration: duration}
	}
	delays := []time.Duration{10 * time.Second, time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour}
	delay := delays[attempt-1]
	if retryAfter > 0 && retryAfter <= 2*time.Hour {
		delay = retryAfter
	}
	next := now.Add(delay)
	return openinfra.DeliveryOutcome{Status: "RETRY_WAIT", FailureClass: class, HTTPStatus: httpStatus, Duration: duration, NextAttemptAt: &next}
}

func boundedRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		delay := time.Duration(seconds) * time.Second
		if delay > 0 && delay <= 2*time.Hour {
			return delay
		}
		return 0
	}
	parsed, err := http.ParseTime(value)
	if err != nil {
		return 0
	}
	delay := parsed.Sub(now)
	if delay <= 0 || delay > 2*time.Hour {
		return 0
	}
	return delay
}

type Projector struct {
	store    *openinfra.Store
	interval time.Duration
	logger   *zap.Logger
}

func NewProjector(store *openinfra.Store, interval time.Duration, logger *zap.Logger) *Projector {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Projector{store: store, interval: interval, logger: logger}
}

func (p *Projector) Run(ctx context.Context) {
	if p.interval <= 0 {
		p.interval = time.Second
	}
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		if _, err := p.store.ProjectTerminalEvents(ctx, 32); err != nil && ctx.Err() == nil {
			p.logger.Warn("project Open Task terminal events", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *WebhookWorker) String() string { return fmt.Sprintf("WebhookWorker(%s)", w.owner) }
