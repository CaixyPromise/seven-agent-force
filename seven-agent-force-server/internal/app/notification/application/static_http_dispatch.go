package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
)

func staticHTTPSnapshotFromChannel(id int64, deliveryID string, channel domain.Channel, scopeID string) (*domain.HTTPDeliverySnapshot, error) {
	if !domain.IsStaticHTTPChannelType(channel.ChannelType) {
		return nil, fmt.Errorf("notification channel is not a static HTTP channel")
	}
	if strings.TrimSpace(channel.ScopeID) != strings.TrimSpace(scopeID) || strings.TrimSpace(channel.ChannelCode) == "" {
		return nil, ErrNotificationScopeMismatch
	}
	if channel.ChannelType == domain.ChannelTypeHTTPConnector {
		config, err := domain.ParseHTTPConnectorConfig(channel.ConfigJSON)
		if err != nil {
			return nil, fmt.Errorf("HTTP connector snapshot configuration is invalid")
		}
		if config.Authentication.Mode != domain.HTTPConnectorAuthNone && (strings.TrimSpace(channel.SecretCiphertext) == "" || strings.TrimSpace(channel.SecretEDEK) == "" || strings.TrimSpace(channel.SecretWrapKeyRef) == "") {
			return nil, fmt.Errorf("HTTP connector snapshot secret is not configured")
		}
	} else if _, err := domain.ParseWebhookProfileConfig(channel.ConfigJSON); err != nil {
		return nil, fmt.Errorf("webhook profile snapshot configuration is invalid")
	}
	if domain.IsWebhookProfileChannelType(channel.ChannelType) && (strings.TrimSpace(channel.SecretCiphertext) == "" || strings.TrimSpace(channel.SecretEDEK) == "" || strings.TrimSpace(channel.SecretWrapKeyRef) == "") {
		return nil, fmt.Errorf("webhook profile snapshot secret is not configured")
	}
	return &domain.HTTPDeliverySnapshot{
		ID:               id,
		DeliveryID:       deliveryID,
		ScopeID:          scopeID,
		ChannelCode:      channel.ChannelCode,
		ChannelType:      channel.ChannelType,
		ChannelPriority:  channel.Priority,
		ConfigJSON:       channel.ConfigJSON,
		SecretCiphertext: channel.SecretCiphertext,
		SecretEDEK:       channel.SecretEDEK,
		SecretWrapKeyRef: channel.SecretWrapKeyRef,
	}, nil
}

// dispatchStaticHTTP uses the immutable accepted connection snapshot while it
// still consults the live channel only for scope, type and enabled rollback.
// The static route has no platform recipient or G4 ExternalTarget, so it can
// never materialize inbox state or emit a mailbox/SSE change.
func (s *Service) dispatchStaticHTTP(ctx context.Context, delivery *domain.Delivery, channel *domain.Channel) error {
	if delivery == nil || !domain.IsStaticHTTPChannelType(delivery.ChannelType) {
		return fmt.Errorf("static HTTP delivery is invalid")
	}
	if channel == nil || channel.Status != domain.ChannelStatusEnabled || !s.channelBelongsToCurrentScope(channel) || channel.ChannelCode != delivery.ChannelCode || channel.ChannelType != delivery.ChannelType {
		return s.finishExternalFailure(ctx, delivery, DriverResult{Status: DriverResultFailed, FailureClass: "CONNECTION_UNAVAILABLE", Diagnostic: "CONNECTION_UNAVAILABLE"})
	}
	snapshot, err := s.repo.FindHTTPDeliverySnapshotByDeliveryID(ctx, delivery.DeliveryID)
	if err != nil {
		return err
	}
	if snapshot == nil || snapshot.ScopeID != s.scopeID || snapshot.ChannelCode != delivery.ChannelCode || snapshot.ChannelType != delivery.ChannelType {
		return s.finishExternalFailure(ctx, delivery, DriverResult{Status: DriverResultFailed, FailureClass: "SNAPSHOT_INVALID", Diagnostic: "SNAPSHOT_INVALID"})
	}
	mention, suppressGroup, groupErr := s.resolveGroupRouteDispatch(ctx, delivery, channel)
	if groupErr != nil {
		return groupErr
	}
	if suppressGroup {
		return s.suppressExternalDelivery(ctx, delivery, domain.ErrGroupRouteNotEnabled.Error())
	}
	snapshotChannel := *channel
	snapshotChannel.Priority = snapshot.ChannelPriority
	snapshotChannel.ConfigJSON = snapshot.ConfigJSON
	snapshotChannel.SecretCiphertext = snapshot.SecretCiphertext
	snapshotChannel.SecretEDEK = snapshot.SecretEDEK
	snapshotChannel.SecretWrapKeyRef = snapshot.SecretWrapKeyRef
	secret, err := s.decryptSecret(ctx, snapshotChannel)
	if err != nil {
		return s.finishExternalFailure(ctx, delivery, DriverResult{Status: DriverResultFailed, FailureClass: "SNAPSHOT_DECRYPT", Diagnostic: "SNAPSHOT_DECRYPT"})
	}
	if s.drivers == nil {
		return s.finishExternalFailure(ctx, delivery, DriverResult{Status: DriverResultFailed, FailureClass: "DRIVER_UNAVAILABLE", Diagnostic: "DRIVER_UNAVAILABLE"})
	}
	driver, ok := s.drivers.Driver(snapshotChannel.ChannelType).(ResultChannelDriver)
	if !ok || driver == nil {
		return s.finishExternalFailure(ctx, delivery, DriverResult{Status: DriverResultFailed, FailureClass: "DRIVER_UNAVAILABLE", Diagnostic: "DRIVER_UNAVAILABLE"})
	}
	variables := map[string]any{}
	_ = json.Unmarshal([]byte(delivery.PayloadJSON), &variables)
	eventKey := delivery.SceneCode
	category := ""
	priority := strconv.Itoa(snapshot.ChannelPriority)
	deepLink := ""
	if delivery.NotificationID != nil {
		notification, findErr := s.repo.FindLogicalNotificationByID(ctx, *delivery.NotificationID)
		if findErr != nil || notification == nil || notification.ScopeID != s.scopeID {
			return s.finishExternalFailure(ctx, delivery, DriverResult{Status: DriverResultFailed, FailureClass: "NOTIFICATION_INVALID", Diagnostic: "NOTIFICATION_INVALID"})
		}
		eventKey = notification.EventKey
		category = notification.Category
		priority = notification.Priority
		deepLink = notification.DeepLink
	}
	result, sendErr := driver.SendResult(ctx, DriverMessage{
		Channel:           snapshotChannel,
		SecretPlain:       secret,
		Subject:           delivery.RenderedSubject,
		Text:              delivery.RenderedText,
		HTML:              delivery.RenderedHTML,
		Markdown:          delivery.RenderedMarkdown,
		Variables:         variables,
		DeliveryID:        delivery.DeliveryID,
		EventKey:          eventKey,
		Category:          category,
		Priority:          priority,
		TraceID:           delivery.TraceID,
		DeepLink:          deepLink,
		PrimaryAction:     frozenPrimaryAction(delivery, deepLink),
		ExternalActionURL: delivery.ActionURLSnapshot,
		Presentation:      delivery.Presentation,
		Mention:           mention,
	})
	if sendErr != nil && strings.TrimSpace(result.Status) == "" {
		result = DriverResult{Status: DriverResultUnknown, FailureClass: "TRANSPORT", Diagnostic: "HTTP_RESPONSE_UNCONFIRMED"}
	}
	result = normalizeExternalDriverResult(result)
	switch result.Status {
	case DriverResultProviderAccepted:
		return s.finishExternalAccepted(ctx, delivery, result)
	case DriverResultUnknown:
		return s.finishExternalUnknown(ctx, delivery, result)
	default:
		// Only drivers that prove a failure occurred before a request could
		// reach the receiver may opt into the bounded retry path. A timeout,
		// dropped response, or any UNKNOWN result remains non-replayable.
		if result.Retryable {
			return s.failOrRetryWithAttempt(ctx, delivery, result.Diagnostic, s.externalAttempt(delivery, result))
		}
		return s.finishExternalFailure(ctx, delivery, result)
	}
}
