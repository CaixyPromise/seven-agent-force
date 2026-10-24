package application

import (
	"context"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
)

// TestActionableMessageFakeUpstreamsEndToEnd proves one accepted logical
// notification fans out through the platform-owned scene, recipient, action,
// public URL and driver-capability contracts. Provider-specific payload bytes
// are covered by the infrastructure request fixtures; this test protects the
// cross-layer routing and immutable delivery snapshots they consume.
func TestActionableMessageFakeUpstreamsEndToEnd(t *testing.T) {
	repo := newSceneRevisionTestRepository()
	webhookConfig, err := domain.EncodeWebhookProfileConfig(domain.WebhookProfileConfig{TimeoutMilliseconds: 5000, SuccessStatusCodes: []int{200}})
	if err != nil {
		t.Fatalf("encode webhook config: %v", err)
	}
	channels := []*domain.Channel{
		enterpriseTestChannel(t, domain.ChannelTypeEmail, "smtp-primary", `{"host":"127.0.0.1","port":1025,"from":"notice@example.com"}`, "{}"),
		enterpriseTestChannel(t, domain.ChannelTypeFeishuApp, "feishu-app", `{"appId":"cli_fake"}`, "{}"),
		enterpriseTestChannel(t, domain.ChannelTypeWeComApp, "wecom-app", `{"corpId":"ww_fake","agentId":"100001"}`, "{}"),
		enterpriseTestChannel(t, domain.ChannelTypeFeishuWebhook, "feishu-group", webhookConfig, "{}"),
		enterpriseTestChannel(t, domain.ChannelTypeWeComWebhook, "wecom-group", webhookConfig, "{}"),
	}
	for _, channel := range channels {
		repo.channels[channel.ChannelCode] = channel
	}

	drivers := actionableMessageTestRegistry{drivers: map[string]*externalResultDriver{}}
	for _, channel := range channels {
		drivers.drivers[channel.ChannelType] = &externalResultDriver{result: DriverResult{Status: DriverResultProviderAccepted}}
	}
	service := newSceneRevisionTestService(t, repo)
	service.drivers = drivers
	service.urls = allowAllChannelURLs{}
	service.BindVerifiedEmailResolver(readinessEmailResolver{ready: true})
	publicURL, err := domain.ParsePublicWebBaseURL("https://workforce.example.com/console", true)
	if err != nil {
		t.Fatalf("parse public URL: %v", err)
	}
	service.BindPublicWebBaseURL(publicURL)

	template := createPublishedSceneTemplate(t, service, "workforce_actionable", "任务需要处理", "{{.name}} 需要你确认后继续。")
	const sceneCode = "actionable_message_e2e"
	createPublishedScene(t, service, sceneCode, "站内信", domain.SceneReceiverKindInApp, template.ID, "", true)
	createPublishedScene(t, service, sceneCode, "用户邮箱", domain.SceneReceiverKindEmailUser, template.ID, "smtp-primary", true)
	createPublishedScene(t, service, sceneCode, "飞书成员", domain.SceneReceiverKindFeishuOpenID, template.ID, "feishu-app", true)
	createPublishedScene(t, service, sceneCode, "企业微信成员", domain.SceneReceiverKindWeComUserID, template.ID, "wecom-app", true)
	createPublishedScene(t, service, sceneCode, "飞书研发群", domain.SceneReceiverKindFeishuWebhookGroup, template.ID, "feishu-group", true)
	createPublishedScene(t, service, sceneCode, "企业微信研发群", domain.SceneReceiverKindWeComWebhookGroup, template.ID, "wecom-group", true)

	request := facade.PublishRequest{
		EventKey: "task.action.required", IdempotencyKey: "actionable-e2e-1", SceneCode: sceneCode,
		TemplateVariables: map[string]any{"name": "补充用户查询接口"},
		Audience:          facade.Audience{UserIDs: []int64{42}},
		ReceiverKinds: []facade.ReceiverKind{
			facade.ReceiverKindInApp,
			facade.ReceiverKindEmailUser,
			facade.ReceiverKindFeishuWebhookGroup,
			facade.ReceiverKindWeComWebhookGroup,
		},
		ExternalRecipients: []facade.ExternalRecipient{
			{IdentityKind: facade.ExternalIdentityFeishuOpenID, Subject: "ou_fake_user"},
			{IdentityKind: facade.ExternalIdentityWeComUserID, Subject: "zhangsan"},
		},
		DeepLink: "/workforce/tasks/2065424359060983808?turn=2065424359060983812&focus=evaluation",
	}
	receipt, err := service.Publish(context.Background(), request)
	if err != nil {
		t.Fatalf("publish actionable notification: %v", err)
	}
	if receipt.NotificationID == "" || receipt.Duplicate || len(repo.notifications) != 1 || repo.inboxRecipients != 1 || len(repo.externalTargets) != 3 || len(repo.deliveries) != 5 {
		t.Fatalf("unexpected fan-out receipt=%#v notifications=%d inbox=%d targets=%d deliveries=%d", receipt, len(repo.notifications), repo.inboxRecipients, len(repo.externalTargets), len(repo.deliveries))
	}
	for _, delivery := range repo.deliveries {
		if delivery.ActionLabelSnapshot != "查看任务" || delivery.ActionURLSnapshot != "https://workforce.example.com/console/workforce/tasks/2065424359060983808?turn=2065424359060983812&focus=evaluation" || delivery.RenderedPayloadDigest == "" {
			t.Fatalf("delivery did not freeze platform action: %#v", delivery)
		}
		t.Logf("fake_driver channel=%s presentation=%s rendered_digest=%s", delivery.ChannelType, delivery.Presentation, delivery.RenderedPayloadDigest)
		if err := service.dispatch(context.Background(), delivery.DeliveryID); err != nil {
			t.Fatalf("dispatch %s: %v", delivery.ChannelType, err)
		}
		if repo.deliveries[delivery.DeliveryID].Status != domain.DeliveryStatusProviderAccepted {
			t.Fatalf("delivery %s did not reach provider accepted: %#v", delivery.DeliveryID, repo.deliveries[delivery.DeliveryID])
		}
	}
	for channelType, driver := range drivers.drivers {
		if len(driver.messages) != 1 {
			t.Fatalf("fake %s upstream messages=%d, want one", channelType, len(driver.messages))
		}
		message := driver.messages[0]
		if message.PrimaryAction == nil || message.PrimaryAction.Label != "查看任务" || message.ExternalActionURL == "" {
			t.Fatalf("fake %s upstream lost frozen action: %#v", channelType, message)
		}
	}

	duplicate, err := service.Publish(context.Background(), request)
	if err != nil || !duplicate.Duplicate || duplicate.NotificationID != receipt.NotificationID || len(repo.notifications) != 1 || len(repo.deliveries) != 5 {
		t.Fatalf("replay did not converge: receipt=%#v err=%v notifications=%d deliveries=%d", duplicate, err, len(repo.notifications), len(repo.deliveries))
	}
}

func TestActionableMessageWithoutPublicURLKeepsBodyAndOmitsExternalAction(t *testing.T) {
	repo := newSceneRevisionTestRepository()
	channel := enterpriseTestChannel(t, domain.ChannelTypeEmail, "smtp-primary", `{"host":"127.0.0.1","port":1025,"from":"notice@example.com"}`, "{}")
	repo.channels[channel.ChannelCode] = channel
	driver := &externalResultDriver{result: DriverResult{Status: DriverResultProviderAccepted}}
	service := newSceneRevisionTestService(t, repo)
	service.drivers = actionableMessageTestRegistry{drivers: map[string]*externalResultDriver{domain.ChannelTypeEmail: driver}}
	service.BindVerifiedEmailResolver(readinessEmailResolver{ready: true})
	template := createPublishedSceneTemplate(t, service, "actionable_no_url", "任务需要处理", "{{.name}} 需要你确认后继续。")
	createPublishedScene(t, service, "actionable_no_url", "用户邮箱", domain.SceneReceiverKindEmailUser, template.ID, channel.ChannelCode, true)

	_, err := service.Publish(context.Background(), facade.PublishRequest{
		EventKey: "task.action.required", IdempotencyKey: "actionable-no-url-1", SceneCode: "actionable_no_url",
		TemplateVariables: map[string]any{"name": "补充用户查询接口"}, Audience: facade.Audience{UserIDs: []int64{42}},
		ReceiverKinds: []facade.ReceiverKind{facade.ReceiverKindEmailUser}, DeepLink: "/workforce/tasks/42",
	})
	if err != nil || len(repo.deliveries) != 1 {
		t.Fatalf("publish without public URL err=%v deliveries=%d", err, len(repo.deliveries))
	}
	for _, delivery := range repo.deliveries {
		if delivery.ActionLabelSnapshot != "查看任务" || delivery.ActionURLSnapshot != "" || delivery.RenderedText == "" {
			t.Fatalf("missing public URL did not degrade safely: %#v", delivery)
		}
		if err := service.dispatch(context.Background(), delivery.DeliveryID); err != nil {
			t.Fatalf("dispatch without public URL: %v", err)
		}
	}
	if len(driver.messages) != 1 || driver.messages[0].ExternalActionURL != "" || driver.messages[0].Text == "" {
		t.Fatalf("fake SMTP did not receive body-only degradation: %#v", driver.messages)
	}
}

type actionableMessageTestRegistry struct {
	drivers map[string]*externalResultDriver
}

func (r actionableMessageTestRegistry) Driver(channelType string) domain.ChannelDriver {
	return r.drivers[channelType]
}

func (actionableMessageTestRegistry) Capabilities(channelType string) domain.DriverCapabilities {
	switch channelType {
	case domain.ChannelTypeEmail:
		return domain.DriverCapabilities{SupportedPresentations: []string{domain.DeliveryPresentationHTML, domain.DeliveryPresentationText}, SupportsPrimaryAction: true, SupportsExternalURL: true}
	case domain.ChannelTypeFeishuApp, domain.ChannelTypeFeishuWebhook, domain.ChannelTypeWeComApp, domain.ChannelTypeWeComWebhook:
		return domain.DriverCapabilities{SupportedPresentations: []string{domain.DeliveryPresentationCard, domain.DeliveryPresentationText}, SupportsPrimaryAction: true, SupportsExternalURL: true, SupportsProviderIdempotency: channelType == domain.ChannelTypeFeishuApp}
	default:
		return domain.DriverCapabilities{SupportedPresentations: []string{domain.DeliveryPresentationText}}
	}
}
