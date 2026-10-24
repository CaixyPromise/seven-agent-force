package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDefaultSubscriptionUsesCreatorInboxAndLosslessIDs(t *testing.T) {
	item, err := NewSubscription(9007199254740993, 9007199254740995, 9007199254740997, DefaultSubscriptionCommand())
	if err != nil {
		t.Fatal(err)
	}
	item.ID = 9007199254740999
	body, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, expected := range []string{`"id":"9007199254740999"`, `"tenantId":"9007199254740993"`, `"taskId":"9007199254740995"`, `"subscriberUserId":"9007199254740997"`, `"kind":"IN_APP"`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("subscription JSON %s misses %s", text, expected)
		}
	}
	if !item.Enabled || len(item.Events) != 4 || len(item.Channels) != 1 {
		t.Fatalf("unexpected default subscription: %+v", item)
	}
}

func TestExplicitDisabledSubscriptionStillFreezesDefaultPolicy(t *testing.T) {
	item, err := NewSubscription(1, 2, 3, SubscriptionCommand{Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	if item.Enabled || len(item.Events) != 4 || len(item.Channels) != 1 || item.Channels[0].Type != ChannelInApp {
		t.Fatalf("unexpected disabled subscription snapshot: %+v", item)
	}
}

func TestTaskNotificationFactRejectsUnsafeIdentityAndReason(t *testing.T) {
	valid := TaskNotificationFact{TenantID: 1, TaskID: 2, TaskTurnID: 3, FactType: FactActionRequired, SourceType: SourceContinuationCheckpoint, SourceID: 4, ReasonCode: "MORE_INFORMATION_REQUIRED", Focus: FocusEvaluation, OccurredAt: time.Now().UTC()}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid fact rejected: %v", err)
	}
	invalid := valid
	invalid.TaskID = 0
	if !errors.Is(invalid.Validate(), ErrFactInvalid) {
		t.Fatal("zero Task ID was accepted")
	}
	invalid = valid
	invalid.ReasonCode = "unsafe reason"
	if !errors.Is(invalid.Validate(), ErrFactInvalid) {
		t.Fatal("unsafe reason code was accepted")
	}
}

func TestTaskNotificationFactRejectsInvalidFactSourceFocusCombination(t *testing.T) {
	fact := TaskNotificationFact{TenantID: 1, TaskID: 2, TaskTurnID: 3, FactType: FactCompleted, SourceType: SourceContinuationCheckpoint, SourceID: 4, Focus: FocusEvaluation, OccurredAt: time.Now().UTC()}
	if !errors.Is(fact.Validate(), ErrFactInvalid) {
		t.Fatal("completed notification accepted a continuation/evaluation source")
	}
	fact.SourceType = SourceFinalizationRun
	fact.Focus = FocusResult
	if err := fact.Validate(); err != nil {
		t.Fatalf("valid completed combination rejected: %v", err)
	}
}

func TestFrozenPresentationAndDigestAreStable(t *testing.T) {
	record := Record{
		ID: 1, TenantID: 2, TaskID: 9007199254740993, TaskTurnID: 9007199254740995,
		SourceType: SourceContinuationCheckpoint, SourceID: 8, FactType: FactActionRequired, Focus: FocusEvaluation,
		SubscriptionID: 9, SubscriberUserID: 9007199254740997, SubscriptionVersion: 1,
		TaskNameSnapshot: "补充查询接口", TurnNumber: 2,
	}
	if err := FreezePresentation(&record); err != nil {
		t.Fatal(err)
	}
	if record.DeepLinkSnapshot != "/workforce/tasks/9007199254740993?turn=9007199254740995&focus=evaluation" {
		t.Fatalf("unexpected deep link %q", record.DeepLinkSnapshot)
	}
	if record.IdempotencyIdentity != "workforce-task:9007199254740993:turn:9007199254740995:TASK_ACTION_REQUIRED:CONTINUATION_CHECKPOINT:8:user:9007199254740997" {
		t.Fatalf("unexpected idempotency identity %q", record.IdempotencyIdentity)
	}
	first := RecordRequestDigest(record)
	second := RecordRequestDigest(record)
	if first == "" || first != second {
		t.Fatalf("record digest is not stable: %q != %q", first, second)
	}
	mutated := record
	mutated.TitleSnapshot = "changed"
	if RecordRequestDigest(mutated) == first {
		t.Fatal("presentation mutation did not change request digest")
	}
}

func TestOutboxMessageEncodesInt64AsDecimalStrings(t *testing.T) {
	body, err := json.Marshal(OutboxMessage{SchemaVersion: 1, RecordID: 9007199254740993, TaskID: 9007199254740995, TaskTurnID: 9007199254740997})
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if text != `{"schemaVersion":1,"recordId":"9007199254740993","taskId":"9007199254740995","taskTurnId":"9007199254740997"}` {
		t.Fatalf("unexpected lossless wire contract: %s", text)
	}
}
