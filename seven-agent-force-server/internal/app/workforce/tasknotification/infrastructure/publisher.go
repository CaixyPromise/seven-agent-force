package infrastructure

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"go.uber.org/zap"

	notificationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/domain"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

type PublicationStore interface {
	ClaimPublications(context.Context, string, time.Time, time.Duration, int) ([]Publication, error)
	CompletePublication(context.Context, Publication, string, string, time.Time) error
	RetryPublication(context.Context, Publication, string, time.Time) error
	FailPublication(context.Context, Publication, string, time.Time) error
}

type Publisher struct {
	store  PublicationStore
	client notificationfacade.NotificationClient
	now    func() time.Time
	log    *zap.Logger
}

func NewPublisher(store PublicationStore, client notificationfacade.NotificationClient) *Publisher {
	return &Publisher{store: store, client: client, now: time.Now, log: zap.NewNop()}
}

// SetLogger installs the server logger used only for bounded publication
// diagnostics. The record ID and normalized error are safe correlation data;
// the publisher never logs recipient identities, rendered content, deep links,
// provider payloads, or credentials.
func (p *Publisher) SetLogger(log *zap.Logger) {
	if p != nil && log != nil {
		p.log = log.Named("workforce_task_notification_publisher")
	}
}

func (p *Publisher) Run(ctx context.Context) {
	if p == nil || p.store == nil || p.client == nil {
		return
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		_ = p.PublishBatch(ctx, 32)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *Publisher) PublishBatch(ctx context.Context, limit int) error {
	if p == nil || p.store == nil || p.client == nil || limit <= 0 {
		return domain.ErrPublishRetryable
	}
	leaseToken, err := newLeaseToken()
	if err != nil {
		return err
	}
	items, err := p.store.ClaimPublications(ctx, leaseToken, p.now().UTC(), 30*time.Second, limit)
	if err != nil {
		return err
	}
	var firstErr error
	for _, item := range items {
		receipt, publishErr := p.PublishRecord(ctx, item.Record)
		if publishErr != nil {
			p.logger().Warn("task_notification_publish_failed",
				zap.Int64("record_id", item.Record.ID),
				zap.String("fact_type", string(item.Record.FactType)),
				zap.String("scene_code", item.Record.SceneCode),
				zap.String("reason_code", stablePublishErrorCode(publishErr)),
				zap.Error(publishErr),
			)
			if permanentPublicationError(publishErr) {
				if failErr := p.store.FailPublication(ctx, item, stablePublishErrorCode(publishErr), p.now().UTC()); failErr != nil && firstErr == nil {
					firstErr = failErr
				} else if firstErr == nil {
					firstErr = publishErr
				}
			} else if retryErr := p.store.RetryPublication(ctx, item, stablePublishErrorCode(publishErr), p.now().UTC()); retryErr != nil && firstErr == nil {
				firstErr = retryErr
			} else if firstErr == nil {
				firstErr = publishErr
			}
			continue
		}
		warning := ""
		if len(receipt.Warnings) != 0 {
			warning = "NOTIFICATION_PROVIDER_PARAMETER_WARNING"
		}
		if completeErr := p.store.CompletePublication(ctx, item, receipt.NotificationID, warning, p.now().UTC()); completeErr != nil && firstErr == nil {
			firstErr = completeErr
		}
	}
	return firstErr
}

func (p *Publisher) logger() *zap.Logger {
	if p == nil || p.log == nil {
		return zap.NewNop()
	}
	return p.log
}

func (p *Publisher) PublishRecord(ctx context.Context, record domain.Record) (*notificationfacade.PublishReceipt, error) {
	if p == nil || p.client == nil {
		return nil, domain.ErrPublishRetryable
	}
	request, err := PublishRequest(record)
	if err != nil {
		return nil, err
	}
	receipt, err := p.client.Publish(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrPublishRetryable, err)
	}
	if receipt == nil || receipt.NotificationID == "" {
		return nil, domain.ErrPublishFailed
	}
	return receipt, nil
}

func newLeaseToken() (string, error) {
	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate Task notification publication lease: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func permanentPublicationError(err error) bool {
	if errors.Is(err, domain.ErrFactInvalid) || errors.Is(err, domain.ErrDeepLinkInvalid) || errors.Is(err, domain.ErrIdempotencyConflict) || errors.Is(err, domain.ErrPublishFailed) {
		return true
	}
	var appErr apperrors.Error
	if !errors.As(err, &appErr) {
		return false
	}
	switch details := appErr.Details().(type) {
	case map[string]string:
		return details["reasonCode"] == "IDEMPOTENCY_CONFLICT"
	case map[string]any:
		value, _ := details["reasonCode"].(string)
		return value == "IDEMPOTENCY_CONFLICT"
	default:
		return false
	}
}

func stablePublishErrorCode(err error) string {
	if errors.Is(err, domain.ErrFactInvalid) {
		return domain.ErrFactInvalid.Error()
	}
	if errors.Is(err, domain.ErrDeepLinkInvalid) {
		return domain.ErrDeepLinkInvalid.Error()
	}
	if errors.Is(err, domain.ErrPublishFailed) {
		return domain.ErrPublishFailed.Error()
	}
	if permanentPublicationError(err) {
		return domain.ErrIdempotencyConflict.Error()
	}
	return domain.ErrPublishRetryable.Error()
}

func PublishRequest(record domain.Record) (notificationfacade.PublishRequest, error) {
	if record.ID <= 0 || record.TaskID <= 0 || record.TaskTurnID <= 0 || record.SubscriberUserID <= 0 || record.TurnNumber <= 0 || record.TaskNameSnapshot == "" {
		return notificationfacade.PublishRequest{}, domain.ErrFactInvalid
	}
	deepLink, err := record.DeepLink()
	if err != nil {
		return notificationfacade.PublishRequest{}, err
	}
	if record.IdempotencyIdentity == "" || record.TitleSnapshot == "" || record.ContentSnapshot == "" || record.DeepLinkSnapshot == "" {
		return notificationfacade.PublishRequest{}, domain.ErrFactInvalid
	}
	request := notificationfacade.PublishRequest{
		OrganizationID: record.TenantID,
		IdempotencyKey: record.IdempotencyIdentity, SceneCode: "",
		Audience: notificationfacade.Audience{UserIDs: []int64{record.SubscriberUserID}},
		Category: "WORKFORCE_TASK", Mandatory: false, DeepLink: deepLink,
		Title: record.TitleSnapshot, Content: record.ContentSnapshot,
	}
	if record.PublicationContractVersion >= 2 {
		variables := map[string]string{}
		if record.SceneCode == "" || json.Unmarshal([]byte(record.TemplateVariablesJSON), &variables) != nil {
			return notificationfacade.PublishRequest{}, domain.ErrFactInvalid
		}
		request.SceneCode = record.SceneCode
		request.TemplateVariables = make(map[string]any, len(variables))
		for key, value := range variables {
			request.TemplateVariables[key] = value
		}
		if len(record.Channels) == 0 {
			record.Channels = []domain.RecordChannel{{Type: domain.ChannelInApp}}
		}
		groupSeen := map[int64]struct{}{}
		mentionSeen := map[int64]struct{}{}
		subscriberKinds := make([]notificationfacade.ReceiverKind, 0, 4)
		for _, channel := range record.Channels {
			switch channel.Type {
			case domain.ChannelInApp:
				subscriberKinds = append(subscriberKinds, notificationfacade.ReceiverKindInApp)
			case domain.ChannelEmailUser:
				subscriberKinds = append(subscriberKinds, notificationfacade.ReceiverKindEmailUser)
			case domain.ChannelFeishuDirect:
				subscriberKinds = append(subscriberKinds, notificationfacade.ReceiverKindFeishuOpenID)
			case domain.ChannelWeComDirect:
				subscriberKinds = append(subscriberKinds, notificationfacade.ReceiverKindWeComUserID)
			case domain.ChannelGroupRoute:
				if channel.GroupRouteID <= 0 {
					return notificationfacade.PublishRequest{}, domain.ErrFactInvalid
				}
				if _, duplicate := groupSeen[channel.GroupRouteID]; duplicate {
					return notificationfacade.PublishRequest{}, domain.ErrFactInvalid
				}
				groupSeen[channel.GroupRouteID] = struct{}{}
				request.GroupRouteIDs = append(request.GroupRouteIDs, channel.GroupRouteID)
				if channel.MentionUserID > 0 {
					if _, duplicate := mentionSeen[channel.MentionUserID]; !duplicate {
						mentionSeen[channel.MentionUserID] = struct{}{}
						request.MentionUserIDs = append(request.MentionUserIDs, channel.MentionUserID)
					}
				}
			default:
				return notificationfacade.PublishRequest{}, domain.ErrFactInvalid
			}
		}
		if len(record.Recipients) == 0 {
			record.Recipients = []domain.RecordRecipient{{UserID: record.SubscriberUserID, Role: domain.RecipientSubscriber, PersonalChannelPolicy: domain.PersonalChannelsTaskSubscription}}
		}
		request.Audience.UserIDs = nil
		for _, recipient := range record.Recipients {
			plan := notificationfacade.UserAudiencePlan{UserID: recipient.UserID}
			switch recipient.Role {
			case domain.RecipientSubscriber:
				plan.Role = notificationfacade.UserAudienceSubscriber
				plan.ReceiverKinds = append(plan.ReceiverKinds, subscriberKinds...)
			case domain.RecipientAttention:
				plan.Role = notificationfacade.UserAudienceAttention
				plan.ReceiverKinds = []notificationfacade.ReceiverKind{notificationfacade.ReceiverKindInApp, notificationfacade.ReceiverKindEmailUser, notificationfacade.ReceiverKindFeishuOpenID, notificationfacade.ReceiverKindWeComUserID}
			default:
				return notificationfacade.PublishRequest{}, domain.ErrFactInvalid
			}
			request.UserAudiences = append(request.UserAudiences, plan)
			request.Audience.UserIDs = append(request.Audience.UserIDs, recipient.UserID)
			for _, kind := range plan.ReceiverKinds {
				if !slices.Contains(request.ReceiverKinds, kind) {
					request.ReceiverKinds = append(request.ReceiverKinds, kind)
				}
			}
		}
		if len(request.GroupRouteIDs) > 0 {
			request.ReceiverKinds = append(request.ReceiverKinds, notificationfacade.ReceiverKindFeishuGroup, notificationfacade.ReceiverKindWeComGroup)
		}
		if len(request.GroupRouteIDs) > 3 || len(request.ReceiverKinds)+len(request.GroupRouteIDs) == 0 {
			return notificationfacade.PublishRequest{}, domain.ErrFactInvalid
		}
		request.Title = ""
		request.Content = ""
	}
	switch record.FactType {
	case domain.FactActionRequired:
		request.EventKey = "workforce.task.action_required"
		request.Priority = "HIGH"
	case domain.FactCompleted:
		request.EventKey = "workforce.task.completed"
		request.Priority = "NORMAL"
	case domain.FactDeliveryCompleted:
		request.EventKey = "workforce.delivery.completed"
		request.Priority = "NORMAL"
	case domain.FactInterrupted:
		request.EventKey = "workforce.task.interrupted"
		request.Priority = "HIGH"
	default:
		return notificationfacade.PublishRequest{}, domain.ErrFactInvalid
	}
	return request, nil
}
