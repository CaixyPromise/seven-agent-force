package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/domain"
)

type IDGenerator interface{ NextID() int64 }

type transaction interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Store struct {
	db    *pgxpool.Pool
	idGen IDGenerator
}

type Publication struct {
	OutboxID int64
	Record   domain.Record
}

func NewStore(db *pgxpool.Pool, idGen IDGenerator) *Store {
	return &Store{db: db, idGen: idGen}
}

func (s *Store) CreateSubscription(ctx context.Context, tx pgx.Tx, tenantID, taskID, subscriberUserID int64, command domain.SubscriptionCommand) (domain.Subscription, bool, error) {
	return s.createSubscription(ctx, tx, tenantID, taskID, subscriberUserID, command)
}

func (s *Store) createSubscription(ctx context.Context, tx transaction, tenantID, taskID, subscriberUserID int64, command domain.SubscriptionCommand) (domain.Subscription, bool, error) {
	if tx == nil || s == nil || s.idGen == nil {
		return domain.Subscription{}, false, domain.ErrFactInvalid
	}
	item, err := domain.NewSubscription(tenantID, taskID, subscriberUserID, command)
	if err != nil {
		return domain.Subscription{}, false, err
	}
	item.ID = s.idGen.NextID()
	item.CreatedBy = subscriberUserID
	item.UpdatedBy = subscriberUserID
	if item.ID <= 0 {
		return domain.Subscription{}, false, domain.ErrFactInvalid
	}
	eventsJSON, _ := json.Marshal(domain.CanonicalEvents(item.Events))
	expectedEvents := domain.CanonicalEvents(item.Events)
	expectedChannels := domain.CanonicalChannels(item.Channels)
	expectedEnabled := item.Enabled
	err = tx.QueryRow(ctx, `INSERT INTO workforce_task_notification_subscriptions
		(id,tenant_id,task_id,subscriber_user_id,events_json,enabled,version,created_by,updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,1,$4,$4)
		ON CONFLICT (tenant_id,task_id,subscriber_user_id) DO NOTHING
		RETURNING created_at,updated_at`, item.ID, tenantID, taskID, subscriberUserID, eventsJSON, item.Enabled).
		Scan(&item.CreatedAt, &item.UpdatedAt)
	if err == nil {
		if err = s.replaceSubscriptionChannels(ctx, tx, item); err != nil {
			return domain.Subscription{}, false, err
		}
		return item, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Subscription{}, false, fmt.Errorf("create default Task notification subscription: %w", err)
	}
	var storedEvents []byte
	err = tx.QueryRow(ctx, `SELECT id,events_json,enabled,version,created_by,updated_by,created_at,updated_at
		FROM workforce_task_notification_subscriptions
		WHERE tenant_id=$1 AND task_id=$2 AND subscriber_user_id=$3`, tenantID, taskID, subscriberUserID).
		Scan(&item.ID, &storedEvents, &item.Enabled, &item.Version, &item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return domain.Subscription{}, false, fmt.Errorf("load replayed Task notification subscription: %w", err)
	}
	if err = json.Unmarshal(storedEvents, &item.Events); err != nil {
		return domain.Subscription{}, false, fmt.Errorf("decode Task notification events: %w", err)
	}
	if item.Channels, err = s.loadSubscriptionChannels(ctx, tx, item.ID); err != nil {
		return domain.Subscription{}, false, err
	}
	if err = item.Validate(); err != nil {
		return domain.Subscription{}, false, domain.ErrSubscriptionConflict
	}
	if !slicesEqual(item.Events, expectedEvents) || !channelsEqual(item.Channels, expectedChannels) || item.Enabled != expectedEnabled {
		return domain.Subscription{}, false, domain.ErrSubscriptionConflict
	}
	return item, true, nil
}

func (s *Store) RecordTransition(ctx context.Context, tx pgx.Tx, transition domain.TaskNotificationTransition) (domain.TransitionDisposition, error) {
	decision, err := domain.ClassifyTransition(transition)
	if err != nil {
		return "", err
	}
	if decision.Disposition == domain.DispositionNone {
		return decision.Disposition, nil
	}
	fact := domain.TaskNotificationFact{
		TenantID: transition.TenantID, TaskID: transition.TaskID, TaskTurnID: transition.TaskTurnID,
		SourceType: transition.SourceType, SourceID: transition.SourceID, SourceOccurrence: transition.SourceOccurrence,
		FactIdentityVersion: 2, FactType: decision.FactType, ReasonCode: transition.ReasonCode, Focus: decision.Focus,
		AttentionUserID: transition.AttentionUserID, MentionAttention: decision.MentionUser,
		SceneCode: decision.SceneCode, Summary: transition.Summary, OccurredAt: transition.OccurredAt,
	}
	if err = s.recordFact(ctx, tx, fact); err != nil {
		return "", err
	}
	return decision.Disposition, nil
}

func (s *Store) recordFact(ctx context.Context, tx transaction, fact domain.TaskNotificationFact) error {
	if tx == nil || s == nil || s.idGen == nil {
		return domain.ErrFactInvalid
	}
	if err := fact.Validate(); err != nil {
		return err
	}
	var taskName string
	var currentTurnID int64
	var creatorUserID int64
	var turnNumber int
	err := tx.QueryRow(ctx, `SELECT t.name,t.current_task_turn_id,t.created_by,tt.turn_number
		FROM workforce_tasks t
		JOIN workforce_task_turns tt ON tt.id=$3 AND tt.task_id=t.id AND tt.tenant_id=t.tenant_id
		WHERE t.id=$2 AND t.tenant_id=$1
		FOR SHARE OF t,tt`, fact.TenantID, fact.TaskID, fact.TaskTurnID).
		Scan(&taskName, &currentTurnID, &creatorUserID, &turnNumber)
	if errors.Is(err, pgx.ErrNoRows) || currentTurnID != fact.TaskTurnID {
		return domain.ErrStaleTurn
	}
	if err != nil {
		return fmt.Errorf("load Task notification scope: %w", err)
	}
	taskName = strings.TrimSpace(taskName)
	if taskName == "" || utf8.RuneCountInString(taskName) > 120 {
		return domain.ErrFactInvalid
	}
	var subscribersJSON []byte
	err = tx.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(jsonb_build_object(
		'id',id::TEXT,'subscriberUserId',subscriber_user_id::TEXT,'version',version::TEXT,
		'events',events_json,'channels',(SELECT COALESCE(jsonb_agg(jsonb_build_object('kind',c.channel_kind,'routeId',COALESCE(c.group_route_id,0)::TEXT) ORDER BY c.channel_kind,c.group_route_id),'[]'::jsonb) FROM workforce_task_notification_subscription_channels c WHERE c.subscription_id=workforce_task_notification_subscriptions.id),'enabled',enabled) ORDER BY id),'[]'::jsonb)
		FROM workforce_task_notification_subscriptions
		WHERE tenant_id=$1 AND task_id=$2`, fact.TenantID, fact.TaskID).
		Scan(&subscribersJSON)
	if err != nil {
		return fmt.Errorf("load Task notification subscribers: %w", err)
	}
	var subscribers []subscriptionWire
	if err = json.Unmarshal(subscribersJSON, &subscribers); err != nil {
		return fmt.Errorf("decode Task notification subscribers: %w", err)
	}
	if len(subscribers) == 0 {
		subscribers = append(subscribers, subscriptionWire{SubscriberUserID: strconv.FormatInt(creatorUserID, 10), Enabled: false})
	}
	for _, subscriber := range subscribers {
		subscriptionID := int64(0)
		var parseErr error
		if subscriber.ID != "" {
			subscriptionID, parseErr = parsePositiveInt64(subscriber.ID)
			if parseErr != nil {
				return domain.ErrRecipientInvalid
			}
		}
		userID, parseErr := parsePositiveInt64(subscriber.SubscriberUserID)
		if parseErr != nil {
			return domain.ErrRecipientInvalid
		}
		version := int64(0)
		if subscriber.Version != "" {
			version, parseErr = parsePositiveInt64(subscriber.Version)
			if parseErr != nil {
				return domain.ErrSubscriptionConflict
			}
		}
		status := domain.RecordSuppressed
		if subscriber.Enabled && containsEvent(subscriber.Events, fact.FactType) && len(subscriber.Channels) > 0 {
			status = domain.RecordPending
		}
		recordChannels := make([]domain.RecordChannel, 0, len(subscriber.Channels))
		for _, channel := range subscriber.Channels {
			mentionUserID := int64(0)
			if channel.Type == domain.ChannelGroupRoute && (fact.FactType == domain.FactActionRequired || fact.FactType == domain.FactInterrupted) {
				mentionUserID = creatorUserID
			}
			recordChannels = append(recordChannels, domain.RecordChannel{Type: channel.Type, GroupRouteID: channel.GroupRouteID, MentionUserID: mentionUserID})
		}
		record := domain.Record{
			ID: s.idGen.NextID(), TenantID: fact.TenantID, TaskID: fact.TaskID, TaskTurnID: fact.TaskTurnID,
			SourceType: fact.SourceType, SourceID: fact.SourceID, SourceOccurrence: fact.SourceOccurrence, FactIdentityVersion: fact.FactIdentityVersion,
			FactType: fact.FactType, ReasonCode: fact.ReasonCode, Focus: fact.Focus, SceneCode: fact.SceneCode,
			SubscriptionID: subscriptionID, SubscriberUserID: userID, SubscriptionVersion: version,
			TaskNameSnapshot: taskName, TurnNumber: turnNumber, SummarySnapshot: strings.TrimSpace(fact.Summary), Status: status,
			OccurredAt: fact.OccurredAt.UTC(), Channels: recordChannels,
		}
		if record.FactIdentityVersion == 0 {
			record.FactIdentityVersion = 1
		}
		if fact.AttentionUserID > 0 && fact.AttentionUserID != userID {
			record.Recipients = append(record.Recipients,
				domain.RecordRecipient{UserID: userID, Role: domain.RecipientSubscriber, PersonalChannelPolicy: domain.PersonalChannelsTaskSubscription},
				domain.RecordRecipient{UserID: fact.AttentionUserID, Role: domain.RecipientAttention, PersonalChannelPolicy: domain.PersonalChannelsUserPreference, InAppRequired: true})
		} else {
			record.Recipients = append(record.Recipients, domain.RecordRecipient{UserID: userID, Role: domain.RecipientSubscriber, PersonalChannelPolicy: domain.PersonalChannelsTaskSubscription})
		}
		if fact.MentionAttention {
			mentionID := fact.AttentionUserID
			if mentionID == 0 {
				mentionID = creatorUserID
			}
			for index := range record.Channels {
				if record.Channels[index].Type == domain.ChannelGroupRoute {
					record.Channels[index].MentionUserID = mentionID
				}
			}
		} else if record.FactIdentityVersion >= 2 {
			for index := range record.Channels {
				record.Channels[index].MentionUserID = 0
			}
		}
		if record.ID <= 0 {
			return domain.ErrFactInvalid
		}
		if err = domain.FreezePresentation(&record); err != nil {
			return err
		}
		record.RequestDigest = domain.RecordRequestDigest(record)
		inserted, insertErr := s.insertRecord(ctx, tx, record)
		if insertErr != nil {
			return insertErr
		}
		if !inserted || record.Status == domain.RecordSuppressed {
			continue
		}
		if err = s.insertRecordChannels(ctx, tx, record); err != nil {
			return err
		}
		if err = s.insertRecordRecipients(ctx, tx, record); err != nil {
			return err
		}
		if err = s.insertOutbox(ctx, tx, record); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) insertRecordRecipients(ctx context.Context, tx transaction, record domain.Record) error {
	for _, recipient := range record.Recipients {
		id := s.idGen.NextID()
		if id <= 0 || recipient.UserID <= 0 {
			return domain.ErrRecipientInvalid
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workforce_task_notification_record_recipients
			(id,tenant_id,record_id,user_id,recipient_role,personal_channel_policy,in_app_required)
			VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, id, record.TenantID, record.ID, recipient.UserID, recipient.Role, recipient.PersonalChannelPolicy, recipient.InAppRequired); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) replaceSubscriptionChannels(ctx context.Context, tx transaction, item domain.Subscription) error {
	if _, err := tx.Exec(ctx, `DELETE FROM workforce_task_notification_subscription_channels WHERE subscription_id=$1`, item.ID); err != nil {
		return err
	}
	for _, channel := range domain.CanonicalChannels(item.Channels) {
		id := s.idGen.NextID()
		if id <= 0 {
			return domain.ErrFactInvalid
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workforce_task_notification_subscription_channels (id,tenant_id,subscription_id,channel_kind,group_route_id,created_by) VALUES ($1,$2,$3,$4,NULLIF($5::BIGINT,0),$6)`, id, item.TenantID, item.ID, channel.Type, channel.GroupRouteID, item.UpdatedBy); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) loadSubscriptionChannels(ctx context.Context, tx transaction, subscriptionID int64) ([]domain.ChannelSelection, error) {
	var body []byte
	err := tx.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(jsonb_build_object('kind',channel_kind,'routeId',COALESCE(group_route_id,0)::TEXT) ORDER BY channel_kind,group_route_id),'[]'::jsonb) FROM workforce_task_notification_subscription_channels WHERE subscription_id=$1`, subscriptionID).Scan(&body)
	if err != nil {
		return nil, err
	}
	var result []domain.ChannelSelection
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) insertRecordChannels(ctx context.Context, tx transaction, record domain.Record) error {
	for _, channel := range record.Channels {
		id := s.idGen.NextID()
		if id <= 0 {
			return domain.ErrFactInvalid
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workforce_task_notification_record_channels (id,tenant_id,record_id,channel_kind,group_route_id,mention_user_id) VALUES ($1,$2,$3,$4,NULLIF($5::BIGINT,0),NULLIF($6::BIGINT,0)) ON CONFLICT DO NOTHING`, id, record.TenantID, record.ID, channel.Type, channel.GroupRouteID, channel.MentionUserID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) insertRecord(ctx context.Context, tx transaction, record domain.Record) (bool, error) {
	var insertedID int64
	err := tx.QueryRow(ctx, `INSERT INTO workforce_task_notification_records
		(id,tenant_id,task_id,task_turn_id,source_type,source_id,source_occurrence,fact_identity_version,fact_type,reason_code,focus,
		 subscription_id,subscriber_user_id,subscription_version,task_name_snapshot,turn_number,summary_snapshot,
		 title_snapshot,content_snapshot,publication_contract_version,scene_code,template_variables_json,deep_link,idempotency_identity,request_digest,status,occurred_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NULLIF($12::BIGINT,0),$13,NULLIF($14::BIGINT,0),$15,$16,$17,$18,$19,$20,$21,$22::jsonb,$23,$24,$25,$26,$27)
		ON CONFLICT (tenant_id,task_id,task_turn_id,fact_type,source_type,source_id,source_occurrence,subscriber_user_id) DO NOTHING
		RETURNING id`, record.ID, record.TenantID, record.TaskID, record.TaskTurnID, record.SourceType, record.SourceID,
		record.SourceOccurrence, record.FactIdentityVersion, record.FactType, record.ReasonCode, record.Focus, record.SubscriptionID, record.SubscriberUserID,
		record.SubscriptionVersion, record.TaskNameSnapshot, record.TurnNumber, record.SummarySnapshot,
		record.TitleSnapshot, record.ContentSnapshot, record.PublicationContractVersion, record.SceneCode, record.TemplateVariablesJSON, record.DeepLinkSnapshot, record.IdempotencyIdentity, record.RequestDigest, record.Status, record.OccurredAt).
		Scan(&insertedID)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("insert Task notification record: %w", err)
	}
	var existingDigest string
	err = tx.QueryRow(ctx, `SELECT request_digest FROM workforce_task_notification_records
		WHERE tenant_id=$1 AND task_id=$2 AND task_turn_id=$3 AND fact_type=$4 AND source_type=$5 AND source_id=$6 AND source_occurrence=$7 AND subscriber_user_id=$8`,
		record.TenantID, record.TaskID, record.TaskTurnID, record.FactType, record.SourceType, record.SourceID, record.SourceOccurrence, record.SubscriberUserID).
		Scan(&existingDigest)
	if err != nil {
		return false, fmt.Errorf("load replayed Task notification record: %w", err)
	}
	if existingDigest != record.RequestDigest {
		return false, domain.ErrIdempotencyConflict
	}
	return false, nil
}

func (s *Store) insertOutbox(ctx context.Context, tx transaction, record domain.Record) error {
	outboxID := s.idGen.NextID()
	if outboxID <= 0 {
		return domain.ErrFactInvalid
	}
	message := domain.OutboxMessage{SchemaVersion: 1, RecordID: record.ID, TaskID: record.TaskID, TaskTurnID: record.TaskTurnID}
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode Task notification outbox: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO workforce_outbox_events
		(id,aggregate_type,aggregate_id,event_type,payload_json,status)
		VALUES ($1,$2,$3,$4,$5,'PENDING')`, outboxID, domain.OutboxAggregate, record.ID, domain.OutboxEventType, payload)
	if err != nil {
		return fmt.Errorf("insert Task notification outbox: %w", err)
	}
	return nil
}

func (s *Store) ClaimPublications(ctx context.Context, leaseToken string, now time.Time, leaseDuration time.Duration, limit int) ([]Publication, error) {
	if s == nil || s.db == nil || strings.TrimSpace(leaseToken) == "" || leaseDuration <= 0 || limit <= 0 || limit > 100 {
		return nil, domain.ErrFactInvalid
	}
	now = now.UTC()
	leaseUntil := now.Add(leaseDuration)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT e.id,
		r.id,r.tenant_id,r.task_id,r.task_turn_id,r.source_type,r.source_id,r.source_occurrence,r.fact_identity_version,r.fact_type,r.reason_code,r.focus,
		r.subscription_id,r.subscriber_user_id,r.subscription_version,r.task_name_snapshot,r.turn_number,r.summary_snapshot,
		r.title_snapshot,r.content_snapshot,r.publication_contract_version,r.scene_code,r.template_variables_json::text,r.deep_link,r.idempotency_identity,r.request_digest,r.status,r.publish_attempts,r.next_retry_at,
		r.notification_receipt_identity,r.warning_code,r.last_error_code,r.occurred_at,r.created_at,r.published_at,r.updated_at
		FROM workforce_outbox_events e
		JOIN workforce_task_notification_records r ON r.id=e.aggregate_id
		WHERE e.aggregate_type=$1 AND e.event_type=$2 AND e.status='PENDING'
		  AND (e.next_retry_at IS NULL OR e.next_retry_at<=$3)
		  AND ((r.status IN ('PENDING','RETRY_WAIT') AND (r.next_retry_at IS NULL OR r.next_retry_at<=$3))
		       OR (r.status='PROCESSING' AND r.lease_until<$3))
		ORDER BY e.created_at,e.id
		FOR UPDATE OF e,r SKIP LOCKED LIMIT $4`, domain.OutboxAggregate, domain.OutboxEventType, now, limit)
	if err != nil {
		return nil, err
	}
	var publications []Publication
	for rows.Next() {
		var item Publication
		if err = rows.Scan(&item.OutboxID,
			&item.Record.ID, &item.Record.TenantID, &item.Record.TaskID, &item.Record.TaskTurnID, &item.Record.SourceType, &item.Record.SourceID, &item.Record.SourceOccurrence, &item.Record.FactIdentityVersion,
			&item.Record.FactType, &item.Record.ReasonCode, &item.Record.Focus, &item.Record.SubscriptionID, &item.Record.SubscriberUserID,
			&item.Record.SubscriptionVersion, &item.Record.TaskNameSnapshot, &item.Record.TurnNumber, &item.Record.SummarySnapshot,
			&item.Record.TitleSnapshot, &item.Record.ContentSnapshot, &item.Record.PublicationContractVersion, &item.Record.SceneCode, &item.Record.TemplateVariablesJSON, &item.Record.DeepLinkSnapshot, &item.Record.IdempotencyIdentity,
			&item.Record.RequestDigest, &item.Record.Status, &item.Record.PublishAttempts, &item.Record.NextRetryAt,
			&item.Record.NotificationReceiptIdentity, &item.Record.WarningCode, &item.Record.LastErrorCode,
			&item.Record.OccurredAt, &item.Record.CreatedAt, &item.Record.PublishedAt, &item.Record.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		publications = append(publications, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for index := range publications {
		var channelsBody []byte
		if err = tx.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(jsonb_build_object('kind',channel_kind,'routeId',COALESCE(group_route_id,0)::TEXT,'mentionUserId',COALESCE(mention_user_id,0)::TEXT) ORDER BY channel_kind,group_route_id),'[]'::jsonb) FROM workforce_task_notification_record_channels WHERE record_id=$1`, publications[index].Record.ID).Scan(&channelsBody); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(channelsBody, &publications[index].Record.Channels); err != nil {
			return nil, err
		}
		var recipientsBody []byte
		if err = tx.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(jsonb_build_object('userId',user_id::TEXT,'role',recipient_role,'personalChannelPolicy',personal_channel_policy,'inAppRequired',in_app_required) ORDER BY user_id),'[]'::jsonb) FROM workforce_task_notification_record_recipients WHERE record_id=$1`, publications[index].Record.ID).Scan(&recipientsBody); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(recipientsBody, &publications[index].Record.Recipients); err != nil {
			return nil, err
		}
		tag, updateErr := tx.Exec(ctx, `UPDATE workforce_task_notification_records
			SET status='PROCESSING',publish_attempts=publish_attempts+1,lease_token=$2,lease_until=$3,updated_at=$4
			WHERE id=$1 AND status IN ('PENDING','RETRY_WAIT','PROCESSING')`, publications[index].Record.ID, leaseToken, leaseUntil, now)
		if updateErr != nil {
			return nil, updateErr
		}
		if tag.RowsAffected() != 1 {
			return nil, domain.ErrPublishRetryable
		}
		publications[index].Record.Status = domain.RecordProcessing
		publications[index].Record.PublishAttempts++
		publications[index].Record.LeaseToken = leaseToken
		publications[index].Record.LeaseUntil = &leaseUntil
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return publications, nil
}

func (s *Store) CompletePublication(ctx context.Context, item Publication, receiptIdentity, warningCode string, now time.Time) error {
	if s == nil || s.db == nil || item.OutboxID <= 0 || item.Record.ID <= 0 || item.Record.LeaseToken == "" || strings.TrimSpace(receiptIdentity) == "" {
		return domain.ErrPublishFailed
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE workforce_task_notification_records
		SET status='PUBLISHED',notification_receipt_identity=$3,warning_code=$4,last_error_code='',published_at=$5,
		    lease_token='',lease_until=NULL,next_retry_at=NULL,updated_at=$5
		WHERE id=$1 AND status='PROCESSING' AND lease_token=$2`, item.Record.ID, item.Record.LeaseToken, receiptIdentity, boundedCode(warningCode), now.UTC())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		var status, existingReceipt string
		if err = tx.QueryRow(ctx, `SELECT status,notification_receipt_identity FROM workforce_task_notification_records WHERE id=$1`, item.Record.ID).Scan(&status, &existingReceipt); err != nil {
			return err
		}
		if status != domain.RecordPublished || existingReceipt != receiptIdentity {
			return domain.ErrPublishFailed
		}
	}
	outboxTag, err := tx.Exec(ctx, `UPDATE workforce_outbox_events SET status='PUBLISHED',published_at=$3,last_error=NULL
		WHERE id=$1 AND aggregate_id=$2 AND aggregate_type=$4 AND status IN ('PENDING','PUBLISHED')`, item.OutboxID, item.Record.ID, now.UTC(), domain.OutboxAggregate)
	if err != nil {
		return err
	}
	if outboxTag.RowsAffected() != 1 {
		return domain.ErrPublishFailed
	}
	return tx.Commit(ctx)
}

func (s *Store) RetryPublication(ctx context.Context, item Publication, code string, now time.Time) error {
	if s == nil || s.db == nil || item.OutboxID <= 0 || item.Record.ID <= 0 || item.Record.LeaseToken == "" {
		return domain.ErrPublishFailed
	}
	now = now.UTC()
	delay := retryDelay(item.Record.PublishAttempts)
	next := now.Add(delay)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE workforce_task_notification_records
		SET status='RETRY_WAIT',next_retry_at=$3,last_error_code=$4,lease_token='',lease_until=NULL,updated_at=$5
		WHERE id=$1 AND status='PROCESSING' AND lease_token=$2`, item.Record.ID, item.Record.LeaseToken, next, boundedCode(code), now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrPublishFailed
	}
	outboxTag, err := tx.Exec(ctx, `UPDATE workforce_outbox_events
		SET attempts=attempts+1,next_retry_at=$3,last_error=$4
		WHERE id=$1 AND aggregate_id=$2 AND aggregate_type=$5 AND status='PENDING'`, item.OutboxID, item.Record.ID, next, boundedCode(code), domain.OutboxAggregate)
	if err != nil {
		return err
	}
	if outboxTag.RowsAffected() != 1 {
		return domain.ErrPublishFailed
	}
	return tx.Commit(ctx)
}

func (s *Store) FailPublication(ctx context.Context, item Publication, code string, now time.Time) error {
	if s == nil || s.db == nil || item.OutboxID <= 0 || item.Record.ID <= 0 || item.Record.LeaseToken == "" {
		return domain.ErrPublishFailed
	}
	now = now.UTC()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE workforce_task_notification_records
		SET status='FAILED',last_error_code=$3,lease_token='',lease_until=NULL,next_retry_at=NULL,updated_at=$4
		WHERE id=$1 AND status='PROCESSING' AND lease_token=$2`, item.Record.ID, item.Record.LeaseToken, boundedCode(code), now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrPublishFailed
	}
	outboxTag, err := tx.Exec(ctx, `UPDATE workforce_outbox_events
		SET status='FAILED',attempts=attempts+1,next_retry_at=NULL,last_error=$3
		WHERE id=$1 AND aggregate_id=$2 AND aggregate_type=$4 AND status='PENDING'`, item.OutboxID, item.Record.ID, boundedCode(code), domain.OutboxAggregate)
	if err != nil {
		return err
	}
	if outboxTag.RowsAffected() != 1 {
		return domain.ErrPublishFailed
	}
	return tx.Commit(ctx)
}

func retryDelay(attempt int) time.Duration {
	switch {
	case attempt <= 1:
		return time.Second
	case attempt == 2:
		return 10 * time.Second
	default:
		return 60 * time.Second
	}
}

func boundedCode(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 128 {
		return value[:128]
	}
	return value
}

type subscriptionWire struct {
	ID               string                    `json:"id"`
	SubscriberUserID string                    `json:"subscriberUserId"`
	Version          string                    `json:"version"`
	Events           []domain.FactType         `json:"events"`
	Channels         []domain.ChannelSelection `json:"channels"`
	Enabled          bool                      `json:"enabled"`
}

func containsEvent(events []domain.FactType, wanted domain.FactType) bool {
	for _, event := range events {
		if event == wanted {
			return true
		}
	}
	return false
}

func parsePositiveInt64(value string) (int64, error) {
	if value == "" || strings.TrimSpace(value) != value {
		return 0, domain.ErrRecipientInvalid
	}
	result, err := strconv.ParseInt(value, 10, 64)
	if err != nil || result <= 0 {
		return 0, domain.ErrRecipientInvalid
	}
	return result, nil
}

func slicesEqual(left, right []domain.FactType) bool {
	left = domain.CanonicalEvents(left)
	right = domain.CanonicalEvents(right)
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func channelsEqual(left, right []domain.ChannelSelection) bool {
	left = domain.CanonicalChannels(left)
	right = domain.CanonicalChannels(right)
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
