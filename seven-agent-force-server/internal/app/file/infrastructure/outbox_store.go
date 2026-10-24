package infrastructure

import (
	"context"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
	msgoutbox "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/messaging/outbox"
	"github.com/jmoiron/sqlx"
)

type OutboxStore struct {
	events *msgoutbox.Store
	guard  *msgoutbox.ConsumeGuard
}

const fileOutboxOwner = "file"

var fileOutboxEventTypes = []string{
	"UPLOAD_TASK_READY",
	"UPLOAD_TASK_UPLOADED",
	"FILE_PROCESS_TASK",
}

func NewOutboxStore(db *sqlx.DB) *OutboxStore {
	return &OutboxStore{
		events: msgoutbox.NewStore(db),
		guard:  msgoutbox.NewConsumeGuard(db),
	}
}

func (s *OutboxStore) AppendOutbox(ctx context.Context, event *domain.OutboxEvent) error {
	if event == nil {
		return s.events.Append(ctx, nil)
	}
	return s.events.Append(ctx, &msgoutbox.Event{
		ID:            event.ID,
		EventID:       event.EventID,
		EventOwner:    fileOutboxOwner,
		EventType:     event.EventType,
		AggregateType: event.AggregateType,
		AggregateID:   event.AggregateID,
		Payload:       event.Payload,
		Status:        event.Status,
		RetryCount:    event.RetryCount,
		NextRetryAt:   event.NextRetryAt,
		LastError:     event.LastError,
		CreateTime:    event.CreateTime,
		UpdateTime:    event.UpdateTime,
	})
}

func (s *OutboxStore) AppendOutboxBatch(ctx context.Context, events []domain.OutboxEvent) error {
	items := make([]*msgoutbox.Event, 0, len(events))
	for index := range events {
		event := &events[index]
		items = append(items, &msgoutbox.Event{
			ID: event.ID, EventID: event.EventID, EventOwner: fileOutboxOwner,
			EventType: event.EventType, AggregateType: event.AggregateType, AggregateID: event.AggregateID,
			Payload: event.Payload, Status: event.Status, RetryCount: event.RetryCount,
			NextRetryAt: event.NextRetryAt, LastError: event.LastError,
			CreateTime: event.CreateTime, UpdateTime: event.UpdateTime,
		})
	}
	return s.events.AppendBatch(ctx, items)
}

func (s *OutboxStore) ListReadyOutbox(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	events, err := s.events.ListReady(ctx, fileOutboxOwner, fileOutboxEventTypes, limit)
	if err != nil {
		return nil, err
	}
	result := make([]domain.OutboxEvent, 0, len(events))
	for _, event := range events {
		result = append(result, domain.OutboxEvent{
			ID:            event.ID,
			EventID:       event.EventID,
			EventOwner:    event.EventOwner,
			EventType:     event.EventType,
			AggregateType: event.AggregateType,
			AggregateID:   event.AggregateID,
			Payload:       event.Payload,
			Status:        event.Status,
			RetryCount:    event.RetryCount,
			NextRetryAt:   event.NextRetryAt,
			LastError:     event.LastError,
			CreateTime:    event.CreateTime,
			UpdateTime:    event.UpdateTime,
		})
	}
	return result, nil
}

func (s *OutboxStore) ListUnknownOutbox(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	events, err := s.events.ListUnknownReady(ctx, fileOutboxOwner, fileOutboxEventTypes, limit)
	if err != nil {
		return nil, err
	}
	result := make([]domain.OutboxEvent, 0, len(events))
	for _, event := range events {
		result = append(result, domain.OutboxEvent{
			ID:            event.ID,
			EventID:       event.EventID,
			EventOwner:    event.EventOwner,
			EventType:     event.EventType,
			AggregateType: event.AggregateType,
			AggregateID:   event.AggregateID,
			Payload:       event.Payload,
			Status:        event.Status,
			RetryCount:    event.RetryCount,
			NextRetryAt:   event.NextRetryAt,
			LastError:     event.LastError,
			CreateTime:    event.CreateTime,
			UpdateTime:    event.UpdateTime,
		})
	}
	return result, nil
}

func (s *OutboxStore) TryClaimOutbox(ctx context.Context, id int64, eventType, worker string) (*domain.OutboxLease, bool, error) {
	lease, claimed, err := s.events.Claim(ctx, fileOutboxOwner, eventType, id, worker)
	if err != nil || !claimed {
		return nil, claimed, err
	}
	return &domain.OutboxLease{Token: lease.Token, Until: lease.Until}, true, nil
}

func (s *OutboxStore) MarkOutbox(ctx context.Context, id int64, eventType, leaseToken, status, lastError string, retryCount int, nextRetryAt *time.Time) (bool, error) {
	return s.events.Mark(ctx, fileOutboxOwner, eventType, id, leaseToken, status, lastError, retryCount, nextRetryAt)
}

func (s *OutboxStore) BeginConsume(ctx context.Context, messageID, consumer, worker, detail string) (*domain.ConsumeLease, bool, error) {
	lease, claimed, err := s.guard.Begin(ctx, messageID, consumer, worker, detail)
	if err != nil || !claimed {
		return nil, claimed, err
	}
	return &domain.ConsumeLease{Token: lease.Token, Until: lease.Until}, true, nil
}

func (s *OutboxStore) MarkConsumed(ctx context.Context, messageID, consumer, leaseToken, detail string) (bool, error) {
	return s.guard.Finish(ctx, messageID, consumer, leaseToken, detail)
}

func (s *OutboxStore) MarkConsumeFailed(ctx context.Context, messageID, consumer, leaseToken, detail string) (bool, error) {
	return s.guard.Fail(ctx, messageID, consumer, leaseToken, detail)
}
