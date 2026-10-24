package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/domain"
)

var ErrNotFound = errors.New("TASK_NOTIFICATION_NOT_FOUND")

func (s *Store) TaskCreator(ctx context.Context, tenantID, taskID int64) (int64, error) {
	if s == nil || s.db == nil || tenantID <= 0 || taskID <= 0 {
		return 0, ErrNotFound
	}
	var creator int64
	if err := s.db.QueryRow(ctx, `SELECT created_by FROM workforce_tasks WHERE tenant_id=$1 AND id=$2`, tenantID, taskID).Scan(&creator); errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	} else if err != nil {
		return 0, err
	}
	if creator <= 0 {
		return 0, domain.ErrRecipientInvalid
	}
	return creator, nil
}

func (s *Store) GetSubscription(ctx context.Context, tenantID, taskID, subscriberUserID int64) (domain.Subscription, error) {
	var item domain.Subscription
	var eventsJSON []byte
	err := s.db.QueryRow(ctx, `SELECT id,tenant_id,task_id,subscriber_user_id,events_json,enabled,version,created_by,updated_by,created_at,updated_at
		FROM workforce_task_notification_subscriptions
		WHERE tenant_id=$1 AND task_id=$2 AND subscriber_user_id=$3`, tenantID, taskID, subscriberUserID).
		Scan(&item.ID, &item.TenantID, &item.TaskID, &item.SubscriberUserID, &eventsJSON, &item.Enabled, &item.Version, &item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Subscription{}, ErrNotFound
	}
	if err != nil {
		return domain.Subscription{}, err
	}
	if err = json.Unmarshal(eventsJSON, &item.Events); err != nil {
		return domain.Subscription{}, fmt.Errorf("decode Task notification events: %w", err)
	}
	if item.Channels, err = s.loadSubscriptionChannels(ctx, s.db, item.ID); err != nil {
		return domain.Subscription{}, err
	}
	if err = item.Validate(); err != nil {
		return domain.Subscription{}, err
	}
	return item, nil
}

func (s *Store) UpdateSubscription(ctx context.Context, tenantID, taskID, subscriberUserID, actorUserID, expectedVersion int64, command domain.SubscriptionCommand) (domain.Subscription, error) {
	if s == nil || s.db == nil || actorUserID <= 0 || expectedVersion <= 0 {
		return domain.Subscription{}, domain.ErrSubscriptionConflict
	}
	item, err := domain.NewSubscription(tenantID, taskID, subscriberUserID, command)
	if err != nil {
		return domain.Subscription{}, err
	}
	eventsJSON, _ := json.Marshal(domain.CanonicalEvents(item.Events))
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.Subscription{}, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `UPDATE workforce_task_notification_subscriptions
		SET events_json=$6,enabled=$7,version=version+1,updated_by=$5,updated_at=CURRENT_TIMESTAMP
		WHERE tenant_id=$1 AND task_id=$2 AND subscriber_user_id=$3 AND version=$4
		RETURNING id,tenant_id,task_id,subscriber_user_id,enabled,version,created_by,updated_by,created_at,updated_at`,
		tenantID, taskID, subscriberUserID, expectedVersion, actorUserID, eventsJSON, item.Enabled).
		Scan(&item.ID, &item.TenantID, &item.TaskID, &item.SubscriberUserID, &item.Enabled, &item.Version, &item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Subscription{}, domain.ErrSubscriptionConflict
	}
	if err != nil {
		return domain.Subscription{}, err
	}
	if err = s.replaceSubscriptionChannels(ctx, tx, item); err != nil {
		return domain.Subscription{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Subscription{}, err
	}
	return item, nil
}

func (s *Store) ListRecords(ctx context.Context, tenantID, taskID, subscriberUserID int64, limit int) ([]domain.Record, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `SELECT id,tenant_id,task_id,task_turn_id,source_type,source_id,fact_type,reason_code,focus,
		COALESCE(subscription_id,0),subscriber_user_id,COALESCE(subscription_version,0),task_name_snapshot,turn_number,summary_snapshot,
		title_snapshot,content_snapshot,publication_contract_version,scene_code,template_variables_json::text,deep_link,idempotency_identity,request_digest,status,publish_attempts,next_retry_at,
		notification_receipt_identity,warning_code,last_error_code,occurred_at,published_at,created_at,updated_at
		FROM workforce_task_notification_records
		WHERE tenant_id=$1 AND task_id=$2 AND subscriber_user_id=$3
		ORDER BY occurred_at DESC,id DESC LIMIT $4`, tenantID, taskID, subscriberUserID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.Record, 0)
	for rows.Next() {
		var item domain.Record
		if err = rows.Scan(&item.ID, &item.TenantID, &item.TaskID, &item.TaskTurnID, &item.SourceType, &item.SourceID, &item.FactType,
			&item.ReasonCode, &item.Focus, &item.SubscriptionID, &item.SubscriberUserID, &item.SubscriptionVersion,
			&item.TaskNameSnapshot, &item.TurnNumber, &item.SummarySnapshot, &item.TitleSnapshot, &item.ContentSnapshot,
			&item.PublicationContractVersion, &item.SceneCode, &item.TemplateVariablesJSON,
			&item.DeepLinkSnapshot, &item.IdempotencyIdentity, &item.RequestDigest, &item.Status, &item.PublishAttempts,
			&item.NextRetryAt, &item.NotificationReceiptIdentity, &item.WarningCode, &item.LastErrorCode, &item.OccurredAt,
			&item.PublishedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
