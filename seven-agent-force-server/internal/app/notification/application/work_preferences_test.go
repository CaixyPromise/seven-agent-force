package application

import (
	"context"
	"reflect"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
)

type workPreferenceTestRepository struct {
	domain.Repository
	items map[string]facade.WorkPreference
}

func preferenceKey(organizationID, userID int64, eventFamily, channelKind string) string {
	return eventFamily + "\x00" + channelKind + "\x00" + string(rune(organizationID)) + "\x00" + string(rune(userID))
}

func (r *workPreferenceTestRepository) ListWorkPreferences(_ context.Context, organizationID, userID int64) ([]facade.WorkPreference, error) {
	items := make([]facade.WorkPreference, 0, len(r.items))
	for _, item := range r.items {
		if item.OrganizationID == organizationID && item.UserID == userID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (r *workPreferenceTestRepository) FindWorkPreference(_ context.Context, organizationID, userID int64, eventFamily, channelKind string) (*facade.WorkPreference, error) {
	item, ok := r.items[preferenceKey(organizationID, userID, eventFamily, channelKind)]
	if !ok {
		return nil, nil
	}
	return &item, nil
}

func (r *workPreferenceTestRepository) InsertWorkPreference(_ context.Context, item facade.WorkPreference) error {
	r.items[preferenceKey(item.OrganizationID, item.UserID, item.EventFamily, item.ChannelKind)] = item
	return nil
}

func (r *workPreferenceTestRepository) UpdateWorkPreference(_ context.Context, item facade.WorkPreference, expectedVersion int64) (bool, error) {
	key := preferenceKey(item.OrganizationID, item.UserID, item.EventFamily, item.ChannelKind)
	current, ok := r.items[key]
	if !ok || current.Version != expectedVersion {
		return false, nil
	}
	item.Version = expectedVersion + 1
	r.items[key] = item
	return true, nil
}

func newWorkPreferenceTestService(t *testing.T, repository *workPreferenceTestRepository) *Service {
	t.Helper()
	idGen, err := xid.New(71)
	if err != nil {
		t.Fatal(err)
	}
	return NewService(nil, repository, domain.NewService(), nil, nil, nil, nil, idGen)
}

func TestWorkPreferenceContractRejectsGroupsAndUnknownEvents(t *testing.T) {
	if !validWorkPreference("TASK_ACTION_REQUIRED", "IN_APP") {
		t.Fatal("valid preference rejected")
	}
	for _, item := range [][2]string{{"TASK_ACTION_REQUIRED", "GROUP_ROUTE"}, {"OTHER", "IN_APP"}, {"TASK_COMPLETED", "WEBHOOK"}} {
		if validWorkPreference(item[0], item[1]) {
			t.Fatalf("unexpected valid preference=%v", item)
		}
	}
}

func TestUserAudiencePreferenceFilteringPreservesGroupReceiverKinds(t *testing.T) {
	service := &Service{}
	request := facade.PublishRequest{
		OrganizationID: 1,
		EventKey:       "workforce.task.action_required",
		ReceiverKinds: []facade.ReceiverKind{
			facade.ReceiverKindInApp,
			facade.ReceiverKindFeishuGroup,
			facade.ReceiverKindWeComGroup,
		},
		UserAudiences: []facade.UserAudiencePlan{{UserID: 2, Role: facade.UserAudienceSubscriber, ReceiverKinds: []facade.ReceiverKind{facade.ReceiverKindInApp}}},
	}
	got, err := service.applyUserAudiencePreferences(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	want := []facade.ReceiverKind{facade.ReceiverKindFeishuGroup, facade.ReceiverKindWeComGroup, facade.ReceiverKindInApp}
	if !reflect.DeepEqual(got.ReceiverKinds, want) {
		t.Fatalf("receiver kinds=%v want=%v", got.ReceiverKinds, want)
	}
}

func TestMissingWorkPreferenceDefaultsToAllowedAndDisabledPreferenceOnlySuppressesFuturePersonalDelivery(t *testing.T) {
	repository := &workPreferenceTestRepository{items: map[string]facade.WorkPreference{}}
	service := newWorkPreferenceTestService(t, repository)
	ctx := context.Background()

	request := facade.PublishRequest{
		OrganizationID: 1,
		EventKey:       "workforce.task.action_required",
		ReceiverKinds: []facade.ReceiverKind{
			facade.ReceiverKindFeishuGroup,
			facade.ReceiverKindWeComGroup,
		},
		UserAudiences: []facade.UserAudiencePlan{{
			UserID: 2,
			Role:   facade.UserAudienceSubscriber,
			ReceiverKinds: []facade.ReceiverKind{
				facade.ReceiverKindInApp,
				facade.ReceiverKindEmailUser,
				facade.ReceiverKindFeishuOpenID,
				facade.ReceiverKindWeComUserID,
			},
		}},
	}
	allowed, err := service.applyUserAudiencePreferences(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(allowed.UserAudiences[0].ReceiverKinds, request.UserAudiences[0].ReceiverKinds) {
		t.Fatalf("missing preference unexpectedly suppressed channels: %#v", allowed.UserAudiences[0])
	}

	if _, err = service.UpdateMyWorkPreference(ctx, 1, 2, "TASK_ACTION_REQUIRED", "EMAIL_USER", facade.WorkPreferenceUpdate{Enabled: false, ExpectedVersion: 0}); err != nil {
		t.Fatalf("disable email preference: %v", err)
	}
	filtered, err := service.applyUserAudiencePreferences(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	wantPersonal := []facade.ReceiverKind{facade.ReceiverKindInApp, facade.ReceiverKindFeishuOpenID, facade.ReceiverKindWeComUserID}
	if !reflect.DeepEqual(filtered.UserAudiences[0].ReceiverKinds, wantPersonal) {
		t.Fatalf("personal filtering=%v want=%v", filtered.UserAudiences[0].ReceiverKinds, wantPersonal)
	}
	for _, group := range []facade.ReceiverKind{facade.ReceiverKindFeishuGroup, facade.ReceiverKindWeComGroup} {
		if !containsReceiverKind(filtered.ReceiverKinds, group) {
			t.Fatalf("disabled personal preference removed group route %s", group)
		}
	}
}

func TestAttentionInAppIsMandatoryEvenWhenOwnerDisabledIt(t *testing.T) {
	repository := &workPreferenceTestRepository{items: map[string]facade.WorkPreference{}}
	service := newWorkPreferenceTestService(t, repository)
	ctx := context.Background()
	if _, err := service.UpdateMyWorkPreference(ctx, 1, 9, "TASK_ACTION_REQUIRED", "IN_APP", facade.WorkPreferenceUpdate{Enabled: false, ExpectedVersion: 0}); err != nil {
		t.Fatalf("disable in-app preference: %v", err)
	}
	got, err := service.applyUserAudiencePreferences(ctx, facade.PublishRequest{
		OrganizationID: 1,
		EventKey:       "workforce.task.action_required",
		UserAudiences: []facade.UserAudiencePlan{{
			UserID:        9,
			Role:          facade.UserAudienceAttention,
			ReceiverKinds: []facade.ReceiverKind{facade.ReceiverKindInApp, facade.ReceiverKindEmailUser},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !containsReceiverKind(got.UserAudiences[0].ReceiverKinds, facade.ReceiverKindInApp) {
		t.Fatalf("attention recipient lost mandatory in-app delivery: %#v", got.UserAudiences[0])
	}
}

func containsReceiverKind(items []facade.ReceiverKind, want facade.ReceiverKind) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
