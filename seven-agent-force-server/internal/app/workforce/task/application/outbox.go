package application

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/infrastructure"
)

type TaskPublisher interface {
	PublishTask(context.Context, infrastructure.OutboxEvent) error
}

type OutboxPublisher struct {
	store  *infrastructure.Store
	broker TaskPublisher
	logger *zap.Logger
}

func NewOutboxPublisher(store *infrastructure.Store, broker TaskPublisher, logger *zap.Logger) *OutboxPublisher {
	return &OutboxPublisher{store: store, broker: broker, logger: logger}
}

func (p *OutboxPublisher) Run(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := p.publishBatch(ctx); err != nil && ctx.Err() == nil {
			p.logger.Warn("publish outbox batch", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *OutboxPublisher) publishBatch(ctx context.Context) error {
	events, err := p.store.PendingOutbox(ctx, 32)
	if err != nil {
		return err
	}
	for _, event := range events {
		publishCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
		err := p.broker.PublishTask(publishCtx, event)
		cancel()
		if err != nil {
			_ = p.store.MarkOutboxFailed(ctx, event.ID, err)
			return err
		}
		if err := p.store.MarkOutboxPublished(ctx, event.ID); err != nil {
			return err
		}
	}
	return nil
}
