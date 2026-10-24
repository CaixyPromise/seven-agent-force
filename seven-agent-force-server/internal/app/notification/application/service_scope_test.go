package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
)

func TestRelayOutboxLeavesForeignScopeEventPendingBeforeClaim(t *testing.T) {
	repo := &foreignScopeOutboxRepository{}
	service := NewService(nil, repo, domain.NewService(), nil, nil, nil, disabledPublisher{}, nil)
	service.SetScopeID("node:gray-b")

	if err := service.RelayOutbox(context.Background(), 1); err != nil {
		t.Fatalf("RelayOutbox() error=%v", err)
	}
	if repo.claimCalls != 0 || repo.markCalls != 0 {
		t.Fatalf("foreign event was mutated claims=%d marks=%d", repo.claimCalls, repo.markCalls)
	}
}

func TestHandleDispatchMessageRejectsForeignScopeBeforeConsumeLease(t *testing.T) {
	repo := &foreignScopeConsumeRepository{}
	service := NewService(nil, repo, domain.NewService(), nil, nil, nil, nil, nil)
	service.SetScopeID("node:gray-b")

	err := service.HandleDispatchMessage(context.Background(), domain.DeliveryMessage{
		MessageID:  "notification:foreign",
		DeliveryID: "foreign",
		ScopeID:    "node:gray-a",
	})
	if !errors.Is(err, ErrNotificationScopeMismatch) {
		t.Fatalf("HandleDispatchMessage() error=%v, want scope mismatch", err)
	}
	if repo.beginCalls != 0 {
		t.Fatalf("foreign message created a consume lease %d times", repo.beginCalls)
	}
}

func TestDispatchDoesNotMarkForeignExternalDeliverySending(t *testing.T) {
	targetID := int64(42)
	repo := &foreignScopeDeliveryRepository{targetID: targetID}
	service := NewService(nil, repo, domain.NewService(), nil, nil, nil, nil, nil)
	service.SetScopeID("node:gray-b")

	err := service.dispatch(context.Background(), "delivery-foreign")
	if !errors.Is(err, ErrNotificationScopeMismatch) {
		t.Fatalf("dispatch() error=%v, want scope mismatch", err)
	}
	if repo.markSendingCalls != 0 {
		t.Fatalf("foreign external delivery was marked SENDING %d times", repo.markSendingCalls)
	}
}

func TestUpsertChannelDoesNotOverwriteForeignScope(t *testing.T) {
	repo := &foreignScopeChannelRepository{channel: &domain.Channel{
		ID:          10,
		ChannelCode: "foreign-mock",
		ChannelType: domain.ChannelTypeMock,
		ScopeID:     "node:gray-a",
		Status:      domain.ChannelStatusEnabled,
	}}
	service := NewService(nil, repo, domain.NewService(), nil, nil, nil, nil, nil)
	service.SetScopeID("node:gray-b")

	_, err := service.UpsertChannel(context.Background(), facade.ChannelUpsertRequest{
		ChannelCode: "foreign-mock",
		ChannelName: "foreign",
		ChannelType: domain.ChannelTypeMock,
		Status:      domain.ChannelStatusEnabled,
	}, 1)
	if !errors.Is(err, ErrNotificationScopeMismatch) {
		t.Fatalf("UpsertChannel() error=%v, want scope mismatch", err)
	}
	if repo.upsertCalls != 0 {
		t.Fatalf("foreign channel was overwritten %d times", repo.upsertCalls)
	}
}

func TestUpsertChannelMapsScopedRepositoryMissToScopeMismatch(t *testing.T) {
	repo := &foreignScopeChannelRepository{
		channel: &domain.Channel{
			ID:          10,
			ChannelCode: "scope-b-mock",
			ChannelType: domain.ChannelTypeMock,
			ScopeID:     "node:gray-b",
			Status:      domain.ChannelStatusEnabled,
		},
		upsertChannelErr: domain.ErrScopedConfigurationNotFound,
	}
	service := NewService(nil, repo, domain.NewService(), nil, nil, nil, nil, nil)
	service.SetScopeID("node:gray-b")

	_, err := service.UpsertChannel(context.Background(), facade.ChannelUpsertRequest{
		ChannelCode: "scope-b-mock",
		ChannelName: "Scope B mock",
		ChannelType: domain.ChannelTypeMock,
		Status:      domain.ChannelStatusEnabled,
	}, 1)
	if !errors.Is(err, ErrNotificationScopeMismatch) {
		t.Fatalf("UpsertChannel() error=%v, want scope mismatch", err)
	}
	if repo.upsertCalls != 1 {
		t.Fatalf("UpsertChannel() calls=%d, want one guarded repository write", repo.upsertCalls)
	}
}

func TestListChannelsBindsRepositoryQueryToCurrentScope(t *testing.T) {
	repo := &foreignScopeChannelRepository{}
	service := NewService(nil, repo, domain.NewService(), nil, nil, nil, nil, nil)
	service.SetScopeID("node:gray-b")

	if _, err := service.ListChannels(context.Background(), domain.ChannelQuery{Current: 1, PageSize: 20}); err != nil {
		t.Fatalf("ListChannels() error=%v", err)
	}
	if repo.listScope != "node:gray-b" {
		t.Fatalf("ListChannels() scope=%q, want current scope", repo.listScope)
	}
}

type foreignScopeOutboxRepository struct {
	domain.Repository
	claimCalls int
	markCalls  int
}

func (r *foreignScopeOutboxRepository) ListUnknownOutbox(context.Context, int) ([]domain.OutboxEvent, error) {
	return nil, nil
}

func (r *foreignScopeOutboxRepository) ListReadyOutbox(context.Context, int) ([]domain.OutboxEvent, error) {
	return []domain.OutboxEvent{{
		ID:        1,
		EventID:   "notification:foreign",
		ScopeID:   "node:gray-a",
		EventType: domain.OutboxEventNotificationDispatch,
		Payload:   `{"messageId":"notification:foreign","deliveryId":"foreign","scopeId":"node:gray-a"}`,
		Status:    "PENDING",
	}}, nil
}

func (r *foreignScopeOutboxRepository) TryClaimOutbox(context.Context, int64, string, string) (*domain.OutboxLease, bool, error) {
	r.claimCalls++
	return &domain.OutboxLease{Token: "unexpected"}, true, nil
}

func (r *foreignScopeOutboxRepository) MarkOutbox(context.Context, int64, string, string, string, string, int, *time.Time) (bool, error) {
	r.markCalls++
	return true, nil
}

type foreignScopeConsumeRepository struct {
	domain.Repository
	beginCalls int
}

func (r *foreignScopeConsumeRepository) BeginConsume(context.Context, string, string, string, string) (*domain.ConsumeLease, bool, error) {
	r.beginCalls++
	return &domain.ConsumeLease{Token: "unexpected"}, true, nil
}

type foreignScopeDeliveryRepository struct {
	domain.Repository
	targetID         int64
	markSendingCalls int
}

type foreignScopeChannelRepository struct {
	domain.Repository
	channel          *domain.Channel
	channelCalls     int
	upsertCalls      int
	upsertChannelErr error
	listScope        string
}

func (r *foreignScopeChannelRepository) FindChannelByCode(context.Context, string) (*domain.Channel, error) {
	r.channelCalls++
	if r.channel == nil {
		return nil, nil
	}
	copy := *r.channel
	return &copy, nil
}

func (r *foreignScopeChannelRepository) UpsertChannel(context.Context, *domain.Channel) error {
	r.upsertCalls++
	return r.upsertChannelErr
}

func (r *foreignScopeChannelRepository) ListChannels(_ context.Context, query domain.ChannelQuery) ([]domain.Channel, int64, error) {
	r.listScope = query.ScopeID
	return nil, 0, nil
}

func (r *foreignScopeDeliveryRepository) FindDeliveryByID(context.Context, string) (*domain.Delivery, error) {
	return &domain.Delivery{DeliveryID: "delivery-foreign", ExternalTargetID: &r.targetID, Status: domain.DeliveryStatusPending}, nil
}

func (r *foreignScopeDeliveryRepository) FindExternalTargetByID(context.Context, int64) (*domain.ExternalTarget, error) {
	return &domain.ExternalTarget{ID: r.targetID, ScopeID: "node:gray-a"}, nil
}

func (r *foreignScopeDeliveryRepository) MarkDeliverySending(context.Context, string) (bool, error) {
	r.markSendingCalls++
	return true, nil
}
