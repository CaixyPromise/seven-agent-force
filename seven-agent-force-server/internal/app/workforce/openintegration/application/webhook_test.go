package application

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/domain"
	openinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/infrastructure"
	secretvalue "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
)

type webhookTestSecrets struct{}

func (webhookTestSecrets) EncryptString(_ context.Context, plain string) (secretvalue.SecretValue, error) {
	return secretvalue.SecretValue{Plain: plain}, nil
}
func (webhookTestSecrets) DecryptString(_ context.Context, value secretvalue.SecretValue) (string, error) {
	return value.Plain, nil
}
func (webhookTestSecrets) EncryptBytes(_ context.Context, plain []byte) (secretvalue.SecretValue, error) {
	return secretvalue.SecretValue{Plain: string(plain)}, nil
}
func (webhookTestSecrets) DecryptBytes(_ context.Context, value secretvalue.SecretValue) ([]byte, error) {
	return []byte(value.Plain), nil
}

type fixedWebhookClient struct{ client *http.Client }

func (f fixedWebhookClient) HTTPClient(string) (*http.Client, error) { return f.client, nil }

func TestWebhookSignatureUsesExactRawBody(t *testing.T) {
	body := []byte(`{"eventId":"evt_1","status":"SUCCEEDED"}`)
	credentialID := int64(9_007_199_254_740_993)
	signature := SignatureV2(credentialID, "whsec_fixture", "1788065721", body)
	mac := hmac.New(sha256.New, []byte("whsec_fixture"))
	_, _ = mac.Write([]byte("1788065721."))
	_, _ = mac.Write(body)
	want := "v2=9007199254740993." + hex.EncodeToString(mac.Sum(nil))
	if signature != want {
		t.Fatalf("SignatureV2()=%q want %q", signature, want)
	}
	mutated := append([]byte(nil), body...)
	mutated[len(mutated)-2] = 'x'
	if SignatureV2(credentialID, "whsec_fixture", "1788065721", mutated) == signature {
		t.Fatal("signature accepted a byte-changed body")
	}
}

func TestWebhookSignatureHeaderOrdersCurrentBeforeGrace(t *testing.T) {
	body := []byte(`{"eventId":"9007199254740993"}`)
	header := SignatureHeader(
		DecryptedSigningCredential{ID: 9_007_199_254_740_995, Secret: "whsec_current"},
		&DecryptedSigningCredential{ID: 9_007_199_254_740_994, Secret: "whsec_grace"},
		"1788065721",
		body,
	)
	parts := strings.Split(header, ",")
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "v2=9007199254740995.") || !strings.HasPrefix(parts[1], "v2=9007199254740994.") {
		t.Fatalf("signature order=%q", header)
	}
	if strings.Contains(header, "whsec_current") || strings.Contains(header, "whsec_grace") {
		t.Fatal("signature header exposed plaintext signing secret")
	}
}

func TestGroupClaimsByApplicationPreservesApplicationAndClaimOrder(t *testing.T) {
	claims := []openinfra.DeliveryClaim{
		{Delivery: domain.Delivery{ID: 11, ApplicationID: 101}},
		{Delivery: domain.Delivery{ID: 21, ApplicationID: 202}},
		{Delivery: domain.Delivery{ID: 12, ApplicationID: 101}},
		{Delivery: domain.Delivery{ID: 22, ApplicationID: 202}},
		{Delivery: domain.Delivery{ID: 31, ApplicationID: 303}},
	}
	groups := groupClaimsByApplication(claims)
	if len(groups) != 3 {
		t.Fatalf("group count=%d want 3", len(groups))
	}
	want := [][]int64{{11, 12}, {21, 22}, {31}}
	for groupIndex := range want {
		if len(groups[groupIndex]) != len(want[groupIndex]) {
			t.Fatalf("group %d length=%d want %d", groupIndex, len(groups[groupIndex]), len(want[groupIndex]))
		}
		for claimIndex := range want[groupIndex] {
			if groups[groupIndex][claimIndex].Delivery.ID != want[groupIndex][claimIndex] {
				t.Fatalf("group %d claim %d ID=%d want %d", groupIndex, claimIndex, groups[groupIndex][claimIndex].Delivery.ID, want[groupIndex][claimIndex])
			}
		}
	}
}

func TestProcessClaimGroupsSerializesOneApplicationAndRunsApplicationsConcurrently(t *testing.T) {
	groups := groupClaimsByApplication([]openinfra.DeliveryClaim{
		{Delivery: domain.Delivery{ID: 11, ApplicationID: 101}},
		{Delivery: domain.Delivery{ID: 21, ApplicationID: 202}},
		{Delivery: domain.Delivery{ID: 12, ApplicationID: 101}},
	})
	entered := make(chan int64, 3)
	releaseFirst := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- processClaimGroups(context.Background(), groups, func(_ context.Context, claim openinfra.DeliveryClaim) error {
			entered <- claim.Delivery.ID
			if claim.Delivery.ID == 11 {
				<-releaseFirst
			}
			return nil
		})
	}()

	firstTwo := map[int64]bool{}
	for range 2 {
		select {
		case deliveryID := <-entered:
			firstTwo[deliveryID] = true
		case <-time.After(time.Second):
			t.Fatal("different Application groups did not enter concurrently")
		}
	}
	if !firstTwo[11] || !firstTwo[21] {
		t.Fatalf("first concurrent deliveries=%v want Application 101/202 heads", firstTwo)
	}
	select {
	case deliveryID := <-entered:
		t.Fatalf("same Application delivery %d entered before its predecessor completed", deliveryID)
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseFirst)
	select {
	case deliveryID := <-entered:
		if deliveryID != 12 {
			t.Fatalf("next same-Application delivery=%d want 12", deliveryID)
		}
	case <-time.After(time.Second):
		t.Fatal("same Application successor did not resume")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("processClaimGroups() error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("processClaimGroups() did not finish")
	}
}

func TestDeliveryFailureScheduleAndExhaustion(t *testing.T) {
	now := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	wants := []time.Duration{10 * time.Second, time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour}
	for attempt, want := range wants {
		outcome := DeliveryFailureOutcome(attempt+1, "NETWORK_ERROR", nil, time.Second, now, 0)
		if outcome.Status != "RETRY_WAIT" || outcome.NextAttemptAt == nil || outcome.NextAttemptAt.Sub(now) != want {
			t.Fatalf("attempt %d outcome=%+v want delay %s", attempt+1, outcome, want)
		}
	}
	exhausted := DeliveryFailureOutcome(6, "NETWORK_ERROR", nil, time.Second, now, 0)
	if exhausted.Status != "EXHAUSTED" || exhausted.NextAttemptAt != nil {
		t.Fatalf("sixth failure=%+v", exhausted)
	}
	bounded := DeliveryFailureOutcome(1, "HTTP_RETRYABLE", nil, time.Second, now, 90*time.Second)
	if bounded.NextAttemptAt == nil || bounded.NextAttemptAt.Sub(now) != 90*time.Second {
		t.Fatalf("bounded Retry-After not honored: %+v", bounded)
	}
	unbounded := DeliveryFailureOutcome(1, "HTTP_RETRYABLE", nil, time.Second, now, 3*time.Hour)
	if unbounded.NextAttemptAt == nil || unbounded.NextAttemptAt.Sub(now) != 10*time.Second {
		t.Fatalf("unbounded Retry-After was not ignored: %+v", unbounded)
	}
}

func TestBoundedRetryAfter(t *testing.T) {
	now := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	if got := boundedRetryAfter("120", now); got != 2*time.Minute {
		t.Fatalf("Retry-After seconds=%s", got)
	}
	if got := boundedRetryAfter("10800", now); got != 0 {
		t.Fatalf("oversized Retry-After=%s", got)
	}
}

func TestWebhookDeliveryCarriesStableIdentityAndValidSignature(t *testing.T) {
	body := []byte(`{"schema":"seven.workforce.public-event/v1","eventId":"evt_1"}`)
	var received bool
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		if r.Header.Get("X-Seven-Event-ID") != "4002" || r.Header.Get("X-Seven-Delivery-ID") != "4001" {
			t.Errorf("unstable webhook identity headers: %#v", r.Header)
		}
		timestamp := r.Header.Get("X-Seven-Timestamp")
		if r.Header.Get("X-Seven-Signature") != SignatureV2(9_007_199_254_740_993, "whsec_fixture", timestamp, body) {
			t.Error("webhook signature did not cover exact body")
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer receiver.Close()
	now := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	worker := NewWebhookWorker(nil, webhookTestSecrets{}, fixedWebhookClient{client: receiver.Client()}, nil, 1, time.Minute, time.Second, time.Second)
	worker.now = func() time.Time { return now }
	outcome := worker.deliver(context.Background(), openinfra.DeliveryClaim{
		Delivery: domain.Delivery{ID: 4001, EventID: 4002, AttemptCount: 1, Payload: body, WebhookURL: receiver.URL},
		CurrentSigningCredential: openinfra.EncryptedSigningCredential{
			ID: 9_007_199_254_740_993, Secret: secretvalue.SecretValue{Plain: "whsec_fixture"},
		},
	})
	if !received || outcome.Status != "RETRY_WAIT" || outcome.HTTPStatus == nil || *outcome.HTTPStatus != http.StatusInternalServerError || !outcome.SigningCredentialsUsed {
		t.Fatalf("delivery outcome=%+v received=%v", outcome, received)
	}
}

func TestWebhookDeliveryDoubleSignsWithCurrentThenGrace(t *testing.T) {
	body := []byte(`{"schema":"seven.workforce.public-event/v1","eventId":"9007199254740993"}`)
	var signature string
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		signature = r.Header.Get("X-Seven-Signature")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()
	now := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	worker := NewWebhookWorker(nil, webhookTestSecrets{}, fixedWebhookClient{client: receiver.Client()}, nil, 1, time.Minute, time.Second, time.Second)
	worker.now = func() time.Time { return now }
	graceID := int64(9_007_199_254_740_994)
	outcome := worker.deliver(context.Background(), openinfra.DeliveryClaim{
		Delivery: domain.Delivery{ID: 4001, EventID: 4002, AttemptCount: 1, Payload: body, WebhookURL: receiver.URL},
		CurrentSigningCredential: openinfra.EncryptedSigningCredential{
			ID: 9_007_199_254_740_995, Secret: secretvalue.SecretValue{Plain: "whsec_current"},
		},
		GraceSigningCredential: &openinfra.EncryptedSigningCredential{
			ID: graceID, Secret: secretvalue.SecretValue{Plain: "whsec_grace"},
		},
	})
	want := SignatureV2(9_007_199_254_740_995, "whsec_current", strconv.FormatInt(now.Unix(), 10), body) + "," +
		SignatureV2(graceID, "whsec_grace", strconv.FormatInt(now.Unix(), 10), body)
	if outcome.Status != "DELIVERED" || !outcome.SigningCredentialsUsed || signature != want {
		t.Fatalf("outcome=%+v signature=%q want=%q", outcome, signature, want)
	}
}
