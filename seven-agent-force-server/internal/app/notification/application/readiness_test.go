package application

import (
	"context"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
)

type readinessEmailResolver struct{ ready bool }

func (r readinessEmailResolver) ResolveVerifiedNotificationEmail(context.Context, int64) (string, bool, error) {
	if !r.ready {
		return "", false, nil
	}
	return "verified@example.com", true, nil
}

func TestResolveReadinessReturnsOnlySafeUserFacingCapabilityState(t *testing.T) {
	repo := newSceneRevisionTestRepository()
	repo.templateDefinitions[1] = &domain.TemplateDefinition{ID: 1, ScopeID: "local", TemplateCode: "workforce_action", CurrentPublishedRevisionID: int64Ptr(2)}
	repo.templateRevisions[2] = &domain.TemplateRevision{ID: 2, TemplateDefinitionID: 1, State: domain.TemplateRevisionStatePublished, VariableSchemaJSON: "[]", PrimaryActionLabelTemplate: "查看任务"}
	repo.sceneDefinitions[10] = &domain.SceneDefinition{ID: 10, ScopeID: "local", SceneCode: "workforce_task_action_required", ReceiverKind: domain.SceneReceiverKindInApp, CurrentPublishedRevisionID: int64Ptr(11)}
	repo.sceneRevisions[11] = &domain.SceneRevision{ID: 11, SceneDefinitionID: 10, State: domain.SceneRevisionStatePublished, Enabled: true, TemplateRevisionID: 2}
	emailChannel := &domain.Channel{ID: 20, ScopeID: "local", ChannelCode: "smtp-primary", ChannelName: "企业邮箱", ChannelType: domain.ChannelTypeEmail, Status: domain.ChannelStatusEnabled}
	repo.channels[emailChannel.ChannelCode] = emailChannel
	repo.sceneDefinitions[12] = &domain.SceneDefinition{ID: 12, ScopeID: "local", SceneCode: "workforce_task_action_required", ReceiverKind: domain.SceneReceiverKindEmailUser, CurrentPublishedRevisionID: int64Ptr(13)}
	repo.sceneRevisions[13] = &domain.SceneRevision{ID: 13, SceneDefinitionID: 12, State: domain.SceneRevisionStatePublished, Enabled: true, TemplateRevisionID: 2, ConnectionRef: emailChannel.ChannelCode, ConnectionDigest: domain.SceneConnectionDigest(emailChannel)}

	service := newSceneRevisionTestService(t, repo)
	service.BindVerifiedEmailResolver(readinessEmailResolver{ready: true})
	publicURL, err := domain.ParsePublicWebBaseURL("https://workforce.example.com/console", true)
	if err != nil {
		t.Fatal(err)
	}
	service.BindPublicWebBaseURL(publicURL)
	result, err := service.ResolveReadiness(context.Background(), "workforce_task_action_required", 42)
	if err != nil {
		t.Fatalf("ResolveReadiness() error=%v", err)
	}
	if !result.InApp.Available || !result.Email.Available || !result.Email.RecipientReady || !result.TaskLink.Available || !result.Action.Configured {
		t.Fatalf("unexpected readiness=%+v", result)
	}
	if result.Feishu.Available || result.WeCom.Available || result.Feishu.BoundRouteName != "" || result.WeCom.BoundRouteName != "" {
		t.Fatalf("unconfigured enterprise routes leaked readiness=%+v", result)
	}
}

func TestResolveReadinessKeepsUnverifiedEmailAndMissingPublicURLUnavailable(t *testing.T) {
	repo := newSceneRevisionTestRepository()
	repo.templateDefinitions[1] = &domain.TemplateDefinition{ID: 1, ScopeID: "local", TemplateCode: "workforce_action", CurrentPublishedRevisionID: int64Ptr(2)}
	repo.templateRevisions[2] = &domain.TemplateRevision{ID: 2, TemplateDefinitionID: 1, State: domain.TemplateRevisionStatePublished, VariableSchemaJSON: "[]", PrimaryActionLabelTemplate: "查看任务"}
	emailChannel := &domain.Channel{ID: 20, ScopeID: "local", ChannelCode: "smtp-primary", ChannelName: "企业邮箱", ChannelType: domain.ChannelTypeEmail, Status: domain.ChannelStatusEnabled}
	repo.channels[emailChannel.ChannelCode] = emailChannel
	repo.sceneDefinitions[12] = &domain.SceneDefinition{ID: 12, ScopeID: "local", SceneCode: "workforce_task_action_required", ReceiverKind: domain.SceneReceiverKindEmailUser, CurrentPublishedRevisionID: int64Ptr(13)}
	repo.sceneRevisions[13] = &domain.SceneRevision{ID: 13, SceneDefinitionID: 12, State: domain.SceneRevisionStatePublished, Enabled: true, TemplateRevisionID: 2, ConnectionRef: emailChannel.ChannelCode, ConnectionDigest: domain.SceneConnectionDigest(emailChannel)}

	service := newSceneRevisionTestService(t, repo)
	service.BindVerifiedEmailResolver(readinessEmailResolver{})
	result, err := service.ResolveReadiness(context.Background(), "workforce_task_action_required", 42)
	if err != nil {
		t.Fatalf("ResolveReadiness() error=%v", err)
	}
	if !result.Email.Available || result.Email.RecipientReady || result.Email.ReasonCode != domain.ReasonNotificationRecipientNotVerified {
		t.Fatalf("unverified email readiness=%+v", result.Email)
	}
	if result.TaskLink.Available || result.TaskLink.ReasonCode != domain.ReasonPublicWebBaseURLNotConfigured {
		t.Fatalf("missing public URL readiness=%+v", result.TaskLink)
	}
}
