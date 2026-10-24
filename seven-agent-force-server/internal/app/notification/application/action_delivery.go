package application

import (
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
)

func (s *Service) freezeDeliveryPresentation(notification *domain.LogicalNotification, channelType, actionLabel string) (presentation, frozenLabel, frozenURL, renderedDigest string, err error) {
	capabilities := driverCapabilitiesFor(s.drivers, channelType)
	presentation, err = domain.SelectDeliveryPresentation(domain.DeliveryPresentationAuto, capabilities)
	if err != nil {
		return "", "", "", "", err
	}
	frozenLabel = strings.TrimSpace(actionLabel)
	if frozenLabel == "" && notification != nil {
		frozenLabel = strings.TrimSpace(notification.ActionLabelSnapshot)
	}
	if frozenLabel != "" && notification != nil && strings.TrimSpace(notification.DeepLink) != "" && capabilities.SupportsPrimaryAction && capabilities.SupportsExternalURL {
		link, parseErr := domain.ParseInternalDeepLink(notification.DeepLink)
		if parseErr != nil {
			return "", "", "", "", parseErr
		}
		if s.publicWebBaseURL.Configured() {
			frozenURL, err = s.publicWebBaseURL.Resolve(link)
			if err != nil {
				return "", "", "", "", err
			}
		}
	}
	renderedDigest = digest("delivery-presentation-v1", presentation, frozenLabel, frozenURL)
	return presentation, frozenLabel, frozenURL, renderedDigest, nil
}

func driverCapabilitiesFor(registry DriverRegistry, channelType string) domain.DriverCapabilities {
	if capable, ok := registry.(domain.CapabilityRegistry); ok && capable != nil {
		if result := capable.Capabilities(channelType); len(result.SupportedPresentations) > 0 {
			return result
		}
	}
	return domain.DriverCapabilities{SupportedPresentations: []string{domain.DeliveryPresentationText}}
}

func frozenPrimaryAction(delivery *domain.Delivery, deepLink string) *domain.NotificationAction {
	if delivery == nil || strings.TrimSpace(delivery.ActionLabelSnapshot) == "" || strings.TrimSpace(deepLink) == "" {
		return nil
	}
	link, err := domain.ParseInternalDeepLink(deepLink)
	if err != nil {
		return nil
	}
	action, err := domain.NewPrimaryInternalAction(delivery.ActionLabelSnapshot, link)
	if err != nil {
		return nil
	}
	return &action
}
