package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewcontract"
)

func validIdentity(kind ExecutionKind) RequestIdentity {
	identity := RequestIdentity{
		ExecutionKind: kind, WorkerID: 11, InstanceID: "worker-instance", HostBootID: "boot",
		TaskID: 12, OwnerEpoch: 2, LeaseToken: "lease", ProtocolInvocationKey: "invocation", Nonce: "nonce",
		BodySHA256: strings.Repeat("a", 64), Timestamp: time.Unix(10, 0).UTC(),
	}
	if kind == ExecutionKindReviewRun {
		identity.ReviewRunID = 13
		identity.ReviewRunAttemptID = 14
		identity.StructuredOutputContract = reviewcontract.StructuredOutputContractV1
	} else {
		identity.AttemptID = 15
		identity.Generation = 1
	}
	return identity
}

func TestRequestIdentityKeepsAgentCompatibilityAndSeparatesReviewRunShape(t *testing.T) {
	agent := validIdentity("")
	if err := agent.Validate(); err != nil {
		t.Fatalf("legacy Agent identity rejected: %v", err)
	}
	if agent.ExecutionKind.Normalized() != ExecutionKindAgentAttempt {
		t.Fatalf("empty execution kind did not normalize to Agent Attempt")
	}

	review := validIdentity(ExecutionKindReviewRun)
	if err := review.Validate(); err != nil {
		t.Fatalf("ReviewRun identity rejected: %v", err)
	}
	for _, mutate := range []func(*RequestIdentity){
		func(value *RequestIdentity) { value.AttemptID = 15 },
		func(value *RequestIdentity) { value.Generation = 1 },
		func(value *RequestIdentity) { value.ReviewRunID = 0 },
		func(value *RequestIdentity) { value.ReviewRunAttemptID = 0 },
	} {
		mixed := review
		mutate(&mixed)
		if err := mixed.Validate(); err == nil || Reason(err) != "MODEL_REVIEW_RUN_IDENTITY_MIXED" {
			t.Fatalf("mixed ReviewRun identity error=%v, want MODEL_REVIEW_RUN_IDENTITY_MIXED", err)
		}
	}
}

func TestReviewRunIdentityDoesNotUseModelCapabilityContract(t *testing.T) {
	identity := validIdentity(ExecutionKindReviewRun)
	identity.StructuredOutputContract = reviewcontract.ReportSchemaVersion
	if err := identity.Validate(); err != nil {
		t.Fatalf("model invocation identity should not enforce review output capability: %v", err)
	}
}

func TestRequestIdentityRequiresReviewRunFieldsAsAGroup(t *testing.T) {
	for _, mutate := range []func(*RequestIdentity){
		func(value *RequestIdentity) { value.ReviewRunID = 0 },
		func(value *RequestIdentity) { value.ReviewRunAttemptID = 0 },
	} {
		identity := validIdentity(ExecutionKindReviewRun)
		mutate(&identity)
		if err := identity.Validate(); err == nil || Reason(err) != "MODEL_REVIEW_RUN_IDENTITY_MIXED" {
			t.Fatalf("incomplete ReviewRun tuple error=%v, want stable mixed-identity error", err)
		}
	}
}
