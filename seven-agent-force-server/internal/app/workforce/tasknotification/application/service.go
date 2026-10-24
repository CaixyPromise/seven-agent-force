package application

import (
	"context"
	"errors"
	notificationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/infrastructure"
)

var ErrForbidden = errors.New("TASK_NOTIFICATION_FORBIDDEN")

type Service struct {
	store          Repository
	inAppAvailable bool
	platform       PlatformReadinessResolver
}

type PlatformReadinessResolver interface {
	ResolveReadinessForOrganization(context.Context, int64, string, int64) (*notificationfacade.Readiness, error)
	ListAvailableGroupRoutes(context.Context, int64, int64, string) ([]notificationfacade.GroupRouteOption, error)
	ResolvePublicationStatus(context.Context, int64, string, int64) (*notificationfacade.PublicationStatus, error)
}

type Repository interface {
	TaskCreator(context.Context, int64, int64) (int64, error)
	GetSubscription(context.Context, int64, int64, int64) (domain.Subscription, error)
	UpdateSubscription(context.Context, int64, int64, int64, int64, int64, domain.SubscriptionCommand) (domain.Subscription, error)
	ListRecords(context.Context, int64, int64, int64, int) ([]domain.Record, error)
}

func NewService(store Repository, inAppAvailable bool) *Service {
	return &Service{store: store, inAppAvailable: inAppAvailable}
}

func (s *Service) BindPlatformReadiness(resolver PlatformReadinessResolver) {
	if s != nil {
		s.platform = resolver
	}
}

type Readiness struct {
	InApp       ChannelReadiness                      `json:"inApp"`
	Email       ChannelReadiness                      `json:"email"`
	Feishu      ChannelReadiness                      `json:"feishu"`
	WeCom       ChannelReadiness                      `json:"wecom"`
	GroupRoutes []notificationfacade.GroupRouteOption `json:"groupRoutes"`
}

type ChannelReadiness struct {
	Available    bool   `json:"available"`
	ReasonCode   string `json:"reasonCode,omitempty"`
	DirectReady  bool   `json:"directReady,omitempty"`
	MentionReady bool   `json:"mentionReady,omitempty"`
}

func (s *Service) ResolveReadiness(ctx context.Context, tenantID, userID int64) (Readiness, error) {
	item := Readiness{InApp: ChannelReadiness{Available: s != nil && s.inAppAvailable}}
	if s != nil && s.platform != nil && userID > 0 {
		platform, err := s.platform.ResolveReadinessForOrganization(ctx, tenantID, "workforce_task_action_required", userID)
		if err != nil {
			return Readiness{}, err
		}
		if platform != nil {
			item.InApp.Available = platform.InApp.Available
			item.InApp.ReasonCode = platform.InApp.ReasonCode
			item.Email = ChannelReadiness{Available: platform.Email.Available && platform.Email.RecipientReady, ReasonCode: platform.Email.ReasonCode}
			item.Feishu = ChannelReadiness{Available: platform.Feishu.DirectReady, DirectReady: platform.Feishu.DirectReady, MentionReady: platform.Feishu.MentionReady, ReasonCode: platform.Feishu.ReasonCode}
			item.WeCom = ChannelReadiness{Available: platform.WeCom.DirectReady, DirectReady: platform.WeCom.DirectReady, MentionReady: platform.WeCom.MentionReady, ReasonCode: platform.WeCom.ReasonCode}
		}
		routes, routeErr := s.platform.ListAvailableGroupRoutes(ctx, tenantID, userID, "workforce_task_action_required")
		if routeErr != nil {
			return Readiness{}, routeErr
		}
		item.GroupRoutes = routes
	}
	if !item.InApp.Available {
		if item.InApp.ReasonCode == "" {
			item.InApp.ReasonCode = "TASK_NOTIFICATION_PUBLISH_RETRYABLE"
		}
	}
	return item, nil
}

func (s *Service) GetSubscription(ctx context.Context, tenantID, taskID, actorUserID int64, _ bool) (domain.Subscription, error) {
	creator, err := s.resolveCreator(ctx, tenantID, taskID, actorUserID)
	if err != nil {
		return domain.Subscription{}, err
	}
	return s.store.GetSubscription(ctx, tenantID, taskID, creator)
}

func (s *Service) UpdateSubscription(ctx context.Context, tenantID, taskID, actorUserID, expectedVersion int64, operator bool, command domain.SubscriptionCommand) (domain.Subscription, error) {
	creator, err := s.authorize(ctx, tenantID, taskID, actorUserID, operator)
	if err != nil {
		return domain.Subscription{}, err
	}
	if _, validateErr := domain.NewSubscription(tenantID, taskID, creator, command); validateErr != nil {
		return domain.Subscription{}, domain.ErrFactInvalid
	}
	if validateErr := s.validateChannelReadiness(ctx, tenantID, creator, command); validateErr != nil {
		return domain.Subscription{}, validateErr
	}
	return s.store.UpdateSubscription(ctx, tenantID, taskID, creator, actorUserID, expectedVersion, command)
}

func (s *Service) validateChannelReadiness(ctx context.Context, tenantID, userID int64, command domain.SubscriptionCommand) error {
	needs := false
	groups := map[int64]struct{}{}
	for _, c := range command.Channels {
		if c.Type != domain.ChannelInApp {
			needs = true
		}
		if c.Type == domain.ChannelGroupRoute {
			groups[c.GroupRouteID] = struct{}{}
		}
	}
	if !needs {
		return nil
	}
	if s.platform == nil {
		return domain.ErrFactInvalid
	}
	ready, err := s.ResolveReadiness(ctx, tenantID, userID)
	if err != nil {
		return err
	}
	for _, c := range command.Channels {
		switch c.Type {
		case domain.ChannelEmailUser:
			if !ready.Email.Available {
				return domain.ErrRecipientInvalid
			}
		case domain.ChannelFeishuDirect:
			if !ready.Feishu.DirectReady {
				return domain.ErrRecipientInvalid
			}
		case domain.ChannelWeComDirect:
			if !ready.WeCom.DirectReady {
				return domain.ErrRecipientInvalid
			}
		}
	}
	allowed := map[int64]struct{}{}
	for _, r := range ready.GroupRoutes {
		allowed[r.ID] = struct{}{}
	}
	for id := range groups {
		if _, ok := allowed[id]; !ok {
			return domain.ErrFactInvalid
		}
	}
	return nil
}

type RecordView struct {
	domain.Record
	Deliveries []notificationfacade.PublicationChannelStatus `json:"deliveries"`
}

func (s *Service) ListRecords(ctx context.Context, tenantID, taskID, actorUserID int64, _ bool) ([]RecordView, error) {
	creator, err := s.resolveCreator(ctx, tenantID, taskID, actorUserID)
	if err != nil {
		return nil, err
	}
	items, err := s.store.ListRecords(ctx, tenantID, taskID, creator, 50)
	if err != nil {
		return nil, err
	}
	result := make([]RecordView, 0, len(items))
	for _, item := range items {
		view := RecordView{Record: item, Deliveries: []notificationfacade.PublicationChannelStatus{}}
		if s.platform != nil && item.NotificationReceiptIdentity != "" {
			status, statusErr := s.platform.ResolvePublicationStatus(ctx, tenantID, item.NotificationReceiptIdentity, creator)
			if statusErr == nil && status != nil {
				view.Deliveries = status.Channels
			}
		}
		result = append(result, view)
	}
	return result, nil
}

// Query endpoints are already protected by workforce:task:query. Resolving the
// creator here preserves tenant fencing while allowing tenant auditors to read
// the creator's notification policy and low-sensitivity delivery history.
// Mutation remains creator-owned unless the caller has the dedicated operate
// permission.
func (s *Service) resolveCreator(ctx context.Context, tenantID, taskID, actorUserID int64) (int64, error) {
	if s == nil || s.store == nil || tenantID <= 0 || taskID <= 0 || actorUserID <= 0 {
		return 0, infrastructure.ErrNotFound
	}
	return s.store.TaskCreator(ctx, tenantID, taskID)
}

func (s *Service) authorize(ctx context.Context, tenantID, taskID, actorUserID int64, operator bool) (int64, error) {
	if s == nil || s.store == nil || tenantID <= 0 || taskID <= 0 || actorUserID <= 0 {
		return 0, infrastructure.ErrNotFound
	}
	creator, err := s.store.TaskCreator(ctx, tenantID, taskID)
	if err != nil {
		return 0, err
	}
	if creator != actorUserID && !operator {
		return 0, ErrForbidden
	}
	return creator, nil
}
