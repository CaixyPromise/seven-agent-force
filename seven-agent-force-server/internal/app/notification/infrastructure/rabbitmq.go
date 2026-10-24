package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	rabbitinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/messaging/rabbitmq"
)

const (
	NotificationExchange   = "seven.notification.direct"
	NotificationQueue      = "seven.notification.dispatch"
	NotificationRoutingKey = "notification.dispatch"
	NotificationDLX        = "seven.notification.dlx"
	NotificationDLQ        = "seven.notification.dead"
	NotificationDLK        = "notification.dead"
)

// ErrRabbitMQScopeMismatch means a caller attempted to publish a delivery to
// a broker adapter owned by another notification scope. It is intentionally
// checked before any publish so an application cannot accidentally bridge two
// Hub/Node/installations through the shared exchange.
var ErrRabbitMQScopeMismatch = errors.New("notification RabbitMQ scope does not match delivery scope")

type rabbitTopology struct {
	exchange   string
	queue      string
	routingKey string
	dlx        string
	dlq        string
	dlk        string
}

type RabbitMQ struct {
	client   *rabbitinfra.Client
	scopeID  string
	topology rabbitTopology
}

func NewRabbitMQ(client *rabbitinfra.Client, declare bool) (*RabbitMQ, error) {
	return NewScopedRabbitMQ(client, declare, "local")
}

// NewScopedRabbitMQ builds one notification transport endpoint for exactly
// one installation, Hub, or Node scope. The legacy local scope intentionally
// keeps its existing names so an upgrade does not strand its established
// queue. Every non-local scope receives a stable, opaque route and queue.
func NewScopedRabbitMQ(client *rabbitinfra.Client, declare bool, scopeID string) (*RabbitMQ, error) {
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" {
		return nil, fmt.Errorf("notification RabbitMQ scope is required")
	}
	topology := notificationRabbitTopology(scopeID)
	if client == nil || !client.Enabled() {
		return &RabbitMQ{client: rabbitinfra.Disabled(), scopeID: scopeID, topology: topology}, nil
	}
	adapter := &RabbitMQ{client: client, scopeID: scopeID, topology: topology}
	if declare {
		if err := adapter.Declare(); err != nil {
			return nil, err
		}
	}
	return adapter, nil
}

func (r *RabbitMQ) Enabled() bool {
	return r != nil && r.client != nil && r.client.Enabled()
}

func (r *RabbitMQ) Reconnect(ctx context.Context) error {
	if r == nil || r.client == nil {
		return nil
	}
	return r.client.Reconnect(ctx)
}

func (r *RabbitMQ) Declare() error {
	if !r.Enabled() {
		return nil
	}
	if err := r.client.DeclareDirectExchange(r.topology.exchange); err != nil {
		return err
	}
	if err := r.client.DeclareDirectExchange(r.topology.dlx); err != nil {
		return err
	}
	if err := r.client.DeclareQueue(r.topology.queue, rabbitinfra.QueueOptions{DeadLetterExchange: r.topology.dlx, DeadLetterRoutingKey: r.topology.dlk}, r.topology.exchange, r.topology.routingKey); err != nil {
		return err
	}
	return r.client.DeclareQueue(r.topology.dlq, rabbitinfra.QueueOptions{}, r.topology.dlx, r.topology.dlk)
}

func (r *RabbitMQ) PublishDispatch(ctx context.Context, message domain.DeliveryMessage) error {
	if message.ScopeID = strings.TrimSpace(message.ScopeID); message.ScopeID == "" {
		message.ScopeID = r.scopeID
	}
	if message.ScopeID != r.scopeID {
		return ErrRabbitMQScopeMismatch
	}
	if !r.Enabled() {
		return nil
	}
	return r.client.PublishJSON(ctx, r.topology.exchange, r.topology.routingKey, message.MessageID, message, 0)
}

func (r *RabbitMQ) ConsumeDispatch(ctx context.Context, consumer string, handler func(context.Context, domain.DeliveryMessage) error) error {
	return rabbitinfra.ConsumeJSON(ctx, r.client, r.topology.queue, consumer, func(ctx context.Context, delivery rabbitinfra.Delivery[domain.DeliveryMessage]) error {
		message := delivery.Message
		message.MessageID = normalizeDeliveryMessageID(message.MessageID, delivery.MessageID, delivery.Body)
		if message.ScopeID = strings.TrimSpace(message.ScopeID); message.ScopeID == "" {
			// Only the legacy local queue may contain a message produced before
			// scope-aware payloads existed. A non-local queue never assigns a
			// missing scope, because that would turn an ambiguous record into a
			// cross-scope delivery.
			if r.scopeID != "local" {
				return rabbitinfra.PermanentConsumeError(ErrRabbitMQScopeMismatch)
			}
			message.ScopeID = r.scopeID
		}
		if message.ScopeID != r.scopeID {
			return rabbitinfra.PermanentConsumeError(ErrRabbitMQScopeMismatch)
		}
		if strings.TrimSpace(message.DeliveryID) == "" {
			return rabbitinfra.PermanentConsumeError(errors.New("notification delivery id is required"))
		}
		return handler(ctx, message)
	})
}

func notificationRabbitTopology(scopeID string) rabbitTopology {
	if scopeID == "local" {
		return rabbitTopology{
			exchange: NotificationExchange, queue: NotificationQueue, routingKey: NotificationRoutingKey,
			dlx: NotificationDLX, dlq: NotificationDLQ, dlk: NotificationDLK,
		}
	}
	token := notificationScopeRoutingToken(scopeID)
	return rabbitTopology{
		exchange:   NotificationExchange,
		queue:      "seven.notification.dispatch.scope." + token,
		routingKey: "notification.dispatch.scope." + token,
		dlx:        NotificationDLX,
		dlq:        "seven.notification.dead.scope." + token,
		dlk:        "notification.dead.scope." + token,
	}
}

func notificationScopeRoutingToken(scopeID string) string {
	sum := sha256.Sum256([]byte("seven-notification-scope-routing-v1\x00" + strings.TrimSpace(scopeID)))
	// 128 bits keeps names short without exposing raw Hub/Node identifiers.
	return hex.EncodeToString(sum[:16])
}

func normalizeDeliveryMessageID(bodyMessageID, amqpMessageID string, body []byte) string {
	if value := strings.TrimSpace(bodyMessageID); value != "" {
		return value
	}
	if value := strings.TrimSpace(amqpMessageID); value != "" {
		return value
	}
	sum := sha256.Sum256(body)
	return fmt.Sprintf("notification:body-sha256:%s", hex.EncodeToString(sum[:]))
}
