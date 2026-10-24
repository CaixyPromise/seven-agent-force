package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
)

// EnsureEnterpriseDirectRoute materializes one organization-scoped, secretless
// Notification channel plus immutable scene revisions. The application secret
// remains owned exclusively by Enterprise Identity and is leased only at send.
func (s *Service) EnsureEnterpriseDirectRoute(ctx context.Context, organizationID, connectionID, connectionVersion int64, connectionDigest, provider string, enabled bool, actorID int64) error {
	if organizationID <= 0 || connectionID <= 0 || connectionVersion <= 0 || actorID <= 0 || strings.TrimSpace(connectionDigest) == "" {
		return fmt.Errorf("enterprise direct route identity is invalid")
	}
	provider = strings.ToUpper(strings.TrimSpace(provider))
	channelType, receiverKind := domain.ChannelTypeFeishuApp, domain.SceneReceiverKindFeishuOpenID
	if provider == "WECOM" {
		channelType, receiverKind = domain.ChannelTypeWeComApp, domain.SceneReceiverKindWeComUserID
	} else if provider != "FEISHU" {
		return fmt.Errorf("enterprise direct route provider is invalid")
	}
	channelCode := fmt.Sprintf("enterprise_%d_%s", organizationID, strings.ToLower(provider))
	organization := organizationID
	connection := connectionID
	status := domain.ChannelStatusDisabled
	if enabled {
		status = domain.ChannelStatusEnabled
	}
	channel := &domain.Channel{ID: s.nextID(), ChannelCode: channelCode, ChannelName: provider + " 个人消息", ChannelType: channelType, ScopeID: s.scopeID, ScopeType: "ORGANIZATION", OrganizationID: &organization, EnterpriseConnectionID: &connection, EnterpriseConnectionVersion: connectionVersion, EnterpriseConnectionDigest: connectionDigest, Status: status, Priority: 10, ConfigJSON: `{}`, RateLimitJSON: `{}`, MetadataJSON: `{"credentialOwner":"ENTERPRISE_IDENTITY"}`, CreatorID: int64Ptr(actorID), UpdaterID: int64Ptr(actorID)}
	if existing, err := s.repo.FindChannelByCode(ctx, channelCode); err != nil {
		return err
	} else if existing != nil {
		channel.ID = existing.ID
		channel.CreatorID = existing.CreatorID
	}
	if err := s.repo.UpsertChannel(ctx, channel); err != nil {
		return err
	}
	for _, spec := range builtinWorkforceScenes {
		if err := s.ensureEnterpriseDirectScene(ctx, spec, organizationID, receiverKind, channelCode, actorID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ensureEnterpriseDirectScene(ctx context.Context, spec builtinWorkforceScene, organizationID int64, receiverKind, channelCode string, actorID int64) error {
	templates, err := s.templateRevisionRepository()
	if err != nil {
		return err
	}
	templateDefinition, err := templates.FindTemplateDefinitionByCode(ctx, s.templateRevisionScopeID(), spec.code)
	if err != nil || templateDefinition == nil || templateDefinition.CurrentPublishedRevisionID == nil {
		return fmt.Errorf("enterprise direct scene template is unavailable")
	}
	templateRevisionID := *templateDefinition.CurrentPublishedRevisionID
	sceneCode := enterpriseSceneCode(spec.code, receiverKind, organizationID)
	scenes, err := s.sceneRevisionRepository()
	if err != nil {
		return err
	}
	definition, err := scenes.FindSceneDefinitionByCodeAndReceiverKind(ctx, s.sceneRevisionScopeID(), sceneCode, receiverKind)
	if err != nil {
		return err
	}
	draftInput := facade.SceneRevisionDraftInput{SceneName: spec.name + " · 个人消息", ReceiverKind: receiverKind, TemplateRevisionID: templateRevisionID, ConnectionRef: channelCode, Enabled: true}
	if definition == nil {
		created, err := s.CreateSceneDefinition(ctx, facade.SceneDefinitionCreateRequest{SceneCode: sceneCode, Draft: draftInput}, actorID)
		if err != nil {
			return err
		}
		_, err = s.PublishSceneRevision(ctx, created.CurrentDraft.ID, facade.SceneRevisionPublishRequest{ExpectedVersion: created.CurrentDraft.RevisionVersion}, actorID)
		return err
	}
	if definition.CurrentPublishedRevisionID != nil {
		published, findErr := scenes.FindSceneRevisionByID(ctx, *definition.CurrentPublishedRevisionID)
		channel, channelErr := s.repo.FindChannelByCode(ctx, channelCode)
		if findErr != nil || channelErr != nil {
			if findErr != nil {
				return findErr
			}
			return channelErr
		}
		if published != nil && channel != nil && published.ConnectionRef == channelCode && published.ConnectionDigest == domain.SceneConnectionDigest(channel) && published.Enabled {
			return nil
		}
	}
	next, err := s.CreateSceneDraftFromPublished(ctx, sceneCode, receiverKind, actorID)
	if err != nil {
		return err
	}
	saved, err := s.SaveSceneRevisionDraft(ctx, next.CurrentDraft.ID, facade.SceneRevisionSaveRequest{ExpectedVersion: next.CurrentDraft.RevisionVersion, Draft: draftInput}, actorID)
	if err != nil {
		return err
	}
	_, err = s.PublishSceneRevision(ctx, saved.CurrentDraft.ID, facade.SceneRevisionPublishRequest{ExpectedVersion: saved.CurrentDraft.RevisionVersion}, actorID)
	return err
}
