package listener

import (
	"context"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
)

type Broker interface {
	Enabled() bool
	Reconnect(ctx context.Context) error
	ConsumeDispatch(ctx context.Context, consumer string, handler func(context.Context, domain.DeliveryMessage) error) error
}

type DispatchHandler interface {
	HandleDispatchMessage(ctx context.Context, message domain.DeliveryMessage) error
}

func StartRabbitConsumers(ctx context.Context, broker Broker, handler DispatchHandler) {
	if broker == nil || !broker.Enabled() || handler == nil {
		return
	}
	go consumeDispatch(ctx, broker, handler)
}

func consumeDispatch(ctx context.Context, broker Broker, handler DispatchHandler) {
	for {
		if ctx.Err() != nil {
			return
		}
		_ = broker.ConsumeDispatch(ctx, "notification-dispatch", handler.HandleDispatchMessage)
		if ctx.Err() == nil {
			_ = broker.Reconnect(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}
