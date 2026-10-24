package application

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
)

func TestPublishUsesCanonicalIdempotencyAndRollsBackWithOutboxFailure(t *testing.T) {
	repo := newInboxTestRepository()
	service := newInboxTestService(t, repo, nil)
	request := facade.PublishRequest{
		EventKey:       "billing.invoice.ready",
		IdempotencyKey: "invoice-1001",
		Audience:       facade.Audience{UserIDs: []int64{101}},
		Category:       "BILLING",
		Priority:       "NORMAL",
		Title:          "账单已生成",
		Content:        "你的账单已生成，请查看详情。",
	}

	first, err := service.Publish(context.Background(), request)
	if err != nil {
		t.Fatalf("first publish: %v", err)
	}
	second, err := service.Publish(context.Background(), request)
	if err != nil {
		t.Fatalf("second publish: %v", err)
	}
	if first.NotificationID == "" || second.NotificationID != first.NotificationID || !second.Duplicate {
		t.Fatalf("idempotent receipt mismatch: first=%#v second=%#v", first, second)
	}
	if got := len(repo.notificationsByID); got != 1 {
		t.Fatalf("logical notification count=%d, want 1", got)
	}
	if got := len(repo.recipientsByID); got != 1 {
		t.Fatalf("recipient count=%d, want 1", got)
	}
	if got := len(repo.outbox); got != 2 {
		t.Fatalf("outbox count=%d, want intent and inbox-change events", got)
	}

	conflict := request
	conflict.Title = "账单已变更"
	_, err = service.Publish(context.Background(), conflict)
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeObjectStateInvalid {
		t.Fatalf("idempotency conflict error=%v, want object-state error", err)
	} else if details, ok := appErr.Details().(map[string]string); !ok || details["reasonCode"] != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("idempotency conflict details=%#v", appErr.Details())
	}
	if got := len(repo.notificationsByID); got != 1 {
		t.Fatalf("conflict created another notification: %d", got)
	}

	repo.appendErr = errors.New("outbox unavailable")
	rollbackRequest := request
	rollbackRequest.IdempotencyKey = "invoice-1002"
	_, err = service.Publish(context.Background(), rollbackRequest)
	if err == nil {
		t.Fatal("publish unexpectedly succeeded after outbox failure")
	}
	if got := len(repo.notificationsByID); got != 1 {
		t.Fatalf("rolled-back notification count=%d, want 1", got)
	}
	if got := len(repo.recipientsByID); got != 1 {
		t.Fatalf("rolled-back recipient count=%d, want 1", got)
	}
	if got := len(repo.outbox); got != 2 {
		t.Fatalf("rolled-back outbox count=%d, want preserved intent and inbox-change events", got)
	}
}

func TestPublishIdempotencyIgnoresRetryTraceIDButRetainsFirstTrace(t *testing.T) {
	repo := newInboxTestRepository()
	service := newInboxTestService(t, repo, nil)
	request := facade.PublishRequest{
		EventKey:       "billing.invoice.ready",
		IdempotencyKey: "invoice-trace-retry",
		Audience:       facade.Audience{UserIDs: []int64{101}},
		Title:          "账单已生成",
		Content:        "请查看账单。",
		TraceID:        "trace-first",
	}
	first, err := service.Publish(context.Background(), request)
	if err != nil {
		t.Fatalf("first publish: %v", err)
	}
	retry := request
	retry.TraceID = "trace-retry"
	second, err := service.Publish(context.Background(), retry)
	if err != nil {
		t.Fatalf("trace-only retry: %v", err)
	}
	if !second.Duplicate || second.NotificationID != first.NotificationID || len(repo.notificationsByID) != 1 {
		t.Fatalf("trace-only retry first=%#v second=%#v notifications=%d", first, second, len(repo.notificationsByID))
	}
	for _, item := range repo.notificationsByID {
		if item.TraceID != "trace-first" {
			t.Fatalf("accepted notification trace=%q, want first trace", item.TraceID)
		}
	}
}

func TestInboxRecipientStateIsOrthogonalAndOwnerScoped(t *testing.T) {
	repo := newInboxTestRepository()
	service := newInboxTestService(t, repo, nil)
	receipt, err := service.Publish(context.Background(), facade.PublishRequest{
		EventKey:       "user.profile.changed",
		IdempotencyKey: "profile-42",
		Audience:       facade.Audience{UserIDs: []int64{42}},
		Category:       "ACCOUNT",
		Priority:       "NORMAL",
		Title:          "资料已更新",
		Content:        "你的个人资料已更新。",
	})
	if err != nil || receipt == nil {
		t.Fatalf("publish direct inbox message: receipt=%#v err=%v", receipt, err)
	}
	recipientID := onlyRecipientID(t, repo)

	page, err := service.ListInbox(context.Background(), 42, facade.InboxQuery{PageSize: 20})
	if err != nil || len(page.Records) != 1 || page.Records[0].RecipientID != recipientID {
		t.Fatalf("list inbox result=%#v err=%v", page, err)
	}
	if _, err := service.GetInboxRecipient(context.Background(), 99, recipientID); apperrors.From(err) == nil || apperrors.From(err).Code() != apperrors.CodeNotFound {
		t.Fatalf("cross-user recipient access err=%v, want not found", err)
	}

	seen, err := service.MutateInboxRecipient(context.Background(), 42, recipientID, domain.InboxActionSeen, facade.InboxMutationRequest{})
	if err != nil || seen.FirstSeenAt == nil || seen.ReadAt != nil || seen.ArchivedAt != nil {
		t.Fatalf("seen state=%#v err=%v", seen, err)
	}
	read, err := service.MutateInboxRecipient(context.Background(), 42, recipientID, domain.InboxActionRead, facade.InboxMutationRequest{ExpectedMailboxVersion: seen.MailboxVersion})
	if err != nil || read.ReadAt == nil || read.FirstSeenAt == nil || !read.FirstSeenAt.Equal(*seen.FirstSeenAt) {
		t.Fatalf("read state=%#v err=%v", read, err)
	}
	unread, err := service.MutateInboxRecipient(context.Background(), 42, recipientID, domain.InboxActionUnread, facade.InboxMutationRequest{ExpectedMailboxVersion: read.MailboxVersion})
	if err != nil || unread.ReadAt != nil || unread.FirstSeenAt == nil || !unread.FirstSeenAt.Equal(*seen.FirstSeenAt) {
		t.Fatalf("unread state=%#v err=%v", unread, err)
	}
	archived, err := service.MutateInboxRecipient(context.Background(), 42, recipientID, domain.InboxActionArchive, facade.InboxMutationRequest{ExpectedMailboxVersion: unread.MailboxVersion})
	if err != nil || archived.ArchivedAt == nil || archived.ReadAt != nil {
		t.Fatalf("archive state=%#v err=%v", archived, err)
	}
	count, err := service.UnreadCount(context.Background(), 42)
	if err != nil || count.Count != 0 {
		t.Fatalf("archived unread count=%#v err=%v", count, err)
	}
	restored, err := service.MutateInboxRecipient(context.Background(), 42, recipientID, domain.InboxActionRestore, facade.InboxMutationRequest{ExpectedMailboxVersion: archived.MailboxVersion})
	if err != nil || restored.ArchivedAt != nil || restored.ReadAt != nil || restored.FirstSeenAt == nil {
		t.Fatalf("restore state=%#v err=%v", restored, err)
	}
	count, err = service.UnreadCount(context.Background(), 42)
	if err != nil || count.Count != 1 {
		t.Fatalf("restored unread count=%#v err=%v", count, err)
	}

	_, err = service.MutateInboxRecipient(context.Background(), 42, recipientID, domain.InboxActionRead, facade.InboxMutationRequest{ExpectedMailboxVersion: seen.MailboxVersion})
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeObjectStateInvalid {
		t.Fatalf("stale mailbox mutation error=%v, want object-state error", err)
	} else if details, ok := appErr.Details().(map[string]string); !ok || details["reasonCode"] != "MAILBOX_VERSION_CONFLICT" {
		t.Fatalf("stale mailbox mutation details=%#v", appErr.Details())
	}
}

func TestInboxReadShapesKeepDetailPrivateUntilExplicitOpen(t *testing.T) {
	repo := newInboxTestRepository()
	service := newInboxTestService(t, repo, nil)
	const fullContent = "这是一段只能在用户明确打开详情后才能看到的完整正文。"
	const deepLink = "/account/security"
	if _, err := service.Publish(context.Background(), facade.PublishRequest{
		EventKey:       "account.security.changed",
		IdempotencyKey: "shape-boundary-42",
		Audience:       facade.Audience{UserIDs: []int64{42}},
		Category:       "ACCOUNT",
		Priority:       "NORMAL",
		Title:          "账号安全设置已更新",
		Content:        fullContent,
		DeepLink:       deepLink,
	}); err != nil {
		t.Fatalf("publish inbox message: %v", err)
	}

	count, err := service.UnreadCount(context.Background(), 42)
	if err != nil {
		t.Fatalf("read unread count: %v", err)
	}
	countJSON := marshalInboxShape(t, count)
	if _, ok := countJSON["mailboxKey"]; !ok {
		t.Fatalf("count response must expose an opaque mailbox key: %#v", countJSON)
	}
	forbiddenInboxFields(t, countJSON, fullContent, deepLink)

	page, err := service.ListInbox(context.Background(), 42, facade.InboxQuery{PageSize: 20})
	if err != nil || len(page.Records) != 1 {
		t.Fatalf("list inbox page=%#v err=%v", page, err)
	}
	listJSON := marshalInboxShape(t, page)
	forbiddenInboxFields(t, listJSON, fullContent, deepLink)
	records, ok := listJSON["records"].([]any)
	if !ok || len(records) != 1 {
		t.Fatalf("list records=%#v, want one compact record", listJSON["records"])
	}
	record, ok := records[0].(map[string]any)
	if !ok || strings.TrimSpace(fmt.Sprint(record["summary"])) == "" || fmt.Sprint(record["summary"]) != "打开查看详情" {
		t.Fatalf("list record=%#v, want non-body reading cue", record)
	}

	detail, err := service.GetInboxRecipient(context.Background(), 42, page.Records[0].RecipientID)
	if err != nil {
		t.Fatalf("read explicit inbox detail: %v", err)
	}
	if detail.Content != fullContent || detail.DeepLink != deepLink {
		t.Fatalf("detail=%#v, want full detail only after explicit open", detail)
	}
}

func TestInboxPreviewAndDeltaKeepContentPrivateAndTokensOwnerBound(t *testing.T) {
	repo := newInboxTestRepository()
	service := newInboxTestService(t, repo, nil)
	const fullContent = "这段完整正文不得进入铃铛预览、列表或增量同步响应。"
	for index := 1; index <= 2; index++ {
		if _, err := service.Publish(context.Background(), facade.PublishRequest{
			EventKey:       fmt.Sprintf("account.notice.%d", index),
			IdempotencyKey: fmt.Sprintf("preview-delta-%d", index),
			Audience:       facade.Audience{UserIDs: []int64{42}},
			Category:       "ACCOUNT",
			Priority:       "NORMAL",
			Title:          fmt.Sprintf("账号提醒 %d", index),
			Content:        fullContent,
			DeepLink:       "/account/security",
		}); err != nil {
			t.Fatalf("publish inbox message %d: %v", index, err)
		}
	}

	preview, err := service.UnreadPreview(context.Background(), 42, 99)
	if err != nil || len(preview.Records) != 2 || preview.MailboxKey == "" || preview.ChangeToken == "" {
		t.Fatalf("unread preview=%#v err=%v", preview, err)
	}
	for _, item := range preview.Records {
		if item.Summary != "打开查看详情" {
			t.Fatalf("preview summary=%q, want non-body reading cue", item.Summary)
		}
	}
	forbiddenInboxFields(t, marshalInboxShape(t, preview), fullContent, "/account/security")

	page, err := service.ListInbox(context.Background(), 42, facade.InboxQuery{PageSize: 20})
	if err != nil || len(page.Records) != 2 {
		t.Fatalf("initial inbox page=%#v err=%v", page, err)
	}
	changed, err := service.MutateInboxRecipient(context.Background(), 42, page.Records[0].RecipientID, domain.InboxActionRead, facade.InboxMutationRequest{ExpectedMailboxVersion: page.Records[0].MailboxVersion})
	if err != nil || changed.ReadAt == nil {
		t.Fatalf("mark one recipient read: %#v err=%v", changed, err)
	}

	delta, err := service.ListInboxChanges(context.Background(), 42, facade.InboxChangeQuery{AfterChangeToken: page.ChangeToken, Limit: 1})
	if err != nil || delta.ResyncRequired || len(delta.Upserts) != 1 || delta.Upserts[0].RecipientID != changed.RecipientID || delta.UnreadCount != 1 || delta.TargetChangeToken == "" {
		t.Fatalf("inbox delta=%#v err=%v", delta, err)
	}
	forbiddenInboxFields(t, marshalInboxShape(t, delta), fullContent, "/account/security")
	if delta.Upserts[0].Summary != "打开查看详情" {
		t.Fatalf("delta summary=%q, want non-body reading cue", delta.Upserts[0].Summary)
	}

	foreign, err := service.ListInboxChanges(context.Background(), 43, facade.InboxChangeQuery{AfterChangeToken: page.ChangeToken})
	if err != nil || !foreign.ResyncRequired || len(foreign.Upserts) != 0 || foreign.MailboxKey != "" {
		t.Fatalf("foreign token result=%#v err=%v", foreign, err)
	}
}

func TestInboxDetailSanitizesNonRenderingControlCharacters(t *testing.T) {
	detail := mapInboxDetail(domain.Recipient{
		RecipientID: "nrc_detail_controls",
		Title:       " 测试\x00消息 \n",
		Content:     " 第一行\x00\n第二行\x1f\t保留制表符 ",
		DeepLink:    "https://attacker.example/redirect",
	})
	if detail.Title != "测试消息" {
		t.Fatalf("sanitized detail title=%q", detail.Title)
	}
	if detail.Content != "第一行\n第二行\t保留制表符" {
		t.Fatalf("sanitized detail content=%q", detail.Content)
	}
	if detail.DeepLink != "" {
		t.Fatalf("unsafe legacy deep link=%q", detail.DeepLink)
	}
}

func TestInboxDeltaKeepsOneTargetAcrossPagesAndExpiredTokenResyncs(t *testing.T) {
	repo := newInboxTestRepository()
	service := newInboxTestService(t, repo, nil)
	publish := func(key string) {
		t.Helper()
		if _, err := service.Publish(context.Background(), facade.PublishRequest{
			EventKey:       "account.notice",
			IdempotencyKey: key,
			Audience:       facade.Audience{UserIDs: []int64{42}},
			Category:       "ACCOUNT",
			Priority:       "NORMAL",
			Title:          "账号提醒",
			Content:        "这是一段不能出现在增量响应中的完整正文。",
		}); err != nil {
			t.Fatalf("publish %s: %v", key, err)
		}
	}

	publish("delta-base")
	base, err := service.ListInbox(context.Background(), 42, facade.InboxQuery{PageSize: 20})
	if err != nil || base.ChangeToken == "" {
		t.Fatalf("base inbox page=%#v err=%v", base, err)
	}
	publish("delta-1")
	publish("delta-2")
	publish("delta-3")

	first, err := service.ListInboxChanges(context.Background(), 42, facade.InboxChangeQuery{AfterChangeToken: base.ChangeToken, Limit: 1})
	if err != nil || first.ResyncRequired || !first.HasMore || len(first.Upserts) != 1 || first.NextChangeToken == "" || first.TargetChangeToken == "" {
		t.Fatalf("first delta=%#v err=%v", first, err)
	}
	second, err := service.ListInboxChanges(context.Background(), 42, facade.InboxChangeQuery{
		AfterChangeToken: first.NextChangeToken,
		UntilChangeToken: first.TargetChangeToken,
		Limit:            1,
	})
	if err != nil || second.ResyncRequired || !second.HasMore || len(second.Upserts) != 1 || second.TargetChangeToken != first.TargetChangeToken {
		t.Fatalf("second delta=%#v err=%v", second, err)
	}
	last, err := service.ListInboxChanges(context.Background(), 42, facade.InboxChangeQuery{
		AfterChangeToken: second.NextChangeToken,
		UntilChangeToken: first.TargetChangeToken,
		Limit:            1,
	})
	if err != nil || last.ResyncRequired || last.HasMore || len(last.Upserts) != 1 || last.TargetChangeToken != first.TargetChangeToken || last.NextChangeToken != first.TargetChangeToken {
		t.Fatalf("last delta=%#v err=%v", last, err)
	}
	for _, page := range []*facade.InboxChanges{first, second, last} {
		forbiddenInboxFields(t, marshalInboxShape(t, page), "这是一段不能出现在增量响应中的完整正文。", "")
	}

	repo.clock = repo.clock.Add(inboxTokenTTL + time.Second)
	expired, err := service.ListInboxChanges(context.Background(), 42, facade.InboxChangeQuery{AfterChangeToken: base.ChangeToken})
	if err != nil || !expired.ResyncRequired || len(expired.Upserts) != 0 || expired.MailboxKey != "" {
		t.Fatalf("expired token result=%#v err=%v", expired, err)
	}
}

func TestInboxMailboxSequenceOnlyAdvancesForVisibleChangeAndIntentStaysContentFree(t *testing.T) {
	repo := newInboxTestRepository()
	service := newInboxTestService(t, repo, nil)
	if _, err := service.Publish(context.Background(), facade.PublishRequest{
		EventKey:       "billing.invoice.ready",
		IdempotencyKey: "sequence-42",
		Audience:       facade.Audience{UserIDs: []int64{42}},
		Category:       "BILLING",
		Priority:       "NORMAL",
		Title:          "账单已生成",
		Content:        "请打开消息中心查看账单明细。",
	}); err != nil {
		t.Fatalf("publish inbox message: %v", err)
	}
	page, err := service.ListInbox(context.Background(), 42, facade.InboxQuery{PageSize: 20})
	if err != nil || len(page.Records) != 1 {
		t.Fatalf("list inbox=%#v err=%v", page, err)
	}
	first, err := service.MutateInboxRecipient(context.Background(), 42, page.Records[0].RecipientID, domain.InboxActionRead, facade.InboxMutationRequest{ExpectedMailboxVersion: page.Records[0].MailboxVersion})
	if err != nil || first.MailboxVersion != "2" {
		t.Fatalf("first read=%#v err=%v, want sequence 2", first, err)
	}
	beforeNoop := len(repo.outbox)
	second, err := service.MutateInboxRecipient(context.Background(), 42, page.Records[0].RecipientID, domain.InboxActionRead, facade.InboxMutationRequest{ExpectedMailboxVersion: first.MailboxVersion})
	if err != nil || second.MailboxVersion != first.MailboxVersion {
		t.Fatalf("no-op read=%#v err=%v, want unchanged version", second, err)
	}
	if got := len(repo.outbox); got != beforeNoop {
		t.Fatalf("no-op read emitted outbox event: before=%d after=%d", beforeNoop, got)
	}
	mailbox := repo.mailboxesByOwner[mailboxOwnerKey("local", 42)]
	if mailbox == nil || mailbox.ChangeSequence != 2 {
		t.Fatalf("mailbox=%#v, want durable sequence 2", mailbox)
	}

	for _, event := range repo.outbox {
		if event.EventType != domain.OutboxEventNotificationInboxChanged {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
			t.Fatalf("decode inbox change payload: %v", err)
		}
		for _, forbidden := range []string{"title", "content", "deepLink", "recipientId"} {
			if _, exists := payload[forbidden]; exists {
				t.Fatalf("inbox change payload leaked %q: %#v", forbidden, payload)
			}
		}
	}
}

func marshalInboxShape(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal inbox response: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode inbox response: %v", err)
	}
	return result
}

func forbiddenInboxFields(t *testing.T, value map[string]any, fullContent, deepLink string) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal inbox shape: %v", err)
	}
	encoded := string(raw)
	for _, forbidden := range []string{
		"\"content\"", "\"deepLink\"", "\"eventKey\"", "\"category\"", "\"priority\"", "\"mandatory\"",
		fullContent, deepLink,
	} {
		if forbidden == "" {
			continue
		}
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestInboxPaginationCursorIsStableAndOwnerBound(t *testing.T) {
	repo := newInboxTestRepository()
	service := newInboxTestService(t, repo, nil)
	for index := 1; index <= 3; index++ {
		_, err := service.Publish(context.Background(), facade.PublishRequest{
			EventKey:       fmt.Sprintf("billing.invoice.%d", index),
			IdempotencyKey: fmt.Sprintf("invoice-%d", index),
			Audience:       facade.Audience{UserIDs: []int64{42}},
			Category:       "BILLING",
			Priority:       "NORMAL",
			Title:          fmt.Sprintf("账单 %d", index),
			Content:        "请查看账单详情。",
		})
		if err != nil {
			t.Fatalf("publish inbox record %d: %v", index, err)
		}
	}

	first, err := service.ListInbox(context.Background(), 42, facade.InboxQuery{PageSize: 2})
	if err != nil || len(first.Records) != 2 || first.NextPageCursor == "" || first.ChangeToken == "" {
		t.Fatalf("first inbox page=%#v err=%v", first, err)
	}
	firstIDs := map[string]struct{}{
		first.Records[0].RecipientID: {},
		first.Records[1].RecipientID: {},
	}
	second, err := service.ListInbox(context.Background(), 42, facade.InboxQuery{PageSize: 2, PageCursor: first.NextPageCursor})
	if err != nil || len(second.Records) != 1 || second.NextPageCursor != "" {
		t.Fatalf("second inbox page=%#v err=%v", second, err)
	}
	if _, duplicated := firstIDs[second.Records[0].RecipientID]; duplicated {
		t.Fatalf("page cursor repeated recipient %q", second.Records[0].RecipientID)
	}

	_, err = service.ListInbox(context.Background(), 43, facade.InboxQuery{PageCursor: first.NextPageCursor})
	assertInvalidInboxCursor(t, err)
	_, err = service.ListInbox(context.Background(), 42, facade.InboxQuery{Archived: true, PageCursor: first.NextPageCursor})
	assertInvalidInboxCursor(t, err)
}

func TestRoleAudienceMaterializationIsBoundedAndDuplicateSafe(t *testing.T) {
	repo := newInboxTestRepository()
	resolver := &fakeAudienceResolver{members: map[int64][]int64{7: {101, 102, 103}}}
	service := newInboxTestService(t, repo, resolver)
	receipt, err := service.Publish(context.Background(), facade.PublishRequest{
		EventKey:       "security.policy.updated",
		IdempotencyKey: "policy-7",
		Audience:       facade.Audience{UserIDs: []int64{101, 102}, RoleIDs: []int64{7}},
		Category:       "SECURITY",
		Priority:       "HIGH",
		Mandatory:      true,
		Title:          "安全策略已更新",
		Content:        "请查看新的安全策略。",
	})
	if err != nil || receipt.MaterializationStatus != domain.NotificationStatusAccepted {
		t.Fatalf("deferred publish receipt=%#v err=%v", receipt, err)
	}
	if got := len(repo.recipientsByID); got != 0 {
		t.Fatalf("role audience was materialized synchronously: %d recipients", got)
	}
	if got := len(repo.tasksByID); got != 1 {
		t.Fatalf("materialization task count=%d, want 1", got)
	}
	if err := service.MaterializePending(context.Background(), 1); err != nil {
		t.Fatalf("materialize pending: %v", err)
	}
	if got := len(repo.recipientsByID); got != 3 {
		t.Fatalf("recipient count=%d, want deduplicated 3", got)
	}
	for _, task := range repo.tasksByID {
		if task.Status != domain.TaskStatusDone || task.LeaseToken != "" {
			t.Fatalf("task did not finish with cleared lease: %#v", task)
		}
	}
	for _, item := range repo.notificationsByID {
		if item.Status != domain.NotificationStatusMaterialized {
			t.Fatalf("logical notification status=%q, want materialized", item.Status)
		}
	}
	if calls := resolver.calls; calls != 1 {
		t.Fatalf("resolver calls=%d, want one bounded page", calls)
	}
}

func TestRoleAudienceMaterializationResumesInBoundedPages(t *testing.T) {
	repo := newInboxTestRepository()
	members := make([]int64, 0, 205)
	for userID := int64(1); userID <= 205; userID++ {
		members = append(members, userID)
	}
	resolver := &fakeAudienceResolver{members: map[int64][]int64{7: members}}
	service := newInboxTestService(t, repo, resolver)
	if _, err := service.Publish(context.Background(), facade.PublishRequest{
		EventKey:       "security.rotation.required",
		IdempotencyKey: "rotation-7",
		Audience:       facade.Audience{RoleIDs: []int64{7}},
		Category:       "SECURITY",
		Priority:       "HIGH",
		Mandatory:      true,
		Title:          "请完成凭据轮换",
		Content:        "安全策略要求完成凭据轮换。",
	}); err != nil {
		t.Fatalf("publish deferred role audience: %v", err)
	}

	for expected := 100; expected <= 200; expected += 100 {
		if err := service.MaterializePending(context.Background(), 1); err != nil {
			t.Fatalf("materialize page ending at %d: %v", expected, err)
		}
		if got := len(repo.recipientsByID); got != expected {
			t.Fatalf("recipient count after bounded page=%d, want %d", got, expected)
		}
		for _, task := range repo.tasksByID {
			if task.Status != domain.TaskStatusPending || task.MaterializedCount != int64(expected) {
				t.Fatalf("task did not checkpoint after %d recipients: %#v", expected, task)
			}
		}
	}
	if err := service.MaterializePending(context.Background(), 1); err != nil {
		t.Fatalf("materialize final page: %v", err)
	}
	if got := len(repo.recipientsByID); got != 205 {
		t.Fatalf("recipient count after final page=%d, want 205", got)
	}
	for _, task := range repo.tasksByID {
		if task.Status != domain.TaskStatusDone || task.MaterializedCount != 205 || task.LeaseToken != "" {
			t.Fatalf("task did not finish after final page: %#v", task)
		}
	}
	if resolver.calls != 3 {
		t.Fatalf("role resolver calls=%d, want 3 bounded pages", resolver.calls)
	}
}

func TestMaterializationWorkerLeavesForeignScopePending(t *testing.T) {
	repo := newInboxTestRepository()
	service := newInboxTestService(t, repo, &fakeAudienceResolver{members: map[int64][]int64{7: {101}, 8: {202}}})

	if _, err := service.Publish(context.Background(), facade.PublishRequest{
		EventKey: "local.role", IdempotencyKey: "local-role", Audience: facade.Audience{RoleIDs: []int64{7}},
		Title: "本地通知", Content: "本地正文",
	}); err != nil {
		t.Fatalf("publish local notification: %v", err)
	}
	service.SetScopeID("node:foreign")
	if _, err := service.Publish(context.Background(), facade.PublishRequest{
		EventKey: "foreign.role", IdempotencyKey: "foreign-role", Audience: facade.Audience{RoleIDs: []int64{8}},
		Title: "外部通知", Content: "外部正文",
	}); err != nil {
		t.Fatalf("publish foreign notification: %v", err)
	}
	service.SetScopeID("local")
	if err := service.MaterializePending(context.Background(), 20); err != nil {
		t.Fatalf("materialize local scope: %v", err)
	}
	for _, recipient := range repo.recipientsByID {
		if recipient.ScopeID != "local" || recipient.UserID != 101 {
			t.Fatalf("local worker materialized foreign recipient: %#v", recipient)
		}
	}
	foreignPending := 0
	for _, task := range repo.tasksByID {
		if task.ScopeID == "node:foreign" && task.Status == domain.TaskStatusPending {
			foreignPending++
		}
	}
	if foreignPending != 1 {
		t.Fatalf("foreign pending tasks=%d, want 1", foreignPending)
	}
}

func TestMaterializationWorkerRejectsTaskNotificationScopeMismatch(t *testing.T) {
	repo := newInboxTestRepository()
	service := newInboxTestService(t, repo, nil)
	foreign := &domain.LogicalNotification{
		ID: 9001, NotificationID: "ntf_foreign", ScopeID: "node:foreign",
		EventKey: "foreign.event", IdempotencyKey: "foreign-key", AudienceJSON: `{"userIds":[202]}`,
		Category: "GENERAL", Priority: "NORMAL", Title: "外部通知", Content: "外部正文",
		Status: domain.NotificationStatusAccepted,
	}
	repo.notificationsByID[foreign.ID] = foreign
	repo.tasksByID[9101] = &domain.MaterializationTask{
		ID: 9101, TaskID: "task-mismatched-owner", NotificationID: foreign.ID, ScopeID: "local",
		AudienceJSON: foreign.AudienceJSON, Cursor: encodeMaterializationCursor(materializationCursor{}),
		Status: domain.TaskStatusPending, NextRunAt: repo.clock,
	}

	err := service.MaterializePending(context.Background(), 1)
	if err == nil || !strings.Contains(err.Error(), "ownership does not match") {
		t.Fatalf("scope-mismatched task error=%v", err)
	}
	if len(repo.recipientsByID) != 0 {
		t.Fatalf("scope-mismatched task created recipients=%#v", repo.recipientsByID)
	}
	task := repo.tasksByID[9101]
	if task.Status != domain.TaskStatusPending || !strings.Contains(task.LastError, "ownership does not match") {
		t.Fatalf("scope-mismatched task state=%#v", task)
	}
}

func TestPublishRejectsSecretEphemeralAndExternalDeepLinks(t *testing.T) {
	repo := newInboxTestRepository()
	service := newInboxTestService(t, repo, nil)
	base := facade.PublishRequest{
		EventKey:       "general.notice",
		IdempotencyKey: "notice-1",
		Audience:       facade.Audience{UserIDs: []int64{1}},
		Category:       "GENERAL",
		Priority:       "NORMAL",
		Title:          "通知",
		Content:        "内容",
	}
	external := base
	external.DeepLink = "https://attacker.example/redirect"
	if _, err := service.Publish(context.Background(), external); apperrors.From(err) == nil || apperrors.From(err).Code() != apperrors.CodeParamsError {
		t.Fatalf("external deep link err=%v, want params error", err)
	}
	secret := base
	secret.IdempotencyKey = "notice-2"
	secret.Category = "SECRET_EPHEMERAL"
	if _, err := service.Publish(context.Background(), secret); apperrors.From(err) == nil || apperrors.From(err).Code() != apperrors.CodeParamsError {
		t.Fatalf("secret ephemeral err=%v, want params error", err)
	}
	if len(repo.notificationsByID) != 0 || len(repo.recipientsByID) != 0 || len(repo.outbox) != 0 {
		t.Fatalf("rejected publish persisted data: notifications=%d recipients=%d outbox=%d", len(repo.notificationsByID), len(repo.recipientsByID), len(repo.outbox))
	}
}

func assertInvalidInboxCursor(t *testing.T, err error) {
	t.Helper()
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code() != apperrors.CodeParamsError {
		t.Fatalf("invalid page cursor error=%v, want params error", err)
	}
	details, ok := appErr.Details().(map[string]string)
	if !ok || details["reasonCode"] != "INVALID_PAGE_CURSOR" {
		t.Fatalf("invalid page cursor details=%#v", appErr.Details())
	}
}

func newInboxTestService(t *testing.T, repo *inboxTestRepository, resolver AudienceResolver) *Service {
	t.Helper()
	idGen, err := xid.New(19)
	if err != nil {
		t.Fatalf("new id generator: %v", err)
	}
	service := NewService(&inboxTestTransactor{repo: repo}, repo, domain.NewService(), inboxTestSecretService{}, nil, nil, nil, idGen)
	service.SetScopeID("local")
	service.now = func() time.Time { return repo.clock }
	if resolver != nil {
		service.BindAudienceResolver(resolver)
	}
	return service
}

func onlyRecipientID(t *testing.T, repo *inboxTestRepository) string {
	t.Helper()
	if len(repo.recipientsByID) != 1 {
		t.Fatalf("recipient count=%d, want 1", len(repo.recipientsByID))
	}
	for recipientID := range repo.recipientsByID {
		return recipientID
	}
	return ""
}

type inboxTestTransactor struct {
	repo *inboxTestRepository
}

func (t *inboxTestTransactor) Enabled() bool { return t != nil && t.repo != nil }

func (t *inboxTestTransactor) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	if t == nil || t.repo == nil {
		return fmt.Errorf("test transaction is not configured")
	}
	snapshot := t.repo.snapshot()
	if err := fn(ctx); err != nil {
		t.repo.restore(snapshot)
		return err
	}
	return nil
}

type inboxTestRepository struct {
	domain.Repository
	notificationsByID  map[int64]*domain.LogicalNotification
	notificationsByKey map[string]int64
	recipientsByID     map[string]*domain.Recipient
	recipientByOwner   map[string]string
	tasksByID          map[int64]*domain.MaterializationTask
	taskByNotification map[int64]int64
	mailboxesByOwner   map[string]*domain.Mailbox
	outbox             []domain.OutboxEvent
	appendErr          error
	clock              time.Time
	sequence           int64
	traceExpiryLocks   bool
	expiryLockTrace    []string
}

func newInboxTestRepository() *inboxTestRepository {
	return &inboxTestRepository{
		notificationsByID:  make(map[int64]*domain.LogicalNotification),
		notificationsByKey: make(map[string]int64),
		recipientsByID:     make(map[string]*domain.Recipient),
		recipientByOwner:   make(map[string]string),
		tasksByID:          make(map[int64]*domain.MaterializationTask),
		taskByNotification: make(map[int64]int64),
		mailboxesByOwner:   make(map[string]*domain.Mailbox),
		clock:              time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC),
	}
}

func (r *inboxTestRepository) FindLogicalNotificationByIdempotency(_ context.Context, scopeID, eventKey, idempotencyKey string) (*domain.LogicalNotification, error) {
	if r == nil {
		return nil, nil
	}
	return cloneLogicalNotification(r.notificationsByID[r.notificationsByKey[notificationKey(scopeID, eventKey, idempotencyKey)]]), nil
}

func (r *inboxTestRepository) FindLogicalNotificationByID(_ context.Context, notificationID int64) (*domain.LogicalNotification, error) {
	return cloneLogicalNotification(r.notificationsByID[notificationID]), nil
}

// ListExternalTargetsByNotificationID keeps the G2/G3 inbox fake explicitly
// external-free while allowing the shared idempotency path to check the
// snapshot table introduced by G4.
func (r *inboxTestRepository) ListExternalTargetsByNotificationID(_ context.Context, _ int64) ([]domain.ExternalTarget, error) {
	return []domain.ExternalTarget{}, nil
}

// ListDeliveriesByNotificationID keeps the G2/G3 inbox fake explicitly free
// of static HTTP routes while allowing the shared idempotency path to compare
// the G5.2 route set. Static-route behavior itself is covered by the external
// delivery test repository.
func (r *inboxTestRepository) ListDeliveriesByNotificationID(_ context.Context, _ int64) ([]domain.Delivery, error) {
	return []domain.Delivery{}, nil
}

func (r *inboxTestRepository) CreateLogicalNotification(_ context.Context, item *domain.LogicalNotification) (bool, error) {
	key := notificationKey(item.ScopeID, item.EventKey, item.IdempotencyKey)
	if _, found := r.notificationsByKey[key]; found {
		return false, nil
	}
	copy := cloneLogicalNotification(item)
	copy.CreateTime = r.nextTime()
	copy.UpdateTime = copy.CreateTime
	r.notificationsByID[copy.ID] = copy
	r.notificationsByKey[key] = copy.ID
	return true, nil
}

func (r *inboxTestRepository) MarkLogicalNotificationMaterialized(_ context.Context, notificationID int64) error {
	item := r.notificationsByID[notificationID]
	if item == nil {
		return fmt.Errorf("logical notification missing")
	}
	item.Status = domain.NotificationStatusMaterialized
	item.UpdateTime = r.nextTime()
	return nil
}

func (r *inboxTestRepository) InsertRecipients(_ context.Context, items []domain.Recipient) (int, error) {
	inserted := 0
	for _, item := range items {
		key := recipientOwnerKey(item.NotificationID, item.UserID)
		if _, found := r.recipientByOwner[key]; found {
			continue
		}
		copy := cloneRecipient(&item)
		copy.CreateTime = r.nextTime()
		copy.UpdateTime = copy.CreateTime
		r.recipientsByID[copy.RecipientID] = copy
		r.recipientByOwner[key] = copy.RecipientID
		inserted++
	}
	return inserted, nil
}

func (r *inboxTestRepository) InsertInboxRecipients(ctx context.Context, items []domain.Recipient) ([]domain.Recipient, error) {
	created := make([]domain.Recipient, 0, len(items))
	for _, item := range items {
		key := recipientOwnerKey(item.NotificationID, item.UserID)
		if _, found := r.recipientByOwner[key]; found {
			continue
		}
		mailbox, err := r.AdvanceMailboxChange(ctx, item.ScopeID, item.UserID)
		if err != nil {
			return created, err
		}
		copy := cloneRecipient(&item)
		copy.MailboxVersion = mailbox.ChangeSequence
		copy.CreateTime = r.nextTime()
		copy.UpdateTime = copy.CreateTime
		r.recipientsByID[copy.RecipientID] = copy
		r.recipientByOwner[key] = copy.RecipientID
		created = append(created, *cloneRecipient(copy))
	}
	return created, nil
}

func (r *inboxTestRepository) CreateMaterializationTask(_ context.Context, item *domain.MaterializationTask) (bool, error) {
	if _, found := r.taskByNotification[item.NotificationID]; found {
		return false, nil
	}
	copy := cloneMaterializationTask(item)
	copy.CreateTime = r.nextTime()
	copy.UpdateTime = copy.CreateTime
	r.tasksByID[copy.ID] = copy
	r.taskByNotification[copy.NotificationID] = copy.ID
	return true, nil
}

func (r *inboxTestRepository) FindMaterializationTaskByNotificationID(_ context.Context, notificationID int64) (*domain.MaterializationTask, error) {
	return cloneMaterializationTask(r.tasksByID[r.taskByNotification[notificationID]]), nil
}

func (r *inboxTestRepository) ListReadyMaterializationTasks(_ context.Context, scopeID string, limit int) ([]domain.MaterializationTask, error) {
	items := make([]domain.MaterializationTask, 0)
	for _, task := range r.tasksByID {
		if task.ScopeID != scopeID {
			continue
		}
		ready := task.Status == domain.TaskStatusPending && !task.NextRunAt.After(r.clock)
		if task.Status == domain.TaskStatusProcessing && (task.LeaseUntil == nil || !task.LeaseUntil.After(r.clock)) {
			ready = true
		}
		if ready {
			items = append(items, *cloneMaterializationTask(task))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *inboxTestRepository) TryClaimMaterializationTask(_ context.Context, scopeID string, taskID int64, _ string, now time.Time) (*domain.MaterializationTask, bool, error) {
	task := r.tasksByID[taskID]
	if task == nil || task.ScopeID != scopeID || (task.Status != domain.TaskStatusPending && task.Status != domain.TaskStatusProcessing) {
		return nil, false, nil
	}
	if task.Status == domain.TaskStatusProcessing && task.LeaseUntil != nil && task.LeaseUntil.After(now) {
		return nil, false, nil
	}
	task.Status = domain.TaskStatusProcessing
	task.LeaseToken = fmt.Sprintf("lease-%d", taskID)
	task.LeaseOwner = "test-worker"
	until := now.Add(time.Minute)
	task.LeaseUntil = &until
	task.UpdateTime = r.nextTime()
	return cloneMaterializationTask(task), true, nil
}

func (r *inboxTestRepository) AdvanceMaterializationTask(_ context.Context, scopeID string, taskID int64, leaseToken, cursor, status string, materializedCount int64, nextRunAt time.Time) (bool, error) {
	task := r.tasksByID[taskID]
	if task == nil || task.ScopeID != scopeID || task.Status != domain.TaskStatusProcessing || task.LeaseToken != leaseToken {
		return false, nil
	}
	task.Cursor = cursor
	task.Status = status
	task.MaterializedCount = materializedCount
	task.NextRunAt = nextRunAt
	task.LeaseOwner = ""
	task.LeaseToken = ""
	task.LeaseUntil = nil
	task.LastError = ""
	task.UpdateTime = r.nextTime()
	return true, nil
}

func (r *inboxTestRepository) FailMaterializationTask(_ context.Context, scopeID string, taskID int64, leaseToken, status, lastError string, retryCount int, nextRunAt time.Time) (bool, error) {
	task := r.tasksByID[taskID]
	if task == nil || task.ScopeID != scopeID || task.Status != domain.TaskStatusProcessing || task.LeaseToken != leaseToken {
		return false, nil
	}
	task.Status = status
	task.LastError = lastError
	task.RetryCount = retryCount
	task.NextRunAt = nextRunAt
	task.LeaseOwner = ""
	task.LeaseToken = ""
	task.LeaseUntil = nil
	task.UpdateTime = r.nextTime()
	return true, nil
}

func (r *inboxTestRepository) AppendOutbox(_ context.Context, event *domain.OutboxEvent) error {
	if r.appendErr != nil {
		return r.appendErr
	}
	if event == nil {
		return fmt.Errorf("outbox event is nil")
	}
	r.outbox = append(r.outbox, *event)
	return nil
}

func (r *inboxTestRepository) AppendOutboxBatch(_ context.Context, events []domain.OutboxEvent) error {
	if r.appendErr != nil {
		return r.appendErr
	}
	r.outbox = append(r.outbox, events...)
	return nil
}

func (r *inboxTestRepository) ListInboxRecipients(_ context.Context, query domain.InboxQuery) ([]domain.Recipient, error) {
	items := make([]domain.Recipient, 0)
	for _, item := range r.recipientsByID {
		if item.ScopeID != query.ScopeID || item.UserID != query.UserID || item.ExpiredAt != nil || (item.ExpiresAt != nil && !item.ExpiresAt.After(r.clock)) {
			continue
		}
		if query.Archived != (item.ArchivedAt != nil) {
			continue
		}
		if query.Cursor != nil && (item.CreateTime.After(query.Cursor.CreateTime) || (item.CreateTime.Equal(query.Cursor.CreateTime) && item.ID >= query.Cursor.ID)) {
			continue
		}
		items = append(items, *cloneRecipient(item))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreateTime.Equal(items[j].CreateTime) {
			return items[i].ID > items[j].ID
		}
		return items[i].CreateTime.After(items[j].CreateTime)
	})
	if query.Limit > 0 && len(items) > query.Limit {
		items = items[:query.Limit]
	}
	return items, nil
}

func (r *inboxTestRepository) ListUnreadInboxRecipients(_ context.Context, scopeID string, userID int64, limit int) ([]domain.Recipient, error) {
	items := make([]domain.Recipient, 0)
	for _, item := range r.recipientsByID {
		if item.ScopeID != scopeID || item.UserID != userID || item.ExpiredAt != nil || item.ArchivedAt != nil || item.ReadAt != nil || (item.ExpiresAt != nil && !item.ExpiresAt.After(r.clock)) {
			continue
		}
		items = append(items, *cloneRecipient(item))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreateTime.Equal(items[j].CreateTime) {
			return items[i].ID > items[j].ID
		}
		return items[i].CreateTime.After(items[j].CreateTime)
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *inboxTestRepository) ListInboxRecipientChanges(_ context.Context, query domain.InboxChangeQuery) ([]domain.Recipient, error) {
	items := make([]domain.Recipient, 0)
	for _, item := range r.recipientsByID {
		if item.ScopeID != query.ScopeID || item.UserID != query.UserID || item.MailboxVersion <= query.AfterSequence || item.MailboxVersion > query.UntilSequence {
			continue
		}
		items = append(items, *cloneRecipient(item))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].MailboxVersion < items[j].MailboxVersion })
	if query.Limit > 0 && len(items) > query.Limit {
		items = items[:query.Limit]
	}
	return items, nil
}

func (r *inboxTestRepository) ListExpiredInboxRecipients(_ context.Context, scopeID string, now time.Time, limit int) ([]domain.Recipient, error) {
	items := make([]domain.Recipient, 0)
	for _, item := range r.recipientsByID {
		if item.ScopeID != scopeID || item.ExpiredAt != nil || item.ExpiresAt == nil || item.ExpiresAt.After(now) {
			continue
		}
		items = append(items, *cloneRecipient(item))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ExpiresAt.Equal(*items[j].ExpiresAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].ExpiresAt.Before(*items[j].ExpiresAt)
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *inboxTestRepository) LockExpiredInboxRecipient(_ context.Context, recipientID int64, now time.Time) (*domain.Recipient, error) {
	if r.traceExpiryLocks {
		r.expiryLockTrace = append(r.expiryLockTrace, "recipient")
	}
	for _, item := range r.recipientsByID {
		if item.ID != recipientID {
			continue
		}
		if item.ExpiredAt != nil || item.ExpiresAt == nil || item.ExpiresAt.After(now) {
			return nil, nil
		}
		return cloneRecipient(item), nil
	}
	return nil, nil
}

func (r *inboxTestRepository) FindInboxRecipient(_ context.Context, scopeID string, userID int64, recipientID string) (*domain.Recipient, error) {
	item := r.recipientsByID[recipientID]
	if item == nil || item.ScopeID != scopeID || item.UserID != userID || item.ExpiredAt != nil || (item.ExpiresAt != nil && !item.ExpiresAt.After(r.clock)) {
		return nil, nil
	}
	return cloneRecipient(item), nil
}

func (r *inboxTestRepository) CountUnreadInboxRecipients(_ context.Context, scopeID string, userID int64) (int64, error) {
	var count int64
	for _, item := range r.recipientsByID {
		if item.ScopeID == scopeID && item.UserID == userID && item.ExpiredAt == nil && item.ArchivedAt == nil && item.ReadAt == nil && (item.ExpiresAt == nil || item.ExpiresAt.After(r.clock)) {
			count++
		}
	}
	return count, nil
}

func (r *inboxTestRepository) LockMailbox(_ context.Context, scopeID string, userID int64, mailboxKey string) (*domain.Mailbox, error) {
	if r.traceExpiryLocks {
		r.expiryLockTrace = append(r.expiryLockTrace, "mailbox")
	}
	key := mailboxOwnerKey(scopeID, userID)
	if existing := r.mailboxesByOwner[key]; existing != nil {
		return cloneMailbox(existing), nil
	}
	mailbox := &domain.Mailbox{
		ID:             int64(len(r.mailboxesByOwner) + 1),
		ScopeID:        scopeID,
		UserID:         userID,
		MailboxKey:     mailboxKey,
		ChangeSequence: 0,
		CreateTime:     r.nextTime(),
	}
	mailbox.UpdateTime = mailbox.CreateTime
	r.mailboxesByOwner[key] = mailbox
	return cloneMailbox(mailbox), nil
}

func (r *inboxTestRepository) AdvanceMailboxChange(ctx context.Context, scopeID string, userID int64) (*domain.Mailbox, error) {
	_, err := r.LockMailbox(ctx, scopeID, userID, fmt.Sprintf("mbx-test-%s-%d", scopeID, userID))
	if err != nil {
		return nil, err
	}
	stored := r.mailboxesByOwner[mailboxOwnerKey(scopeID, userID)]
	stored.ChangeSequence++
	stored.UpdateTime = r.nextTime()
	return cloneMailbox(stored), nil
}

func (r *inboxTestRepository) CompareAndSetInboxRecipient(_ context.Context, item *domain.Recipient, expectedMailboxVersion int64) (bool, error) {
	stored := r.recipientsByID[item.RecipientID]
	if stored == nil || stored.ScopeID != item.ScopeID || stored.UserID != item.UserID || stored.MailboxVersion != expectedMailboxVersion {
		return false, nil
	}
	copy := cloneRecipient(item)
	r.recipientsByID[copy.RecipientID] = copy
	return true, nil
}

func (r *inboxTestRepository) snapshot() inboxTestRepositoryState {
	result := inboxTestRepositoryState{
		notificationsByID:  make(map[int64]*domain.LogicalNotification, len(r.notificationsByID)),
		notificationsByKey: make(map[string]int64, len(r.notificationsByKey)),
		recipientsByID:     make(map[string]*domain.Recipient, len(r.recipientsByID)),
		recipientByOwner:   make(map[string]string, len(r.recipientByOwner)),
		tasksByID:          make(map[int64]*domain.MaterializationTask, len(r.tasksByID)),
		taskByNotification: make(map[int64]int64, len(r.taskByNotification)),
		mailboxesByOwner:   make(map[string]*domain.Mailbox, len(r.mailboxesByOwner)),
		outbox:             append([]domain.OutboxEvent(nil), r.outbox...),
		sequence:           r.sequence,
	}
	for key, item := range r.notificationsByID {
		result.notificationsByID[key] = cloneLogicalNotification(item)
	}
	for key, item := range r.notificationsByKey {
		result.notificationsByKey[key] = item
	}
	for key, item := range r.recipientsByID {
		result.recipientsByID[key] = cloneRecipient(item)
	}
	for key, item := range r.recipientByOwner {
		result.recipientByOwner[key] = item
	}
	for key, item := range r.tasksByID {
		result.tasksByID[key] = cloneMaterializationTask(item)
	}
	for key, item := range r.taskByNotification {
		result.taskByNotification[key] = item
	}
	for key, item := range r.mailboxesByOwner {
		result.mailboxesByOwner[key] = cloneMailbox(item)
	}
	return result
}

func (r *inboxTestRepository) restore(snapshot inboxTestRepositoryState) {
	r.notificationsByID = snapshot.notificationsByID
	r.notificationsByKey = snapshot.notificationsByKey
	r.recipientsByID = snapshot.recipientsByID
	r.recipientByOwner = snapshot.recipientByOwner
	r.tasksByID = snapshot.tasksByID
	r.taskByNotification = snapshot.taskByNotification
	r.mailboxesByOwner = snapshot.mailboxesByOwner
	r.outbox = snapshot.outbox
	r.sequence = snapshot.sequence
}

func (r *inboxTestRepository) nextTime() time.Time {
	r.sequence++
	return r.clock.Add(time.Duration(r.sequence) * time.Microsecond)
}

type inboxTestRepositoryState struct {
	notificationsByID  map[int64]*domain.LogicalNotification
	notificationsByKey map[string]int64
	recipientsByID     map[string]*domain.Recipient
	recipientByOwner   map[string]string
	tasksByID          map[int64]*domain.MaterializationTask
	taskByNotification map[int64]int64
	mailboxesByOwner   map[string]*domain.Mailbox
	outbox             []domain.OutboxEvent
	sequence           int64
}

type fakeAudienceResolver struct {
	members map[int64][]int64
	calls   int
}

func (r *fakeAudienceResolver) ListActiveUserIDsByRoleIDPage(_ context.Context, roleID, afterUserID int64, limit int) ([]int64, error) {
	r.calls++
	result := make([]int64, 0)
	for _, userID := range r.members[roleID] {
		if userID <= afterUserID {
			continue
		}
		result = append(result, userID)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result, nil
}

func notificationKey(scopeID, eventKey, idempotencyKey string) string {
	return scopeID + "\x00" + eventKey + "\x00" + idempotencyKey
}

func recipientOwnerKey(notificationID, userID int64) string {
	return fmt.Sprintf("%d:%d", notificationID, userID)
}

func mailboxOwnerKey(scopeID string, userID int64) string {
	return scopeID + "\x00" + fmt.Sprintf("%d", userID)
}

func cloneLogicalNotification(item *domain.LogicalNotification) *domain.LogicalNotification {
	if item == nil {
		return nil
	}
	copy := *item
	copy.ScheduleAt = cloneTime(item.ScheduleAt)
	copy.ExpiresAt = cloneTime(item.ExpiresAt)
	copy.CreatorID = cloneInt64(item.CreatorID)
	return &copy
}

func cloneRecipient(item *domain.Recipient) *domain.Recipient {
	if item == nil {
		return nil
	}
	copy := *item
	copy.ExpiresAt = cloneTime(item.ExpiresAt)
	copy.ExpiredAt = cloneTime(item.ExpiredAt)
	copy.FirstSeenAt = cloneTime(item.FirstSeenAt)
	copy.ReadAt = cloneTime(item.ReadAt)
	copy.ArchivedAt = cloneTime(item.ArchivedAt)
	return &copy
}

func cloneMaterializationTask(item *domain.MaterializationTask) *domain.MaterializationTask {
	if item == nil {
		return nil
	}
	copy := *item
	copy.LeaseUntil = cloneTime(item.LeaseUntil)
	return &copy
}

func cloneMailbox(item *domain.Mailbox) *domain.Mailbox {
	if item == nil {
		return nil
	}
	copy := *item
	return &copy
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

type inboxTestSecretService struct{}

func (inboxTestSecretService) EncryptString(_ context.Context, plain string) (secretvalueinfra.SecretValue, error) {
	return secretvalueinfra.SecretValue{
		CiphertextB64: base64.RawURLEncoding.EncodeToString([]byte(plain)),
		EDEKB64:       "test-edek",
		WrapKeyRef:    "test-key",
	}, nil
}

func (inboxTestSecretService) DecryptString(_ context.Context, value secretvalueinfra.SecretValue) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value.CiphertextB64)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
