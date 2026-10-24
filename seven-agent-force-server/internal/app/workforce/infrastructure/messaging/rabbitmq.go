package messaging

import (
	"errors"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	config "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
)

// Broker is the Worker-side transport for the queue contract returned by
// registration. The Framework Server is the topology authority: this adapter
// never declares a queue and has no shared-queue fallback.
type Broker struct {
	cfg  config.RabbitMQConfig
	conn *amqp.Connection
}

func Open(cfg config.RabbitMQConfig) (*Broker, error) {
	if !cfg.Enabled {
		return nil, errors.New("rabbitmq is required for the Worker")
	}
	if cfg.TaskQueue == "" {
		return nil, errors.New("registered worker queue is required")
	}
	conn, err := amqp.DialConfig(cfg.URL(), amqp.Config{Heartbeat: 10 * time.Second, Locale: "en_US"})
	if err != nil {
		return nil, fmt.Errorf("connect rabbitmq: %w", err)
	}
	return &Broker{cfg: cfg, conn: conn}, nil
}

func (b *Broker) Ready() bool {
	return b != nil && b.conn != nil && !b.conn.IsClosed()
}

func (b *Broker) Close() error {
	if b == nil || b.conn == nil {
		return nil
	}
	return b.conn.CloseDeadline(time.Now().Add(2 * time.Second))
}

func (b *Broker) Consume() (<-chan amqp.Delivery, func() error, error) {
	return b.ConsumeQueue(b.cfg.TaskQueue, b.cfg.Prefetch)
}

func (b *Broker) ConsumeQueue(queue string, prefetch int) (<-chan amqp.Delivery, func() error, error) {
	if !b.Ready() {
		return nil, nil, errors.New("rabbitmq connection is unavailable")
	}
	if queue == "" || prefetch < 1 || prefetch > 64 {
		return nil, nil, errors.New("rabbitmq consumer contract is invalid")
	}
	ch, err := b.conn.Channel()
	if err != nil {
		return nil, nil, fmt.Errorf("open rabbitmq consumer channel: %w", err)
	}
	if err := ch.Qos(prefetch, 0, false); err != nil {
		_ = ch.Close()
		return nil, nil, fmt.Errorf("set rabbitmq prefetch: %w", err)
	}
	deliveries, err := ch.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		return nil, nil, fmt.Errorf("consume registered worker queue: %w", err)
	}
	return deliveries, ch.Close, nil
}
