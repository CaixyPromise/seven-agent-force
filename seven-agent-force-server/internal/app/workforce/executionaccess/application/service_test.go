package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/facade"
)

func TestToolResultRetainsAdapterCorrelationAndSafeReason(t *testing.T) {
	o := domain.Operation{ID: 91, RoundKey: "round-1", LocalToolCallKey: "call-1", NativeToolCallID: "item_tool_call_01HXYZ", AdapterSessionGeneration: 7, Status: domain.StatusOutcomeUnknown, ResultSummary: "result file unavailable"}
	result, err := (&Service{}).toolResult(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if result.OperationID != 91 || result.RoundKey != "round-1" || result.LocalToolCallKey != "call-1" || result.NativeToolCallID != "item_tool_call_01HXYZ" || result.AdapterSessionGeneration != 7 || result.ReasonCode != "OPERATION_RESULT_NOT_RECOVERABLE" {
		t.Fatalf("unexpected tool result: %#v", result)
	}
}

func TestPublishedOccurrenceCannotReplaceNativeToolCallIdentity(t *testing.T) {
	target := facade.Target{TenantID: 1, TaskID: 2}
	request := facade.PublishRequest{RoundKey: "round-1", LocalToolCallKey: "call-1", NativeToolCallID: "item_tool_call_new"}
	operation := domain.Operation{TenantID: 1, TaskID: 2, RoundKey: "round-1", LocalToolCallKey: "call-1", NativeToolCallID: "item_tool_call_original", Kind: domain.KindNetworkCommand, DescriptorDigest: "digest"}
	if publishedOccurrenceMatches(operation, target, request, "digest") {
		t.Fatal("same occurrence accepted a replacement native tool call identity")
	}
	request.NativeToolCallID = operation.NativeToolCallID
	if !publishedOccurrenceMatches(operation, target, request, "digest") {
		t.Fatal("matching occurrence identity was rejected")
	}
}

func TestSafeCollapsesDependencyDiagnostics(t *testing.T) {
	if got := safe(errors.New("sql password=secret")); !errors.Is(got, domain.ErrUnavailable) || got.Error() != domain.ErrUnavailable.Error() {
		t.Fatalf("unsafe error: %v", got)
	}
	if got := safe(domain.ErrStale); !errors.Is(got, domain.ErrStale) {
		t.Fatalf("stable error lost: %v", got)
	}
}

func TestServiceFailsClosedWhenUnwired(t *testing.T) {
	var service *Service
	if service.configured() {
		t.Fatal("nil service reported configured")
	}
}

func TestAttemptAuthorityMatchesEveryFrozenExecutionIdentity(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	service := &Service{now: func() time.Time { return now }}
	machine := facade.Machine{
		WorkerID: 41, InstanceID: "worker-instance", HostBootID: "host-boot",
		OwnerEpoch: 7, ExecutionLease: "execution-lease",
	}
	target := facade.Target{TenantID: 11, TaskID: 21, WorkerID: 41, OwnerEpoch: 7}
	attempt := domain.Attempt{
		TenantID: 11, TaskID: 21, WorkerID: 41, WorkerInstanceID: "worker-instance",
		HostBootID: "host-boot", OwnerEpoch: 7, LeaseDigest: domain.Digest("execution-lease"),
		LeaseExpiresAt: now.Add(time.Minute),
	}
	if !service.attemptAuthorityMatches(machine, target, attempt) {
		t.Fatal("matching operation attempt authority was rejected")
	}

	tests := map[string]func(*domain.Attempt){
		"worker":          func(a *domain.Attempt) { a.WorkerID++ },
		"instance":        func(a *domain.Attempt) { a.WorkerInstanceID = "old-instance" },
		"host boot":       func(a *domain.Attempt) { a.HostBootID = "old-boot" },
		"owner epoch":     func(a *domain.Attempt) { a.OwnerEpoch-- },
		"execution lease": func(a *domain.Attempt) { a.LeaseDigest = domain.Digest("old-lease") },
		"lease expiry":    func(a *domain.Attempt) { a.LeaseExpiresAt = now },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			copy := attempt
			mutate(&copy)
			if service.attemptAuthorityMatches(machine, target, copy) {
				t.Fatal("stale operation attempt authority was accepted")
			}
		})
	}
}

func TestReadResultReceiptRequiresCanonicalDigest(t *testing.T) {
	service := &Service{}
	for _, digest := range []string{"", "short", strings.Repeat("g", 64), strings.ToUpper(domain.Digest("result"))} {
		if _, err := service.ReadResultReceipt(context.Background(), facade.Machine{}, facade.ResultReceiptRequest{OperationID: 1, AttemptID: 2, ResultDigest: digest}); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("digest %q returned %v, want invalid", digest, err)
		}
	}
}
