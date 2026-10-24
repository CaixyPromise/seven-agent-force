package infrastructure

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	notificationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/domain"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

type fakeNotificationClient struct {
	requests []notificationfacade.PublishRequest
	receipts []*notificationfacade.PublishReceipt
	errors   []error
}

func (c *fakeNotificationClient) Publish(_ context.Context, request notificationfacade.PublishRequest) (*notificationfacade.PublishReceipt, error) {
	c.requests = append(c.requests, request)
	index := len(c.requests) - 1
	if index < len(c.errors) && c.errors[index] != nil {
		return nil, c.errors[index]
	}
	if index < len(c.receipts) {
		return c.receipts[index], nil
	}
	return &notificationfacade.PublishReceipt{NotificationID: "ntf_default", Status: "MATERIALIZED"}, nil
}

type fakePublicationStore struct {
	items       [][]Publication
	completed   []Publication
	retried     []Publication
	failed      []Publication
	completeErr error
}

func (s *fakePublicationStore) ClaimPublications(context.Context, string, time.Time, time.Duration, int) ([]Publication, error) {
	if len(s.items) == 0 {
		return nil, nil
	}
	items := s.items[0]
	s.items = s.items[1:]
	return items, nil
}

func (s *fakePublicationStore) CompletePublication(_ context.Context, item Publication, _, _ string, _ time.Time) error {
	s.completed = append(s.completed, item)
	if s.completeErr != nil {
		err := s.completeErr
		s.completeErr = nil
		return err
	}
	return nil
}

func (s *fakePublicationStore) RetryPublication(_ context.Context, item Publication, _ string, _ time.Time) error {
	s.retried = append(s.retried, item)
	return nil
}

func (s *fakePublicationStore) FailPublication(_ context.Context, item Publication, _ string, _ time.Time) error {
	s.failed = append(s.failed, item)
	return nil
}

func publicationRecord(t *testing.T) domain.Record {
	t.Helper()
	record := domain.Record{
		ID: 1, TenantID: 2, TaskID: 9007199254740993, TaskTurnID: 9007199254740995,
		SourceType: domain.SourceContinuationCheckpoint, SourceID: 7, FactType: domain.FactActionRequired, Focus: domain.FocusEvaluation,
		SubscriptionID: 8, SubscriberUserID: 9007199254740997, SubscriptionVersion: 1,
		TaskNameSnapshot: "补充查询接口", TurnNumber: 2, Status: domain.RecordProcessing, LeaseToken: "lease",
		OccurredAt: time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC),
	}
	if err := domain.FreezePresentation(&record); err != nil {
		t.Fatal(err)
	}
	record.RequestDigest = domain.RecordRequestDigest(record)
	return record
}

func TestPublishRequestUsesOnlyFrozenInAppContract(t *testing.T) {
	record := publicationRecord(t)
	request, err := PublishRequest(record)
	if err != nil {
		t.Fatal(err)
	}
	if request.EventKey != "workforce.task.action_required" || request.SceneCode != "workforce_task_action_required" || request.Priority != "HIGH" || request.CreatorID != 0 {
		t.Fatalf("unexpected request: %+v", request)
	}
	if request.Title != "" || request.Content != "" || request.DeepLink != record.DeepLinkSnapshot || request.IdempotencyKey != record.IdempotencyIdentity {
		t.Fatalf("publisher did not use frozen snapshots: %+v", request)
	}
	if !reflect.DeepEqual(request.Audience.UserIDs, []int64{record.SubscriberUserID}) || !reflect.DeepEqual(request.ReceiverKinds, []notificationfacade.ReceiverKind{notificationfacade.ReceiverKindInApp}) || len(request.ExternalRecipients) != 0 || len(request.StaticRoutes) != 0 {
		t.Fatalf("unexpected recipients/routes: %+v", request)
	}
}

func TestPublishRequestKeepsOneLogicalNotificationForSubscriberAttentionAndGroup(t *testing.T) {
	record := publicationRecord(t)
	record.Channels = []domain.RecordChannel{
		{Type: domain.ChannelInApp},
		{Type: domain.ChannelFeishuDirect},
		{Type: domain.ChannelGroupRoute, GroupRouteID: 41, MentionUserID: 9007199254740999},
	}
	record.Recipients = []domain.RecordRecipient{
		{UserID: record.SubscriberUserID, Role: domain.RecipientSubscriber, PersonalChannelPolicy: domain.PersonalChannelsTaskSubscription},
		{UserID: 9007199254740999, Role: domain.RecipientAttention, PersonalChannelPolicy: domain.PersonalChannelsUserPreference, InAppRequired: true},
	}
	request, err := PublishRequest(record)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.UserAudiences) != 2 || len(request.GroupRouteIDs) != 1 || request.GroupRouteIDs[0] != 41 {
		t.Fatalf("unexpected audience/route plan: %+v", request)
	}
	if !reflect.DeepEqual(request.MentionUserIDs, []int64{9007199254740999}) {
		t.Fatalf("mention plan=%v", request.MentionUserIDs)
	}
	if !reflect.DeepEqual(request.UserAudiences[0].ReceiverKinds, []notificationfacade.ReceiverKind{notificationfacade.ReceiverKindInApp, notificationfacade.ReceiverKindFeishuOpenID}) {
		t.Fatalf("subscriber channels=%v", request.UserAudiences[0].ReceiverKinds)
	}
	if !reflect.DeepEqual(request.UserAudiences[1].ReceiverKinds, []notificationfacade.ReceiverKind{notificationfacade.ReceiverKindInApp, notificationfacade.ReceiverKindEmailUser, notificationfacade.ReceiverKindFeishuOpenID, notificationfacade.ReceiverKindWeComUserID}) {
		t.Fatalf("attention channels=%v", request.UserAudiences[1].ReceiverKinds)
	}
}

func TestPublishRequestKeepsVersionOnePendingRecordCompatible(t *testing.T) {
	record := publicationRecord(t)
	record.PublicationContractVersion = 1
	record.SceneCode = ""
	record.TemplateVariablesJSON = ""
	request, err := PublishRequest(record)
	if err != nil {
		t.Fatal(err)
	}
	if request.SceneCode != "" || request.Title != record.TitleSnapshot || request.Content != record.ContentSnapshot || len(request.ReceiverKinds) != 0 {
		t.Fatalf("version one compatibility drifted: %+v", request)
	}
}

func TestPublishBatchRetriesTransientFailure(t *testing.T) {
	record := publicationRecord(t)
	item := Publication{OutboxID: 10, Record: record}
	store := &fakePublicationStore{items: [][]Publication{{item}}}
	client := &fakeNotificationClient{errors: []error{errors.New("temporarily unavailable")}}
	publisher := NewPublisher(store, client)
	publisher.now = func() time.Time { return time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC) }
	if err := publisher.PublishBatch(context.Background(), 1); err == nil {
		t.Fatal("transient publish failure was hidden")
	}
	if len(store.retried) != 1 || len(store.failed) != 0 {
		t.Fatalf("retry=%d failed=%d", len(store.retried), len(store.failed))
	}
}

func TestPublishBatchFailsPermanentNotificationIdempotencyConflict(t *testing.T) {
	record := publicationRecord(t)
	item := Publication{OutboxID: 10, Record: record}
	store := &fakePublicationStore{items: [][]Publication{{item}}}
	conflict := apperrors.ObjectState("conflict").WithDetails(map[string]string{"reasonCode": "IDEMPOTENCY_CONFLICT"})
	client := &fakeNotificationClient{errors: []error{conflict}}
	publisher := NewPublisher(store, client)
	if err := publisher.PublishBatch(context.Background(), 1); err == nil {
		t.Fatal("permanent conflict was hidden")
	}
	if len(store.failed) != 1 || len(store.retried) != 0 {
		t.Fatalf("failed=%d retry=%d", len(store.failed), len(store.retried))
	}
}

func TestPublishBatchFailsPermanentFrozenContractErrorWithoutCallingClient(t *testing.T) {
	record := publicationRecord(t)
	record.DeepLinkSnapshot = "//evil.example"
	item := Publication{OutboxID: 10, Record: record}
	store := &fakePublicationStore{items: [][]Publication{{item}}}
	client := &fakeNotificationClient{}
	publisher := NewPublisher(store, client)
	if err := publisher.PublishBatch(context.Background(), 1); err == nil {
		t.Fatal("permanent frozen-contract error was hidden")
	}
	if len(store.failed) != 1 || len(store.retried) != 0 || len(client.requests) != 0 {
		t.Fatalf("failed=%d retry=%d client=%d", len(store.failed), len(store.retried), len(client.requests))
	}
}

func TestPublishBatchFailsPermanentEmptyNotificationReceipt(t *testing.T) {
	record := publicationRecord(t)
	item := Publication{OutboxID: 10, Record: record}
	store := &fakePublicationStore{items: [][]Publication{{item}}}
	client := &fakeNotificationClient{receipts: []*notificationfacade.PublishReceipt{{}}}
	publisher := NewPublisher(store, client)
	if err := publisher.PublishBatch(context.Background(), 1); !errors.Is(err, domain.ErrPublishFailed) {
		t.Fatalf("publish error = %v, want %s", err, domain.ErrPublishFailed)
	}
	if len(store.failed) != 1 || len(store.retried) != 0 {
		t.Fatalf("failed=%d retry=%d", len(store.failed), len(store.retried))
	}
}

func TestResponseLossReplayUsesSameNotificationIdentity(t *testing.T) {
	record := publicationRecord(t)
	item := Publication{OutboxID: 10, Record: record}
	store := &fakePublicationStore{items: [][]Publication{{item}, {item}}, completeErr: errors.New("database response lost")}
	client := &fakeNotificationClient{receipts: []*notificationfacade.PublishReceipt{
		{NotificationID: "ntf_same", Status: "MATERIALIZED"},
		{NotificationID: "ntf_same", Status: "MATERIALIZED", Duplicate: true},
	}}
	publisher := NewPublisher(store, client)
	if err := publisher.PublishBatch(context.Background(), 1); err == nil {
		t.Fatal("first lost completion response was hidden")
	}
	if err := publisher.PublishBatch(context.Background(), 1); err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if len(client.requests) != 2 || client.requests[0].IdempotencyKey != client.requests[1].IdempotencyKey {
		t.Fatalf("replay changed idempotency: %+v", client.requests)
	}
}
