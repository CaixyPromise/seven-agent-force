package application

import (
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
)

func mapChannel(item domain.Channel) *facade.ChannelRecord {
	record := &facade.ChannelRecord{
		ID:               item.ID,
		ChannelCode:      item.ChannelCode,
		ChannelName:      item.ChannelName,
		ChannelType:      item.ChannelType,
		Status:           item.Status,
		Priority:         item.Priority,
		ConfigJSON:       item.ConfigJSON,
		RateLimitJSON:    item.RateLimitJSON,
		MetadataJSON:     item.MetadataJSON,
		SecretConfigured: strings.TrimSpace(item.SecretCiphertext) != "",
		CreateTime:       item.CreateTime,
		UpdateTime:       item.UpdateTime,
	}
	if item.ChannelType == domain.ChannelTypeHTTPConnector {
		// The connector's internal JSON contains the persisted secret-reference
		// shape. Expose only the typed, non-secret management contract so read
		// APIs cannot become a generic raw-config escape hatch.
		record.ConfigJSON = ""
		record.MetadataJSON = ""
		record.RateLimitJSON = ""
		if config, err := domain.ParseHTTPConnectorConfig(item.ConfigJSON); err == nil {
			mappings := make([]facade.HTTPConnectorFieldMapping, 0, len(config.FieldMappings))
			for _, mapping := range config.FieldMappings {
				mappings = append(mappings, facade.HTTPConnectorFieldMapping{Source: mapping.Source, Target: mapping.Target})
			}
			record.HTTPConnectorConfig = &facade.HTTPConnectorConfig{
				EndpointURL:         config.EndpointURL,
				EgressPolicyRef:     config.EgressPolicyRef,
				Method:              config.Method,
				AuthenticationMode:  config.Authentication.Mode,
				FieldMappings:       mappings,
				HeaderAllowlist:     append([]string(nil), config.HeaderAllowlist...),
				IdempotencyHeader:   config.IdempotencyHeader,
				TimeoutMilliseconds: config.TimeoutMilliseconds,
				SuccessStatusCodes:  append([]int(nil), config.SuccessStatusCodes...),
			}
		}
		return record
	}
	if domain.IsWebhookProfileChannelType(item.ChannelType) {
		// A group profile's complete webhook URL/key and optional signing secret
		// are stored only in the encrypted secret envelope. Read APIs expose
		// presence plus the small non-secret request settings, never the URL.
		record.ConfigJSON = ""
		record.MetadataJSON = ""
		record.RateLimitJSON = ""
		if config, err := domain.ParseWebhookProfileConfig(item.ConfigJSON); err == nil {
			record.WebhookProfileConfig = &facade.WebhookProfileConfig{
				TimeoutMilliseconds: config.TimeoutMilliseconds,
				SuccessStatusCodes:  append([]int(nil), config.SuccessStatusCodes...),
			}
		}
		return record
	}
	if !domain.IsEnterpriseApplicationChannelType(item.ChannelType) {
		return record
	}
	// Enterprise application configuration is deliberately re-expressed through
	// the small structured contract below. Management clients never receive the
	// persistence JSON, even though it remains the storage representation.
	record.ConfigJSON = ""
	record.MetadataJSON = ""
	if config, err := domain.ParseEnterpriseApplicationConfig(item.ChannelType, item.ConfigJSON); err == nil {
		record.ProviderConfig = &facade.ProviderChannelConfig{
			FeishuAppID:  config.AppID,
			WeComCorpID:  config.CorpID,
			WeComAgentID: config.AgentID,
		}
	}
	for _, descriptor := range domain.ProviderParameterCatalog(item.ChannelType) {
		record.ProviderParameterCatalog = append(record.ProviderParameterCatalog, facade.ProviderParameterDescriptor{
			Key:           descriptor.Key,
			Label:         descriptor.Label,
			ValueType:     descriptor.ValueType,
			MaxItems:      descriptor.MaxItems,
			MaxValueBytes: descriptor.MaxValueBytes,
			AllowDefault:  descriptor.AllowDefault,
		})
	}
	if settings, err := domain.ParseProviderParameterSettings(item.MetadataJSON); err == nil {
		for _, setting := range settings {
			record.ProviderParameterSettings = append(record.ProviderParameterSettings, facade.ProviderParameterSetting{
				Key:          setting.Key,
				Enabled:      setting.Enabled,
				DefaultValue: setting.DefaultValue,
			})
		}
	}
	return record
}

func mapDelivery(item domain.Delivery) *facade.DeliveryRecord {
	return &facade.DeliveryRecord{
		ID:                item.ID,
		DeliveryID:        item.DeliveryID,
		SceneCode:         item.SceneCode,
		ChannelCode:       item.ChannelCode,
		ChannelType:       item.ChannelType,
		TemplateCode:      item.TemplateCode,
		TargetMasked:      item.TargetMasked,
		Status:            item.Status,
		RetryCount:        item.RetryCount,
		MaxRetry:          item.MaxRetry,
		NextRetryAt:       item.NextRetryAt,
		LastError:         item.LastError,
		TraceID:           item.TraceID,
		SentAt:            item.SentAt,
		CreateTime:        item.CreateTime,
		UpdateTime:        item.UpdateTime,
		RenderedSubject:   item.RenderedSubject,
		ProviderReference: item.ProviderReference,
	}
}

// mapDeliverySummary creates the deliberately content-free management view.
// Raw provider diagnostics remain inside the application boundary only long
// enough to produce a stable, safe error class and never enter the response.
func mapDeliverySummary(item domain.DeliverySummary) *facade.DeliverySummaryRecord {
	failureCode, failureMessage := deliveryFailureSummary(item.Status, item.LastError)
	return &facade.DeliverySummaryRecord{
		ID:             item.ID,
		DeliveryID:     item.DeliveryID,
		SceneCode:      item.SceneCode,
		ChannelCode:    item.ChannelCode,
		ChannelType:    item.ChannelType,
		TemplateCode:   item.TemplateCode,
		TargetMasked:   item.TargetMasked,
		Status:         item.Status,
		RetryCount:     item.RetryCount,
		MaxRetry:       item.MaxRetry,
		NextRetryAt:    item.NextRetryAt,
		FailureCode:    failureCode,
		FailureMessage: failureMessage,
		TraceID:        item.TraceID,
		SentAt:         item.SentAt,
		CreateTime:     item.CreateTime,
		UpdateTime:     item.UpdateTime,
	}
}

func deliveryFailureSummary(status, raw string) (string, string) {
	if strings.TrimSpace(raw) == "" {
		return "", ""
	}
	value := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.Contains(value, "timeout"), strings.Contains(value, "deadline"):
		return "TIMEOUT", "投递超时"
	case strings.Contains(value, "credential"), strings.Contains(value, "token"), strings.Contains(value, "auth"), strings.Contains(value, "access_token"):
		return "CREDENTIAL_REJECTED", "连接凭据不可用"
	case strings.Contains(value, "receive_id"), strings.Contains(value, "recipient"), strings.Contains(value, "target"), strings.Contains(value, "open_id"), strings.Contains(value, "userid"):
		return "RECIPIENT_UNAVAILABLE", "接收对象不可用"
	case strings.EqualFold(strings.TrimSpace(status), domain.DeliveryStatusUnknown):
		return "OUTCOME_UNKNOWN", "投递结果暂时无法确认"
	default:
		return "DELIVERY_FAILED", "投递未完成"
	}
}
