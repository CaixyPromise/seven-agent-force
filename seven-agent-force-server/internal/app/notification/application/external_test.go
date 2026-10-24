package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	dbstore "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
)

func TestPrepareStaticRoutesLoadsDistinctChannelsInOneQuery(t *testing.T) {
	repo := newExternalTestRepository()
	repo.channels["route-a"] = staticHTTPConnectorTestChannel(t, "route-a", "unused")
	repo.channels["route-b"] = staticHTTPConnectorTestChannel(t, "route-b", "unused")
	service := newExternalTestService(t, repo, nil)

	items, err := service.prepareStaticRoutes(context.Background(), []facade.StaticRoute{
		{ConnectionRef: "route-b"},
		{ConnectionRef: "route-a"},
	})
	if err != nil {
		t.Fatalf("prepare static routes: %v", err)
	}
	if len(items) != 2 || repo.listChannelCalls != 1 || repo.findChannelCalls != 0 {
		t.Fatalf("items=%d batch=%d single=%d, want two items from one batch query", len(items), repo.listChannelCalls, repo.findChannelCalls)
	}
}

func TestCreateExternalTargetsAndDeliveriesUsesBatchWrites(t *testing.T) {
	repo := newExternalTestRepository()
	service := newExternalTestService(t, repo, nil)
	channel := enterpriseTestChannel(t, domain.ChannelTypeFeishuApp, "feishu-app", `{"appId":"cli_test"}`, "{}")
	notification := &domain.LogicalNotification{
		ID:             1001,
		NotificationID: "ntf_1001",
		ScopeID:        "local",
		EventKey:       "invoice.created",
		Title:          "Invoice",
	}
	items := []preparedExternalRecipient{
		{channel: *channel, identityKind: domain.ExternalIdentityFeishuOpenID, subject: "ou_a", subjectDigest: "digest-a", subjectDigestKeyRef: "kid", providerParamsJSON: "{}"},
		{channel: *channel, identityKind: domain.ExternalIdentityFeishuOpenID, subject: "ou_b", subjectDigest: "digest-b", subjectDigestKeyRef: "kid", providerParamsJSON: "{}"},
	}

	if err := service.createExternalTargetsAndDeliveries(context.Background(), notification, items); err != nil {
		t.Fatalf("create external targets and deliveries: %v", err)
	}
	if repo.insertDeliveryCalls != 0 || repo.appendOutboxCalls != 0 {
		t.Fatalf("individual writes delivery=%d outbox=%d, want zero", repo.insertDeliveryCalls, repo.appendOutboxCalls)
	}
	if repo.insertDeliveryBatchCalls != 1 || repo.appendOutboxBatchCalls != 1 {
		t.Fatalf("batch writes delivery=%d outbox=%d, want one each", repo.insertDeliveryBatchCalls, repo.appendOutboxBatchCalls)
	}
	if len(repo.deliveries) != 2 || len(repo.outbox) != 2 {
		t.Fatalf("persisted deliveries=%d outbox=%d, want two each", len(repo.deliveries), len(repo.outbox))
	}
}

func TestCreateStaticRouteDeliveriesUsesThreeBoundedBatchWrites(t *testing.T) {
	repo := newExternalTestRepository()
	service := newExternalTestService(t, repo, nil)
	items := make([]preparedStaticRoute, 0, 3)
	for index := 1; index <= 3; index++ {
		items = append(items, preparedStaticRoute{channel: *staticHTTPConnectorTestChannel(t, fmt.Sprintf("route-%d", index), "unused")})
	}
	notification := &domain.LogicalNotification{
		ID:             1002,
		NotificationID: "ntf_1002",
		ScopeID:        "local",
		EventKey:       "system.static",
		Title:          "Static",
		Content:        "Route",
	}

	if err := service.createStaticRouteDeliveries(context.Background(), notification, items); err != nil {
		t.Fatalf("create static route deliveries: %v", err)
	}
	if repo.insertDeliveryCalls != 0 || repo.insertHTTPSnapshotCalls != 0 || repo.appendOutboxCalls != 0 {
		t.Fatalf("individual writes delivery=%d snapshot=%d outbox=%d, want zero", repo.insertDeliveryCalls, repo.insertHTTPSnapshotCalls, repo.appendOutboxCalls)
	}
	if repo.insertDeliveryBatchCalls != 1 || repo.insertHTTPSnapshotBatchCalls != 1 || repo.appendOutboxBatchCalls != 1 {
		t.Fatalf("batch writes delivery=%d snapshot=%d outbox=%d, want one each", repo.insertDeliveryBatchCalls, repo.insertHTTPSnapshotBatchCalls, repo.appendOutboxBatchCalls)
	}
	if len(repo.deliveries) != 3 || len(repo.httpSnapshots) != 3 || len(repo.outbox) != 3 {
		t.Fatalf("persisted delivery=%d snapshot=%d outbox=%d, want three each", len(repo.deliveries), len(repo.httpSnapshots), len(repo.outbox))
	}
}

func TestPublishWithoutTransactionFailsBeforeRepositoryWrites(t *testing.T) {
	cases := []struct {
		name string
		tx   dbstore.Transactor
	}{
		{name: "nil"},
		{name: "disabled", tx: disabledExternalTestTransactor{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newExternalTestRepository()
			idGen, err := xid.New(23)
			if err != nil {
				t.Fatalf("new id generator: %v", err)
			}
			service := NewService(tc.tx, repo, domain.NewService(), inboxTestSecretService{}, nil, nil, nil, idGen)
			service.SetScopeID("local")

			_, err = service.Publish(context.Background(), facade.PublishRequest{
				EventKey:       "transaction.required",
				IdempotencyKey: "transaction-required-1",
				Audience:       facade.Audience{UserIDs: []int64{101}},
				Category:       "GENERAL",
				Priority:       "NORMAL",
				Title:          "Transaction",
				Content:        "Must fail before writes",
			})
			if err == nil || !strings.Contains(err.Error(), "transaction") {
				t.Fatalf("Publish() error=%v, want transaction configuration failure", err)
			}
			if len(repo.notifications) != 0 || repo.inboxRecipientWrites != 0 || len(repo.externalTargets) != 0 || len(repo.deliveries) != 0 || len(repo.outbox) != 0 {
				t.Fatalf("transaction failure wrote notifications=%d inbox=%d targets=%d deliveries=%d outbox=%d",
					len(repo.notifications), repo.inboxRecipientWrites, len(repo.externalTargets), len(repo.deliveries), len(repo.outbox))
			}
		})
	}
}

func TestCreateExternalTargetsAndDeliveriesRejectsMoreThanLimitBeforeWrites(t *testing.T) {
	repo := newExternalTestRepository()
	service := newExternalTestService(t, repo, nil)
	channel := enterpriseTestChannel(t, domain.ChannelTypeFeishuApp, "feishu-app", `{"appId":"cli_test"}`, "{}")
	items := make([]preparedExternalRecipient, externalRecipientLimit+1)
	for index := range items {
		items[index] = preparedExternalRecipient{
			channel:             *channel,
			identityKind:        domain.ExternalIdentityFeishuOpenID,
			subject:             fmt.Sprintf("ou_%d", index),
			subjectDigest:       fmt.Sprintf("digest-%d", index),
			subjectDigestKeyRef: "kid",
			providerParamsJSON:  "{}",
		}
	}

	err := service.createExternalTargetsAndDeliveries(context.Background(), &domain.LogicalNotification{ID: 1001}, items)
	if err == nil {
		t.Fatal("expected external recipient hard limit rejection")
	}
	if len(repo.externalTargets) != 0 || len(repo.deliveries) != 0 || len(repo.outbox) != 0 {
		t.Fatalf("limit rejection wrote targets=%d deliveries=%d outbox=%d", len(repo.externalTargets), len(repo.deliveries), len(repo.outbox))
	}
}

func TestAppendInboxChangedIntentsUsesOneBatchWrite(t *testing.T) {
	repo := newExternalTestRepository()
	service := newExternalTestService(t, repo, nil)
	recipients := []domain.Recipient{
		{RecipientID: "nrc_2", ScopeID: "local", UserID: 2, MailboxVersion: 1},
		{RecipientID: "nrc_1", ScopeID: "local", UserID: 1, MailboxVersion: 1},
	}

	if err := service.appendInboxChangedIntents(context.Background(), recipients, true); err != nil {
		t.Fatalf("append inbox intents: %v", err)
	}
	if repo.appendOutboxCalls != 0 || repo.appendOutboxBatchCalls != 1 {
		t.Fatalf("outbox individual=%d batch=%d, want 0/1", repo.appendOutboxCalls, repo.appendOutboxBatchCalls)
	}
	if len(repo.outbox) != 2 || repo.outbox[0].AggregateID != "nrc_1" || repo.outbox[1].AggregateID != "nrc_2" {
		t.Fatalf("outbox order=%#v, want deterministic mailbox order", repo.outbox)
	}
}

func TestPublishExternalTargetIsEncryptedIdempotentAndDoesNotCreateInboxRecipient(t *testing.T) {
	repo := newExternalTestRepository()
	repo.channels["feishu-app"] = enterpriseTestChannel(t, domain.ChannelTypeFeishuApp, "feishu-app", `{"appId":"cli_test"}`, "{}")
	service := newExternalTestService(t, repo, driverRegistryFunc(func(string) ChannelDriver {
		return &externalResultDriver{result: DriverResult{Status: DriverResultProviderAccepted, ProviderReference: "om_test"}}
	}))
	request := facade.PublishRequest{
		EventKey:       "billing.invoice.ready",
		IdempotencyKey: "invoice-ext-1001",
		Category:       "BILLING",
		Priority:       "NORMAL",
		Title:          "账单已生成",
		Content:        strings.Repeat("不应以原始收件箱正文形式持久化第三方成员标识。", 80),
		ExternalRecipients: []facade.ExternalRecipient{{
			ConnectionRef:  "feishu-app",
			IdentityKind:   facade.ExternalIdentityFeishuOpenID,
			Subject:        "ou_7d7a3d3d",
			ProviderParams: map[string]any{"headers": "must-ignore"},
		}},
	}

	first, err := service.Publish(context.Background(), request)
	if err != nil {
		t.Fatalf("publish external notification: %v", err)
	}
	if first.Duplicate || len(first.Warnings) != 1 || first.Warnings[0].Key != "headers" || first.Warnings[0].Reason != "PROTECTED_KEY" {
		t.Fatalf("first receipt=%#v, want non-blocking protected-param warning", first)
	}
	second, err := service.Publish(context.Background(), request)
	if err != nil {
		t.Fatalf("repeat external publish: %v", err)
	}
	if !second.Duplicate || second.NotificationID != first.NotificationID {
		t.Fatalf("idempotent external receipt first=%#v second=%#v", first, second)
	}
	if len(repo.notifications) != 1 || len(repo.externalTargets) != 1 || len(repo.deliveries) != 1 {
		t.Fatalf("external persistence notifications=%d targets=%d deliveries=%d", len(repo.notifications), len(repo.externalTargets), len(repo.deliveries))
	}
	if repo.inboxRecipientWrites != 1 {
		t.Fatalf("empty platform audience must be evaluated once, writes=%d", repo.inboxRecipientWrites)
	}
	for _, target := range repo.externalTargets {
		if strings.Contains(target.SubjectCiphertext, "ou_7d7a3d3d") || target.SubjectDigest == "" || target.SubjectDigestKeyRef == "" {
			t.Fatalf("external target must encrypt subject and retain keyed digest: %#v", target)
		}
	}
	for _, delivery := range repo.deliveries {
		if delivery.Target != "" || delivery.ExternalTargetID == nil || delivery.NotificationID == nil || strings.Contains(delivery.TargetMasked, "ou_7d7a3d3d") {
			t.Fatalf("external delivery leaked or lost target boundary: %#v", delivery)
		}
		if len([]rune(delivery.RenderedText)) > externalTextLimit || !strings.Contains(delivery.RenderedText, externalDetailHint) || strings.Contains(delivery.RenderedText, "不应以原始收件箱正文形式持久化第三方成员标识") {
			t.Fatalf("external delivery must retain only the safe outbound text, got %q", delivery.RenderedText)
		}
	}
	if got := countOutboxEvent(repo.outbox, domain.OutboxEventNotificationInboxChanged); got != 0 {
		t.Fatalf("external-only publish emitted inbox change events=%d", got)
	}
	if got := countOutboxEvent(repo.outbox, domain.OutboxEventNotificationDispatch); got != 1 {
		t.Fatalf("external-only publish dispatch events=%d, want 1", got)
	}
}

func TestPublishStaticRouteIdempotencyIgnoresRetryTraceID(t *testing.T) {
	repo := newExternalTestRepository()
	channel := staticHTTPConnectorTestChannel(t, "fixed-trace-route", "unused")
	repo.channels[channel.ChannelCode] = channel
	service := newExternalTestService(t, repo, nil)
	request := facade.PublishRequest{
		EventKey:       "ops.fixed.notice",
		IdempotencyKey: "fixed-trace-retry",
		Title:          "固定连接提醒",
		Content:        "固定连接正文",
		TraceID:        "trace-first",
		StaticRoutes:   []facade.StaticRoute{{ConnectionRef: channel.ChannelCode}},
	}
	first, err := service.Publish(context.Background(), request)
	if err != nil {
		t.Fatalf("first static-route publish: %v", err)
	}
	retry := request
	retry.TraceID = "trace-retry"
	second, err := service.Publish(context.Background(), retry)
	if err != nil {
		t.Fatalf("static-route trace-only retry: %v", err)
	}
	if !second.Duplicate || second.NotificationID != first.NotificationID || len(repo.notifications) != 1 || len(repo.deliveries) != 1 {
		t.Fatalf("static-route retry first=%#v second=%#v notifications=%d deliveries=%d", first, second, len(repo.notifications), len(repo.deliveries))
	}
	for _, item := range repo.notifications {
		if item.TraceID != "trace-first" {
			t.Fatalf("static-route accepted trace=%q, want first trace", item.TraceID)
		}
	}
}

func TestPublishFeishuGroupTargetRemainsExternalAndDistinctFromUserTarget(t *testing.T) {
	repo := newExternalTestRepository()
	repo.channels["feishu-app"] = enterpriseTestChannel(t, domain.ChannelTypeFeishuApp, "feishu-app", `{"appId":"cli_test"}`, "{}")
	driver := &externalResultDriver{result: DriverResult{Status: DriverResultProviderAccepted, ProviderReference: "om_group"}}
	service := newExternalTestService(t, repo, driverRegistryFunc(func(string) ChannelDriver { return driver }))

	_, err := service.Publish(context.Background(), facade.PublishRequest{
		EventKey:       "ops.group.notice",
		IdempotencyKey: "feishu-group-target-1",
		Category:       "SYSTEM",
		Priority:       "NORMAL",
		Title:          "系统提醒",
		Content:        "完整正文只能留在系统内。",
		ExternalRecipients: []facade.ExternalRecipient{
			{ConnectionRef: "feishu-app", IdentityKind: facade.ExternalIdentityFeishuOpenID, Subject: "same-provider-subject"},
			{ConnectionRef: "feishu-app", IdentityKind: facade.ExternalIdentityFeishuChatID, Subject: "same-provider-subject"},
		},
	})
	if err != nil {
		t.Fatalf("publish Feishu direct-user and group targets: %v", err)
	}
	if len(repo.externalTargets) != 2 || len(repo.deliveries) != 2 || repo.inboxRecipients != 0 {
		t.Fatalf("group and user targets must be separate external deliveries: targets=%d deliveries=%d inbox=%d", len(repo.externalTargets), len(repo.deliveries), repo.inboxRecipients)
	}
	seenKinds := map[string]bool{}
	for _, target := range repo.externalTargets {
		seenKinds[target.IdentityKind] = true
		if strings.Contains(target.SubjectCiphertext, "same-provider-subject") {
			t.Fatalf("external subject must remain encrypted: %#v", target)
		}
	}
	if !seenKinds[domain.ExternalIdentityFeishuOpenID] || !seenKinds[domain.ExternalIdentityFeishuChatID] {
		t.Fatalf("external target kinds=%#v, want direct user and group", seenKinds)
	}
	for deliveryID := range repo.deliveries {
		if err := service.dispatch(context.Background(), deliveryID); err != nil {
			t.Fatalf("dispatch %s: %v", deliveryID, err)
		}
	}
	if len(driver.messages) != 2 {
		t.Fatalf("driver messages=%d, want two", len(driver.messages))
	}
	dispatchedKinds := map[string]bool{}
	for _, message := range driver.messages {
		dispatchedKinds[message.IdentityKind] = true
	}
	if !dispatchedKinds[domain.ExternalIdentityFeishuOpenID] || !dispatchedKinds[domain.ExternalIdentityFeishuChatID] {
		t.Fatalf("driver identity kinds=%#v, want direct user and group", dispatchedKinds)
	}
	if got := countOutboxEvent(repo.outbox, domain.OutboxEventNotificationInboxChanged); got != 0 || repo.inboxMutations != 0 {
		t.Fatalf("external Feishu direct/group targets must not create inbox hints or mutations: hints=%d mutations=%d", got, repo.inboxMutations)
	}
}

func TestFeishuGroupDeliveryFailsClosedAfterChannelDisableWithoutInboxEffects(t *testing.T) {
	repo := newExternalTestRepository()
	repo.channels["feishu-app"] = enterpriseTestChannel(t, domain.ChannelTypeFeishuApp, "feishu-app", `{"appId":"cli_test"}`, "{}")
	driver := &externalResultDriver{result: DriverResult{Status: DriverResultProviderAccepted, ProviderReference: "om_unexpected"}}
	service := newExternalTestService(t, repo, driverRegistryFunc(func(string) ChannelDriver { return driver }))

	if _, err := service.Publish(context.Background(), facade.PublishRequest{
		EventKey:       "ops.group.notice",
		IdempotencyKey: "feishu-group-disabled-before-dispatch-1",
		Category:       "SYSTEM",
		Priority:       "NORMAL",
		Title:          "系统提醒",
		Content:        "外部群聊不应生成站内信。",
		ExternalRecipients: []facade.ExternalRecipient{{
			ConnectionRef: "feishu-app",
			IdentityKind:  facade.ExternalIdentityFeishuChatID,
			Subject:       "oc_group_target",
		}},
	}); err != nil {
		t.Fatalf("publish Feishu group target: %v", err)
	}
	if len(repo.deliveries) != 1 || repo.inboxRecipients != 0 {
		t.Fatalf("external group publish state deliveries=%d inbox=%d", len(repo.deliveries), repo.inboxRecipients)
	}
	repo.channels["feishu-app"].Status = domain.ChannelStatusDisabled
	var deliveryID string
	for id := range repo.deliveries {
		deliveryID = id
	}
	if err := service.dispatch(context.Background(), deliveryID); err == nil || !isDeliveryAsyncHandled(err) {
		t.Fatalf("disabled channel dispatch error=%v, want persisted handled failure", err)
	}
	delivery := repo.deliveries[deliveryID]
	if delivery.Status != domain.DeliveryStatusFailed || delivery.LastError != "CONNECTION_UNAVAILABLE" || delivery.RetryCount != 1 {
		t.Fatalf("disabled channel delivery=%#v", delivery)
	}
	if len(driver.messages) != 0 || len(repo.attempts) != 1 || repo.attempts[0].FailureClass != "CONNECTION_UNAVAILABLE" {
		t.Fatalf("disabled channel must not call provider: messages=%d attempts=%#v", len(driver.messages), repo.attempts)
	}
	if repo.inboxRecipients != 0 || repo.inboxMutations != 0 || countOutboxEvent(repo.outbox, domain.OutboxEventNotificationInboxChanged) != 0 {
		t.Fatalf("disabled external group delivery polluted inbox: recipients=%d mutations=%d outbox=%#v", repo.inboxRecipients, repo.inboxMutations, repo.outbox)
	}
}

func TestRejectedFeishuGroupDeliveryDoesNotCreateInboxEffects(t *testing.T) {
	repo := newExternalTestRepository()
	repo.channels["feishu-app"] = enterpriseTestChannel(t, domain.ChannelTypeFeishuApp, "feishu-app", `{"appId":"cli_test"}`, "{}")
	driver := &externalResultDriver{result: DriverResult{Status: DriverResultFailed, FailureClass: "PROVIDER_REJECTED", Diagnostic: "FEISHU_REJECTED"}}
	service := newExternalTestService(t, repo, driverRegistryFunc(func(string) ChannelDriver { return driver }))

	if _, err := service.Publish(context.Background(), facade.PublishRequest{
		EventKey:       "ops.group.notice",
		IdempotencyKey: "feishu-group-provider-rejected-1",
		Category:       "SYSTEM",
		Priority:       "NORMAL",
		Title:          "系统提醒",
		Content:        "外部群聊拒绝不应产生站内信。",
		ExternalRecipients: []facade.ExternalRecipient{{
			ConnectionRef: "feishu-app",
			IdentityKind:  facade.ExternalIdentityFeishuChatID,
			Subject:       "oc_unavailable_group",
		}},
	}); err != nil {
		t.Fatalf("publish rejected Feishu group target: %v", err)
	}
	var deliveryID string
	for id := range repo.deliveries {
		deliveryID = id
	}
	if err := service.dispatch(context.Background(), deliveryID); err == nil || !isDeliveryAsyncHandled(err) {
		t.Fatalf("rejected group dispatch error=%v, want persisted handled failure", err)
	}
	delivery := repo.deliveries[deliveryID]
	if delivery.Status != domain.DeliveryStatusFailed || delivery.LastError != "FEISHU_REJECTED" || len(repo.attempts) != 1 || repo.attempts[0].FailureClass != "PROVIDER_REJECTED" {
		t.Fatalf("rejected group delivery=%#v attempts=%#v", delivery, repo.attempts)
	}
	if len(driver.messages) != 1 || repo.inboxRecipients != 0 || repo.inboxMutations != 0 || countOutboxEvent(repo.outbox, domain.OutboxEventNotificationInboxChanged) != 0 {
		t.Fatalf("rejected external group polluted inbox: messages=%d recipients=%d mutations=%d outbox=%#v", len(driver.messages), repo.inboxRecipients, repo.inboxMutations, repo.outbox)
	}
}

func TestPublishIgnoresWeComBroadcastMentionWithoutBlockingBaseNotification(t *testing.T) {
	repo := newExternalTestRepository()
	repo.channels["wecom-app"] = enterpriseTestChannel(t, domain.ChannelTypeWeComApp, "wecom-app", `{"corpId":"ww_test","agentId":"100001"}`, "{}")
	driver := &externalResultDriver{result: DriverResult{Status: DriverResultProviderAccepted, ProviderReference: "msg_123"}}
	service := newExternalTestService(t, repo, driverRegistryFunc(func(string) ChannelDriver { return driver }))
	receipt, err := service.Publish(context.Background(), facade.PublishRequest{
		EventKey:       "account.security.changed",
		IdempotencyKey: "wecom-ignore-broadcast-1",
		Audience:       facade.Audience{UserIDs: []int64{42}},
		Category:       "ACCOUNT",
		Priority:       "HIGH",
		Title:          "账号安全设置已更新",
		Content:        "站内信正文保持在系统内。",
		ExternalRecipients: []facade.ExternalRecipient{{
			ConnectionRef:  "wecom-app",
			IdentityKind:   facade.ExternalIdentityWeComUserID,
			Subject:        "external-member-7",
			ProviderParams: map[string]any{domain.ProviderParameterMentionedList: []string{"@all"}},
		}},
	})
	if err != nil {
		t.Fatalf("publish with disallowed optional mention: %v", err)
	}
	if len(receipt.Warnings) != 1 || receipt.Warnings[0].Key != domain.ProviderParameterMentionedList || receipt.Warnings[0].Reason != "DISALLOWED_VALUE" {
		t.Fatalf("receipt warnings=%#v, want disallowed optional mention warning", receipt.Warnings)
	}
	if len(repo.notifications) != 1 || repo.inboxRecipients != 1 || len(repo.externalTargets) != 1 || len(repo.deliveries) != 1 {
		t.Fatalf("base and external notification must be accepted independently: notifications=%d inboxRecipients=%d targets=%d deliveries=%d", len(repo.notifications), repo.inboxRecipients, len(repo.externalTargets), len(repo.deliveries))
	}
	for _, target := range repo.externalTargets {
		params, parseErr := domain.ParseProviderParamsJSON(target.ProviderParamsJSON)
		if parseErr != nil || len(params) != 0 {
			t.Fatalf("target parameter snapshot=%q params=%#v err=%v, want empty", target.ProviderParamsJSON, params, parseErr)
		}
	}
	var deliveryID string
	for id := range repo.deliveries {
		deliveryID = id
	}
	if err := service.dispatch(context.Background(), deliveryID); err != nil {
		t.Fatalf("dispatch without disallowed mention: %v", err)
	}
	if len(driver.messages) != 1 || len(driver.messages[0].ProviderParams) != 0 {
		t.Fatalf("driver messages=%#v, want one send with no broadcast parameter", driver.messages)
	}
}

func TestPublishRejectsWeComAggregateExternalTarget(t *testing.T) {
	repo := newExternalTestRepository()
	repo.channels["wecom-app"] = enterpriseTestChannel(t, domain.ChannelTypeWeComApp, "wecom-app", `{"corpId":"ww_test","agentId":"100001"}`, "{}")
	service := newExternalTestService(t, repo, nil)
	_, err := service.Publish(context.Background(), facade.PublishRequest{
		EventKey:       "account.security.changed",
		IdempotencyKey: "wecom-aggregate-target-1",
		Audience:       facade.Audience{UserIDs: []int64{42}},
		Category:       "ACCOUNT",
		Priority:       "HIGH",
		Title:          "账号安全设置已更新",
		Content:        "站内信正文保持在系统内。",
		ExternalRecipients: []facade.ExternalRecipient{{
			ConnectionRef: "wecom-app",
			IdentityKind:  facade.ExternalIdentityWeComUserID,
			Subject:       "member-one|@all",
		}},
	})
	if err == nil {
		t.Fatal("aggregate WeCom target must be rejected")
	}
	if len(repo.notifications) != 0 || len(repo.externalTargets) != 0 || len(repo.deliveries) != 0 || len(repo.outbox) != 0 {
		t.Fatalf("rejected aggregate target must not persist notification state: notifications=%d targets=%d deliveries=%d outbox=%d", len(repo.notifications), len(repo.externalTargets), len(repo.deliveries), len(repo.outbox))
	}
}

func TestExternalDeliveryUsesSnapshotAndDoesNotMutateInboxOnProviderFailure(t *testing.T) {
	repo := newExternalTestRepository()
	repo.channels["wecom-app"] = enterpriseTestChannel(t, domain.ChannelTypeWeComApp, "wecom-app", `{"corpId":"ww_test","agentId":"100001"}`, `{"providerParameterSettings":[{"key":"mentionedList","enabled":true,"defaultValue":["default-user"]}]}`)
	driver := &externalResultDriver{result: DriverResult{Status: DriverResultFailed, FailureClass: "INVALID_TARGET", Diagnostic: "INVALID_TARGET"}}
	service := newExternalTestService(t, repo, driverRegistryFunc(func(string) ChannelDriver { return driver }))
	request := facade.PublishRequest{
		EventKey:       "account.security.changed",
		IdempotencyKey: "wecom-snapshot-1",
		Audience:       facade.Audience{UserIDs: []int64{42}},
		Category:       "ACCOUNT",
		Priority:       "HIGH",
		Title:          "账号安全设置已更新",
		Content:        "这是正文；外部投递只能使用受限文本。",
		ExternalRecipients: []facade.ExternalRecipient{{
			ConnectionRef: "wecom-app",
			IdentityKind:  facade.ExternalIdentityWeComUserID,
			Subject:       "external-member-7",
		}},
	}
	if _, err := service.Publish(context.Background(), request); err != nil {
		t.Fatalf("publish combined notification: %v", err)
	}
	if repo.inboxRecipients != 1 {
		t.Fatalf("platform audience recipients=%d, want one", repo.inboxRecipients)
	}
	// Change the live default after acceptance. Dispatch must use the immutable
	// snapshot rather than resolving the changed setting again.
	repo.channels["wecom-app"].MetadataJSON = `{"providerParameterSettings":[{"key":"mentionedList","enabled":true,"defaultValue":["changed-user"]}]}`
	duplicate, err := service.Publish(context.Background(), request)
	if err != nil || !duplicate.Duplicate {
		t.Fatalf("idempotent publish after default change receipt=%#v err=%v", duplicate, err)
	}
	if len(repo.notifications) != 1 || len(repo.externalTargets) != 1 || len(repo.deliveries) != 1 {
		t.Fatalf("default change created new semantic delivery notifications=%d targets=%d deliveries=%d", len(repo.notifications), len(repo.externalTargets), len(repo.deliveries))
	}
	var deliveryID string
	for id := range repo.deliveries {
		deliveryID = id
	}
	if err := service.dispatch(context.Background(), deliveryID); err == nil || !isDeliveryAsyncHandled(err) {
		t.Fatalf("permanent external failure err=%v, want handled persisted failure", err)
	}
	if len(driver.messages) != 1 {
		t.Fatalf("driver calls=%d, want one", len(driver.messages))
	}
	mentionedJSON, err := json.Marshal(driver.messages[0].ProviderParams[domain.ProviderParameterMentionedList])
	if err != nil {
		t.Fatalf("marshal mentioned snapshot: %v", err)
	}
	var mentioned []string
	if err := json.Unmarshal(mentionedJSON, &mentioned); err != nil || len(mentioned) != 1 || mentioned[0] != "default-user" {
		t.Fatalf("provider parameters=%#v, want immutable default snapshot", driver.messages[0].ProviderParams)
	}
	delivery := repo.deliveries[deliveryID]
	if delivery.Status != domain.DeliveryStatusFailed || len(repo.attempts) != 1 || repo.attempts[0].FailureClass != "INVALID_TARGET" {
		t.Fatalf("external failure state delivery=%#v attempts=%#v", delivery, repo.attempts)
	}
	if repo.inboxRecipients != 1 || repo.inboxMutations != 0 {
		t.Fatalf("external failure changed inbox state recipients=%d mutations=%d", repo.inboxRecipients, repo.inboxMutations)
	}
}

func TestExternalUncertainOutcomeIsTerminalWithoutRawTargetEvidence(t *testing.T) {
	repo := newExternalTestRepository()
	repo.channels["feishu-app"] = enterpriseTestChannel(t, domain.ChannelTypeFeishuApp, "feishu-app", `{"appId":"cli_test"}`, "{}")
	driver := &externalResultDriver{result: DriverResult{Status: DriverResultUnknown, FailureClass: "AMBIGUOUS", Diagnostic: "PROVIDER_REQUEST_UNCERTAIN"}}
	service := newExternalTestService(t, repo, driverRegistryFunc(func(string) ChannelDriver { return driver }))
	if _, err := service.Publish(context.Background(), facade.PublishRequest{
		EventKey:       "system.check",
		IdempotencyKey: "unknown-1",
		Category:       "SYSTEM",
		Priority:       "NORMAL",
		Title:          "系统提醒",
		Content:        "外部平台请求结果不确定。",
		ExternalRecipients: []facade.ExternalRecipient{{
			ConnectionRef: "feishu-app",
			IdentityKind:  facade.ExternalIdentityFeishuOpenID,
			Subject:       "ou_sensitive_target",
		}},
	}); err != nil {
		t.Fatalf("publish unknown test: %v", err)
	}
	var deliveryID string
	for id := range repo.deliveries {
		deliveryID = id
	}
	if err := service.dispatch(context.Background(), deliveryID); err != nil {
		t.Fatalf("unknown external dispatch: %v", err)
	}
	if repo.deliveries[deliveryID].Status != domain.DeliveryStatusUnknown || len(repo.outbox) != 2 {
		t.Fatalf("unknown state delivery=%#v outbox=%#v", repo.deliveries[deliveryID], repo.outbox)
	}
	if len(repo.attempts) != 1 || strings.Contains(fmt.Sprintf("%#v", repo.attempts[0]), "ou_sensitive_target") {
		t.Fatalf("attempt leaked target or missing: %#v", repo.attempts)
	}
}

func TestFeishuRetryStopsAtUUIDDeduplicationWindow(t *testing.T) {
	repo := newExternalTestRepository()
	repo.channels["feishu-app"] = enterpriseTestChannel(t, domain.ChannelTypeFeishuApp, "feishu-app", `{"appId":"cli_test"}`, "{}")
	driver := &externalResultDriver{result: DriverResult{Status: DriverResultFailed, FailureClass: "TRANSIENT", Diagnostic: "FEISHU_REQUEST_UNCONFIRMED", Retryable: true}}
	service := newExternalTestService(t, repo, driverRegistryFunc(func(string) ChannelDriver { return driver }))
	if _, err := service.Publish(context.Background(), facade.PublishRequest{
		EventKey:       "system.check",
		IdempotencyKey: "feishu-window-1",
		Category:       "SYSTEM",
		Priority:       "NORMAL",
		Title:          "系统提醒",
		Content:        "该正文不能在飞书应用消息里完整复制。",
		ExternalRecipients: []facade.ExternalRecipient{{
			ConnectionRef: "feishu-app",
			IdentityKind:  facade.ExternalIdentityFeishuOpenID,
			Subject:       "ou_target",
		}},
	}); err != nil {
		t.Fatalf("publish Feishu window test: %v", err)
	}
	var deliveryID string
	for id := range repo.deliveries {
		deliveryID = id
	}
	repo.deliveries[deliveryID].CreateTime = service.now().Add(-feishuUUIDDeduplicationTTL)
	if err := service.dispatch(context.Background(), deliveryID); err != nil {
		t.Fatalf("expired Feishu retry must become terminal UNKNOWN, err=%v", err)
	}
	delivery := repo.deliveries[deliveryID]
	if delivery.Status != domain.DeliveryStatusUnknown || delivery.LastError != "FEISHU_UUID_WINDOW_EXPIRED" {
		t.Fatalf("expired Feishu retry state=%#v", delivery)
	}
	if len(repo.attempts) != 1 || countOutboxEvent(repo.outbox, domain.OutboxEventNotificationDispatch) != 1 {
		t.Fatalf("expired Feishu retry must not enqueue a second send, attempts=%#v outbox=%#v", repo.attempts, repo.outbox)
	}
	if len(driver.messages) != 1 || strings.Contains(driver.messages[0].Text, "该正文不能在飞书应用消息里完整复制") {
		t.Fatalf("Feishu outbound text leaked the inbox body: %#v", driver.messages)
	}
}

func TestUpsertEnterpriseApplicationUsesStructuredConfigAndDescriptorSettings(t *testing.T) {
	repo := newExternalTestRepository()
	service := newExternalTestService(t, repo, nil)
	service.urls = allowAllChannelURLs{}
	record, err := service.UpsertChannel(context.Background(), facade.ChannelUpsertRequest{
		ChannelCode:  "wecom-app",
		ChannelName:  "企业微信应用",
		ChannelType:  domain.ChannelTypeWeComApp,
		Status:       domain.ChannelStatusEnabled,
		ConfigJSON:   `{this raw config must not be parsed}`,
		MetadataJSON: `{this raw metadata must not be parsed}`,
		SecretPlain:  "corp-secret",
		ProviderConfig: &facade.ProviderChannelConfig{
			WeComCorpID:  "ww_test",
			WeComAgentID: "100001",
		},
		ProviderParameterSettings: []facade.ProviderParameterSetting{{
			Key:          domain.ProviderParameterMentionedList,
			Enabled:      true,
			DefaultValue: []string{"default-user"},
		}},
	}, 1)
	if err != nil {
		t.Fatalf("upsert structured enterprise channel: %v", err)
	}
	if record.ProviderConfig == nil || record.ProviderConfig.WeComCorpID != "ww_test" || record.ProviderConfig.WeComAgentID != "100001" || len(record.ProviderParameterCatalog) != 1 || len(record.ProviderParameterSettings) != 1 || record.ConfigJSON != "" || record.MetadataJSON != "" {
		t.Fatalf("structured channel record=%#v", record)
	}
	saved := repo.channels["wecom-app"]
	if saved == nil || saved.ScopeID != "local" || strings.Contains(saved.ConfigJSON, "this raw") || strings.Contains(saved.MetadataJSON, "this raw") || !strings.Contains(saved.MetadataJSON, "providerParameterSettings") {
		t.Fatalf("structured channel persistence=%#v", saved)
	}
	_, err = service.UpsertChannel(context.Background(), facade.ChannelUpsertRequest{
		ChannelCode: "wecom-invalid-param",
		ChannelName: "企业微信应用",
		ChannelType: domain.ChannelTypeWeComApp,
		Status:      domain.ChannelStatusDisabled,
		ProviderConfig: &facade.ProviderChannelConfig{
			WeComCorpID:  "ww_test",
			WeComAgentID: "100001",
		},
		ProviderParameterSettings: []facade.ProviderParameterSetting{{Key: "url", Enabled: true, DefaultValue: "https://forbidden"}},
	}, 1)
	if err == nil || repo.channels["wecom-invalid-param"] != nil {
		t.Fatalf("undeclared provider parameter err=%v saved=%#v", err, repo.channels["wecom-invalid-param"])
	}
}

func TestEnterpriseConnectionProbeDoesNotPersistThirdPartyTarget(t *testing.T) {
	repo := newExternalTestRepository()
	repo.channels["feishu-app"] = enterpriseTestChannel(t, domain.ChannelTypeFeishuApp, "feishu-app", `{"appId":"cli_test"}`, "{}")
	driver := &externalResultDriver{result: DriverResult{Status: DriverResultProviderAccepted, ProviderReference: "om_probe"}}
	service := newExternalTestService(t, repo, driverRegistryFunc(func(string) ChannelDriver { return driver }))
	result, err := service.TestEnterpriseConnection(context.Background(), facade.EnterpriseConnectionTestRequest{
		ConnectionRef: "feishu-app",
		IdentityKind:  facade.ExternalIdentityFeishuOpenID,
		Subject:       "ou_probe_target",
		Text:          "请确认连接",
	})
	if err != nil || result.Status != DriverResultProviderAccepted || result.ProviderReference != "om_probe" {
		t.Fatalf("probe result=%#v err=%v", result, err)
	}
	if len(repo.externalTargets) != 0 || len(repo.deliveries) != 0 || len(repo.notifications) != 0 || len(repo.outbox) != 0 {
		t.Fatalf("probe persisted production state targets=%d deliveries=%d notifications=%d outbox=%d", len(repo.externalTargets), len(repo.deliveries), len(repo.notifications), len(repo.outbox))
	}
}

func TestEnterpriseConnectionProbeSupportsFeishuGroupWithoutPersistence(t *testing.T) {
	repo := newExternalTestRepository()
	repo.channels["feishu-app"] = enterpriseTestChannel(t, domain.ChannelTypeFeishuApp, "feishu-app", `{"appId":"cli_test"}`, "{}")
	driver := &externalResultDriver{result: DriverResult{Status: DriverResultProviderAccepted, ProviderReference: "om_group_probe"}}
	service := newExternalTestService(t, repo, driverRegistryFunc(func(string) ChannelDriver { return driver }))
	result, err := service.TestEnterpriseConnection(context.Background(), facade.EnterpriseConnectionTestRequest{
		ConnectionRef: "feishu-app",
		IdentityKind:  facade.ExternalIdentityFeishuChatID,
		Subject:       "oc_group_probe",
		Text:          "请确认群聊连接",
	})
	if err != nil || result.Status != DriverResultProviderAccepted || len(driver.messages) != 1 || driver.messages[0].IdentityKind != domain.ExternalIdentityFeishuChatID {
		t.Fatalf("group probe result=%#v err=%v messages=%#v", result, err, driver.messages)
	}
	if len(repo.externalTargets) != 0 || len(repo.deliveries) != 0 || len(repo.notifications) != 0 || len(repo.outbox) != 0 {
		t.Fatalf("group probe persisted production state targets=%d deliveries=%d notifications=%d outbox=%d", len(repo.externalTargets), len(repo.deliveries), len(repo.notifications), len(repo.outbox))
	}
}

func TestEnterpriseConnectionProbeLeasesCredentialForOrganizationGroupChannel(t *testing.T) {
	repo := newExternalTestRepository()
	organizationID, connectionID := int64(101), int64(301)
	channel := enterpriseTestChannel(t, domain.ChannelTypeFeishuApp, "group-route-401", `{}`, `{}`)
	channel.ScopeType = "ORGANIZATION"
	channel.OrganizationID = &organizationID
	channel.EnterpriseConnectionID = &connectionID
	channel.EnterpriseConnectionVersion = 1
	channel.EnterpriseConnectionDigest = "connection-digest"
	channel.SecretCiphertext, channel.SecretEDEK, channel.SecretWrapKeyRef = "", "", ""
	repo.channels[channel.ChannelCode] = channel
	driver := &externalResultDriver{result: DriverResult{Status: DriverResultProviderAccepted, ProviderReference: "om_group_probe"}}
	service := newExternalTestService(t, repo, driverRegistryFunc(func(string) ChannelDriver { return driver }))
	service.enterpriseIdentities = enterpriseIdentityResolverStub{}

	result, err := service.TestEnterpriseConnection(context.Background(), facade.EnterpriseConnectionTestRequest{
		ConnectionRef: channel.ChannelCode,
		IdentityKind:  facade.ExternalIdentityFeishuChatID,
		Subject:       "oc_group_probe",
		Text:          "请确认群聊连接",
	})
	if err != nil || result == nil || result.Status != DriverResultProviderAccepted || len(driver.messages) != 1 {
		t.Fatalf("organization group probe result=%#v err=%v messages=%#v", result, err, driver.messages)
	}
	message := driver.messages[0]
	if message.SecretPlain != "secret" || !strings.Contains(message.Channel.ConfigJSON, `"appId":"app"`) {
		t.Fatalf("organization group probe did not use leased credential: %#v", message)
	}
	if len(repo.externalTargets) != 0 || len(repo.deliveries) != 0 || len(repo.notifications) != 0 || len(repo.outbox) != 0 {
		t.Fatalf("organization group probe persisted production state targets=%d deliveries=%d notifications=%d outbox=%d", len(repo.externalTargets), len(repo.deliveries), len(repo.notifications), len(repo.outbox))
	}
}

func TestEnterpriseConnectionProbeReturnsSanitizedProviderErrorWithoutPersistence(t *testing.T) {
	repo := newExternalTestRepository()
	repo.channels["feishu-app"] = enterpriseTestChannel(t, domain.ChannelTypeFeishuApp, "feishu-app", `{"appId":"cli_test"}`, "{}")
	driver := &externalResultDriver{result: DriverResult{
		Status:       DriverResultFailed,
		FailureClass: "INVALID_TARGET",
		Diagnostic:   "INVALID_TARGET",
		ProviderError: &ProviderError{
			Provider:   domain.ChannelTypeFeishuApp,
			HTTPStatus: 400,
			Code:       "230001",
			Message:    "The application bot is not in this chat.",
			LogID:      "feishu-log-123",
		},
	}}
	service := newExternalTestService(t, repo, driverRegistryFunc(func(string) ChannelDriver { return driver }))
	result, err := service.TestEnterpriseConnection(context.Background(), facade.EnterpriseConnectionTestRequest{
		ConnectionRef: "feishu-app",
		IdentityKind:  facade.ExternalIdentityFeishuChatID,
		Subject:       "oc_group_probe",
		Text:          "请确认群聊连接",
	})
	if err != nil || result == nil || result.ProviderError == nil {
		t.Fatalf("probe result=%#v err=%v, want source provider error", result, err)
	}
	if result.ProviderError.Provider != domain.ChannelTypeFeishuApp || result.ProviderError.HTTPStatus != 400 || result.ProviderError.Code != "230001" || result.ProviderError.Message != "The application bot is not in this chat." || result.ProviderError.LogID != "feishu-log-123" {
		t.Fatalf("provider error=%#v", result.ProviderError)
	}
	if len(repo.externalTargets) != 0 || len(repo.deliveries) != 0 || len(repo.notifications) != 0 || len(repo.outbox) != 0 {
		t.Fatalf("probe must not persist source error targets=%d deliveries=%d notifications=%d outbox=%d", len(repo.externalTargets), len(repo.deliveries), len(repo.notifications), len(repo.outbox))
	}
}

func TestEnterpriseConnectionProbeFailureLogExcludesTargetAndSecret(t *testing.T) {
	repo := newExternalTestRepository()
	repo.channels["wecom-app"] = enterpriseTestChannel(t, domain.ChannelTypeWeComApp, "wecom-app", `{"corpId":"ww_test","agentId":"100001"}`, "{}")
	driver := &externalResultDriver{result: DriverResult{
		Status:       DriverResultFailed,
		FailureClass: "INVALID_TARGET",
		Diagnostic:   "INVALID_TARGET",
		ProviderError: &ProviderError{
			Provider:   domain.ChannelTypeWeComApp,
			HTTPStatus: 200,
			Code:       "81013",
			Message:    "invalid user",
		},
	}}
	service := newExternalTestService(t, repo, driverRegistryFunc(func(string) ChannelDriver { return driver }))
	core, observed := observer.New(zap.WarnLevel)
	service.SetLogger(zap.New(core))

	result, err := service.TestEnterpriseConnection(context.Background(), facade.EnterpriseConnectionTestRequest{
		ConnectionRef: "wecom-app",
		IdentityKind:  facade.ExternalIdentityWeComUserID,
		Subject:       "member-private-value",
		Text:          "连接测试",
	})
	if err != nil || result == nil || result.FailureClass != "INVALID_TARGET" {
		t.Fatalf("probe result=%#v err=%v", result, err)
	}
	entries := observed.FilterMessage("notification_enterprise_connection_probe_failed").All()
	if len(entries) != 1 {
		t.Fatalf("probe failure logs=%d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["provider"] != domain.ChannelTypeWeComApp || fields["failureClass"] != "INVALID_TARGET" || fields["providerCode"] != "81013" {
		t.Fatalf("unexpected probe failure log=%#v", fields)
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshal probe failure log: %v", err)
	}
	for _, forbidden := range []string{"member-private-value", "test-secret", "wecom-app"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("probe failure log leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestSafeProviderErrorMessageRedactsProviderNetworkAddress(t *testing.T) {
	message := safeProviderErrorMessage("not allow to access from your ip, hint: [request-hint], from ip: 203.0.113.10, more info at https://provider.example/error")
	if strings.Contains(message, "203.0.113.10") {
		t.Fatalf("provider message leaked network address: %q", message)
	}
	if !strings.Contains(message, "from ip: [redacted]") || !strings.Contains(message, "[provider-url]") {
		t.Fatalf("provider message=%q, want redacted network address and URL", message)
	}
}

type externalTestRepository struct {
	domain.Repository
	channels                     map[string]*domain.Channel
	templates                    map[string]*domain.Template
	sceneBindings                map[string]*domain.SceneBinding
	notifications                map[string]*domain.LogicalNotification
	notificationsByKey           map[string]*domain.LogicalNotification
	externalTargets              map[int64]*domain.ExternalTarget
	httpSnapshots                map[string]*domain.HTTPDeliverySnapshot
	deliveries                   map[string]*domain.Delivery
	attempts                     []domain.DeliveryAttempt
	outbox                       []domain.OutboxEvent
	inboxRecipientWrites         int
	inboxRecipients              int
	inboxMutations               int
	ephemeralContents            map[string]*domain.DeliveryEphemeralContent
	diagnosticAudits             []domain.DeliveryDiagnosticAudit
	findChannelCalls             int
	listChannelCalls             int
	insertDeliveryCalls          int
	insertDeliveryBatchCalls     int
	insertHTTPSnapshotCalls      int
	insertHTTPSnapshotBatchCalls int
	appendOutboxCalls            int
	appendOutboxBatchCalls       int
}

func newExternalTestRepository() *externalTestRepository {
	return &externalTestRepository{
		channels:           make(map[string]*domain.Channel),
		templates:          make(map[string]*domain.Template),
		sceneBindings:      make(map[string]*domain.SceneBinding),
		notifications:      make(map[string]*domain.LogicalNotification),
		notificationsByKey: make(map[string]*domain.LogicalNotification),
		externalTargets:    make(map[int64]*domain.ExternalTarget),
		httpSnapshots:      make(map[string]*domain.HTTPDeliverySnapshot),
		deliveries:         make(map[string]*domain.Delivery),
		ephemeralContents:  make(map[string]*domain.DeliveryEphemeralContent),
	}
}

func (r *externalTestRepository) FindChannelByCode(_ context.Context, code string) (*domain.Channel, error) {
	r.findChannelCalls++
	if item := r.channels[code]; item != nil {
		copy := *item
		return &copy, nil
	}
	return nil, nil
}

func (r *externalTestRepository) ListChannelsByCodes(_ context.Context, codes []string) ([]domain.Channel, error) {
	r.listChannelCalls++
	result := make([]domain.Channel, 0, len(codes))
	for _, code := range codes {
		if item := r.channels[code]; item != nil {
			result = append(result, *item)
		}
	}
	return result, nil
}

func (r *externalTestRepository) UpsertChannel(_ context.Context, item *domain.Channel) error {
	copy := *item
	r.channels[item.ChannelCode] = &copy
	return nil
}

func (r *externalTestRepository) FindTemplateByCode(_ context.Context, code string) (*domain.Template, error) {
	if item := r.templates[code]; item != nil {
		copy := *item
		return &copy, nil
	}
	return nil, nil
}

func (r *externalTestRepository) FindActiveSceneBinding(_ context.Context, scopeID, sceneCode string) (*domain.SceneBinding, error) {
	item := r.sceneBindings[scopeID+"\x00"+sceneCode]
	if item == nil || !item.Enabled {
		return nil, nil
	}
	copy := *item
	return &copy, nil
}

func (r *externalTestRepository) FindLogicalNotificationByIdempotency(_ context.Context, scopeID, eventKey, idempotencyKey string) (*domain.LogicalNotification, error) {
	if item := r.notificationsByKey[scopeID+"\x00"+eventKey+"\x00"+idempotencyKey]; item != nil {
		copy := *item
		return &copy, nil
	}
	return nil, nil
}

func (r *externalTestRepository) FindLogicalNotificationByID(_ context.Context, notificationID int64) (*domain.LogicalNotification, error) {
	for _, item := range r.notifications {
		if item.ID == notificationID {
			copy := *item
			return &copy, nil
		}
	}
	return nil, nil
}

func (r *externalTestRepository) CreateLogicalNotification(_ context.Context, item *domain.LogicalNotification) (bool, error) {
	key := item.ScopeID + "\x00" + item.EventKey + "\x00" + item.IdempotencyKey
	if r.notificationsByKey[key] != nil {
		return false, nil
	}
	copy := *item
	r.notifications[item.NotificationID] = &copy
	r.notificationsByKey[key] = &copy
	return true, nil
}

func (r *externalTestRepository) InsertInboxRecipients(_ context.Context, items []domain.Recipient) ([]domain.Recipient, error) {
	r.inboxRecipientWrites++
	r.inboxRecipients += len(items)
	created := make([]domain.Recipient, 0, len(items))
	for _, item := range items {
		item.MailboxVersion = int64(r.inboxRecipients)
		created = append(created, item)
	}
	return created, nil
}

func (r *externalTestRepository) InsertExternalTargets(_ context.Context, items []domain.ExternalTarget) error {
	for _, item := range items {
		copy := item
		r.externalTargets[item.ID] = &copy
	}
	return nil
}

func (r *externalTestRepository) FindExternalTargetByID(_ context.Context, id int64) (*domain.ExternalTarget, error) {
	if item := r.externalTargets[id]; item != nil {
		copy := *item
		return &copy, nil
	}
	return nil, nil
}

func (r *externalTestRepository) ListExternalTargetsByNotificationID(_ context.Context, notificationID int64) ([]domain.ExternalTarget, error) {
	items := make([]domain.ExternalTarget, 0)
	for _, item := range r.externalTargets {
		if item.NotificationID == notificationID {
			items = append(items, *item)
		}
	}
	return items, nil
}

func (r *externalTestRepository) InsertDelivery(_ context.Context, item *domain.Delivery) error {
	r.insertDeliveryCalls++
	copy := *item
	r.deliveries[item.DeliveryID] = &copy
	return nil
}

func (r *externalTestRepository) InsertDeliveries(_ context.Context, items []domain.Delivery) error {
	r.insertDeliveryBatchCalls++
	for _, item := range items {
		copy := item
		r.deliveries[item.DeliveryID] = &copy
	}
	return nil
}

func (r *externalTestRepository) FindDeliveryByID(_ context.Context, deliveryID string) (*domain.Delivery, error) {
	if item := r.deliveries[deliveryID]; item != nil {
		copy := *item
		return &copy, nil
	}
	return nil, nil
}

func (r *externalTestRepository) ListDeliverySummaries(_ context.Context, query domain.DeliveryQuery) ([]domain.DeliverySummary, int64, error) {
	items := make([]domain.DeliverySummary, 0, len(r.deliveries))
	for _, item := range r.deliveries {
		if query.ScopeID != "" && query.ScopeID != "local" {
			continue
		}
		items = append(items, domain.DeliverySummary{
			ID: item.ID, DeliveryID: item.DeliveryID, SceneCode: item.SceneCode, ChannelCode: item.ChannelCode,
			ChannelType: item.ChannelType, TemplateCode: item.TemplateCode, TargetMasked: item.TargetMasked,
			Status: item.Status, RetryCount: item.RetryCount, MaxRetry: item.MaxRetry, NextRetryAt: item.NextRetryAt,
			LastError: item.LastError, TraceID: item.TraceID, SentAt: item.SentAt,
			ContentTier: item.ContentTier, CreateTime: item.CreateTime, UpdateTime: item.UpdateTime,
		})
	}
	return items, int64(len(items)), nil
}

func (r *externalTestRepository) FindDeliveryForDiagnostic(_ context.Context, scopeID, deliveryID string) (*domain.Delivery, error) {
	if scopeID != "local" {
		return nil, nil
	}
	return r.FindDeliveryByID(context.Background(), deliveryID)
}

func (r *externalTestRepository) FindDeliveryEphemeralContent(_ context.Context, scopeID, deliveryID string) (*domain.DeliveryEphemeralContent, error) {
	if scopeID != "local" {
		return nil, nil
	}
	if item := r.ephemeralContents[deliveryID]; item != nil {
		copy := *item
		return &copy, nil
	}
	return nil, nil
}

func (r *externalTestRepository) InsertDeliveryEphemeralContent(_ context.Context, item *domain.DeliveryEphemeralContent) error {
	if item == nil || r.ephemeralContents[item.DeliveryID] != nil {
		return fmt.Errorf("duplicate ephemeral content")
	}
	copy := *item
	r.ephemeralContents[item.DeliveryID] = &copy
	return nil
}

func (r *externalTestRepository) InsertDeliveryDiagnosticAudit(_ context.Context, item *domain.DeliveryDiagnosticAudit) error {
	if item == nil {
		return fmt.Errorf("diagnostic audit is nil")
	}
	r.diagnosticAudits = append(r.diagnosticAudits, *item)
	return nil
}

func (r *externalTestRepository) FindDeliveryByDigest(_ context.Context, digest string) (*domain.Delivery, error) {
	for _, item := range r.deliveries {
		if item.RequestDigest == digest {
			copy := *item
			return &copy, nil
		}
	}
	return nil, nil
}

func (r *externalTestRepository) ListDeliveriesByNotificationID(_ context.Context, notificationID int64) ([]domain.Delivery, error) {
	items := make([]domain.Delivery, 0)
	for _, item := range r.deliveries {
		if item.NotificationID != nil && *item.NotificationID == notificationID {
			items = append(items, *item)
		}
	}
	return items, nil
}

func (r *externalTestRepository) InsertHTTPDeliverySnapshot(_ context.Context, item *domain.HTTPDeliverySnapshot) error {
	r.insertHTTPSnapshotCalls++
	copy := *item
	r.httpSnapshots[item.DeliveryID] = &copy
	return nil
}

func (r *externalTestRepository) InsertHTTPDeliverySnapshots(_ context.Context, items []domain.HTTPDeliverySnapshot) error {
	r.insertHTTPSnapshotBatchCalls++
	for _, item := range items {
		copy := item
		r.httpSnapshots[item.DeliveryID] = &copy
	}
	return nil
}

func (r *externalTestRepository) FindHTTPDeliverySnapshotByDeliveryID(_ context.Context, deliveryID string) (*domain.HTTPDeliverySnapshot, error) {
	if item := r.httpSnapshots[deliveryID]; item != nil {
		copy := *item
		return &copy, nil
	}
	return nil, nil
}

func (r *externalTestRepository) MarkDeliverySending(_ context.Context, deliveryID string) (bool, error) {
	item := r.deliveries[deliveryID]
	if item == nil || item.Status != domain.DeliveryStatusPending {
		return false, nil
	}
	item.Status = domain.DeliveryStatusSending
	return true, nil
}

func (r *externalTestRepository) MarkDeliveryProviderAccepted(_ context.Context, deliveryID, reference string, at time.Time) error {
	item := r.deliveries[deliveryID]
	item.Status = domain.DeliveryStatusProviderAccepted
	item.ProviderReference = reference
	item.SentAt = &at
	return nil
}

func (r *externalTestRepository) MarkDeliveryUnknown(_ context.Context, deliveryID, diagnostic string) error {
	item := r.deliveries[deliveryID]
	item.Status = domain.DeliveryStatusUnknown
	item.LastError = diagnostic
	return nil
}

func (r *externalTestRepository) MarkDeliveryFailed(_ context.Context, deliveryID string, retryCount int, diagnostic string) error {
	item := r.deliveries[deliveryID]
	item.Status = domain.DeliveryStatusFailed
	item.RetryCount = retryCount
	item.LastError = diagnostic
	return nil
}

func (r *externalTestRepository) MarkDeliveryRetry(_ context.Context, deliveryID string, retryCount int, next time.Time, diagnostic string) error {
	item := r.deliveries[deliveryID]
	item.Status = domain.DeliveryStatusPending
	item.RetryCount = retryCount
	item.NextRetryAt = &next
	item.LastError = diagnostic
	return nil
}

func (r *externalTestRepository) InsertDeliveryAttempt(_ context.Context, item *domain.DeliveryAttempt) error {
	r.attempts = append(r.attempts, *item)
	return nil
}

func (r *externalTestRepository) AppendOutbox(_ context.Context, event *domain.OutboxEvent) error {
	r.appendOutboxCalls++
	r.outbox = append(r.outbox, *event)
	return nil
}

func (r *externalTestRepository) AppendOutboxBatch(_ context.Context, events []domain.OutboxEvent) error {
	r.appendOutboxBatchCalls++
	r.outbox = append(r.outbox, events...)
	return nil
}

type externalResultDriver struct {
	result   DriverResult
	messages []DriverMessage
}

func (d *externalResultDriver) Send(ctx context.Context, message DriverMessage) error {
	_, err := d.SendResult(ctx, message)
	return err
}

func (d *externalResultDriver) SendResult(_ context.Context, message DriverMessage) (DriverResult, error) {
	d.messages = append(d.messages, message)
	return d.result, nil
}

func newExternalTestService(t *testing.T, repo *externalTestRepository, drivers DriverRegistry) *Service {
	t.Helper()
	idGen, err := xid.New(23)
	if err != nil {
		t.Fatalf("new id generator: %v", err)
	}
	service := NewService(externalTestTransactor{}, repo, domain.NewService(), inboxTestSecretService{}, drivers, nil, nil, idGen)
	service.SetScopeID("local")
	service.BindExternalTargetDigester(externalTestDigester{})
	service.now = func() time.Time { return time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC) }
	return service
}

type externalTestTransactor struct{}

func (externalTestTransactor) Enabled() bool { return true }

func (externalTestTransactor) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type disabledExternalTestTransactor struct{}

func (disabledExternalTestTransactor) Enabled() bool { return false }

func (disabledExternalTestTransactor) WithinTransaction(context.Context, func(context.Context) error) error {
	return fmt.Errorf("disabled test transaction must not execute")
}

func enterpriseTestChannel(t *testing.T, channelType, code, configJSON, metadataJSON string) *domain.Channel {
	t.Helper()
	secret, err := (inboxTestSecretService{}).EncryptString(context.Background(), "test-secret")
	if err != nil {
		t.Fatalf("encrypt channel secret: %v", err)
	}
	return &domain.Channel{
		ID:               1,
		ChannelCode:      code,
		ChannelName:      code,
		ChannelType:      channelType,
		ScopeID:          "local",
		Status:           domain.ChannelStatusEnabled,
		ConfigJSON:       configJSON,
		MetadataJSON:     metadataJSON,
		SecretCiphertext: secret.CiphertextB64,
		SecretEDEK:       secret.EDEKB64,
		SecretWrapKeyRef: secret.WrapKeyRef,
	}
}

type externalTestDigester struct{}

func (externalTestDigester) Digest(_ context.Context, keyRef, scopeID, connectionRef, identityKind, subject string) (string, string, error) {
	if keyRef == "" {
		keyRef = "test-digest-kid"
	}
	digest := sha256.Sum256([]byte(strings.Join([]string{scopeID, connectionRef, identityKind, subject}, "\x00")))
	return hex.EncodeToString(digest[:]), keyRef, nil
}

func countOutboxEvent(events []domain.OutboxEvent, eventType string) int {
	count := 0
	for _, event := range events {
		if event.EventType == eventType {
			count++
		}
	}
	return count
}

type allowAllChannelURLs struct{}

func (allowAllChannelURLs) ValidateChannel(context.Context, domain.Channel) error { return nil }

func (allowAllChannelURLs) ValidateWebhookProfileEndpoint(context.Context, string, string) error {
	return nil
}

func decodeProviderParams(t *testing.T, raw string) map[string]any {
	t.Helper()
	values := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		t.Fatalf("decode provider params: %v", err)
	}
	return values
}
