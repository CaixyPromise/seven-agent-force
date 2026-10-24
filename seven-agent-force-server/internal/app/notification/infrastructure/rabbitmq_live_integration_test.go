//go:build integration

package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	rabbitinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/messaging/rabbitmq"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	amqp "github.com/rabbitmq/amqp091-go"
)

const g43RabbitMQIntegrationVHost = "seven_notification_g43"

// TestLiveNotificationRabbitMQConfirmReconnectAndBoundedConsumer exercises the
// actual broker only when explicitly enabled. It is pinned to a dedicated local
// vhost so it never declares, purges, or consumes queues from a shared runtime.
func TestLiveNotificationRabbitMQConfirmReconnectAndBoundedConsumer(t *testing.T) {
	if strings.TrimSpace(os.Getenv("SEVEN_RABBITMQ_INTEGRATION")) != "1" {
		t.Skip("set SEVEN_RABBITMQ_INTEGRATION=1 to run the local RabbitMQ integration check")
	}
	cfg := g43RabbitMQConfig()
	if cfg.VHost != g43RabbitMQIntegrationVHost {
		t.Fatalf("refusing to run against non-isolated vhost %q", cfg.VHost)
	}

	client, err := rabbitinfra.New(cfg)
	if err != nil {
		t.Fatalf("connect isolated RabbitMQ vhost: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	adapter, err := NewRabbitMQ(client, true)
	if err != nil {
		t.Fatalf("declare isolated notification topology: %v", err)
	}
	t.Cleanup(func() { cleanupG43NotificationTopology(t, cfg) })
	clearG43NotificationQueues(t, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	firstReceived, firstCancel, firstDone, firstMaxActive := startG43Consumer(adapter, "g43-live-consumer-first")
	for index := 0; index < 4; index++ {
		message := domain.DeliveryMessage{
			MessageID:  fmt.Sprintf("g43-live-before-reconnect-%d", index),
			DeliveryID: fmt.Sprintf("g43-live-before-reconnect-delivery-%d", index),
		}
		if err := adapter.PublishDispatch(ctx, message); err != nil {
			t.Fatalf("publish confirmed notification %d: %v", index, err)
		}
	}
	awaitG43Messages(t, firstReceived, 4)
	if maxActive := firstMaxActive(); maxActive != 1 {
		t.Fatalf("consumer concurrency=%d, want exactly 1 for the bounded single-consumer probe", maxActive)
	}
	stopG43Consumer(t, firstCancel, firstDone)

	if err := client.Close(); err != nil {
		t.Fatalf("close client before recovery: %v", err)
	}
	if err := client.Reconnect(ctx); err != nil {
		t.Fatalf("reconnect RabbitMQ client: %v", err)
	}
	if err := adapter.Declare(); err != nil {
		t.Fatalf("redeclare topology after reconnect: %v", err)
	}
	secondReceived, secondCancel, secondDone, _ := startG43Consumer(adapter, "g43-live-consumer-recovered")
	defer stopG43Consumer(t, secondCancel, secondDone)
	recovered := domain.DeliveryMessage{MessageID: "g43-live-after-reconnect", DeliveryID: "g43-live-after-reconnect-delivery"}
	if err := adapter.PublishDispatch(ctx, recovered); err != nil {
		t.Fatalf("publish confirmed notification after reconnect: %v", err)
	}
	awaitG43Messages(t, secondReceived, 1)

	err = client.PublishJSON(ctx, NotificationExchange, "g43.unroutable", "g43-live-unroutable", map[string]string{"probe": "g43"}, 0)
	if !errors.Is(err, rabbitinfra.ErrPublishUnroutable) {
		t.Fatalf("unroutable publish error=%v, want ErrPublishUnroutable", err)
	}
}

// TestLiveNotificationRabbitMQSeparatesScopes proves the gray-release
// boundary on the real isolated broker: two runtimes sharing the same vhost
// receive only the delivery route for their own stable scope. It never calls a
// third-party provider and deletes only the dedicated G4.3 topology.
func TestLiveNotificationRabbitMQSeparatesScopes(t *testing.T) {
	if strings.TrimSpace(os.Getenv("SEVEN_RABBITMQ_INTEGRATION")) != "1" {
		t.Skip("set SEVEN_RABBITMQ_INTEGRATION=1 to run the local RabbitMQ integration check")
	}
	cfg := g43RabbitMQConfig()
	if cfg.VHost != g43RabbitMQIntegrationVHost {
		t.Fatalf("refusing to run against non-isolated vhost %q", cfg.VHost)
	}
	clientA, err := rabbitinfra.New(cfg)
	if err != nil {
		t.Fatalf("connect isolated RabbitMQ scope A: %v", err)
	}
	t.Cleanup(func() { _ = clientA.Close() })
	clientB, err := rabbitinfra.New(cfg)
	if err != nil {
		t.Fatalf("connect isolated RabbitMQ scope B: %v", err)
	}
	t.Cleanup(func() { _ = clientB.Close() })

	const (
		scopeA = "node:g43-gray-a"
		scopeB = "node:g43-gray-b"
	)
	adapterA, err := NewScopedRabbitMQ(clientA, true, scopeA)
	if err != nil {
		t.Fatalf("declare isolated scope A topology: %v", err)
	}
	adapterB, err := NewScopedRabbitMQ(clientB, true, scopeB)
	if err != nil {
		t.Fatalf("declare isolated scope B topology: %v", err)
	}
	t.Cleanup(func() { cleanupG43NotificationTopologies(t, cfg, adapterA.topology, adapterB.topology) })
	clearG43NotificationTopologies(t, cfg, adapterA.topology, adapterB.topology)

	receivedA, cancelA, doneA, _ := startG43Consumer(adapterA, "g43-gray-consumer-a")
	defer stopG43Consumer(t, cancelA, doneA)
	receivedB, cancelB, doneB, _ := startG43Consumer(adapterB, "g43-gray-consumer-b")
	defer stopG43Consumer(t, cancelB, doneB)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := adapterA.PublishDispatch(ctx, domain.DeliveryMessage{MessageID: "g43-gray-a", DeliveryID: "g43-gray-a-delivery", ScopeID: scopeA}); err != nil {
		t.Fatalf("publish scope A: %v", err)
	}
	if err := adapterB.PublishDispatch(ctx, domain.DeliveryMessage{MessageID: "g43-gray-b", DeliveryID: "g43-gray-b-delivery", ScopeID: scopeB}); err != nil {
		t.Fatalf("publish scope B: %v", err)
	}
	assertG43ScopeMessage(t, receivedA, scopeA)
	assertG43ScopeMessage(t, receivedB, scopeB)
	assertNoG43Message(t, receivedA)
	assertNoG43Message(t, receivedB)
}

// TestLiveNotificationRabbitMQScopeBProgressesPastScopeABacklog proves that a
// substantial queue backlog in scope A cannot delay or leak into scope B. It
// intentionally leaves A unconsumed, uses the dedicated G4.3 vhost only, and
// never invokes a third-party provider. The result is an isolation check, not
// a production throughput or latency SLO.
func TestLiveNotificationRabbitMQScopeBProgressesPastScopeABacklog(t *testing.T) {
	if strings.TrimSpace(os.Getenv("SEVEN_RABBITMQ_INTEGRATION")) != "1" {
		t.Skip("set SEVEN_RABBITMQ_INTEGRATION=1 to run the local RabbitMQ integration check")
	}
	cfg := g43RabbitMQConfig()
	if cfg.VHost != g43RabbitMQIntegrationVHost {
		t.Fatalf("refusing to run against non-isolated vhost %q", cfg.VHost)
	}
	clientA, err := rabbitinfra.New(cfg)
	if err != nil {
		t.Fatalf("connect isolated RabbitMQ backlog scope A: %v", err)
	}
	t.Cleanup(func() { _ = clientA.Close() })
	clientB, err := rabbitinfra.New(cfg)
	if err != nil {
		t.Fatalf("connect isolated RabbitMQ backlog scope B: %v", err)
	}
	t.Cleanup(func() { _ = clientB.Close() })

	const (
		scopeA       = "node:g43-gray-backlog-a"
		scopeB       = "node:g43-gray-backlog-b"
		backlogCount = 100
	)
	adapterA, err := NewScopedRabbitMQ(clientA, true, scopeA)
	if err != nil {
		t.Fatalf("declare isolated backlog scope A topology: %v", err)
	}
	adapterB, err := NewScopedRabbitMQ(clientB, true, scopeB)
	if err != nil {
		t.Fatalf("declare isolated backlog scope B topology: %v", err)
	}
	t.Cleanup(func() { cleanupG43NotificationTopologies(t, cfg, adapterA.topology, adapterB.topology) })
	clearG43NotificationTopologies(t, cfg, adapterA.topology, adapterB.topology)

	receivedB, cancelB, doneB, _ := startG43Consumer(adapterB, "g43-gray-backlog-consumer-b")
	defer stopG43Consumer(t, cancelB, doneB)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for index := 0; index < backlogCount; index++ {
		message := domain.DeliveryMessage{
			MessageID:  fmt.Sprintf("g43-gray-backlog-a-%d", index),
			DeliveryID: fmt.Sprintf("g43-gray-backlog-a-delivery-%d", index),
			ScopeID:    scopeA,
		}
		if err := adapterA.PublishDispatch(ctx, message); err != nil {
			t.Fatalf("publish scope A backlog message %d: %v", index, err)
		}
	}
	if queued := g43QueueMessageCount(t, cfg, adapterA.topology.queue); queued != backlogCount {
		t.Fatalf("scope A queued backlog=%d, want %d", queued, backlogCount)
	}
	started := time.Now()
	if err := adapterB.PublishDispatch(ctx, domain.DeliveryMessage{
		MessageID:  "g43-gray-backlog-b",
		DeliveryID: "g43-gray-backlog-b-delivery",
		ScopeID:    scopeB,
	}); err != nil {
		t.Fatalf("publish scope B while A has backlog: %v", err)
	}
	assertG43ScopeMessage(t, receivedB, scopeB)
	assertNoG43Message(t, receivedB)
	if queued := g43QueueMessageCount(t, cfg, adapterA.topology.queue); queued != backlogCount {
		t.Fatalf("scope B consumption changed scope A backlog=%d, want %d", queued, backlogCount)
	}
	t.Logf("observed scope B delivery with scope A backlog: scopeABacklog=%d deliveredIn=%s", backlogCount, time.Since(started).Round(time.Millisecond))
}

// TestLiveNotificationRabbitMQDrainsConfiguredPrefetchWaves is an observed
// local pressure probe, not a production SLO. It drains four waves of the
// currently configured prefetch with one consumer and reports the measured
// duration, while asserting that the handler never fans out unboundedly.
func TestLiveNotificationRabbitMQDrainsConfiguredPrefetchWaves(t *testing.T) {
	if strings.TrimSpace(os.Getenv("SEVEN_RABBITMQ_INTEGRATION")) != "1" {
		t.Skip("set SEVEN_RABBITMQ_INTEGRATION=1 to run the local RabbitMQ integration check")
	}
	cfg := g43RabbitMQConfig()
	if cfg.VHost != g43RabbitMQIntegrationVHost {
		t.Fatalf("refusing to run against non-isolated vhost %q", cfg.VHost)
	}
	if cfg.Prefetch <= 0 {
		t.Fatalf("configured RabbitMQ prefetch must be positive, got %d", cfg.Prefetch)
	}
	client, err := rabbitinfra.New(cfg)
	if err != nil {
		t.Fatalf("connect isolated RabbitMQ vhost: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	adapter, err := NewRabbitMQ(client, true)
	if err != nil {
		t.Fatalf("declare isolated notification topology: %v", err)
	}
	t.Cleanup(func() { cleanupG43NotificationTopology(t, cfg) })
	clearG43NotificationQueues(t, cfg)

	messageCount := cfg.Prefetch * 4
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	received, stop, done, maxActive := startG43Consumer(adapter, "g43-capacity-consumer")
	defer stopG43Consumer(t, stop, done)
	for index := 0; index < messageCount; index++ {
		message := domain.DeliveryMessage{
			MessageID:  fmt.Sprintf("g43-capacity-%d", index),
			DeliveryID: fmt.Sprintf("g43-capacity-delivery-%d", index),
		}
		if err := adapter.PublishDispatch(ctx, message); err != nil {
			t.Fatalf("publish pressure message %d: %v", index, err)
		}
	}
	started := time.Now()
	awaitG43Messages(t, received, messageCount)
	if active := maxActive(); active != 1 {
		t.Fatalf("pressure probe handler concurrency=%d, want 1 bounded consumer", active)
	}
	t.Logf("observed local notification pressure probe: prefetch=%d messages=%d drainedIn=%s", cfg.Prefetch, messageCount, time.Since(started).Round(time.Millisecond))
}

func g43RabbitMQConfig() config.RabbitMQConfig {
	return config.RabbitMQConfig{
		Enabled:  true,
		Host:     g43EnvOrDefault("SEVEN_RABBITMQ_HOST", "127.0.0.1"),
		Port:     5672,
		Username: g43EnvOrDefault("SEVEN_RABBITMQ_USERNAME", "guest"),
		Password: g43EnvOrDefault("SEVEN_RABBITMQ_PASSWORD", "guest"),
		VHost:    g43EnvOrDefault("SEVEN_RABBITMQ_VHOST", g43RabbitMQIntegrationVHost),
		Prefetch: g43EnvIntOrDefault("SEVEN_RABBITMQ_PREFETCH", 10),
		Declare:  true,
	}
}

func g43EnvOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func g43EnvIntOrDefault(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func startG43Consumer(adapter *RabbitMQ, consumer string) (<-chan domain.DeliveryMessage, context.CancelFunc, <-chan error, func() int) {
	ctx, cancel := context.WithCancel(context.Background())
	received := make(chan domain.DeliveryMessage, 8)
	done := make(chan error, 1)
	var mu sync.Mutex
	active := 0
	maxActive := 0
	go func() {
		done <- adapter.ConsumeDispatch(ctx, consumer, func(_ context.Context, message domain.DeliveryMessage) error {
			mu.Lock()
			active++
			if active > maxActive {
				maxActive = active
			}
			mu.Unlock()
			defer func() {
				mu.Lock()
				active--
				mu.Unlock()
			}()
			time.Sleep(15 * time.Millisecond)
			received <- message
			return nil
		})
	}()
	return received, cancel, done, func() int {
		mu.Lock()
		defer mu.Unlock()
		return maxActive
	}
}

func awaitG43Messages(t *testing.T, received <-chan domain.DeliveryMessage, want int) {
	t.Helper()
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	for index := 0; index < want; index++ {
		select {
		case message := <-received:
			if strings.TrimSpace(message.MessageID) == "" {
				t.Fatal("received notification message without message id")
			}
		case <-deadline.C:
			t.Fatalf("received fewer than %d notification messages", want)
		}
	}
}

func assertG43ScopeMessage(t *testing.T, received <-chan domain.DeliveryMessage, scopeID string) {
	t.Helper()
	select {
	case message := <-received:
		if message.ScopeID != scopeID {
			t.Fatalf("received scope=%q, want %q", message.ScopeID, scopeID)
		}
	case <-time.After(8 * time.Second):
		t.Fatalf("did not receive scoped message for %q", scopeID)
	}
}

func assertNoG43Message(t *testing.T, received <-chan domain.DeliveryMessage) {
	t.Helper()
	select {
	case message := <-received:
		t.Fatalf("consumer received a foreign extra message: %#v", message)
	case <-time.After(300 * time.Millisecond):
	}
}

func stopG43Consumer(t *testing.T, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("notification consumer stopped with error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("notification consumer did not stop")
	}
}

func clearG43NotificationQueues(t *testing.T, cfg config.RabbitMQConfig) {
	clearG43NotificationTopologies(t, cfg, notificationRabbitTopology("local"))
}

func clearG43NotificationTopologies(t *testing.T, cfg config.RabbitMQConfig, topologies ...rabbitTopology) {
	t.Helper()
	for _, topology := range topologies {
		withG43Channel(t, cfg, func(ch *amqp.Channel) error {
			_, err := ch.QueuePurge(topology.queue, false)
			return err
		})
		withG43Channel(t, cfg, func(ch *amqp.Channel) error {
			_, err := ch.QueuePurge(topology.dlq, false)
			return err
		})
	}
}

func cleanupG43NotificationTopology(t *testing.T, cfg config.RabbitMQConfig) {
	cleanupG43NotificationTopologies(t, cfg, notificationRabbitTopology("local"))
}

func cleanupG43NotificationTopologies(t *testing.T, cfg config.RabbitMQConfig, topologies ...rabbitTopology) {
	t.Helper()
	for _, topology := range topologies {
		for _, remove := range []func(*amqp.Channel) error{
			func(ch *amqp.Channel) error {
				_, err := ch.QueueDelete(topology.queue, false, false, false)
				return err
			},
			func(ch *amqp.Channel) error {
				_, err := ch.QueueDelete(topology.dlq, false, false, false)
				return err
			},
		} {
			withG43Channel(t, cfg, remove)
		}
	}
	for _, remove := range []func(*amqp.Channel) error{
		func(ch *amqp.Channel) error { return ch.ExchangeDelete(NotificationExchange, false, false) },
		func(ch *amqp.Channel) error { return ch.ExchangeDelete(NotificationDLX, false, false) },
	} {
		withG43Channel(t, cfg, remove)
	}
}

func withG43Channel(t *testing.T, cfg config.RabbitMQConfig, use func(*amqp.Channel) error) {
	t.Helper()
	conn, err := amqp.Dial(g43RabbitMQURL(cfg))
	if err != nil {
		t.Fatalf("connect isolated RabbitMQ cleanup channel: %v", err)
	}
	defer func() { _ = conn.Close() }()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("open isolated RabbitMQ cleanup channel: %v", err)
	}
	defer func() { _ = ch.Close() }()
	if err := use(ch); err != nil {
		t.Fatalf("operate isolated RabbitMQ notification topology: %v", err)
	}
}

func g43QueueMessageCount(t *testing.T, cfg config.RabbitMQConfig, queue string) int {
	t.Helper()
	count := -1
	withG43Channel(t, cfg, func(ch *amqp.Channel) error {
		result, err := ch.QueueInspect(queue)
		if err != nil {
			return err
		}
		count = result.Messages
		return nil
	})
	return count
}

func g43RabbitMQURL(cfg config.RabbitMQConfig) string {
	return fmt.Sprintf(
		"amqp://%s:%s@%s:%d/%s",
		url.QueryEscape(cfg.Username),
		url.QueryEscape(cfg.Password),
		cfg.Host,
		cfg.Port,
		strings.TrimPrefix(url.PathEscape(cfg.VHost), "/"),
	)
}
