package application

import (
	"context"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

type receiverReadiness struct {
	ready    bool
	channel  *domain.Channel
	template *domain.TemplateRevision
	reason   string
}

// ResolveReadiness returns only safe capability state for the current user.
// It never returns an address, external member identity, endpoint or secret.
func (s *Service) ResolveReadiness(ctx context.Context, sceneCode string, userID int64) (*facade.Readiness, error) {
	return s.ResolveReadinessForOrganization(ctx, 0, sceneCode, userID)
}

func (s *Service) ResolveReadinessForOrganization(ctx context.Context, organizationID int64, sceneCode string, userID int64) (*facade.Readiness, error) {
	sceneCode = strings.TrimSpace(sceneCode)
	if userID <= 0 {
		return nil, apperrors.Unauthorized("未登录")
	}
	if err := domain.ValidateSceneDefinitionCode(sceneCode); err != nil {
		return nil, apperrors.Params("sceneCode无效")
	}
	result := &facade.Readiness{SceneCode: sceneCode}

	inApp, err := s.resolveReceiverReadiness(ctx, 0, sceneCode, domain.SceneReceiverKindInApp)
	if err != nil {
		return nil, err
	}
	result.InApp.Available = inApp.ready
	result.InApp.ReasonCode = inApp.reason
	if inApp.template != nil && strings.TrimSpace(inApp.template.PrimaryActionLabelTemplate) != "" {
		result.Action.Configured = true
	}

	email, err := s.resolveReceiverReadiness(ctx, 0, sceneCode, domain.SceneReceiverKindEmailUser)
	if err != nil {
		return nil, err
	}
	result.Email.Available = email.ready
	if email.ready && s.verifiedEmails != nil {
		_, result.Email.RecipientReady, err = s.verifiedEmails.ResolveVerifiedNotificationEmail(ctx, userID)
		if err != nil {
			return nil, err
		}
	}
	result.Email.ReasonCode = email.reason
	if email.ready && !result.Email.RecipientReady {
		result.Email.ReasonCode = domain.ReasonNotificationRecipientNotVerified
	}

	feishuDirect, err := s.resolveReceiverReadiness(ctx, organizationID, sceneCode, domain.SceneReceiverKindFeishuOpenID)
	if err != nil {
		return nil, err
	}
	feishuGroup, err := s.resolveReceiverReadiness(ctx, 0, sceneCode, domain.SceneReceiverKindFeishuWebhookGroup)
	if err != nil {
		return nil, err
	}
	result.Feishu.Available = feishuDirect.ready || feishuGroup.ready
	if s.enterpriseIdentities != nil && organizationID > 0 {
		available, direct, mention, reason, resolveErr := s.enterpriseIdentities.ResolveEnterpriseReadiness(ctx, organizationID, userID, "FEISHU")
		if resolveErr != nil {
			return nil, resolveErr
		}
		result.Feishu.Available = result.Feishu.Available || available
		result.Feishu.DirectReady = direct && feishuDirect.ready
		result.Feishu.MentionReady = mention
		if reason != "" {
			result.Feishu.ReasonCode = reason
		}
	}
	result.Feishu.GroupReady = feishuGroup.ready
	if feishuGroup.ready && feishuGroup.channel != nil {
		result.Feishu.BoundRouteName = strings.TrimSpace(feishuGroup.channel.ChannelName)
	}
	if !result.Feishu.Available {
		result.Feishu.ReasonCode = domain.ReasonEnterpriseConnectionNotConfigured
	}

	wecomDirect, err := s.resolveReceiverReadiness(ctx, organizationID, sceneCode, domain.SceneReceiverKindWeComUserID)
	if err != nil {
		return nil, err
	}
	wecomGroup, err := s.resolveReceiverReadiness(ctx, 0, sceneCode, domain.SceneReceiverKindWeComWebhookGroup)
	if err != nil {
		return nil, err
	}
	result.WeCom.Available = wecomDirect.ready || wecomGroup.ready
	if s.enterpriseIdentities != nil && organizationID > 0 {
		available, direct, mention, reason, resolveErr := s.enterpriseIdentities.ResolveEnterpriseReadiness(ctx, organizationID, userID, "WECOM")
		if resolveErr != nil {
			return nil, resolveErr
		}
		result.WeCom.Available = result.WeCom.Available || available
		result.WeCom.DirectReady = direct && wecomDirect.ready
		result.WeCom.MentionReady = mention
		if reason != "" {
			result.WeCom.ReasonCode = reason
		}
	}
	result.WeCom.GroupReady = wecomGroup.ready
	if wecomGroup.ready && wecomGroup.channel != nil {
		result.WeCom.BoundRouteName = strings.TrimSpace(wecomGroup.channel.ChannelName)
	}
	if !result.WeCom.Available {
		result.WeCom.ReasonCode = domain.ReasonEnterpriseConnectionNotConfigured
	}

	result.TaskLink.Available = s.publicWebBaseURL.Configured()
	if !result.TaskLink.Available {
		result.TaskLink.ReasonCode = domain.ReasonPublicWebBaseURLNotConfigured
	}
	return result, nil
}

func (s *Service) resolveReceiverReadiness(ctx context.Context, organizationID int64, sceneCode, receiverKind string) (receiverReadiness, error) {
	repo, err := s.sceneRevisionRepository()
	if err != nil {
		return receiverReadiness{}, err
	}
	definition, err := repo.FindSceneDefinitionByCodeAndReceiverKind(ctx, s.sceneRevisionScopeID(), enterpriseSceneCode(sceneCode, receiverKind, organizationID), receiverKind)
	if err != nil {
		return receiverReadiness{}, err
	}
	if definition == nil || definition.CurrentPublishedRevisionID == nil {
		return receiverReadiness{reason: domain.ReasonNotificationRouteNotConfigured}, nil
	}
	revision, err := repo.FindSceneRevisionByID(ctx, *definition.CurrentPublishedRevisionID)
	if err != nil {
		return receiverReadiness{}, err
	}
	if revision == nil || revision.State != domain.SceneRevisionStatePublished || !revision.Enabled {
		return receiverReadiness{reason: domain.ReasonNotificationRouteNotConfigured}, nil
	}
	_, template, err := s.loadSceneTemplateRevision(ctx, revision.TemplateRevisionID)
	if err != nil {
		return receiverReadiness{}, err
	}
	state := receiverReadiness{ready: true, template: template}
	if receiverKind == domain.SceneReceiverKindInApp {
		return state, nil
	}
	channel, err := s.repo.FindChannelByCode(ctx, revision.ConnectionRef)
	if err != nil {
		return receiverReadiness{}, err
	}
	if channel == nil || channel.Status != domain.ChannelStatusEnabled || !s.channelBelongsToCurrentScope(channel) || domain.SceneConnectionDigest(channel) != revision.ConnectionDigest {
		return receiverReadiness{reason: domain.ReasonNotificationRouteNotConfigured}, nil
	}
	state.channel = channel
	return state, nil
}
