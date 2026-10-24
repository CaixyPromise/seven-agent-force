package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	rabbitinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/messaging/rabbitmq"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	"github.com/bytedance/sonic"
	"github.com/google/uuid"
)

const (
	FanoutExchange           = "seven.cache-governance.invalidate.v1"
	FanoutDeadLetterExchange = "seven.cache-governance.invalidate.dlq.v1"
	// FanoutTopologyQueueVersion changes whenever durable source or diagnostic
	// queue arguments change. RabbitMQ rejects an in-place declaration with
	// different x-arguments, so an upgraded stable instance must bind a new
	// queue rather than need a destructive QueueDelete or remain unavailable.
	FanoutTopologyQueueVersion = "v3"
	// MaxFanoutEnvelopeBytes aliases the shared durable-and-broker wire bound.
	// The five fixed v1 fields measure below 300 bytes in the real acceptance
	// harness; 1 KiB leaves a reviewed evolution margin while preventing a
	// broker publisher from forcing multi-megabyte copies/decodes.
	MaxFanoutEnvelopeBytes = cachepolicy.MaxInvalidationEnvelopeBytes
	queueExpires           = 30 * time.Minute
)

func validateFanoutEnvelopePayload(payload []byte) error {
	if len(payload) == 0 || len(payload) > MaxFanoutEnvelopeBytes {
		return cachepolicy.ErrInvalidationEnvelope
	}
	return nil
}

// FanoutAdapter maps one instance to a durable fanout queue. The queue name
// uses an opaque instance digest so broker metadata does not reveal a runtime
// identity; both source and diagnostic queues expire after a controlled period
// of inactivity and cannot become an unbounded fleet of stale queues. The
// separate opaque DLQ retains only local, content-free rejection diagnostics
// during that retention period after a confirmed replacement publish.
type FanoutAdapter struct {
	client               *rabbitinfra.Client
	generation           cachepolicy.GenerationPort
	queue                string
	deadLetterQueue      string
	deadLetterRoutingKey string
	consumer             string
	declare              bool
}

func NewFanoutAdapter(client *rabbitinfra.Client, generation cachepolicy.GenerationPort, instanceID string, declare bool) (*FanoutAdapter, error) {
	if client == nil || !client.Enabled() {
		return nil, cachepolicy.ErrFanoutUnavailable
	}
	if generation == nil {
		return nil, fmt.Errorf("cache generation adapter is required")
	}
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return nil, fmt.Errorf("cache governance instance id is required")
	}
	digest := cachepolicy.EventDigest(instanceID)
	queue := fanoutQueueName(digest)
	adapter := &FanoutAdapter{
		client:               client,
		generation:           generation,
		queue:                queue,
		deadLetterQueue:      queue + ".dlq",
		deadLetterRoutingKey: queue + ".dead",
		consumer:             "cache-governance-" + digest[:16],
		declare:              declare,
	}
	if declare {
		if err := adapter.ensureTopology(); err != nil {
			return nil, err
		}
	}
	return adapter, nil
}

func fanoutQueueName(instanceDigest string) string {
	return "seven.cache-governance.dg5." + FanoutTopologyQueueVersion + "." + instanceDigest[:24]
}

func (a *FanoutAdapter) Enabled() bool {
	return a != nil && a.client != nil && a.client.Enabled()
}

func (a *FanoutAdapter) Publish(ctx context.Context, event cachepolicy.InvalidationEnvelope) error {
	if !a.Enabled() {
		return cachepolicy.ErrFanoutUnavailable
	}
	if err := event.Validate(); err != nil {
		return err
	}
	// DG5 owns its Sonic transport boundary. The generic RabbitMQ client only
	// transports these bytes and keeps the legacy encoding/json behavior for
	// file, notification, and other consumers.
	payload, err := sonic.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode cache invalidation fanout envelope: %w", err)
	}
	return a.client.PublishRaw(ctx, rabbitinfra.RawPublishOptions{
		Exchange:  FanoutExchange,
		MessageID: event.EventID,
		Body:      payload,
	})
}

// PublishTargeted is intentionally Sonic-only at the same governed cache
// boundary. The generic RabbitMQ client continues to use encoding/json for
// unrelated consumers.
func (a *FanoutAdapter) PublishTargeted(ctx context.Context, event cachepolicy.TargetedInvalidationEnvelope) error {
	if !a.Enabled() {
		return cachepolicy.ErrFanoutUnavailable
	}
	if err := event.Validate(); err != nil {
		return err
	}
	payload, err := sonic.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode targeted cache invalidation fanout envelope: %w", err)
	}
	return a.client.PublishRaw(ctx, rabbitinfra.RawPublishOptions{Exchange: FanoutExchange, MessageID: event.EventID, Body: payload})
}

// PublishRefresh transports only the strict DG6.3 V3 envelope on the existing
// governed fanout exchange. It does not create a second broker topology or
// change the generic RabbitMQ client's JSON behavior.
func (a *FanoutAdapter) PublishRefresh(ctx context.Context, event cachepolicy.CacheRefreshEnvelope) error {
	if !a.Enabled() {
		return cachepolicy.ErrFanoutUnavailable
	}
	if err := event.Validate(); err != nil {
		return err
	}
	payload, err := sonic.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode cache refresh fanout envelope: %w", err)
	}
	return a.client.PublishRaw(ctx, rabbitinfra.RawPublishOptions{Exchange: FanoutExchange, MessageID: event.EventID, Body: payload})
}

// Consume starts a manual-ACK loop. The handler performs local L1 eviction
// before the generic transport ACKs. Unknown or malformed DG5 messages are
// never broker-dead-lettered verbatim: a confirmed content-free diagnostic is
// published to the controlled DLQ before the source delivery is acknowledged.
func (a *FanoutAdapter) Consume(ctx context.Context, handler func(context.Context, cachepolicy.InvalidationEnvelope) error) error {
	if !a.Enabled() {
		a.setHealthy(false)
		return cachepolicy.ErrFanoutUnavailable
	}
	if handler == nil {
		a.setHealthy(false)
		return errors.New("cache fanout handler is required")
	}
	if err := a.ensureTopology(); err != nil {
		a.setHealthy(false)
		return err
	}
	a.setHealthy(true)
	err := rabbitinfra.ConsumeJSONWithDecoder(ctx, a.client, a.queue, a.consumer, func(payload []byte) ([]byte, error) {
		// The handler settles a hostile body through the content-free diagnostic
		// protocol, so keep the AMQP delivery slice transient rather than making
		// a second unbounded copy in an identity decoder.
		return payload, nil
	}, nil, func(messageCtx context.Context, delivery rabbitinfra.Delivery[[]byte]) error {
		if err := validateFanoutEnvelopePayload(delivery.Message); err != nil {
			return a.replaceRejectedDelivery(messageCtx, delivery, cachepolicy.FanoutRejectionInvalidEnvelope)
		}
		event, decodeErr := cachepolicy.DecodeInvalidationEnvelope(delivery.Message)
		if decodeErr != nil {
			return a.replaceRejectedDelivery(messageCtx, delivery, cachepolicy.FanoutRejectionInvalidEnvelope)
		}
		if strings.TrimSpace(delivery.MessageID) == "" || strings.TrimSpace(delivery.MessageID) != strings.TrimSpace(event.EventID) {
			return a.replaceRejectedDelivery(messageCtx, delivery, cachepolicy.FanoutRejectionInvalidDelivery)
		}
		if err := event.Validate(); err != nil {
			return a.replaceRejectedDelivery(messageCtx, delivery, cachepolicy.FanoutRejectionInvalidEnvelope)
		}
		if err := handler(messageCtx, event); err != nil {
			if errors.Is(err, cachepolicy.ErrInvalidationEnvelope) {
				return a.replaceRejectedDelivery(messageCtx, delivery, cachepolicy.FanoutRejectionInvalidEnvelope)
			}
			return err
		}
		return nil
	})
	a.setHealthy(false)
	return err
}

// ConsumeMixed keeps the V1 queue/topology intact while allowing only the
// second strict envelope shape. Unknown bodies still take the content-free
// terminal diagnostic path; no generic JSON decoding is widened.
func (a *FanoutAdapter) ConsumeMixed(ctx context.Context, v1 func(context.Context, cachepolicy.InvalidationEnvelope) error, v2 func(context.Context, cachepolicy.TargetedInvalidationEnvelope) error) error {
	return a.ConsumeGoverned(ctx, v1, v2, nil)
}

// ConsumeGoverned keeps V1/V2 strict decoders intact and adds an explicitly
// separate V3 path. A nil V3 handler deliberately rejects V3 rather than
// letting a legacy consumer reinterpret it.
func (a *FanoutAdapter) ConsumeGoverned(ctx context.Context, v1 func(context.Context, cachepolicy.InvalidationEnvelope) error, v2 func(context.Context, cachepolicy.TargetedInvalidationEnvelope) error, v3 func(context.Context, cachepolicy.CacheRefreshEnvelope) error) error {
	if !a.Enabled() {
		a.setHealthy(false)
		return cachepolicy.ErrFanoutUnavailable
	}
	if v1 == nil || v2 == nil {
		a.setHealthy(false)
		return errors.New("cache fanout handlers are required")
	}
	if err := a.ensureTopology(); err != nil {
		a.setHealthy(false)
		return err
	}
	a.setHealthy(true)
	err := rabbitinfra.ConsumeJSONWithDecoder(ctx, a.client, a.queue, a.consumer, func(payload []byte) ([]byte, error) { return payload, nil }, nil, func(messageCtx context.Context, delivery rabbitinfra.Delivery[[]byte]) error {
		if err := validateFanoutEnvelopePayload(delivery.Message); err != nil {
			return a.replaceRejectedDelivery(messageCtx, delivery, cachepolicy.FanoutRejectionInvalidEnvelope)
		}
		if event, err := cachepolicy.DecodeInvalidationEnvelope(delivery.Message); err == nil {
			if strings.TrimSpace(delivery.MessageID) != strings.TrimSpace(event.EventID) {
				return a.replaceRejectedDelivery(messageCtx, delivery, cachepolicy.FanoutRejectionInvalidDelivery)
			}
			if err := v1(messageCtx, event); err != nil {
				return err
			}
			return nil
		}
		if event, err := cachepolicy.DecodeTargetedInvalidationEnvelope(delivery.Message); err == nil {
			if strings.TrimSpace(delivery.MessageID) != strings.TrimSpace(event.EventID) {
				return a.replaceRejectedDelivery(messageCtx, delivery, cachepolicy.FanoutRejectionInvalidDelivery)
			}
			return v2(messageCtx, event)
		}
		event, err := cachepolicy.DecodeCacheRefreshEnvelope(delivery.Message)
		if err != nil || v3 == nil {
			return a.replaceRejectedDelivery(messageCtx, delivery, cachepolicy.FanoutRejectionInvalidEnvelope)
		}
		if strings.TrimSpace(delivery.MessageID) != strings.TrimSpace(event.EventID) {
			return a.replaceRejectedDelivery(messageCtx, delivery, cachepolicy.FanoutRejectionInvalidDelivery)
		}
		return v3(messageCtx, event)
	})
	a.setHealthy(false)
	return err
}

var _ cachepolicy.RefreshFanoutPort = (*FanoutAdapter)(nil)

// replaceRejectedDelivery never sends an untrusted broker body to the DLQ.
// If its safe replacement cannot be confirmed, the original source delivery
// remains retryable so a permanent failure cannot disappear without a durable
// terminal diagnostic.
func (a *FanoutAdapter) replaceRejectedDelivery(ctx context.Context, delivery rabbitinfra.Delivery[[]byte], category string) error {
	if a == nil || a.client == nil {
		return cachepolicy.ErrFanoutUnavailable
	}
	diagnostic, err := cachepolicy.NewFanoutRejectionDiagnostic(uuid.NewString(), a.queue, category)
	if err != nil {
		a.setHealthy(false)
		return err
	}
	payload, err := sonic.Marshal(diagnostic)
	if err != nil {
		a.setHealthy(false)
		return fmt.Errorf("encode cache fanout rejection diagnostic: %w", err)
	}
	if err := a.client.PublishRaw(ctx, rabbitinfra.RawPublishOptions{
		Exchange:   FanoutDeadLetterExchange,
		RoutingKey: a.deadLetterRoutingKey,
		MessageID:  diagnostic.EventID,
		Body:       payload,
	}); err != nil {
		a.setHealthy(false)
		return fmt.Errorf("publish cache fanout rejection diagnostic: %w", err)
	}
	if err := delivery.Ack(false); err != nil {
		a.setHealthy(false)
		return fmt.Errorf("ack rejected cache fanout delivery: %w", err)
	}
	a.generation.RecordRejectedFanout()
	a.setHealthy(true)
	return rabbitinfra.DeliverySettled()
}

func (a *FanoutAdapter) Reconnect(ctx context.Context) error {
	if a == nil || a.client == nil {
		return cachepolicy.ErrFanoutUnavailable
	}
	a.setHealthy(false)
	if err := a.client.Reconnect(ctx); err != nil {
		return err
	}
	return a.ensureTopology()
}

func (a *FanoutAdapter) QueueName() string {
	if a == nil {
		return ""
	}
	return a.queue
}

// DeadLetterQueueName is an opaque, instance-scoped diagnostic identifier;
// it is never exposed through an application route or ordinary cache API.
func (a *FanoutAdapter) DeadLetterQueueName() string {
	if a == nil {
		return ""
	}
	return a.deadLetterQueue
}

// DeadLetterCount exposes only an aggregate queue depth for diagnostics. It
// does not consume, return, or reveal a dead-letter body.
func (a *FanoutAdapter) DeadLetterCount(ctx context.Context) (int, error) {
	if a == nil || a.client == nil {
		return 0, cachepolicy.ErrFanoutUnavailable
	}
	if ctx != nil {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}
	}
	return a.client.QueueMessageCount(a.deadLetterQueue)
}

func (a *FanoutAdapter) ensureTopology() error {
	if a == nil || a.client == nil || !a.client.Enabled() {
		return cachepolicy.ErrFanoutUnavailable
	}
	if !a.declare {
		return nil
	}
	return a.client.DeclareTopology(a.topology())
}

func (a *FanoutAdapter) topology() rabbitinfra.Topology {
	return rabbitinfra.Topology{
		Exchanges: []rabbitinfra.ExchangeOptions{
			{Name: FanoutExchange, Type: "fanout", Durable: true},
			{Name: FanoutDeadLetterExchange, Type: "direct", Durable: true},
		},
		Queues: []rabbitinfra.QueueDeclaration{
			{
				Name: a.queue,
				Options: rabbitinfra.QueueOptions{
					Expires: queueExpires,
				},
			},
			{
				Name: a.deadLetterQueue,
				// A content-free terminal diagnostic must survive a short instance
				// outage, but a permanently idle per-instance queue must not retain
				// file descriptors forever. This is queue retention, not a message
				// TTL, and it never broker-dead-letters the untrusted raw body.
				Options: rabbitinfra.QueueOptions{Expires: queueExpires},
			},
		},
		Bindings: []rabbitinfra.Binding{
			{Exchange: FanoutExchange, Queue: a.queue},
			{Exchange: FanoutDeadLetterExchange, Queue: a.deadLetterQueue, RoutingKey: a.deadLetterRoutingKey},
		},
	}
}

func (a *FanoutAdapter) setHealthy(healthy bool) {
	if a != nil && a.generation != nil {
		a.generation.SetFanoutHealthy(healthy)
	}
}

var _ cachepolicy.FanoutPort = (*FanoutAdapter)(nil)
var _ cachepolicy.TargetedFanoutPort = (*FanoutAdapter)(nil)
