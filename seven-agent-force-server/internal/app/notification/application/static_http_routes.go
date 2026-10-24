package application

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

const staticRouteLimit = 100

// preparedStaticRoute is an accepted, operator-owned static connection. It
// has no recipient information; the connection's immutable snapshot carries
// all outbound capability when the delivery is created.
type preparedStaticRoute struct {
	channel    domain.Channel
	groupRoute *preparedGroupRouteMetadata
}

func (s *Service) prepareStaticRoutes(ctx context.Context, routes []facade.StaticRoute) ([]preparedStaticRoute, error) {
	references, err := normalizeStaticRouteReferences(routes)
	if err != nil {
		return nil, err
	}
	channels, err := s.loadChannelsByCodes(ctx, references)
	if err != nil {
		return nil, err
	}
	prepared := make([]preparedStaticRoute, 0, len(references))
	for _, reference := range references {
		channel, found := channels[reference]
		if !found || channel.Status != domain.ChannelStatusEnabled || !s.channelBelongsToCurrentScope(&channel) || !domain.IsStaticHTTPChannelType(channel.ChannelType) {
			return nil, apperrors.Operation("受控 HTTP 连接不可用")
		}
		if _, validateErr := staticHTTPSnapshotFromChannel(1, "accepted-static-route", channel, s.scopeID); validateErr != nil {
			return nil, apperrors.Operation("受控 HTTP 连接配置不完整")
		}
		prepared = append(prepared, preparedStaticRoute{channel: channel})
	}
	return prepared, nil
}

// TestStaticConnection performs a privileged, non-persistent probe of one
// saved HTTP Connector or compiled group webhook. It reuses the same guarded
// driver as asynchronous delivery, but it never creates a notification,
// delivery, Outbox event, inbox recipient, or realtime hint.
func (s *Service) TestStaticConnection(ctx context.Context, request facade.StaticConnectionTestRequest) (*facade.StaticConnectionTestResult, error) {
	if s == nil || s.repo == nil || s.drivers == nil {
		return nil, fmt.Errorf("notification service is not configured")
	}
	items, err := s.prepareStaticRoutes(ctx, []facade.StaticRoute{{ConnectionRef: request.ConnectionRef}})
	if err != nil {
		return nil, err
	}
	if len(items) != 1 {
		return nil, fmt.Errorf("static notification connection is invalid")
	}
	channel := items[0].channel
	secretPlain, err := s.decryptSecret(ctx, channel)
	if err != nil {
		return nil, apperrors.Operation("受控 HTTP 连接密钥不可用")
	}
	driver, ok := s.drivers.Driver(channel.ChannelType).(ResultChannelDriver)
	if !ok || driver == nil {
		return nil, apperrors.Operation("受控 HTTP 连接暂不可测试")
	}
	text := strings.TrimSpace(request.Text)
	if text == "" {
		text = "连接测试消息"
	}
	result, sendErr := driver.SendResult(ctx, DriverMessage{
		Channel:     channel,
		SecretPlain: secretPlain,
		Subject:     "连接测试",
		Text:        externalProbeText(text),
		DeliveryID:  "probe-" + s.nextStringID(),
		EventKey:    "notification.connection.probe",
		Category:    "SYSTEM",
		Priority:    "NORMAL",
		Probe:       true,
	})
	if sendErr != nil && strings.TrimSpace(result.Status) == "" {
		result = DriverResult{Status: DriverResultUnknown, FailureClass: "TRANSPORT", Diagnostic: "HTTP_RESPONSE_UNCONFIRMED"}
	}
	result = normalizeExternalDriverResult(result)
	s.logStaticConnectionProbeFailure(channel.ChannelType, result)
	return &facade.StaticConnectionTestResult{
		Status:            result.Status,
		FailureClass:      result.FailureClass,
		ProviderReference: result.ProviderReference,
		Diagnostic:        result.Diagnostic,
		ProviderError:     facadeProviderError(result.ProviderError),
	}, nil
}

func normalizeStaticRouteReferences(routes []facade.StaticRoute) ([]string, error) {
	if len(routes) > staticRouteLimit {
		return nil, apperrors.Params("静态通知连接数量超过单次上限")
	}
	seen := make(map[string]struct{}, len(routes))
	references := make([]string, 0, len(routes))
	for _, route := range routes {
		reference := strings.TrimSpace(route.ConnectionRef)
		if reference == "" || len(reference) > 64 {
			return nil, apperrors.Params("静态通知连接引用无效")
		}
		if _, exists := seen[reference]; exists {
			return nil, apperrors.Params("同一静态通知连接不能重复选择")
		}
		seen[reference] = struct{}{}
		references = append(references, reference)
	}
	sort.Strings(references)
	return references, nil
}

func (s *Service) staticRouteFingerprints(items []preparedStaticRoute) []domain.StaticRouteFingerprint {
	result := make([]domain.StaticRouteFingerprint, 0, len(items))
	for _, item := range items {
		result = append(result, domain.StaticRouteFingerprint{ConnectionRef: item.channel.ChannelCode, ProviderCode: item.channel.ChannelType})
	}
	return result
}

func (s *Service) matchExistingStaticRoutes(ctx context.Context, notification *domain.LogicalNotification, routes []facade.StaticRoute) ([]domain.StaticRouteFingerprint, error) {
	references, err := normalizeStaticRouteReferences(routes)
	if err != nil {
		return nil, err
	}
	if notification == nil || notification.ID <= 0 {
		return nil, fmt.Errorf("notification static route idempotency state is invalid")
	}
	deliveries, err := s.repo.ListDeliveriesByNotificationID(ctx, notification.ID)
	if err != nil {
		return nil, err
	}
	stored := make(map[string]domain.StaticRouteFingerprint)
	for _, delivery := range deliveries {
		if domain.IsStaticHTTPChannelType(delivery.ChannelType) {
			stored[delivery.ChannelCode] = domain.StaticRouteFingerprint{ConnectionRef: delivery.ChannelCode, ProviderCode: delivery.ChannelType}
		}
	}
	if len(stored) != len(references) {
		return nil, idempotencyConflict()
	}
	result := make([]domain.StaticRouteFingerprint, 0, len(references))
	for _, reference := range references {
		fingerprint, exists := stored[reference]
		if !exists {
			return nil, idempotencyConflict()
		}
		result = append(result, fingerprint)
	}
	return result, nil
}

func (s *Service) createStaticRouteDeliveries(ctx context.Context, notification *domain.LogicalNotification, items []preparedStaticRoute) error {
	return s.createStaticRouteDeliveriesWithScene(ctx, notification, items, nil)
}

// createStaticRouteDeliveriesWithScene freezes an optional G6.2 scene render
// alongside the existing complete HTTP connection snapshot. The live channel
// remains only an emergency enabled/scope gate at dispatch time.
func (s *Service) createStaticRouteDeliveriesWithScene(ctx context.Context, notification *domain.LogicalNotification, items []preparedStaticRoute, renders map[string]sceneDeliveryRender) error {
	if len(items) == 0 {
		return nil
	}
	if len(items) > staticRouteLimit {
		return apperrors.Params("静态通知连接数量超过单次上限")
	}
	if notification == nil || notification.ID <= 0 || strings.TrimSpace(notification.ScopeID) == "" {
		return fmt.Errorf("notification static route has no logical notification")
	}
	deliveryBatch, ok := s.repo.(deliveryBatchRepository)
	if !ok {
		return fmt.Errorf("notification delivery batch repository is not configured")
	}
	snapshotBatch, ok := s.repo.(httpDeliverySnapshotBatchRepository)
	if !ok {
		return fmt.Errorf("notification HTTP delivery snapshot batch repository is not configured")
	}
	outboxBatch, ok := s.repo.(outboxBatchRepository)
	if !ok {
		return fmt.Errorf("notification outbox batch repository is not configured")
	}
	deliveries := make([]domain.Delivery, 0, len(items))
	snapshots := make([]domain.HTTPDeliverySnapshot, 0, len(items))
	outboxEvents := make([]domain.OutboxEvent, 0, len(items))
	groupSnapshots := make([]domain.GroupRouteSnapshot, 0)
	for _, item := range items {
		render, hasRender := renders[item.channel.ChannelCode]
		deliveryID := "ntf_http_" + s.nextStringID()
		notificationID := notification.ID
		createdAt := s.now().UTC()
		delivery := domain.Delivery{
			ID:              s.nextID(),
			DeliveryID:      deliveryID,
			RequestDigest:   digest("static", notification.NotificationID, item.channel.ChannelCode, item.channel.ChannelType),
			NotificationID:  &notificationID,
			SceneCode:       externalDeliverySceneCode(notification.EventKey),
			ChannelCode:     item.channel.ChannelCode,
			ChannelType:     item.channel.ChannelType,
			TemplateCode:    "semantic-static",
			RenderedSubject: notification.Title,
			RenderedText:    notification.Content,
			Status:          domain.DeliveryStatusPending,
			// HTTP routes never replay an ambiguous response. They do, however,
			// retry a small bounded number of explicitly pre-dial failures (for
			// example DNS resolution before any socket is opened), using the same
			// persisted Idempotency-Key.
			MaxRetry:   externalDeliveryMaxRetry,
			TraceID:    notification.TraceID,
			CreatorID:  notification.CreatorID,
			CreateTime: createdAt,
			UpdateTime: createdAt,
		}
		if hasRender {
			delivery.SceneCode = render.SceneCode
			delivery.TemplateCode = render.TemplateCode
			delivery.SceneSnapshotID = render.SceneSnapshotID
			delivery.RenderedSubject = render.RenderedSubject
			delivery.RenderedText = render.RenderedText
			delivery.RenderedHTML = render.RenderedHTML
			delivery.RenderedMarkdown = render.RenderedMarkdown
			delivery.ContentTier = render.ContentTier
		}
		actionLabel := ""
		if hasRender {
			actionLabel = render.PrimaryActionLabel
		}
		presentation, frozenLabel, actionURL, renderedDigest, freezeErr := s.freezeDeliveryPresentation(notification, item.channel.ChannelType, actionLabel)
		if freezeErr != nil {
			return freezeErr
		}
		delivery.Presentation = presentation
		delivery.ActionLabelSnapshot = frozenLabel
		delivery.ActionURLSnapshot = actionURL
		mentionDigest := ""
		if item.groupRoute != nil && item.groupRoute.mentionAuthority != nil {
			mentionDigest = item.groupRoute.mentionAuthority.authorityDigest
		}
		delivery.RenderedPayloadDigest = digest(renderedDigest, delivery.RenderedSubject, delivery.RenderedText, delivery.RenderedHTML, delivery.RenderedMarkdown, mentionDigest)
		if item.groupRoute != nil {
			groupSnapshots = append(groupSnapshots, s.newGroupRouteSnapshot(notification, delivery, *item.groupRoute))
		}
		snapshot, err := staticHTTPSnapshotFromChannel(s.nextID(), deliveryID, item.channel, notification.ScopeID)
		if err != nil {
			return fmt.Errorf("snapshot static notification route: %w", err)
		}
		message := domain.DeliveryMessage{MessageID: "notification:" + deliveryID, DeliveryID: deliveryID, ScopeID: notification.ScopeID}
		deliveries = append(deliveries, delivery)
		snapshots = append(snapshots, *snapshot)
		outboxEvents = append(outboxEvents, domain.OutboxEvent{
			ID:            s.nextID(),
			EventID:       message.MessageID,
			ScopeID:       notification.ScopeID,
			EventType:     domain.OutboxEventNotificationDispatch,
			AggregateType: domain.OutboxAggregateNotification,
			AggregateID:   deliveryID,
			Payload:       mustJSON(message),
			Status:        "PENDING",
		})
	}
	if err := deliveryBatch.InsertDeliveries(ctx, deliveries); err != nil {
		return err
	}
	if err := snapshotBatch.InsertHTTPDeliverySnapshots(ctx, snapshots); err != nil {
		return err
	}
	if len(groupSnapshots) > 0 {
		repo, ok := s.repo.(groupRouteSnapshotRepository)
		if !ok {
			return fmt.Errorf("notification group route snapshot repository is not configured")
		}
		if err := repo.InsertGroupRouteSnapshots(ctx, groupSnapshots); err != nil {
			return err
		}
	}
	return outboxBatch.AppendOutboxBatch(ctx, outboxEvents)
}
