package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
)

func TestSceneRevisionLifecycleIsImmutableScopedAndSingleSendingWay(t *testing.T) {
	repo := newSceneRevisionTestRepository()
	service := newSceneRevisionTestService(t, repo)
	template := createPublishedSceneTemplate(t, service, "invoice_notice", "账单提醒", "账单 {{.name}} 已生成")

	created, err := service.CreateSceneDefinition(context.Background(), facade.SceneDefinitionCreateRequest{
		SceneCode: "invoice_ready",
		Draft: facade.SceneRevisionDraftInput{
			SceneName:          "账单已生成",
			ReceiverKind:       domain.SceneReceiverKindInApp,
			TemplateRevisionID: template.ID,
			Enabled:            true,
		},
	}, 11)
	if err != nil {
		t.Fatalf("CreateSceneDefinition() error = %v", err)
	}
	if created.CurrentDraft == nil || created.CurrentPublished != nil {
		t.Fatalf("created scene = %#v", created)
	}

	published, err := service.PublishSceneRevision(context.Background(), created.CurrentDraft.ID, facade.SceneRevisionPublishRequest{ExpectedVersion: created.CurrentDraft.RevisionVersion}, 12)
	if err != nil {
		t.Fatalf("PublishSceneRevision() error = %v", err)
	}
	if published.CurrentDraft != nil || published.CurrentPublished == nil || published.CurrentPublished.State != domain.SceneRevisionStatePublished {
		t.Fatalf("published scene = %#v", published)
	}
	if _, err := service.SaveSceneRevisionDraft(context.Background(), published.CurrentPublished.ID, facade.SceneRevisionSaveRequest{ExpectedVersion: published.CurrentPublished.RevisionVersion, Draft: facade.SceneRevisionDraftInput{SceneName: "不应修改", ReceiverKind: domain.SceneReceiverKindInApp, TemplateRevisionID: template.ID, Enabled: true}}, 13); !errors.Is(err, domain.ErrSceneRevisionImmutable) {
		t.Fatalf("SaveSceneRevisionDraft(published) error = %v, want immutable", err)
	}

	next, err := service.CreateSceneDraftFromPublished(context.Background(), "invoice_ready", domain.SceneReceiverKindInApp, 14)
	if err != nil {
		t.Fatalf("CreateSceneDraftFromPublished() error = %v", err)
	}
	if next.CurrentDraft == nil || next.CurrentDraft.RevisionNo != 2 || next.CurrentPublished == nil || next.CurrentPublished.RevisionNo != 1 {
		t.Fatalf("scene clone changed published history: %#v", next)
	}
	if _, err := service.PublishSceneRevision(context.Background(), next.CurrentDraft.ID, facade.SceneRevisionPublishRequest{ExpectedVersion: next.CurrentDraft.RevisionVersion + 1}, 15); !errors.Is(err, domain.ErrSceneRevisionConflict) {
		t.Fatalf("stale publish error = %v, want conflict", err)
	}
	nextPublished, err := service.PublishSceneRevision(context.Background(), next.CurrentDraft.ID, facade.SceneRevisionPublishRequest{ExpectedVersion: next.CurrentDraft.RevisionVersion}, 15)
	if err != nil {
		t.Fatalf("PublishSceneRevision(next) error = %v", err)
	}
	if len(nextPublished.Revisions) != 2 || nextPublished.Revisions[0].RevisionNo != 2 || nextPublished.Revisions[0].State != domain.SceneRevisionStatePublished || nextPublished.Revisions[1].State != domain.SceneRevisionStateSuperseded {
		t.Fatalf("scene history = %#v", nextPublished.Revisions)
	}

	stopped, err := service.StopSceneDefinition(context.Background(), "invoice_ready", domain.SceneReceiverKindInApp, 16)
	if err != nil {
		t.Fatalf("StopSceneDefinition() error = %v", err)
	}
	if stopped.CurrentPublished == nil || stopped.CurrentPublished.Enabled || stopped.CurrentPublished.RevisionNo != 3 {
		t.Fatalf("stopped scene = %#v", stopped)
	}

	otherScope := newSceneRevisionTestService(t, repo)
	otherScope.SetScopeID("other-scope")
	if _, err := otherScope.GetSceneDefinition(context.Background(), "invoice_ready", domain.SceneReceiverKindInApp); err == nil {
		t.Fatal("foreign scope read unexpectedly succeeded")
	}
	if _, err := service.CreateSceneDefinition(context.Background(), facade.SceneDefinitionCreateRequest{
		SceneCode: "invalid_inbox_way",
		Draft:     facade.SceneRevisionDraftInput{SceneName: "错误发送方式", ReceiverKind: domain.SceneReceiverKindInApp, TemplateRevisionID: template.ID, ConnectionRef: "must-not-exist", Enabled: true},
	}, 17); err == nil {
		t.Fatal("in-app scene accepted a connection")
	}
}

func TestListSceneDefinitionsLoadsCurrentRevisionsInOneBoundedBatch(t *testing.T) {
	repo := newSceneRevisionTestRepository()
	for definitionID := int64(1); definitionID <= 3; definitionID++ {
		draftID := definitionID * 10
		publishedID := draftID + 1
		repo.sceneDefinitions[definitionID] = &domain.SceneDefinition{
			ID:                         definitionID,
			ScopeID:                    "local",
			SceneCode:                  fmt.Sprintf("scene_%d", definitionID),
			SceneName:                  fmt.Sprintf("Scene %d", definitionID),
			ReceiverKind:               domain.SceneReceiverKindInApp,
			CurrentDraftRevisionID:     int64Ptr(draftID),
			CurrentPublishedRevisionID: int64Ptr(publishedID),
		}
		repo.sceneRevisions[draftID] = &domain.SceneRevision{ID: draftID, SceneDefinitionID: definitionID, State: domain.SceneRevisionStateDraft}
		repo.sceneRevisions[publishedID] = &domain.SceneRevision{ID: publishedID, SceneDefinitionID: definitionID, State: domain.SceneRevisionStatePublished}
	}
	service := newSceneRevisionTestService(t, repo)

	page, err := service.ListSceneDefinitions(context.Background(), domain.SceneDefinitionQuery{Current: 1, PageSize: 200})
	if err != nil {
		t.Fatalf("ListSceneDefinitions() error=%v", err)
	}
	if len(page.Records) != 3 {
		t.Fatalf("records=%d, want 3", len(page.Records))
	}
	if repo.findSceneRevisionByIDCalls != 0 || repo.listSceneRevisionsByIDsCalls != 1 {
		t.Fatalf("revision queries: FindByID=%d ListByIDs=%d, want 0 and 1", repo.findSceneRevisionByIDCalls, repo.listSceneRevisionsByIDsCalls)
	}
	if got := len(repo.lastSceneRevisionBatchIDs); got != 6 {
		t.Fatalf("batched revision IDs=%d, want 6", got)
	}
}

func TestSceneRevisionRejectsForeignTemplateAndMismatchedConnection(t *testing.T) {
	repo := newSceneRevisionTestRepository()
	local := newSceneRevisionTestService(t, repo)
	foreign := newSceneRevisionTestService(t, repo)
	foreignIDGen, err := xid.New(92)
	if err != nil {
		t.Fatalf("xid.New(foreign) error = %v", err)
	}
	foreign.idGen = foreignIDGen
	foreign.SetScopeID("foreign-scope")
	foreignTemplate := createPublishedSceneTemplate(t, foreign, "foreign_template", "外部范围模板", "外部 {{.name}}")

	if _, err := local.CreateSceneDefinition(context.Background(), facade.SceneDefinitionCreateRequest{
		SceneCode: "foreign_template_scene",
		Draft: facade.SceneRevisionDraftInput{
			SceneName:          "错误模板范围",
			ReceiverKind:       domain.SceneReceiverKindInApp,
			TemplateRevisionID: foreignTemplate.ID,
			Enabled:            true,
		},
	}, 18); !errors.Is(err, domain.ErrTemplateRevisionNotFound) {
		t.Fatalf("foreign template scene error=%v, want scoped rejection", err)
	}

	localTemplate := createPublishedSceneTemplate(t, local, "local_template", "本地模板", "本地 {{.name}}")
	repo.channels["wecom-only"] = enterpriseTestChannel(t, domain.ChannelTypeWeComApp, "wecom-only", `{"corpId":"ww_local","agentId":"100001"}`, "{}")
	if _, err := local.CreateSceneDefinition(context.Background(), facade.SceneDefinitionCreateRequest{
		SceneCode: "wrong_receiver_connection",
		Draft: facade.SceneRevisionDraftInput{
			SceneName:          "错误发送方式",
			ReceiverKind:       domain.SceneReceiverKindFeishuOpenID,
			TemplateRevisionID: localTemplate.ID,
			ConnectionRef:      "wecom-only",
			Enabled:            true,
		},
	}, 19); err == nil || !strings.Contains(err.Error(), "飞书场景") {
		t.Fatalf("mismatched receiver connection error=%v", err)
	}
	if len(repo.sceneDefinitions) != 0 {
		t.Fatalf("invalid scene inputs wrote definitions=%#v", repo.sceneDefinitions)
	}
}

func TestPublishSceneFreezesExternalDeliveryAndDoesNotCreateInbox(t *testing.T) {
	repo := newSceneRevisionTestRepository()
	repo.channels["feishu-app"] = enterpriseTestChannel(t, domain.ChannelTypeFeishuApp, "feishu-app", `{"appId":"cli_scene"}`, "{}")
	service := newSceneRevisionTestService(t, repo)
	template := createPublishedSceneTemplate(t, service, "external_invoice", "外部账单", "给 {{.name}} 的外部账单")
	created, err := service.CreateSceneDefinition(context.Background(), facade.SceneDefinitionCreateRequest{
		SceneCode: "invoice_ready",
		Draft:     facade.SceneRevisionDraftInput{SceneName: "飞书账单", ReceiverKind: domain.SceneReceiverKindFeishuOpenID, TemplateRevisionID: template.ID, ConnectionRef: "feishu-app", Enabled: true},
	}, 20)
	if err != nil {
		t.Fatalf("CreateSceneDefinition(external) error = %v", err)
	}
	published, err := service.PublishSceneRevision(context.Background(), created.CurrentDraft.ID, facade.SceneRevisionPublishRequest{ExpectedVersion: created.CurrentDraft.RevisionVersion}, 21)
	if err != nil {
		t.Fatalf("PublishSceneRevision(external) error = %v", err)
	}
	request := facade.PublishRequest{
		EventKey:          "billing.invoice.ready",
		IdempotencyKey:    "scene-external-1",
		SceneCode:         "invoice_ready",
		TemplateVariables: map[string]any{"name": "Ada"},
		ExternalRecipients: []facade.ExternalRecipient{{
			IdentityKind: facade.ExternalIdentityFeishuOpenID,
			Subject:      "ou_scene_ada",
		}},
	}
	first, err := service.Publish(context.Background(), request)
	if err != nil {
		t.Fatalf("Publish(scene external) error = %v", err)
	}
	if first.Duplicate || len(repo.notifications) != 1 || len(repo.externalTargets) != 1 || len(repo.deliveries) != 1 || repo.inboxRecipients != 0 {
		t.Fatalf("unexpected external scene writes: receipt=%#v notifications=%d targets=%d deliveries=%d inbox=%d", first, len(repo.notifications), len(repo.externalTargets), len(repo.deliveries), repo.inboxRecipients)
	}
	if len(repo.sceneSnapshots) != 1 {
		t.Fatalf("scene snapshots = %d, want one", len(repo.sceneSnapshots))
	}
	for _, snapshot := range repo.sceneSnapshots {
		if snapshot.SceneRevisionID != published.CurrentPublished.ID || snapshot.TemplateRevisionID != template.ID || snapshot.ConnectionRef != "feishu-app" || snapshot.Resolution != domain.SceneSnapshotResolutionAccepted {
			t.Fatalf("snapshot did not freeze published selection: %#v", snapshot)
		}
	}
	for _, delivery := range repo.deliveries {
		if delivery.SceneSnapshotID == nil || delivery.TemplateCode != "external_invoice" || delivery.RenderedText != "给 Ada 的外部账单" || delivery.ContentTier != domain.DeliveryContentTierPublic || delivery.ExternalTargetID == nil {
			t.Fatalf("external scene delivery = %#v", delivery)
		}
	}
	if got := countOutboxEvent(repo.outbox, domain.OutboxEventNotificationInboxChanged); got != 0 {
		t.Fatalf("external scene emitted inbox mutation events = %d", got)
	}
	callerOverride := request
	callerOverride.IdempotencyKey = "scene-external-override"
	callerOverride.ExternalRecipients = []facade.ExternalRecipient{{
		ConnectionRef: "different-connection",
		IdentityKind:  facade.ExternalIdentityFeishuOpenID,
		Subject:       "ou_scene_ada",
	}}
	if _, err := service.Publish(context.Background(), callerOverride); err == nil || !strings.Contains(err.Error(), "不一致") {
		t.Fatalf("published scene accepted caller connection override: %v", err)
	}

	// Later template/channel changes must not be re-read by the same
	// idempotency key. A retry returns the old receipt and writes nothing.
	draft, err := service.CreateTemplateDraftFromPublished(context.Background(), "external_invoice", 22)
	if err != nil {
		t.Fatalf("CreateTemplateDraftFromPublished() error = %v", err)
	}
	updated, err := service.SaveTemplateRevisionDraft(context.Background(), draft.CurrentDraft.ID, facade.TemplateRevisionSaveRequest{ExpectedVersion: draft.CurrentDraft.RevisionVersion, Draft: sceneTemplateInput("外部账单", "给 {{.name}} 的新版本")}, 22)
	if err != nil {
		t.Fatalf("SaveTemplateRevisionDraft() error = %v", err)
	}
	if _, err := service.PublishTemplateRevision(context.Background(), updated.CurrentDraft.ID, facade.TemplateRevisionPublishRequest{ExpectedVersion: updated.CurrentDraft.RevisionVersion}, 22); err != nil {
		t.Fatalf("PublishTemplateRevision() error = %v", err)
	}
	repo.channels["feishu-app"].Status = domain.ChannelStatusDisabled
	second, err := service.Publish(context.Background(), request)
	if err != nil {
		t.Fatalf("Publish(scene retry after changes) error = %v", err)
	}
	if !second.Duplicate || second.NotificationID != first.NotificationID || len(repo.sceneSnapshots) != 1 || len(repo.deliveries) != 1 || len(repo.externalTargets) != 1 {
		t.Fatalf("retry re-resolved accepted scene: first=%#v second=%#v snapshots=%d deliveries=%d targets=%d", first, second, len(repo.sceneSnapshots), len(repo.deliveries), len(repo.externalTargets))
	}
}

func TestPublishSceneFreezesSensitiveTemplateContentTier(t *testing.T) {
	repo := newSceneRevisionTestRepository()
	repo.channels["feishu-sensitive"] = enterpriseTestChannel(t, domain.ChannelTypeFeishuApp, "feishu-sensitive", `{"appId":"cli_sensitive"}`, "{}")
	service := newSceneRevisionTestService(t, repo)
	draft := sceneTemplateInput("敏感账单", "账单 {{.name}} 已生成")
	draft.Variables[0].Classification = domain.TemplateVariableClassificationSensitive
	createdTemplate, err := service.CreateTemplateDefinition(context.Background(), facade.TemplateDefinitionCreateRequest{
		TemplateCode: "sensitive_invoice",
		Draft:        draft,
	}, 23)
	if err != nil {
		t.Fatalf("CreateTemplateDefinition(sensitive) error=%v", err)
	}
	publishedTemplate, err := service.PublishTemplateRevision(context.Background(), createdTemplate.CurrentDraft.ID, facade.TemplateRevisionPublishRequest{
		ExpectedVersion: createdTemplate.CurrentDraft.RevisionVersion,
	}, 23)
	if err != nil || publishedTemplate.CurrentPublished == nil {
		t.Fatalf("PublishTemplateRevision(sensitive) template=%#v err=%v", publishedTemplate, err)
	}
	createPublishedScene(t, service, "sensitive_invoice_ready", "敏感飞书账单", domain.SceneReceiverKindFeishuOpenID, publishedTemplate.CurrentPublished.ID, "feishu-sensitive", true)

	if _, err := service.Publish(context.Background(), facade.PublishRequest{
		EventKey:          "billing.sensitive.ready",
		IdempotencyKey:    "scene-sensitive-1",
		SceneCode:         "sensitive_invoice_ready",
		TemplateVariables: map[string]any{"name": "Ada"},
		ExternalRecipients: []facade.ExternalRecipient{{
			IdentityKind: facade.ExternalIdentityFeishuOpenID,
			Subject:      "ou_sensitive_ada",
		}},
	}); err != nil {
		t.Fatalf("Publish(sensitive scene) error=%v", err)
	}
	if len(repo.deliveries) != 1 || repo.inboxRecipients != 0 {
		t.Fatalf("sensitive external delivery writes deliveries=%#v inbox=%d", repo.deliveries, repo.inboxRecipients)
	}
	for _, delivery := range repo.deliveries {
		if delivery.ContentTier != domain.DeliveryContentTierSensitive || delivery.RenderedText != "账单 Ada 已生成" {
			t.Fatalf("sensitive template tier was not frozen on delivery: %#v", delivery)
		}
	}
}

func TestPublishSceneUsesConfiguredStaticConnectionAndKeepsItOutOfInbox(t *testing.T) {
	repo := newSceneRevisionTestRepository()
	channel := staticHTTPConnectorTestChannel(t, "scene-http", "first-scene-secret")
	repo.channels[channel.ChannelCode] = channel
	driver := &externalResultDriver{result: DriverResult{Status: DriverResultProviderAccepted, ProviderReference: "controlled-receiver"}}
	service := newSceneRevisionTestService(t, repo)
	service.drivers = driverRegistryFunc(func(string) ChannelDriver { return driver })
	service.urls = allowAllChannelURLs{}
	template := createPublishedSceneTemplate(t, service, "static_invoice", "固定连接账单", "固定 {{.name}} 已生成")
	created := createPublishedScene(t, service, "invoice_ready", "固定 HTTP", domain.SceneReceiverKindFixedConnection, template.ID, channel.ChannelCode, true)
	if created.CurrentPublished == nil {
		t.Fatalf("configured static scene was not published: %#v", created)
	}

	request := facade.PublishRequest{
		EventKey:                   "billing.static.ready",
		IdempotencyKey:             "scene-static-1",
		SceneCode:                  "invoice_ready",
		TemplateVariables:          map[string]any{"name": "Ada"},
		SendToConfiguredConnection: true,
	}
	receipt, err := service.Publish(context.Background(), request)
	if err != nil {
		t.Fatalf("Publish(configured static scene) error = %v", err)
	}
	if receipt.NotificationID == "" || len(repo.deliveries) != 1 || len(repo.httpSnapshots) != 1 || len(repo.externalTargets) != 0 || repo.inboxRecipients != 0 {
		t.Fatalf("configured static scene writes receipt=%#v deliveries=%d snapshots=%d targets=%d inbox=%d", receipt, len(repo.deliveries), len(repo.httpSnapshots), len(repo.externalTargets), repo.inboxRecipients)
	}
	deliveryID := onlyExternalDeliveryID(t, repo.externalTestRepository)
	delivery := repo.deliveries[deliveryID]
	if delivery.SceneSnapshotID == nil || delivery.ChannelCode != channel.ChannelCode || delivery.TemplateCode != "static_invoice" || delivery.RenderedText != "固定 Ada 已生成" || delivery.ContentTier != domain.DeliveryContentTierPublic {
		t.Fatalf("configured static scene delivery = %#v", delivery)
	}
	if len(repo.sceneSnapshots) != 1 {
		t.Fatalf("configured static scene snapshots=%#v", repo.sceneSnapshots)
	}
	retry := request
	retry.TraceID = "trace-retry"
	duplicate, err := service.Publish(context.Background(), retry)
	if err != nil || !duplicate.Duplicate || duplicate.NotificationID != receipt.NotificationID || len(repo.notifications) != 1 || len(repo.deliveries) != 1 {
		t.Fatalf("scene trace-only retry receipt=%#v err=%v notifications=%d deliveries=%d", duplicate, err, len(repo.notifications), len(repo.deliveries))
	}
	for _, snapshot := range repo.sceneSnapshots {
		if snapshot.ConnectionRef != channel.ChannelCode || snapshot.ReceiverKind != domain.SceneReceiverKindFixedConnection || snapshot.Resolution != domain.SceneSnapshotResolutionAccepted {
			t.Fatalf("configured static scene snapshot=%#v", snapshot)
		}
	}

	// A later emergency disable is an execution gate. It must prevent the
	// accepted static delivery from leaving through another connection, while
	// the accepted HTTP snapshot remains immutable for audit/retry evidence.
	repo.channels[channel.ChannelCode].Status = domain.ChannelStatusDisabled
	if err := service.dispatch(context.Background(), deliveryID); err == nil || !isDeliveryAsyncHandled(err) {
		t.Fatalf("dispatch through disabled configured static connection error=%v", err)
	}
	if len(driver.messages) != 0 || repo.deliveries[deliveryID].Status != domain.DeliveryStatusFailed || repo.inboxRecipients != 0 {
		t.Fatalf("disabled configured static connection sent or polluted inbox: messages=%d delivery=%#v inbox=%d", len(driver.messages), repo.deliveries[deliveryID], repo.inboxRecipients)
	}
}

func TestPublishSceneDisabledExternalDoesNotBlockInAppOrLeakIdentity(t *testing.T) {
	repo := newSceneRevisionTestRepository()
	repo.channels["feishu-app"] = enterpriseTestChannel(t, domain.ChannelTypeFeishuApp, "feishu-app", `{"appId":"cli_scene"}`, "{}")
	service := newSceneRevisionTestService(t, repo)
	template := createPublishedSceneTemplate(t, service, "mixed_notice", "混合提醒", "你好 {{.name}}")

	inbox := createPublishedScene(t, service, "mixed_notice", "站内信", domain.SceneReceiverKindInApp, template.ID, "", true)
	_ = inbox
	external := createPublishedScene(t, service, "mixed_notice", "飞书已停用", domain.SceneReceiverKindFeishuOpenID, template.ID, "feishu-app", false)
	if external.CurrentPublished == nil || external.CurrentPublished.Enabled {
		t.Fatalf("external disabled scene = %#v", external)
	}

	receipt, err := service.Publish(context.Background(), facade.PublishRequest{
		EventKey:          "billing.mixed.ready",
		IdempotencyKey:    "scene-mixed-1",
		SceneCode:         "mixed_notice",
		TemplateVariables: map[string]any{"name": "Ada"},
		Audience:          facade.Audience{UserIDs: []int64{42}},
		ExternalRecipients: []facade.ExternalRecipient{{
			IdentityKind: facade.ExternalIdentityFeishuOpenID,
			Subject:      "ou_mixed_ada",
		}},
	})
	if err != nil {
		t.Fatalf("Publish(mixed disabled external) error = %v", err)
	}
	if receipt.NotificationID == "" || repo.inboxRecipients != 1 || len(repo.externalTargets) != 0 || len(repo.deliveries) != 0 {
		t.Fatalf("disabled external polluted mixed publish: receipt=%#v inbox=%d targets=%d deliveries=%d", receipt, repo.inboxRecipients, len(repo.externalTargets), len(repo.deliveries))
	}
	if countOutboxEvent(repo.outbox, domain.OutboxEventNotificationInboxChanged) != 1 {
		t.Fatalf("in-app audience did not retain its own inbox event: %#v", repo.outbox)
	}
	if len(repo.sceneSnapshots) != 2 {
		t.Fatalf("mixed scene snapshots = %d, want in-app + disabled external", len(repo.sceneSnapshots))
	}
	seenDisabled := false
	for _, snapshot := range repo.sceneSnapshots {
		if snapshot.ReceiverKind == domain.SceneReceiverKindFeishuOpenID {
			seenDisabled = snapshot.Resolution == domain.SceneSnapshotResolutionDisabled
		}
	}
	if !seenDisabled {
		t.Fatalf("disabled scene snapshot missing: %#v", repo.sceneSnapshots)
	}

	if _, err := service.Publish(context.Background(), facade.PublishRequest{EventKey: "billing.disabled.only", IdempotencyKey: "scene-disabled-only", SceneCode: "mixed_notice", TemplateVariables: map[string]any{"name": "Ada"}, ExternalRecipients: []facade.ExternalRecipient{{IdentityKind: facade.ExternalIdentityFeishuOpenID, Subject: "ou_only_disabled"}}}); sceneRevisionReasonCode(err) != "SCENE_DISABLED" {
		t.Fatalf("disabled-only scene error = %v, want SCENE_DISABLED", err)
	}
}

func TestPublishSceneResolvesOnlyVerifiedUserEmailAndFreezesEncryptedTarget(t *testing.T) {
	repo := newSceneRevisionTestRepository()
	emailChannel := enterpriseTestChannel(t, domain.ChannelTypeEmail, "smtp-primary", `{"host":"127.0.0.1","port":1025,"from":"notice@example.com"}`, "{}")
	emailChannel.ChannelName = "企业邮箱"
	repo.channels[emailChannel.ChannelCode] = emailChannel
	service := newSceneRevisionTestService(t, repo)
	service.drivers = capabilityTestRegistry{driver: &externalResultDriver{result: DriverResult{Status: DriverResultProviderAccepted}}}
	service.BindVerifiedEmailResolver(readinessEmailResolver{ready: true})
	publicURL, err := domain.ParsePublicWebBaseURL("https://workforce.example.com/console", true)
	if err != nil {
		t.Fatal(err)
	}
	service.BindPublicWebBaseURL(publicURL)
	template := createPublishedSceneTemplate(t, service, "verified_email_notice", "任务提醒", "你好 {{.name}}")
	createPublishedScene(t, service, "verified_email_notice", "站内信", domain.SceneReceiverKindInApp, template.ID, "", true)
	createPublishedScene(t, service, "verified_email_notice", "用户邮箱", domain.SceneReceiverKindEmailUser, template.ID, emailChannel.ChannelCode, true)

	receipt, err := service.Publish(context.Background(), facade.PublishRequest{
		EventKey: "task.action.required", IdempotencyKey: "verified-email-1", SceneCode: "verified_email_notice",
		TemplateVariables: map[string]any{"name": "Ada"}, Audience: facade.Audience{UserIDs: []int64{42}}, ReceiverKinds: []facade.ReceiverKind{facade.ReceiverKindInApp, facade.ReceiverKindEmailUser}, DeepLink: "/workforce/tasks/42",
	})
	if err != nil || receipt.NotificationID == "" || repo.inboxRecipients != 1 || len(repo.externalTargets) != 1 || len(repo.deliveries) != 1 {
		t.Fatalf("verified email publish receipt=%#v err=%v inbox=%d targets=%d deliveries=%d", receipt, err, repo.inboxRecipients, len(repo.externalTargets), len(repo.deliveries))
	}
	for _, target := range repo.externalTargets {
		if target.IdentityKind != domain.ExternalIdentityEmailAddress || target.ProviderCode != domain.ChannelTypeEmail || target.SubjectCiphertext == "" || target.SubjectDigest == "" || strings.Contains(target.SubjectCiphertext, "verified@example.com") {
			t.Fatalf("email target was not safely frozen: %#v", target)
		}
	}
	for _, delivery := range repo.deliveries {
		if delivery.Presentation != domain.DeliveryPresentationHTML || delivery.ActionLabelSnapshot != "查看任务" || delivery.ActionURLSnapshot != "https://workforce.example.com/console/workforce/tasks/42" || delivery.RenderedPayloadDigest == "" {
			t.Fatalf("email action was not frozen: %#v", delivery)
		}
	}

	duplicate, err := service.Publish(context.Background(), facade.PublishRequest{
		EventKey: "task.action.required", IdempotencyKey: "verified-email-1", SceneCode: "verified_email_notice",
		TemplateVariables: map[string]any{"name": "Ada"}, Audience: facade.Audience{UserIDs: []int64{42}}, ReceiverKinds: []facade.ReceiverKind{facade.ReceiverKindInApp, facade.ReceiverKindEmailUser}, DeepLink: "/workforce/tasks/42",
	})
	if err != nil || !duplicate.Duplicate || len(repo.externalTargets) != 1 || len(repo.deliveries) != 1 {
		t.Fatalf("email replay receipt=%#v err=%v targets=%d deliveries=%d", duplicate, err, len(repo.externalTargets), len(repo.deliveries))
	}
}

func TestPublishSceneSuppressesUnverifiedEmailWithoutBlockingInApp(t *testing.T) {
	repo := newSceneRevisionTestRepository()
	emailChannel := enterpriseTestChannel(t, domain.ChannelTypeEmail, "smtp-primary", `{"host":"127.0.0.1","port":1025,"from":"notice@example.com"}`, "{}")
	repo.channels[emailChannel.ChannelCode] = emailChannel
	service := newSceneRevisionTestService(t, repo)
	service.BindVerifiedEmailResolver(readinessEmailResolver{})
	template := createPublishedSceneTemplate(t, service, "unverified_email_notice", "任务提醒", "你好 {{.name}}")
	createPublishedScene(t, service, "unverified_email_notice", "站内信", domain.SceneReceiverKindInApp, template.ID, "", true)
	createPublishedScene(t, service, "unverified_email_notice", "用户邮箱", domain.SceneReceiverKindEmailUser, template.ID, emailChannel.ChannelCode, true)

	receipt, err := service.Publish(context.Background(), facade.PublishRequest{
		EventKey: "task.action.required", IdempotencyKey: "unverified-email-1", SceneCode: "unverified_email_notice",
		TemplateVariables: map[string]any{"name": "Ada"}, Audience: facade.Audience{UserIDs: []int64{42}}, ReceiverKinds: []facade.ReceiverKind{facade.ReceiverKindInApp, facade.ReceiverKindEmailUser},
	})
	if err != nil || receipt.NotificationID == "" || repo.inboxRecipients != 1 || len(repo.externalTargets) != 0 || len(repo.deliveries) != 0 {
		t.Fatalf("unverified partial publish receipt=%#v err=%v inbox=%d targets=%d deliveries=%d", receipt, err, repo.inboxRecipients, len(repo.externalTargets), len(repo.deliveries))
	}
}

func sceneRevisionReasonCode(err error) string {
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr == nil {
		return ""
	}
	details, ok := appErr.Details().(map[string]string)
	if !ok {
		return ""
	}
	return details["reasonCode"]
}

func TestPublishSceneDraftOnlyNeverFallsBackToCallerContent(t *testing.T) {
	repo := newSceneRevisionTestRepository()
	service := newSceneRevisionTestService(t, repo)
	template := createPublishedSceneTemplate(t, service, "draft_notice", "草稿提醒", "草稿 {{.name}}")
	if _, err := service.CreateSceneDefinition(context.Background(), facade.SceneDefinitionCreateRequest{SceneCode: "draft_only", Draft: facade.SceneRevisionDraftInput{SceneName: "仅草稿", ReceiverKind: domain.SceneReceiverKindInApp, TemplateRevisionID: template.ID, Enabled: true}}, 30); err != nil {
		t.Fatalf("CreateSceneDefinition(draft) error = %v", err)
	}
	if _, err := service.Publish(context.Background(), facade.PublishRequest{EventKey: "draft.notice", IdempotencyKey: "draft-only-fail", SceneCode: "draft_only", Audience: facade.Audience{UserIDs: []int64{9}}, TemplateVariables: map[string]any{"name": "Ada"}}); err == nil || !strings.Contains(err.Error(), "场景尚未发布") {
		t.Fatalf("draft-only without legacy content error = %v", err)
	}
	if _, err := service.Publish(context.Background(), facade.PublishRequest{EventKey: "draft.notice", IdempotencyKey: "draft-only-content", SceneCode: "draft_only", Audience: facade.Audience{UserIDs: []int64{9}}, Title: "调用方标题", Content: "调用方正文"}); sceneRevisionReasonCode(err) != "SCENE_NOT_PUBLISHED" {
		t.Fatalf("draft-only caller content fallback error = %v", err)
	}
	if len(repo.notifications) != 0 || repo.inboxRecipients != 0 {
		t.Fatalf("draft-only caller content created state notifications=%d inbox=%d", len(repo.notifications), repo.inboxRecipients)
	}
}

func createPublishedSceneTemplate(t *testing.T, service *Service, code, name, text string) *facade.TemplateRevisionRecord {
	t.Helper()
	created, err := service.CreateTemplateDefinition(context.Background(), facade.TemplateDefinitionCreateRequest{TemplateCode: code, Draft: sceneTemplateInput(name, text)}, 1)
	if err != nil {
		t.Fatalf("CreateTemplateDefinition(%s) error = %v", code, err)
	}
	published, err := service.PublishTemplateRevision(context.Background(), created.CurrentDraft.ID, facade.TemplateRevisionPublishRequest{ExpectedVersion: created.CurrentDraft.RevisionVersion}, 1)
	if err != nil {
		t.Fatalf("PublishTemplateRevision(%s) error = %v", code, err)
	}
	return published.CurrentPublished
}

func sceneTemplateInput(name, text string) facade.TemplateRevisionDraftInput {
	return facade.TemplateRevisionDraftInput{
		TemplateName:               name,
		Locale:                     "zh-CN",
		SubjectTemplate:            "通知 {{.name}}",
		TextTemplate:               text,
		PrimaryActionLabelTemplate: "查看任务",
		Variables: []facade.TemplateRevisionVariable{{
			Name:           "name",
			Type:           domain.TemplateVariableTypeString,
			Required:       true,
			MaxLength:      80,
			Classification: domain.TemplateVariableClassificationPublic,
		}},
	}
}

type capabilityTestRegistry struct{ driver ChannelDriver }

func (r capabilityTestRegistry) Driver(string) ChannelDriver { return r.driver }
func (capabilityTestRegistry) Capabilities(channelType string) domain.DriverCapabilities {
	if channelType == domain.ChannelTypeEmail {
		return domain.DriverCapabilities{SupportedPresentations: []string{domain.DeliveryPresentationHTML, domain.DeliveryPresentationText}, SupportsPrimaryAction: true, SupportsExternalURL: true}
	}
	return domain.DriverCapabilities{SupportedPresentations: []string{domain.DeliveryPresentationText}}
}

func createPublishedScene(t *testing.T, service *Service, code, name, receiverKind string, templateRevisionID int64, connectionRef string, enabled bool) *facade.SceneDefinitionRecord {
	t.Helper()
	created, err := service.CreateSceneDefinition(context.Background(), facade.SceneDefinitionCreateRequest{SceneCode: code, Draft: facade.SceneRevisionDraftInput{SceneName: name, ReceiverKind: receiverKind, TemplateRevisionID: templateRevisionID, ConnectionRef: connectionRef, Enabled: enabled}}, 1)
	if err != nil {
		t.Fatalf("CreateSceneDefinition(%s/%s) error = %v", code, receiverKind, err)
	}
	published, err := service.PublishSceneRevision(context.Background(), created.CurrentDraft.ID, facade.SceneRevisionPublishRequest{ExpectedVersion: created.CurrentDraft.RevisionVersion}, 1)
	if err != nil {
		t.Fatalf("PublishSceneRevision(%s/%s) error = %v", code, receiverKind, err)
	}
	return published
}

type sceneRevisionTestRepository struct {
	*externalTestRepository
	templateDefinitions          map[int64]*domain.TemplateDefinition
	templateRevisions            map[int64]*domain.TemplateRevision
	templateAudits               []*domain.TemplateRevisionAudit
	legacyTemplates              map[string]*domain.Template
	sceneDefinitions             map[int64]*domain.SceneDefinition
	sceneRevisions               map[int64]*domain.SceneRevision
	sceneAudits                  []*domain.SceneRevisionAudit
	sceneSnapshots               map[int64]*domain.SceneSnapshot
	legacyBindings               map[int64]*domain.SceneBinding
	findSceneRevisionByIDCalls   int
	listSceneRevisionsByIDsCalls int
	lastSceneRevisionBatchIDs    []int64
}

func newSceneRevisionTestRepository() *sceneRevisionTestRepository {
	return &sceneRevisionTestRepository{
		externalTestRepository: newExternalTestRepository(),
		templateDefinitions:    map[int64]*domain.TemplateDefinition{},
		templateRevisions:      map[int64]*domain.TemplateRevision{},
		legacyTemplates:        map[string]*domain.Template{},
		sceneDefinitions:       map[int64]*domain.SceneDefinition{},
		sceneRevisions:         map[int64]*domain.SceneRevision{},
		sceneSnapshots:         map[int64]*domain.SceneSnapshot{},
		legacyBindings:         map[int64]*domain.SceneBinding{},
	}
}

func newSceneRevisionTestService(t *testing.T, repo *sceneRevisionTestRepository) *Service {
	t.Helper()
	idGen, err := xid.New(91)
	if err != nil {
		t.Fatalf("xid.New() error = %v", err)
	}
	service := NewService(externalTestTransactor{}, repo, domain.NewService(), inboxTestSecretService{}, driverRegistryFunc(func(string) ChannelDriver {
		return &externalResultDriver{result: DriverResult{Status: DriverResultProviderAccepted}}
	}), nil, nil, idGen)
	service.SetScopeID("local")
	service.BindExternalTargetDigester(externalTestDigester{})
	service.now = func() time.Time { return time.Date(2026, 7, 27, 16, 0, 0, 0, time.UTC) }
	return service
}

func staticHTTPConnectorTestChannel(t *testing.T, code, _ string) *domain.Channel {
	t.Helper()
	config, err := domain.EncodeWebhookProfileConfig(domain.WebhookProfileConfig{TimeoutMilliseconds: 5000, SuccessStatusCodes: []int{200}})
	if err != nil {
		t.Fatalf("encode fixed connection config: %v", err)
	}
	return enterpriseTestChannel(t, domain.ChannelTypeFeishuWebhook, code, config, "{}")
}

func onlyExternalDeliveryID(t *testing.T, repo *externalTestRepository) string {
	t.Helper()
	if len(repo.deliveries) != 1 {
		t.Fatalf("deliveries=%d, want one", len(repo.deliveries))
	}
	for deliveryID := range repo.deliveries {
		return deliveryID
	}
	return ""
}

func (r *sceneRevisionTestRepository) ListTemplateDefinitions(_ context.Context, query domain.TemplateDefinitionQuery) ([]domain.TemplateDefinition, int64, error) {
	items := make([]domain.TemplateDefinition, 0)
	for _, item := range r.templateDefinitions {
		if item.ScopeID == query.ScopeID && item.IsDeleted == 0 && (query.Keyword == "" || strings.Contains(item.TemplateCode, query.Keyword) || strings.Contains(item.TemplateName, query.Keyword)) {
			items = append(items, *cloneTemplateDefinition(item))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID > items[j].ID })
	return items, int64(len(items)), nil
}

func (r *sceneRevisionTestRepository) FindTemplateDefinitionByCode(_ context.Context, scopeID, templateCode string) (*domain.TemplateDefinition, error) {
	for _, item := range r.templateDefinitions {
		if item.ScopeID == scopeID && item.TemplateCode == templateCode && item.IsDeleted == 0 {
			return cloneTemplateDefinition(item), nil
		}
	}
	return nil, nil
}

func (r *sceneRevisionTestRepository) FindTemplateDefinitionByID(_ context.Context, definitionID int64) (*domain.TemplateDefinition, error) {
	return cloneTemplateDefinition(r.templateDefinitions[definitionID]), nil
}

func (r *sceneRevisionTestRepository) LockTemplateDefinitionByCode(ctx context.Context, scopeID, templateCode string) (*domain.TemplateDefinition, error) {
	return r.FindTemplateDefinitionByCode(ctx, scopeID, templateCode)
}

func (r *sceneRevisionTestRepository) FindTemplateRevisionByID(_ context.Context, revisionID int64) (*domain.TemplateRevision, error) {
	return cloneTemplateRevision(r.templateRevisions[revisionID]), nil
}

func (r *sceneRevisionTestRepository) ListTemplateRevisionsByIDs(_ context.Context, revisionIDs []int64) ([]domain.TemplateRevision, error) {
	items := make([]domain.TemplateRevision, 0, len(revisionIDs))
	for _, revisionID := range revisionIDs {
		if item := r.templateRevisions[revisionID]; item != nil {
			items = append(items, *cloneTemplateRevision(item))
		}
	}
	return items, nil
}

func (r *sceneRevisionTestRepository) FindTemplateRevisionByDefinitionAndState(_ context.Context, definitionID int64, state string) (*domain.TemplateRevision, error) {
	var result *domain.TemplateRevision
	for _, item := range r.templateRevisions {
		if item.TemplateDefinitionID == definitionID && item.State == state && (result == nil || item.RevisionNo > result.RevisionNo) {
			result = item
		}
	}
	return cloneTemplateRevision(result), nil
}

func (r *sceneRevisionTestRepository) ListTemplateRevisionsByDefinition(_ context.Context, definitionID int64) ([]domain.TemplateRevision, error) {
	items := make([]domain.TemplateRevision, 0)
	for _, item := range r.templateRevisions {
		if item.TemplateDefinitionID == definitionID {
			items = append(items, *cloneTemplateRevision(item))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].RevisionNo > items[j].RevisionNo })
	return items, nil
}

func (r *sceneRevisionTestRepository) InsertTemplateDefinition(_ context.Context, item *domain.TemplateDefinition) error {
	if item == nil || r.templateDefinitions[item.ID] != nil {
		return errors.New("invalid template definition")
	}
	copy := cloneTemplateDefinition(item)
	copy.CreateTime = time.Now()
	copy.UpdateTime = copy.CreateTime
	r.templateDefinitions[copy.ID] = copy
	return nil
}

func (r *sceneRevisionTestRepository) InsertTemplateRevision(_ context.Context, item *domain.TemplateRevision) error {
	if item == nil || r.templateRevisions[item.ID] != nil {
		return errors.New("invalid template revision")
	}
	copy := cloneTemplateRevision(item)
	copy.CreateTime = time.Now()
	copy.UpdateTime = copy.CreateTime
	r.templateRevisions[copy.ID] = copy
	return nil
}

func (r *sceneRevisionTestRepository) UpdateTemplateDefinitionMetadata(_ context.Context, definitionID int64, name, locale string, actorID int64) error {
	item := r.templateDefinitions[definitionID]
	if item == nil {
		return domain.ErrTemplateDefinitionNotFound
	}
	item.TemplateName, item.Locale, item.UpdaterID = name, locale, int64Ptr(actorID)
	return nil
}

func (r *sceneRevisionTestRepository) UpdateTemplateRevisionDraft(_ context.Context, item *domain.TemplateRevision, expected int) (bool, error) {
	current := r.templateRevisions[item.ID]
	if current == nil || current.State != domain.TemplateRevisionStateDraft || current.RevisionVersion != expected {
		return false, nil
	}
	current.SubjectTemplate, current.TextTemplate, current.HTMLTemplate, current.MarkdownTemplate = item.SubjectTemplate, item.TextTemplate, item.HTMLTemplate, item.MarkdownTemplate
	current.VariableSchemaJSON, current.ContentDigest, current.UpdaterID = item.VariableSchemaJSON, item.ContentDigest, item.UpdaterID
	current.RevisionVersion++
	return true, nil
}

func (r *sceneRevisionTestRepository) SetTemplateDefinitionDraft(_ context.Context, definitionID, revisionID int64, expected int) (bool, error) {
	definition := r.templateDefinitions[definitionID]
	if definition == nil || definition.CurrentDraftRevisionID != nil || definition.Version != expected {
		return false, nil
	}
	definition.CurrentDraftRevisionID = int64Ptr(revisionID)
	definition.Version++
	return true, nil
}

func (r *sceneRevisionTestRepository) PublishTemplateRevision(_ context.Context, definitionID, revisionID int64, expected int, actorID int64, at time.Time) (bool, error) {
	definition, candidate := r.templateDefinitions[definitionID], r.templateRevisions[revisionID]
	if definition == nil || candidate == nil || definition.CurrentDraftRevisionID == nil || *definition.CurrentDraftRevisionID != revisionID || candidate.State != domain.TemplateRevisionStateDraft || candidate.RevisionVersion != expected {
		return false, nil
	}
	for _, item := range r.templateRevisions {
		if item.TemplateDefinitionID == definitionID && item.State == domain.TemplateRevisionStatePublished {
			item.State = domain.TemplateRevisionStateSuperseded
		}
	}
	candidate.State, candidate.RevisionVersion, candidate.PublishedAt, candidate.PublishedBy = domain.TemplateRevisionStatePublished, candidate.RevisionVersion+1, &at, int64Ptr(actorID)
	definition.CurrentDraftRevisionID, definition.CurrentPublishedRevisionID, definition.Version = nil, int64Ptr(revisionID), definition.Version+1
	return true, nil
}

func (r *sceneRevisionTestRepository) InsertTemplateRevisionAudit(_ context.Context, item *domain.TemplateRevisionAudit) error {
	copy := *item
	r.templateAudits = append(r.templateAudits, &copy)
	return nil
}

func (r *sceneRevisionTestRepository) FindTemplateByCode(_ context.Context, code string) (*domain.Template, error) {
	if item := r.legacyTemplates[code]; item != nil {
		copy := *item
		return &copy, nil
	}
	return nil, nil
}

func (r *sceneRevisionTestRepository) ListSceneDefinitions(_ context.Context, query domain.SceneDefinitionQuery) ([]domain.SceneDefinition, int64, error) {
	items := make([]domain.SceneDefinition, 0)
	for _, item := range r.sceneDefinitions {
		if item.ScopeID == query.ScopeID && item.IsDeleted == 0 && (query.Keyword == "" || strings.Contains(item.SceneCode, query.Keyword) || strings.Contains(item.SceneName, query.Keyword)) {
			items = append(items, *cloneSceneDefinition(item))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID > items[j].ID })
	return items, int64(len(items)), nil
}

func (r *sceneRevisionTestRepository) FindSceneDefinitionByCodeAndReceiverKind(_ context.Context, scopeID, code, receiverKind string) (*domain.SceneDefinition, error) {
	for _, item := range r.sceneDefinitions {
		if item.ScopeID == scopeID && item.SceneCode == code && item.ReceiverKind == receiverKind && item.IsDeleted == 0 {
			return cloneSceneDefinition(item), nil
		}
	}
	return nil, nil
}

func (r *sceneRevisionTestRepository) FindSceneDefinitionByID(_ context.Context, id int64) (*domain.SceneDefinition, error) {
	return cloneSceneDefinition(r.sceneDefinitions[id]), nil
}

func (r *sceneRevisionTestRepository) LockSceneDefinitionByCodeAndReceiverKind(ctx context.Context, scopeID, code, receiverKind string) (*domain.SceneDefinition, error) {
	return r.FindSceneDefinitionByCodeAndReceiverKind(ctx, scopeID, code, receiverKind)
}

func (r *sceneRevisionTestRepository) FindSceneRevisionByID(_ context.Context, id int64) (*domain.SceneRevision, error) {
	r.findSceneRevisionByIDCalls++
	return cloneSceneRevision(r.sceneRevisions[id]), nil
}

func (r *sceneRevisionTestRepository) ListSceneRevisionsByIDs(_ context.Context, revisionIDs []int64) ([]domain.SceneRevision, error) {
	r.listSceneRevisionsByIDsCalls++
	r.lastSceneRevisionBatchIDs = append([]int64(nil), revisionIDs...)
	items := make([]domain.SceneRevision, 0, len(revisionIDs))
	for _, revisionID := range revisionIDs {
		if item := r.sceneRevisions[revisionID]; item != nil {
			items = append(items, *cloneSceneRevision(item))
		}
	}
	return items, nil
}

func (r *sceneRevisionTestRepository) ListSceneRevisionsByDefinition(_ context.Context, definitionID int64) ([]domain.SceneRevision, error) {
	items := make([]domain.SceneRevision, 0)
	for _, item := range r.sceneRevisions {
		if item.SceneDefinitionID == definitionID {
			items = append(items, *cloneSceneRevision(item))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].RevisionNo > items[j].RevisionNo })
	return items, nil
}

func (r *sceneRevisionTestRepository) InsertSceneDefinition(_ context.Context, item *domain.SceneDefinition) error {
	if item == nil || r.sceneDefinitions[item.ID] != nil {
		return errors.New("invalid scene definition")
	}
	r.sceneDefinitions[item.ID] = cloneSceneDefinition(item)
	return nil
}

func (r *sceneRevisionTestRepository) InsertSceneRevision(_ context.Context, item *domain.SceneRevision) error {
	if item == nil || r.sceneRevisions[item.ID] != nil {
		return errors.New("invalid scene revision")
	}
	r.sceneRevisions[item.ID] = cloneSceneRevision(item)
	return nil
}

func (r *sceneRevisionTestRepository) UpdateSceneDefinitionMetadata(_ context.Context, definitionID int64, name string, actorID int64) error {
	item := r.sceneDefinitions[definitionID]
	if item == nil {
		return domain.ErrSceneDefinitionNotFound
	}
	item.SceneName, item.UpdaterID = name, int64Ptr(actorID)
	return nil
}

func (r *sceneRevisionTestRepository) UpdateSceneRevisionDraft(_ context.Context, item *domain.SceneRevision, expected int) (bool, error) {
	current := r.sceneRevisions[item.ID]
	if current == nil || current.State != domain.SceneRevisionStateDraft || current.RevisionVersion != expected {
		return false, nil
	}
	current.Enabled, current.TemplateRevisionID, current.ConnectionRef, current.ConnectionDigest, current.UpdaterID = item.Enabled, item.TemplateRevisionID, item.ConnectionRef, item.ConnectionDigest, item.UpdaterID
	current.RevisionVersion++
	return true, nil
}

func (r *sceneRevisionTestRepository) SetSceneDefinitionDraft(_ context.Context, definitionID, revisionID int64, expected int) (bool, error) {
	definition := r.sceneDefinitions[definitionID]
	if definition == nil || definition.CurrentDraftRevisionID != nil || definition.Version != expected {
		return false, nil
	}
	definition.CurrentDraftRevisionID, definition.Version = int64Ptr(revisionID), definition.Version+1
	return true, nil
}

func (r *sceneRevisionTestRepository) PublishSceneRevision(_ context.Context, definitionID, revisionID int64, expected int, actorID int64, at time.Time) (bool, error) {
	definition, candidate := r.sceneDefinitions[definitionID], r.sceneRevisions[revisionID]
	if definition == nil || candidate == nil || definition.CurrentDraftRevisionID == nil || *definition.CurrentDraftRevisionID != revisionID || candidate.State != domain.SceneRevisionStateDraft || candidate.RevisionVersion != expected {
		return false, nil
	}
	for _, item := range r.sceneRevisions {
		if item.SceneDefinitionID == definitionID && item.State == domain.SceneRevisionStatePublished {
			item.State = domain.SceneRevisionStateSuperseded
		}
	}
	candidate.State, candidate.RevisionVersion, candidate.PublishedAt, candidate.PublishedBy = domain.SceneRevisionStatePublished, candidate.RevisionVersion+1, &at, int64Ptr(actorID)
	definition.CurrentDraftRevisionID, definition.CurrentPublishedRevisionID, definition.Version = nil, int64Ptr(revisionID), definition.Version+1
	return true, nil
}

func (r *sceneRevisionTestRepository) InsertSceneRevisionAudit(_ context.Context, item *domain.SceneRevisionAudit) error {
	copy := *item
	r.sceneAudits = append(r.sceneAudits, &copy)
	return nil
}

func (r *sceneRevisionTestRepository) InsertSceneSnapshot(_ context.Context, item *domain.SceneSnapshot) error {
	if item == nil || r.sceneSnapshots[item.ID] != nil {
		return errors.New("invalid scene snapshot")
	}
	copy := *item
	r.sceneSnapshots[item.ID] = &copy
	return nil
}

func (r *sceneRevisionTestRepository) ListSceneSnapshotsByNotificationID(_ context.Context, notificationID int64) ([]domain.SceneSnapshot, error) {
	items := make([]domain.SceneSnapshot, 0)
	for _, item := range r.sceneSnapshots {
		if item.NotificationID == notificationID {
			items = append(items, *item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func cloneSceneDefinition(item *domain.SceneDefinition) *domain.SceneDefinition {
	if item == nil {
		return nil
	}
	copy := *item
	if item.CurrentDraftRevisionID != nil {
		copy.CurrentDraftRevisionID = int64Ptr(*item.CurrentDraftRevisionID)
	}
	if item.CurrentPublishedRevisionID != nil {
		copy.CurrentPublishedRevisionID = int64Ptr(*item.CurrentPublishedRevisionID)
	}
	return &copy
}

func cloneSceneRevision(item *domain.SceneRevision) *domain.SceneRevision {
	if item == nil {
		return nil
	}
	copy := *item
	if item.PublishedAt != nil {
		value := *item.PublishedAt
		copy.PublishedAt = &value
	}
	if item.PublishedBy != nil {
		copy.PublishedBy = int64Ptr(*item.PublishedBy)
	}
	return &copy
}
