package application

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/google/uuid"
)

const (
	inlineRecipientLimit          = 100
	materializationRecipientLimit = 100
	materializationTaskLimit      = 20
	inboxExpiryRecipientLimit     = 50
	materializationLeaseOwner     = "notification-materializer"
)

// publishDirect preserves the semantic G2-G5 Client contract. It is not an
// administration compatibility path: it does not read or expose the removed
// legacy template and scene management APIs.
func (s *Service) publishDirect(ctx context.Context, request facade.PublishRequest) (*facade.PublishReceipt, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("notification service is not configured")
	}
	if strings.TrimSpace(s.scopeID) == "" {
		return nil, fmt.Errorf("notification scope is not configured")
	}
	receipt := &facade.PublishReceipt{}
	var warnings []facade.ProviderParameterWarning
	err := s.withinTx(ctx, func(txCtx context.Context) error {
		item, audience, createErr := s.newLogicalNotificationBase(request, len(request.ExternalRecipients)+len(request.StaticRoutes))
		if createErr != nil {
			return createErr
		}
		existing, err := s.repo.FindLogicalNotificationByIdempotency(txCtx, item.ScopeID, item.EventKey, item.IdempotencyKey)
		if err != nil {
			return err
		}
		if existing != nil {
			existingWarnings, matchErr := s.applyExistingDirectIdempotency(txCtx, existing, item, audience, request.ExternalRecipients, request.StaticRoutes, receipt)
			if matchErr != nil {
				return matchErr
			}
			warnings = existingWarnings
			receipt.Warnings = append([]facade.ProviderParameterWarning(nil), existingWarnings...)
			return nil
		}

		preparedExternal, preparedWarnings, prepareErr := s.prepareExternalRecipients(txCtx, request.ExternalRecipients)
		if prepareErr != nil {
			return prepareErr
		}
		preparedStatic, staticErr := s.prepareStaticRoutes(txCtx, request.StaticRoutes)
		if staticErr != nil {
			return staticErr
		}
		item.RequestFingerprint = domain.CanonicalFingerprintWithStaticRoutes(*item, audience, s.externalRecipientFingerprints(preparedExternal), s.staticRouteFingerprints(preparedStatic))

		created, err := s.repo.CreateLogicalNotification(txCtx, item)
		if err != nil {
			return err
		}
		if !created {
			existing, findErr := s.repo.FindLogicalNotificationByIdempotency(txCtx, item.ScopeID, item.EventKey, item.IdempotencyKey)
			if findErr != nil {
				return findErr
			}
			if existing == nil {
				return fmt.Errorf("notification idempotency record disappeared")
			}
			if err := applyIdempotentReceipt(existing, item, receipt); err != nil {
				return err
			}
			warnings = preparedWarnings
			receipt.Warnings = append([]facade.ProviderParameterWarning(nil), preparedWarnings...)
			return nil
		}

		deferred := audience.HasDeferredAudience() || len(audience.UserIDs) > inlineRecipientLimit || isScheduled(item.ScheduleAt, s.now())
		if deferred {
			if err := s.createMaterializationTask(txCtx, item, audience); err != nil {
				return err
			}
		} else {
			recipients := s.newRecipients(item, audience.UserIDs)
			createdRecipients, err := s.repo.InsertInboxRecipients(txCtx, recipients)
			if err != nil {
				return err
			}
			if err := s.appendInboxChangedIntents(txCtx, createdRecipients, true); err != nil {
				return err
			}
		}
		if err := s.createExternalTargetsAndDeliveries(txCtx, item, preparedExternal); err != nil {
			return err
		}
		if err := s.createStaticRouteDeliveries(txCtx, item, preparedStatic); err != nil {
			return err
		}

		intent := domain.IntentMessage{NotificationID: item.ID, ScopeID: item.ScopeID}
		if err := s.repo.AppendOutbox(txCtx, &domain.OutboxEvent{
			ID:            s.nextID(),
			EventID:       "notification-intent:" + item.NotificationID,
			ScopeID:       item.ScopeID,
			EventType:     domain.OutboxEventNotificationIntent,
			AggregateType: domain.OutboxAggregateNotification,
			AggregateID:   item.NotificationID,
			Payload:       mustJSON(intent),
			Status:        "PENDING",
		}); err != nil {
			return err
		}

		receipt.NotificationID = item.NotificationID
		receipt.Status = item.Status
		receipt.MaterializationStatus = materializationStatus(deferred, item.Status)
		warnings = preparedWarnings
		receipt.Warnings = append([]facade.ProviderParameterWarning(nil), preparedWarnings...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.logProviderParameterWarnings(warnings)
	return receipt, nil
}

// ListInbox returns only compact recipient cards for the current user. Full
// content and deep links stay behind GetInboxRecipient.
func (s *Service) ListInbox(ctx context.Context, userID int64, query facade.InboxQuery) (*facade.InboxPage, error) {
	if err := s.requireInboxOwner(userID); err != nil {
		return nil, err
	}
	createTime, rowID, err := s.decodeInboxPageCursor(ctx, query.PageCursor, s.scopeID, userID, query.Archived)
	if err != nil {
		return nil, err
	}
	var cursor *domain.InboxCursor
	if !createTime.IsZero() {
		cursor = &domain.InboxCursor{CreateTime: createTime, ID: rowID}
	}
	limit := normalizeInboxPageSize(query.PageSize)
	var page *facade.InboxPage
	err = s.withinTx(ctx, func(txCtx context.Context) error {
		mailbox, lockErr := s.repo.LockMailbox(txCtx, s.scopeID, userID, s.newMailboxKey())
		if lockErr != nil {
			return lockErr
		}
		items, listErr := s.repo.ListInboxRecipients(txCtx, domain.InboxQuery{
			ScopeID:  s.scopeID,
			UserID:   userID,
			Archived: query.Archived,
			Cursor:   cursor,
			Limit:    limit + 1,
		})
		if listErr != nil {
			return listErr
		}
		page = &facade.InboxPage{Records: make([]facade.InboxListItem, 0, minInbox(limit, len(items)))}
		if len(items) > limit {
			items = items[:limit]
			last := items[len(items)-1]
			page.NextPageCursor, listErr = s.encodeInboxPageCursor(txCtx, s.scopeID, userID, query.Archived, last.CreateTime, last.RecipientID, last.ID)
			if listErr != nil {
				return listErr
			}
		}
		for _, item := range items {
			page.Records = append(page.Records, mapInboxListItem(item))
		}
		page.ChangeToken, listErr = s.encodeInboxChangeToken(txCtx, s.scopeID, userID, mailbox.ChangeSequence)
		return listErr
	})
	if err != nil {
		return nil, err
	}
	return page, nil
}

// GetInboxRecipient loads one recipient after scoping both the lookup and the
// result to the authenticated mailbox owner.
func (s *Service) GetInboxRecipient(ctx context.Context, userID int64, recipientID string) (*facade.InboxDetail, error) {
	item, err := s.findInboxRecipient(ctx, userID, recipientID)
	if err != nil || item == nil {
		return nil, err
	}
	record := mapInboxDetail(*item)
	return &record, nil
}

// UnreadCount counts only non-archived, unexpired recipient projections that
// have no read timestamp. It never consults external delivery state.
func (s *Service) UnreadCount(ctx context.Context, userID int64) (*facade.UnreadCount, error) {
	if err := s.requireInboxOwner(userID); err != nil {
		return nil, err
	}
	var result *facade.UnreadCount
	err := s.withinTx(ctx, func(txCtx context.Context) error {
		mailbox, lockErr := s.repo.LockMailbox(txCtx, s.scopeID, userID, s.newMailboxKey())
		if lockErr != nil {
			return lockErr
		}
		count, countErr := s.repo.CountUnreadInboxRecipients(txCtx, s.scopeID, userID)
		if countErr != nil {
			return countErr
		}
		token, tokenErr := s.encodeInboxChangeToken(txCtx, s.scopeID, userID, mailbox.ChangeSequence)
		if tokenErr != nil {
			return tokenErr
		}
		result = &facade.UnreadCount{MailboxKey: mailbox.MailboxKey, Count: count, ChangeToken: token}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// SubscribeInboxChanges returns only the current user's content-free committed
// change hints. A nil realtime adapter leaves the SSE endpoint connected but
// quiet; REST count and reads remain the source of truth.
func (s *Service) SubscribeInboxChanges(userID int64) (<-chan domain.InboxChangedIntent, func()) {
	if s == nil || s.realtime == nil || userID <= 0 {
		closed := make(chan domain.InboxChangedIntent)
		close(closed)
		return closed, func() {}
	}
	return s.realtime.SubscribeInboxChanges(userID)
}

// InboxRealtimeHint converts an internal change sequence into an opaque token
// only after confirming that it belongs to the same authenticated mailbox.
// The result intentionally carries no recipient ID, title, preview or body.
func (s *Service) InboxRealtimeHint(ctx context.Context, userID int64, intent domain.InboxChangedIntent) (*facade.InboxRealtimeHint, error) {
	if err := s.requireInboxOwner(userID); err != nil {
		return nil, err
	}
	if intent.ScopeID != s.scopeID || intent.UserID != userID || intent.ChangeSequence <= 0 {
		return nil, fmt.Errorf("notification inbox realtime hint is outside the current mailbox")
	}
	token, err := s.encodeInboxChangeToken(ctx, s.scopeID, userID, intent.ChangeSequence)
	if err != nil {
		return nil, err
	}
	return &facade.InboxRealtimeHint{ChangeToken: token, NewUnread: intent.NewUnread}, nil
}

// UnreadPreview returns at most five safe unread summaries after the user has
// explicitly opened the bell preview. It never returns full bodies or links.
func (s *Service) UnreadPreview(ctx context.Context, userID int64, limit int) (*facade.InboxPreview, error) {
	if err := s.requireInboxOwner(userID); err != nil {
		return nil, err
	}
	limit = normalizeInboxPreviewLimit(limit)
	var result *facade.InboxPreview
	err := s.withinTx(ctx, func(txCtx context.Context) error {
		mailbox, lockErr := s.repo.LockMailbox(txCtx, s.scopeID, userID, s.newMailboxKey())
		if lockErr != nil {
			return lockErr
		}
		items, listErr := s.repo.ListUnreadInboxRecipients(txCtx, s.scopeID, userID, limit)
		if listErr != nil {
			return listErr
		}
		token, tokenErr := s.encodeInboxChangeToken(txCtx, s.scopeID, userID, mailbox.ChangeSequence)
		if tokenErr != nil {
			return tokenErr
		}
		result = &facade.InboxPreview{Records: make([]facade.InboxPreviewItem, 0, len(items)), MailboxKey: mailbox.MailboxKey, ChangeToken: token}
		for _, item := range items {
			result.Records = append(result.Records, mapInboxPreviewItem(item))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ListInboxChanges returns a bounded compact delta for an already-open message
// center. Invalid, foreign, expired, or impossible tokens request resync
// without disclosing another mailbox's state.
func (s *Service) ListInboxChanges(ctx context.Context, userID int64, query facade.InboxChangeQuery) (*facade.InboxChanges, error) {
	if err := s.requireInboxOwner(userID); err != nil {
		return nil, err
	}
	after, err := s.decodeOptionalInboxChangeToken(ctx, query.AfterChangeToken, userID)
	if err != nil {
		return s.inboxResyncResult(), nil
	}
	until, hasUntil, err := s.decodeOptionalInboxChangeTokenWithPresence(ctx, query.UntilChangeToken, userID)
	if err != nil {
		return s.inboxResyncResult(), nil
	}
	limit := normalizeInboxChangeLimit(query.Limit)
	var result *facade.InboxChanges
	err = s.withinTx(ctx, func(txCtx context.Context) error {
		mailbox, lockErr := s.repo.LockMailbox(txCtx, s.scopeID, userID, s.newMailboxKey())
		if lockErr != nil {
			return lockErr
		}
		target := mailbox.ChangeSequence
		if hasUntil {
			target = until
		}
		if after > target || target > mailbox.ChangeSequence {
			result = s.inboxResyncResult()
			return nil
		}
		items, listErr := s.repo.ListInboxRecipientChanges(txCtx, domain.InboxChangeQuery{
			ScopeID:       s.scopeID,
			UserID:        userID,
			AfterSequence: after,
			UntilSequence: target,
			Limit:         limit + 1,
		})
		if listErr != nil {
			return listErr
		}
		count, countErr := s.repo.CountUnreadInboxRecipients(txCtx, s.scopeID, userID)
		if countErr != nil {
			return countErr
		}
		result = &facade.InboxChanges{
			Upserts:             make([]facade.InboxListItem, 0, minInbox(limit, len(items))),
			RemovedRecipientIDs: make([]string, 0),
			MailboxKey:          mailbox.MailboxKey,
			UnreadCount:         count,
			ServerTime:          s.now().UTC(),
		}
		if len(items) > limit {
			items = items[:limit]
			result.HasMore = true
		}
		for _, item := range items {
			if item.ExpiredAt != nil {
				result.RemovedRecipientIDs = append(result.RemovedRecipientIDs, item.RecipientID)
				continue
			}
			result.Upserts = append(result.Upserts, mapInboxListItem(item))
		}
		nextSequence := target
		if len(items) > 0 && result.HasMore {
			nextSequence = items[len(items)-1].MailboxVersion
		}
		result.NextChangeToken, listErr = s.encodeInboxChangeToken(txCtx, s.scopeID, userID, nextSequence)
		if listErr != nil {
			return listErr
		}
		result.TargetChangeToken, listErr = s.encodeInboxChangeToken(txCtx, s.scopeID, userID, target)
		return listErr
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// MutateInboxRecipient applies one idempotent recipient-state action. A stale
// expected mailbox version fails closed and does not overwrite newer state.
func (s *Service) MutateInboxRecipient(ctx context.Context, userID int64, recipientID string, action string, request facade.InboxMutationRequest) (*facade.InboxListItem, error) {
	if err := s.requireInboxOwner(userID); err != nil {
		return nil, err
	}
	expected, err := parseExpectedMailboxVersion(request.ExpectedMailboxVersion)
	if err != nil {
		return nil, err
	}
	var result *facade.InboxListItem
	err = s.withinTx(ctx, func(txCtx context.Context) error {
		item, findErr := s.findInboxRecipient(txCtx, userID, recipientID)
		if findErr != nil || item == nil {
			return findErr
		}
		if expected > 0 && expected != item.MailboxVersion {
			return mailboxVersionConflict(item.MailboxVersion)
		}
		currentMailboxVersion := item.MailboxVersion
		changed, actionErr := item.ApplyInboxAction(action, s.now())
		if actionErr != nil {
			return apperrors.Params(actionErr.Error())
		}
		if changed {
			mailbox, advanceErr := s.repo.AdvanceMailboxChange(txCtx, s.scopeID, userID)
			if advanceErr != nil {
				return advanceErr
			}
			if versionErr := item.SetMailboxVersion(mailbox.ChangeSequence); versionErr != nil {
				return versionErr
			}
			updated, updateErr := s.repo.CompareAndSetInboxRecipient(txCtx, item, currentMailboxVersion)
			if updateErr != nil {
				return updateErr
			}
			if !updated {
				current, currentErr := s.findInboxRecipient(txCtx, userID, recipientID)
				if currentErr != nil {
					return currentErr
				}
				if current == nil {
					return apperrors.NotFound("通知不存在")
				}
				return mailboxVersionConflict(current.MailboxVersion)
			}
			if appendErr := s.appendInboxChangedIntents(txCtx, []domain.Recipient{*item}, false); appendErr != nil {
				return appendErr
			}
		}
		record := mapInboxListItem(*item)
		result = &record
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// MaterializePending processes a bounded number of durable tasks. It is called
// by the existing bounded notification Outbox job and does not create unbounded
// goroutines or request-sized worker pools.
func (s *Service) MaterializePending(ctx context.Context, limit int) error {
	if s == nil || s.repo == nil {
		return nil
	}
	scopeID := strings.TrimSpace(s.scopeID)
	if scopeID == "" {
		return fmt.Errorf("notification scope is not configured")
	}
	if limit <= 0 || limit > materializationTaskLimit {
		limit = materializationTaskLimit
	}
	tasks, err := s.repo.ListReadyMaterializationTasks(ctx, scopeID, limit)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if task.ScopeID != scopeID {
			return fmt.Errorf("notification materialization candidate is outside the configured scope")
		}
		if err := s.materializeTask(ctx, scopeID, task.ID); err != nil {
			return err
		}
	}
	return nil
}

// ExpireInboxRecipients records a bounded set of due recipient projections as
// no longer visible. Each accepted expiry advances that recipient's mailbox
// sequence and appends a content-free change intent in the same transaction so
// an already-open message center can remove stale cards without reading a body.
func (s *Service) ExpireInboxRecipients(ctx context.Context, limit int) (int, error) {
	if s == nil || s.repo == nil {
		return 0, nil
	}
	if strings.TrimSpace(s.scopeID) == "" {
		return 0, fmt.Errorf("notification scope is not configured")
	}
	if limit <= 0 || limit > inboxExpiryRecipientLimit {
		limit = inboxExpiryRecipientLimit
	}
	now := s.now().UTC()
	candidates, err := s.repo.ListExpiredInboxRecipients(ctx, s.scopeID, now, limit)
	if err != nil {
		return 0, err
	}
	expired := 0
	for _, candidate := range candidates {
		changed := false
		err = s.withinTx(ctx, func(txCtx context.Context) error {
			if candidate.ScopeID != s.scopeID || candidate.UserID <= 0 {
				return fmt.Errorf("notification inbox expiry candidate is outside the configured scope")
			}
			// Every visible mutation takes the mailbox lock before the recipient
			// row. MutateInboxRecipient advances this same mailbox before its CAS;
			// preserving that order prevents the expiry worker and a user action
			// from holding the two rows in opposite order.
			if _, lockErr := s.repo.LockMailbox(txCtx, candidate.ScopeID, candidate.UserID, s.newMailboxKey()); lockErr != nil {
				return lockErr
			}
			item, lockErr := s.repo.LockExpiredInboxRecipient(txCtx, candidate.ID, now)
			if lockErr != nil || item == nil {
				return lockErr
			}
			if item.ScopeID != candidate.ScopeID || item.UserID != candidate.UserID {
				return fmt.Errorf("notification inbox expiry candidate ownership changed")
			}
			expectedMailboxVersion := item.MailboxVersion
			visibleChange, actionErr := item.ApplyInboxAction(domain.InboxActionExpire, now)
			if actionErr != nil {
				return actionErr
			}
			if !visibleChange {
				return nil
			}
			mailbox, advanceErr := s.repo.AdvanceMailboxChange(txCtx, item.ScopeID, item.UserID)
			if advanceErr != nil {
				return advanceErr
			}
			if versionErr := item.SetMailboxVersion(mailbox.ChangeSequence); versionErr != nil {
				return versionErr
			}
			updated, updateErr := s.repo.CompareAndSetInboxRecipient(txCtx, item, expectedMailboxVersion)
			if updateErr != nil {
				return updateErr
			}
			if !updated {
				return fmt.Errorf("notification inbox expiry update was superseded")
			}
			if appendErr := s.appendInboxChangedIntents(txCtx, []domain.Recipient{*item}, false); appendErr != nil {
				return appendErr
			}
			changed = true
			return nil
		})
		if err != nil {
			return expired, err
		}
		if changed {
			expired++
		}
	}
	return expired, nil
}

func (s *Service) newLogicalNotification(request facade.PublishRequest, external []preparedExternalRecipient) (*domain.LogicalNotification, domain.AudienceSnapshot, error) {
	item, audience, err := s.newLogicalNotificationBase(request, len(external))
	if err != nil {
		return nil, domain.AudienceSnapshot{}, err
	}
	item.RequestFingerprint = domain.CanonicalFingerprintWithExternal(*item, audience, s.externalRecipientFingerprints(external))
	return item, audience, nil
}

func (s *Service) newLogicalNotificationBase(request facade.PublishRequest, externalRecipientCount int) (*domain.LogicalNotification, domain.AudienceSnapshot, error) {
	audience, err := domain.NormalizeOptionalAudience(request.Audience.UserIDs, request.Audience.RoleIDs)
	if err != nil {
		return nil, domain.AudienceSnapshot{}, apperrors.Params(err.Error())
	}
	if len(audience.UserIDs) == 0 && len(audience.RoleIDs) == 0 && externalRecipientCount == 0 {
		return nil, domain.AudienceSnapshot{}, apperrors.Params("通知至少需要一个站内信受众或第三方收件人")
	}
	audienceJSON, err := audience.JSON()
	if err != nil {
		return nil, domain.AudienceSnapshot{}, err
	}
	item := &domain.LogicalNotification{
		ID:             s.nextID(),
		NotificationID: "ntf_" + s.nextStringID(),
		ScopeID:        strings.TrimSpace(s.scopeID),
		EventKey:       strings.TrimSpace(request.EventKey),
		IdempotencyKey: strings.TrimSpace(request.IdempotencyKey),
		AudienceJSON:   audienceJSON,
		Category:       strings.ToUpper(defaultNotificationCategory(request.Category)),
		Priority:       strings.ToUpper(defaultNotificationPriority(request.Priority)),
		Mandatory:      request.Mandatory,
		Title:          strings.TrimSpace(request.Title),
		Content:        strings.TrimSpace(request.Content),
		DeepLink:       strings.TrimSpace(request.DeepLink),
		TraceID:        strings.TrimSpace(request.TraceID),
		CreatorID:      notificationCreatorID(request.CreatorID),
	}
	if request.OrganizationID > 0 {
		organizationID := request.OrganizationID
		item.OrganizationID = &organizationID
	}
	if item.DeepLink != "" {
		if action := legacyDefaultAction(item.Category, item.DeepLink); action != nil {
			item.ActionType = string(action.Type)
			item.ActionLabelSnapshot = action.Label
			item.ActionStyle = string(action.Style)
		}
	}
	if request.ScheduleAt != nil {
		value := request.ScheduleAt.UTC()
		item.ScheduleAt = &value
	}
	if request.ExpiresAt != nil {
		value := request.ExpiresAt.UTC()
		item.ExpiresAt = &value
	}
	if strings.EqualFold(item.EventKey, domain.SceneChallengeOTP) || item.Category == "SECRET_EPHEMERAL" {
		return nil, domain.AudienceSnapshot{}, apperrors.Params("SECRET_EPHEMERAL 通知不得写入站内信")
	}
	if err := domain.ValidateLogicalNotification(*item); err != nil {
		return nil, domain.AudienceSnapshot{}, apperrors.Params(err.Error())
	}
	if isScheduled(item.ScheduleAt, s.now()) {
		item.Status = domain.NotificationStatusScheduled
	} else if audience.HasDeferredAudience() || len(audience.UserIDs) > inlineRecipientLimit {
		item.Status = domain.NotificationStatusAccepted
	} else {
		item.Status = domain.NotificationStatusMaterialized
	}
	return item, audience, nil
}

func (s *Service) createMaterializationTask(ctx context.Context, item *domain.LogicalNotification, audience domain.AudienceSnapshot) error {
	nextRunAt := s.now().UTC()
	if item.ScheduleAt != nil && item.ScheduleAt.After(nextRunAt) {
		nextRunAt = item.ScheduleAt.UTC()
	}
	created, err := s.repo.CreateMaterializationTask(ctx, &domain.MaterializationTask{
		ID:             s.nextID(),
		TaskID:         "ntf_task_" + s.nextStringID(),
		NotificationID: item.ID,
		ScopeID:        item.ScopeID,
		AudienceJSON:   item.AudienceJSON,
		Cursor:         encodeMaterializationCursor(materializationCursor{}),
		Status:         domain.TaskStatusPending,
		NextRunAt:      nextRunAt,
	})
	if err != nil {
		return err
	}
	if !created {
		return fmt.Errorf("notification materialization task already exists for new notification")
	}
	return nil
}

func (s *Service) newRecipients(item *domain.LogicalNotification, userIDs []int64) []domain.Recipient {
	unique := uniquePositiveUserIDs(userIDs)
	result := make([]domain.Recipient, 0, len(unique))
	for _, userID := range unique {
		result = append(result, domain.Recipient{
			ID:                  s.nextID(),
			RecipientID:         "nrc_" + s.nextStringID(),
			NotificationID:      item.ID,
			ScopeID:             item.ScopeID,
			UserID:              userID,
			EventKey:            item.EventKey,
			Category:            item.Category,
			Priority:            item.Priority,
			Mandatory:           item.Mandatory,
			Title:               item.Title,
			Content:             item.Content,
			DeepLink:            item.DeepLink,
			ActionType:          item.ActionType,
			ActionLabelSnapshot: item.ActionLabelSnapshot,
			ActionStyle:         item.ActionStyle,
			ExpiresAt:           item.ExpiresAt,
			MailboxVersion:      0,
		})
	}
	return result
}

func (s *Service) materializeTask(ctx context.Context, scopeID string, taskID int64) error {
	task, claimed, err := s.repo.TryClaimMaterializationTask(ctx, scopeID, taskID, materializationLeaseOwner, s.now())
	if err != nil || !claimed || task == nil {
		return err
	}
	if task.ScopeID != scopeID {
		return fmt.Errorf("notification materialization lease is outside the configured scope")
	}
	var materializationErr error
	materializationErr = s.withinTx(ctx, func(txCtx context.Context) error {
		item, err := s.repo.FindLogicalNotificationByID(txCtx, task.NotificationID)
		if err != nil {
			return err
		}
		if item == nil {
			return fmt.Errorf("logical notification %d does not exist", task.NotificationID)
		}
		if item.ScopeID != scopeID || item.ScopeID != task.ScopeID {
			return fmt.Errorf("notification materialization task ownership does not match its logical notification")
		}
		audience, err := domain.ParseAudienceSnapshot(task.AudienceJSON)
		if err != nil {
			return err
		}
		cursor, err := decodeMaterializationCursor(task.Cursor)
		if err != nil {
			return err
		}
		userIDs, nextCursor, done, err := s.nextMaterializationBatch(txCtx, audience, cursor, materializationRecipientLimit)
		if err != nil {
			return err
		}
		createdRecipients, err := s.repo.InsertInboxRecipients(txCtx, s.newRecipients(item, userIDs))
		if err != nil {
			return err
		}
		if err := s.appendInboxChangedIntents(txCtx, createdRecipients, true); err != nil {
			return err
		}
		status := domain.TaskStatusPending
		nextRunAt := s.now().UTC()
		if done {
			status = domain.TaskStatusDone
		}
		updated, err := s.repo.AdvanceMaterializationTask(txCtx, scopeID, task.ID, task.LeaseToken, encodeMaterializationCursor(nextCursor), status, task.MaterializedCount+int64(len(createdRecipients)), nextRunAt)
		if err != nil {
			return err
		}
		if !updated {
			return fmt.Errorf("notification materialization task lease was superseded")
		}
		if done {
			return s.repo.MarkLogicalNotificationMaterialized(txCtx, item.ID)
		}
		return nil
	})
	if materializationErr == nil {
		return nil
	}
	retryCount := task.RetryCount + 1
	status := domain.TaskStatusPending
	nextRunAt := s.now().Add(backoff(retryCount)).UTC()
	if retryCount >= 10 {
		status = domain.TaskStatusFailed
	}
	if _, err := s.repo.FailMaterializationTask(ctx, scopeID, task.ID, task.LeaseToken, status, materializationErr.Error(), retryCount, nextRunAt); err != nil {
		return err
	}
	return materializationErr
}

func (s *Service) nextMaterializationBatch(ctx context.Context, audience domain.AudienceSnapshot, cursor materializationCursor, limit int) ([]int64, materializationCursor, bool, error) {
	if limit <= 0 {
		limit = materializationRecipientLimit
	}
	result := make([]int64, 0, limit)
	for cursor.DirectOffset < len(audience.UserIDs) && len(result) < limit {
		result = append(result, audience.UserIDs[cursor.DirectOffset])
		cursor.DirectOffset++
	}
	for len(result) < limit && cursor.DirectOffset >= len(audience.UserIDs) && cursor.RoleIndex < len(audience.RoleIDs) {
		if s.audiences == nil {
			return nil, cursor, false, fmt.Errorf("notification audience resolver is not configured")
		}
		remaining := limit - len(result)
		userIDs, err := s.audiences.ListActiveUserIDsByRoleIDPage(ctx, audience.RoleIDs[cursor.RoleIndex], cursor.RoleAfterUserID, remaining)
		if err != nil {
			return nil, cursor, false, err
		}
		userIDs = uniquePositiveUserIDs(userIDs)
		if len(userIDs) == 0 {
			cursor.RoleIndex++
			cursor.RoleAfterUserID = 0
			continue
		}
		result = append(result, userIDs...)
		cursor.RoleAfterUserID = userIDs[len(userIDs)-1]
		if len(userIDs) < remaining {
			cursor.RoleIndex++
			cursor.RoleAfterUserID = 0
		}
		if len(result) >= limit {
			break
		}
	}
	return uniquePositiveUserIDs(result), cursor, cursor.DirectOffset >= len(audience.UserIDs) && cursor.RoleIndex >= len(audience.RoleIDs), nil
}

func (s *Service) findInboxRecipient(ctx context.Context, userID int64, recipientID string) (*domain.Recipient, error) {
	if err := s.requireInboxOwner(userID); err != nil {
		return nil, err
	}
	item, err := s.repo.FindInboxRecipient(ctx, s.scopeID, userID, strings.TrimSpace(recipientID))
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, apperrors.NotFound("通知不存在")
	}
	return item, nil
}

func (s *Service) requireInboxOwner(userID int64) error {
	if s == nil || s.repo == nil || strings.TrimSpace(s.scopeID) == "" {
		return fmt.Errorf("notification inbox service is not configured")
	}
	if userID <= 0 {
		return apperrors.Unauthorized("未登录或登录信息失效")
	}
	return nil
}

func applyIdempotentReceipt(existing, incoming *domain.LogicalNotification, receipt *facade.PublishReceipt) error {
	if existing == nil || incoming == nil || receipt == nil {
		return fmt.Errorf("notification idempotency state is invalid")
	}
	if existing.RequestFingerprint != incoming.RequestFingerprint {
		return apperrors.ObjectState("幂等键与既有请求不一致").WithDetails(map[string]string{
			"reasonCode": "IDEMPOTENCY_CONFLICT",
		})
	}
	receipt.NotificationID = existing.NotificationID
	receipt.Status = existing.Status
	receipt.MaterializationStatus = materializationStatus(existing.Status != domain.NotificationStatusMaterialized, existing.Status)
	receipt.Duplicate = true
	return nil
}

func materializationStatus(deferred bool, status string) string {
	if status == domain.NotificationStatusScheduled {
		return domain.NotificationStatusScheduled
	}
	if deferred {
		return domain.NotificationStatusAccepted
	}
	return domain.NotificationStatusMaterialized
}

func defaultNotificationCategory(value string) string {
	if strings.TrimSpace(value) == "" {
		return "GENERAL"
	}
	return value
}

func defaultNotificationPriority(value string) string {
	if strings.TrimSpace(value) == "" {
		return "NORMAL"
	}
	return value
}

func notificationCreatorID(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	return &value
}

func isScheduled(scheduleAt *time.Time, now time.Time) bool {
	return scheduleAt != nil && scheduleAt.After(now.UTC())
}

func uniquePositiveUserIDs(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func mapInboxListItem(item domain.Recipient) facade.InboxListItem {
	return facade.InboxListItem{
		RecipientID:    item.RecipientID,
		Title:          sanitizeInboxTitle(item.Title),
		Summary:        inboxSummary(item.Content),
		FirstSeenAt:    item.FirstSeenAt,
		ReadAt:         item.ReadAt,
		ArchivedAt:     item.ArchivedAt,
		MailboxVersion: strconv.FormatInt(item.MailboxVersion, 10),
		CreateTime:     item.CreateTime,
		UpdateTime:     item.UpdateTime,
	}
}

func mapInboxDetail(item domain.Recipient) facade.InboxDetail {
	detail := facade.InboxDetail{
		RecipientID:    item.RecipientID,
		Title:          sanitizeInboxTitle(item.Title),
		Content:        sanitizeInboxDetailContent(item.Content),
		DeepLink:       safeInboxDeepLink(item.DeepLink),
		FirstSeenAt:    item.FirstSeenAt,
		ReadAt:         item.ReadAt,
		ArchivedAt:     item.ArchivedAt,
		MailboxVersion: strconv.FormatInt(item.MailboxVersion, 10),
		CreateTime:     item.CreateTime,
		UpdateTime:     item.UpdateTime,
	}
	if link := safeInboxDeepLink(item.DeepLink); link != "" && item.ActionType == domain.NotificationActionTypeOpenInternal && item.ActionStyle == domain.NotificationActionStylePrimary && strings.TrimSpace(item.ActionLabelSnapshot) != "" {
		detail.PrimaryAction = &facade.NotificationAction{Type: item.ActionType, Label: item.ActionLabelSnapshot, Link: link, Style: item.ActionStyle}
	}
	return detail
}

func legacyDefaultAction(category, deepLink string) *domain.NotificationAction {
	link, err := domain.ParseInternalDeepLink(deepLink)
	if err != nil {
		return nil
	}
	label := ""
	switch strings.ToUpper(strings.TrimSpace(category)) {
	case "WORKFORCE_TASK":
		label = "查看任务"
	case "APPROVAL":
		label = "去处理"
	case "SECURITY":
		label = "查看详情"
	default:
		return nil
	}
	action, err := domain.NewPrimaryInternalAction(label, link)
	if err != nil {
		return nil
	}
	return &action
}

func mapInboxPreviewItem(item domain.Recipient) facade.InboxPreviewItem {
	return facade.InboxPreviewItem{
		RecipientID:    item.RecipientID,
		Title:          sanitizeInboxTitle(item.Title),
		Summary:        inboxSummary(item.Content),
		MailboxVersion: strconv.FormatInt(item.MailboxVersion, 10),
		CreateTime:     item.CreateTime,
	}
}

// inboxSummary intentionally does not derive an excerpt from the stored body.
// The title is the contextual cue in a closed card; the body remains available
// only after the user explicitly opens its detail.
func inboxSummary(_ string) string {
	return "打开查看详情"
}

// sanitizeInboxDetailContent removes control characters that cannot be
// meaningfully rendered as plain text while preserving normal paragraphs and
// tabs. The UI must still render this value as text, never injected HTML.
func sanitizeInboxDetailContent(content string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t':
			return r
		case 127:
			return -1
		default:
			if r < 32 {
				return -1
			}
			return r
		}
	}, content))
}

func sanitizeInboxTitle(title string) string {
	normalized := strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, title)), " ")
	if normalized == "" {
		return "消息"
	}
	return normalized
}

func safeInboxDeepLink(value string) string {
	if !domain.IsSafeInternalDeepLink(value) {
		return ""
	}
	return strings.TrimSpace(value)
}

func (s *Service) appendInboxChangedIntents(ctx context.Context, recipients []domain.Recipient, newUnread bool) error {
	ordered := append([]domain.Recipient(nil), recipients...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].ScopeID == ordered[j].ScopeID {
			if ordered[i].UserID == ordered[j].UserID {
				return ordered[i].MailboxVersion < ordered[j].MailboxVersion
			}
			return ordered[i].UserID < ordered[j].UserID
		}
		return ordered[i].ScopeID < ordered[j].ScopeID
	})
	events := make([]domain.OutboxEvent, 0, len(ordered))
	for _, recipient := range ordered {
		if strings.TrimSpace(recipient.ScopeID) == "" || recipient.UserID <= 0 || recipient.MailboxVersion <= 0 {
			return fmt.Errorf("notification inbox changed intent is invalid")
		}
		intent := domain.InboxChangedIntent{
			ScopeID:        recipient.ScopeID,
			UserID:         recipient.UserID,
			ChangeSequence: recipient.MailboxVersion,
			NewUnread:      newUnread,
		}
		events = append(events, domain.OutboxEvent{
			ID:            s.nextID(),
			EventID:       fmt.Sprintf("notification-inbox-changed:%s:%d:%d", recipient.ScopeID, recipient.UserID, recipient.MailboxVersion),
			ScopeID:       recipient.ScopeID,
			EventType:     domain.OutboxEventNotificationInboxChanged,
			AggregateType: domain.OutboxAggregateNotification,
			AggregateID:   recipient.RecipientID,
			Payload:       mustJSON(intent),
			Status:        "PENDING",
		})
	}
	if len(events) == 0 {
		return nil
	}
	if len(events) == 1 {
		return s.repo.AppendOutbox(ctx, &events[0])
	}
	batch, ok := s.repo.(outboxBatchRepository)
	if !ok {
		return fmt.Errorf("notification outbox batch repository is not configured")
	}
	return batch.AppendOutboxBatch(ctx, events)
}

func (s *Service) newMailboxKey() string {
	return "mbx_" + uuid.NewString()
}

func (s *Service) decodeOptionalInboxChangeToken(ctx context.Context, raw string, userID int64) (int64, error) {
	value, _, err := s.decodeOptionalInboxChangeTokenWithPresence(ctx, raw, userID)
	return value, err
}

func (s *Service) decodeOptionalInboxChangeTokenWithPresence(ctx context.Context, raw string, userID int64) (int64, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, false, nil
	}
	value, err := s.decodeInboxChangeToken(ctx, raw, s.scopeID, userID)
	if err != nil {
		return 0, true, err
	}
	return value, true, nil
}

func (s *Service) inboxResyncResult() *facade.InboxChanges {
	return &facade.InboxChanges{ResyncRequired: true, ServerTime: s.now().UTC()}
}

func normalizeInboxPageSize(value int) int {
	if value <= 0 {
		return 20
	}
	if value > 100 {
		return 100
	}
	return value
}

func normalizeInboxPreviewLimit(value int) int {
	if value <= 0 {
		return 5
	}
	if value > 5 {
		return 5
	}
	return value
}

func normalizeInboxChangeLimit(value int) int {
	if value <= 0 {
		return 50
	}
	if value > 100 {
		return 100
	}
	return value
}

func minInbox(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func parseExpectedMailboxVersion(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, apperrors.Params("expectedMailboxVersion 无效")
	}
	return value, nil
}

func mailboxVersionConflict(current int64) error {
	return apperrors.ObjectState("收件箱状态已变化").WithDetails(map[string]string{
		"reasonCode":     "MAILBOX_VERSION_CONFLICT",
		"mailboxVersion": strconv.FormatInt(current, 10),
	})
}

type materializationCursor struct {
	DirectOffset    int   `json:"directOffset"`
	RoleIndex       int   `json:"roleIndex"`
	RoleAfterUserID int64 `json:"roleAfterUserId"`
}

func encodeMaterializationCursor(cursor materializationCursor) string {
	raw, _ := json.Marshal(cursor)
	return string(raw)
}

func decodeMaterializationCursor(raw string) (materializationCursor, error) {
	if strings.TrimSpace(raw) == "" {
		return materializationCursor{}, nil
	}
	var cursor materializationCursor
	if err := json.Unmarshal([]byte(raw), &cursor); err != nil || cursor.DirectOffset < 0 || cursor.RoleIndex < 0 || cursor.RoleAfterUserID < 0 {
		return materializationCursor{}, fmt.Errorf("notification materialization cursor is invalid")
	}
	return cursor, nil
}
