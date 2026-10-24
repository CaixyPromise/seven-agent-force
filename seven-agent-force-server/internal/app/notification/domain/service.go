package domain

import (
	"strings"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

type Service struct{}

func NewService() *Service { return &Service{} }

func (s *Service) ValidateChannel(item *Channel) error {
	if item == nil {
		return apperrors.Params("通知渠道不能为空")
	}
	if strings.TrimSpace(item.ChannelCode) == "" {
		return apperrors.Params("渠道编码不能为空")
	}
	if strings.TrimSpace(item.ChannelName) == "" {
		return apperrors.Params("渠道名称不能为空")
	}
	if !ValidChannelType(item.ChannelType) {
		return apperrors.Params("渠道类型不支持")
	}
	if IsDeferredChannelType(item.ChannelType) && item.Status == ChannelStatusEnabled {
		return apperrors.Params("该渠道将在后续版本开放，暂不能启用")
	}
	if item.Status != ChannelStatusEnabled && item.Status != ChannelStatusDisabled {
		return apperrors.Params("渠道状态不支持")
	}
	return nil
}

func (s *Service) ValidateTemplate(item *Template) error {
	if item == nil {
		return apperrors.Params("通知模板不能为空")
	}
	if strings.TrimSpace(item.TemplateCode) == "" || strings.TrimSpace(item.TemplateName) == "" {
		return apperrors.Params("模板编码和名称不能为空")
	}
	if strings.TrimSpace(item.SceneCode) == "" {
		return apperrors.Params("场景编码不能为空")
	}
	if !ValidChannelType(item.ChannelType) {
		return apperrors.Params("渠道类型不支持")
	}
	return nil
}

func (s *Service) ValidateSceneBinding(item *SceneBinding) error {
	if item == nil {
		return apperrors.Params("通知场景不能为空")
	}
	if strings.TrimSpace(item.SceneCode) == "" || strings.TrimSpace(item.ChannelCode) == "" || strings.TrimSpace(item.TemplateCode) == "" {
		return apperrors.Params("场景、渠道和模板不能为空")
	}
	if item.MaxRetry < 0 {
		return apperrors.Params("最大重试次数不能小于0")
	}
	if item.RetryIntervalSeconds <= 0 {
		item.RetryIntervalSeconds = 60
	}
	return nil
}

func ValidChannelType(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case ChannelTypeMock, ChannelTypeEmail, ChannelTypeFeishu, ChannelTypeWeCom, ChannelTypeDingTalk, ChannelTypeWebhook,
		ChannelTypeFeishuApp, ChannelTypeWeComApp, ChannelTypeHTTPConnector, ChannelTypeFeishuWebhook, ChannelTypeWeComWebhook:
		return true
	default:
		return false
	}
}
