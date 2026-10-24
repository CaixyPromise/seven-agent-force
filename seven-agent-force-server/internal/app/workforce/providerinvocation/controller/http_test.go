package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	invocationdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/providerinvocation/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewcontract"
	"github.com/cloudwego/hertz/pkg/app"
)

func TestReadIdentityAcceptsProxyHeadersAndNormalizesBodyDigest(t *testing.T) {
	context := &app.RequestContext{}
	headers := map[string]string{
		"Authorization": "Bearer " + strings.Repeat("t", 32), "X-Seven-Worker-ID": "101",
		"X-Seven-Worker-Instance": "wki_1", "X-Seven-Host-Boot-ID": "boot_1", "X-Seven-Task-ID": "102",
		"X-Seven-Attempt-ID": "103", "X-Seven-Generation": "1", "X-Seven-Owner-Epoch": "2",
		"X-Seven-Attempt-Lease": strings.Repeat("l", 32), "X-Seven-Invocation-ID": "pinv_1",
		"X-Seven-Nonce": strings.Repeat("n", 32), "X-Seven-Timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"X-Seven-Body-SHA256": "sha256:" + strings.Repeat("a", 64),
	}
	for name, value := range headers {
		context.Request.Header.Set(name, value)
	}
	identity, token, err := readIdentity(context)
	if err != nil {
		t.Fatal(err)
	}
	if token != strings.Repeat("t", 32) || identity.BodySHA256 != strings.Repeat("a", 64) || identity.OwnerEpoch != 2 {
		t.Fatalf("unexpected normalized identity: token=%q identity=%+v", token, identity)
	}
}

func TestReadIdentityRejectsMalformedProxyDigest(t *testing.T) {
	context := &app.RequestContext{}
	for name, value := range map[string]string{
		"Authorization": "Bearer " + strings.Repeat("t", 32), "X-Seven-Worker-ID": "101",
		"X-Seven-Worker-Instance": "wki_1", "X-Seven-Host-Boot-ID": "boot_1", "X-Seven-Task-ID": "102",
		"X-Seven-Attempt-ID": "103", "X-Seven-Generation": "1", "X-Seven-Owner-Epoch": "1",
		"X-Seven-Attempt-Lease": strings.Repeat("l", 32), "X-Seven-Invocation-ID": "pinv_1",
		"X-Seven-Nonce": strings.Repeat("n", 32), "X-Seven-Timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"X-Seven-Body-SHA256": "sha256:" + strings.Repeat("z", 64),
	} {
		context.Request.Header.Set(name, value)
	}
	if _, _, err := readIdentity(context); err == nil {
		t.Fatal("malformed digest was accepted")
	}
}

func TestReadIdentityAcceptsMachineOnlyReviewRunTuple(t *testing.T) {
	context := &app.RequestContext{}
	body := []byte(`{"model":"gpt-test"}`)
	digest := sha256.Sum256(body)
	for name, value := range map[string]string{
		"Authorization": "Bearer " + strings.Repeat("t", 32), "X-Seven-Execution-Kind": "REVIEW_RUN",
		"X-Seven-Worker-ID": "101", "X-Seven-Worker-Instance": "wki_1", "X-Seven-Host-Boot-ID": "boot_1",
		"X-Seven-Task-ID": "102", "X-Seven-Review-Run-ID": "103", "X-Seven-Review-Run-Attempt-ID": "104",
		"X-Seven-Owner-Epoch": "2", "X-Seven-Review-Run-Lease": strings.Repeat("l", 32), "X-Seven-Invocation-ID": "pinv_review",
		"X-Seven-Review-Contract": reviewcontract.StructuredOutputContractV1,
		"X-Seven-Nonce":           strings.Repeat("n", 32), "X-Seven-Timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"X-Seven-Body-SHA256": hex.EncodeToString(digest[:]),
	} {
		context.Request.Header.Set(name, value)
	}
	identity, _, err := readIdentity(context)
	if err != nil {
		t.Fatal(err)
	}
	if identity.ExecutionKind != invocationdomain.ExecutionKindReviewRun || identity.ReviewRunID != 103 || identity.ReviewRunAttemptID != 104 || identity.AttemptID != 0 || identity.Generation != 0 {
		t.Fatalf("unexpected ReviewRun identity: %+v", identity)
	}
}

func TestReadIdentityRejectsReviewRunWithOrdinaryAttemptHeaders(t *testing.T) {
	context := &app.RequestContext{}
	for name, value := range map[string]string{
		"Authorization": "Bearer " + strings.Repeat("t", 32), "X-Seven-Execution-Kind": "REVIEW_RUN",
		"X-Seven-Worker-ID": "101", "X-Seven-Worker-Instance": "wki_1", "X-Seven-Host-Boot-ID": "boot_1",
		"X-Seven-Task-ID": "102", "X-Seven-Review-Run-ID": "103", "X-Seven-Review-Run-Attempt-ID": "104",
		"X-Seven-Generation": "1", "X-Seven-Attempt-ID": "105", "X-Seven-Owner-Epoch": "2",
		"X-Seven-Review-Run-Lease": strings.Repeat("l", 32), "X-Seven-Invocation-ID": "pinv_review_mixed", "X-Seven-Review-Contract": reviewcontract.StructuredOutputContractV1,
		"X-Seven-Nonce": strings.Repeat("n", 32), "X-Seven-Timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"X-Seven-Body-SHA256": strings.Repeat("a", 64),
	} {
		context.Request.Header.Set(name, value)
	}
	if _, _, err := readIdentity(context); invocationdomain.Reason(err) != "PROVIDER_REVIEW_RUN_IDENTITY_MIXED" {
		t.Fatalf("mixed ReviewRun identity error=%v, want PROVIDER_REVIEW_RUN_IDENTITY_MIXED", err)
	}
}
