package application

import (
	"context"
	"errors"
	"testing"

	notificationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/domain"
)

type readyPlatform struct{}

func (readyPlatform) ResolveReadinessForOrganization(context.Context, int64, string, int64) (*notificationfacade.Readiness, error) {
	return &notificationfacade.Readiness{
		InApp: notificationfacade.ReadinessCapability{Available: true},
		Email: notificationfacade.ReadinessCapability{Available: true, RecipientReady: true},
	}, nil
}

func (readyPlatform) ListAvailableGroupRoutes(context.Context, int64, int64, string) ([]notificationfacade.GroupRouteOption, error) {
	return []notificationfacade.GroupRouteOption{}, nil
}
func (readyPlatform) ResolvePublicationStatus(context.Context, int64, string, int64) (*notificationfacade.PublicationStatus, error) {
	return &notificationfacade.PublicationStatus{}, nil
}

type serviceRepository struct {
	creator int64
	item    domain.Subscription
	updated bool
}

func (r *serviceRepository) TaskCreator(context.Context, int64, int64) (int64, error) {
	if r.creator <= 0 {
		return 0, errors.New("missing")
	}
	return r.creator, nil
}
func (r *serviceRepository) GetSubscription(context.Context, int64, int64, int64) (domain.Subscription, error) {
	return r.item, nil
}
func (r *serviceRepository) UpdateSubscription(_ context.Context, _, _, _, _, _ int64, command domain.SubscriptionCommand) (domain.Subscription, error) {
	r.updated = true
	r.item.Enabled = command.Enabled
	r.item.Events = command.Events
	r.item.Channels = command.Channels
	return r.item, nil
}
func (r *serviceRepository) ListRecords(context.Context, int64, int64, int64, int) ([]domain.Record, error) {
	return []domain.Record{{ID: 99}}, nil
}

func TestCreatorCanManageOwnSubscriptionButForeignUserCannot(t *testing.T) {
	repository := &serviceRepository{creator: 7, item: domain.Subscription{ID: 1, TenantID: 2, TaskID: 3, SubscriberUserID: 7}}
	service := NewService(repository, true)
	command := domain.DefaultSubscriptionCommand()
	command.Enabled = false
	if _, err := service.UpdateSubscription(context.Background(), 2, 3, 8, 1, false, command); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign update error=%v, want forbidden", err)
	}
	if repository.updated {
		t.Fatal("foreign user updated creator subscription")
	}
	if _, err := service.UpdateSubscription(context.Background(), 2, 3, 7, 1, false, command); err != nil {
		t.Fatalf("creator update failed: %v", err)
	}
	if !repository.updated {
		t.Fatal("creator update did not reach repository")
	}
}

func TestOperatorMayReadCreatorSubscriptionAndReadinessDegrades(t *testing.T) {
	repository := &serviceRepository{creator: 7, item: domain.Subscription{ID: 1, TenantID: 2, TaskID: 3, SubscriberUserID: 7}}
	service := NewService(repository, false)
	item, err := service.GetSubscription(context.Background(), 2, 3, 9, true)
	if err != nil || item.SubscriberUserID != 7 {
		t.Fatalf("operator read item=%+v err=%v", item, err)
	}
	readiness, err := service.ResolveReadiness(context.Background(), 2, 7)
	if err != nil {
		t.Fatal(err)
	}
	if readiness.InApp.Available || readiness.InApp.ReasonCode == "" {
		t.Fatalf("unexpected readiness: %+v", readiness)
	}
}

func TestGateFourAcceptsSafeExternalChannelKinds(t *testing.T) {
	repository := &serviceRepository{creator: 7}
	service := NewService(repository, true)
	service.BindPlatformReadiness(readyPlatform{})
	_, err := service.UpdateSubscription(context.Background(), 2, 3, 7, 1, false, domain.SubscriptionCommand{
		Enabled: true, Events: []domain.FactType{domain.FactCompleted}, Channels: []domain.ChannelSelection{{Type: domain.ChannelEmailUser}},
	})
	if err != nil {
		t.Fatalf("safe Gate 4 email channel rejected: %v", err)
	}
}
