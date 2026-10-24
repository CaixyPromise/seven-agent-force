package application

import (
	"context"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
)

type groupRouteTestRepository struct {
	*externalTestRepository
	routes   map[int64]domain.GroupRoute
	channels map[int64]domain.Channel
}

func newGroupRouteTestRepository() *groupRouteTestRepository {
	return &groupRouteTestRepository{externalTestRepository: newExternalTestRepository(), routes: map[int64]domain.GroupRoute{}, channels: map[int64]domain.Channel{}}
}

func (r *groupRouteTestRepository) FindGroupRoutesByIDs(_ context.Context, organizationID int64, ids []int64) ([]domain.GroupRoute, error) {
	result := make([]domain.GroupRoute, 0, len(ids))
	for _, id := range ids {
		if item, ok := r.routes[id]; ok && item.OrganizationID == organizationID {
			result = append(result, item)
		}
	}
	return result, nil
}

func (r *groupRouteTestRepository) FindChannelByID(_ context.Context, id int64) (*domain.Channel, error) {
	item, ok := r.channels[id]
	if !ok {
		return nil, nil
	}
	copy := item
	return &copy, nil
}

func validEnabledGroupRouteFixture() (domain.GroupRoute, domain.Channel) {
	organizationID := int64(101)
	route := domain.GroupRoute{
		ID: 301, OrganizationID: organizationID, DisplayName: "研发通知群", Provider: domain.GroupRouteProviderFeishu,
		Transport: domain.GroupRouteTransportFeishuWebhook, DeliveryChannelID: 401, RequiredPermissionCode: "workforce:task:create",
		Status: domain.GroupRouteEnabled, RouteVersion: 3, ChannelVersion: 2, Capabilities: []string{"TASK_ACTION_REQUIRED", "TASK_COMPLETED", "MENTION"},
		LastProbeStatus: "PASSED", CreatorID: 201, UpdaterID: 201, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	route.ConfigDigest = domain.GroupRouteConfigDigest(route)
	channel := domain.Channel{ID: 401, ChannelCode: "group-route-301", ChannelName: "研发通知群", ChannelType: domain.ChannelTypeFeishuWebhook, ScopeID: "local", ScopeType: "ORGANIZATION", OrganizationID: &organizationID, Status: domain.ChannelStatusEnabled}
	return route, channel
}

func TestResolveGroupRouteDeliveriesDegradesUnavailableMentionWithoutDroppingGroup(t *testing.T) {
	repo := newGroupRouteTestRepository()
	route, channel := validEnabledGroupRouteFixture()
	repo.routes[route.ID], repo.channels[channel.ID] = route, channel
	service := newExternalTestService(t, repo.externalTestRepository, nil)
	service.repo = repo
	service.enterpriseIdentities = enterpriseIdentityResolverStub{}

	_, static, err := service.resolveGroupRouteDeliveries(context.Background(), facade.PublishRequest{
		OrganizationID: 101, ReceiverKinds: []facade.ReceiverKind{facade.ReceiverKindFeishuGroup},
		GroupRouteIDs: []int64{route.ID}, MentionUserIDs: []int64{201},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(static) != 1 || static[0].groupRoute == nil || static[0].groupRoute.warningCode != domain.ErrMentionIdentityNotAvailable.Error() || static[0].groupRoute.mentionUserID != 201 {
		t.Fatalf("degraded route=%#v", static)
	}
}

func TestResolveGroupRouteDeliveriesCompletedHasNoMention(t *testing.T) {
	repo := newGroupRouteTestRepository()
	route, channel := validEnabledGroupRouteFixture()
	repo.routes[route.ID], repo.channels[channel.ID] = route, channel
	service := newExternalTestService(t, repo.externalTestRepository, nil)
	service.repo = repo

	_, static, err := service.resolveGroupRouteDeliveries(context.Background(), facade.PublishRequest{
		OrganizationID: 101, ReceiverKinds: []facade.ReceiverKind{facade.ReceiverKindFeishuGroup}, GroupRouteIDs: []int64{route.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(static) != 1 || static[0].groupRoute == nil || static[0].groupRoute.mentionUserID != 0 || static[0].groupRoute.warningCode != "" {
		t.Fatalf("completed route=%#v", static)
	}
}

func TestResolveGroupRouteDeliveriesRejectsCrossOrganizationAndLimit(t *testing.T) {
	service := &Service{repo: newGroupRouteTestRepository()}
	_, _, err := service.resolveGroupRouteDeliveries(context.Background(), facade.PublishRequest{
		OrganizationID: 101, ReceiverKinds: []facade.ReceiverKind{facade.ReceiverKindFeishuGroup}, GroupRouteIDs: []int64{1, 2, 3, 4},
	})
	if err == nil {
		t.Fatal("four group routes were accepted")
	}

	repo := newGroupRouteTestRepository()
	route, channel := validEnabledGroupRouteFixture()
	route.OrganizationID = 202
	route.ConfigDigest = domain.GroupRouteConfigDigest(route)
	repo.routes[route.ID], repo.channels[channel.ID] = route, channel
	service.repo = repo
	_, _, err = service.resolveGroupRouteDeliveries(context.Background(), facade.PublishRequest{
		OrganizationID: 101, ReceiverKinds: []facade.ReceiverKind{facade.ReceiverKindFeishuGroup}, GroupRouteIDs: []int64{route.ID},
	})
	if err == nil {
		t.Fatal("cross-organization group route was accepted")
	}
}

func TestGroupRouteProbeReasonUsesBoundedActionableCodes(t *testing.T) {
	tests := []struct {
		name         string
		status       string
		failureClass string
		diagnostic   string
		provider     *facade.ProviderError
		want         string
	}{
		{name: "bot is not in chat", status: domain.DriverResultFailed, failureClass: "INVALID_TARGET", provider: &facade.ProviderError{Code: "230001", Message: "raw provider message must not persist"}, want: "FEISHU_GROUP_UNAVAILABLE"},
		{name: "credential rejected", status: domain.DriverResultFailed, failureClass: "AUTHENTICATION", want: "CONNECTION_AUTHENTICATION_FAILED"},
		{name: "response uncertain", status: domain.DriverResultUnknown, diagnostic: "FEISHU_REQUEST_UNCONFIRMED", want: "PROVIDER_OUTCOME_UNKNOWN"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := groupRouteProbeReason(test.status, test.failureClass, test.diagnostic, test.provider, nil); got != test.want {
				t.Fatalf("reason=%q, want %q", got, test.want)
			}
		})
	}
}
