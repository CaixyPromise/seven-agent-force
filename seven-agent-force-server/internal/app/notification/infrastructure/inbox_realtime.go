package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	redisclient "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// ErrInboxRealtimeUnavailable means Redis Pub/Sub is not available for an
// optional freshness hint. It must never be treated as an inbox-write failure.
var ErrInboxRealtimeUnavailable = errors.New("notification inbox realtime is unavailable")

const (
	inboxRealtimeChannelSize = 64
	inboxRealtimePublishWait = 2 * time.Second
	inboxRealtimeRetryWait   = time.Second
)

// InboxRealtimeBus publishes only content-free mailbox change hints. Redis is
// a best-effort transport: REST count/list/detail remain authoritative when it
// is disabled or unavailable.
type InboxRealtimeBus struct {
	client  redisclient.UniversalClient
	channel string
	scopeID string
	log     *zap.Logger
	hub     *inboxRealtimeHub
	ready   chan struct{}
	readyMu sync.Once
}

// NewInboxRealtimeBus builds the local fan-out hub and optional Redis Pub/Sub
// bridge. The scope becomes part of the channel identity without exposing its
// literal value in Redis channel names.
func NewInboxRealtimeBus(client redisclient.UniversalClient, keyPrefix, scopeID string, log *zap.Logger) *InboxRealtimeBus {
	if log == nil {
		log = zap.NewNop()
	}
	return &InboxRealtimeBus{
		client:  client,
		channel: inboxRealtimeChannel(keyPrefix, scopeID),
		scopeID: strings.TrimSpace(scopeID),
		log:     log.Named("notification.inbox.realtime"),
		hub:     newInboxRealtimeHub(),
		ready:   make(chan struct{}),
	}
}

// Start receives cross-instance hints until ctx is cancelled. It uses one
// bounded reconnect loop; a Redis outage stops freshness only, never writes.
func (b *InboxRealtimeBus) Start(ctx context.Context) {
	if b == nil || b.client == nil || strings.TrimSpace(b.scopeID) == "" {
		return
	}
	go b.consume(ctx)
}

// PublishInboxChanged publishes the post-commit hint. It never accepts title,
// preview, body, deep-link, recipient state or any other notification content.
func (b *InboxRealtimeBus) PublishInboxChanged(ctx context.Context, intent domain.InboxChangedIntent) error {
	if b == nil || b.client == nil {
		return ErrInboxRealtimeUnavailable
	}
	if err := b.validate(intent); err != nil {
		return err
	}
	payload, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	publishCtx, cancel := context.WithTimeout(ctx, inboxRealtimePublishWait)
	defer cancel()
	return b.client.Publish(publishCtx, b.channel, payload).Err()
}

// SubscribeInboxChanges returns a current-user-only hint stream. The caller
// must invoke the returned stop function when its HTTP stream closes.
func (b *InboxRealtimeBus) SubscribeInboxChanges(userID int64) (<-chan domain.InboxChangedIntent, func()) {
	if b == nil || b.hub == nil || userID <= 0 {
		closed := make(chan domain.InboxChangedIntent)
		close(closed)
		return closed, func() {}
	}
	return b.hub.Subscribe(userID)
}

func (b *InboxRealtimeBus) consume(ctx context.Context) {
	for {
		if ctx == nil || ctx.Err() != nil {
			return
		}
		pubsub := b.client.Subscribe(ctx, b.channel)
		if _, err := pubsub.ReceiveTimeout(ctx, inboxRealtimePublishWait); err != nil {
			_ = pubsub.Close()
			b.log.Warn("notification_inbox_realtime_subscribe_failed", zap.Error(err))
			if !waitInboxRealtimeRetry(ctx) {
				return
			}
			continue
		}
		b.readyMu.Do(func() { close(b.ready) })
		messages := pubsub.Channel(redisclient.WithChannelSize(inboxRealtimeChannelSize))
		reconnect := false
		for !reconnect {
			select {
			case <-ctx.Done():
				_ = pubsub.Close()
				return
			case message, ok := <-messages:
				if !ok {
					reconnect = true
					break
				}
				var intent domain.InboxChangedIntent
				if err := json.Unmarshal([]byte(message.Payload), &intent); err != nil {
					b.log.Warn("notification_inbox_realtime_payload_invalid", zap.Error(err))
					continue
				}
				if err := b.validate(intent); err != nil {
					b.log.Warn("notification_inbox_realtime_hint_rejected", zap.Error(err))
					continue
				}
				b.hub.Publish(intent)
			}
		}
		_ = pubsub.Close()
		if !waitInboxRealtimeRetry(ctx) {
			return
		}
	}
}

func (b *InboxRealtimeBus) validate(intent domain.InboxChangedIntent) error {
	if strings.TrimSpace(intent.ScopeID) == "" || intent.ScopeID != b.scopeID {
		return errors.New("notification inbox realtime scope is invalid")
	}
	if intent.UserID <= 0 || intent.ChangeSequence <= 0 {
		return errors.New("notification inbox realtime hint is invalid")
	}
	return nil
}

func waitInboxRealtimeRetry(ctx context.Context) bool {
	timer := time.NewTimer(inboxRealtimeRetryWait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func inboxRealtimeChannel(keyPrefix, scopeID string) string {
	prefix := strings.Trim(strings.TrimSpace(keyPrefix), ":")
	if prefix == "" {
		prefix = "seven"
	}
	digest := sha256.Sum256([]byte(strings.TrimSpace(scopeID)))
	return prefix + ":notification:inbox:changed:" + hex.EncodeToString(digest[:12])
}

type inboxRealtimeHub struct {
	mu          sync.Mutex
	subscribers map[int64]map[*inboxRealtimeSubscription]struct{}
}

type inboxRealtimeSubscription struct {
	events       chan domain.InboxChangedIntent
	lastSequence int64
}

func newInboxRealtimeHub() *inboxRealtimeHub {
	return &inboxRealtimeHub{subscribers: make(map[int64]map[*inboxRealtimeSubscription]struct{})}
}

func (h *inboxRealtimeHub) Subscribe(userID int64) (<-chan domain.InboxChangedIntent, func()) {
	if h == nil || userID <= 0 {
		closed := make(chan domain.InboxChangedIntent)
		close(closed)
		return closed, func() {}
	}
	subscription := &inboxRealtimeSubscription{events: make(chan domain.InboxChangedIntent, 1)}
	h.mu.Lock()
	if h.subscribers[userID] == nil {
		h.subscribers[userID] = make(map[*inboxRealtimeSubscription]struct{})
	}
	h.subscribers[userID][subscription] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	stop := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if group := h.subscribers[userID]; group != nil {
				delete(group, subscription)
				if len(group) == 0 {
					delete(h.subscribers, userID)
				}
			}
			close(subscription.events)
		})
	}
	return subscription.events, stop
}

func (h *inboxRealtimeHub) Publish(intent domain.InboxChangedIntent) {
	if h == nil || intent.UserID <= 0 || intent.ChangeSequence <= 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for subscription := range h.subscribers[intent.UserID] {
		if intent.ChangeSequence <= subscription.lastSequence {
			continue
		}
		subscription.lastSequence = intent.ChangeSequence
		select {
		case subscription.events <- intent:
		default:
			// Keep the newest sequence if a slow SSE writer has not consumed the
			// prior hint. Preserve newUnread so coalescing cannot suppress the
			// user's only generic notification prompt.
			previous := <-subscription.events
			intent.NewUnread = intent.NewUnread || previous.NewUnread
			subscription.events <- intent
		}
	}
}
