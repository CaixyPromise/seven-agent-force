package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	msgoutbox "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/messaging/outbox"
)

func TestUpsertChannelInvokesURLGuardBeforePersistence(t *testing.T) {
	repo := &channelURLTestRepository{}
	validator := &recordingChannelURLValidator{err: errors.New("SSRF destination denied")}
	service := NewService(nil, repo, domain.NewService(), nil, nil, validator, nil, nil)

	_, err := service.UpsertChannel(context.Background(), facade.ChannelUpsertRequest{
		ChannelCode: "webhook",
		ChannelName: "Webhook",
		ChannelType: domain.ChannelTypeWebhook,
		Status:      domain.ChannelStatusEnabled,
		ConfigJSON:  `{"endpointUrl":"https://127.0.0.1/hook"}`,
	}, 1)
	if err == nil || !validator.called {
		t.Fatalf("UpsertChannel() err=%v validatorCalled=%t, want URL guard denial", err, validator.called)
	}
	if repo.saved {
		t.Fatal("UpsertChannel() persisted a channel after URL guard denial")
	}
}

func TestRelayOutboxUsesBoundedSynchronousFallbackWhenBrokerIsDisabled(t *testing.T) {
	driver := &blockingDriver{started: make(chan struct{}), release: make(chan struct{})}
	repo := &boundedFallbackRepository{}
	service := NewService(nil, repo, domain.NewService(), nil, driverRegistryFunc(func(string) ChannelDriver { return driver }), nil, disabledPublisher{}, nil)
	service.now = func() time.Time { return time.Date(2026, 7, 22, 11, 0, 0, 0, time.UTC) }

	done := make(chan error, 1)
	go func() { done <- service.RelayOutbox(context.Background(), 1) }()
	select {
	case <-driver.started:
	case <-time.After(time.Second):
		t.Fatal("fallback driver did not start")
	}
	select {
	case err := <-done:
		t.Fatalf("RelayOutbox() returned before bounded fallback completed: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(driver.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RelayOutbox() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RelayOutbox() did not complete after fallback dispatch released")
	}
	if repo.outboxStatus != "DONE" {
		t.Fatalf("outbox status=%q, want DONE", repo.outboxStatus)
	}
}

func TestRelayOutboxFailsUnknownOwnerEventClosed(t *testing.T) {
	repo := &unknownOutboxRepository{}
	service := NewService(nil, repo, domain.NewService(), nil, nil, nil, disabledPublisher{}, nil)
	if err := service.RelayOutbox(context.Background(), 1); err != nil {
		t.Fatalf("RelayOutbox() error = %v", err)
	}
	if repo.outboxStatus != "DEAD" {
		t.Fatalf("unknown event status=%q, want DEAD", repo.outboxStatus)
	}
}

func TestRelayOutboxKeepsNotificationIntentInsideMaterializationPath(t *testing.T) {
	repo := &intentOutboxRepository{}
	publisher := &recordingDispatchPublisher{}
	service := NewService(nil, repo, domain.NewService(), nil, nil, nil, publisher, nil)
	if err := service.RelayOutbox(context.Background(), 1); err != nil {
		t.Fatalf("RelayOutbox() error = %v", err)
	}
	if publisher.calls != 0 {
		t.Fatalf("notification.intent was published to dispatch broker %d times", publisher.calls)
	}
	if repo.outboxStatus != "DONE" {
		t.Fatalf("notification.intent outbox status=%q, want DONE", repo.outboxStatus)
	}
}

func TestRelaySelectedOutboxClaimsOnlyExactAcceptanceEvents(t *testing.T) {
	repo := newSelectedOutboxRepository()
	publisher := &recordingEnabledPublisher{}
	service := NewService(nil, repo, domain.NewService(), nil, nil, nil, publisher, nil)
	if err := service.RelaySelectedOutbox(context.Background(), []domain.OutboxEventSelection{
		{EventID: "notification-intent:ntf_acceptance", EventType: domain.OutboxEventNotificationIntent},
		{EventID: "notification:delivery-acceptance", EventType: domain.OutboxEventNotificationDispatch},
	}); err != nil {
		t.Fatalf("RelaySelectedOutbox() error = %v", err)
	}
	if repo.readyListCalls != 0 || repo.unknownListCalls != 0 {
		t.Fatalf("selected relay scanned a global queue ready=%d unknown=%d", repo.readyListCalls, repo.unknownListCalls)
	}
	if repo.materializationListCalls != 0 {
		t.Fatalf("selected relay inspected materialization work %d times", repo.materializationListCalls)
	}
	if publisher.calls != 0 {
		t.Fatalf("selected relay published %d messages to the shared broker", publisher.calls)
	}
	if got := repo.claimed; len(got) != 2 || got[0] != "notification-intent:ntf_acceptance" || got[1] != "notification:delivery-acceptance" {
		t.Fatalf("claimed events=%v, want only the exact acceptance events", got)
	}
	if status := repo.status("notification-intent:ntf_unrelated", domain.OutboxEventNotificationIntent); status != "PENDING" {
		t.Fatalf("unrelated notification intent status=%q, want PENDING", status)
	}
	if status := repo.status("notification-intent:ntf_acceptance", domain.OutboxEventNotificationIntent); status != "DONE" {
		t.Fatalf("acceptance notification intent status=%q, want DONE", status)
	}
	if status := repo.status("notification:delivery-acceptance", domain.OutboxEventNotificationDispatch); status != "DONE" {
		t.Fatalf("acceptance delivery dispatch status=%q, want DONE", status)
	}
}

func TestRelaySelectedOutboxRejectsEmptyOrUnknownSelectionsWithoutLookup(t *testing.T) {
	repo := newSelectedOutboxRepository()
	service := NewService(nil, repo, domain.NewService(), nil, nil, nil, disabledPublisher{}, nil)
	if err := service.RelaySelectedOutbox(context.Background(), nil); err == nil {
		t.Fatal("RelaySelectedOutbox() accepted an empty selection")
	}
	if err := service.RelaySelectedOutbox(context.Background(), []domain.OutboxEventSelection{{EventID: "notification:unknown", EventType: "notification.unknown"}}); err == nil {
		t.Fatal("RelaySelectedOutbox() accepted an unknown event type")
	}
	if repo.readyListCalls != 0 || repo.unknownListCalls != 0 || len(repo.claimed) != 0 {
		t.Fatalf("invalid selection touched Outbox ready=%d unknown=%d claims=%v", repo.readyListCalls, repo.unknownListCalls, repo.claimed)
	}
}

func TestRelayInboxChangedTreatsRealtimeFailureAsFreshnessOnly(t *testing.T) {
	repo := &inboxRealtimeOutboxRepository{}
	realtime := &recordingInboxRealtime{err: errors.New("redis unavailable")}
	service := NewService(nil, repo, domain.NewService(), nil, nil, nil, disabledPublisher{}, nil)
	service.SetScopeID("local")
	service.BindInboxRealtime(realtime)

	if err := service.RelayOutbox(context.Background(), 1); err != nil {
		t.Fatalf("RelayOutbox() error = %v", err)
	}
	if repo.outboxStatus != "DONE" {
		t.Fatalf("inbox realtime outage status=%q, want DONE", repo.outboxStatus)
	}
	if len(realtime.intents) != 1 || realtime.intents[0].UserID != 42 || realtime.intents[0].ChangeSequence != 9 {
		t.Fatalf("realtime intents=%#v", realtime.intents)
	}
}

func TestUpsertURLChannelFailsClosedWithoutURLGuard(t *testing.T) {
	repo := &channelURLTestRepository{}
	service := NewService(nil, repo, domain.NewService(), nil, nil, nil, nil, nil)
	_, err := service.UpsertChannel(context.Background(), facade.ChannelUpsertRequest{
		ChannelCode: "webhook",
		ChannelName: "Webhook",
		ChannelType: domain.ChannelTypeWebhook,
		Status:      domain.ChannelStatusEnabled,
		ConfigJSON:  `{"endpointUrl":"https://public.example/hook"}`,
	}, 1)
	if err == nil {
		t.Fatal("UpsertChannel() accepted URL channel without an outbound URL guard")
	}
	if repo.saved {
		t.Fatal("UpsertChannel() persisted URL channel without an outbound URL guard")
	}
}

func TestHandleDispatchMessageReturnsLiveLeaseContentionForBrokerRequeue(t *testing.T) {
	service := NewService(nil, &liveLeaseNotificationRepository{}, domain.NewService(), nil, nil, nil, nil, nil)
	err := service.HandleDispatchMessage(context.Background(), domain.DeliveryMessage{MessageID: "notification:delivery-lease", DeliveryID: "delivery-lease"})
	if !errors.Is(err, msgoutbox.ErrConsumeLeaseHeld) {
		t.Fatalf("HandleDispatchMessage() error=%v, want requeueable live-lease error", err)
	}
}

func TestHandleDispatchMessageClassifiesMissingDurableDeliveryAsPermanent(t *testing.T) {
	repo := &missingDeliveryNotificationRepository{}
	service := NewService(nil, repo, domain.NewService(), nil, nil, nil, nil, nil)

	err := service.HandleDispatchMessage(context.Background(), domain.DeliveryMessage{
		MessageID:  "notification:missing-delivery",
		DeliveryID: "missing-delivery",
	})
	if err == nil {
		t.Fatal("HandleDispatchMessage() error=nil, want permanent missing-delivery error")
	}
	var permanent interface{ Permanent() bool }
	if !errors.As(err, &permanent) || !permanent.Permanent() {
		t.Fatalf("HandleDispatchMessage() error=%T %v, want permanent classification", err, err)
	}
	if !repo.failed {
		t.Fatal("HandleDispatchMessage() did not record the failed consume attempt")
	}
}

type recordingChannelURLValidator struct {
	called bool
	err    error
}

func (v *recordingChannelURLValidator) ValidateChannel(_ context.Context, _ domain.Channel) error {
	v.called = true
	return v.err
}

type channelURLTestRepository struct {
	domain.Repository
	saved bool
}

// UpsertChannel now reads the existing code before validation so it can reject
// a foreign scope rather than overwriting its configuration. This isolated URL
// guard fixture intentionally represents a new channel.
func (r *channelURLTestRepository) FindChannelByCode(context.Context, string) (*domain.Channel, error) {
	return nil, nil
}

type liveLeaseNotificationRepository struct{ domain.Repository }

func (r *liveLeaseNotificationRepository) BeginConsume(context.Context, string, string, string, string) (*domain.ConsumeLease, bool, error) {
	return nil, false, msgoutbox.ConsumeLeaseHeldError{}
}

type missingDeliveryNotificationRepository struct {
	domain.Repository
	failed bool
}

func (r *missingDeliveryNotificationRepository) BeginConsume(context.Context, string, string, string, string) (*domain.ConsumeLease, bool, error) {
	return &domain.ConsumeLease{Token: "missing-delivery-lease"}, true, nil
}

func (r *missingDeliveryNotificationRepository) FindDeliveryByID(context.Context, string) (*domain.Delivery, error) {
	return nil, nil
}

func (r *missingDeliveryNotificationRepository) MarkConsumeFailed(context.Context, string, string, string, string) (bool, error) {
	r.failed = true
	return true, nil
}

func (r *channelURLTestRepository) UpsertChannel(context.Context, *domain.Channel) error {
	r.saved = true
	return nil
}

type driverRegistryFunc func(string) ChannelDriver

func (f driverRegistryFunc) Driver(channelType string) ChannelDriver { return f(channelType) }

type blockingDriver struct {
	started chan struct{}
	release chan struct{}
}

func (d *blockingDriver) Send(context.Context, DriverMessage) error {
	select {
	case <-d.started:
	default:
		close(d.started)
	}
	<-d.release
	return nil
}

type disabledPublisher struct{}

func (disabledPublisher) Enabled() bool { return false }
func (disabledPublisher) PublishDispatch(context.Context, domain.DeliveryMessage) error {
	return nil
}

type recordingEnabledPublisher struct {
	calls int
}

func (*recordingEnabledPublisher) Enabled() bool { return true }

func (p *recordingEnabledPublisher) PublishDispatch(context.Context, domain.DeliveryMessage) error {
	p.calls++
	return nil
}

type boundedFallbackRepository struct {
	domain.Repository
	outboxStatus string
}

func (r *boundedFallbackRepository) ListUnknownOutbox(context.Context, int) ([]domain.OutboxEvent, error) {
	return nil, nil
}

func (r *boundedFallbackRepository) ListReadyOutbox(context.Context, int) ([]domain.OutboxEvent, error) {
	return []domain.OutboxEvent{{
		ID:        9,
		EventID:   "notification:delivery-1",
		EventType: domain.OutboxEventNotificationDispatch,
		Payload:   `{"messageId":"notification:delivery-1","deliveryId":"delivery-1"}`,
		Status:    "PENDING",
	}}, nil
}

func (r *boundedFallbackRepository) TryClaimOutbox(_ context.Context, _ int64, _ string, _ string) (*domain.OutboxLease, bool, error) {
	return &domain.OutboxLease{Token: "lease-1"}, true, nil
}

func (r *boundedFallbackRepository) MarkOutbox(_ context.Context, _ int64, _ string, _ string, status, _ string, _ int, _ *time.Time) (bool, error) {
	r.outboxStatus = status
	return true, nil
}

func (r *boundedFallbackRepository) FindDeliveryByID(context.Context, string) (*domain.Delivery, error) {
	return &domain.Delivery{DeliveryID: "delivery-1", ChannelCode: "mock", Status: domain.DeliveryStatusPending, MaxRetry: 1}, nil
}

func (r *boundedFallbackRepository) MarkDeliverySending(context.Context, string) (bool, error) {
	return true, nil
}

func (r *boundedFallbackRepository) FindChannelByCode(context.Context, string) (*domain.Channel, error) {
	return &domain.Channel{ChannelCode: "mock", ChannelType: domain.ChannelTypeMock, Status: domain.ChannelStatusEnabled}, nil
}

func (r *boundedFallbackRepository) MarkDeliverySent(context.Context, string, time.Time) error {
	return nil
}

type unknownOutboxRepository struct {
	domain.Repository
	outboxStatus string
}

type intentOutboxRepository struct {
	domain.Repository
	outboxStatus string
}

type selectedOutboxRepository struct {
	domain.Repository
	events                   map[string]domain.OutboxEvent
	readyListCalls           int
	unknownListCalls         int
	materializationListCalls int
	claimed                  []string
}

func newSelectedOutboxRepository() *selectedOutboxRepository {
	repo := &selectedOutboxRepository{events: map[string]domain.OutboxEvent{}}
	repo.put(domain.OutboxEvent{ID: 21, EventID: "notification-intent:ntf_acceptance", EventType: domain.OutboxEventNotificationIntent, Payload: `{"notificationId":11}`, Status: "PENDING"})
	repo.put(domain.OutboxEvent{ID: 22, EventID: "notification:delivery-acceptance", EventType: domain.OutboxEventNotificationDispatch, Payload: `{"messageId":"notification:delivery-acceptance","deliveryId":"delivery-acceptance"}`, Status: "PENDING"})
	repo.put(domain.OutboxEvent{ID: 23, EventID: "notification-intent:ntf_unrelated", EventType: domain.OutboxEventNotificationIntent, Payload: `{"notificationId":42}`, Status: "PENDING"})
	return repo
}

func (r *selectedOutboxRepository) put(event domain.OutboxEvent) {
	r.events[event.EventType+"\x00"+event.EventID] = event
}

func (r *selectedOutboxRepository) status(eventID, eventType string) string {
	return r.events[eventType+"\x00"+eventID].Status
}

func (r *selectedOutboxRepository) FindReadyOutbox(_ context.Context, eventID, eventType string) (*domain.OutboxEvent, error) {
	event, ok := r.events[eventType+"\x00"+eventID]
	if !ok || event.Status != "PENDING" {
		return nil, nil
	}
	return &event, nil
}

func (r *selectedOutboxRepository) ListReadyOutbox(context.Context, int) ([]domain.OutboxEvent, error) {
	r.readyListCalls++
	return nil, nil
}

func (r *selectedOutboxRepository) ListUnknownOutbox(context.Context, int) ([]domain.OutboxEvent, error) {
	r.unknownListCalls++
	return nil, nil
}

func (r *selectedOutboxRepository) TryClaimOutbox(_ context.Context, id int64, eventType, _ string) (*domain.OutboxLease, bool, error) {
	for key, event := range r.events {
		if event.ID == id && event.EventType == eventType && event.Status == "PENDING" {
			event.Status = "PROCESSING"
			r.events[key] = event
			r.claimed = append(r.claimed, event.EventID)
			return &domain.OutboxLease{Token: "selected-lease-" + event.EventID}, true, nil
		}
	}
	return nil, false, nil
}

func (r *selectedOutboxRepository) MarkOutbox(_ context.Context, id int64, eventType, _ string, status, _ string, _ int, _ *time.Time) (bool, error) {
	for key, event := range r.events {
		if event.ID == id && event.EventType == eventType && event.Status == "PROCESSING" {
			event.Status = status
			r.events[key] = event
			return true, nil
		}
	}
	return false, nil
}

func (r *selectedOutboxRepository) FindLogicalNotificationByID(_ context.Context, id int64) (*domain.LogicalNotification, error) {
	if id != 11 {
		return nil, nil
	}
	return &domain.LogicalNotification{ID: 11, NotificationID: "ntf_acceptance"}, nil
}

func (r *selectedOutboxRepository) FindDeliveryByID(_ context.Context, deliveryID string) (*domain.Delivery, error) {
	if deliveryID != "delivery-acceptance" {
		return nil, nil
	}
	return &domain.Delivery{DeliveryID: deliveryID, Status: domain.DeliveryStatusProviderAccepted}, nil
}

func (r *selectedOutboxRepository) ListReadyMaterializationTasks(context.Context, string, int) ([]domain.MaterializationTask, error) {
	r.materializationListCalls++
	return nil, nil
}

func (r *intentOutboxRepository) ListUnknownOutbox(context.Context, int) ([]domain.OutboxEvent, error) {
	return nil, nil
}

func (r *intentOutboxRepository) ListReadyOutbox(context.Context, int) ([]domain.OutboxEvent, error) {
	return []domain.OutboxEvent{{
		ID:        11,
		EventID:   "notification-intent:11",
		EventType: domain.OutboxEventNotificationIntent,
		Payload:   `{"notificationId":11}`,
		Status:    "PENDING",
	}}, nil
}

func (r *intentOutboxRepository) TryClaimOutbox(_ context.Context, _ int64, _ string, _ string) (*domain.OutboxLease, bool, error) {
	return &domain.OutboxLease{Token: "intent-lease"}, true, nil
}

func (r *intentOutboxRepository) MarkOutbox(_ context.Context, _ int64, _ string, _ string, status, _ string, _ int, _ *time.Time) (bool, error) {
	r.outboxStatus = status
	return true, nil
}

func (r *intentOutboxRepository) FindLogicalNotificationByID(context.Context, int64) (*domain.LogicalNotification, error) {
	return &domain.LogicalNotification{ID: 11, NotificationID: "ntf_11"}, nil
}

type recordingDispatchPublisher struct {
	calls int
}

func (p *recordingDispatchPublisher) Enabled() bool { return true }

func (p *recordingDispatchPublisher) PublishDispatch(context.Context, domain.DeliveryMessage) error {
	p.calls++
	return nil
}

type recordingInboxRealtime struct {
	intents []domain.InboxChangedIntent
	err     error
}

func (r *recordingInboxRealtime) PublishInboxChanged(_ context.Context, intent domain.InboxChangedIntent) error {
	r.intents = append(r.intents, intent)
	return r.err
}

func (r *recordingInboxRealtime) SubscribeInboxChanges(int64) (<-chan domain.InboxChangedIntent, func()) {
	closed := make(chan domain.InboxChangedIntent)
	close(closed)
	return closed, func() {}
}

type inboxRealtimeOutboxRepository struct {
	domain.Repository
	outboxStatus string
}

func (r *inboxRealtimeOutboxRepository) ListUnknownOutbox(context.Context, int) ([]domain.OutboxEvent, error) {
	return nil, nil
}

func (r *inboxRealtimeOutboxRepository) ListReadyOutbox(context.Context, int) ([]domain.OutboxEvent, error) {
	return []domain.OutboxEvent{{
		ID:        12,
		EventID:   "notification-inbox-changed:local:42:9",
		EventType: domain.OutboxEventNotificationInboxChanged,
		Payload:   `{"scopeId":"local","userId":42,"changeSequence":9,"newUnread":true}`,
		Status:    "PENDING",
	}}, nil
}

func (r *inboxRealtimeOutboxRepository) TryClaimOutbox(_ context.Context, _ int64, _ string, _ string) (*domain.OutboxLease, bool, error) {
	return &domain.OutboxLease{Token: "inbox-realtime-lease"}, true, nil
}

func (r *inboxRealtimeOutboxRepository) MarkOutbox(_ context.Context, _ int64, _ string, _ string, status, _ string, _ int, _ *time.Time) (bool, error) {
	r.outboxStatus = status
	return true, nil
}

func (r *unknownOutboxRepository) ListUnknownOutbox(context.Context, int) ([]domain.OutboxEvent, error) {
	return []domain.OutboxEvent{{ID: 10, EventType: "notification.unrecognized", Status: "PENDING"}}, nil
}

func (r *unknownOutboxRepository) ListReadyOutbox(context.Context, int) ([]domain.OutboxEvent, error) {
	return nil, nil
}

func (r *unknownOutboxRepository) TryClaimOutbox(_ context.Context, _ int64, _ string, _ string) (*domain.OutboxLease, bool, error) {
	return &domain.OutboxLease{Token: "lease-unknown"}, true, nil
}

func (r *unknownOutboxRepository) MarkOutbox(_ context.Context, _ int64, _ string, _ string, status, _ string, _ int, _ *time.Time) (bool, error) {
	r.outboxStatus = status
	return true, nil
}
