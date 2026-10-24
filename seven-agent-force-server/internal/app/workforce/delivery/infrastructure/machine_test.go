package infrastructure

import (
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
)

func TestReconciliationTransitionSchedulesSafeRetry(t *testing.T) {
	decision, attemptStatus, retryable := reconciliationTransition(domain.RemoteDecision{
		Status: "RETRY_WAIT", ReasonCode: "REMOTE_UNCHANGED", SafeToRetry: true,
	}, 2)
	if decision.Status != "RETRY_WAIT" || attemptStatus != "RETRY_WAIT" || !retryable {
		t.Fatalf("safe readback must remain schedulable: decision=%+v attempt=%s retryable=%v", decision, attemptStatus, retryable)
	}
}

func TestReconciliationTransitionExhaustsBoundedEpoch(t *testing.T) {
	decision, attemptStatus, retryable := reconciliationTransition(domain.RemoteDecision{
		Status: "RETRY_WAIT", ReasonCode: "REMOTE_UNCHANGED", SafeToRetry: true,
	}, 3)
	if decision.Status != "FAILED" || decision.ReasonCode != "DELIVERY_RETRY_EXHAUSTED" || attemptStatus != "FAILED" || retryable {
		t.Fatalf("exhausted readback must fail closed: decision=%+v attempt=%s retryable=%v", decision, attemptStatus, retryable)
	}
}

func TestReconciliationTransitionPreservesSuccess(t *testing.T) {
	decision, attemptStatus, retryable := reconciliationTransition(domain.RemoteDecision{
		Status: "SUCCEEDED", ReasonCode: "REMOTE_MATCHES_DELIVERY",
	}, 3)
	if decision.Status != "SUCCEEDED" || attemptStatus != "SUCCEEDED" || retryable {
		t.Fatalf("successful readback changed state: decision=%+v attempt=%s retryable=%v", decision, attemptStatus, retryable)
	}
}
