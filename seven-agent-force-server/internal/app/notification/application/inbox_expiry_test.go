package application

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
)

func TestExpiredInboxRecipientProducesContentFreeRemovalDelta(t *testing.T) {
	repo := newInboxTestRepository()
	service := newInboxTestService(t, repo, nil)
	expiresAt := repo.clock.Add(time.Minute)
	if _, err := service.Publish(context.Background(), facade.PublishRequest{
		EventKey:       "account.session.expiring",
		IdempotencyKey: "expiry-removal-42",
		Audience:       facade.Audience{UserIDs: []int64{42}},
		Category:       "ACCOUNT",
		Priority:       "NORMAL",
		Title:          "会话即将到期",
		Content:        "这段完整正文不能通过到期增量再次返回。",
		DeepLink:       "/account/security",
		ExpiresAt:      &expiresAt,
	}); err != nil {
		t.Fatalf("publish expiring inbox message: %v", err)
	}

	page, err := service.ListInbox(context.Background(), 42, facade.InboxQuery{PageSize: 20})
	if err != nil || len(page.Records) != 1 {
		t.Fatalf("baseline inbox page=%#v err=%v", page, err)
	}
	recipientID := page.Records[0].RecipientID
	foreignRecipientID := "nrc_other_scope_expiring"
	repo.recipientsByID[foreignRecipientID] = &domain.Recipient{
		ID:             999_001,
		RecipientID:    foreignRecipientID,
		NotificationID: 999_001,
		ScopeID:        "node:other",
		UserID:         42,
		Title:          "其他 scope 的消息",
		Content:        "不得由当前实例的到期任务处理。",
		ExpiresAt:      &expiresAt,
		MailboxVersion: 1,
		CreateTime:     repo.clock,
		UpdateTime:     repo.clock,
	}

	repo.clock = expiresAt.Add(time.Second)
	expired, err := service.ExpireInboxRecipients(context.Background(), 50)
	if err != nil || expired != 1 {
		t.Fatalf("expire due recipients=%d err=%v, want 1 nil", expired, err)
	}

	if listed, listErr := service.ListInbox(context.Background(), 42, facade.InboxQuery{PageSize: 20}); listErr != nil || len(listed.Records) != 0 {
		t.Fatalf("expired recipient remained in list=%#v err=%v", listed, listErr)
	}
	if preview, previewErr := service.UnreadPreview(context.Background(), 42, 5); previewErr != nil || len(preview.Records) != 0 {
		t.Fatalf("expired recipient remained in preview=%#v err=%v", preview, previewErr)
	}

	delta, err := service.ListInboxChanges(context.Background(), 42, facade.InboxChangeQuery{
		AfterChangeToken: page.ChangeToken,
		Limit:            20,
	})
	if err != nil || delta.ResyncRequired {
		t.Fatalf("expiry delta=%#v err=%v", delta, err)
	}
	if len(delta.Upserts) != 0 || !reflect.DeepEqual(delta.RemovedRecipientIDs, []string{recipientID}) {
		t.Fatalf("expiry delta must contain only the removal marker: %#v", delta)
	}
	forbiddenInboxFields(t, marshalInboxShape(t, delta), "这段完整正文不能通过到期增量再次返回。", "/account/security")

	again, err := service.ExpireInboxRecipients(context.Background(), 50)
	if err != nil || again != 0 {
		t.Fatalf("second expiry pass=%d err=%v, want 0 nil", again, err)
	}
	if foreign := repo.recipientsByID[foreignRecipientID]; foreign == nil || foreign.ExpiredAt != nil {
		t.Fatalf("current scope expiry worker touched foreign scope recipient=%#v", foreign)
	}
}

func TestExpireInboxRecipientsLocksMailboxBeforeRecipient(t *testing.T) {
	repo := newInboxTestRepository()
	service := newInboxTestService(t, repo, nil)
	expiresAt := repo.clock.Add(-time.Second)
	if _, err := service.Publish(context.Background(), facade.PublishRequest{
		EventKey:       "account.session.expiring",
		IdempotencyKey: "expiry-lock-order-42",
		Audience:       facade.Audience{UserIDs: []int64{42}},
		Category:       "ACCOUNT",
		Priority:       "NORMAL",
		Title:          "会话已到期",
		Content:        "用于验证到期任务的锁顺序。",
		ExpiresAt:      &expiresAt,
	}); err != nil {
		t.Fatalf("publish expired inbox message: %v", err)
	}

	repo.traceExpiryLocks = true
	repo.expiryLockTrace = nil
	if expired, err := service.ExpireInboxRecipients(context.Background(), 1); err != nil || expired != 1 {
		t.Fatalf("expire due recipient=%d err=%v, want 1 nil", expired, err)
	}
	if len(repo.expiryLockTrace) < 2 || !reflect.DeepEqual(repo.expiryLockTrace[:2], []string{"mailbox", "recipient"}) {
		t.Fatalf("expiry must lock mailbox before recipient to match user mutation order, trace=%v", repo.expiryLockTrace)
	}
}
